package dal

import (
	"fmt"
	"time"

	"HuaTug.com/cmd/relation/dal/db"
	"HuaTug.com/config"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

var ShardedFollowDBInstance *db.ShardedFollowDB

func Init() {
	db.Init() // mysql init
	if err := initShardingManager(); err != nil {
		hlog.Errorf("Failed to initialize sharding manager: %v", err)
		panic("Sharding manager initialization failed: " + err.Error())
	}
	hlog.Info("Sharding manager initialized successfully")
}

func initShardingManager() error {
	hlog.Info("Starting sharding manager initialization...")

	// 解析连接超时时间
	connMaxLifetime := time.Hour // 默认1小时
	if config.ConfigInfo.FollowsSharding.ConnMaxLifetime != "" {
		var err error
		connMaxLifetime, err = time.ParseDuration(config.ConfigInfo.FollowsSharding.ConnMaxLifetime)
		if err != nil {
			hlog.Errorf("Failed to parse conn_max_lifetime '%s': %v, using default: 1h",
				config.ConfigInfo.FollowsSharding.ConnMaxLifetime, err)
		}
	} else {
		hlog.Warn("conn_max_lifetime not configured, using default: 1h")
	}

	// 验证分片配置
	if config.ConfigInfo.FollowsSharding.DatabaseCount <= 0 {
		config.ConfigInfo.FollowsSharding.DatabaseCount = 1 // 默认1个数据库
		hlog.Warn("DatabaseCount not configured, using default value: 1")
	}
	if config.ConfigInfo.FollowsSharding.TableCount <= 0 {
		config.ConfigInfo.FollowsSharding.TableCount = 4 // 默认4个表
		hlog.Warn("TableCount not configured, using default value: 4")
	}

	// 验证MasterDSNs配置
	if len(config.ConfigInfo.FollowsSharding.MasterDSNs) == 0 {
		return fmt.Errorf("MasterDSNs cannot be empty")
	}

	// 从配置中获取分片配置
	// 处理SlaveDSNs类型转换 - 将[][]string转换为[]string
	var slaveDSNs []string
	for _, dsns := range config.ConfigInfo.FollowsSharding.SlaveDSNs {
		slaveDSNs = append(slaveDSNs, dsns...)
	}

	shardingConfig := &db.ShardingConfig{
		DatabaseCount:   config.ConfigInfo.FollowsSharding.DatabaseCount,
		TableCount:      config.ConfigInfo.FollowsSharding.TableCount,
		MasterDSNs:      config.ConfigInfo.FollowsSharding.MasterDSNs,
		SlaveDSNs:       slaveDSNs,
		MaxOpenConns:    config.ConfigInfo.FollowsSharding.MaxOpenConns,
		MaxIdleConns:    config.ConfigInfo.FollowsSharding.MaxIdleConns,
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

	// 创建分片评论数据库实例
	ShardedFollowDBInstance = db.NewShardedFollowDB(shardingManager)
	hlog.Info("ShardedFollowDBInstance created successfully")
	return nil
}
