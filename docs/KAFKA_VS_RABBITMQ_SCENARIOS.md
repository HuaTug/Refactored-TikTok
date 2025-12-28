# Kafka vs RabbitMQ 场景选择指南

## 📊 当前架构分析

### RabbitMQ 使用场景 (已实现)

当前项目中，RabbitMQ 主要用于处理**实时事务型事件**:

1. **点赞事件** (`LikeEvent`)
   - 用户点赞/取消点赞
   - 需要实时反馈
   - 需要保证幂等性
   - 需要事务一致性

2. **评论事件** (`CommentEvent`)
   - 创建/更新/删除评论
   - 需要实时通知
   - 需要强一致性保证

3. **通知事件** (`NotificationEvent`)
   - 用户通知推送
   - 需要可靠投递
   - 需要消息确认机制

**RabbitMQ 的优势**:
- ✅ 低延迟 (毫秒级)
- ✅ 强大的路由能力 (Exchange + Routing Key)
- ✅ 消息确认机制 (ACK/NACK)
- ✅ 优先级队列
- ✅ 死信队列 (DLQ)
- ✅ 适合复杂的业务逻辑

---

## 🚀 Kafka 适用场景

### 1. 用户行为日志采集 ⭐⭐⭐⭐⭐

**场景描述**: 收集用户在平台上的所有行为数据，用于后续分析

**具体行为**:
```
- 视频观看记录 (播放、暂停、快进、完播)
- 视频曝光记录 (Feed流中展示的视频)
- 搜索记录 (搜索词、搜索结果)
- 页面访问记录 (PV/UV)
- 点击行为 (按钮、链接)
- 滑动行为 (上滑下滑)
- 停留时长
```

**为什么用 Kafka**:
- 海量数据 (每秒百万级事件)
- 高吞吐量需求
- 允许一定延迟 (秒级)
- 需要持久化存储 (用于离线分析)
- 多个消费者 (实时分析、离线分析、推荐系统)

**数据流向**:
```
客户端 → API Gateway → Kafka → [实时分析、离线分析、推荐系统、数据仓库]
```

---

### 2. 推荐系统特征更新 ⭐⭐⭐⭐⭐

**场景描述**: 实时更新用户画像和视频特征，用于推荐系统

**数据类型**:
```
- 用户兴趣变化 (观看分类、标签偏好)
- 视频热度变化 (播放量、点赞量实时统计)
- 用户-视频交互记录
- 用户活跃时段
```

**为什么用 Kafka**:
- 需要多个消费者同时订阅 (用户画像服务、视频特征服务、推荐引擎)
- 数据可重放 (新的推荐算法可以重新消费历史数据)
- 高吞吐量
- 顺序保证 (同一用户的行为按时间顺序处理)

**架构**:
```
用户行为 → Kafka Topic: user_behavior
                ↓
         ┌──────┼──────┐
         ↓      ↓      ↓
    用户画像  视频特征  推荐引擎
    更新服务  更新服务  实时计算
```

---

### 3. 实时数据分析 ⭐⭐⭐⭐

**场景描述**: 实时统计和监控平台核心指标

**分析指标**:
```
- 实时在线用户数
- 视频实时播放量
- 热门视频榜单 (1小时榜、24小时榜)
- 流量监控 (QPS、错误率)
- 用户增长趋势
- 视频发布趋势
```

**技术栈**:
```
Kafka → Flink/Spark Streaming → Redis/时序数据库 → Grafana
```

**为什么用 Kafka**:
- 流式处理友好
- 与 Flink/Spark 无缝集成
- 支持窗口计算 (滑动窗口、滚动窗口)
- 支持状态管理

---

### 4. 数据库 CDC (Change Data Capture) ⭐⭐⭐⭐

**场景描述**: 捕获数据库变更，实现数据同步和缓存更新

**应用**:
```
1. 主从数据库同步
2. 缓存更新 (MySQL → Kafka → Redis)
3. 搜索索引更新 (MySQL → Kafka → Elasticsearch)
4. 数据仓库同步 (OLTP → Kafka → OLAP)
```

**工具链**:
```
MySQL Binlog → Debezium → Kafka → Kafka Connect → [Redis, ES, Hive]
```

**为什么用 Kafka**:
- 解耦数据源和目标系统
- 支持多个下游消费者
- 数据可回放 (故障恢复)
- 保证数据顺序

---

### 5. 视频处理任务队列 ⭐⭐⭐

**场景描述**: 视频上传后的异步处理流程

**处理流程**:
```
视频上传 → Kafka Topic: video_upload
              ↓
      ┌───────┼───────┐
      ↓       ↓       ↓
   转码服务  审核服务  封面生成
      ↓       ↓       ↓
   Kafka    Kafka   Kafka
      ↓       ↓       ↓
   CDN上传  标签提取  通知发送
```

**为什么用 Kafka**:
- 任务解耦 (每个处理阶段独立)
- 可扩展 (增加消费者实例)
- 容错性 (消息持久化，处理失败可重试)
- 顺序保证 (同一视频的任务按顺序处理)

**对比 RabbitMQ**:
| 特性 | Kafka | RabbitMQ |
|------|-------|----------|
| 吞吐量 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 延迟 | 10-100ms | 1-10ms |
| 持久化 | 磁盘持久化 | 内存+磁盘 |
| 消息回溯 | 支持 | 不支持 |
| 适合场景 | 大批量视频处理 | 单个视频实时处理 |

---

### 6. 审计日志和安全事件 ⭐⭐⭐

**场景描述**: 记录所有敏感操作和安全事件

**日志类型**:
```
- 用户登录/登出
- 管理员操作
- 数据修改记录
- API调用记录
- 异常访问记录
- 安全告警
```

**为什么用 Kafka**:
- 不能丢失 (合规要求)
- 长期存储 (7天-30天)
- 支持审计查询
- 支持多系统消费 (安全中心、日志分析、告警系统)

---

### 7. 消息推送和通知分发 ⭐⭐⭐

**场景描述**: 大规模消息推送 (如系统公告、营销活动)

**推送类型**:
```
- 系统公告 (全员推送)
- 营销活动 (用户分组推送)
- 新功能通知
- 版本更新提醒
```

**为什么用 Kafka**:
- 百万级用户推送
- 支持分批处理
- 降低推送服务压力
- 支持推送状态追踪

**对比 RabbitMQ**:
- RabbitMQ: 适合点对点实时通知 (如点赞、评论通知)
- Kafka: 适合大规模广播推送 (如系统公告)

---

## 🔧 Kafka 实现方案

### 场景 1: 用户行为日志采集

#### 数据模型

```go
// UserBehaviorEvent 用户行为事件
type UserBehaviorEvent struct {
    EventID     string                 `json:"event_id"`     // 事件ID
    UserID      int64                  `json:"user_id"`      // 用户ID
    EventType   string                 `json:"event_type"`   // view, click, search, scroll
    VideoID     int64                  `json:"video_id"`     // 视频ID (可选)
    ActionType  string                 `json:"action_type"`  // play, pause, seek, complete
    Timestamp   int64                  `json:"timestamp"`    // 时间戳
    Duration    int64                  `json:"duration"`     // 观看时长
    Position    float64                `json:"position"`     // 播放进度
    Source      string                 `json:"source"`       // feed, search, recommend
    DeviceType  string                 `json:"device_type"`  // mobile, web
    Platform    string                 `json:"platform"`     // ios, android, web
    SessionID   string                 `json:"session_id"`   // 会话ID
    Extra       map[string]interface{} `json:"extra"`        // 额外字段
}
```

#### Producer 实现

```go
// 文件: pkg/kafka/behavior_producer.go
package kafka

import (
    "context"
    "encoding/json"
    "github.com/segmentio/kafka-go"
)

type BehaviorProducer struct {
    writer *kafka.Writer
}

func NewBehaviorProducer(brokers []string) *BehaviorProducer {
    writer := &kafka.Writer{
        Addr:         kafka.TCP(brokers...),
        Topic:        "user_behavior",
        Balancer:     &kafka.Hash{}, // 按 UserID 分区
        RequiredAcks: kafka.RequireOne,
        Compression:  kafka.Snappy,
        BatchSize:    100,           // 批量发送
        BatchTimeout: 10 * time.Millisecond,
    }
    
    return &BehaviorProducer{writer: writer}
}

func (p *BehaviorProducer) PublishBehavior(ctx context.Context, event *UserBehaviorEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    return p.writer.WriteMessages(ctx, kafka.Message{
        Key:   []byte(fmt.Sprintf("%d", event.UserID)), // 同一用户到同一分区
        Value: data,
    })
}
```

#### Consumer 实现

```go
// 文件: pkg/kafka/behavior_consumer.go
package kafka

import (
    "context"
    "encoding/json"
    "github.com/segmentio/kafka-go"
)

type BehaviorConsumer struct {
    reader  *kafka.Reader
    handler BehaviorHandler
}

type BehaviorHandler interface {
    HandleBehavior(ctx context.Context, event *UserBehaviorEvent) error
}

func NewBehaviorConsumer(brokers []string, groupID string, handler BehaviorHandler) *BehaviorConsumer {
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:        brokers,
        Topic:          "user_behavior",
        GroupID:        groupID,
        MinBytes:       10e3,  // 10KB
        MaxBytes:       10e6,  // 10MB
        CommitInterval: time.Second,
        StartOffset:    kafka.LastOffset,
    })
    
    return &BehaviorConsumer{
        reader:  reader,
        handler: handler,
    }
}

func (c *BehaviorConsumer) Start(ctx context.Context) error {
    for {
        msg, err := c.reader.ReadMessage(ctx)
        if err != nil {
            return err
        }
        
        var event UserBehaviorEvent
        if err := json.Unmarshal(msg.Value, &event); err != nil {
            continue
        }
        
        if err := c.handler.HandleBehavior(ctx, &event); err != nil {
            // 记录错误，继续处理下一条
            log.Error("Failed to handle behavior:", err)
        }
    }
}
```

---

### 场景 2: 推荐系统特征更新

#### Topic 设计

```yaml
Topics:
  - user_behavior:      # 用户行为原始数据
      partitions: 30
      replication: 3
      retention: 7 days
      
  - user_profile:       # 用户画像更新
      partitions: 10
      replication: 3
      retention: 30 days
      
  - video_feature:      # 视频特征更新
      partitions: 20
      replication: 3
      retention: 30 days
```

#### 实时计算 (Flink)

```java
// Flink 作业示例
StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();

// 从 Kafka 读取用户行为
FlinkKafkaConsumer<UserBehavior> behaviorSource = new FlinkKafkaConsumer<>(
    "user_behavior",
    new UserBehaviorSchema(),
    properties
);

DataStream<UserBehavior> behaviors = env.addSource(behaviorSource);

// 实时计算用户兴趣标签
DataStream<UserProfile> profiles = behaviors
    .keyBy(UserBehavior::getUserId)
    .window(TumblingEventTimeWindows.of(Time.hours(1)))
    .aggregate(new UserInterestAggregator());

// 写回 Kafka
profiles.addSink(new FlinkKafkaProducer<>("user_profile", ...));
```

---

## 📋 最佳实践建议

### RabbitMQ 使用场景 (保持当前架构)

✅ **适合**:
- 点赞/评论/关注等 **实时交互事件**
- 需要 **低延迟** (毫秒级) 响应
- 需要 **复杂路由** (Exchange)
- 需要 **消息确认** 和 **死信队列**
- **小批量、高优先级** 的业务

### Kafka 使用场景 (新增)

✅ **适合**:
- **用户行为日志** (PV/UV/观看记录)
- **数据分析** (实时统计/离线分析)
- **推荐系统** (特征更新/模型训练)
- **CDC** (数据库变更捕获)
- **大批量异步任务** (视频处理/批量推送)

### 混合架构设计

```
┌─────────────────────────────────────────────────────────┐
│                      业务场景                            │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  实时交互层 (RabbitMQ)                                   │
│  ├─ 点赞事件                                            │
│  ├─ 评论事件                                            │
│  ├─ 关注事件                                            │
│  └─ 实时通知                                            │
│                                                         │
│  数据分析层 (Kafka)                                      │
│  ├─ 用户行为日志                                         │
│  ├─ 推荐特征更新                                         │
│  ├─ 实时数据统计                                         │
│  └─ 视频处理任务                                         │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 🎯 实施路线图

### 阶段 1: 用户行为采集 (1-2周)

1. ✅ 启动 Kafka 服务 (已在 docker-compose.yml 中配置)
2. 📝 实现 Behavior Producer
3. 📝 客户端埋点 (视频播放、点击等)
4. 📝 实现基础 Consumer (日志存储)

### 阶段 2: 推荐系统集成 (2-3周)

1. 📝 设计用户画像更新流程
2. 📝 实现 Flink 实时计算作业
3. 📝 集成到推荐引擎

### 阶段 3: 实时数据分析 (2-3周)

1. 📝 搭建 Flink + Kafka 流处理平台
2. 📝 实现热门视频榜单计算
3. 📝 对接 Grafana 监控

### 阶段 4: 视频处理优化 (1-2周)

1. 📝 迁移视频处理任务到 Kafka
2. 📝 优化转码流程
3. 📝 提升处理吞吐量

---

## 📚 参考资源

- [Kafka vs RabbitMQ 选型对比](https://www.cloudamqp.com/blog/when-to-use-rabbitmq-or-apache-kafka.html)
- [Kafka 官方文档](https://kafka.apache.org/documentation/)
- [Flink + Kafka 最佳实践](https://flink.apache.org/features/2018/11/30/kafka-connectors.html)

---

**总结**: 
- RabbitMQ: 专注**实时事务型业务** (点赞、评论)
- Kafka: 专注**海量数据分析** (行为日志、推荐系统)
- 两者配合使用，发挥各自优势
