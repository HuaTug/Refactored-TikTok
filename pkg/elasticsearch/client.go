package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ESConfig Elasticsearch 配置
type ESConfig struct {
	Addresses   []string // ES 节点地址列表
	Username    string   // 用户名
	Password    string   // 密码
	IndexPrefix string   // 索引前缀
	MaxRetries  int      // 最大重试次数
	EnableSniff bool     // 是否启用节点嗅探
}

// Client Elasticsearch 客户端封装
type Client struct {
	client      *elasticsearch.Client
	config      *ESConfig
	mu          sync.RWMutex
	initialized bool
}

var (
	defaultClient *Client
	once          sync.Once
)

// GetClient 获取默认 ES 客户端
func GetClient() *Client {
	once.Do(func() {
		defaultClient = &Client{}
	})
	return defaultClient
}

// Init 初始化 ES 客户端
func (c *Client) Init(config *ESConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	if len(config.Addresses) == 0 {
		hlog.Warn("[ES Client] No addresses configured, ES client disabled")
		return nil
	}

	cfg := elasticsearch.Config{
		Addresses:  config.Addresses,
		MaxRetries: config.MaxRetries,
	}

	if config.Username != "" && config.Password != "" {
		cfg.Username = config.Username
		cfg.Password = config.Password
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create ES client: %w", err)
	}

	// 测试连接
	res, err := client.Info()
	if err != nil {
		return fmt.Errorf("failed to connect to ES: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ES connection error: %s", res.String())
	}

	c.client = client
	c.config = config
	c.initialized = true

	hlog.Infof("[ES Client] Connected to Elasticsearch: %v", config.Addresses)
	return nil
}

// IsInitialized 检查是否已初始化
func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

// GetIndexName 获取带前缀和日期的索引名
func (c *Client) GetIndexName(indexType string) string {
	date := time.Now().Format("2006.01.02")
	return fmt.Sprintf("%s-%s-%s", c.config.IndexPrefix, indexType, date)
}

// GetIndexPattern 获取索引模式 (用于查询)
func (c *Client) GetIndexPattern(indexType string) string {
	return fmt.Sprintf("%s-%s-*", c.config.IndexPrefix, indexType)
}

// IndexDocument 索引单个文档
func (c *Client) IndexDocument(ctx context.Context, indexType string, docID string, document interface{}) error {
	if !c.IsInitialized() {
		return fmt.Errorf("ES client not initialized")
	}

	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	indexName := c.GetIndexName(indexType)

	req := esapi.IndexRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(data),
		Refresh:    "false", // 异步刷新，提高写入性能
	}

	res, err := req.Do(ctx, c.client)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index error: %s", res.String())
	}

	return nil
}

// BulkIndex 批量索引文档
func (c *Client) BulkIndex(ctx context.Context, indexType string, documents []BulkDocument) error {
	if !c.IsInitialized() {
		return fmt.Errorf("ES client not initialized")
	}

	if len(documents) == 0 {
		return nil
	}

	indexName := c.GetIndexName(indexType)
	var buf bytes.Buffer

	for _, doc := range documents {
		// 写入 action 行
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
			},
		}
		if doc.ID != "" {
			meta["index"].(map[string]interface{})["_id"] = doc.ID
		}

		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal meta: %w", err)
		}
		buf.Write(metaBytes)
		buf.WriteByte('\n')

		// 写入文档行
		docBytes, err := json.Marshal(doc.Document)
		if err != nil {
			return fmt.Errorf("failed to marshal document: %w", err)
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
	}

	res, err := c.client.Bulk(bytes.NewReader(buf.Bytes()), c.client.Bulk.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to bulk index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("bulk index error: %s", res.String())
	}

	// 解析响应检查是否有错误
	var bulkRes BulkResponse
	if err := json.NewDecoder(res.Body).Decode(&bulkRes); err != nil {
		return fmt.Errorf("failed to parse bulk response: %w", err)
	}

	if bulkRes.Errors {
		// 统计错误数量
		errorCount := 0
		for _, item := range bulkRes.Items {
			if item.Index.Error.Type != "" {
				errorCount++
				hlog.Warnf("[ES Client] Bulk index item error: %s - %s",
					item.Index.Error.Type, item.Index.Error.Reason)
			}
		}
		return fmt.Errorf("bulk index completed with %d errors", errorCount)
	}

	return nil
}

// Search 搜索文档
func (c *Client) Search(ctx context.Context, indexType string, query map[string]interface{}) (*SearchResult, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("ES client not initialized")
	}

	indexPattern := c.GetIndexPattern(indexType)

	queryBytes, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	res, err := c.client.Search(
		c.client.Search.WithContext(ctx),
		c.client.Search.WithIndex(indexPattern),
		c.client.Search.WithBody(bytes.NewReader(queryBytes)),
		c.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search error: %s", res.String())
	}

	var result SearchResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse search result: %w", err)
	}

	return &result, nil
}

// CreateIndexTemplate 创建索引模板
func (c *Client) CreateIndexTemplate(ctx context.Context, templateName string, template IndexTemplate) error {
	if !c.IsInitialized() {
		return fmt.Errorf("ES client not initialized")
	}

	templateBytes, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	res, err := c.client.Indices.PutIndexTemplate(
		templateName,
		bytes.NewReader(templateBytes),
		c.client.Indices.PutIndexTemplate.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("failed to create index template: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("create index template error: %s", res.String())
	}

	hlog.Infof("[ES Client] Created index template: %s", templateName)
	return nil
}

// DeleteOldIndices 删除旧索引 (用于日志轮转)
func (c *Client) DeleteOldIndices(ctx context.Context, indexType string, retentionDays int) error {
	if !c.IsInitialized() {
		return fmt.Errorf("ES client not initialized")
	}

	// 获取所有匹配的索引
	indexPattern := c.GetIndexPattern(indexType)
	res, err := c.client.Cat.Indices(
		c.client.Cat.Indices.WithContext(ctx),
		c.client.Cat.Indices.WithIndex(indexPattern),
		c.client.Cat.Indices.WithFormat("json"),
	)
	if err != nil {
		return fmt.Errorf("failed to list indices: %w", err)
	}
	defer res.Body.Close()

	var indices []CatIndex
	if err := json.NewDecoder(res.Body).Decode(&indices); err != nil {
		return fmt.Errorf("failed to parse indices: %w", err)
	}

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	indicesToDelete := make([]string, 0)

	for _, idx := range indices {
		// 从索引名中解析日期
		parts := strings.Split(idx.Index, "-")
		if len(parts) < 3 {
			continue
		}
		dateStr := parts[len(parts)-1]
		indexDate, err := time.Parse("2006.01.02", dateStr)
		if err != nil {
			continue
		}

		if indexDate.Before(cutoffDate) {
			indicesToDelete = append(indicesToDelete, idx.Index)
		}
	}

	if len(indicesToDelete) == 0 {
		return nil
	}

	// 删除旧索引
	deleteRes, err := c.client.Indices.Delete(
		indicesToDelete,
		c.client.Indices.Delete.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("failed to delete indices: %w", err)
	}
	defer deleteRes.Body.Close()

	if deleteRes.IsError() {
		return fmt.Errorf("delete indices error: %s", deleteRes.String())
	}

	hlog.Infof("[ES Client] Deleted %d old indices for %s", len(indicesToDelete), indexType)
	return nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.initialized = false
	c.client = nil
	hlog.Info("[ES Client] Closed")
	return nil
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	if !c.IsInitialized() {
		return fmt.Errorf("ES client not initialized")
	}

	res, err := c.client.Cluster.Health(
		c.client.Cluster.Health.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("health check error: %s", res.String())
	}

	return nil
}

// ============ 类型定义 ============

// BulkDocument 批量索引文档
type BulkDocument struct {
	ID       string      // 文档ID
	Document interface{} // 文档内容
}

// BulkResponse 批量操作响应
type BulkResponse struct {
	Took   int  `json:"took"`
	Errors bool `json:"errors"`
	Items  []struct {
		Index struct {
			ID     string `json:"_id"`
			Result string `json:"result"`
			Status int    `json:"status"`
			Error  struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
			} `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore float64 `json:"max_score"`
		Hits     []struct {
			Index  string                 `json:"_index"`
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// CatIndex 索引信息
type CatIndex struct {
	Health       string `json:"health"`
	Status       string `json:"status"`
	Index        string `json:"index"`
	UUID         string `json:"uuid"`
	Primary      string `json:"pri"`
	Replica      string `json:"rep"`
	DocsCount    string `json:"docs.count"`
	DocsDeleted  string `json:"docs.deleted"`
	StoreSize    string `json:"store.size"`
	PriStoreSize string `json:"pri.store.size"`
}

// IndexTemplate 索引模板
type IndexTemplate struct {
	IndexPatterns []string               `json:"index_patterns"`
	Template      TemplateSettings       `json:"template"`
	Priority      int                    `json:"priority"`
	ComposedOf    []string               `json:"composed_of,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

// TemplateSettings 模板设置
type TemplateSettings struct {
	Settings map[string]interface{} `json:"settings"`
	Mappings map[string]interface{} `json:"mappings"`
}
