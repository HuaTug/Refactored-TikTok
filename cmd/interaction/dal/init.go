package dal

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/config"
	"HuaTug.com/pkg/cache"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	goredisv9 "github.com/redis/go-redis/v9"
)

// ShardedCommentDBInstance 全局分片评论数据库实例
var ShardedCommentDBInstance *db.ShardedCommentDB

func Init() {
	db.Init()
	// 初始化分片管理器
	if err := initShardingManager(); err != nil {
		hlog.Errorf("Failed to initialize sharding manager: %v", err)
		// 对于分片管理器初始化失败，应该panic，因为系统依赖分片功能
		panic("Sharding manager initialization failed: " + err.Error())
	}
}

func initShardingManager() error {
	hlog.Info("Starting sharding manager initialization...")

	// 解析连接超时时间
	connMaxLifetime, err := time.ParseDuration(config.ConfigInfo.CommentSharding.ConnMaxLifetime)
	if err != nil {
		hlog.Errorf("Failed to parse conn_max_lifetime: %v", err)
		connMaxLifetime = time.Hour // 默认1小时
	}

	// 从配置中获取分片配置
	shardingConfig := &db.ShardingConfig{
		DatabaseCount:   config.ConfigInfo.CommentSharding.DatabaseCount,
		TableCount:      config.ConfigInfo.CommentSharding.TableCount,
		MasterDSNs:      config.ConfigInfo.CommentSharding.MasterDSNs,
		SlaveDSNs:       config.ConfigInfo.CommentSharding.SlaveDSNs,
		MaxOpenConns:    config.ConfigInfo.CommentSharding.MaxOpenConns,
		MaxIdleConns:    config.ConfigInfo.CommentSharding.MaxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
		EnableIndex:     true, // 启用全局索引
	}

	// 使用全局的InitShardingManager初始化分片管理器
	if err := db.InitShardingManager(shardingConfig); err != nil {
		return fmt.Errorf("failed to initialize sharding manager: %w", err)
	}

	// 验证分片管理器是否成功初始化
	shardingManager := db.GetShardingManager()
	if shardingManager == nil {
		return fmt.Errorf("sharding manager is nil after initialization")
	}

	// 初始化缓存管理器 (connect to Redis for comment caching)
	var cacheManager *cache.CommentCacheManager
	redisAddr := config.ConfigInfo.Redis.Addr
	redisPassword := config.ConfigInfo.Redis.Password
	if redisAddr != "" {
		redisClient := goredisv9.NewClient(&goredisv9.Options{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       2, // Use DB 2 for comment cache
		})
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			hlog.Warnf("Failed to connect to Redis for comment cache: %v, caching disabled", err)
		} else {
			cacheManager = cache.NewCommentCacheManager(redisClient)
			hlog.Info("Comment cache manager initialized successfully")
		}
	} else {
		hlog.Warn("Redis address not configured, comment cache disabled")
	}

	// 创建分片评论数据库实例
	ShardedCommentDBInstance = db.NewShardedCommentDB(shardingManager, cacheManager)
	hlog.Info("ShardedCommentDBInstance created successfully: ", ShardedCommentDBInstance)
	return nil
}
