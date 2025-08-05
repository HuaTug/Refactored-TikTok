package db

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type ShardingManager struct {
	config     *ShardingConfig
	calculator *ShardCalculator
	databases  map[string]*gorm.DB
	mu         sync.RWMutex
}

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

func NewShardingManager(config *ShardingConfig) (*ShardingManager, error) {
	if config == nil {
		return nil, fmt.Errorf("sharding config cannot be nil")
	}

	shardConfig := &ShardConfig{
		DatabaseCount: config.DatabaseCount,
		TableCount:    config.TableCount,
	}

	manager := &ShardingManager{
		config:     config,
		calculator: NewShardCalculator(shardConfig),
		databases:  make(map[string]*gorm.DB),
	}

	if err := manager.initConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize connections: %w", err)
	}
	return manager, nil
}

func (sm *ShardingManager) initConnections() error {
	for i, dsn := range sm.config.MasterDSNs {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to open database connection for %s: %w", dsn, err)
		}

		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get sql.DB for %s: %w", dsn, err)
		}

		sqlDB.SetMaxOpenConns(sm.config.MaxOpenConns)
		sqlDB.SetMaxIdleConns(sm.config.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(sm.config.ConnMaxLifetime)

		dbKey := fmt.Sprintf("db_%d", i)
		sm.databases[dbKey] = db
	}
	return nil
}

func (sm *ShardingManager) ExecuteInShard(ctx context.Context, shardKey int64, userWriteDB bool, fn func(db *gorm.DB, tableName string) error) error {
	dbIndex := sm.calculator.GetDatabaseIndex(shardKey)
	tableIndex := sm.calculator.GetTableIndex(shardKey)

	dbKey := fmt.Sprintf("db_%d", dbIndex)
	tableName := fmt.Sprintf("follows_%d", tableIndex)

	sm.mu.RLock()
	db, exists := sm.databases[dbKey]
	sm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("database for key %s not found", dbKey)
	}

	return fn(db, tableName)
}

func (sm *ShardingManager) GetAllDatabases() map[string]*gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*gorm.DB)
	for k, v := range sm.databases {
		result[k] = v
	}
	return result
}

func (sm *ShardingManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, db := range sm.databases {
		if sqlDB, err := db.DB(); err != nil {
			sqlDB.Close()
		}
	}

	return nil
}

func (sm *ShardingManager) HealthCheck() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for dbKey, db := range sm.databases {
		if sqlDB, err := db.DB(); err != nil {
			return fmt.Errorf("database %s health check failed: %v", dbKey, err)
		} else if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("database %s ping failed: %v", dbKey, err)
		}
	}

	return nil
}
