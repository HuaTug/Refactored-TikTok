package logger

import (
	"bytes"
	"context"
	"io"
	"time"

	"HuaTug.com/pkg/kafka"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// LoggingMiddleware 日志记录中间件
type LoggingMiddleware struct {
	collector          *LogCollector
	enableRequestBody  bool            // 是否记录请求体
	enableResponseBody bool            // 是否记录响应体
	maxBodySize        int             // 最大记录的 body 大小
	sensitiveEndpoints map[string]bool // 敏感接口列表 (不记录 body)
	skipEndpoints      map[string]bool // 跳过日志的接口
}

// MiddlewareConfig 中间件配置
type MiddlewareConfig struct {
	EnableRequestBody  bool
	EnableResponseBody bool
	MaxBodySize        int
	SensitiveEndpoints []string
	SkipEndpoints      []string
}

// DefaultMiddlewareConfig 默认中间件配置
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		EnableRequestBody:  false,
		EnableResponseBody: false,
		MaxBodySize:        4096,
		SensitiveEndpoints: []string{"/api/user/login", "/api/user/register"},
		SkipEndpoints:      []string{"/health", "/metrics", "/favicon.ico"},
	}
}

// NewLoggingMiddleware 创建日志记录中间件
func NewLoggingMiddleware(collector *LogCollector, config *MiddlewareConfig) *LoggingMiddleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}

	sensitiveEndpoints := make(map[string]bool)
	for _, ep := range config.SensitiveEndpoints {
		sensitiveEndpoints[ep] = true
	}

	skipEndpoints := make(map[string]bool)
	for _, ep := range config.SkipEndpoints {
		skipEndpoints[ep] = true
	}

	return &LoggingMiddleware{
		collector:          collector,
		enableRequestBody:  config.EnableRequestBody,
		enableResponseBody: config.EnableResponseBody,
		maxBodySize:        config.MaxBodySize,
		sensitiveEndpoints: sensitiveEndpoints,
		skipEndpoints:      skipEndpoints,
	}
}

// Handler 返回 Hertz 中间件处理函数
func (m *LoggingMiddleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		endpoint := string(c.Request.URI().Path())

		// 跳过不需要记录日志的接口
		if m.skipEndpoints[endpoint] {
			c.Next(ctx)
			return
		}

		startTime := time.Now()

		// 生成 TraceID
		traceID := c.Request.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = uuid.New().String()
		}
		c.Set("trace_id", traceID)

		// 获取请求体 (如果需要)
		var requestBody string
		if m.enableRequestBody && !m.sensitiveEndpoints[endpoint] {
			body, _ := c.Body()
			if len(body) > m.maxBodySize {
				requestBody = string(body[:m.maxBodySize]) + "...(truncated)"
			} else {
				requestBody = string(body)
			}
			// 重新设置 body，因为读取后会被清空
			c.Request.SetBody(body)
		}

		// 创建响应体捕获器
		var responseBody string
		originalWriter := c.Response.BodyWriter()

		// 执行后续处理
		c.Next(ctx)

		// 计算耗时
		duration := time.Since(startTime).Milliseconds()

		// 获取响应体 (如果需要)
		if m.enableResponseBody && !m.sensitiveEndpoints[endpoint] {
			respBody := c.Response.Body()
			if len(respBody) > m.maxBodySize {
				responseBody = string(respBody[:m.maxBodySize]) + "...(truncated)"
			} else {
				responseBody = string(respBody)
			}
		}
		_ = originalWriter // 避免未使用警告

		// 获取状态码
		statusCode := c.Response.StatusCode()
		success := statusCode >= 200 && statusCode < 400

		// 获取用户ID (从上下文中)
		var userID int64
		if uid, exists := c.Get("user_id"); exists {
			if id, ok := uid.(int64); ok {
				userID = id
			}
		}

		// 获取错误信息
		var errorCode, errorMessage string
		if errCode, exists := c.Get("error_code"); exists {
			if code, ok := errCode.(string); ok {
				errorCode = code
			}
		}
		if errMsg, exists := c.Get("error_message"); exists {
			if msg, ok := errMsg.(string); ok {
				errorMessage = msg
			}
		}

		// 记录服务调用日志
		m.collector.LogServiceCall(ctx, &ServiceCallLog{
			TraceID:      traceID,
			MethodName:   string(c.Request.Method()),
			Endpoint:     endpoint,
			HTTPMethod:   string(c.Request.Method()),
			StatusCode:   statusCode,
			Success:      success,
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
			UserID:       userID,
			ClientIP:     c.ClientIP(),
			UserAgent:    string(c.Request.Header.UserAgent()),
			RequestSize:  int64(len(c.Request.Body())),
			ResponseSize: int64(len(c.Response.Body())),
			Duration:     duration,
			RequestBody:  requestBody,
			ResponseBody: responseBody,
			Headers:      extractHeaders(c),
		})

		// 如果是错误响应，额外记录错误日志
		if !success {
			m.collector.LogError(ctx, &ErrorLog{
				TraceID:      traceID,
				MethodName:   endpoint,
				ErrorCode:    errorCode,
				ErrorType:    "http_error",
				ErrorMessage: errorMessage,
				Level:        getLogLevel(statusCode),
				UserID:       userID,
				ClientIP:     c.ClientIP(),
				Context: map[string]string{
					"http_method": string(c.Request.Method()),
					"status_code": string(rune(statusCode)),
				},
			})
		}
	}
}

// RecoveryMiddleware panic 恢复中间件，记录 panic 日志
func (m *LoggingMiddleware) RecoveryMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if r := recover(); r != nil {
				endpoint := string(c.Request.URI().Path())

				// 获取 TraceID
				traceID, _ := c.Get("trace_id")
				traceIDStr, _ := traceID.(string)

				// 获取用户ID
				var userID int64
				if uid, exists := c.Get("user_id"); exists {
					if id, ok := uid.(int64); ok {
						userID = id
					}
				}

				// 记录 panic 日志
				m.collector.LogPanic(ctx, endpoint, r, userID, c.ClientIP())

				// 设置错误响应
				c.Set("error_code", "INTERNAL_ERROR")
				c.Set("error_message", "Internal Server Error")

				hlog.Errorf("[Recovery] Panic recovered: %v, trace_id=%s", r, traceIDStr)

				// 返回 500 错误
				c.AbortWithStatus(500)
			}
		}()

		c.Next(ctx)
	}
}

// extractHeaders 提取请求头 (排除敏感信息)
func extractHeaders(c *app.RequestContext) map[string]string {
	headers := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"Authorization": true,
		"Cookie":        true,
		"X-Api-Key":     true,
	}

	c.Request.Header.VisitAll(func(key, value []byte) {
		k := string(key)
		if !sensitiveHeaders[k] {
			headers[k] = string(value)
		} else {
			headers[k] = "[REDACTED]"
		}
	})

	return headers
}

// getLogLevel 根据状态码获取日志级别
func getLogLevel(statusCode int) kafka.LogLevel {
	switch {
	case statusCode >= 500:
		return kafka.LogLevelError
	case statusCode >= 400:
		return kafka.LogLevelWarn
	default:
		return kafka.LogLevelInfo
	}
}

// ResponseCapture 响应捕获器
type ResponseCapture struct {
	io.Writer
	body *bytes.Buffer
}

func (rc *ResponseCapture) Write(p []byte) (int, error) {
	rc.body.Write(p)
	return rc.Writer.Write(p)
}
