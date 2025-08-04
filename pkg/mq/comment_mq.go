package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/rabbitmq/amqp091-go"
)

// 事件类型结构体已移至 events.go 统一管理

// CommentMQManager 评论消息队列管理器
type CommentMQManager struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel

	// 队列配置
	commentQueue      string
	likeQueue         string
	notificationQueue string

	// 交换机配置
	commentExchange string

	// 生产者配置
	publishTimeout time.Duration
}

// NewCommentMQManager 创建评论消息队列管理器
func NewCommentMQManager(rabbitmqURL string) (*CommentMQManager, error) {
	conn, err := amqp091.Dial(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	manager := &CommentMQManager{
		conn:              conn,
		channel:           ch,
		commentQueue:      "comment_events",
		likeQueue:         "like_events",
		notificationQueue: "notification_events",
		commentExchange:   "comment_exchange",
		publishTimeout:    30 * time.Second,
	}

	// 初始化队列和交换机
	if err := manager.setupQueuesAndExchanges(); err != nil {
		manager.Close()
		return nil, fmt.Errorf("failed to setup queues and exchanges: %w", err)
	}

	return manager, nil
}

// setupQueuesAndExchanges 设置队列和交换机
func (cmm *CommentMQManager) setupQueuesAndExchanges() error {
	// 声明交换机
	err := cmm.channel.ExchangeDeclare(
		cmm.commentExchange, // name
		"topic",             // type
		true,                // durable
		false,               // auto-deleted
		false,               // internal
		false,               // no-wait
		nil,                 // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// 声明队列
	queues := []struct {
		name       string
		routingKey string
	}{
		{cmm.commentQueue, "comment.*"},
		{cmm.likeQueue, "like.*"},
		{cmm.notificationQueue, "notification.*"},
	}

	for _, q := range queues {
		// 声明队列
		_, err := cmm.channel.QueueDeclare(
			q.name, // name
			true,   // durable
			false,  // delete when unused
			false,  // exclusive
			false,  // no-wait
			amqp091.Table{
				"x-message-ttl":             int32(24 * 60 * 60 * 1000), // 24小时TTL
				"x-max-length":              int32(1000000),             // 最大消息数
				"x-dead-letter-exchange":    "dlx_exchange",             // 死信交换机
				"x-dead-letter-routing-key": "dlx." + q.name,            // 死信路由键
			},
		)
		if err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", q.name, err)
		}

		// 绑定队列到交换机
		err = cmm.channel.QueueBind(
			q.name,              // queue name
			q.routingKey,        // routing key
			cmm.commentExchange, // exchange
			false,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to bind queue %s: %w", q.name, err)
		}
	}

	return nil
}

// PublishCommentEvent 发布评论事件
func (cmm *CommentMQManager) PublishCommentEvent(ctx context.Context, event *CommentEvent) error {
	// 设置时间戳
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	// 序列化事件数据
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal comment event: %w", err)
	}

	// 根据事件类型确定路由键
	routingKey := fmt.Sprintf("comment.%s", event.Type)

	// 创建带超时的上下文
	publishCtx, cancel := context.WithTimeout(ctx, cmm.publishTimeout)
	defer cancel()

	// 发布消息
	err = cmm.channel.PublishWithContext(
		publishCtx,
		cmm.commentExchange, // exchange
		routingKey,          // routing key
		false,               // mandatory
		false,               // immediate
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp091.Persistent, // 持久化消息
			Priority:     0,
			Timestamp:    time.Now(),
			MessageId:    fmt.Sprintf("comment_%d_%d", event.VideoID, time.Now().UnixNano()),
			Headers: amqp091.Table{
				"video_id":   event.VideoID,
				"user_id":    event.UserID,
				"event_type": event.Type,
			},
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish comment event: %w", err)
	}

	hlog.Debugf("Published comment event: type=%s, video_id=%d, user_id=%d",
		event.Type, event.VideoID, event.UserID)
	return nil
}

// PublishLikeEvent 发布点赞事件
func (cmm *CommentMQManager) PublishLikeEvent(ctx context.Context, event *LikeEvent) error {
	// 设置时间戳
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal like event: %w", err)
	}

	routingKey := fmt.Sprintf("like.%s", event.ActionType)

	publishCtx, cancel := context.WithTimeout(ctx, cmm.publishTimeout)
	defer cancel()

	err = cmm.channel.PublishWithContext(
		publishCtx,
		cmm.commentExchange,
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp091.Persistent,
			Priority:     1, // 点赞事件优先级稍高
			Timestamp:    time.Now(),
			MessageId:    fmt.Sprintf("like_%d_%d_%d", event.CommentID, event.UserID, time.Now().UnixNano()),
			Headers: amqp091.Table{
				"comment_id": event.CommentID,
				"user_id":    event.UserID,
				"action":     event.ActionType,
			},
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish like event: %w", err)
	}

	return nil
}

// PublishNotificationEvent 发布通知事件
func (cmm *CommentMQManager) PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal notification event: %w", err)
	}

	routingKey := fmt.Sprintf("notification.%s", event.Type)

	publishCtx, cancel := context.WithTimeout(ctx, cmm.publishTimeout)
	defer cancel()

	err = cmm.channel.PublishWithContext(
		publishCtx,
		cmm.commentExchange,
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp091.Persistent,
			Priority:     2, // 通知事件优先级最高
			Timestamp:    time.Now(),
			MessageId:    fmt.Sprintf("notification_%d_%d", event.ReceiverID, time.Now().UnixNano()),
			Headers: amqp091.Table{
				"receiver_id": event.ReceiverID,
				"sender_id":   event.SenderID,
				"type":        event.Type,
			},
		},
	)

	if err != nil {
		return fmt.Errorf("failed to publish notification event: %w", err)
	}

	return nil
}

// BatchPublishCommentEvents 批量发布评论事件
func (cmm *CommentMQManager) BatchPublishCommentEvents(ctx context.Context, events []*CommentEvent) error {
	if len(events) == 0 {
		return nil
	}

	// 使用事务确保批量发布的原子性
	err := cmm.channel.Tx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// 发布所有事件
	for _, event := range events {
		if err := cmm.PublishCommentEvent(ctx, event); err != nil {
			// 回滚事务
			cmm.channel.TxRollback()
			return fmt.Errorf("failed to publish event in batch: %w", err)
		}
	}

	// 提交事务
	err = cmm.channel.TxCommit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	hlog.Infof("Batch published %d comment events", len(events))
	return nil
}

// ConsumeCommentEvents 消费评论事件
func (cmm *CommentMQManager) ConsumeCommentEvents(ctx context.Context, handler CommentEventHandler) error {
	msgs, err := cmm.channel.Consume(
		cmm.commentQueue,   // queue
		"comment_consumer", // consumer
		false,              // auto-ack
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // args
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

				// 处理消息
				if err := cmm.handleCommentMessage(ctx, msg, handler); err != nil {
					hlog.Errorf("Failed to handle comment message: %v", err)
					// 拒绝消息并重新入队
					msg.Nack(false, true)
				} else {
					// 确认消息
					msg.Ack(false)
				}
			}
		}
	}()

	return nil
}

// handleCommentMessage 处理评论消息
func (cmm *CommentMQManager) handleCommentMessage(ctx context.Context, msg amqp091.Delivery, handler CommentEventHandler) error {
	var event CommentEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal comment event: %w", err)
	}

	// 调用处理器
	return handler.HandleCommentEvent(ctx, &event)
}

// CommentEventHandler 接口已移至 unified_manager.go

// GetQueueInfo 获取队列信息
func (cmm *CommentMQManager) GetQueueInfo(queueName string) (amqp091.Queue, error) {
	return cmm.channel.QueueInspect(queueName)
}

// PurgeQueue 清空队列
func (cmm *CommentMQManager) PurgeQueue(queueName string) error {
	_, err := cmm.channel.QueuePurge(queueName, false)
	return err
}

// Close 关闭连接
func (cmm *CommentMQManager) Close() error {
	if cmm.channel != nil {
		if err := cmm.channel.Close(); err != nil {
			hlog.Errorf("Failed to close channel: %v", err)
		}
	}

	if cmm.conn != nil {
		if err := cmm.conn.Close(); err != nil {
			hlog.Errorf("Failed to close connection: %v", err)
		}
	}

	return nil
}

// HealthCheck 健康检查
func (cmm *CommentMQManager) HealthCheck() error {
	if cmm.conn == nil || cmm.conn.IsClosed() {
		return fmt.Errorf("RabbitMQ connection is closed")
	}

	if cmm.channel == nil {
		return fmt.Errorf("RabbitMQ channel is nil")
	}

	// 尝试声明一个临时队列来测试连接
	_, err := cmm.channel.QueueDeclare(
		"",    // name (empty for auto-generated)
		false, // durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)

	return err
}
