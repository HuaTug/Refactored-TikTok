package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"HuaTug.com/cmd/interaction/dal"
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
	configPaths := []string{
		"../../../config", // 从consumer目录到根目录的config
		"../../config",    // 备用路径
		"./config",        // 当前目录的config
		"../config",       // 上级目录的config
		".",               // 当前目录
	}

	for _, path := range configPaths {
		viper.AddConfigPath(path)
		absPath, _ := filepath.Abs(path)
		logrus.Infof("Added config path: %s (absolute: %s)", path, absPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logrus.Errorf("config file not found: %v", err)
		} else {
			logrus.Errorf("config error: %v", err)
		}
		return
	}

	logrus.Infof("Successfully read config file: %s", viper.ConfigFileUsed())

	// 手动从viper获取配置值，避免Unmarshal问题
	config.ConfigInfo.Mysql.Addr = viper.GetString("mysql.addr")
	config.ConfigInfo.Mysql.Database = viper.GetString("mysql.database")
	config.ConfigInfo.Mysql.Username = viper.GetString("mysql.username")
	config.ConfigInfo.Mysql.Password = viper.GetString("mysql.password")
	config.ConfigInfo.Mysql.Charset = viper.GetString("mysql.charset")

	config.ConfigInfo.CommentSharding.DatabaseCount = viper.GetInt("comment_sharding.database_count")
	config.ConfigInfo.CommentSharding.TableCount = viper.GetInt("comment_sharding.table_count")
	config.ConfigInfo.CommentSharding.MaxOpenConns = viper.GetInt("comment_sharding.max_open_conns")
	config.ConfigInfo.CommentSharding.MaxIdleConns = viper.GetInt("comment_sharding.max_idle_conns")
	config.ConfigInfo.CommentSharding.ConnMaxLifetime = viper.GetString("comment_sharding.conn_max_lifetime")
	config.ConfigInfo.CommentSharding.MasterDSNs = viper.GetStringSlice("comment_sharding.master_dsns")

	// 获取slave_dsns (二维数组)
	if slaveDSNsInterface := viper.Get("comment_sharding.slave_dsns"); slaveDSNsInterface != nil {
		if slaveDSNsSlice, ok := slaveDSNsInterface.([]interface{}); ok {
			config.ConfigInfo.CommentSharding.SlaveDSNs = make([][]string, len(slaveDSNsSlice))
			for i, item := range slaveDSNsSlice {
				if itemSlice, ok := item.([]interface{}); ok {
					config.ConfigInfo.CommentSharding.SlaveDSNs[i] = make([]string, len(itemSlice))
					for j, dsn := range itemSlice {
						if dsnStr, ok := dsn.(string); ok {
							config.ConfigInfo.CommentSharding.SlaveDSNs[i][j] = dsnStr
						}
					}
				}
			}
		}
	}

	config.ConfigInfo.Redis.Addr = viper.GetString("redis.addr")
	config.ConfigInfo.Redis.Password = viper.GetString("redis.password")

	config.ConfigInfo.Etcd.Addr = viper.GetString("etcd.addr")

	config.ConfigInfo.RabbitMq.Addr = viper.GetString("rabbitmq.addr")
	config.ConfigInfo.RabbitMq.Username = viper.GetString("rabbitmq.username")
	config.ConfigInfo.RabbitMq.Password = viper.GetString("rabbitmq.password")

	logrus.Infof("Config loaded - MySQL: %s:***@%s/%s",
		config.ConfigInfo.Mysql.Username,
		config.ConfigInfo.Mysql.Addr,
		config.ConfigInfo.Mysql.Database)
	logrus.Infof("Comment sharding - DatabaseCount: %d, TableCount: %d, MasterDSNs count: %d",
		config.ConfigInfo.CommentSharding.DatabaseCount,
		config.ConfigInfo.CommentSharding.TableCount,
		len(config.ConfigInfo.CommentSharding.MasterDSNs))
}

func main() {
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
