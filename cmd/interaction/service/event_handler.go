package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/common"
	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
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
type NotificationEventHandler struct{}

func NewNotificationEventHandler() *NotificationEventHandler {
	return &NotificationEventHandler{}
}

// HandleNotificationEvent 处理通知事件
func (h *NotificationEventHandler) HandleNotificationEvent(ctx context.Context, event *mq.NotificationEvent) error {
	hlog.CtxInfof(ctx, "Processing notification event: %+v", event)

	// 1. 将通知保存到数据库
	notification := &Notification{
		UserID:           event.UserID,
		FromUserID:       event.FromUserID,
		NotificationType: event.NotificationType,
		TargetID:         event.TargetID,
		Content:          event.Content,
		IsRead:           false,
		CreatedAt:        time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05"),
	}

	if err := db.CreateNotification(ctx, notification); err != nil {
		hlog.CtxErrorf(ctx, "Failed to save notification to database: %v", err)
		return err
	}

	// 2. 可选：推送实时通知到用户（WebSocket、推送服务等）
	// 这里可以集成推送服务，如APNs、FCM等
	h.pushRealTimeNotification(ctx, notification)

	return nil
}

// 推送实时通知（简化版，实际项目中需要集成推送服务）
func (h *NotificationEventHandler) pushRealTimeNotification(ctx context.Context, notification *Notification) {
	// TODO: 集成WebSocket或其他推送服务
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
