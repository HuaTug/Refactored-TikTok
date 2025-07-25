package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func main() {
	// 初始化日志
	hlog.SetLevel(hlog.LevelInfo)

	// RabbitMQ连接URL，可以从配置文件或环境变量读取
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	// 创建消费者
	consumer, err := mq.NewConsumer(rabbitmqURL)
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建事件处理器
	likeHandler := service.NewLikeEventHandler()
	notificationHandler := service.NewNotificationEventHandler()

	// 启动点赞事件消费者
	if err := consumer.ConsumeLikeEvents(ctx, likeHandler); err != nil {
		log.Fatalf("Failed to start like event consumer: %v", err)
	}
	hlog.Info("Like event consumer started")

	// 启动通知事件消费者
	if err := consumer.ConsumeNotificationEvents(ctx, notificationHandler); err != nil {
		log.Fatalf("Failed to start notification event consumer: %v", err)
	}
	hlog.Info("Notification event consumer started")

	hlog.Info("Event consumer started successfully, waiting for messages...")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	hlog.Info("Shutting down event consumer...")

	// 优雅关闭
	cancel()
	time.Sleep(2 * time.Second) // 给消费者一些时间来处理正在进行的消息

	hlog.Info("Event consumer stopped")
}
