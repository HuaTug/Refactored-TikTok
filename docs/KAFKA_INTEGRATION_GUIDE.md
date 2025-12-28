# Kafka 集成指南

## 概述

本项目已集成 Kafka 用于处理**高吞吐量**的事件流场景，与 RabbitMQ 形成互补：

| 消息队列 | 使用场景 | 特点 |
|---------|---------|------|
| **RabbitMQ** | 点赞、评论、通知 | 低延迟、强一致性、事务支持 |
| **Kafka** | 用户行为、播放统计、推荐特征 | 高吞吐量、持久化、多消费者 |

## 架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Kafka 事件流架构                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  用户行为采集                     实时处理                      下游系统     │
│  ┌──────────────┐           ┌──────────────┐           ┌──────────────┐    │
│  │ Video Service │ ────────►│ user_behavior│───┬──────►│ 推荐系统      │    │
│  └──────────────┘           └──────────────┘   │       └──────────────┘    │
│                                                │                            │
│  ┌──────────────┐           ┌──────────────┐   │       ┌──────────────┐    │
│  │ API Gateway  │ ────────►│ video_view   │───┼──────►│ 数据分析      │    │
│  └──────────────┘           └──────────────┘   │       └──────────────┘    │
│                                                │                            │
│  ┌──────────────┐           ┌──────────────┐   │       ┌──────────────┐    │
│  │ Feed Service │ ────────►│video_exposure│───┴──────►│ 数据仓库      │    │
│  └──────────────┘           └──────────────┘           └──────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 快速开始

### 1. 配置

在 `config/config.yml` 中配置 Kafka：

```yaml
kafka:
  brokers:
    - localhost:9092
  version: "2.8.0"
  producer_retries: 3
  consumer_offset_init: newest
```

### 2. 初始化 Kafka Manager

```go
package main

import (
    "HuaTug.com/config"
    "HuaTug.com/pkg/kafka"
)

func main() {
    // 初始化配置
    config.Init()
    
    // 创建 Kafka Manager
    kafkaConfig := &kafka.KafkaConfig{
        Brokers:         config.ConfigInfo.Kafka.Brokers,
        Version:         config.ConfigInfo.Kafka.Version,
        ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
    }
    
    manager, err := kafka.NewManager(kafkaConfig)
    if err != nil {
        panic(err)
    }
    defer manager.Close()
    
    // 初始化所有 Topics
    if err := manager.InitTopics(); err != nil {
        panic(err)
    }
}
```

## 使用场景

### 场景 1: 用户行为日志采集

适用于收集用户在平台上的所有行为数据，用于后续分析和推荐。

```go
import (
    "context"
    "time"
    "HuaTug.com/pkg/kafka"
)

// 在 Video Service 中记录用户行为
func (s *VideoService) RecordUserBehavior(ctx context.Context, userID, videoID int64, behavior kafka.BehaviorType) error {
    event := &kafka.UserBehaviorEvent{
        UserID:     userID,
        VideoID:    videoID,
        Behavior:   behavior,
        Timestamp:  time.Now(),
        DeviceType: "mobile",
        Platform:   "ios",
    }
    
    // 使用高吞吐量生产者发送
    return s.kafkaManager.PublishUserBehavior(ctx, event)
}

// 使用示例
func (s *VideoService) OnVideoPlay(ctx context.Context, userID, videoID int64) {
    // 记录播放行为
    s.RecordUserBehavior(ctx, userID, videoID, kafka.BehaviorPlay)
}

func (s *VideoService) OnVideoComplete(ctx context.Context, userID, videoID int64, watchTime int64) {
    // 记录完播行为
    event := &kafka.UserBehaviorEvent{
        UserID:    userID,
        VideoID:   videoID,
        Behavior:  kafka.BehaviorComplete,
        Duration:  watchTime,
        Timestamp: time.Now(),
    }
    s.kafkaManager.PublishUserBehavior(ctx, event)
}
```

### 场景 2: 视频播放统计

适用于实时统计视频播放数据，更新热度分数。

```go
// 记录视频播放事件
func (s *VideoService) RecordVideoView(ctx context.Context, view *VideoViewRequest) error {
    event := &kafka.VideoViewEvent{
        VideoID:       view.VideoID,
        UserID:        view.UserID,
        AuthorID:      view.AuthorID,
        WatchTime:     view.WatchTime,
        VideoDuration: view.VideoDuration,
        WatchPercent:  float64(view.WatchTime) / float64(view.VideoDuration),
        IsComplete:    view.WatchTime >= view.VideoDuration * 90 / 100, // 90%算完播
        Source:        view.Source,
    }
    
    return s.kafkaManager.PublishVideoView(ctx, event)
}

// 实时更新视频统计计数
func (s *VideoService) IncrementVideoStats(ctx context.Context, videoID int64, statsType string) error {
    event := &kafka.VideoStatsEvent{
        VideoID:   videoID,
        StatsType: statsType, // "play", "like", "comment", "share"
        Delta:     1,
    }
    
    return s.kafkaManager.PublishVideoStats(ctx, event)
}
```

### 场景 3: 推荐系统特征更新

适用于实时更新用户画像和视频特征，供推荐系统使用。

```go
// 更新用户画像
func (s *RecommendService) UpdateUserProfile(ctx context.Context, userID int64, tags []string, scores map[string]float64) error {
    event := &kafka.UserProfileUpdateEvent{
        UserID:     userID,
        UpdateType: "interest",
        Tags:       tags,
        Scores:     scores,
    }
    
    return s.kafkaManager.PublishUserProfileUpdate(ctx, event)
}

// 更新视频特征
func (s *RecommendService) UpdateVideoFeature(ctx context.Context, videoID int64, stats VideoStats) error {
    event := &kafka.VideoFeatureUpdateEvent{
        VideoID:      videoID,
        UpdateType:   "stats",
        PlayCount:    stats.PlayCount,
        LikeCount:    stats.LikeCount,
        CommentCount: stats.CommentCount,
        ShareCount:   stats.ShareCount,
        HotScore:     stats.CalculateHotScore(),
    }
    
    return s.kafkaManager.PublishVideoFeatureUpdate(ctx, event)
}

// 记录推荐结果（用于效果评估）
func (s *RecommendService) LogRecommendation(ctx context.Context, userID int64, videoIDs []int64, scores []float64) error {
    event := &kafka.RecommendationEvent{
        UserID:       userID,
        VideoIDs:     videoIDs,
        Scores:       scores,
        RecallType:   "collaborative_filtering",
        ABTestGroup:  "experiment_v2",
        ModelVersion: "model_20250101",
    }
    
    return s.kafkaManager.PublishRecommendation(ctx, event)
}
```

### 场景 4: 视频曝光追踪

适用于评估推荐效果，计算点击率等指标。

```go
// 记录视频曝光
func (s *FeedService) RecordExposure(ctx context.Context, userID int64, videoIDs []int64, recallType string) error {
    event := &kafka.VideoExposureEvent{
        UserID:     userID,
        VideoIDs:   videoIDs,
        Source:     "feed",
        RecallType: recallType,
    }
    
    return s.kafkaManager.PublishVideoExposure(ctx, event)
}
```

### 场景 5: 搜索日志

适用于搜索词分析和搜索排序优化。

```go
// 记录搜索行为
func (s *SearchService) LogSearch(ctx context.Context, userID int64, query string, resultCount int, clickedIDs []int64) error {
    event := &kafka.SearchLogEvent{
        UserID:      userID,
        Query:       query,
        ResultCount: resultCount,
        ClickedIDs:  clickedIDs,
    }
    
    return s.kafkaManager.PublishSearchLog(ctx, event)
}
```

## 消费者实现

### 创建消费者

```go
func StartAnalyticsConsumer(manager *kafka.Manager) error {
    // 创建消费者
    consumer, err := manager.CreateConsumer(
        kafka.GroupAnalytics,
        []string{kafka.TopicUserBehavior, kafka.TopicVideoView},
    )
    if err != nil {
        return err
    }
    
    // 注册处理器
    consumer.RegisterUserBehaviorHandler(&AnalyticsUserBehaviorHandler{})
    consumer.RegisterVideoViewHandler(&AnalyticsVideoViewHandler{})
    
    // 启动消费
    return consumer.Start()
}

// 用户行为处理器
type AnalyticsUserBehaviorHandler struct{}

func (h *AnalyticsUserBehaviorHandler) HandleUserBehavior(ctx context.Context, event *kafka.UserBehaviorEvent) error {
    // 处理用户行为事件
    log.Printf("Received user behavior: user=%d, video=%d, behavior=%s", 
        event.UserID, event.VideoID, event.Behavior)
    
    // 写入数据仓库、更新实时统计等
    return nil
}

// 视频播放处理器
type AnalyticsVideoViewHandler struct{}

func (h *AnalyticsVideoViewHandler) HandleVideoView(ctx context.Context, event *kafka.VideoViewEvent) error {
    // 处理视频播放事件
    log.Printf("Received video view: video=%d, user=%d, watchTime=%dms", 
        event.VideoID, event.UserID, event.WatchTime)
    
    // 更新视频统计、计算完播率等
    return nil
}
```

### 推荐系统消费者

```go
func StartRecommendConsumer(manager *kafka.Manager) error {
    consumer, err := manager.CreateConsumer(
        kafka.GroupRecommend,
        []string{
            kafka.TopicUserBehavior,
            kafka.TopicUserProfile,
            kafka.TopicVideoFeature,
        },
    )
    if err != nil {
        return err
    }
    
    // 注册推荐系统处理器
    consumer.RegisterUserBehaviorHandler(&RecommendUserBehaviorHandler{
        profileUpdater: NewUserProfileUpdater(),
    })
    
    consumer.RegisterUserProfileUpdateHandler(&RecommendUserProfileHandler{
        recommendEngine: NewRecommendEngine(),
    })
    
    return consumer.Start()
}
```

## 生产者配置说明

### 三种预设配置

| 配置 | 适用场景 | ACK模式 | 重试次数 | 压缩 |
|-----|---------|--------|---------|------|
| **DefaultProducerConfig** | 通用场景 | WaitForLocal | 3 | Snappy |
| **HighThroughputConfig** | 日志类事件 | NoResponse | 1 | LZ4 |
| **HighReliabilityConfig** | CDC事件 | WaitForAll | 5 | Snappy |

### 使用不同的生产者

```go
// 高吞吐量场景（用户行为、播放统计）
manager.GetHighThroughputProducer().PublishUserBehavior(ctx, event)

// 高可靠性场景（CDC事件）
manager.GetHighReliabilityProducer().PublishCDCEvent(ctx, topic, event)

// 默认场景（推荐特征更新）
manager.GetDefaultProducer().PublishUserProfileUpdate(ctx, event)
```

## Topic 设计

### 已定义的 Topics

| Topic | 分区数 | 用途 |
|-------|-------|------|
| `user_behavior` | 12 | 用户行为日志 |
| `video_view` | 12 | 视频播放事件 |
| `video_exposure` | 6 | 视频曝光事件 |
| `search_log` | 6 | 搜索日志 |
| `recommendation` | 6 | 推荐事件 |
| `user_profile_update` | 6 | 用户画像更新 |
| `video_feature_update` | 6 | 视频特征更新 |
| `video_stats` | 12 | 视频统计 |
| `realtime_stats` | 6 | 实时统计 |
| `cdc_video` | 6 | 视频数据变更 |
| `cdc_user` | 6 | 用户数据变更 |

### 分区键设计

- **用户相关事件**: 使用 `user_id` 作为 key，保证同一用户事件有序
- **视频相关事件**: 使用 `video_id` 作为 key，保证同一视频事件有序

## 与 RabbitMQ 的职责划分

```
┌─────────────────────────────────────────────────────────────────┐
│                        事件类型选择                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  需要实时反馈？          是 ──────────► RabbitMQ                 │
│       │                               (点赞、评论、通知)         │
│       否                                                        │
│       │                                                         │
│       ▼                                                         │
│  需要强一致性？          是 ──────────► RabbitMQ                 │
│       │                               (订单、支付)              │
│       否                                                        │
│       │                                                         │
│       ▼                                                         │
│  高吞吐量？              是 ──────────► Kafka                   │
│  需要多消费者？                        (行为日志、统计、推荐)     │
│  需要数据持久化？                                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 监控与运维

### 健康检查

```go
if err := manager.HealthCheck(); err != nil {
    log.Errorf("Kafka health check failed: %v", err)
}
```

### 集群信息

```go
info, err := manager.GetClusterInfo()
if err == nil {
    log.Printf("Kafka cluster info: %+v", info)
}
```

### Kafka UI

访问 `http://localhost:8080` 查看 Kafka UI (由 docker-compose 提供)

## 参考文档

- [Apache Kafka 官方文档](https://kafka.apache.org/documentation/)
- [Sarama Go Client](https://github.com/IBM/sarama)
- [Kafka vs RabbitMQ 场景选择](./KAFKA_VS_RABBITMQ_SCENARIOS.md)
