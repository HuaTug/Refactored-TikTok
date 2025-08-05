package db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ShardingManager 分片管理器
type ShardingManager struct {
	config     *ShardingConfig
	calculator *ShardCalculator
	databases  map[string]*gorm.DB
	mu         sync.RWMutex
}

// ShardingConfig 分片配置结构
type ShardingConfig struct {
	DatabaseCount   int
	TableCount      int
	MasterDSNs      []string
	SlaveDSNs       []string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// 全局分片管理器实例
var globalShardingManager *ShardingManager

// InitShardingManager 初始化全局分片管理器
func InitShardingManager(config *ShardingConfig) error {
	var err error
	globalShardingManager, err = NewShardingManager(config)
	return err
}

// GetShardingManager 获取全局分片管理器
func GetShardingManager() *ShardingManager {
	return globalShardingManager
}


// NewShardingManager 创建分片管理器
func NewShardingManager(config *ShardingConfig) (*ShardingManager, error) {
	if config == nil {
		return nil, fmt.Errorf("sharding config cannot be nil")
	}

	// 创建分片计算器
	shardConfig := &ShardConfig{
		DatabaseCount: config.DatabaseCount,
		TableCount:    config.TableCount,
	}

	manager := &ShardingManager{
		config:     config,
		calculator: NewShardCalculator(shardConfig),
		databases:  make(map[string]*gorm.DB),
	}

	// 初始化数据库连接
	if err := manager.initConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize connections: %w", err)
	}

	return manager, nil
}

// initConnections 初始化所有数据库连接
func (sm *ShardingManager) initConnections() error {
	for i, dsn := range sm.config.MasterDSNs {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to database %d: %w", i, err)
		}

		// 配置连接池
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get sql.DB for database %d: %w", i, err)
		}

		sqlDB.SetMaxOpenConns(sm.config.MaxOpenConns)
		sqlDB.SetMaxIdleConns(sm.config.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(sm.config.ConnMaxLifetime)

		dbKey := fmt.Sprintf("db_%d", i)
		sm.databases[dbKey] = db
	}

	return nil
}

// ExecuteInShard 在指定分片中执行操作
func (sm *ShardingManager) ExecuteInShard(ctx context.Context, shardKey int64, useWriteDB bool, fn func(db *gorm.DB, tableName string) error) error {
	dbIndex := sm.calculator.GetDatabaseIndex(shardKey)
	tableIndex := sm.calculator.GetTableIndex(shardKey)

	dbKey := fmt.Sprintf("db_%d", dbIndex)
	tableName := fmt.Sprintf("comments_%d", tableIndex)

	sm.mu.RLock()
	db, exists := sm.databases[dbKey]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("database %s not found", dbKey)
	}

	return fn(db, tableName)
}

// GetAllDatabases 获取所有数据库连接
func (sm *ShardingManager) GetAllDatabases() map[string]*gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*gorm.DB)
	for k, v := range sm.databases {
		result[k] = v
	}
	return result
}

// Close 关闭所有数据库连接
func (sm *ShardingManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, db := range sm.databases {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}

	return nil
}

// HealthCheck 健康检查
func (sm *ShardingManager) HealthCheck(ctx context.Context) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for dbKey, db := range sm.databases {
		if sqlDB, err := db.DB(); err != nil {
			return fmt.Errorf("failed to get sql.DB for %s: %w", dbKey, err)
		} else if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("failed to ping database %s: %w", dbKey, err)
		}
	}

	return nil
}

