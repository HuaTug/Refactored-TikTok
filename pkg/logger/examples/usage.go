package examples

/*
日志系统使用示例

本文档展示如何在业务代码中使用基于 Kafka + Elasticsearch 的日志系统。

## 架构概览

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                              日志系统架构                                        │
│                                                                                 │
│   ┌───────────────┐      ┌─────────────────┐      ┌─────────────────────────┐   │
│   │  业务服务      │ ───▶ │  LogCollector   │ ───▶ │    Kafka Topics         │   │
│   │ (HTTP 请求)   │      │  (日志采集器)    │      │  • service_log          │   │
│   └───────────────┘      └─────────────────┘      │  • error_log            │   │
│                                                   │  • access_log           │   │
│                                                   │  • audit_log            │   │
│                                                   │  • alert_log            │   │
│                                                   └───────────┬─────────────┘   │
│                                                               │                 │
│                                                               ▼                 │
│                                                   ┌─────────────────────────┐   │
│                                                   │    LogConsumer          │   │
│                                                   │  (日志消费者)            │   │
│                                                   │  • 批量写入              │   │
│                                                   │  • 定时刷新              │   │
│                                                   └───────────┬─────────────┘   │
│                                                               │                 │
│                                                               ▼                 │
│                                                   ┌─────────────────────────┐   │
│                                                   │    Elasticsearch        │   │
│                                                   │  • 日志存储              │   │
│                                                   │  • 日志查询              │   │
│                                                   │  • 日志分析              │   │
│                                                   └─────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## 日志类型

| 日志类型 | Kafka Topic | ES 索引 | 用途 |
|---------|-------------|---------|------|
| 服务调用日志 | service_log | tiktok-service-log-* | 记录所有 API 请求/响应 |
| 错误日志 | error_log | tiktok-error-log-* | 记录错误和异常 |
| 访问日志 | access_log | tiktok-access-log-* | 记录用户访问 |
| 审计日志 | audit_log | tiktok-audit-log-* | 记录敏感操作 |
| 告警日志 | alert_log | tiktok-alert-log-* | 记录系统告警 |

*/

import (
	"context"
	"encoding/json"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/pkg/elasticsearch"
	"HuaTug.com/pkg/kafka"
	"HuaTug.com/pkg/logger"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

// ================================
// 1. 初始化日志系统
// ================================

func InitLogSystem() (*logger.LogCollector, error) {
	// 1.1 初始化 Kafka Manager
	kafkaConfig := &kafka.KafkaConfig{
		Brokers:         config.ConfigInfo.Kafka.Brokers,
		Version:         config.ConfigInfo.Kafka.Version,
		ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
	}

	kafkaManager, err := kafka.NewManager(kafkaConfig)
	if err != nil {
		return nil, err
	}

	// 1.2 初始化 Topics
	if err := kafkaManager.InitTopics(); err != nil {
		// 不返回错误，Topics 可能已存在
	}

	// 1.3 初始化 LogCollector
	collector := logger.GetCollector()
	if err := collector.Init(kafkaManager, "video-service", "production", "v1.0.0"); err != nil {
		return nil, err
	}

	return collector, nil
}

// ================================
// 2. 在 Hertz 服务中使用日志中间件
// ================================

func SetupHertzWithLogging(collector *logger.LogCollector) *server.Hertz {
	h := server.Default()

	// 2.1 创建日志中间件
	loggingMiddleware := logger.NewLoggingMiddleware(collector, &logger.MiddlewareConfig{
		EnableRequestBody:  true,                                              // 记录请求体
		EnableResponseBody: false,                                             // 不记录响应体
		MaxBodySize:        4096,                                              // 最大记录 4KB
		SensitiveEndpoints: []string{"/api/user/login", "/api/user/register"}, // 敏感接口不记录 body
		SkipEndpoints:      []string{"/health", "/metrics"},                   // 跳过健康检查
	})

	// 2.2 使用 Recovery 中间件 (记录 panic)
	h.Use(loggingMiddleware.RecoveryMiddleware())

	// 2.3 使用日志中间件
	h.Use(loggingMiddleware.Handler())

	return h
}

// ================================
// 3. 手动记录日志
// ================================

// 3.1 记录服务调用失败日志
func ExampleLogServiceError(ctx context.Context, collector *logger.LogCollector) {
	// 当服务调用失败时，记录详细的错误日志
	collector.LogServiceCall(ctx, &logger.ServiceCallLog{
		TraceID:      "trace-123456",
		MethodName:   "CreateVideo",
		Endpoint:     "/api/video/create",
		HTTPMethod:   "POST",
		StatusCode:   500,
		Success:      false,
		ErrorCode:    "VIDEO_CREATE_FAILED",
		ErrorMessage: "Failed to upload video to storage",
		UserID:       10001,
		ClientIP:     "192.168.1.100",
		Duration:     1500, // 1.5 秒
		Extra: map[string]string{
			"video_size": "10485760",
			"format":     "mp4",
		},
	})
}

// 3.2 记录服务调用成功日志
func ExampleLogServiceSuccess(ctx context.Context, collector *logger.LogCollector) {
	collector.LogServiceSuccess(ctx, "GetVideo", "/api/video/123", "GET", 200, 10001, "192.168.1.100", 50)
}

// 3.3 记录错误日志
func ExampleLogError(ctx context.Context, collector *logger.LogCollector) {
	collector.LogError(ctx, &logger.ErrorLog{
		TraceID:      "trace-123456",
		MethodName:   "ProcessVideo",
		ErrorCode:    "TRANSCODE_FAILED",
		ErrorType:    "system", // panic/business/system/network
		ErrorMessage: "Video transcoding failed: codec not supported",
		Level:        kafka.LogLevelError,
		UserID:       10001,
		ClientIP:     "192.168.1.100",
		Context: map[string]string{
			"video_id":  "12345",
			"codec":     "av1",
			"worker_id": "worker-3",
		},
		Cause: "FFmpeg returned error code -1",
	})
}

// 3.4 记录审计日志
func ExampleLogAudit(ctx context.Context, collector *logger.LogCollector) {
	// 记录敏感操作，如删除视频、修改用户信息等
	collector.LogAudit(ctx, &logger.AuditLog{
		TraceID:    "trace-123456",
		UserID:     10001,                                          // 操作者
		TargetID:   12345,                                          // 被操作对象 ID
		TargetType: "video",                                        // 对象类型
		Action:     "delete",                                       // 操作类型
		Resource:   "video",                                        // 资源名称
		OldValue:   `{"status": "published", "title": "My Video"}`, // 旧值
		NewValue:   `{"status": "deleted"}`,                        // 新值
		ClientIP:   "192.168.1.100",
		Success:    true,
	})
}

// 3.5 记录告警日志
func ExampleLogAlert(ctx context.Context, collector *logger.LogCollector) {
	// 当系统出现异常指标时记录告警
	collector.LogAlert(ctx, &logger.AlertLog{
		AlertID:     "alert-error-rate-high",
		AlertName:   "High Error Rate",
		AlertType:   "error_rate",
		Severity:    "critical", // critical/warning/info
		MetricName:  "error_rate",
		MetricValue: 15.5, // 15.5%
		Threshold:   5.0,  // 阈值 5%
		Message:     "Error rate exceeded threshold: 15.5% > 5%",
		Status:      "firing", // firing/resolved
		Labels: map[string]string{
			"service":  "video-service",
			"endpoint": "/api/video/upload",
		},
		Annotations: map[string]string{
			"summary":     "Video upload error rate is too high",
			"description": "The error rate for video uploads has exceeded 5% in the last 5 minutes",
		},
	})
}

// ================================
// 4. 在业务 Handler 中使用
// ================================

func VideoUploadHandler(ctx context.Context, c *app.RequestContext) {
	collector := logger.GetCollector()
	startTime := time.Now()

	// 获取 TraceID
	traceID, _ := c.Get("trace_id")
	traceIDStr, _ := traceID.(string)

	// 获取用户ID (从认证中间件设置)
	userID, _ := c.Get("user_id")
	userIDInt, _ := userID.(int64)

	// 业务逻辑...
	err := processVideoUpload(c)

	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		// 记录错误日志
		collector.LogError(ctx, &logger.ErrorLog{
			TraceID:      traceIDStr,
			MethodName:   "VideoUploadHandler",
			ErrorCode:    "UPLOAD_FAILED",
			ErrorType:    "business",
			ErrorMessage: err.Error(),
			Level:        kafka.LogLevelError,
			UserID:       userIDInt,
			ClientIP:     c.ClientIP(),
		})

		// 设置错误信息供日志中间件使用
		c.Set("error_code", "UPLOAD_FAILED")
		c.Set("error_message", err.Error())

		c.JSON(500, map[string]interface{}{
			"code":    "UPLOAD_FAILED",
			"message": err.Error(),
		})
		return
	}

	// 记录审计日志 (视频上传是重要操作)
	collector.LogAudit(ctx, &logger.AuditLog{
		TraceID:    traceIDStr,
		UserID:     userIDInt,
		TargetType: "video",
		Action:     "create",
		Resource:   "video",
		ClientIP:   c.ClientIP(),
		Success:    true,
	})

	c.JSON(200, map[string]interface{}{
		"code":    "SUCCESS",
		"message": "Video uploaded successfully",
	})

	// 注意：成功的服务调用日志由中间件自动记录
	_ = duration // 可用于额外的性能监控
}

func processVideoUpload(c *app.RequestContext) error {
	// 模拟视频上传处理
	return nil
}

// ================================
// 5. 从 ES 查询日志
// ================================

func QueryErrorLogs(ctx context.Context, esClient *elasticsearch.Client) (*elasticsearch.SearchResult, error) {
	// 查询最近1小时的错误日志
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"match": map[string]string{
							"level": "ERROR",
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]string{
								"gte": "now-1h",
							},
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]string{
					"order": "desc",
				},
			},
		},
		"size": 100,
	}

	return esClient.Search(ctx, elasticsearch.IndexTypeErrorLog, query)
}

func QueryServiceLogsByTraceID(ctx context.Context, esClient *elasticsearch.Client, traceID string) (*elasticsearch.SearchResult, error) {
	// 根据 TraceID 查询完整调用链
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]string{
				"trace_id": traceID,
			},
		},
		"sort": []map[string]interface{}{
			{
				"timestamp": map[string]string{
					"order": "asc",
				},
			},
		},
	}

	return esClient.Search(ctx, elasticsearch.IndexTypeServiceLog, query)
}

func QueryFailedRequests(ctx context.Context, esClient *elasticsearch.Client) (*elasticsearch.SearchResult, error) {
	// 查询失败的请求
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"term": map[string]bool{
							"success": false,
						},
					},
					{
						"range": map[string]interface{}{
							"timestamp": map[string]string{
								"gte": "now-24h",
							},
						},
					},
				},
			},
		},
		"aggs": map[string]interface{}{
			"by_endpoint": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "endpoint",
					"size":  10,
				},
			},
			"by_error_code": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "error_code",
					"size":  10,
				},
			},
		},
		"size": 0,
	}

	return esClient.Search(ctx, elasticsearch.IndexTypeServiceLog, query)
}

// ================================
// 6. 完整的服务启动示例
// ================================

func ExampleFullServiceSetup() {
	// 1. 初始化配置
	config.Init()

	// 2. 初始化日志系统
	collector, err := InitLogSystem()
	if err != nil {
		panic(err)
	}

	// 3. 创建 Hertz 服务并注册日志中间件
	h := SetupHertzWithLogging(collector)

	// 4. 注册路由
	h.POST("/api/video/upload", VideoUploadHandler)

	// 5. 启动服务
	h.Spin()
}

// ================================
// 7. Kibana/Grafana 查询示例
// ================================

/*
# Kibana 查询示例

## 查询所有错误日志
```
level: ERROR
```

## 查询特定服务的错误
```
service_name: "video-service" AND success: false
```

## 查询高延迟请求 (>1000ms)
```
duration: >1000
```

## 查询特定 TraceID
```
trace_id: "abc123"
```

## 查询特定时间段的审计日志
```
action: "delete" AND timestamp: [2024-01-01 TO 2024-01-31]
```

# Grafana Dashboard 指标

## 错误率
```
sum(rate(service_log_success_false[5m])) / sum(rate(service_log_total[5m])) * 100
```

## 平均响应时间
```
avg(service_log_duration)
```

## P99 响应时间
```
histogram_quantile(0.99, sum(rate(service_log_duration_bucket[5m])) by (le))
```
*/

// ================================
// 类型定义 (仅用于示例)
// ================================

type VideoUploadRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type VideoUploadResponse struct {
	VideoID string `json:"video_id"`
	URL     string `json:"url"`
}

// 将结构体转为 JSON 字符串 (用于审计日志)
func toJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
