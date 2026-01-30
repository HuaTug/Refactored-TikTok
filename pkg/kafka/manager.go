package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers            []string `yaml:"brokers" mapstructure:"brokers"`
	Version            string   `yaml:"version" mapstructure:"version"`
	ProducerRetries    int      `yaml:"producer_retries" mapstructure:"producer_retries"`
	ConsumerOffsetInit string   `yaml:"consumer_offset_init" mapstructure:"consumer_offset_init"` // newest / oldest
}

// Manager Kafka 统一管理器
type Manager struct {
	config *KafkaConfig

	// 生产者
	defaultProducer *Producer // 默认生产者 (平衡配置)
	highTpProducer  *Producer // 高吞吐量生产者
	highRelProducer *Producer // 高可靠性生产者

	// 消费者
	consumers map[string]*Consumer

	// admin client
	admin sarama.ClusterAdmin

	mu     sync.RWMutex
	closed bool
}

// NewManager 创建 Kafka 管理器
func NewManager(config *KafkaConfig) (*Manager, error) {
	// 创建 admin client
	saramaConfig := sarama.NewConfig()
	saramaConfig.Version = sarama.V2_8_0_0 // 默认使用 Kafka 2.8+

	admin, err := sarama.NewClusterAdmin(config.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster admin: %w", err)
	}

	m := &Manager{
		config:    config,
		consumers: make(map[string]*Consumer),
		admin:     admin,
	}

	// 初始化默认生产者
	if err := m.initProducers(); err != nil {
		admin.Close()
		return nil, err
	}

	hlog.Infof("[Kafka Manager] Initialized with brokers: %v", config.Brokers)
	return m, nil
}

// initProducers 初始化生产者
func (m *Manager) initProducers() error {
	var err error

	// 默认生产者
	m.defaultProducer, err = NewProducer(DefaultProducerConfig(m.config.Brokers))
	if err != nil {
		return fmt.Errorf("failed to create default producer: %w", err)
	}

	// 高吞吐量生产者
	m.highTpProducer, err = NewProducer(HighThroughputConfig(m.config.Brokers))
	if err != nil {
		m.defaultProducer.Close()
		return fmt.Errorf("failed to create high throughput producer: %w", err)
	}

	// 高可靠性生产者
	m.highRelProducer, err = NewProducer(HighReliabilityConfig(m.config.Brokers))
	if err != nil {
		m.defaultProducer.Close()
		m.highTpProducer.Close()
		return fmt.Errorf("failed to create high reliability producer: %w", err)
	}

	return nil
}

// GetDefaultProducer 获取默认生产者
func (m *Manager) GetDefaultProducer() *Producer {
	return m.defaultProducer
}

// GetHighThroughputProducer 获取高吞吐量生产者
func (m *Manager) GetHighThroughputProducer() *Producer {
	return m.highTpProducer
}

// GetHighReliabilityProducer 获取高可靠性生产者
func (m *Manager) GetHighReliabilityProducer() *Producer {
	return m.highRelProducer
}

// CreateConsumer 创建消费者
func (m *Manager) CreateConsumer(groupID string, topics []string) (*Consumer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("manager is closed")
	}

	config := DefaultConsumerConfig(m.config.Brokers, groupID, topics)
	consumer, err := NewConsumer(config)
	if err != nil {
		return nil, err
	}

	m.consumers[groupID] = consumer
	return consumer, nil
}

// GetConsumer 获取已创建的消费者
func (m *Manager) GetConsumer(groupID string) (*Consumer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	consumer, ok := m.consumers[groupID]
	return consumer, ok
}

// CreateTopic 创建 Topic
func (m *Manager) CreateTopic(topic string, numPartitions int32, replicationFactor int16) error {
	detail := &sarama.TopicDetail{
		NumPartitions:     numPartitions,
		ReplicationFactor: replicationFactor,
	}

	err := m.admin.CreateTopic(topic, detail, false)
	if err != nil {
		// 如果 topic 已存在，不报错
		if err.Error() == "kafka server: Topic with this name already exists." ||
			err.Error() == sarama.ErrTopicAlreadyExists.Error() {
			hlog.Infof("[Kafka Manager] Topic %s already exists", topic)
			return nil
		}
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	hlog.Infof("[Kafka Manager] Created topic: %s (partitions=%d, replication=%d)",
		topic, numPartitions, replicationFactor)
	return nil
}

// DeleteTopic 删除 Topic
func (m *Manager) DeleteTopic(topic string) error {
	err := m.admin.DeleteTopic(topic)
	if err != nil {
		return fmt.Errorf("failed to delete topic %s: %w", topic, err)
	}

	hlog.Infof("[Kafka Manager] Deleted topic: %s", topic)
	return nil
}

// ListTopics 列出所有 Topics
func (m *Manager) ListTopics() ([]string, error) {
	topics, err := m.admin.ListTopics()
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}

	result := make([]string, 0, len(topics))
	for topic := range topics {
		result = append(result, topic)
	}
	return result, nil
}

// DescribeTopic 获取 Topic 详情
func (m *Manager) DescribeTopic(topic string) (*sarama.TopicMetadata, error) {
	metadata, err := m.admin.DescribeTopics([]string{topic})
	if err != nil {
		return nil, fmt.Errorf("failed to describe topic %s: %w", topic, err)
	}

	if len(metadata) == 0 {
		return nil, fmt.Errorf("topic %s not found", topic)
	}

	return metadata[0], nil
}

// InitTopics 初始化所有需要的 Topics
func (m *Manager) InitTopics() error {
	topics := []struct {
		Name       string
		Partitions int32
		Replicas   int16
	}{
		// 用户行为 Topics - 高吞吐量，多分区
		{TopicUserBehavior, 12, 1},
		{TopicVideoView, 12, 1},
		{TopicVideoExposure, 6, 1},
		{TopicSearchLog, 6, 1},
		{TopicUserActivityLog, 6, 1},

		// 推荐系统 Topics
		{TopicRecommendation, 6, 1},
		{TopicUserProfile, 6, 1},
		{TopicVideoFeature, 6, 1},

		// 实时统计 Topics
		{TopicRealtimeStats, 6, 1},
		{TopicVideoStats, 12, 1},

		// CDC Topics
		{TopicCDCVideo, 6, 1},
		{TopicCDCUser, 6, 1},

		// 日志系统 Topics
		{TopicServiceLog, 6, 1},
		{TopicErrorLog, 6, 1},
		{TopicAccessLog, 6, 1},
		{TopicAuditLog, 6, 1},
		{TopicAlertLog, 6, 1},
	}

	for _, t := range topics {
		if err := m.CreateTopic(t.Name, t.Partitions, t.Replicas); err != nil {
			return err
		}
	}

	hlog.Info("[Kafka Manager] All topics initialized")
	return nil
}

// Close 关闭管理器
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	var errs []error

	// 关闭所有消费者
	for id, consumer := range m.consumers {
		if err := consumer.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop consumer %s: %w", id, err))
		}
	}

	// 关闭生产者
	if m.defaultProducer != nil {
		if err := m.defaultProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close default producer: %w", err))
		}
	}
	if m.highTpProducer != nil {
		if err := m.highTpProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close high throughput producer: %w", err))
		}
	}
	if m.highRelProducer != nil {
		if err := m.highRelProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close high reliability producer: %w", err))
		}
	}

	// 关闭 admin client
	if m.admin != nil {
		if err := m.admin.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close admin: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	hlog.Info("[Kafka Manager] Closed")
	return nil
}

// ============ 便捷发布方法 ============

// PublishUserBehavior 发布用户行为事件 (使用高吞吐量生产者)
func (m *Manager) PublishUserBehavior(ctx context.Context, event *UserBehaviorEvent) error {
	return m.highTpProducer.PublishUserBehavior(ctx, event)
}

// PublishVideoView 发布视频播放事件 (使用高吞吐量生产者)
func (m *Manager) PublishVideoView(ctx context.Context, event *VideoViewEvent) error {
	return m.highTpProducer.PublishVideoView(ctx, event)
}

// PublishVideoExposure 发布视频曝光事件 (使用高吞吐量生产者)
func (m *Manager) PublishVideoExposure(ctx context.Context, event *VideoExposureEvent) error {
	return m.highTpProducer.PublishVideoExposure(ctx, event)
}

// PublishSearchLog 发布搜索日志事件 (使用高吞吐量生产者)
func (m *Manager) PublishSearchLog(ctx context.Context, event *SearchLogEvent) error {
	return m.highTpProducer.PublishSearchLog(ctx, event)
}

// PublishUserProfileUpdate 发布用户画像更新事件 (使用默认生产者)
func (m *Manager) PublishUserProfileUpdate(ctx context.Context, event *UserProfileUpdateEvent) error {
	return m.defaultProducer.PublishUserProfileUpdate(ctx, event)
}

// PublishVideoFeatureUpdate 发布视频特征更新事件 (使用默认生产者)
func (m *Manager) PublishVideoFeatureUpdate(ctx context.Context, event *VideoFeatureUpdateEvent) error {
	return m.defaultProducer.PublishVideoFeatureUpdate(ctx, event)
}

// PublishVideoStats 发布视频统计事件 (使用高吞吐量生产者)
func (m *Manager) PublishVideoStats(ctx context.Context, event *VideoStatsEvent) error {
	return m.highTpProducer.PublishVideoStats(ctx, event)
}

// PublishRecommendation 发布推荐事件 (使用默认生产者)
func (m *Manager) PublishRecommendation(ctx context.Context, event *RecommendationEvent) error {
	return m.defaultProducer.PublishRecommendation(ctx, event)
}

// PublishCDCEvent 发布 CDC 事件 (使用高可靠性生产者)
func (m *Manager) PublishCDCEvent(ctx context.Context, topic string, event *CDCEvent) error {
	return m.highRelProducer.PublishCDCEvent(ctx, topic, event)
}

// ============ 日志系统发布方法 ============

// PublishServiceLog 发布服务调用日志 (使用高吞吐量生产者)
func (m *Manager) PublishServiceLog(ctx context.Context, event *ServiceLogEvent) error {
	return m.highTpProducer.PublishServiceLog(ctx, event)
}

// PublishErrorLog 发布错误日志 (使用默认生产者，保证可靠性)
func (m *Manager) PublishErrorLog(ctx context.Context, event *ErrorLogEvent) error {
	return m.defaultProducer.PublishErrorLog(ctx, event)
}

// PublishAccessLog 发布访问日志 (使用高吞吐量生产者)
func (m *Manager) PublishAccessLog(ctx context.Context, event *AccessLogEvent) error {
	return m.highTpProducer.PublishAccessLog(ctx, event)
}

// PublishAuditLog 发布审计日志 (使用高可靠性生产者)
func (m *Manager) PublishAuditLog(ctx context.Context, event *AuditLogEvent) error {
	return m.highRelProducer.PublishAuditLog(ctx, event)
}

// PublishAlertLog 发布告警日志 (使用默认生产者)
func (m *Manager) PublishAlertLog(ctx context.Context, event *AlertLogEvent) error {
	return m.defaultProducer.PublishAlertLog(ctx, event)
}

// ============ 健康检查 ============

// HealthCheck 健康检查
func (m *Manager) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 检查是否能获取 broker 信息
	brokers, _, err := m.admin.DescribeCluster()
	if err != nil {
		return fmt.Errorf("failed to describe cluster: %w", err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("health check timeout")
	default:
		if len(brokers) == 0 {
			return fmt.Errorf("no brokers available")
		}
	}

	return nil
}

// GetClusterInfo 获取集群信息
func (m *Manager) GetClusterInfo() (map[string]interface{}, error) {
	brokers, controllerID, err := m.admin.DescribeCluster()
	if err != nil {
		return nil, fmt.Errorf("failed to describe cluster: %w", err)
	}

	brokerInfos := make([]map[string]interface{}, 0, len(brokers))
	for _, broker := range brokers {
		brokerInfos = append(brokerInfos, map[string]interface{}{
			"id":   broker.ID(),
			"addr": broker.Addr(),
		})
	}

	topics, err := m.admin.ListTopics()
	if err != nil {
		return nil, fmt.Errorf("failed to list topics: %w", err)
	}

	topicNames := make([]string, 0, len(topics))
	for topic := range topics {
		topicNames = append(topicNames, topic)
	}

	return map[string]interface{}{
		"brokers":       brokerInfos,
		"controller_id": controllerID,
		"topics":        topicNames,
		"topic_count":   len(topicNames),
	}, nil
}
