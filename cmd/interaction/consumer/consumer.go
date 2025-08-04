package consumer

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"HuaTug.com/cmd/interaction/common"
	"HuaTug.com/cmd/interaction/dal"
	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/config"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func initConfig() {
	// 获取当前工作目录用于调试
	wd, _ := os.Getwd()
	logrus.Infof("Current working directory: %s", wd)

	viper.SetConfigType("yaml")
	viper.SetConfigName("config.yml")

	// 为消费者服务添加正确的配置文件路径
	viper.AddConfigPath("../../../config")
	viper.AddConfigPath("../../config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	// 尝试读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		logrus.Warnf("Failed to read config file: %v", err)
		// 如果读取失败，尝试从环境变量获取配置
		viper.AutomaticEnv()
	} else {
		configFile := viper.ConfigFileUsed()
		absPath, _ := filepath.Abs(configFile)
		logrus.Infof("Using config file: %s", absPath)
	}

	// 初始化全局配置
	config.Init()
}

func Init() {
	// 初始化日志
	hlog.SetLevel(hlog.LevelInfo)

	// 初始化配置和依赖
	initConfig()
	dal.Init()
	redis.Load()
	hlog.Info("Dependencies initialized successfully")

	// RabbitMQ连接URL，可以从配置文件或环境变量读取
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	// 创建消息队列生产者（用于EventDrivenSyncService）
	producer, err := mq.NewProducer(rabbitmqURL)
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}

	// 创建事件驱动同步服务
	syncService := common.NewEventDrivenSyncService(producer, db.DB)
	if err := syncService.Start(); err != nil {
		log.Fatalf("Failed to start sync service: %v", err)
	}
	hlog.Info("Event-driven sync service started")

	// 创建消费者
	consumer, err := mq.NewConsumer(rabbitmqURL)
	if err != nil {
		log.Fatalf("Failed to create consumer: %v", err)
	}
	defer consumer.Close()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建带同步服务的事件处理器
	likeHandler := service.NewLikeEventHandlerWithSync(syncService)
	//notificationHandler := service.NewNotificationEventHandler()

	// 创建评论事件消费者和处理器
	commentMQManager, err := mq.NewCommentMQManager(rabbitmqURL)
	if err != nil {
		log.Fatalf("Failed to create comment MQ manager: %v", err)
	}
	defer commentMQManager.Close()

	// 这里需要创建评论事件处理器，但目前缺少依赖
	// commentEventProcessor := service.NewCommentEventProcessor(shardedDB, cacheManager, commentMQManager)

	// 启动点赞事件消费者
	if err := consumer.ConsumeLikeEvents(ctx, likeHandler); err != nil {
		log.Fatalf("Failed to start like event consumer: %v", err)
	}
	hlog.Info("Like event consumer started")

	// 启动评论事件消费者 (暂时注释，等待依赖完善)
	// if err := commentMQManager.ConsumeCommentEvents(ctx, commentEventProcessor); err != nil {
	// 	log.Fatalf("Failed to start comment event consumer: %v", err)
	// }
	// hlog.Info("Comment event consumer started")

	// // 启动通知事件消费者
	// if err := consumer.ConsumeNotificationEvents(ctx, notificationHandler); err != nil {
	// 	log.Fatalf("Failed to start notification event consumer: %v", err)
	// }
	// hlog.Info("Notification event consumer started")

	hlog.Info("Event consumer started successfully, waiting for messages...")

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	hlog.Info("Shutting down event consumer...")

	// 优雅关闭
	cancel()

	// 停止同步服务
	if err := syncService.Stop(); err != nil {
		hlog.Errorf("Failed to stop sync service: %v", err)
	}

	time.Sleep(2 * time.Second) // 给消费者一些时间来处理正在进行的消息

	hlog.Info("Event consumer stopped")
}
