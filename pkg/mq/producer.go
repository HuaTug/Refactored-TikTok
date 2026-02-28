package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
}

func NewProducer(rabbitmqURL string) (*Producer, error) {
	conn, err := amqp091.Dial(rabbitmqURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	producer := &Producer{
		conn:    conn,
		channel: ch,
	}

	// 声明exchanges和queues
	if err := producer.setupTopology(); err != nil {
		producer.Close()
		return nil, fmt.Errorf("failed to setup topology: %w", err)
	}

	return producer, nil
}

func (p *Producer) setupTopology() error {
	// 声明 direct 类型交换机（点赞、评论）
	directExchanges := []string{
		LikeEventExchange,
		CommentEventExchange,
	}

	for _, exchange := range directExchanges {
		err := p.channel.ExchangeDeclare(
			exchange,
			"direct",
			true,  // durable
			false, // auto-delete
			false, // internal
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			return fmt.Errorf("failed to declare exchange %s: %w", exchange, err)
		}
	}

	// 声明 topic 类型交换机（通知）—— 支持按 routing key 路由到不同类型队列
	err := p.channel.ExchangeDeclare(
		NotificationEventExchange,
		"topic", // Topic 类型，支持通配符路由
		true,    // durable
		false,   // auto-delete
		false,   // internal
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare topic exchange %s: %w", NotificationEventExchange, err)
	}

	// 声明点赞和评论队列
	basicQueues := []string{
		LikeEventQueue,
		CommentEventQueue,
	}
	for _, queue := range basicQueues {
		_, err := p.channel.QueueDeclare(
			queue,
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", queue, err)
		}
	}

	// 声明通知全量队列（接收所有类型通知，绑定 notification.#）
	if _, err := p.channel.QueueDeclare(
		NotificationEventQueue,
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", NotificationEventQueue, err)
	}

	// 声明各通知类型独立队列
	typeQueues := map[string]string{
		NotificationVideoLikeQueue:    "notification.video_like",
		NotificationCommentQueue:      "notification.comment",
		NotificationCommentLikeQueue:  "notification.comment_like",
		NotificationCommentReplyQueue: "notification.comment_reply",
		NotificationFollowQueue:       "notification.follow",
		NotificationMentionQueue:      "notification.mention",
		NotificationSystemQueue:       "notification.system",
	}

	for queue, routingKey := range typeQueues {
		if _, err := p.channel.QueueDeclare(
			queue, true, false, false, false, nil,
		); err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", queue, err)
		}
		// 绑定类型队列到 topic exchange，精确匹配 routing key
		if err := p.channel.QueueBind(
			queue,
			routingKey,
			NotificationEventExchange,
			false, nil,
		); err != nil {
			return fmt.Errorf("failed to bind queue %s with key %s: %w", queue, routingKey, err)
		}
	}

	// 绑定全量队列，使用 # 通配符接收所有 notification.* 消息
	if err := p.channel.QueueBind(
		NotificationEventQueue,
		"notification.#",
		NotificationEventExchange,
		false, nil,
	); err != nil {
		return fmt.Errorf("failed to bind all-notification queue: %w", err)
	}

	// 绑定点赞和评论队列到 direct exchange
	basicBindings := map[string]string{
		LikeEventQueue:    LikeEventExchange,
		CommentEventQueue: CommentEventExchange,
	}
	for queue, exchange := range basicBindings {
		if err := p.channel.QueueBind(queue, "", exchange, false, nil); err != nil {
			return fmt.Errorf("failed to bind queue %s to exchange %s: %w", queue, exchange, err)
		}
	}

	hlog.Info("MQ topology setup completed: topic exchange for notifications with type-based routing")
	return nil
}

func (p *Producer) PublishLikeEvent(ctx context.Context, event *LikeEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal like event: %w", err)
	}

	err = p.channel.PublishWithContext(
		ctx,
		LikeEventExchange,
		"",
		false, // mandatory
		false, // immediate
		amqp091.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp091.Persistent,
			Timestamp:    time.Now(),
			Body:         body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish like event: %w", err)
	}

	hlog.CtxInfof(ctx, "Published like event: %+v", event)
	return nil
}

func (p *Producer) PublishCommentEvent(ctx context.Context, event *CommentEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal comment event: %w", err)
	}

	err = p.channel.PublishWithContext(
		ctx,
		CommentEventExchange,
		"",
		false, // mandatory
		false, // immediate
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

func (p *Producer) PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal notification event: %w", err)
	}

	// 根据事件类型生成 routing key: notification.video_like, notification.comment 等
	routingKey := NotificationRoutingKeyPrefix + event.Type

	err = p.channel.PublishWithContext(
		ctx,
		NotificationEventExchange,
		routingKey,
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
		return fmt.Errorf("failed to publish notification event: %w", err)
	}

	hlog.CtxInfof(ctx, "Published notification event [routing_key=%s]: type=%s, receiver=%d", routingKey, event.Type, event.ReceiverID)
	return nil
}

func (p *Producer) Close() error {
	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
