package common

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	redisClient "github.com/go-redis/redis"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EventDrivenSyncService 事件驱动同步服务
type EventDrivenSyncService struct {
	ctx          context.Context
	cancel       context.CancelFunc
	producer     mq.MessageProducer
	db           *gorm.DB
	retryManager *RetryManager
	lockManager  *DistributedLockManager
	cacheManager *redis.LikeCacheManager
	eventStore   *EventStore
	metrics      *SyncMetrics
	mu           sync.RWMutex
	isRunning    bool
}

// SyncEvent 同步事件结构
type SyncEvent struct {
	EventID        string                 `json:"event_id"`
	EventType      string                 `json:"event_type"`    // video_like, comment_like, video_sync, comment_sync
	ResourceType   string                 `json:"resource_type"` // video, comment
	ResourceID     int64                  `json:"resource_id"`
	UserID         int64                  `json:"user_id"`
	ActionType     string                 `json:"action_type"` // like, unlike, sync
	Data           map[string]interface{} `json:"data"`
	Timestamp      int64                  `json:"timestamp"`
	RetryCount     int                    `json:"retry_count"`
	MaxRetries     int                    `json:"max_retries"`
	Priority       int                    `json:"priority"` // 0-低, 1-中, 2-高
	IdempotencyKey string                 `json:"idempotency_key"`
}

// RetryManager 重试管理器
type RetryManager struct {
	maxRetries    int
	baseDelay     time.Duration
	maxDelay      time.Duration
	backoffFactor float64
}

// DistributedLockManager 分布式锁管理器
type DistributedLockManager struct {
	redis *redisClient.Client
}

// EventStore 事件存储
type EventStore struct {
	db *gorm.DB
}

// SyncMetrics 同步指标
type SyncMetrics struct {
	ProcessedEvents int64
	FailedEvents    int64
	RetryEvents     int64
	AverageLatency  time.Duration
	LastSyncTime    time.Time
	mu              sync.RWMutex
}

// NewEventDrivenSyncService 创建事件驱动同步服务
func NewEventDrivenSyncService(producer mq.MessageProducer, database *gorm.DB) *EventDrivenSyncService {
	ctx, cancel := context.WithCancel(context.Background())

	return &EventDrivenSyncService{
		ctx:      ctx,
		cancel:   cancel,
		producer: producer,
		db:       database,
		retryManager: &RetryManager{
			maxRetries:    3,
			baseDelay:     time.Second,
			maxDelay:      time.Minute,
			backoffFactor: 2.0,
		},
		lockManager: &DistributedLockManager{},
		eventStore:  &EventStore{db: database},
		metrics:     &SyncMetrics{},
	}
}

// Start 启动同步服务
func (s *EventDrivenSyncService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("sync service is already running")
	}

	s.isRunning = true
	hlog.Info("Starting event-driven sync service...")

	// 启动事件处理协程
	go s.processEvents()

	// 启动重试处理协程
	go s.processRetryEvents()

	// 启动指标收集协程
	go s.collectMetrics()

	hlog.Info("Event-driven sync service started successfully")
	return nil
}

// updateCommentLikeCount 更新评论表的点赞数
func (s *EventDrivenSyncService) updateCommentLikeCount(ctx context.Context, tx *gorm.DB, commentID int64, delta int64) error {
	// 使用原子操作更新点赞数，避免并发问题
	result := tx.Model(&model.Comment{}).
		Where("comment_id = ?", commentID).
		Update("like_count", gorm.Expr("like_count + ?", delta))

	if result.Error != nil {
		return fmt.Errorf("failed to update comment like count: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		hlog.CtxWarnf(ctx, "No comment found with ID %d for like count update", commentID)
	}

	hlog.CtxInfof(ctx, "Updated comment %d like count by %d", commentID, delta)
	return nil
}

// sendCommentLikeNotification 发送评论点赞通知
func (s *EventDrivenSyncService) sendCommentLikeNotification(ctx context.Context, userID, commentID int64) error {
	// 1. 获取评论信息以确定通知接收者
	var comment model.Comment
	if err := s.db.Where("comment_id = ?", commentID).First(&comment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			hlog.CtxWarnf(ctx, "Comment %d not found for like notification", commentID)
			return nil // 评论不存在，不发送通知
		}
		return fmt.Errorf("failed to get comment info: %w", err)
	}

	// 2. 不给自己发通知
	if comment.UserId == userID {
		return nil
	}

	// 3. 创建通知事件
	notificationEvent := &mq.NotificationEvent{
		Type:       "comment_like",
		ReceiverID: comment.UserId,
		SenderID:   userID,
		Content:    "点赞了你的评论",
		Extra: map[string]interface{}{
			"comment_id": commentID,
			"video_id":   comment.VideoId,
			"content":    comment.Content,
		},
		Timestamp: time.Now().Unix(),
		EventID:   uuid.New().String(),

		// 兼容字段
		UserID:           comment.UserId,
		FromUserID:       userID,
		NotificationType: "like",
		TargetID:         commentID,
	}

	// 4. 发送通知事件到消息队列
	if s.producer != nil {
		if err := s.producer.PublishNotificationEvent(ctx, notificationEvent); err != nil {
			return fmt.Errorf("failed to publish notification event: %w", err)
		}
		hlog.CtxInfof(ctx, "Sent comment like notification: user %d liked comment %d by user %d",
			userID, commentID, comment.UserId)
	}

	return nil
}

// Stop 停止同步服务
func (s *EventDrivenSyncService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	s.isRunning = false
	s.cancel()

	hlog.Info("Event-driven sync service stopped")
	return nil
}

// PublishSyncEvent 发布同步事件（直接处理，不再发布到MQ避免循环）
func (s *EventDrivenSyncService) PublishSyncEvent(ctx context.Context, event *SyncEvent) error {
	// 设置事件ID和时间戳
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = s.generateIdempotencyKey(event)
	}

	// 检查幂等性
	if exists, err := s.checkIdempotency(ctx, event.IdempotencyKey); err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	} else if exists {
		hlog.Warnf("Event already processed: %s", event.IdempotencyKey)
		return nil
	}

	// 存储事件到事件存储
	if err := s.eventStore.StoreEvent(ctx, event); err != nil {
		hlog.Errorf("Failed to store event: %v", err)
		// 不阻塞主流程，继续处理
	}

	// 直接处理事件，不再发布到MQ避免事件循环
	return s.processEvent(ctx, event)
}

// ProcessVideoLikeEvent 处理视频点赞事件
func (s *EventDrivenSyncService) ProcessVideoLikeEvent(ctx context.Context, event *SyncEvent) error {
	lockKey := fmt.Sprintf("video_like_lock:%d:%d", event.ResourceID, event.UserID)

	// 获取分布式锁
	if !s.lockManager.AcquireLock(ctx, lockKey, 30*time.Second) {
		return fmt.Errorf("failed to acquire lock for video like: %s", lockKey)
	}
	defer s.lockManager.ReleaseLock(ctx, lockKey)

	// 使用事务处理数据库操作
	return s.db.Transaction(func(tx *gorm.DB) error {
		switch event.ActionType {
		case "like":
			return s.processVideoLike(ctx, tx, event)
		case "unlike":
			return s.processVideoUnlike(ctx, tx, event)
		default:
			return fmt.Errorf("unknown action type: %s", event.ActionType)
		}
	})
}

// ProcessCommentLikeEvent 处理评论点赞事件
func (s *EventDrivenSyncService) ProcessCommentLikeEvent(ctx context.Context, event *SyncEvent) error {
	lockKey := fmt.Sprintf("comment_like_lock:%d:%d", event.ResourceID, event.UserID)

	if !s.lockManager.AcquireLock(ctx, lockKey, 30*time.Second) {
		return fmt.Errorf("failed to acquire lock for comment like: %s", lockKey)
	}
	defer s.lockManager.ReleaseLock(ctx, lockKey)

	return s.db.Transaction(func(tx *gorm.DB) error {
		switch event.ActionType {
		case "like":
			return s.processCommentLike(ctx, tx, event)
		case "unlike":
			return s.processCommentUnlike(ctx, tx, event)
		default:
			return fmt.Errorf("unknown action type: %s", event.ActionType)
		}
	})
}

// processVideoLike 处理视频点赞
func (s *EventDrivenSyncService) processVideoLike(ctx context.Context, tx *gorm.DB, event *SyncEvent) error {
	// 处理时间戳：如果时间戳为0或无效，使用当前时间
	var eventTime time.Time
	if event.Timestamp > 0 {
		eventTime = time.Unix(event.Timestamp, 0)
	} else {
		eventTime = time.Now()
		hlog.CtxWarnf(ctx, "Event timestamp is invalid (%d), using current time for event %s",
			event.Timestamp, event.EventID)
	}

	// 1. 创建用户行为记录
	behavior := &model.UserBehavior{
		UserId:       event.UserID,
		VideoId:      event.ResourceID,
		BehaviorType: "like",
		BehaviorTime: eventTime.Format("2006-01-02 15:04:05"),
	}

	if err := tx.Create(behavior).Error; err != nil {
		return fmt.Errorf("failed to create user behavior: %w", err)
	}

	// 2. 创建或更新video_likes记录
	uuid := uuid.New().ID()

	videoLike := &model.VideoLike{
		VideoLikesId: int64(uuid),
		UserId:       event.UserID,
		VideoId:      event.ResourceID,
		CreatedAt:    eventTime.Format("2006-01-02 15:04:05"),
		DeletedAt:    "", // 空字符串表示未删除
	}

	if err := tx.Create(videoLike).Error; err != nil {
		return fmt.Errorf("failed to create video like: %w", err)
	}

	// 3. 更新视频表的点赞数 - 关键缺失功能补充
	if err := s.updateVideoLikeCount(ctx, tx, event.ResourceID, 1); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update video like count: %v", err)
		// 不返回错误，避免影响主流程，但记录日志
	}

	// 4. 异步发送点赞通知 - 关键缺失功能补充
	go func() {
		if err := s.sendVideoLikeNotification(context.Background(), event.UserID, event.ResourceID); err != nil {
			hlog.Errorf("Failed to send video like notification: %v", err)
		}
	}()

	// 注意：不在这里更新Redis缓存，避免重复计数
	// Redis缓存已经在EventHandler中更新过了

	hlog.Infof("Successfully processed video like: user=%d, video=%d", event.UserID, event.ResourceID)
	return nil
}

// processVideoUnlike 处理视频取消点赞
func (s *EventDrivenSyncService) processVideoUnlike(ctx context.Context, tx *gorm.DB, event *SyncEvent) error {
	// 1. 删除用户行为记录
	if err := tx.Where("user_id = ? AND video_id = ? AND behavior_type = ?",
		event.UserID, event.ResourceID, "like").Delete(&model.UserBehavior{}).Error; err != nil {
		return fmt.Errorf("failed to delete user behavior: %w", err)
	}

	// 2. 删除video_likes记录
	if err := tx.Where("user_id = ? AND video_id = ?",
		event.UserID, event.ResourceID).Delete(&model.VideoLike{}).Error; err != nil {
		return fmt.Errorf("failed to delete video like: %w", err)
	}

	// 3. 更新视频表的点赞数 - 减少点赞数
	if err := s.updateVideoLikeCount(ctx, tx, event.ResourceID, -1); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update video like count: %v", err)
		// 不返回错误，避免影响主流程，但记录日志
	}

	// 注意：不在这里更新Redis缓存，避免重复计数
	// Redis缓存已经在EventHandler中更新过了

	hlog.Infof("Successfully processed video unlike: user=%d, video=%d", event.UserID, event.ResourceID)
	return nil
}

// processCommentLike 处理评论点赞
func (s *EventDrivenSyncService) processCommentLike(ctx context.Context, tx *gorm.DB, event *SyncEvent) error {
	// 创建评论点赞记录
	commentLike := &model.CommentLike{
		UserId:    event.UserID,
		CommentId: event.ResourceID,
		CreatedAt: time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05"),
	}

	if err := tx.Create(commentLike).Error; err != nil {
		return fmt.Errorf("failed to create comment like: %w", err)
	}

	// 更新评论表的点赞数 - 关键缺失功能补充
	if err := s.updateCommentLikeCount(ctx, tx, event.ResourceID, 1); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update comment like count: %v", err)
		// 不返回错误，避免影响主流程，但记录日志
	}

	// 异步发送评论点赞通知 - 关键缺失功能补充
	go func() {
		if err := s.sendCommentLikeNotification(context.Background(), event.UserID, event.ResourceID); err != nil {
			hlog.Errorf("Failed to send comment like notification: %v", err)
		}
	}()

	// 注意：不在这里更新Redis缓存，避免重复计数
	// Redis缓存已经在EventHandler中更新过了

	return nil
}

// processCommentUnlike 处理评论取消点赞
func (s *EventDrivenSyncService) processCommentUnlike(ctx context.Context, tx *gorm.DB, event *SyncEvent) error {
	// 删除评论点赞记录
	if err := tx.Where("user_id = ? AND comment_id = ?",
		event.UserID, event.ResourceID).Delete(&model.CommentLike{}).Error; err != nil {
		return fmt.Errorf("failed to delete comment like: %w", err)
	}

	// 更新评论表的点赞数 - 减少点赞数
	if err := s.updateCommentLikeCount(ctx, tx, event.ResourceID, -1); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update comment like count: %v", err)
		// 不返回错误，避免影响主流程，但记录日志
	}

	// 注意：不在这里更新Redis缓存，避免重复计数
	// Redis缓存已经在EventHandler中更新过了

	return nil
}

// processEvents 处理事件的主循环
func (s *EventDrivenSyncService) processEvents() {
	// 创建一个定时器来处理待处理的同步事件
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			hlog.Info("Event processing stopped")
			return
		case <-ticker.C:
			// 定期检查并处理待处理的同步事件
			s.processPendingEvents()
		}
	}
}

// processPendingEvents 处理待处理的同步事件
func (s *EventDrivenSyncService) processPendingEvents() {
	// 从事件存储中获取待处理的事件
	pendingEvents, err := s.eventStore.GetPendingEvents(s.ctx, 50)
	if err != nil {
		hlog.Errorf("Failed to get pending events: %v", err)
		return
	}

	if len(pendingEvents) == 0 {
		return
	}

	hlog.Infof("Processing %d pending sync events", len(pendingEvents))

	for _, event := range pendingEvents {
		// 更新事件状态为处理中
		if err := s.eventStore.UpdateEventStatus(s.ctx, event.EventID, "processing"); err != nil {
			hlog.Errorf("Failed to update event status: %v", err)
			continue
		}

		// 处理事件
		if err := s.processEvent(s.ctx, event); err != nil {
			hlog.Errorf("Failed to process event %s: %v", event.EventID, err)
			s.eventStore.UpdateEventStatus(s.ctx, event.EventID, "failed")
			s.metrics.mu.Lock()
			s.metrics.FailedEvents++
			s.metrics.mu.Unlock()
		} else {
			s.eventStore.UpdateEventStatus(s.ctx, event.EventID, "completed")
			hlog.Infof("Successfully processed sync event: %s", event.EventID)
		}
	}
}

// processRetryEvents 处理重试事件
func (s *EventDrivenSyncService) processRetryEvents() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			hlog.Info("Retry processing stopped")
			return
		case <-ticker.C:
			s.processFailedEvents()
		}
	}
}

// processFailedEvents 处理失败的事件
func (s *EventDrivenSyncService) processFailedEvents() {
	failedEvents, err := s.eventStore.GetFailedEvents(s.ctx, 100)
	if err != nil {
		hlog.Errorf("Failed to get failed events: %v", err)
		return
	}

	for _, event := range failedEvents {
		if event.RetryCount >= event.MaxRetries {
			hlog.Warnf("Event exceeded max retries: %s", event.EventID)
			continue
		}

		// 计算退避延迟
		delay := s.retryManager.calculateBackoffDelay(event.RetryCount)
		time.Sleep(delay)

		// 重试处理事件
		event.RetryCount++
		if err := s.processEvent(s.ctx, event); err != nil {
			hlog.Errorf("Failed to retry event %s: %v", event.EventID, err)
			s.eventStore.UpdateEventStatus(s.ctx, event.EventID, "failed")
		} else {
			s.eventStore.UpdateEventStatus(s.ctx, event.EventID, "completed")
		}
	}
}

// processEvent 处理单个事件
func (s *EventDrivenSyncService) processEvent(ctx context.Context, event *SyncEvent) error {
	start := time.Now()
	defer func() {
		s.metrics.mu.Lock()
		s.metrics.ProcessedEvents++
		s.metrics.AverageLatency = time.Since(start)
		s.metrics.LastSyncTime = time.Now()
		s.metrics.mu.Unlock()
	}()

	// 添加详细的调试日志
	hlog.CtxInfof(ctx, "Processing event: ID=%s, Type=%s, ResourceType=%s, ResourceID=%d, UserID=%d, ActionType=%s",
		event.EventID, event.EventType, event.ResourceType, event.ResourceID, event.UserID, event.ActionType)

	// 检查EventType是否为空
	if event.EventType == "" {
		hlog.CtxErrorf(ctx, "Event has empty EventType! Event details: %+v", event)
		return fmt.Errorf("event has empty EventType: event_id=%s", event.EventID)
	}

	switch event.EventType {
	case "video_like":
		return s.ProcessVideoLikeEvent(ctx, event)
	case "comment_like":
		return s.ProcessCommentLikeEvent(ctx, event)
	default:
		hlog.CtxErrorf(ctx, "Unknown event type '%s' for event %s. Event details: %+v",
			event.EventType, event.EventID, event)
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}
}

// collectMetrics 收集指标
func (s *EventDrivenSyncService) collectMetrics() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.logMetrics()
		}
	}
}

// logMetrics 记录指标
func (s *EventDrivenSyncService) logMetrics() {
	s.metrics.mu.RLock()
	defer s.metrics.mu.RUnlock()

	hlog.Infof("Sync Metrics - Processed: %d, Failed: %d, Retry: %d, Avg Latency: %v",
		s.metrics.ProcessedEvents,
		s.metrics.FailedEvents,
		s.metrics.RetryEvents,
		s.metrics.AverageLatency)
}

// 辅助方法
func (s *EventDrivenSyncService) generateIdempotencyKey(event *SyncEvent) string {
	return fmt.Sprintf("%s_%s_%d_%d_%d",
		event.EventType, event.ActionType, event.ResourceID, event.UserID, event.Timestamp)
}

func (s *EventDrivenSyncService) checkIdempotency(ctx context.Context, key string) (bool, error) {
	// 使用数据库检查幂等性键是否已存在
	var count int64
	err := s.db.WithContext(ctx).Model(&model.SyncEvent{}).
		Where("idempotency_key = ?", key).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check idempotency: %w", err)
	}

	return count > 0, nil
}

// publishToMQ 已弃用 - 避免事件循环
// 原来的逻辑会导致 LikeEvent -> SyncEvent -> LikeEvent 的循环处理
// publishToMQ 方法已删除，避免事件循环和重复计数

// RetryManager 方法
func (rm *RetryManager) calculateBackoffDelay(retryCount int) time.Duration {
	delay := time.Duration(float64(rm.baseDelay) * float64(retryCount) * rm.backoffFactor)
	if delay > rm.maxDelay {
		delay = rm.maxDelay
	}
	return delay
}

// DistributedLockManager 方法
func (dlm *DistributedLockManager) AcquireLock(ctx context.Context, key string, expiration time.Duration) bool {
	// 简化实现，实际应该使用Redis分布式锁
	return true
}

func (dlm *DistributedLockManager) ReleaseLock(ctx context.Context, key string) {
	// 释放锁的实现
}

// EventStore 方法
func (es *EventStore) StoreEvent(ctx context.Context, event *SyncEvent) error {
	// 验证必要字段
	if event.EventID == "" {
		return fmt.Errorf("event ID is required")
	}
	if event.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if event.UserID == 0 {
		return fmt.Errorf("user ID is required")
	}

	hlog.CtxInfof(ctx, "Storing event: ID=%s, Type=%s, ResourceType=%s, ResourceID=%d, UserID=%d, ActionType=%s",
		event.EventID, event.EventType, event.ResourceType, event.ResourceID, event.UserID, event.ActionType)

	// 将业务SyncEvent转换为数据库模型
	eventData, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	dbEvent := &model.SyncEvent{
		ID:             event.EventID,
		EventType:      event.EventType,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		UserID:         event.UserID,
		ActionType:     event.ActionType,
		Status:         "pending",
		Data:           string(eventData),
		RetryCount:     event.RetryCount,
		MaxRetries:     event.MaxRetries,
		Priority:       event.Priority,
		IdempotencyKey: event.IdempotencyKey,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := es.db.Create(dbEvent).Error; err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	hlog.CtxInfof(ctx, "Successfully stored event %s with type %s", event.EventID, event.EventType)
	return nil
}

func (es *EventStore) GetFailedEvents(ctx context.Context, limit int) ([]*SyncEvent, error) {
	// 获取失败的事件
	var records []struct {
		ID             string
		EventType      string
		ResourceType   string
		ResourceID     int64
		UserID         int64
		ActionType     string
		Status         string
		Data           string
		RetryCount     int
		MaxRetries     int
		Priority       int
		IdempotencyKey string
		CreatedAt      time.Time
		UpdatedAt      time.Time
	}

	err := es.db.Table("sync_events").
		Where("status = ?", "failed").
		Limit(limit).
		Find(&records).Error

	if err != nil {
		return nil, err
	}

	var events []*SyncEvent
	for _, record := range records {
		var dataMap map[string]interface{}
		if record.Data != "" {
			if err := json.Unmarshal([]byte(record.Data), &dataMap); err != nil {
				hlog.Warnf("Failed to unmarshal failed event data: %v", err)
				dataMap = make(map[string]interface{})
			}
		} else {
			dataMap = make(map[string]interface{})
		}

		// 从数据库记录重建SyncEvent
		event := &SyncEvent{
			EventID:        record.ID,
			EventType:      record.EventType,    // 从数据库字段设置
			ResourceType:   record.ResourceType, // 从数据库字段设置
			ResourceID:     record.ResourceID,   // 从数据库字段设置
			UserID:         record.UserID,       // 从数据库字段设置
			ActionType:     record.ActionType,   // 从数据库字段设置
			Data:           dataMap,
			RetryCount:     record.RetryCount,
			MaxRetries:     record.MaxRetries,
			Priority:       record.Priority,
			IdempotencyKey: record.IdempotencyKey,
		}

		// 从Data中获取Timestamp（如果存在）
		if timestamp, ok := dataMap["timestamp"]; ok {
			if ts, ok := timestamp.(float64); ok {
				event.Timestamp = int64(ts)
			}
		}

		events = append(events, event)
	}

	return events, nil
}

// GetPendingEvents 获取待处理的事件
func (es *EventStore) GetPendingEvents(ctx context.Context, limit int) ([]*SyncEvent, error) {
	var records []struct {
		ID             string
		EventType      string
		ResourceType   string
		ResourceID     int64
		UserID         int64
		ActionType     string
		Status         string
		Data           string
		RetryCount     int
		MaxRetries     int
		Priority       int
		IdempotencyKey string
		CreatedAt      time.Time
		UpdatedAt      time.Time
	}

	err := es.db.Table("sync_events").
		Where("status = ?", "pending").
		Order("created_at ASC").
		Limit(limit).
		Find(&records).Error

	if err != nil {
		return nil, err
	}

	var events []*SyncEvent
	for _, record := range records {
		var dataMap map[string]interface{}
		if record.Data != "" {
			if err := json.Unmarshal([]byte(record.Data), &dataMap); err != nil {
				hlog.Warnf("Failed to unmarshal event data: %v", err)
				// 如果JSON解析失败，使用空map
				dataMap = make(map[string]interface{})
			}
		} else {
			dataMap = make(map[string]interface{})
		}

		// 从数据库记录重建SyncEvent
		event := &SyncEvent{
			EventID:        record.ID,
			EventType:      record.EventType,    // 从数据库字段设置
			ResourceType:   record.ResourceType, // 从数据库字段设置
			ResourceID:     record.ResourceID,   // 从数据库字段设置
			UserID:         record.UserID,       // 从数据库字段设置
			ActionType:     record.ActionType,   // 从数据库字段设置
			Data:           dataMap,
			RetryCount:     record.RetryCount,
			MaxRetries:     record.MaxRetries,
			Priority:       record.Priority,
			IdempotencyKey: record.IdempotencyKey,
		}

		// 从Data中获取Timestamp（如果存在）
		if timestamp, ok := dataMap["timestamp"]; ok {
			if ts, ok := timestamp.(float64); ok {
				event.Timestamp = int64(ts)
			}
		}

		events = append(events, event)
	}

	return events, nil
}

func (es *EventStore) UpdateEventStatus(ctx context.Context, eventID, status string) error {
	return es.db.Table("sync_events").
		Where("id = ?", eventID).
		Update("status", status).
		Update("updated_at", time.Now()).Error
}

// updateVideoLikeCount 更新视频表的点赞数
func (s *EventDrivenSyncService) updateVideoLikeCount(ctx context.Context, tx *gorm.DB, videoID int64, delta int64) error {
	// 使用原子操作更新点赞数，避免并发问题
	result := tx.Model(&model.Video{}).
		Where("video_id = ?", videoID).
		Update("like_count", gorm.Expr("like_count + ?", delta))

	if result.Error != nil {
		return fmt.Errorf("failed to update video like count: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		hlog.CtxWarnf(ctx, "No video found with ID %d for like count update", videoID)
	}

	hlog.CtxInfof(ctx, "Updated video %d like count by %d", videoID, delta)
	return nil
}

// sendVideoLikeNotification 发送视频点赞通知
func (s *EventDrivenSyncService) sendVideoLikeNotification(ctx context.Context, userID, videoID int64) error {
	// 1. 获取视频信息以确定通知接收者
	var video model.Video
	if err := s.db.Where("video_id = ?", videoID).First(&video).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			hlog.CtxWarnf(ctx, "Video %d not found for like notification", videoID)
			return nil // 视频不存在，不发送通知
		}
		return fmt.Errorf("failed to get video info: %w", err)
	}

	// 2. 不给自己发通知
	if video.UserId == userID {
		return nil
	}

	// 3. 创建通知事件
	notificationEvent := &mq.NotificationEvent{
		Type:       "video_like",
		ReceiverID: video.UserId,
		SenderID:   userID,
		Content:    "点赞了你的视频",
		Extra: map[string]interface{}{
			"video_id": videoID,
			"title":    video.Title,
		},
		Timestamp: time.Now().Unix(),
		EventID:   uuid.New().String(),

		// 兼容字段
		UserID:           video.UserId,
		FromUserID:       userID,
		NotificationType: "like",
		TargetID:         videoID,
	}

	// 4. 发送通知事件到消息队列
	if s.producer != nil {
		if err := s.producer.PublishNotificationEvent(ctx, notificationEvent); err != nil {
			return fmt.Errorf("failed to publish notification event: %w", err)
		}
		hlog.CtxInfof(ctx, "Sent video like notification: user %d liked video %d by user %d",
			userID, videoID, video.UserId)
	}

	return nil
}
