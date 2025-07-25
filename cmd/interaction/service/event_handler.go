package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// LikeEventHandler 处理点赞事件
type LikeEventHandler struct{}

func NewLikeEventHandler() *LikeEventHandler {
	return &LikeEventHandler{}
}

// HandleLikeEvent 处理点赞事件
func (h *LikeEventHandler) HandleLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	hlog.CtxInfof(ctx, "Processing like event: %+v", event)

	if event.EventType == "video_like" {
		return h.handleVideoLikeEvent(ctx, event)
	} else if event.EventType == "comment_like" {
		return h.handleCommentLikeEvent(ctx, event)
	}

	return fmt.Errorf("unknown event type: %s", event.EventType)
}

// 处理视频点赞事件
func (h *LikeEventHandler) handleVideoLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	// 1. 更新Redis中的计数器
	var err error
	if event.ActionType == "like" {
		// 增加点赞数
		err = redis.IncrVideoLikeCount(event.VideoID)
	} else if event.ActionType == "unlike" {
		// 减少点赞数
		err = redis.DecrVideoLikeCount(event.VideoID)
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to update video like count in Redis: %v", err)
		return err
	}

	// 2. 可选：批量写回到主数据库（这里简化，直接更新）
	// 在生产环境中，可能会使用定时任务来批量同步Redis数据到数据库

	return nil
}

// 处理评论点赞事件
func (h *LikeEventHandler) handleCommentLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	// 1. 更新Redis中的评论点赞计数器
	var err error
	if event.ActionType == "like" {
		err = redis.IncrCommentLikeCount(event.CommentID)
	} else if event.ActionType == "unlike" {
		err = redis.DecrCommentLikeCount(event.CommentID)
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to update comment like count in Redis: %v", err)
		return err
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
