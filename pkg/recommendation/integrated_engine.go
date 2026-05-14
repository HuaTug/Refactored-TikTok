package recommendation

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// =====================================================
// 集成 DeepCTR 精排的推荐引擎
// 将 CTR 预估服务与现有召回层集成
// =====================================================

// IntegratedRecommendConfig 集成推荐配置
type IntegratedRecommendConfig struct {
	// 召回配置
	MaxRecallCandidates int `json:"max_recall_candidates"` // 最大召回数

	// CTR 精排配置
	EnableCTRRanking bool   `json:"enable_ctr_ranking"` // 是否启用 CTR 精排
	CTRServiceURL    string `json:"ctr_service_url"`    // CTR 服务地址
	CTRTimeout       int    `json:"ctr_timeout_ms"`     // CTR 超时 (毫秒)
	CTRModel         string `json:"ctr_model"`          // 使用的模型

	// 重排序配置
	DiversityLambda  float64 `json:"diversity_lambda"`  // 多样性权重
	ExplorationRatio float64 `json:"exploration_ratio"` // 探索比例

	// 性能配置
	MaxConcurrency int `json:"max_concurrency"` // 最大并发
}

// DefaultIntegratedConfig 默认配置
func DefaultIntegratedConfig() *IntegratedRecommendConfig {
	return &IntegratedRecommendConfig{
		MaxRecallCandidates: 500,
		EnableCTRRanking:    true,
		CTRServiceURL:       "http://localhost:8000",
		CTRTimeout:          2000,
		CTRModel:            "deepfm",
		DiversityLambda:     0.7,
		ExplorationRatio:    0.1,
		MaxConcurrency:      10,
	}
}

// IntegratedRecommendationEngine 集成推荐引擎
// 召回 (Go) -> 粗排 (Go) -> 精排 (Python CTR) -> 重排序 (Go)
type IntegratedRecommendationEngine struct {
	config       *IntegratedRecommendConfig
	recallEngine *RecommendationEngine // 现有召回引擎
	ctrClient    *CTRServiceClient     // CTR 服务客户端
	rankingModel *EnhancedRankingModel // 粗排模型
}

// NewIntegratedRecommendationEngine 创建集成推荐引擎
func NewIntegratedRecommendationEngine(
	config *IntegratedRecommendConfig,
	recallEngine *RecommendationEngine,
	redisClient *redis.Client,
	database *gorm.DB,
) *IntegratedRecommendationEngine {
	if config == nil {
		config = DefaultIntegratedConfig()
	}

	engine := &IntegratedRecommendationEngine{
		config:       config,
		recallEngine: recallEngine,
	}

	// 初始化 CTR 客户端
	if config.EnableCTRRanking {
		ctrConfig := &CTRServiceConfig{
			ServiceURL:   config.CTRServiceURL,
			Timeout:      time.Duration(config.CTRTimeout) * time.Millisecond,
			DefaultModel: config.CTRModel,
		}
		engine.ctrClient = NewCTRServiceClient(ctrConfig)
	}

	// 初始化粗排模型
	engine.rankingModel = NewEnhancedRankingModel(redisClient, database)

	return engine
}

// Recommend 生成推荐列表
// 完整推荐流程: 召回 -> 粗排 -> 精排(CTR) -> 重排序
func (e *IntegratedRecommendationEngine) Recommend(
	ctx context.Context,
	req *RecommendRequest,
) (*RecommendResponse, error) {
	startTime := time.Now()

	hlog.Infof("[IntegratedEngine] Start recommend for user %d, limit %d", req.UserID, req.Limit)

	// ========================================
	// 1. 多路召回
	// ========================================
	recallStartTime := time.Now()

	candidateIDs, recallStats, err := e.multiChannelRecall(ctx, req.UserID, e.config.MaxRecallCandidates)
	if err != nil {
		hlog.Errorf("[IntegratedEngine] Recall failed: %v", err)
		return nil, err
	}

	hlog.Infof("[IntegratedEngine] Recall completed: %d candidates, took %v",
		len(candidateIDs), time.Since(recallStartTime))

	if len(candidateIDs) == 0 {
		return &RecommendResponse{
			Videos:      []ScoredVideo{},
			RequestID:   req.RequestID,
			RecallStats: recallStats,
		}, nil
	}

	// ========================================
	// 2. 粗排 (基于规则和简单特征)
	// ========================================
	coarseStartTime := time.Now()

	// 粗排: 快速筛选 Top 200
	coarseLimit := 200
	if coarseLimit > len(candidateIDs) {
		coarseLimit = len(candidateIDs)
	}

	coarseRanked, err := e.coarseRanking(ctx, req.UserID, candidateIDs, coarseLimit)
	if err != nil {
		hlog.Warnf("[IntegratedEngine] Coarse ranking failed: %v, using original order", err)
		coarseRanked = candidateIDs[:coarseLimit]
	}

	hlog.Infof("[IntegratedEngine] Coarse ranking completed: %d -> %d, took %v",
		len(candidateIDs), len(coarseRanked), time.Since(coarseStartTime))

	// ========================================
	// 3. 精排 (CTR 预估)
	// ========================================
	var fineRanked []ScoredVideo

	if e.config.EnableCTRRanking && e.ctrClient != nil && e.ctrClient.IsHealthy() {
		fineStartTime := time.Now()

		// 构建上下文
		ctxInfo := map[string]string{}
		if req.Context != nil {
			ctxInfo = req.Context
		}

		// 调用 CTR 服务
		predictions, err := e.ctrClient.Predict(ctx, req.UserID, coarseRanked, ctxInfo)
		if err != nil {
			hlog.Warnf("[IntegratedEngine] CTR predict failed: %v, using coarse scores", err)
			// 降级: 使用粗排结果
			fineRanked = e.convertToScoredVideos(coarseRanked)
		} else {
			// 转换为 ScoredVideo
			fineRanked = make([]ScoredVideo, len(predictions))
			for i, pred := range predictions {
				fineRanked[i] = PredictionToScoredVideo(pred)
			}

			// 按 CTR 分数排序
			fineRanked = e.sortByScore(fineRanked)
		}

		hlog.Infof("[IntegratedEngine] Fine ranking (CTR) completed: took %v", time.Since(fineStartTime))
	} else {
		// CTR 服务不可用，使用粗排结果
		fineRanked = e.convertToScoredVideos(coarseRanked)
	}

	// ========================================
	// 3.5 个性化 Boost：对 CTR 分数应用基于用户实时偏好的乘子
	//     这一步把"用户喜欢的分类/topic"信号显式注入到最终排序，
	//     避免 DeepCTR 仅基于全局视频热度排序而忽略个性化。
	// ========================================
	fineRanked = e.applyPersonalizedBoost(ctx, req.UserID, fineRanked)
	fineRanked = e.sortByScore(fineRanked)

	// ========================================
	// 4. 重排序 (多样性、探索)
	// ========================================
	rerankStartTime := time.Now()

	// MMR 重排序保证多样性
	reranked := e.rerankMMR(fineRanked, req.Limit, e.config.DiversityLambda)

	// 注入探索性视频
	reranked = e.injectExploration(ctx, req.UserID, reranked, req.Limit)

	hlog.Infof("[IntegratedEngine] Reranking completed: %d videos, took %v",
		len(reranked), time.Since(rerankStartTime))

	// 截取最终结果
	finalResults := reranked
	if len(finalResults) > req.Limit {
		finalResults = finalResults[:req.Limit]
	}

	totalLatency := time.Since(startTime)
	hlog.Infof("[IntegratedEngine] Recommend completed: %d videos, total latency %v",
		len(finalResults), totalLatency)

	return &RecommendResponse{
		Videos:        finalResults,
		RequestID:     req.RequestID,
		RecallStats:   recallStats,
		CandidateSize: len(candidateIDs),
		LatencyMs:     totalLatency.Milliseconds(),
	}, nil
}

// multiChannelRecall 多路召回
func (e *IntegratedRecommendationEngine) multiChannelRecall(
	ctx context.Context,
	userID int64,
	limit int,
) ([]int64, map[string]int, error) {
	recallStats := make(map[string]int)
	candidateSet := make(map[int64]bool)

	if e.recallEngine == nil {
		return nil, recallStats, nil
	}

	// 使用现有召回引擎的策略
	for _, strategy := range e.recallEngine.recallStrategies {
		strategyLimit := int(float64(limit) * strategy.Weight())
		videos, err := strategy.Recall(ctx, userID, strategyLimit)
		if err != nil {
			hlog.Warnf("[IntegratedEngine] Recall %s failed: %v", strategy.Name(), err)
			continue
		}

		recallStats[strategy.Name()] = len(videos)
		for _, vid := range videos {
			candidateSet[vid] = true
		}
	}

	// 转为切片
	candidates := make([]int64, 0, len(candidateSet))
	for vid := range candidateSet {
		candidates = append(candidates, vid)
	}

	return candidates, recallStats, nil
}

// coarseRanking 粗排
func (e *IntegratedRecommendationEngine) coarseRanking(
	ctx context.Context,
	userID int64,
	candidates []int64,
	limit int,
) ([]int64, error) {
	if e.rankingModel == nil {
		return candidates[:limit], nil
	}

	// 使用粗排模型
	scored, err := e.rankingModel.Rank(ctx, userID, candidates)
	if err != nil {
		return nil, err
	}

	// 取 Top N
	result := make([]int64, 0, limit)
	for i := 0; i < limit && i < len(scored); i++ {
		result = append(result, scored[i].VideoID)
	}

	return result, nil
}

// applyPersonalizedBoost 给 CTR 排序后的视频应用基于用户实时偏好的乘子。
//
// 规则：
//   - 视频分类命中 user.focused_topic（实时焦点）   -> Score *= 1.5
//   - 视频分类命中 user.category_prefer 强偏好(>0.2) -> Score *= 1.3
//   - 视频分类命中 user.category_prefer 弱偏好(<=0.2)-> Score *= 1.1
//   - 用户有偏好但视频分类不在偏好里                  -> Score *= 0.7
//   - 用户没偏好数据（冷启动）                         -> Score 不变
//
// 这样能在不重新训练 DeepCTR 的情况下，让用户实时行为立刻影响排序。
func (e *IntegratedRecommendationEngine) applyPersonalizedBoost(
	ctx context.Context, userID int64, videos []ScoredVideo,
) []ScoredVideo {
	if len(videos) == 0 || userID <= 0 || e.rankingModel == nil {
		return videos
	}

	// 复用粗排里的 getUserProfile 逻辑（读 Redis user:category_prefer / user:interests + FocusedTopic）
	profile := e.rankingModel.getUserProfile(ctx, userID)
	if profile == nil {
		return videos
	}
	hasPref := len(profile.CategoryPrefer) > 0 || profile.FocusedTopic != ""
	if !hasPref {
		hlog.Infof("[PersonalizedBoost] user_id=%d no preference data, skip", userID)
		return videos
	}

	// 一次性查所有 video category（粗排里已经 cache 过的话不会重复查 db）
	videoIDs := make([]int64, len(videos))
	for i, v := range videos {
		videoIDs[i] = v.VideoID
	}
	videoCats := e.rankingModel.batchLoadVideoCategoriesFromDB(ctx, videoIDs)

	// 统计四种 boost 命中数，方便定位"为什么没倾斜"
	var (
		cntFocused, cntStrong, cntWeak, cntPenalty int
	)

	for i := range videos {
		cat := videoCats[videos[i].VideoID]
		mult := 1.0
		switch {
		case cat == "":
			// 未知分类不动
		case profile.FocusedTopic != "" && cat == profile.FocusedTopic:
			mult = 1.5
			cntFocused++
		default:
			if w, ok := profile.CategoryPrefer[cat]; ok && w > 0.2 {
				mult = 1.3
				cntStrong++
			} else if ok && w > 0 {
				mult = 1.1
				cntWeak++
			} else {
				mult = 0.7
				cntPenalty++
			}
		}
		if mult != 1.0 {
			videos[i].Score *= mult
			if videos[i].Features == nil {
				videos[i].Features = map[string]float64{}
			}
			videos[i].Features["personalized_boost"] = mult
		}
	}

	hlog.Infof("[PersonalizedBoost] user_id=%d focused_topic=%q candidates=%d boost: focused=%d strong=%d weak=%d penalty=%d",
		userID, profile.FocusedTopic, len(videos),
		cntFocused, cntStrong, cntWeak, cntPenalty)
	return videos
}


// sortByScore 按分数排序
func (e *IntegratedRecommendationEngine) sortByScore(videos []ScoredVideo) []ScoredVideo {
	// 简单冒泡排序 (降序)
	for i := 0; i < len(videos)-1; i++ {
		for j := i + 1; j < len(videos); j++ {
			if videos[j].Score > videos[i].Score {
				videos[i], videos[j] = videos[j], videos[i]
			}
		}
	}
	return videos
}

// rerankMMR MMR 重排序
func (e *IntegratedRecommendationEngine) rerankMMR(
	videos []ScoredVideo,
	limit int,
	lambda float64,
) []ScoredVideo {
	if len(videos) <= limit {
		return videos
	}

	selected := make([]ScoredVideo, 0, limit)
	remaining := make([]ScoredVideo, len(videos))
	copy(remaining, videos)

	// 选择分数最高的作为第一个
	if len(remaining) > 0 {
		selected = append(selected, remaining[0])
		remaining = remaining[1:]
	}

	for len(selected) < limit && len(remaining) > 0 {
		maxMMR := -1e9
		maxIdx := -1

		for i, video := range remaining {
			// 计算与已选视频的最大相似度
			maxSim := 0.0
			for _, sel := range selected {
				sim := e.calculateSimilarity(video, sel)
				if sim > maxSim {
					maxSim = sim
				}
			}

			// MMR = λ * score - (1-λ) * maxSim
			mmr := lambda*video.Score - (1-lambda)*maxSim

			if mmr > maxMMR {
				maxMMR = mmr
				maxIdx = i
			}
		}

		if maxIdx >= 0 {
			selected = append(selected, remaining[maxIdx])
			remaining = append(remaining[:maxIdx], remaining[maxIdx+1:]...)
		}
	}

	return selected
}

// calculateSimilarity 计算相似度
func (e *IntegratedRecommendationEngine) calculateSimilarity(v1, v2 ScoredVideo) float64 {
	// 基于特征计算余弦相似度
	if len(v1.Features) == 0 || len(v2.Features) == 0 {
		return 0
	}

	dot := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for key, val1 := range v1.Features {
		if val2, ok := v2.Features[key]; ok {
			dot += val1 * val2
		}
		norm1 += val1 * val1
	}

	for _, val := range v2.Features {
		norm2 += val * val
	}

	if norm1 == 0 || norm2 == 0 {
		return 0
	}

	return dot / (sqrt(norm1) * sqrt(norm2))
}

// sqrt 平方根
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// injectExploration 注入探索性视频
func (e *IntegratedRecommendationEngine) injectExploration(
	ctx context.Context,
	userID int64,
	videos []ScoredVideo,
	limit int,
) []ScoredVideo {
	if e.config.ExplorationRatio <= 0 {
		return videos
	}

	// 计算探索位置数量
	exploreCount := int(float64(limit) * e.config.ExplorationRatio)
	if exploreCount == 0 {
		return videos
	}

	// Fetch exploration videos from the recall engine (recent/long-tail/random)
	var exploreVideos []ScoredVideo
	if e.recallEngine != nil {
		// Use recall engine to get candidate videos for exploration
		recallCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()

		candidates, err := e.recallEngine.RecommendSimple(recallCtx, userID, exploreCount*3)
		if err == nil && len(candidates) > 0 {
			exploreVideos = candidates

			// Apply lower scores to exploration items (they're supplementary)
			for i := range exploreVideos {
				exploreVideos[i].Score *= 0.5 // Lower weight for exploration
				exploreVideos[i].RecallSource = "exploration"
			}
		} else {
			hlog.CtxWarnf(ctx, "Failed to get exploration videos: %v", err)
		}
	}

	if len(exploreVideos) == 0 {
		return videos
	}

	// Deduplicate: remove videos already in the main list
	existingIDs := make(map[int64]bool)
	for _, v := range videos {
		existingIDs[v.VideoID] = true
	}

	var uniqueExplore []ScoredVideo
	for _, v := range exploreVideos {
		if !existingIDs[v.VideoID] {
			uniqueExplore = append(uniqueExplore, v)
			if len(uniqueExplore) >= exploreCount {
				break
			}
		}
	}

	// Interleave exploration videos at regular intervals
	if len(uniqueExplore) == 0 {
		return videos
	}

	result := make([]ScoredVideo, 0, len(videos)+len(uniqueExplore))
	interval := len(videos) / (len(uniqueExplore) + 1)
	if interval < 3 {
		interval = 3 // Minimum gap between exploration items
	}

	exploreIdx := 0
	for i, v := range videos {
		result = append(result, v)
		if exploreIdx < len(uniqueExplore) && (i+1)%interval == 0 {
			result = append(result, uniqueExplore[exploreIdx])
			exploreIdx++
		}
	}
	// Append remaining exploration videos at the end
	for ; exploreIdx < len(uniqueExplore); exploreIdx++ {
		result = append(result, uniqueExplore[exploreIdx])
	}

	return result
}

// convertToScoredVideos 转换为 ScoredVideo
func (e *IntegratedRecommendationEngine) convertToScoredVideos(videoIDs []int64) []ScoredVideo {
	result := make([]ScoredVideo, len(videoIDs))
	for i, vid := range videoIDs {
		result[i] = ScoredVideo{
			VideoID: vid,
			Score:   float64(len(videoIDs)-i) / float64(len(videoIDs)), // 保持原有顺序的分数
		}
	}
	return result
}

// =====================================================
// 快捷方法
// =====================================================

// QuickRecommend 快速推荐 (使用默认配置)
func QuickRecommend(
	ctx context.Context,
	userID int64,
	limit int,
	recallEngine *RecommendationEngine,
) ([]ScoredVideo, error) {
	engine := NewIntegratedRecommendationEngine(nil, recallEngine, nil, nil)

	resp, err := engine.Recommend(ctx, &RecommendRequest{
		UserID: userID,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}

	return resp.Videos, nil
}
