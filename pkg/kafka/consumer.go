package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// ConsumerConfig Kafka Consumer 配置
type ConsumerConfig struct {
	Brokers           []string               // Kafka broker 地址列表
	GroupID           string                 // 消费者组 ID
	Topics            []string               // 订阅的 topic 列表
	OffsetInitial     int64                  // 初始 offset (sarama.OffsetNewest / sarama.OffsetOldest)
	SessionTimeout    time.Duration          // 会话超时
	HeartbeatInterval time.Duration          // 心跳间隔
	RebalanceStrategy sarama.BalanceStrategy // 重平衡策略
	AutoCommit        bool                   // 是否自动提交 offset
	CommitInterval    time.Duration          // 自动提交间隔
}

// DefaultConsumerConfig 默认消费者配置
func DefaultConsumerConfig(brokers []string, groupID string, topics []string) *ConsumerConfig {
	return &ConsumerConfig{
		Brokers:           brokers,
		GroupID:           groupID,
		Topics:            topics,
		OffsetInitial:     sarama.OffsetNewest,
		SessionTimeout:    30 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		RebalanceStrategy: sarama.NewBalanceStrategyRoundRobin(),
		AutoCommit:        true,
		CommitInterval:    1 * time.Second,
	}
}

// Consumer Kafka 消费者组
type Consumer struct {
	client   sarama.ConsumerGroup
	config   *ConsumerConfig
	handlers map[string]MessageHandler
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	running  bool
}

// MessageHandler 消息处理接口
type MessageHandler interface {
	Handle(ctx context.Context, message *sarama.ConsumerMessage) error
}

// MessageHandlerFunc 函数类型的消息处理器
type MessageHandlerFunc func(ctx context.Context, message *sarama.ConsumerMessage) error

func (f MessageHandlerFunc) Handle(ctx context.Context, message *sarama.ConsumerMessage) error {
	return f(ctx, message)
}

// NewConsumer 创建 Kafka Consumer
func NewConsumer(config *ConsumerConfig) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()

	// 消费者配置
	saramaConfig.Consumer.Group.Session.Timeout = config.SessionTimeout
	saramaConfig.Consumer.Group.Heartbeat.Interval = config.HeartbeatInterval
	saramaConfig.Consumer.Group.Rebalance.Strategy = config.RebalanceStrategy
	saramaConfig.Consumer.Offsets.Initial = config.OffsetInitial
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = config.AutoCommit
	saramaConfig.Consumer.Offsets.AutoCommit.Interval = config.CommitInterval

	// 返回错误
	saramaConfig.Consumer.Return.Errors = true

	client, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Consumer{
		client:   client,
		config:   config,
		handlers: make(map[string]MessageHandler),
		ctx:      ctx,
		cancel:   cancel,
	}

	hlog.Infof("[Kafka Consumer] Connected to brokers: %v, group: %s", config.Brokers, config.GroupID)
	return c, nil
}

// RegisterHandler 注册消息处理器
func (c *Consumer) RegisterHandler(topic string, handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = handler
}

// RegisterHandlerFunc 注册函数类型的消息处理器
func (c *Consumer) RegisterHandlerFunc(topic string, handler func(ctx context.Context, message *sarama.ConsumerMessage) error) {
	c.RegisterHandler(topic, MessageHandlerFunc(handler))
}

// Start 启动消费者
func (c *Consumer) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer is already running")
	}
	c.running = true
	c.mu.Unlock()

	c.wg.Add(1)
	go c.consumeLoop()

	return nil
}

// consumeLoop 消费循环
func (c *Consumer) consumeLoop() {
	defer c.wg.Done()

	handler := &consumerGroupHandler{
		consumer: c,
	}

	for {
		select {
		case <-c.ctx.Done():
			hlog.Info("[Kafka Consumer] Context cancelled, stopping consumer loop")
			return
		default:
			// 消费消息
			err := c.client.Consume(c.ctx, c.config.Topics, handler)
			if err != nil {
				hlog.Errorf("[Kafka Consumer] Error during consume: %v", err)
				// 短暂等待后重试
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}
	}
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	c.cancel()
	c.wg.Wait()

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close consumer: %w", err)
	}

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	hlog.Info("[Kafka Consumer] Stopped")
	return nil
}

// consumerGroupHandler 实现 sarama.ConsumerGroupHandler 接口
type consumerGroupHandler struct {
	consumer *Consumer
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	hlog.Infof("[Kafka Consumer] Consumer group setup: member=%s, generation=%d",
		session.MemberID(), session.GenerationID())
	return nil
}

func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	hlog.Infof("[Kafka Consumer] Consumer group cleanup: member=%s", session.MemberID())
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message, ok := <-claim.Messages():
			if !ok {
				return nil
			}

			h.consumer.mu.RLock()
			handler, exists := h.consumer.handlers[message.Topic]
			h.consumer.mu.RUnlock()

			if !exists {
				hlog.Warnf("[Kafka Consumer] No handler for topic: %s", message.Topic)
				session.MarkMessage(message, "")
				continue
			}

			ctx := context.Background()
			if err := handler.Handle(ctx, message); err != nil {
				hlog.Errorf("[Kafka Consumer] Failed to handle message: topic=%s, partition=%d, offset=%d, error=%v",
					message.Topic, message.Partition, message.Offset, err)
				// 根据业务需求决定是否继续消费
				// 这里选择继续消费，避免阻塞
			}

			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// ============ 类型化的事件消费处理器 ============

// UserBehaviorHandler 用户行为事件处理器接口
type UserBehaviorHandler interface {
	HandleUserBehavior(ctx context.Context, event *UserBehaviorEvent) error
}

// VideoViewHandler 视频播放事件处理器接口
type VideoViewHandler interface {
	HandleVideoView(ctx context.Context, event *VideoViewEvent) error
}

// VideoExposureHandler 视频曝光事件处理器接口
type VideoExposureHandler interface {
	HandleVideoExposure(ctx context.Context, event *VideoExposureEvent) error
}

// SearchLogHandler 搜索日志事件处理器接口
type SearchLogHandler interface {
	HandleSearchLog(ctx context.Context, event *SearchLogEvent) error
}

// UserProfileUpdateHandler 用户画像更新事件处理器接口
type UserProfileUpdateHandler interface {
	HandleUserProfileUpdate(ctx context.Context, event *UserProfileUpdateEvent) error
}

// VideoFeatureUpdateHandler 视频特征更新事件处理器接口
type VideoFeatureUpdateHandler interface {
	HandleVideoFeatureUpdate(ctx context.Context, event *VideoFeatureUpdateEvent) error
}

// VideoStatsHandler 视频统计事件处理器接口
type VideoStatsHandler interface {
	HandleVideoStats(ctx context.Context, event *VideoStatsEvent) error
}

// RecommendationHandler 推荐事件处理器接口
type RecommendationHandler interface {
	HandleRecommendation(ctx context.Context, event *RecommendationEvent) error
}

// ============ 便捷注册方法 ============

// RegisterUserBehaviorHandler 注册用户行为事件处理器
func (c *Consumer) RegisterUserBehaviorHandler(handler UserBehaviorHandler) {
	c.RegisterHandlerFunc(TopicUserBehavior, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event UserBehaviorEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal UserBehaviorEvent: %w", err)
		}
		return handler.HandleUserBehavior(ctx, &event)
	})
}

// RegisterVideoViewHandler 注册视频播放事件处理器
func (c *Consumer) RegisterVideoViewHandler(handler VideoViewHandler) {
	c.RegisterHandlerFunc(TopicVideoView, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event VideoViewEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal VideoViewEvent: %w", err)
		}
		return handler.HandleVideoView(ctx, &event)
	})
}

// RegisterVideoExposureHandler 注册视频曝光事件处理器
func (c *Consumer) RegisterVideoExposureHandler(handler VideoExposureHandler) {
	c.RegisterHandlerFunc(TopicVideoExposure, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event VideoExposureEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal VideoExposureEvent: %w", err)
		}
		return handler.HandleVideoExposure(ctx, &event)
	})
}

// RegisterSearchLogHandler 注册搜索日志事件处理器
func (c *Consumer) RegisterSearchLogHandler(handler SearchLogHandler) {
	c.RegisterHandlerFunc(TopicSearchLog, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event SearchLogEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal SearchLogEvent: %w", err)
		}
		return handler.HandleSearchLog(ctx, &event)
	})
}

// RegisterUserProfileUpdateHandler 注册用户画像更新事件处理器
func (c *Consumer) RegisterUserProfileUpdateHandler(handler UserProfileUpdateHandler) {
	c.RegisterHandlerFunc(TopicUserProfile, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event UserProfileUpdateEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal UserProfileUpdateEvent: %w", err)
		}
		return handler.HandleUserProfileUpdate(ctx, &event)
	})
}

// RegisterVideoFeatureUpdateHandler 注册视频特征更新事件处理器
func (c *Consumer) RegisterVideoFeatureUpdateHandler(handler VideoFeatureUpdateHandler) {
	c.RegisterHandlerFunc(TopicVideoFeature, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event VideoFeatureUpdateEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal VideoFeatureUpdateEvent: %w", err)
		}
		return handler.HandleVideoFeatureUpdate(ctx, &event)
	})
}

// RegisterVideoStatsHandler 注册视频统计事件处理器
func (c *Consumer) RegisterVideoStatsHandler(handler VideoStatsHandler) {
	c.RegisterHandlerFunc(TopicVideoStats, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event VideoStatsEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal VideoStatsEvent: %w", err)
		}
		return handler.HandleVideoStats(ctx, &event)
	})
}

// RegisterRecommendationHandler 注册推荐事件处理器
func (c *Consumer) RegisterRecommendationHandler(handler RecommendationHandler) {
	c.RegisterHandlerFunc(TopicRecommendation, func(ctx context.Context, msg *sarama.ConsumerMessage) error {
		var event RecommendationEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal RecommendationEvent: %w", err)
		}
		return handler.HandleRecommendation(ctx, &event)
	})
}
