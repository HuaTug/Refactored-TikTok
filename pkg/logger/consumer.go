package logger

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"HuaTug.com/pkg/elasticsearch"
	"HuaTug.com/pkg/kafka"

	"github.com/IBM/sarama"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// LogConsumer 日志消费者 - 从 Kafka 消费日志并写入 ES
type LogConsumer struct {
	kafkaManager *kafka.Manager
	esClient     *elasticsearch.Client

	// 消费者
	serviceLogConsumer *kafka.Consumer
	errorLogConsumer   *kafka.Consumer
	accessLogConsumer  *kafka.Consumer
	auditLogConsumer   *kafka.Consumer
	alertLogConsumer   *kafka.Consumer

	// 批量写入缓冲
	serviceLogBuffer []elasticsearch.BulkDocument
	errorLogBuffer   []elasticsearch.BulkDocument
	accessLogBuffer  []elasticsearch.BulkDocument
	auditLogBuffer   []elasticsearch.BulkDocument
	alertLogBuffer   []elasticsearch.BulkDocument

	// 配置
	batchSize     int
	flushInterval time.Duration

	// 锁
	serviceLogMu sync.Mutex
	errorLogMu   sync.Mutex
	accessLogMu  sync.Mutex
	auditLogMu   sync.Mutex
	alertLogMu   sync.Mutex

	// 控制
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex
}

// LogConsumerConfig 日志消费者配置
type LogConsumerConfig struct {
	BatchSize     int           // 批量写入大小
	FlushInterval time.Duration // 刷新间隔
}

// DefaultLogConsumerConfig 默认配置
func DefaultLogConsumerConfig() *LogConsumerConfig {
	return &LogConsumerConfig{
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
	}
}

// NewLogConsumer 创建日志消费者
func NewLogConsumer(kafkaManager *kafka.Manager, esClient *elasticsearch.Client, config *LogConsumerConfig) *LogConsumer {
	if config == nil {
		config = DefaultLogConsumerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LogConsumer{
		kafkaManager:     kafkaManager,
		esClient:         esClient,
		batchSize:        config.BatchSize,
		flushInterval:    config.FlushInterval,
		serviceLogBuffer: make([]elasticsearch.BulkDocument, 0, config.BatchSize),
		errorLogBuffer:   make([]elasticsearch.BulkDocument, 0, config.BatchSize),
		accessLogBuffer:  make([]elasticsearch.BulkDocument, 0, config.BatchSize),
		auditLogBuffer:   make([]elasticsearch.BulkDocument, 0, config.BatchSize),
		alertLogBuffer:   make([]elasticsearch.BulkDocument, 0, config.BatchSize),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start 启动所有日志消费者
func (c *LogConsumer) Start() error {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()

	if c.running {
		return nil
	}

	// 创建消费者
	var err error

	// 服务日志消费者
	c.serviceLogConsumer, err = c.kafkaManager.CreateConsumer(
		kafka.GroupLogProcessor+"_service",
		[]string{kafka.TopicServiceLog},
	)
	if err != nil {
		return err
	}

	// 错误日志消费者
	c.errorLogConsumer, err = c.kafkaManager.CreateConsumer(
		kafka.GroupLogProcessor+"_error",
		[]string{kafka.TopicErrorLog},
	)
	if err != nil {
		return err
	}

	// 访问日志消费者
	c.accessLogConsumer, err = c.kafkaManager.CreateConsumer(
		kafka.GroupLogProcessor+"_access",
		[]string{kafka.TopicAccessLog},
	)
	if err != nil {
		return err
	}

	// 审计日志消费者
	c.auditLogConsumer, err = c.kafkaManager.CreateConsumer(
		kafka.GroupLogProcessor+"_audit",
		[]string{kafka.TopicAuditLog},
	)
	if err != nil {
		return err
	}

	// 告警日志消费者
	c.alertLogConsumer, err = c.kafkaManager.CreateConsumer(
		kafka.GroupLogProcessor+"_alert",
		[]string{kafka.TopicAlertLog},
	)
	if err != nil {
		return err
	}

	// 注册处理器 - 使用函数类型包装
	c.serviceLogConsumer.RegisterHandlerFunc(kafka.TopicServiceLog, c.handleServiceLogMessage)
	c.errorLogConsumer.RegisterHandlerFunc(kafka.TopicErrorLog, c.handleErrorLogMessage)
	c.accessLogConsumer.RegisterHandlerFunc(kafka.TopicAccessLog, c.handleAccessLogMessage)
	c.auditLogConsumer.RegisterHandlerFunc(kafka.TopicAuditLog, c.handleAuditLogMessage)
	c.alertLogConsumer.RegisterHandlerFunc(kafka.TopicAlertLog, c.handleAlertLogMessage)

	// 启动消费者
	c.wg.Add(5)
	go c.runConsumer(c.serviceLogConsumer, "service_log")
	go c.runConsumer(c.errorLogConsumer, "error_log")
	go c.runConsumer(c.accessLogConsumer, "access_log")
	go c.runConsumer(c.auditLogConsumer, "audit_log")
	go c.runConsumer(c.alertLogConsumer, "alert_log")

	// 启动定时刷新
	c.wg.Add(1)
	go c.flushLoop()

	c.running = true
	hlog.Info("[LogConsumer] Started all log consumers")
	return nil
}

// runConsumer 运行单个消费者
func (c *LogConsumer) runConsumer(consumer *kafka.Consumer, name string) {
	defer c.wg.Done()

	hlog.Infof("[LogConsumer] Starting consumer: %s", name)
	if err := consumer.Start(); err != nil {
		hlog.Errorf("[LogConsumer] Consumer %s error: %v", name, err)
	}
}

// flushLoop 定时刷新缓冲区
func (c *LogConsumer) flushLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			// 最后刷新一次
			c.flushAll()
			return
		case <-ticker.C:
			c.flushAll()
		}
	}
}

// flushAll 刷新所有缓冲区
func (c *LogConsumer) flushAll() {
	c.flushServiceLog()
	c.flushErrorLog()
	c.flushAccessLog()
	c.flushAuditLog()
	c.flushAlertLog()
}

// ============ 日志处理函数 (sarama.ConsumerMessage) ============

func (c *LogConsumer) handleServiceLogMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return c.handleServiceLog(msg.Value)
}

func (c *LogConsumer) handleErrorLogMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return c.handleErrorLog(msg.Value)
}

func (c *LogConsumer) handleAccessLogMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return c.handleAccessLog(msg.Value)
}

func (c *LogConsumer) handleAuditLogMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return c.handleAuditLog(msg.Value)
}

func (c *LogConsumer) handleAlertLogMessage(ctx context.Context, msg *sarama.ConsumerMessage) error {
	return c.handleAlertLog(msg.Value)
}

// ============ 日志处理函数 (原始字节) ============

func (c *LogConsumer) handleServiceLog(msg []byte) error {
	var event kafka.ServiceLogEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		hlog.Errorf("[LogConsumer] Failed to unmarshal service log: %v", err)
		return err
	}

	c.serviceLogMu.Lock()
	c.serviceLogBuffer = append(c.serviceLogBuffer, elasticsearch.BulkDocument{
		ID:       event.EventID,
		Document: event,
	})
	shouldFlush := len(c.serviceLogBuffer) >= c.batchSize
	c.serviceLogMu.Unlock()

	if shouldFlush {
		c.flushServiceLog()
	}

	return nil
}

func (c *LogConsumer) handleErrorLog(msg []byte) error {
	var event kafka.ErrorLogEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		hlog.Errorf("[LogConsumer] Failed to unmarshal error log: %v", err)
		return err
	}

	c.errorLogMu.Lock()
	c.errorLogBuffer = append(c.errorLogBuffer, elasticsearch.BulkDocument{
		ID:       event.EventID,
		Document: event,
	})
	shouldFlush := len(c.errorLogBuffer) >= c.batchSize
	c.errorLogMu.Unlock()

	if shouldFlush {
		c.flushErrorLog()
	}

	return nil
}

func (c *LogConsumer) handleAccessLog(msg []byte) error {
	var event kafka.AccessLogEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		hlog.Errorf("[LogConsumer] Failed to unmarshal access log: %v", err)
		return err
	}

	c.accessLogMu.Lock()
	c.accessLogBuffer = append(c.accessLogBuffer, elasticsearch.BulkDocument{
		ID:       event.EventID,
		Document: event,
	})
	shouldFlush := len(c.accessLogBuffer) >= c.batchSize
	c.accessLogMu.Unlock()

	if shouldFlush {
		c.flushAccessLog()
	}

	return nil
}

func (c *LogConsumer) handleAuditLog(msg []byte) error {
	var event kafka.AuditLogEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		hlog.Errorf("[LogConsumer] Failed to unmarshal audit log: %v", err)
		return err
	}

	c.auditLogMu.Lock()
	c.auditLogBuffer = append(c.auditLogBuffer, elasticsearch.BulkDocument{
		ID:       event.EventID,
		Document: event,
	})
	shouldFlush := len(c.auditLogBuffer) >= c.batchSize
	c.auditLogMu.Unlock()

	if shouldFlush {
		c.flushAuditLog()
	}

	return nil
}

func (c *LogConsumer) handleAlertLog(msg []byte) error {
	var event kafka.AlertLogEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		hlog.Errorf("[LogConsumer] Failed to unmarshal alert log: %v", err)
		return err
	}

	c.alertLogMu.Lock()
	c.alertLogBuffer = append(c.alertLogBuffer, elasticsearch.BulkDocument{
		ID:       event.EventID,
		Document: event,
	})
	shouldFlush := len(c.alertLogBuffer) >= c.batchSize
	c.alertLogMu.Unlock()

	if shouldFlush {
		c.flushAlertLog()
	}

	return nil
}

// ============ 刷新函数 ============

func (c *LogConsumer) flushServiceLog() {
	c.serviceLogMu.Lock()
	if len(c.serviceLogBuffer) == 0 {
		c.serviceLogMu.Unlock()
		return
	}
	docs := c.serviceLogBuffer
	c.serviceLogBuffer = make([]elasticsearch.BulkDocument, 0, c.batchSize)
	c.serviceLogMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.esClient.BulkIndex(ctx, elasticsearch.IndexTypeServiceLog, docs); err != nil {
		hlog.Errorf("[LogConsumer] Failed to bulk index service logs: %v", err)
	} else {
		hlog.Debugf("[LogConsumer] Flushed %d service logs to ES", len(docs))
	}
}

func (c *LogConsumer) flushErrorLog() {
	c.errorLogMu.Lock()
	if len(c.errorLogBuffer) == 0 {
		c.errorLogMu.Unlock()
		return
	}
	docs := c.errorLogBuffer
	c.errorLogBuffer = make([]elasticsearch.BulkDocument, 0, c.batchSize)
	c.errorLogMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.esClient.BulkIndex(ctx, elasticsearch.IndexTypeErrorLog, docs); err != nil {
		hlog.Errorf("[LogConsumer] Failed to bulk index error logs: %v", err)
	} else {
		hlog.Debugf("[LogConsumer] Flushed %d error logs to ES", len(docs))
	}
}

func (c *LogConsumer) flushAccessLog() {
	c.accessLogMu.Lock()
	if len(c.accessLogBuffer) == 0 {
		c.accessLogMu.Unlock()
		return
	}
	docs := c.accessLogBuffer
	c.accessLogBuffer = make([]elasticsearch.BulkDocument, 0, c.batchSize)
	c.accessLogMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.esClient.BulkIndex(ctx, elasticsearch.IndexTypeAccessLog, docs); err != nil {
		hlog.Errorf("[LogConsumer] Failed to bulk index access logs: %v", err)
	} else {
		hlog.Debugf("[LogConsumer] Flushed %d access logs to ES", len(docs))
	}
}

func (c *LogConsumer) flushAuditLog() {
	c.auditLogMu.Lock()
	if len(c.auditLogBuffer) == 0 {
		c.auditLogMu.Unlock()
		return
	}
	docs := c.auditLogBuffer
	c.auditLogBuffer = make([]elasticsearch.BulkDocument, 0, c.batchSize)
	c.auditLogMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.esClient.BulkIndex(ctx, elasticsearch.IndexTypeAuditLog, docs); err != nil {
		hlog.Errorf("[LogConsumer] Failed to bulk index audit logs: %v", err)
	} else {
		hlog.Debugf("[LogConsumer] Flushed %d audit logs to ES", len(docs))
	}
}

func (c *LogConsumer) flushAlertLog() {
	c.alertLogMu.Lock()
	if len(c.alertLogBuffer) == 0 {
		c.alertLogMu.Unlock()
		return
	}
	docs := c.alertLogBuffer
	c.alertLogBuffer = make([]elasticsearch.BulkDocument, 0, c.batchSize)
	c.alertLogMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.esClient.BulkIndex(ctx, elasticsearch.IndexTypeAlertLog, docs); err != nil {
		hlog.Errorf("[LogConsumer] Failed to bulk index alert logs: %v", err)
	} else {
		hlog.Debugf("[LogConsumer] Flushed %d alert logs to ES", len(docs))
	}
}

// Stop 停止日志消费者
func (c *LogConsumer) Stop() error {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()

	if !c.running {
		return nil
	}

	c.cancel()

	// 停止所有消费者
	if c.serviceLogConsumer != nil {
		c.serviceLogConsumer.Stop()
	}
	if c.errorLogConsumer != nil {
		c.errorLogConsumer.Stop()
	}
	if c.accessLogConsumer != nil {
		c.accessLogConsumer.Stop()
	}
	if c.auditLogConsumer != nil {
		c.auditLogConsumer.Stop()
	}
	if c.alertLogConsumer != nil {
		c.alertLogConsumer.Stop()
	}

	c.wg.Wait()
	c.running = false

	hlog.Info("[LogConsumer] Stopped all log consumers")
	return nil
}
