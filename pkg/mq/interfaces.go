package mq

import "context"

// MessageProducer 消息生产者接口
type MessageProducer interface {
	PublishLikeEvent(ctx context.Context, event *LikeEvent) error
	PublishCommentEvent(ctx context.Context, event *CommentEvent) error
	PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error
}

// LikeEventHandler 点赞事件处理器接口
type LikeEventHandler interface {
	HandleLikeEvent(ctx context.Context, event *LikeEvent) error
}

// CommentEventHandler 评论事件处理器接口
type CommentEventHandler interface {
	HandleCommentEvent(ctx context.Context, event *CommentEvent) error
}

// NotificationEventHandler 通知事件处理器接口
type NotificationEventHandler interface {
	HandleNotificationEvent(ctx context.Context, event *NotificationEvent) error
}

// 确保Producer实现MessageProducer接口
var _ MessageProducer = (*Producer)(nil)

// 确保MQManager实现MessageProducer接口
var _ MessageProducer = (*MQManager)(nil)
