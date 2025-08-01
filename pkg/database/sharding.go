package database

import (
	"context"
	"fmt"
	"hash/crc32"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ShardingConfig 分库分表配置
type ShardingConfig struct {
	// 分库数量
	DatabaseCount int `yaml:"database_count" json:"database_count"`
	// 每个库的分表数量
	TableCount int `yaml:"table_count" json:"table_count"`
	// 主库配置
	MasterDSNs []string `yaml:"master_dsns" json:"master_dsns"`
	// 从库配置
	SlaveDSNs [][]string `yaml:"slave_dsns" json:"slave_dsns"`
	// 连接池配置
	MaxOpenConns int `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns int `yaml:"max_idle_conns" json:"max_idle_conns"`
	// 连接超时配置
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
}

// ShardingManager 分库分表管理器
type ShardingManager struct {
	config    *ShardingConfig
	masterDBs []*gorm.DB   // 主库连接池
	slaveDBs  [][]*gorm.DB // 从库连接池 [db_index][slave_index]
	mu        sync.RWMutex
}

// NewShardingManager 创建分库分表管理器
func NewShardingManager(config *ShardingConfig) (*ShardingManager, error) {
	manager := &ShardingManager{
		config:    config,
		masterDBs: make([]*gorm.DB, config.DatabaseCount),
		slaveDBs:  make([][]*gorm.DB, config.DatabaseCount),
	}

	// 初始化主库连接
	for i := 0; i < config.DatabaseCount; i++ {
		masterDB, err := manager.createDBConnection(config.MasterDSNs[i])
		if err != nil {
			return nil, fmt.Errorf("failed to connect to master database %d: %w", i, err)
		}
		manager.masterDBs[i] = masterDB

		// 初始化从库连接
		slaveCount := len(config.SlaveDSNs[i])
		manager.slaveDBs[i] = make([]*gorm.DB, slaveCount)
		for j := 0; j < slaveCount; j++ {
			slaveDB, err := manager.createDBConnection(config.SlaveDSNs[i][j])
			if err != nil {
				return nil, fmt.Errorf("failed to connect to slave database %d-%d: %w", i, j, err)
			}
			manager.slaveDBs[i][j] = slaveDB
		}
	}

	hlog.Infof("Sharding manager initialized with %d databases, %d tables per database",
		config.DatabaseCount, config.TableCount)
	return manager, nil
}

// createDBConnection 创建数据库连接
func (sm *ShardingManager) createDBConnection(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(sm.config.MaxOpenConns)
	sqlDB.SetMaxIdleConns(sm.config.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(sm.config.ConnMaxLifetime)

	return db, nil
}

// GetShardInfo 根据视频ID获取分片信息
func (sm *ShardingManager) GetShardInfo(videoID int64) (dbIndex, tableIndex int) {
	// 使用CRC32哈希算法进行分片
	hash := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%d", videoID)))
	dbIndex = int(hash) % sm.config.DatabaseCount
	tableIndex = int(hash) % sm.config.TableCount
	return
}

// GetMasterDB 获取主库连接（用于写操作）
func (sm *ShardingManager) GetMasterDB(videoID int64) *gorm.DB {
	dbIndex, _ := sm.GetShardInfo(videoID)
	return sm.masterDBs[dbIndex]
}

// GetSlaveDB 获取从库连接（用于读操作）
func (sm *ShardingManager) GetSlaveDB(videoID int64) *gorm.DB {
	dbIndex, _ := sm.GetShardInfo(videoID)

	// 如果没有从库，返回主库
	if len(sm.slaveDBs[dbIndex]) == 0 {
		return sm.masterDBs[dbIndex]
	}

	// 简单的轮询负载均衡
	slaveIndex := int(time.Now().UnixNano()) % len(sm.slaveDBs[dbIndex])
	return sm.slaveDBs[dbIndex][slaveIndex]
}

// GetTableName 获取分表名称
func (sm *ShardingManager) GetTableName(baseTableName string, videoID int64) string {
	_, tableIndex := sm.GetShardInfo(videoID)
	return fmt.Sprintf("%s_%d", baseTableName, tableIndex)
}

// ExecuteInShard 在指定分片中执行操作
func (sm *ShardingManager) ExecuteInShard(ctx context.Context, videoID int64, isWrite bool,
	fn func(db *gorm.DB, tableName string) error) error {

	var db *gorm.DB
	if isWrite {
		db = sm.GetMasterDB(videoID)
	} else {
		db = sm.GetSlaveDB(videoID)
	}

	tableName := sm.GetTableName("comments", videoID)
	return fn(db.WithContext(ctx), tableName)
}

// Close 关闭所有数据库连接
func (sm *ShardingManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 关闭主库连接
	for i, db := range sm.masterDBs {
		if db != nil {
			if sqlDB, err := db.DB(); err == nil {
				if err := sqlDB.Close(); err != nil {
					hlog.Errorf("Failed to close master database %d: %v", i, err)
				}
			}
		}
	}

	// 关闭从库连接
	for i, slaves := range sm.slaveDBs {
		for j, db := range slaves {
			if db != nil {
				if sqlDB, err := db.DB(); err == nil {
					if err := sqlDB.Close(); err != nil {
						hlog.Errorf("Failed to close slave database %d-%d: %v", i, j, err)
					}
				}
			}
		}
	}

	return nil
}

// HealthCheck 健康检查
func (sm *ShardingManager) HealthCheck(ctx context.Context) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 检查主库
	for i, db := range sm.masterDBs {
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.PingContext(ctx); err != nil {
				return fmt.Errorf("master database %d health check failed: %w", i, err)
			}
		}
	}

	// 检查从库
	for i, slaves := range sm.slaveDBs {
		for j, db := range slaves {
			if sqlDB, err := db.DB(); err == nil {
				if err := sqlDB.PingContext(ctx); err != nil {
					hlog.Warnf("Slave database %d-%d health check failed: %v", i, j, err)
					// 从库失败不返回错误，只记录警告
				}
			}
		}
	}

	return nil
}

// GetDatabaseCount 获取数据库数量
func (sm *ShardingManager) GetDatabaseCount() int {
	return sm.config.DatabaseCount
}

// GetTableCount 获取每个数据库的表数量
func (sm *ShardingManager) GetTableCount() int {
	return sm.config.TableCount
}

// GetMasterDBByIndex 根据索引获取主库连接
func (sm *ShardingManager) GetMasterDBByIndex(index int) *gorm.DB {
	if index >= 0 && index < len(sm.masterDBs) {
		return sm.masterDBs[index]
	}
	return nil
}

// GetSlaveDBByIndex 根据索引获取从库连接
func (sm *ShardingManager) GetSlaveDBByIndex(index int) *gorm.DB {
	if index >= 0 && index < len(sm.slaveDBs) && len(sm.slaveDBs[index]) > 0 {
		// 简单轮询
		slaveIndex := int(time.Now().UnixNano()) % len(sm.slaveDBs[index])
		return sm.slaveDBs[index][slaveIndex]
	}
	return sm.GetMasterDBByIndex(index)
}
