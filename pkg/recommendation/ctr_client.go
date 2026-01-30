package recommendation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// =====================================================
// CTR 预估服务客户端
// 与 Python DeepCTR 服务交互
// =====================================================

// CTRServiceConfig CTR 服务配置
type CTRServiceConfig struct {
	// 服务地址
	ServiceURL string `json:"service_url"`

	// 超时配置
	Timeout time.Duration `json:"timeout"`

	// 重试配置
	MaxRetries int           `json:"max_retries"`
	RetryDelay time.Duration `json:"retry_delay"`

	// 模型配置
	DefaultModel string `json:"default_model"` // deepfm/din/mmoe

	// 是否启用集成预测
	EnableEnsemble bool `json:"enable_ensemble"`

	// 连接池配置
	MaxIdleConns    int `json:"max_idle_conns"`
	MaxConnsPerHost int `json:"max_conns_per_host"`
}

// DefaultCTRServiceConfig 默认配置
func DefaultCTRServiceConfig() *CTRServiceConfig {
	return &CTRServiceConfig{
		ServiceURL:      "http://localhost:8000",
		Timeout:         200 * time.Millisecond,
		MaxRetries:      2,
		RetryDelay:      50 * time.Millisecond,
		DefaultModel:    "deepfm",
		EnableEnsemble:  false,
		MaxIdleConns:    100,
		MaxConnsPerHost: 50,
	}
}

// CTRPredictRequest CTR 预测请求
type CTRPredictRequest struct {
	UserID   int64             `json:"user_id"`
	VideoIDs []int64           `json:"video_ids"`
	Context  map[string]string `json:"context,omitempty"`
	Model    string            `json:"model,omitempty"`
}

// CTRPrediction 单条预测结果
type CTRPrediction struct {
	VideoID   int64   `json:"video_id"`
	Score     float64 `json:"score"`      // 综合分数
	CTR       float64 `json:"ctr"`        // 点击率
	IsFinish  float64 `json:"is_finish"`  // 完播率 (MMoE)
	IsLike    float64 `json:"is_like"`    // 点赞率 (MMoE)
	IsShare   float64 `json:"is_share"`   // 分享率 (MMoE)
}

// CTRPredictResponse CTR 预测响应
type CTRPredictResponse struct {
	Predictions []CTRPrediction `json:"predictions"`
	LatencyMs   float64         `json:"latency_ms"`
	Model       string          `json:"model"`
	ModelsUsed  []string        `json:"models_used,omitempty"` // 集成预测
}

// CTRServiceClient CTR 服务客户端
type CTRServiceClient struct {
	config     *CTRServiceConfig
	httpClient *http.Client
	mu         sync.RWMutex
	isHealthy  bool
}

// NewCTRServiceClient 创建 CTR 服务客户端
func NewCTRServiceClient(config *CTRServiceConfig) *CTRServiceClient {
	if config == nil {
		config = DefaultCTRServiceConfig()
	}

	transport := &http.Transport{
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &CTRServiceClient{
		config: config,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		isHealthy: true,
	}

	// 启动健康检查
	go client.startHealthCheck()

	return client
}

// Predict 预测 CTR 分数
func (c *CTRServiceClient) Predict(
	ctx context.Context,
	userID int64,
	videoIDs []int64,
	ctxInfo map[string]string,
) ([]CTRPrediction, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}

	// 检查服务是否健康
	if !c.IsHealthy() {
		hlog.Warn("[CTRClient] Service unhealthy, using fallback scoring")
		return c.fallbackScoring(videoIDs), nil
	}

	// 构建请求
	req := CTRPredictRequest{
		UserID:   userID,
		VideoIDs: videoIDs,
		Context:  ctxInfo,
		Model:    c.config.DefaultModel,
	}

	// 选择接口
	var endpoint string
	if c.config.EnableEnsemble {
		endpoint = "/predict/ensemble"
	} else {
		endpoint = "/predict"
	}

	// 发送请求
	resp, err := c.doRequest(ctx, endpoint, req)
	if err != nil {
		hlog.Warnf("[CTRClient] Predict failed: %v, using fallback", err)
		return c.fallbackScoring(videoIDs), nil
	}

	return resp.Predictions, nil
}

// PredictWithModel 使用指定模型预测
func (c *CTRServiceClient) PredictWithModel(
	ctx context.Context,
	userID int64,
	videoIDs []int64,
	modelName string,
) ([]CTRPrediction, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}

	req := CTRPredictRequest{
		UserID:   userID,
		VideoIDs: videoIDs,
		Model:    modelName,
	}

	resp, err := c.doRequest(ctx, "/predict", req)
	if err != nil {
		return c.fallbackScoring(videoIDs), nil
	}

	return resp.Predictions, nil
}

// doRequest 发送 HTTP 请求
func (c *CTRServiceClient) doRequest(
	ctx context.Context,
	endpoint string,
	req CTRPredictRequest,
) (*CTRPredictResponse, error) {
	url := c.config.ServiceURL + endpoint

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	var lastErr error
	for retry := 0; retry <= c.config.MaxRetries; retry++ {
		if retry > 0 {
			time.Sleep(c.config.RetryDelay)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("read response failed: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("bad status: %d, body: %s", resp.StatusCode, string(respBody))
			continue
		}

		var result CTRPredictResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			lastErr = fmt.Errorf("unmarshal response failed: %w", err)
			continue
		}

		return &result, nil
	}

	return nil, lastErr
}

// fallbackScoring 降级打分 (当 CTR 服务不可用时)
func (c *CTRServiceClient) fallbackScoring(videoIDs []int64) []CTRPrediction {
	predictions := make([]CTRPrediction, len(videoIDs))
	for i, vid := range videoIDs {
		predictions[i] = CTRPrediction{
			VideoID: vid,
			Score:   0.5, // 默认中等分数
			CTR:     0.5,
		}
	}
	return predictions
}

// ========================================
// 健康检查
// ========================================

// startHealthCheck 启动健康检查
func (c *CTRServiceClient) startHealthCheck() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.checkHealth()
	}
}

// checkHealth 检查服务健康状态
func (c *CTRServiceClient) checkHealth() {
	url := c.config.ServiceURL + "/health"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		c.setHealthy(false)
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.setHealthy(false)
		hlog.Warnf("[CTRClient] Health check failed: %v", err)
		return
	}
	resp.Body.Close()

	healthy := resp.StatusCode == http.StatusOK
	c.setHealthy(healthy)

	if !healthy {
		hlog.Warnf("[CTRClient] Service unhealthy, status: %d", resp.StatusCode)
	}
}

// setHealthy 设置健康状态
func (c *CTRServiceClient) setHealthy(healthy bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isHealthy = healthy
}

// IsHealthy 检查是否健康
func (c *CTRServiceClient) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isHealthy
}

// ========================================
// 工具函数
// ========================================

// SortByScore 按分数排序
func SortByScore(predictions []CTRPrediction) []CTRPrediction {
	// 简单冒泡排序 (降序)
	for i := 0; i < len(predictions)-1; i++ {
		for j := i + 1; j < len(predictions); j++ {
			if predictions[j].Score > predictions[i].Score {
				predictions[i], predictions[j] = predictions[j], predictions[i]
			}
		}
	}
	return predictions
}

// GetTopN 获取 Top N 视频 ID
func GetTopN(predictions []CTRPrediction, n int) []int64 {
	sorted := SortByScore(predictions)
	if n > len(sorted) {
		n = len(sorted)
	}

	result := make([]int64, n)
	for i := 0; i < n; i++ {
		result[i] = sorted[i].VideoID
	}
	return result
}

// PredictionToScoredVideo 转换为 ScoredVideo
func PredictionToScoredVideo(pred CTRPrediction) ScoredVideo {
	return ScoredVideo{
		VideoID: pred.VideoID,
		Score:   pred.Score,
		Features: map[string]float64{
			"ctr":       pred.CTR,
			"is_finish": pred.IsFinish,
			"is_like":   pred.IsLike,
			"is_share":  pred.IsShare,
		},
		Reasons: generateReasons(pred),
	}
}

// generateReasons 生成推荐理由
func generateReasons(pred CTRPrediction) []string {
	var reasons []string

	if pred.CTR > 0.8 {
		reasons = append(reasons, "高点击率内容")
	}
	if pred.IsFinish > 0.7 {
		reasons = append(reasons, "完播率高")
	}
	if pred.IsLike > 0.5 {
		reasons = append(reasons, "用户喜爱")
	}

	return reasons
}

// =====================================================
// 全局客户端实例
// =====================================================

var (
	globalCTRClient *CTRServiceClient
	ctrClientOnce   sync.Once
)

// InitCTRClient 初始化全局 CTR 客户端
func InitCTRClient(config *CTRServiceConfig) {
	ctrClientOnce.Do(func() {
		globalCTRClient = NewCTRServiceClient(config)
		hlog.Info("[CTRClient] Global CTR client initialized")
	})
}

// GetCTRClient 获取全局 CTR 客户端
func GetCTRClient() *CTRServiceClient {
	if globalCTRClient == nil {
		// 使用默认配置初始化
		InitCTRClient(DefaultCTRServiceConfig())
	}
	return globalCTRClient
}
