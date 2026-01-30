package logsystem

import (
	"context"
	"time"

	"HuaTug.com/pkg/kafka"
	"HuaTug.com/pkg/logger"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
)

// ============ 便捷日志记录函数 ============

// LogServiceError 记录服务调用失败日志 (供 Handler 使用)
func LogServiceError(ctx context.Context, c *app.RequestContext, methodName, errorCode, errorMessage string, duration int64) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	// 获取 TraceID
	traceID := getTraceID(c)

	// 获取用户ID
	userID := getUserID(c)

	collector.LogServiceCall(ctx, &logger.ServiceCallLog{
		TraceID:      traceID,
		MethodName:   methodName,
		Endpoint:     string(c.Request.URI().Path()),
		HTTPMethod:   string(c.Request.Method()),
		StatusCode:   c.Response.StatusCode(),
		Success:      false,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		UserID:       userID,
		ClientIP:     c.ClientIP(),
		UserAgent:    string(c.Request.Header.UserAgent()),
		Duration:     duration,
	})
}

// LogError 记录错误日志 (供业务逻辑使用)
func LogError(ctx context.Context, c *app.RequestContext, methodName, errorCode, errorType, errorMessage string) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	// 获取 TraceID
	traceID := getTraceID(c)

	// 获取用户ID
	userID := getUserID(c)

	collector.LogError(ctx, &logger.ErrorLog{
		TraceID:      traceID,
		MethodName:   methodName,
		ErrorCode:    errorCode,
		ErrorType:    errorType, // panic/business/system/network
		ErrorMessage: errorMessage,
		Level:        kafka.LogLevelError,
		UserID:       userID,
		ClientIP:     c.ClientIP(),
	})
}

// LogErrorWithContext 记录带上下文的错误日志
func LogErrorWithContext(ctx context.Context, c *app.RequestContext, methodName, errorCode, errorType, errorMessage string, extraContext map[string]string) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	traceID := getTraceID(c)
	userID := getUserID(c)

	collector.LogError(ctx, &logger.ErrorLog{
		TraceID:      traceID,
		MethodName:   methodName,
		ErrorCode:    errorCode,
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
		Level:        kafka.LogLevelError,
		UserID:       userID,
		ClientIP:     c.ClientIP(),
		Context:      extraContext,
	})
}

// LogAudit 记录审计日志
func LogAudit(ctx context.Context, c *app.RequestContext, targetID int64, targetType, action, resource, oldValue, newValue string, success bool) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	traceID := getTraceID(c)
	userID := getUserID(c)

	collector.LogAudit(ctx, &logger.AuditLog{
		TraceID:    traceID,
		UserID:     userID,
		TargetID:   targetID,
		TargetType: targetType,
		Action:     action,
		Resource:   resource,
		OldValue:   oldValue,
		NewValue:   newValue,
		ClientIP:   c.ClientIP(),
		UserAgent:  string(c.Request.Header.UserAgent()),
		Success:    success,
	})
}

// LogAlert 记录告警日志
func LogAlert(ctx context.Context, alertName, alertType, severity, metricName string, metricValue, threshold float64, message string) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	collector.LogAlert(ctx, &logger.AlertLog{
		AlertID:     uuid.New().String(),
		AlertName:   alertName,
		AlertType:   alertType,
		Severity:    severity,
		MetricName:  metricName,
		MetricValue: metricValue,
		Threshold:   threshold,
		Message:     message,
		Status:      "firing",
	})
}

// ============ RPC 服务日志记录函数 ============

// LogRPCError 记录 RPC 服务调用失败日志 (供 Video Service 等 RPC 服务使用)
func LogRPCError(ctx context.Context, serviceName, methodName, errorCode, errorMessage string, userID int64, duration int64) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	collector.LogServiceCall(ctx, &logger.ServiceCallLog{
		TraceID:      uuid.New().String(),
		MethodName:   methodName,
		Endpoint:     serviceName + "/" + methodName,
		HTTPMethod:   "RPC",
		StatusCode:   500,
		Success:      false,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		UserID:       userID,
		Duration:     duration,
	})
}

// LogRPCErrorWithContext 记录带上下文的 RPC 错误日志
func LogRPCErrorWithContext(ctx context.Context, serviceName, methodName, errorCode, errorType, errorMessage string, userID int64, extraContext map[string]string) {
	collector := GetCollector()
	if collector == nil || !collector.IsEnabled() {
		return
	}

	collector.LogError(ctx, &logger.ErrorLog{
		TraceID:      uuid.New().String(),
		MethodName:   serviceName + "/" + methodName,
		ErrorCode:    errorCode,
		ErrorType:    errorType,
		ErrorMessage: errorMessage,
		Level:        kafka.LogLevelError,
		UserID:       userID,
		Context:      extraContext,
	})
}

// ============ 简化的业务错误日志函数 ============

// LogBusinessError 记录业务错误
func LogBusinessError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string) {
	LogError(ctx, c, string(c.Request.URI().Path()), errorCode, "business", errorMessage)
}

// LogSystemError 记录系统错误
func LogSystemError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string) {
	LogError(ctx, c, string(c.Request.URI().Path()), errorCode, "system", errorMessage)
}

// LogNetworkError 记录网络错误
func LogNetworkError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string) {
	LogError(ctx, c, string(c.Request.URI().Path()), errorCode, "network", errorMessage)
}

// ============ 计时器辅助函数 ============

// Timer 用于计算操作耗时
type Timer struct {
	startTime time.Time
}

// NewTimer 创建新的计时器
func NewTimer() *Timer {
	return &Timer{startTime: time.Now()}
}

// ElapsedMs 返回已过去的毫秒数
func (t *Timer) ElapsedMs() int64 {
	return time.Since(t.startTime).Milliseconds()
}

// ============ 辅助函数 ============

// getTraceID 从请求上下文获取 TraceID
func getTraceID(c *app.RequestContext) string {
	if traceID, exists := c.Get("trace_id"); exists {
		if id, ok := traceID.(string); ok {
			return id
		}
	}
	// 如果没有 TraceID，从 Header 获取或生成新的
	traceID := c.Request.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
		c.Set("trace_id", traceID)
	}
	return traceID
}

// getUserID 从请求上下文获取用户ID
func getUserID(c *app.RequestContext) int64 {
	if userID, exists := c.Get("user_id"); exists {
		switch id := userID.(type) {
		case int64:
			return id
		case int:
			return int64(id)
		case float64:
			return int64(id)
		}
	}
	return 0
}

// SetUserID 设置用户ID到上下文 (供认证中间件使用)
func SetUserID(c *app.RequestContext, userID int64) {
	c.Set("user_id", userID)
}

// SetErrorInfo 设置错误信息到上下文 (供日志中间件使用)
func SetErrorInfo(c *app.RequestContext, errorCode, errorMessage string) {
	c.Set("error_code", errorCode)
	c.Set("error_message", errorMessage)
}
