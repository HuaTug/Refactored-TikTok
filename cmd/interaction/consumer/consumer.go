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
	viper.AddConfigPath("../../../cmd/interaction/config")
	viper.AddConfigPath("../../cmd/interaction/config")

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
	db := db.DB

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

	// 创建统一的消息队列管理器
	mqManager, err := mq.NewMQManager(rabbitmqURL)
	if err != nil {
		log.Fatalf("Failed to create unified MQ manager: %v", err)
	}
	defer mqManager.Close()

	syncService := common.NewEventDrivenSyncService(mqManager, db)
	if err := syncService.Start(); err != nil {
		log.Fatalf("Failed to start sync service: %v", err)
	}
	hlog.Info("Event-driven sync service started")

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建带同步服务的事件处理器
	likeHandler := service.NewLikeEventHandlerWithSync(syncService)

	// 启动点赞事件消费者
	if err := mqManager.ConsumeLikeEvents(ctx, likeHandler); err != nil {
		log.Fatalf("Failed to start like event consumer: %v", err)
	}
	hlog.Info("Like event consumer started")

	// 启动评论事件消费者 (如果需要的话)
	// commentHandler := service.NewCommentEventHandler()
	// if err := mqManager.ConsumeCommentEvents(ctx, commentHandler); err != nil {
	// 	log.Fatalf("Failed to start comment event consumer: %v", err)
	// }
	// hlog.Info("Comment event consumer started")

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
