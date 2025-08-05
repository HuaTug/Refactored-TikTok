package dal

import (
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/config"
	"HuaTug.com/pkg/cache"
	"github.com/cloudwego/hertz/pkg/common/hlog"
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
	// 处理SlaveDSNs类型转换 - 将[][]string转换为[]string
	var slaveDSNs []string
	for _, dsns := range config.ConfigInfo.CommentSharding.SlaveDSNs {
		slaveDSNs = append(slaveDSNs, dsns...)
	}

	shardingConfig := &db.ShardingConfig{
		DatabaseCount:   config.ConfigInfo.CommentSharding.DatabaseCount,
		TableCount:      config.ConfigInfo.CommentSharding.TableCount,
		MasterDSNs:      config.ConfigInfo.CommentSharding.MasterDSNs,
		SlaveDSNs:       slaveDSNs,
		MaxOpenConns:    config.ConfigInfo.CommentSharding.MaxOpenConns,
		MaxIdleConns:    config.ConfigInfo.CommentSharding.MaxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
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

	// 初始化缓存管理器
	var cacheManager *cache.CommentCacheManager
	// TODO: 这里应该根据实际情况初始化缓存管理器

	// 创建分片评论数据库实例
	ShardedCommentDBInstance = db.NewShardedCommentDB(shardingManager, cacheManager)
	hlog.Info("ShardedCommentDBInstance created successfully: ", ShardedCommentDBInstance)
	return nil
}
