package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// LikeEventHandler 处理点赞事件
type LikeEventHandler struct {
	syncService *EventDrivenSyncService
}

func NewLikeEventHandler() *LikeEventHandler {
	return &LikeEventHandler{}
}

// NewLikeEventHandlerWithSync 创建带同步服务的事件处理器
func NewLikeEventHandlerWithSync(syncService *EventDrivenSyncService) *LikeEventHandler {
	return &LikeEventHandler{
		syncService: syncService,
	}
}

// HandleLikeEvent 处理点赞事件
func (h *LikeEventHandler) HandleLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	hlog.CtxInfof(ctx, "Processing like event: %+v", event)

	// 1. 先处理原有的Redis更新逻辑（保持向后兼容）
	var err error
	if event.EventType == "video_like" {
		err = h.handleVideoLikeEvent(ctx, event)
	} else if event.EventType == "comment_like" {
		err = h.handleCommentLikeEvent(ctx, event)
	} else {
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like event: %v", err)
		return err
	}

	// 2. 如果配置了同步服务，则同步数据到video_likes表
	if h.syncService != nil {
		syncEvent := &SyncEvent{
			EventID:      event.EventID,
			EventType:    event.EventType,
			ResourceType: getResourceType(event.EventType),
			ResourceID:   getResourceID(event),
			UserID:       event.UserID,
			ActionType:   event.ActionType,
			Timestamp:    event.Timestamp,
			MaxRetries:   3,
			Priority:     1, // 中等优先级
		}

		if err := h.syncService.PublishSyncEvent(ctx, syncEvent); err != nil {
			hlog.CtxWarnf(ctx, "Failed to publish sync event: %v", err)
			// 不返回错误，避免影响主流程
		} else {
			hlog.CtxInfof(ctx, "Successfully published sync event for %s", event.EventType)
		}
	}

	return nil
}

// 获取资源类型
func getResourceType(eventType string) string {
	switch eventType {
	case "video_like":
		return "video"
	case "comment_like":
		return "comment"
	default:
		return "unknown"
	}
}

// 获取资源ID
func getResourceID(event *mq.LikeEvent) int64 {
	if event.VideoID != 0 {
		return event.VideoID
	}
	return event.CommentID
}

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

	// 2. 异步更新user_behaviors表（保持原有逻辑）
	go func() {
		if event.ActionType == "like" {
			like := &model.UserBehavior{
				UserId:       event.UserID,
				VideoId:      event.VideoID,
				BehaviorType: "like",
				BehaviorTime: time.Now().Format(constants.DataFormate),
			}
			if err := db.AddUserLikeBehavior(context.Background(), like); err != nil {
				hlog.Errorf("Failed to save like behavior to DB: %v", err)
			}
		} else if event.ActionType == "unlike" {
			if err := db.DeleteUserLikeBehavior(context.Background(), event.UserID, event.VideoID, "like"); err != nil {
				hlog.Errorf("Failed to delete like behavior from DB: %v", err)
			}
		}
	}()

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
