package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"HuaTug.com/cmd/interaction/common"
	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/config/cache"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// LikeEventHandler 处理点赞事件
type LikeEventHandler struct {
	syncService  *common.EventDrivenSyncService
	cacheManager *redis.LikeCacheManager
}

func NewLikeEventHandler() *LikeEventHandler {
	return &LikeEventHandler{}
}

// NewLikeEventHandlerWithSync 创建带同步服务的事件处理器
func NewLikeEventHandlerWithSync(syncService *common.EventDrivenSyncService) *LikeEventHandler {
	return &LikeEventHandler{
		syncService:  syncService,
		cacheManager: redis.NewLikeCacheManager(redis.RedisDBInteraction),
	}
}

// HandleLikeEvent 处理点赞事件
func (h *LikeEventHandler) HandleLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	hlog.CtxInfof(ctx, "Processing like event: %+v", event)

	// 注意：Redis 计数和数据库写入已经在 like_service.go 中直接处理
	// MQ 事件仅用于异步处理通知等附加逻辑

	if event.EventType == "video_like" {
		// 可以在这里发送点赞通知
		hlog.CtxInfof(ctx, "Video like event received, user_id: %d, video_id: %d, action: %s",
			event.UserID, event.VideoID, event.ActionType)
	} else if event.EventType == "comment_like" {
		hlog.CtxInfof(ctx, "Comment like event received, user_id: %d, comment_id: %d, action: %s",
			event.UserID, event.CommentID, event.ActionType)
	} else {
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	return nil
}

// NotificationEventHandler 处理通知事件
type NotificationEventHandler struct {
	aggregator *NotificationAggregator
}

func NewNotificationEventHandler() *NotificationEventHandler {
	return &NotificationEventHandler{
		aggregator: NewNotificationAggregator(),
	}
}

// HandleNotificationEvent 处理通知事件
func (h *NotificationEventHandler) HandleNotificationEvent(ctx context.Context, event *mq.NotificationEvent) error {
	hlog.CtxInfof(ctx, "Processing notification event: %+v", event)

	// 检查是否需要聚合
	if h.aggregator != nil && h.aggregator.ShouldAggregate(event.Type) {
		return h.handleAggregatedNotification(ctx, event)
	}

	// 不需要聚合的通知，直接处理
	return h.handleDirectNotification(ctx, event)
}

// handleAggregatedNotification 处理需要聚合的通知
func (h *NotificationEventHandler) handleAggregatedNotification(ctx context.Context, event *mq.NotificationEvent) error {
	// 尝试添加到聚合队列
	shouldSend, err := h.aggregator.AddToAggregation(ctx, event, DefaultAggregationConfig)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to add to aggregation: %v, falling back to direct notification", err)
		return h.handleDirectNotification(ctx, event)
	}

	if shouldSend {
		// 达到聚合阈值，创建聚合通知
		aggNotification, err := h.aggregator.CreateAggregatedNotification(ctx, event, DefaultAggregationConfig)
		if err != nil {
			hlog.CtxErrorf(ctx, "Failed to create aggregated notification: %v", err)
			return h.handleDirectNotification(ctx, event)
		}

		// 更新未读计数
		h.incrementUnreadCount(ctx, event.ReceiverID)

		hlog.CtxInfof(ctx, "Created aggregated notification: %s (count=%d)",
			aggNotification.Content, aggNotification.AggregatedCount)
		return nil
	}

	// 还未达到聚合阈值，但已加入聚合队列
	// 发送单条通知（首条通知）
	return h.handleDirectNotification(ctx, event)
}

// handleDirectNotification 处理直接发送的通知（不聚合）
func (h *NotificationEventHandler) handleDirectNotification(ctx context.Context, event *mq.NotificationEvent) error {
	hlog.CtxInfof(ctx, "Processing notification event: %+v", event)

	// 生成通知ID
	notificationID := uuid.New().String()
	createdAt := time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05")

	// 1. 将通知保存到数据库
	notification := &Notification{
		UserID:           event.ReceiverID, // 使用ReceiverID而不是UserID
		FromUserID:       event.SenderID,   // 使用SenderID而不是FromUserID
		NotificationType: event.Type,
		TargetID:         event.TargetID,
		Content:          event.Content,
		IsRead:           false,
		CreatedAt:        createdAt,
	}

	if err := db.CreateNotification(ctx, notification); err != nil {
		hlog.CtxErrorf(ctx, "Failed to save notification to database: %v", err)
		// 继续执行，不要因为DB错误影响缓存写入
	}

	// 2. 将通知写入Redis缓存（供前端API读取）
	if err := h.writeNotificationToCache(ctx, event, notificationID, createdAt); err != nil {
		hlog.CtxErrorf(ctx, "Failed to write notification to cache: %v", err)
	}

	// 3. 更新未读计数
	h.incrementUnreadCount(ctx, event.ReceiverID)

	// 4. 可选：推送实时通知到用户（WebSocket、推送服务等）
	h.pushRealTimeNotification(ctx, notification)

	return nil
}

// writeNotificationToCache 将通知写入Redis缓存
func (h *NotificationEventHandler) writeNotificationToCache(ctx context.Context, event *mq.NotificationEvent, notificationID, createdAt string) error {
	conn := cache.GetRedis()
	defer conn.Close()

	// 构建通知项
	notificationItem := map[string]interface{}{
		"id":           notificationID,
		"type":         event.Type,
		"title":        h.getNotificationTitle(event.Type),
		"content":      event.Content,
		"from_user_id": event.SenderID,
		"from_user":    "", // 可以从用户服务获取用户名
		"target_id":    event.TargetID,
		"target_type":  h.getTargetType(event.Type),
		"is_read":      false,
		"created_at":   createdAt,
		"extra":        event.Extra,
	}

	data, err := json.Marshal(notificationItem)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// 写入用户的通知列表（ZSet，按时间戳排序）
	userKey := "notification:user:" + strconv.FormatInt(event.ReceiverID, 10)
	score := float64(event.Timestamp)

	if _, err := conn.Do("ZADD", userKey, score, string(data)); err != nil {
		return fmt.Errorf("failed to add notification to zset: %w", err)
	}

	// 设置过期时间（30天）
	if _, err := conn.Do("EXPIRE", userKey, 30*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set expire for notification key: %v", err)
	}

	// 同时写入按类型分类的列表
	typeKey := userKey + ":" + event.Type
	if _, err := conn.Do("ZADD", typeKey, score, string(data)); err != nil {
		hlog.CtxWarnf(ctx, "Failed to add notification to type-specific zset: %v", err)
	}
	if _, err := conn.Do("EXPIRE", typeKey, 30*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set expire for type-specific key: %v", err)
	}

	// 限制每个列表最多保留1000条通知
	if _, err := conn.Do("ZREMRANGEBYRANK", userKey, 0, -1001); err != nil {
		hlog.CtxWarnf(ctx, "Failed to trim notification list: %v", err)
	}

	hlog.CtxInfof(ctx, "Successfully wrote notification to cache for user %d", event.ReceiverID)
	return nil
}

// incrementUnreadCount 增加未读计数
func (h *NotificationEventHandler) incrementUnreadCount(ctx context.Context, userID int64) {
	conn := cache.GetRedis()
	defer conn.Close()

	key := "notification:unread:" + strconv.FormatInt(userID, 10)
	if _, err := conn.Do("INCR", key); err != nil {
		hlog.CtxWarnf(ctx, "Failed to increment unread count for user %d: %v", userID, err)
	}
	// 设置过期时间（7天）
	if _, err := conn.Do("EXPIRE", key, 7*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set expire for unread count key: %v", err)
	}
}

// getNotificationTitle 获取通知标题
func (h *NotificationEventHandler) getNotificationTitle(notificationType string) string {
	titles := map[string]string{
		"video_like":    "视频获得点赞",
		"comment_like":  "评论获得点赞",
		"comment_reply": "收到新回复",
		"comment":       "收到新评论",
		"follow":        "收到新关注",
		"mention":       "有人@了你",
		"system":        "系统通知",
	}
	if title, ok := titles[notificationType]; ok {
		return title
	}
	return "新通知"
}

// getTargetType 获取目标类型
func (h *NotificationEventHandler) getTargetType(notificationType string) string {
	switch notificationType {
	case "video_like", "comment":
		return "video"
	case "comment_like", "comment_reply", "mention":
		return "comment"
	case "follow":
		return "user"
	default:
		return "unknown"
	}
}

// 推送实时通知（简化版，实际项目中需要集成推送服务）
func (h *NotificationEventHandler) pushRealTimeNotification(ctx context.Context, notification *Notification) {
	// TODO: 集成WebSocket或其他推送服务
	// 可以选择的方案：
	// 1. WebSocket 长连接推送（适合Web端）
	// 2. FCM/APNs 推送（适合移动端）
	// 3. SSE (Server-Sent Events) 推送
	// 4. 第三方推送服务（极光、个推等）
	hlog.CtxInfof(ctx, "Would push real-time notification to user %d: %s",
		notification.UserID, notification.Content)
}

// Notification 通知数据模型
type Notification struct {
	NotificationID   int64  `gorm:"column:notification_id;primaryKey;autoIncrement"`
	UserID           int64  `gorm:"column:user_id"`
	FromUserID       int64  `gorm:"column:from_user_id"`
	NotificationType string `gorm:"column:notification_type"`
	TargetID         int64  `gorm:"column:target_id"`
	Content          string `gorm:"column:content"`
	IsRead           bool   `gorm:"column:is_read"`
	CreatedAt        string `gorm:"column:created_at"`
}
