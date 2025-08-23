package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/rabbitmq/amqp091-go"
)

// MQManager 统一的消息队列管理器
type MQManager struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel

	// 生产者功能
	producer *Producer

	// 消费者功能
	consumer *Consumer
}

// NewMQManager 创建统一的消息队列管理器
func NewMQManager(rabbitmqURL string) (*MQManager, error) {
	// 创建生产者
	producer, err := NewProducer(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	// 创建消费者
	consumer, err := NewConsumer(rabbitmqURL)
	if err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &MQManager{
		conn:     producer.conn,
		channel:  producer.channel,
		producer: producer,
		consumer: consumer,
	}, nil
}

// ========== 生产者方法 ==========

// PublishLikeEvent 发布点赞事件
func (umm *MQManager) PublishLikeEvent(ctx context.Context, event *LikeEvent) error {
	return umm.producer.PublishLikeEvent(ctx, event)
}

// PublishCommentEvent 发布评论事件
func (umm *MQManager) PublishCommentEvent(ctx context.Context, event *CommentEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal comment event: %w", err)
	}

	err = umm.channel.PublishWithContext(
		ctx,
		CommentEventExchange,
		"",
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Timestamp:    time.Now(),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish comment event: %w", err)
	}

	hlog.CtxInfof(ctx, "Published comment event: %+v", event)
	return nil
}

// PublishNotificationEvent 发布通知事件
func (umm *MQManager) PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	return umm.producer.PublishNotificationEvent(ctx, event)
}

// ========== 消费者方法 ==========

// ConsumeLikeEvents 消费点赞事件
func (umm *MQManager) ConsumeLikeEvents(ctx context.Context, handler LikeEventHandler) error {
	return umm.consumer.ConsumeLikeEvents(ctx, handler)
}

// ConsumeCommentEvents 消费评论事件
func (umm *MQManager) ConsumeCommentEvents(ctx context.Context, handler CommentEventHandler) error {
	msgs, err := umm.channel.Consume(
		CommentEventQueue,
		"comment_consumer",
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register comment consumer: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				hlog.Info("Comment event consumer stopped")
				return
			case msg, ok := <-msgs:
				if !ok {
					hlog.Warn("Comment event channel closed")
					return
				}

				var event CommentEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					hlog.Errorf("Failed to unmarshal comment event: %v", err)
					msg.Nack(false, false)
					continue
				}

				if err := handler.HandleCommentEvent(ctx, &event); err != nil {
					hlog.Errorf("Failed to handle comment event: %v", err)
					msg.Nack(false, true)
				} else {
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

// ConsumeNotificationEvents 消费通知事件
func (umm *MQManager) ConsumeNotificationEvents(ctx context.Context, handler NotificationEventHandler) error {
	return umm.consumer.ConsumeNotificationEvents(ctx, handler)
}

// ========== 接口定义 ==========

// 接口定义已移至 interfaces.go 统一管理

// ========== 管理方法 ==========

// HealthCheck 健康检查
func (umm *MQManager) HealthCheck() error {
	if umm.conn == nil || umm.conn.IsClosed() {
		return fmt.Errorf("RabbitMQ connection is closed")
	}

	if umm.channel == nil {
		return fmt.Errorf("RabbitMQ channel is nil")
	}

	// 尝试声明一个临时队列来测试连接
	_, err := umm.channel.QueueDeclare(
		"",    // name (empty for auto-generated)
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)

	return err
}

// Close 关闭连接
func (umm *MQManager) Close() error {
	var errs []error

	if umm.producer != nil {
		if err := umm.producer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if umm.consumer != nil {
		if err := umm.consumer.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing MQ manager: %v", errs)
	}

	return nil
}
