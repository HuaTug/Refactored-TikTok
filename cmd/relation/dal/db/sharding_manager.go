package db

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ShardingManager 分片管理器（支持读写分离）
type ShardingManager struct {
	config     *ShardingConfig
	calculator *ShardCalculator
	masterDBs  map[string]*gorm.DB   // 主库（写）
	slaveDBs   map[string][]*gorm.DB // 从库（读），每个分片可有多个从库
	mu         sync.RWMutex
}

// ShardingConfig 分片配置结构
type ShardingConfig struct {
	DatabaseCount   int
	TableCount      int
	MasterDSNs      []string   // 主库DSN列表
	SlaveDSNs       [][]string // 从库DSN列表，二维数组：[分片索引][从库索引]
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

	shardConfig := &ShardConfig{
		DatabaseCount: config.DatabaseCount,
		TableCount:    config.TableCount,
	}

	manager := &ShardingManager{
		config:     config,
		calculator: NewShardCalculator(shardConfig),
		masterDBs:  make(map[string]*gorm.DB),
		slaveDBs:   make(map[string][]*gorm.DB),
	}

	// 初始化主库连接
	if err := manager.initMasterConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize master connections: %w", err)
	}

	// 初始化从库连接
	if err := manager.initSlaveConnections(); err != nil {
		hlog.Warnf("Failed to initialize slave connections: %v (read-write separation disabled)", err)
	}

	return manager, nil
}

// initMasterConnections 初始化主库连接
func (sm *ShardingManager) initMasterConnections() error {
	for i, dsn := range sm.config.MasterDSNs {
		db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
		if err != nil {
			return fmt.Errorf("failed to connect to master database %d: %w", i, err)
		}

		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get sql.DB for master %d: %w", i, err)
		}

		sqlDB.SetMaxOpenConns(sm.config.MaxOpenConns)
		sqlDB.SetMaxIdleConns(sm.config.MaxIdleConns)
		sqlDB.SetConnMaxLifetime(sm.config.ConnMaxLifetime)

		dbKey := fmt.Sprintf("db_%d", i)
		sm.masterDBs[dbKey] = db
		hlog.Infof("Master database %s connected", dbKey)
	}
	return nil
}

// initSlaveConnections 初始化从库连接
func (sm *ShardingManager) initSlaveConnections() error {
	if len(sm.config.SlaveDSNs) == 0 {
		hlog.Info("No slave DSNs configured, read-write separation disabled")
		return nil
	}

	for i, slaveDSNList := range sm.config.SlaveDSNs {
		dbKey := fmt.Sprintf("db_%d", i)
		sm.slaveDBs[dbKey] = make([]*gorm.DB, 0, len(slaveDSNList))

		for j, dsn := range slaveDSNList {
			db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
			if err != nil {
				hlog.Warnf("Failed to connect to slave database %d-%d: %v", i, j, err)
				continue
			}

			sqlDB, err := db.DB()
			if err != nil {
				hlog.Warnf("Failed to get sql.DB for slave %d-%d: %v", i, j, err)
				continue
			}

			sqlDB.SetMaxOpenConns(sm.config.MaxOpenConns)
			sqlDB.SetMaxIdleConns(sm.config.MaxIdleConns)
			sqlDB.SetConnMaxLifetime(sm.config.ConnMaxLifetime)

			sm.slaveDBs[dbKey] = append(sm.slaveDBs[dbKey], db)
			hlog.Infof("Slave database %s-%d connected", dbKey, j)
		}
	}

	return nil
}

// getSlaveDB 获取从库连接（负载均衡）
func (sm *ShardingManager) getSlaveDB(dbKey string) *gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	slaves, exists := sm.slaveDBs[dbKey]
	if !exists || len(slaves) == 0 {
		return sm.masterDBs[dbKey]
	}

	return slaves[rand.Intn(len(slaves))]
}

// getMasterDB 获取主库连接
func (sm *ShardingManager) getMasterDB(dbKey string) *gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.masterDBs[dbKey]
}

// ExecuteInShard 在指定分片中执行操作（支持读写分离）
func (sm *ShardingManager) ExecuteInShard(ctx context.Context, shardKey int64, useWriteDB bool, fn func(db *gorm.DB, tableName string) error) error {
	dbIndex := sm.calculator.GetDatabaseIndex(shardKey)
	tableIndex := sm.calculator.GetTableIndex(shardKey)

	dbKey := fmt.Sprintf("db_%d", dbIndex)
	tableName := fmt.Sprintf("follows_%d", tableIndex)

	var db *gorm.DB
	if useWriteDB {
		db = sm.getMasterDB(dbKey)
	} else {
		db = sm.getSlaveDB(dbKey)
	}

	if db == nil {
		return fmt.Errorf("database for key %s not found", dbKey)
	}

	return fn(db, tableName)
}

// ExecuteWrite 执行写操作（始终使用主库）
func (sm *ShardingManager) ExecuteWrite(ctx context.Context, shardKey int64, fn func(db *gorm.DB, tableName string) error) error {
	return sm.ExecuteInShard(ctx, shardKey, true, fn)
}

// ExecuteRead 执行读操作（优先使用从库）
func (sm *ShardingManager) ExecuteRead(ctx context.Context, shardKey int64, fn func(db *gorm.DB, tableName string) error) error {
	return sm.ExecuteInShard(ctx, shardKey, false, fn)
}

// ExecuteInAllShards 在所有分片执行操作（并发）
func (sm *ShardingManager) ExecuteInAllShards(ctx context.Context, useWriteDB bool, fn func(db *gorm.DB, tableName string) error) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(sm.masterDBs)*sm.config.TableCount)

	for dbKey := range sm.masterDBs {
		for tableIdx := 0; tableIdx < sm.config.TableCount; tableIdx++ {
			wg.Add(1)
			go func(key string, idx int) {
				defer wg.Done()
				tableName := fmt.Sprintf("follows_%d", idx)

				var db *gorm.DB
				if useWriteDB {
					db = sm.masterDBs[key]
				} else {
					db = sm.getSlaveDB(key)
				}

				if err := fn(db, tableName); err != nil {
					errChan <- fmt.Errorf("shard %s.%s: %w", key, tableName, err)
				}
			}(dbKey, tableIdx)
		}
	}

	wg.Wait()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("multiple errors: %v", errs)
	}

	return nil
}

// GetAllDatabases 获取所有主库连接
func (sm *ShardingManager) GetAllDatabases() map[string]*gorm.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*gorm.DB)
	for k, v := range sm.masterDBs {
		result[k] = v
	}
	return result
}

// GetShardCalculator 获取分片计算器
func (sm *ShardingManager) GetShardCalculator() *ShardCalculator {
	return sm.calculator
}

// Close 关闭所有数据库连接
func (sm *ShardingManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, db := range sm.masterDBs {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}

	for _, slaves := range sm.slaveDBs {
		for _, db := range slaves {
			if sqlDB, err := db.DB(); err == nil {
				sqlDB.Close()
			}
		}
	}

	return nil
}

// HealthCheck 健康检查
func (sm *ShardingManager) HealthCheck() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for dbKey, db := range sm.masterDBs {
		if sqlDB, err := db.DB(); err != nil {
			return fmt.Errorf("master database %s health check failed: %v", dbKey, err)
		} else if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("master database %s ping failed: %v", dbKey, err)
		}
	}

	for dbKey, slaves := range sm.slaveDBs {
		for i, db := range slaves {
			if sqlDB, err := db.DB(); err != nil {
				hlog.Warnf("Slave %s-%d health check failed: %v", dbKey, i, err)
			} else if err := sqlDB.Ping(); err != nil {
				hlog.Warnf("Slave %s-%d ping failed: %v", dbKey, i, err)
			}
		}
	}

	return nil
}

// GetShardInfo 获取分片信息
func (sm *ShardingManager) GetShardInfo(shardKey int64) (dbIndex int, tableIndex int) {
	dbIndex = sm.calculator.GetDatabaseIndex(shardKey)
	tableIndex = sm.calculator.GetTableIndex(shardKey)
	return
}

// HasSlaves 检查是否配置了从库
func (sm *ShardingManager) HasSlaves() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for _, slaves := range sm.slaveDBs {
		if len(slaves) > 0 {
			return true
		}
	}
	return false
}
