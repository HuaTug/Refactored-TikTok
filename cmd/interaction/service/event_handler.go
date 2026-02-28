package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	client "HuaTug.com/cmd/interaction/client_rpc"
	common "HuaTug.com/cmd/interaction/sync"
	"HuaTug.com/cmd/interaction/dal/db"
	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/infra/cache"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// Cache key constants for notifications.
const (
	notificationUserKeyFmt = "notification:user:%d"
	notificationUnreadFmt  = "notification:unread:%d"
	notificationExpireDays = 30
	unreadExpireDays       = 7
	maxNotificationsPerKey = 1000
)

// Notification is the DB model for user notifications.
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

// --- Like Event Handler ---

// LikeEventHandler processes like events from the message queue.
type LikeEventHandler struct {
	syncService  *common.EventDrivenSyncService
	cacheManager *redis.LikeCacheManager
}

// NewLikeEventHandler creates a basic like event handler.
func NewLikeEventHandler() *LikeEventHandler {
	return &LikeEventHandler{}
}

// NewLikeEventHandlerWithSync creates a like event handler with sync service.
func NewLikeEventHandlerWithSync(syncService *common.EventDrivenSyncService) *LikeEventHandler {
	return &LikeEventHandler{
		syncService:  syncService,
		cacheManager: redis.NewLikeCacheManager(redis.RedisDBInteraction),
	}
}

// HandleLikeEvent processes a like event: dispatches DB sync and triggers notifications.
func (h *LikeEventHandler) HandleLikeEvent(ctx context.Context, event *mq.LikeEvent) error {
	hlog.CtxInfof(ctx, "Processing like event: %+v", event)

	if h.syncService != nil && event.ActionType == "like" {
		// 构造同步事件，触发 DB 持久化 + 通知发送
		syncEvent := &common.SyncEvent{
			EventType:    event.EventType,
			ResourceType: "video",
			ResourceID:   event.VideoID,
			UserID:       event.UserID,
			ActionType:   event.ActionType,
			Timestamp:    event.Timestamp,
		}
		if event.EventType == "comment_like" {
			syncEvent.ResourceType = "comment"
			syncEvent.ResourceID = event.CommentID
		}

		if err := h.syncService.PublishSyncEvent(ctx, syncEvent); err != nil {
			hlog.CtxErrorf(ctx, "Failed to dispatch sync event for like: %v", err)
		}
	}

	switch event.EventType {
	case "video_like":
		hlog.CtxInfof(ctx, "Video like event: user_id=%d, video_id=%d, action=%s",
			event.UserID, event.VideoID, event.ActionType)
	case "comment_like":
		hlog.CtxInfof(ctx, "Comment like event: user_id=%d, comment_id=%d, action=%s",
			event.UserID, event.CommentID, event.ActionType)
	default:
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	return nil
}

// --- Notification Event Handler ---

// NotificationEventHandler processes notification events with optional aggregation.
type NotificationEventHandler struct {
	aggregator *NotificationAggregator
}

// NewNotificationEventHandler creates a notification event handler.
func NewNotificationEventHandler() *NotificationEventHandler {
	return &NotificationEventHandler{
		aggregator: NewNotificationAggregator(),
	}
}

// HandleNotificationEvent processes a notification event, with optional aggregation.
func (h *NotificationEventHandler) HandleNotificationEvent(ctx context.Context, event *mq.NotificationEvent) error {
	hlog.CtxInfof(ctx, "Processing notification event: type=%s, receiver=%d", event.Type, event.ReceiverID)

	if h.aggregator != nil && h.aggregator.ShouldAggregate(event.Type) {
		return h.handleAggregated(ctx, event)
	}

	return h.handleDirect(ctx, event)
}

// handleAggregated handles notifications that may need aggregation.
func (h *NotificationEventHandler) handleAggregated(ctx context.Context, event *mq.NotificationEvent) error {
	shouldSend, err := h.aggregator.AddToAggregation(ctx, event, DefaultAggregationConfig)
	if err != nil {
		hlog.CtxErrorf(ctx, "Aggregation failed, falling back to direct: %v", err)
		return h.handleDirect(ctx, event)
	}

	if shouldSend {
		agg, aggErr := h.aggregator.CreateAggregatedNotification(ctx, event, DefaultAggregationConfig)
		if aggErr != nil {
			hlog.CtxErrorf(ctx, "Failed to create aggregated notification: %v", aggErr)
			return h.handleDirect(ctx, event)
		}
		h.incrementUnreadCount(ctx, event.ReceiverID)
		hlog.CtxInfof(ctx, "Created aggregated notification: %s (count=%d)", agg.Content, agg.AggregatedCount)
		return nil
	}

	// First notification in aggregation window: send directly.
	return h.handleDirect(ctx, event)
}

// handleDirect sends a notification directly without aggregation.
func (h *NotificationEventHandler) handleDirect(ctx context.Context, event *mq.NotificationEvent) error {
	notificationID := uuid.New().String()
	createdAt := time.Unix(event.Timestamp, 0).Format("2006-01-02 15:04:05")

	notification := &Notification{
		UserID:           event.ReceiverID,
		FromUserID:       event.SenderID,
		NotificationType: event.Type,
		TargetID:         event.TargetID,
		Content:          event.Content,
		IsRead:           false,
		CreatedAt:        createdAt,
	}

	// Persist to DB.
	if err := db.CreateNotification(ctx, notification); err != nil {
		hlog.CtxErrorf(ctx, "Failed to save notification to DB: %v", err)
	}

	// Write to Redis cache.
	if err := h.writeToCache(ctx, event, notificationID, createdAt); err != nil {
		hlog.CtxErrorf(ctx, "Failed to write notification to cache: %v", err)
	}

	h.incrementUnreadCount(ctx, event.ReceiverID)

	hlog.CtxInfof(ctx, "Notification sent to user %d: type=%s", event.ReceiverID, event.Type)
	return nil
}

// writeToCache writes a notification to Redis sorted sets.
func (h *NotificationEventHandler) writeToCache(ctx context.Context, event *mq.NotificationEvent, notificationID, createdAt string) error {
	conn := cache.GetRedis()
	defer conn.Close()

	// 合并 extra 信息
	extra := event.Extra
	if extra == nil {
		extra = make(map[string]interface{})
	}

	// 优先使用 Extra 中生产端预填充的用户信息，缺失时通过 RPC 补充
	fromUserName, _ := extra["from_user_name"].(string)
	fromAvatarUrl, _ := extra["avatar_url"].(string)
	if (fromUserName == "" || fromAvatarUrl == "") && event.SenderID > 0 {
		userResp, uErr := client.GetUserInfo(ctx, &users.GetUserInfoRequest{UserId: event.SenderID})
		if uErr == nil && userResp != nil && userResp.User != nil {
			if fromUserName == "" {
				fromUserName = userResp.User.UserName
			}
			if fromAvatarUrl == "" {
				fromAvatarUrl = userResp.User.AvatarUrl
			}
		} else {
			hlog.CtxWarnf(ctx, "Failed to get sender info for notification: sender_id=%d, err=%v", event.SenderID, uErr)
		}
	}
	extra["avatar_url"] = fromAvatarUrl
	extra["from_user_name"] = fromUserName

	// 优先使用 Extra 中生产端预填充的视频信息，缺失时通过 RPC 补充
	videoCover, _ := extra["video_cover"].(string)
	videoTitle, _ := extra["title"].(string)
	if (videoCover == "" || videoTitle == "") {
		videoID := event.VideoID
		if videoID <= 0 {
			if vid, ok := extra["video_id"]; ok {
				switch v := vid.(type) {
				case float64:
					videoID = int64(v)
				case int64:
					videoID = v
				}
			}
		}
		if videoID > 0 {
			videoResp, vErr := client.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: videoID})
			if vErr == nil && videoResp != nil && videoResp.Items != nil {
				if videoCover == "" {
					videoCover = videoResp.Items.CoverUrl
				}
				if videoTitle == "" {
					videoTitle = videoResp.Items.Title
				}
			} else {
				hlog.CtxWarnf(ctx, "Failed to get video info for notification: video_id=%d, err=%v", videoID, vErr)
			}
		}
	}
	if videoCover != "" {
		extra["video_cover"] = videoCover
	}
	if videoTitle != "" {
		extra["title"] = videoTitle
	}

	item := map[string]interface{}{
		"id":           notificationID,
		"type":         event.Type,
		"title":        h.getTitle(event.Type),
		"content":      event.Content,
		"from_user_id": event.SenderID,
		"from_user":    fromUserName,
		"target_id":    event.TargetID,
		"target_type":  h.getTargetType(event.Type),
		"is_read":      false,
		"created_at":   createdAt,
		"extra":        extra,
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	score := float64(event.Timestamp)
	userKey := fmt.Sprintf(notificationUserKeyFmt, event.ReceiverID)

	if _, err := conn.Do("ZADD", userKey, score, string(data)); err != nil {
		return fmt.Errorf("failed to add notification to zset: %w", err)
	}
	if _, err := conn.Do("EXPIRE", userKey, notificationExpireDays*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set expire for notification key: %v", err)
	}

	// Also store in type-specific sorted set.
	typeKey := userKey + ":" + event.Type
	if _, err := conn.Do("ZADD", typeKey, score, string(data)); err != nil {
		hlog.CtxWarnf(ctx, "Failed to add notification to type zset: %v", err)
	}
	if _, err := conn.Do("EXPIRE", typeKey, notificationExpireDays*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set type key expire: %v", err)
	}

	// Trim to max size.
	if _, err := conn.Do("ZREMRANGEBYRANK", userKey, 0, -(maxNotificationsPerKey + 1)); err != nil {
		hlog.CtxWarnf(ctx, "Failed to trim notification list: %v", err)
	}

	return nil
}

// incrementUnreadCount increments the unread notification counter in Redis.
func (h *NotificationEventHandler) incrementUnreadCount(ctx context.Context, userID int64) {
	conn := cache.GetRedis()
	defer conn.Close()

	key := fmt.Sprintf(notificationUnreadFmt, userID)
	if _, err := conn.Do("INCR", key); err != nil {
		hlog.CtxWarnf(ctx, "Failed to increment unread count for user %d: %v", userID, err)
	}
	if _, err := conn.Do("EXPIRE", key, unreadExpireDays*24*3600); err != nil {
		hlog.CtxWarnf(ctx, "Failed to set unread count expire: %v", err)
	}
}

// getTitle returns a display title for a notification type.
func (h *NotificationEventHandler) getTitle(notificationType string) string {
	titles := map[string]string{
		"video_like":    "视频获得点赞",
		"comment_like":  "评论获得点赞",
		"comment_reply": "收到新回复",
		"comment":       "收到新评论",
		"follow":        "收到新关注",
		"mention":       "有人@了你",
		"system":        "系统通知",
	}
	if t, ok := titles[notificationType]; ok {
		return t
	}
	return "新通知"
}

// getTargetType returns the target entity type for a notification type.
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
