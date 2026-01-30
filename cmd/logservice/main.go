package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/pkg/elasticsearch"
	"HuaTug.com/pkg/kafka"
	"HuaTug.com/pkg/logger"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func main() {
	// 初始化配置
	config.Init()

	hlog.Info("[LogService] Starting log service...")

	// 初始化 Kafka Manager
	if len(config.ConfigInfo.Kafka.Brokers) == 0 {
		hlog.Fatal("[LogService] Kafka brokers not configured")
	}

	kafkaConfig := &kafka.KafkaConfig{
		Brokers:         config.ConfigInfo.Kafka.Brokers,
		Version:         config.ConfigInfo.Kafka.Version,
		ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
	}

	kafkaManager, err := kafka.NewManager(kafkaConfig)
	if err != nil {
		hlog.Fatalf("[LogService] Failed to create Kafka manager: %v", err)
	}
	defer kafkaManager.Close()

	// 初始化 Topics
	if err := kafkaManager.InitTopics(); err != nil {
		hlog.Warnf("[LogService] Failed to init topics: %v", err)
	}

	// 初始化 ES 客户端
	esClient := elasticsearch.GetClient()
	if len(config.ConfigInfo.Elasticsearch.Addresses) > 0 {
		esConfig := &elasticsearch.ESConfig{
			Addresses:   config.ConfigInfo.Elasticsearch.Addresses,
			Username:    config.ConfigInfo.Elasticsearch.Username,
			Password:    config.ConfigInfo.Elasticsearch.Password,
			IndexPrefix: config.ConfigInfo.Elasticsearch.IndexPrefix,
			MaxRetries:  config.ConfigInfo.Elasticsearch.MaxRetries,
			EnableSniff: config.ConfigInfo.Elasticsearch.EnableSniff,
		}

		if err := esClient.Init(esConfig); err != nil {
			hlog.Fatalf("[LogService] Failed to init ES client: %v", err)
		}
		defer esClient.Close()

		// 初始化索引模板
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := esClient.InitLogIndexTemplates(ctx); err != nil {
			hlog.Warnf("[LogService] Failed to init ES templates: %v", err)
		}
		cancel()
	} else {
		hlog.Fatal("[LogService] Elasticsearch addresses not configured")
	}

	// 创建日志消费者
	consumerConfig := &logger.LogConsumerConfig{
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
	}

	logConsumer := logger.NewLogConsumer(kafkaManager, esClient, consumerConfig)
	if err := logConsumer.Start(); err != nil {
		hlog.Fatalf("[LogService] Failed to start log consumer: %v", err)
	}
	defer logConsumer.Stop()

	// 启动日志清理任务 (保留30天)
	go startLogRetentionCleanup(esClient)

	hlog.Info("[LogService] Log service started successfully")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	hlog.Info("[LogService] Shutting down log service...")
}

// startLogRetentionCleanup 启动日志保留清理任务
func startLogRetentionCleanup(esClient *elasticsearch.Client) {
	ticker := time.NewTicker(24 * time.Hour) // 每天执行一次
	defer ticker.Stop()

	// 首次启动时执行一次
	cleanupOldLogs(esClient)

	for range ticker.C {
		cleanupOldLogs(esClient)
	}
}

// cleanupOldLogs 清理旧日志
func cleanupOldLogs(esClient *elasticsearch.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	retentionDays := 30 // 保留30天

	indexTypes := []string{
		elasticsearch.IndexTypeServiceLog,
		elasticsearch.IndexTypeErrorLog,
		elasticsearch.IndexTypeAccessLog,
		elasticsearch.IndexTypeAuditLog,
		elasticsearch.IndexTypeAlertLog,
	}

	for _, indexType := range indexTypes {
		if err := esClient.DeleteOldIndices(ctx, indexType, retentionDays); err != nil {
			hlog.Errorf("[LogService] Failed to cleanup old %s indices: %v", indexType, err)
		}
	}
}
