package mq

import "HuaTug.com/internal/model"

// LikeEvent 点赞事件
type LikeEvent struct {
	UserID     int64  `json:"user_id"`     // 用户ID
	VideoID    int64  `json:"video_id"`    // 视频ID
	CommentID  int64  `json:"comment_id"`  // 评论ID
	ActionType string `json:"action_type"` // "like" or "unlike"
	EventType  string `json:"event_type"`  // "video_like" or "comment_like"
	Timestamp  int64  `json:"timestamp"`   // 时间戳
	EventID    string `json:"event_id"`    // 事件ID
}

// CommentEvent 评论事件
type CommentEvent struct {
	Type      string                 `json:"type"`            // create, update, delete, like, unlike
	Comment   *model.Comment         `json:"comment"`         // 评论数据
	UserID    int64                  `json:"user_id"`         // 操作用户ID
	VideoID   int64                  `json:"video_id"`        // 视频ID
	Timestamp int64                  `json:"timestamp"`       // 时间戳
	Extra     map[string]interface{} `json:"extra,omitempty"` // 额外数据
}

// NotificationEvent 通知事件
type NotificationEvent struct {
	UserID           int64                  `json:"user_id"`           // 接收者ID (兼容字段)
	FromUserID       int64                  `json:"from_user_id"`      // 发送者ID (兼容字段)
	Type             string                 `json:"type"`              // comment, like, reply
	ReceiverID       int64                  `json:"receiver_id"`       // 接收者ID
	SenderID         int64                  `json:"sender_id"`         // 发送者ID
	CommentID        int64                  `json:"comment_id"`        // 评论ID
	VideoID          int64                  `json:"video_id"`          // 视频ID
	Content          string                 `json:"content"`           // 通知内容
	Extra            map[string]interface{} `json:"extra,omitempty"`   // 额外数据
	Timestamp        int64                  `json:"timestamp"`         // 时间戳
	EventID          string                 `json:"event_id"`          // 事件ID
	NotificationType string                 `json:"notification_type"` // 通知类型 (兼容字段)
	TargetID         int64                  `json:"target_id"`         // 目标ID (兼容字段)
}

// 常量定义
const (
	// 交换机名称
	LikeEventExchange         = "like_events"
	CommentEventExchange      = "comment_events"
	NotificationEventExchange = "notification_events_topic" // Topic 类型 Exchange

	// 队列名称
	LikeEventQueue         = "like_event_queue"
	CommentEventQueue      = "comment_event_queue"
	NotificationEventQueue = "notification_event_queue" // 通配队列，接收所有通知

	// 通知类型队列 —— 按类型独立消费
	NotificationVideoLikeQueue   = "notification_queue.video_like"
	NotificationCommentQueue     = "notification_queue.comment"
	NotificationCommentLikeQueue = "notification_queue.comment_like"
	NotificationCommentReplyQueue = "notification_queue.comment_reply"
	NotificationFollowQueue      = "notification_queue.follow"
	NotificationMentionQueue     = "notification_queue.mention"
	NotificationSystemQueue      = "notification_queue.system"

	// Routing Key 前缀
	NotificationRoutingKeyPrefix = "notification."
)
