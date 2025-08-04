package mq

import "context"

// MessageProducer 消息生产者接口
type MessageProducer interface {
	PublishLikeEvent(ctx context.Context, event *LikeEvent) error
	PublishCommentEvent(ctx context.Context, event *CommentEvent) error
	PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error
}

// 确保Producer实现MessageProducer接口
var _ MessageProducer = (*Producer)(nil)

// 确保UnifiedMQManager实现MessageProducer接口
var _ MessageProducer = (*UnifiedMQManager)(nil)
