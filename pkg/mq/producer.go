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

// LikeEvent and NotificationEvent are defined in comment_mq.go

const (
	LikeEventExchange         = "like_events"
	NotificationEventExchange = "notification_events"
	LikeEventQueue            = "like_event_queue"
	NotificationEventQueue    = "notification_event_queue"
)

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
	// 声明交换机
	err := p.channel.ExchangeDeclare(
		LikeEventExchange,
		"direct",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare like event exchange: %w", err)
	}

	err = p.channel.ExchangeDeclare(
		NotificationEventExchange,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare notification event exchange: %w", err)
	}

	// 声明队列
	_, err = p.channel.QueueDeclare(
		LikeEventQueue,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare like event queue: %w", err)
	}

	_, err = p.channel.QueueDeclare(
		NotificationEventQueue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare notification event queue: %w", err)
	}

	// 绑定队列到交换机
	err = p.channel.QueueBind(
		LikeEventQueue,
		"",
		LikeEventExchange,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind like event queue: %w", err)
	}

	err = p.channel.QueueBind(
		NotificationEventQueue,
		"",
		NotificationEventExchange,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind notification event queue: %w", err)
	}

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

func (p *Producer) PublishNotificationEvent(ctx context.Context, event *NotificationEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal notification event: %w", err)
	}

	err = p.channel.PublishWithContext(
		ctx,
		NotificationEventExchange,
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
		return fmt.Errorf("failed to publish notification event: %w", err)
	}

	hlog.CtxInfof(ctx, "Published notification event: %+v", event)
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
