package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// ProducerConfig Kafka Producer 配置
type ProducerConfig struct {
	Brokers         []string                // Kafka broker 地址列表
	RequiredAcks    sarama.RequiredAcks     // 确认模式
	Retries         int                     // 重试次数
	RetryBackoff    time.Duration           // 重试间隔
	MaxMessageBytes int                     // 最大消息大小
	Compression     sarama.CompressionCodec // 压缩算法
	FlushFrequency  time.Duration           // 刷新频率
	FlushMessages   int                     // 刷新消息数
}

// DefaultProducerConfig 默认配置
func DefaultProducerConfig(brokers []string) *ProducerConfig {
	return &ProducerConfig{
		Brokers:         brokers,
		RequiredAcks:    sarama.WaitForLocal, // 只等待 leader 确认，平衡性能和可靠性
		Retries:         3,
		RetryBackoff:    100 * time.Millisecond,
		MaxMessageBytes: 1024 * 1024, // 1MB
		Compression:     sarama.CompressionSnappy,
		FlushFrequency:  100 * time.Millisecond,
		FlushMessages:   100,
	}
}

// HighReliabilityConfig 高可靠性配置（适用于重要事件）
func HighReliabilityConfig(brokers []string) *ProducerConfig {
	return &ProducerConfig{
		Brokers:         brokers,
		RequiredAcks:    sarama.WaitForAll, // 等待所有副本确认
		Retries:         5,
		RetryBackoff:    200 * time.Millisecond,
		MaxMessageBytes: 1024 * 1024,
		Compression:     sarama.CompressionSnappy,
		FlushFrequency:  50 * time.Millisecond,
		FlushMessages:   50,
	}
}

// HighThroughputConfig 高吞吐量配置（适用于日志类事件）
func HighThroughputConfig(brokers []string) *ProducerConfig {
	return &ProducerConfig{
		Brokers:         brokers,
		RequiredAcks:    sarama.NoResponse, // 不等待确认，最高性能
		Retries:         1,
		RetryBackoff:    50 * time.Millisecond,
		MaxMessageBytes: 1024 * 1024,
		Compression:     sarama.CompressionLZ4, // LZ4压缩更快
		FlushFrequency:  500 * time.Millisecond,
		FlushMessages:   500,
	}
}

// Producer Kafka 异步生产者
type Producer struct {
	asyncProducer sarama.AsyncProducer
	config        *ProducerConfig
	wg            sync.WaitGroup
	closed        bool
	mu            sync.RWMutex
}

// NewProducer 创建 Kafka Producer
func NewProducer(config *ProducerConfig) (*Producer, error) {
	saramaConfig := sarama.NewConfig()

	// 生产者配置
	saramaConfig.Producer.RequiredAcks = config.RequiredAcks
	saramaConfig.Producer.Retry.Max = config.Retries
	saramaConfig.Producer.Retry.Backoff = config.RetryBackoff
	saramaConfig.Producer.MaxMessageBytes = config.MaxMessageBytes
	saramaConfig.Producer.Compression = config.Compression
	saramaConfig.Producer.Flush.Frequency = config.FlushFrequency
	saramaConfig.Producer.Flush.Messages = config.FlushMessages

	// 异步生产者配置
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true

	// 分区策略：按 key hash 分区，保证同一用户/视频的事件顺序
	saramaConfig.Producer.Partitioner = sarama.NewHashPartitioner

	asyncProducer, err := sarama.NewAsyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create async producer: %w", err)
	}

	p := &Producer{
		asyncProducer: asyncProducer,
		config:        config,
	}

	// 启动成功/错误处理 goroutine
	p.wg.Add(2)
	go p.handleSuccesses()
	go p.handleErrors()

	hlog.Infof("[Kafka Producer] Connected to brokers: %v", config.Brokers)
	return p, nil
}

// handleSuccesses 处理发送成功的消息
func (p *Producer) handleSuccesses() {
	defer p.wg.Done()
	for msg := range p.asyncProducer.Successes() {
		hlog.Debugf("[Kafka Producer] Message sent successfully: topic=%s, partition=%d, offset=%d",
			msg.Topic, msg.Partition, msg.Offset)
	}
}

// handleErrors 处理发送失败的消息
func (p *Producer) handleErrors() {
	defer p.wg.Done()
	for err := range p.asyncProducer.Errors() {
		hlog.Errorf("[Kafka Producer] Failed to send message: topic=%s, error=%v",
			err.Msg.Topic, err.Err)
	}
}

// Send 发送消息到指定 topic
func (p *Producer) Send(topic string, key string, value interface{}) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(data),
	}

	p.asyncProducer.Input() <- msg
	return nil
}

// SendWithPartition 发送消息到指定分区
func (p *Producer) SendWithPartition(topic string, partition int32, key string, value interface{}) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Partition: partition,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(data),
	}

	p.asyncProducer.Input() <- msg
	return nil
}

// Close 关闭生产者
func (p *Producer) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	if err := p.asyncProducer.Close(); err != nil {
		return fmt.Errorf("failed to close producer: %w", err)
	}

	p.wg.Wait()
	hlog.Info("[Kafka Producer] Closed")
	return nil
}

// ============ 便捷发送方法 ============

// PublishUserBehavior 发送用户行为事件
func (p *Producer) PublishUserBehavior(ctx context.Context, event *UserBehaviorEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	// 使用 UserID 作为 key，保证同一用户的事件有序
	key := fmt.Sprintf("user_%d", event.UserID)
	return p.Send(TopicUserBehavior, key, event)
}

// PublishVideoView 发送视频播放事件
func (p *Producer) PublishVideoView(ctx context.Context, event *VideoViewEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	// 使用 VideoID 作为 key，保证同一视频的事件有序
	key := fmt.Sprintf("video_%d", event.VideoID)
	return p.Send(TopicVideoView, key, event)
}

// PublishVideoExposure 发送视频曝光事件
func (p *Producer) PublishVideoExposure(ctx context.Context, event *VideoExposureEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("user_%d", event.UserID)
	return p.Send(TopicVideoExposure, key, event)
}

// PublishSearchLog 发送搜索日志事件
func (p *Producer) PublishSearchLog(ctx context.Context, event *SearchLogEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("user_%d", event.UserID)
	return p.Send(TopicSearchLog, key, event)
}

// PublishUserProfileUpdate 发送用户画像更新事件
func (p *Producer) PublishUserProfileUpdate(ctx context.Context, event *UserProfileUpdateEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("user_%d", event.UserID)
	return p.Send(TopicUserProfile, key, event)
}

// PublishVideoFeatureUpdate 发送视频特征更新事件
func (p *Producer) PublishVideoFeatureUpdate(ctx context.Context, event *VideoFeatureUpdateEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("video_%d", event.VideoID)
	return p.Send(TopicVideoFeature, key, event)
}

// PublishRealtimeStats 发送实时统计事件
func (p *Producer) PublishRealtimeStats(ctx context.Context, event *RealtimeStatsEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := event.MetricName
	return p.Send(TopicRealtimeStats, key, event)
}

// PublishVideoStats 发送视频统计事件
func (p *Producer) PublishVideoStats(ctx context.Context, event *VideoStatsEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("video_%d", event.VideoID)
	return p.Send(TopicVideoStats, key, event)
}

// PublishRecommendation 发送推荐事件
func (p *Producer) PublishRecommendation(ctx context.Context, event *RecommendationEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	key := fmt.Sprintf("user_%d", event.UserID)
	return p.Send(TopicRecommendation, key, event)
}

// PublishCDCEvent 发送 CDC 事件
func (p *Producer) PublishCDCEvent(ctx context.Context, topic string, event *CDCEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	// 使用主键作为 key
	keyBytes, _ := json.Marshal(event.PrimaryKey)
	return p.Send(topic, string(keyBytes), event)
}
