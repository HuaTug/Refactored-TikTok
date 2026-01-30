package logsystem

import (
	"context"
	"sync"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/pkg/elasticsearch"
	"HuaTug.com/pkg/kafka"
	"HuaTug.com/pkg/logger"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

var (
	kafkaManager *kafka.Manager
	esClient     *elasticsearch.Client
	logCollector *logger.LogCollector
	logConsumer  *logger.LogConsumer
	initialized  bool
	initMu       sync.Mutex
)

// LogSystemConfig 日志系统配置
type LogSystemConfig struct {
	ServiceName      string // 服务名称
	Environment      string // 环境 (dev/test/prod)
	Version          string // 服务版本
	EnableESConsumer bool   // 是否启用 ES 消费者 (独立服务可设为true)
}

// Init 初始化日志系统
func Init(cfg *LogSystemConfig) error {
	initMu.Lock()
	defer initMu.Unlock()

	if initialized {
		return nil
	}

	if cfg == nil {
		cfg = &LogSystemConfig{
			ServiceName: "tiktok-api",
			Environment: "dev",
			Version:     "v1.0.0",
		}
	}

	// 1. 初始化 Kafka Manager
	if len(config.ConfigInfo.Kafka.Brokers) == 0 {
		hlog.Warn("[LogSystem] Kafka brokers not configured, log system disabled")
		return nil
	}

	kafkaConfig := &kafka.KafkaConfig{
		Brokers:         config.ConfigInfo.Kafka.Brokers,
		Version:         config.ConfigInfo.Kafka.Version,
		ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
	}

	var err error
	kafkaManager, err = kafka.NewManager(kafkaConfig)
	if err != nil {
		hlog.Errorf("[LogSystem] Failed to create Kafka manager: %v", err)
		return err
	}

	// 初始化 Topics
	if err := kafkaManager.InitTopics(); err != nil {
		hlog.Warnf("[LogSystem] Failed to init topics: %v", err)
		// 不返回错误，topics 可能已存在
	}

	// 2. 初始化日志收集器
	logCollector = logger.GetCollector()
	if err := logCollector.Init(kafkaManager, cfg.ServiceName, cfg.Environment, cfg.Version); err != nil {
		hlog.Errorf("[LogSystem] Failed to init log collector: %v", err)
		return err
	}

	// 3. 初始化 ES 客户端 (如果配置了)
	if len(config.ConfigInfo.Elasticsearch.Addresses) > 0 {
		esClient = elasticsearch.GetClient()
		esConfig := &elasticsearch.ESConfig{
			Addresses:   config.ConfigInfo.Elasticsearch.Addresses,
			Username:    config.ConfigInfo.Elasticsearch.Username,
			Password:    config.ConfigInfo.Elasticsearch.Password,
			IndexPrefix: config.ConfigInfo.Elasticsearch.IndexPrefix,
			MaxRetries:  config.ConfigInfo.Elasticsearch.MaxRetries,
			EnableSniff: config.ConfigInfo.Elasticsearch.EnableSniff,
		}

		if err := esClient.Init(esConfig); err != nil {
			hlog.Warnf("[LogSystem] Failed to init ES client: %v", err)
			// ES 初始化失败不影响日志收集
		} else {
			// 初始化索引模板
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := esClient.InitLogIndexTemplates(ctx); err != nil {
				hlog.Warnf("[LogSystem] Failed to init ES templates: %v", err)
			}
			cancel()
		}
	}

	// 4. 如果启用 ES 消费者，启动日志消费
	if cfg.EnableESConsumer && esClient != nil && esClient.IsInitialized() {
		consumerConfig := &logger.LogConsumerConfig{
			BatchSize:     100,
			FlushInterval: 5 * time.Second,
		}

		logConsumer = logger.NewLogConsumer(kafkaManager, esClient, consumerConfig)
		if err := logConsumer.Start(); err != nil {
			hlog.Errorf("[LogSystem] Failed to start log consumer: %v", err)
			// 消费者启动失败不影响日志收集
		}
	}

	initialized = true
	hlog.Infof("[LogSystem] Log system initialized successfully for service: %s", cfg.ServiceName)
	return nil
}

// GetCollector 获取日志收集器
func GetCollector() *logger.LogCollector {
	return logCollector
}

// GetKafkaManager 获取 Kafka Manager
func GetKafkaManager() *kafka.Manager {
	return kafkaManager
}

// GetESClient 获取 ES 客户端
func GetESClient() *elasticsearch.Client {
	return esClient
}

// Close 关闭日志系统
func Close() {
	initMu.Lock()
	defer initMu.Unlock()

	if !initialized {
		return
	}

	// 关闭日志消费者
	if logConsumer != nil {
		logConsumer.Stop()
	}

	// 关闭 ES 客户端
	if esClient != nil {
		esClient.Close()
	}

	// 关闭 Kafka Manager
	if kafkaManager != nil {
		kafkaManager.Close()
	}

	initialized = false
	hlog.Info("[LogSystem] Log system closed")
}

// CreateLoggingMiddleware 创建日志中间件 (用于 Hertz)
func CreateLoggingMiddleware(config *logger.MiddlewareConfig) *logger.LoggingMiddleware {
	if logCollector == nil {
		hlog.Warn("[LogSystem] Log collector not initialized, middleware disabled")
		return nil
	}
	return logger.NewLoggingMiddleware(logCollector, config)
}
