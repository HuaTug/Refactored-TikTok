package logger

import (
	"context"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"HuaTug.com/pkg/kafka"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// LogCollector 日志收集器 - 负责将日志发送到 Kafka
type LogCollector struct {
	manager     *kafka.Manager
	serviceName string
	environment string
	version     string
	serverIP    string
	serverHost  string
	mu          sync.RWMutex
	enabled     bool
}

var (
	defaultCollector *LogCollector
	once             sync.Once
)

// GetCollector 获取默认日志收集器
func GetCollector() *LogCollector {
	once.Do(func() {
		defaultCollector = &LogCollector{}
	})
	return defaultCollector
}

// Init 初始化日志收集器
func (c *LogCollector) Init(manager *kafka.Manager, serviceName, environment, version string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.enabled {
		return nil
	}

	c.manager = manager
	c.serviceName = serviceName
	c.environment = environment
	c.version = version
	c.serverIP = getLocalIP()
	c.serverHost, _ = os.Hostname()
	c.enabled = true

	hlog.Infof("[LogCollector] Initialized for service: %s, env: %s", serviceName, environment)
	return nil
}

// IsEnabled 检查是否启用
func (c *LogCollector) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

// ============ 服务调用日志 ============

// LogServiceCall 记录服务调用日志
func (c *LogCollector) LogServiceCall(ctx context.Context, log *ServiceCallLog) {
	if !c.IsEnabled() {
		return
	}

	event := &kafka.ServiceLogEvent{
		EventID:      uuid.New().String(),
		TraceID:      log.TraceID,
		SpanID:       log.SpanID,
		ParentSpanID: log.ParentSpanID,
		ServiceName:  c.serviceName,
		MethodName:   log.MethodName,
		Endpoint:     log.Endpoint,
		HTTPMethod:   log.HTTPMethod,
		StatusCode:   log.StatusCode,
		Success:      log.Success,
		ErrorCode:    log.ErrorCode,
		ErrorMessage: log.ErrorMessage,
		UserID:       log.UserID,
		ClientIP:     log.ClientIP,
		UserAgent:    log.UserAgent,
		RequestSize:  log.RequestSize,
		ResponseSize: log.ResponseSize,
		Duration:     log.Duration,
		Timestamp:    time.Now(),
		RequestBody:  log.RequestBody,
		ResponseBody: log.ResponseBody,
		Headers:      log.Headers,
		Extra:        log.Extra,
		ServerIP:     c.serverIP,
		ServerHost:   c.serverHost,
		Environment:  c.environment,
		Version:      c.version,
	}

	go func() {
		if err := c.manager.PublishServiceLog(ctx, event); err != nil {
			hlog.Errorf("[LogCollector] Failed to publish service log: %v", err)
		}
	}()
}

// LogServiceError 记录服务调用失败日志 (便捷方法)
func (c *LogCollector) LogServiceError(ctx context.Context, methodName, endpoint, httpMethod string,
	statusCode int, errorCode, errorMessage string, userID int64, clientIP string, duration int64) {

	c.LogServiceCall(ctx, &ServiceCallLog{
		MethodName:   methodName,
		Endpoint:     endpoint,
		HTTPMethod:   httpMethod,
		StatusCode:   statusCode,
		Success:      false,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		UserID:       userID,
		ClientIP:     clientIP,
		Duration:     duration,
	})
}

// LogServiceSuccess 记录服务调用成功日志 (便捷方法)
func (c *LogCollector) LogServiceSuccess(ctx context.Context, methodName, endpoint, httpMethod string,
	statusCode int, userID int64, clientIP string, duration int64) {

	c.LogServiceCall(ctx, &ServiceCallLog{
		MethodName: methodName,
		Endpoint:   endpoint,
		HTTPMethod: httpMethod,
		StatusCode: statusCode,
		Success:    true,
		UserID:     userID,
		ClientIP:   clientIP,
		Duration:   duration,
	})
}

// ============ 错误日志 ============

// LogError 记录错误日志
func (c *LogCollector) LogError(ctx context.Context, log *ErrorLog) {
	if !c.IsEnabled() {
		return
	}

	// 获取堆栈信息
	stackTrace := log.StackTrace
	if stackTrace == "" && log.Level == kafka.LogLevelFatal {
		stackTrace = getStackTrace(3)
	}

	event := &kafka.ErrorLogEvent{
		EventID:      uuid.New().String(),
		TraceID:      log.TraceID,
		ServiceName:  c.serviceName,
		MethodName:   log.MethodName,
		ErrorCode:    log.ErrorCode,
		ErrorType:    log.ErrorType,
		ErrorMessage: log.ErrorMessage,
		StackTrace:   stackTrace,
		Level:        log.Level,
		UserID:       log.UserID,
		ClientIP:     log.ClientIP,
		Timestamp:    time.Now(),
		Context:      log.Context,
		Cause:        log.Cause,
		ServerIP:     c.serverIP,
		ServerHost:   c.serverHost,
		Environment:  c.environment,
		Version:      c.version,
	}

	go func() {
		if err := c.manager.PublishErrorLog(ctx, event); err != nil {
			hlog.Errorf("[LogCollector] Failed to publish error log: %v", err)
		}
	}()
}

// LogErrorSimple 简化的错误日志记录
func (c *LogCollector) LogErrorSimple(ctx context.Context, level kafka.LogLevel, errorType, errorCode, errorMessage string) {
	c.LogError(ctx, &ErrorLog{
		Level:        level,
		ErrorType:    errorType,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	})
}

// LogPanic 记录 panic 日志
func (c *LogCollector) LogPanic(ctx context.Context, methodName string, recovered interface{}, userID int64, clientIP string) {
	c.LogError(ctx, &ErrorLog{
		MethodName:   methodName,
		Level:        kafka.LogLevelFatal,
		ErrorType:    "panic",
		ErrorMessage: toString(recovered),
		StackTrace:   getStackTrace(4),
		UserID:       userID,
		ClientIP:     clientIP,
	})
}

// ============ 访问日志 ============

// LogAccess 记录访问日志
func (c *LogCollector) LogAccess(ctx context.Context, log *AccessLog) {
	if !c.IsEnabled() {
		return
	}

	event := &kafka.AccessLogEvent{
		EventID:      uuid.New().String(),
		TraceID:      log.TraceID,
		UserID:       log.UserID,
		ClientIP:     log.ClientIP,
		Endpoint:     log.Endpoint,
		HTTPMethod:   log.HTTPMethod,
		StatusCode:   log.StatusCode,
		Duration:     log.Duration,
		RequestSize:  log.RequestSize,
		ResponseSize: log.ResponseSize,
		UserAgent:    log.UserAgent,
		Referer:      log.Referer,
		Timestamp:    time.Now(),
		Country:      log.Country,
		Region:       log.Region,
		DeviceType:   log.DeviceType,
		Platform:     log.Platform,
	}

	go func() {
		if err := c.manager.PublishAccessLog(ctx, event); err != nil {
			hlog.Errorf("[LogCollector] Failed to publish access log: %v", err)
		}
	}()
}

// ============ 审计日志 ============

// LogAudit 记录审计日志
func (c *LogCollector) LogAudit(ctx context.Context, log *AuditLog) {
	if !c.IsEnabled() {
		return
	}

	event := &kafka.AuditLogEvent{
		EventID:      uuid.New().String(),
		TraceID:      log.TraceID,
		UserID:       log.UserID,
		TargetID:     log.TargetID,
		TargetType:   log.TargetType,
		Action:       log.Action,
		Resource:     log.Resource,
		OldValue:     log.OldValue,
		NewValue:     log.NewValue,
		ClientIP:     log.ClientIP,
		UserAgent:    log.UserAgent,
		Timestamp:    time.Now(),
		Success:      log.Success,
		ErrorMessage: log.ErrorMessage,
		Extra:        log.Extra,
	}

	go func() {
		if err := c.manager.PublishAuditLog(ctx, event); err != nil {
			hlog.Errorf("[LogCollector] Failed to publish audit log: %v", err)
		}
	}()
}

// ============ 告警日志 ============

// LogAlert 记录告警日志
func (c *LogCollector) LogAlert(ctx context.Context, log *AlertLog) {
	if !c.IsEnabled() {
		return
	}

	event := &kafka.AlertLogEvent{
		EventID:     uuid.New().String(),
		AlertID:     log.AlertID,
		AlertName:   log.AlertName,
		AlertType:   log.AlertType,
		Severity:    log.Severity,
		ServiceName: c.serviceName,
		MetricName:  log.MetricName,
		MetricValue: log.MetricValue,
		Threshold:   log.Threshold,
		Message:     log.Message,
		Timestamp:   time.Now(),
		Status:      log.Status,
		Labels:      log.Labels,
		Annotations: log.Annotations,
		Environment: c.environment,
	}

	go func() {
		if err := c.manager.PublishAlertLog(ctx, event); err != nil {
			hlog.Errorf("[LogCollector] Failed to publish alert log: %v", err)
		}
	}()
}

// ============ 日志结构体定义 ============

// ServiceCallLog 服务调用日志结构
type ServiceCallLog struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	MethodName   string
	Endpoint     string
	HTTPMethod   string
	StatusCode   int
	Success      bool
	ErrorCode    string
	ErrorMessage string
	UserID       int64
	ClientIP     string
	UserAgent    string
	RequestSize  int64
	ResponseSize int64
	Duration     int64
	RequestBody  string
	ResponseBody string
	Headers      map[string]string
	Extra        map[string]string
}

// ErrorLog 错误日志结构
type ErrorLog struct {
	TraceID      string
	MethodName   string
	ErrorCode    string
	ErrorType    string // panic/business/system/network
	ErrorMessage string
	StackTrace   string
	Level        kafka.LogLevel
	UserID       int64
	ClientIP     string
	Context      map[string]string
	Cause        string
}

// AccessLog 访问日志结构
type AccessLog struct {
	TraceID      string
	UserID       int64
	ClientIP     string
	Endpoint     string
	HTTPMethod   string
	StatusCode   int
	Duration     int64
	RequestSize  int64
	ResponseSize int64
	UserAgent    string
	Referer      string
	Country      string
	Region       string
	DeviceType   string
	Platform     string
}

// AuditLog 审计日志结构
type AuditLog struct {
	TraceID      string
	UserID       int64
	TargetID     int64
	TargetType   string
	Action       string
	Resource     string
	OldValue     string
	NewValue     string
	ClientIP     string
	UserAgent    string
	Success      bool
	ErrorMessage string
	Extra        map[string]string
}

// AlertLog 告警日志结构
type AlertLog struct {
	AlertID     string
	AlertName   string
	AlertType   string // error_rate/latency/resource
	Severity    string // critical/warning/info
	MetricName  string
	MetricValue float64
	Threshold   float64
	Message     string
	Status      string // firing/resolved
	Labels      map[string]string
	Annotations map[string]string
}

// ============ 辅助函数 ============

// getLocalIP 获取本机 IP
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "unknown"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "unknown"
}

// getStackTrace 获取堆栈信息
func getStackTrace(skip int) string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// toString 将 interface{} 转换为字符串
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		return ""
	}
}
