# 代码库修复报告 - 重复消息和MQ定义混乱问题

## 问题概述

1. **重复消息问题**：user_behaviour表出现同一条数据插入3遍
2. **MQ定义混乱**：存在重复的函数定义和多套MQ系统

## 修复内容

### 1. 解决重复消息问题

#### 1.1 问题根因
- 多个地方同时插入user_behaviors表：
  - `like_service.go` 中的异步保存
  - `event_handler.go` 中的异步更新  
  - `event_driven_sync.go` 中的事件处理

#### 1.2 修复方案
- **统一数据插入入口**：只通过EventDrivenSyncService处理user_behaviors表的插入
- **添加幂等性保证**：修改`AddUserLikeBehavior`函数，使用事务和唯一性检查
- **弃用重复方法**：将`like_service.go`和`event_handler.go`中的数据库操作标记为已弃用

#### 1.3 具体改动
```go
// 修改前：直接插入，可能重复
func AddUserLikeBehavior(ctx context.Context, behavior *model.UserBehavior) error {
    return DB.WithContext(ctx).Create(behavior).Error
}

// 修改后：幂等性保证，避免重复
func AddUserLikeBehavior(ctx context.Context, behavior *model.UserBehavior) error {
    return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        var existingBehavior model.UserBehavior
        err := tx.Where("user_id = ? AND video_id = ? AND behavior_type = ?", 
            behavior.UserId, behavior.VideoId, behavior.BehaviorType).
            First(&existingBehavior).Error
        
        if err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                return tx.Create(behavior).Error
            }
            return err
        }
        
        return tx.Model(&existingBehavior).
            Update("behavior_time", behavior.BehaviorTime).Error
    })
}
```

#### 1.4 数据库修复脚本
创建了 `/scripts/fix_duplicate_user_behaviors.sql` 用于：
- 清理现有重复数据
- 添加唯一性约束防止未来重复

### 2. 解决MQ定义混乱问题

#### 2.1 问题根因
- 存在两套MQ系统：`producer.go/consumer.go` 和 `comment_mq.go`
- 结构体定义重复：`LikeEvent`、`CommentEvent`、`NotificationEvent`
- 接口和常量定义分散

#### 2.2 修复方案
- **统一结构体定义**：创建`events.go`统一管理所有事件结构体
- **创建统一接口**：定义`MessageProducer`接口抽象MQ功能
- **重构MQ管理器**：创建`UnifiedMQManager`统一管理生产者和消费者
- **清理重复代码**：移除`comment_mq.go`中的重复定义

#### 2.3 新的文件结构
```
pkg/mq/
├── events.go           # 统一的事件结构体定义
├── interfaces.go       # MessageProducer接口定义
├── producer.go         # 生产者实现
├── consumer.go         # 消费者实现
├── unified_manager.go  # 统一MQ管理器
└── comment_mq.go       # 已清理重复定义（保留向后兼容）
```

#### 2.4 接口设计
```go
type MessageProducer interface {
    PublishLikeEvent(ctx context.Context, event *LikeEvent) error
    PublishCommentEvent(ctx context.Context, event *CommentEvent) error
    PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error
}
```

### 3. 更新消费者逻辑

#### 3.1 统一MQ管理
- 使用`UnifiedMQManager`替代分散的生产者/消费者
- 通过接口注入依赖，提高代码灵活性
- 保持向后兼容性

#### 3.2 消费者代码更新
```go
// 修改前：使用多个MQ管理器
producer, _ := mq.NewProducer(rabbitmqURL)
consumer, _ := mq.NewConsumer(rabbitmqURL)
commentMQ, _ := mq.NewCommentMQManager(rabbitmqURL)

// 修改后：使用统一管理器
mqManager, _ := mq.NewUnifiedMQManager(rabbitmqURL)
```

## 验证和测试

### 1. 数据库级验证
运行以下SQL检查重复数据是否已清理：
```sql
SELECT user_id, video_id, behavior_type, COUNT(*) as count
FROM user_behaviors 
GROUP BY user_id, video_id, behavior_type 
HAVING COUNT(*) > 1;
```

### 2. 功能测试
- 执行点赞操作，确认只产生一条user_behaviors记录
- 验证事件消费者能正常处理消息
- 检查Redis缓存和数据库数据一致性

### 3. 性能监控
- 监控MQ消息积压情况
- 检查数据库插入性能
- 验证错误重试机制

## 兼容性说明

1. **API兼容性**：所有现有API保持不变
2. **数据兼容性**：现有数据结构保持不变  
3. **消息兼容性**：消息格式保持向后兼容
4. **部署兼容性**：可以平滑升级，无需停机

## 后续优化建议

1. **监控告警**：添加重复数据检测的监控告警
2. **性能优化**：考虑批量处理user_behaviors表插入
3. **数据清理**：定期清理过期的同步事件记录
4. **架构优化**：考虑引入更轻量的消息队列方案

## 风险控制

1. **回滚方案**：保留原有代码分支，可快速回滚
2. **灰度发布**：建议分阶段发布，先在测试环境验证
3. **数据备份**：执行数据库修复前先备份user_behaviors表
4. **监控加强**：加强部署后的数据一致性监控

## 结论

本次修复解决了以下问题：
- ✅ 消除了user_behaviors表的重复插入问题
- ✅ 统一了MQ系统架构和代码定义
- ✅ 提高了代码的可维护性和可扩展性
- ✅ 保持了系统的向后兼容性

修复后的系统具有更好的数据一致性保证和更清晰的代码结构。
