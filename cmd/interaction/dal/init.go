package dal

import (
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/config"
	"HuaTug.com/pkg/database"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func Init() {
	db.Init() // mysql init

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
	shardingConfig := &database.ShardingConfig{
		DatabaseCount:   config.ConfigInfo.CommentSharding.DatabaseCount,
		TableCount:      config.ConfigInfo.CommentSharding.TableCount,
		MasterDSNs:      config.ConfigInfo.CommentSharding.MasterDSNs,
		SlaveDSNs:       config.ConfigInfo.CommentSharding.SlaveDSNs,
		MaxOpenConns:    config.ConfigInfo.CommentSharding.MaxOpenConns,
		MaxIdleConns:    config.ConfigInfo.CommentSharding.MaxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
	}

	// 打印配置信息用于调试
	hlog.Infof("Sharding config - DatabaseCount: %d, TableCount: %d",
		shardingConfig.DatabaseCount, shardingConfig.TableCount)
	hlog.Infof("Master DSNs count: %d", len(shardingConfig.MasterDSNs))
	for i, dsn := range shardingConfig.MasterDSNs {
		// 隐藏密码信息
		hlog.Infof("Master DSN[%d]: %s", i, maskPassword(dsn))
	}

	// 如果没有配置分片DSN，则不初始化分片管理器
	if len(shardingConfig.MasterDSNs) == 0 {
		hlog.Warn("No sharding DSNs configured, this will cause comment operations to fail")
		return nil
	}

	// 验证配置
	if shardingConfig.DatabaseCount <= 0 || shardingConfig.TableCount <= 0 {
		return fmt.Errorf("invalid sharding config: DatabaseCount=%d, TableCount=%d",
			shardingConfig.DatabaseCount, shardingConfig.TableCount)
	}

	if len(shardingConfig.MasterDSNs) != shardingConfig.DatabaseCount {
		return fmt.Errorf("master DSNs count (%d) doesn't match database count (%d)",
			len(shardingConfig.MasterDSNs), shardingConfig.DatabaseCount)
	}

	// 初始化分片管理器
	hlog.Info("Initializing sharding manager...")
	if err := db.InitShardingManager(shardingConfig); err != nil {
		hlog.Errorf("Sharding manager initialization failed: %v", err)
		return err
	}

	hlog.Info("Sharding manager initialized successfully")
	return nil
}

// maskPassword 隐藏DSN中的密码信息
func maskPassword(dsn string) string {
	// 简单的密码隐藏逻辑
	if len(dsn) > 50 {
		return dsn[:20] + "***" + dsn[len(dsn)-20:]
	}
	return dsn[:10] + "***"
}
