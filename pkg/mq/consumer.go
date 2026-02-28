package mq

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
}

// 接口定义已移至 interfaces.go 统一管理

func NewConsumer(rabbitmqURL string) (*Consumer, error) {
	conn, err := amqp091.Dial(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	// 设置QoS，限制未确认消息数量
	err = ch.Qos(
		10,    // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	consumer := &Consumer{
		conn:    conn,
		channel: ch,
	}

	return consumer, nil
}

func (c *Consumer) ConsumeLikeEvents(ctx context.Context, handler LikeEventHandler) error {
	msgs, err := c.channel.Consume(
		LikeEventQueue,
		"",    // consumer
		false, // auto-ack (设置为false，手动确认)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				hlog.Info("Like event consumer context cancelled")
				return
			case d, ok := <-msgs:
				if !ok {
					hlog.Info("Like event consumer channel closed")
					return
				}

				var event LikeEvent
				if err := json.Unmarshal(d.Body, &event); err != nil {
					hlog.Errorf("Failed to unmarshal like event: %v", err)
					d.Nack(false, false) // 拒绝消息，不重新入队
					continue
				}

				if err := handler.HandleLikeEvent(ctx, &event); err != nil {
					hlog.Errorf("Failed to handle like event: %v", err)
					d.Nack(false, true) // 拒绝消息，重新入队
					continue
				}

				d.Ack(false) // 确认消息
				hlog.CtxInfof(ctx, "Successfully processed like event: %+v", event)
			}
		}
	}()

	return nil
}

func (c *Consumer) ConsumeNotificationEvents(ctx context.Context, handler NotificationEventHandler) error {
	msgs, err := c.channel.Consume(
		NotificationEventQueue, // 通配队列 notification.# 接收所有类型
		"notification_all_consumer",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register notification consumer: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				hlog.Info("Notification event consumer context cancelled")
				return
			case d, ok := <-msgs:
				if !ok {
					hlog.Info("Notification event consumer channel closed")
					return
				}

				var event NotificationEvent
				if err := json.Unmarshal(d.Body, &event); err != nil {
					hlog.Errorf("Failed to unmarshal notification event: %v", err)
					d.Nack(false, false)
					continue
				}

				if err := handler.HandleNotificationEvent(ctx, &event); err != nil {
					hlog.Errorf("Failed to handle notification event: %v", err)
					d.Nack(false, true)
					continue
				}

				d.Ack(false)
				hlog.CtxInfof(ctx, "Processed notification event: type=%s, receiver=%d", event.Type, event.ReceiverID)
			}
		}
	}()

	return nil
}

// ConsumeNotificationEventsByType 按通知类型消费特定队列
// notificationType: video_like, comment, comment_like, comment_reply, follow, mention, system
func (c *Consumer) ConsumeNotificationEventsByType(ctx context.Context, notificationType string, handler NotificationEventHandler) error {
	queueMap := map[string]string{
		"video_like":    NotificationVideoLikeQueue,
		"comment":       NotificationCommentQueue,
		"comment_like":  NotificationCommentLikeQueue,
		"comment_reply": NotificationCommentReplyQueue,
		"follow":        NotificationFollowQueue,
		"mention":       NotificationMentionQueue,
		"system":        NotificationSystemQueue,
	}

	queue, ok := queueMap[notificationType]
	if !ok {
		return fmt.Errorf("unknown notification type: %s", notificationType)
	}

	msgs, err := c.channel.Consume(
		queue,
		"notification_"+notificationType+"_consumer",
		false, false, false, false, nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer for %s: %w", queue, err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				hlog.Infof("Notification %s consumer stopped", notificationType)
				return
			case d, ok := <-msgs:
				if !ok {
					hlog.Warnf("Notification %s consumer channel closed", notificationType)
					return
				}

				var event NotificationEvent
				if err := json.Unmarshal(d.Body, &event); err != nil {
					hlog.Errorf("Failed to unmarshal %s notification: %v", notificationType, err)
					d.Nack(false, false)
					continue
				}

				if err := handler.HandleNotificationEvent(ctx, &event); err != nil {
					hlog.Errorf("Failed to handle %s notification: %v", notificationType, err)
					d.Nack(false, true)
					continue
				}

				d.Ack(false)
			}
		}
	}()

	hlog.Infof("Notification consumer started for type: %s (queue: %s)", notificationType, queue)
	return nil
}

func (c *Consumer) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
