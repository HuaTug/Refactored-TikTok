package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"HuaTug.com/config"
	"HuaTug.com/pkg/kafka"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// AnalyticsConsumer 数据分析消费者示例
type AnalyticsConsumer struct {
	manager  *kafka.Manager
	consumer *kafka.Consumer
	wg       sync.WaitGroup
}

// NewAnalyticsConsumer 创建分析消费者
func NewAnalyticsConsumer(manager *kafka.Manager) (*AnalyticsConsumer, error) {
	consumer, err := manager.CreateConsumer(
		kafka.GroupAnalytics,
		[]string{
			kafka.TopicUserBehavior,
			kafka.TopicVideoView,
			kafka.TopicVideoExposure,
			kafka.TopicSearchLog,
			kafka.TopicVideoStats,
		},
	)
	if err != nil {
		return nil, err
	}

	return &AnalyticsConsumer{
		manager:  manager,
		consumer: consumer,
	}, nil
}

// Start 启动消费者
func (c *AnalyticsConsumer) Start() error {
	c.consumer.RegisterUserBehaviorHandler(&UserBehaviorHandler{})
	c.consumer.RegisterVideoViewHandler(&VideoViewHandler{})
	c.consumer.RegisterVideoExposureHandler(&VideoExposureHandler{})
	c.consumer.RegisterSearchLogHandler(&SearchLogHandler{})
	c.consumer.RegisterVideoStatsHandler(&VideoStatsHandler{})
	return c.consumer.Start()
}

// Stop 停止消费者
func (c *AnalyticsConsumer) Stop() error {
	return c.consumer.Stop()
}

// UserBehaviorHandler 用户行为处理器
type UserBehaviorHandler struct{}

// HandleUserBehavior 处理用户行为事件
func (h *UserBehaviorHandler) HandleUserBehavior(ctx context.Context, event *kafka.UserBehaviorEvent) error {
	hlog.Infof("[Analytics] User behavior: user=%d, video=%d, behavior=%s",
		event.UserID, event.VideoID, event.Behavior)
	return nil
}

// VideoViewHandler 视频播放处理器
type VideoViewHandler struct{}

// HandleVideoView 处理视频播放事件
func (h *VideoViewHandler) HandleVideoView(ctx context.Context, event *kafka.VideoViewEvent) error {
	hlog.Infof("[Analytics] Video view: video=%d, user=%d, watchTime=%dms",
		event.VideoID, event.UserID, event.WatchTime)
	return nil
}

// VideoExposureHandler 视频曝光处理器
type VideoExposureHandler struct{}

// HandleVideoExposure 处理视频曝光事件
func (h *VideoExposureHandler) HandleVideoExposure(ctx context.Context, event *kafka.VideoExposureEvent) error {
	hlog.Infof("[Analytics] Video exposure: user=%d, videos=%v",
		event.UserID, event.VideoIDs)
	return nil
}

// SearchLogHandler 搜索日志处理器
type SearchLogHandler struct{}

// HandleSearchLog 处理搜索日志事件
func (h *SearchLogHandler) HandleSearchLog(ctx context.Context, event *kafka.SearchLogEvent) error {
	hlog.Infof("[Analytics] Search log: user=%d, query=%s",
		event.UserID, event.Query)
	return nil
}

// VideoStatsHandler 视频统计处理器
type VideoStatsHandler struct{}

// HandleVideoStats 处理视频统计事件
func (h *VideoStatsHandler) HandleVideoStats(ctx context.Context, event *kafka.VideoStatsEvent) error {
	hlog.Infof("[Analytics] Video stats: video=%d, type=%s, delta=%d",
		event.VideoID, event.StatsType, event.Delta)
	return nil
}

func main() {
	config.Init()

	if len(config.ConfigInfo.Kafka.Brokers) == 0 {
		hlog.Error("Kafka brokers not configured")
		os.Exit(1)
	}

	kafkaConfig := &kafka.KafkaConfig{
		Brokers:         config.ConfigInfo.Kafka.Brokers,
		Version:         config.ConfigInfo.Kafka.Version,
		ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
	}

	manager, err := kafka.NewManager(kafkaConfig)
	if err != nil {
		hlog.Errorf("Failed to create Kafka manager: %v", err)
		os.Exit(1)
	}
	defer manager.Close()

	consumer, err := NewAnalyticsConsumer(manager)
	if err != nil {
		hlog.Errorf("Failed to create analytics consumer: %v", err)
		os.Exit(1)
	}

	if err := consumer.Start(); err != nil {
		hlog.Errorf("Failed to start consumer: %v", err)
		os.Exit(1)
	}

	hlog.Info("Analytics consumer started, waiting for messages...")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	if err := consumer.Stop(); err != nil {
		hlog.Errorf("Error stopping consumer: %v", err)
	}

	hlog.Info("Analytics consumer stopped")
}
