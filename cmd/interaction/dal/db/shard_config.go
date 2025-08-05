package db

import (
	"fmt"
	"sync"
)

// ShardConfig 分片配置
type ShardConfig struct {
	// 数据库分片数量
	DatabaseCount int
	// 每个数据库中的表分片数量
	TableCount int
	// 数据库连接配置
	DatabaseConfigs []DatabaseConfig
}


// DatabaseConfig 单个数据库配置
type DatabaseConfig struct {
	// 数据库名称
	Name string
	// 连接字符串
	DSN string
	// 最大连接数
	MaxOpenConns int
	// 最大空闲连接数
	MaxIdleConns int
	// 连接最大生命周期(秒)
	ConnMaxLifetime int
}

// ShardCalculator 分片计算器
type ShardCalculator struct {
	config *ShardConfig
	mu     sync.RWMutex
}

// NewShardCalculator 创建分片计算器
func NewShardCalculator(config *ShardConfig) *ShardCalculator {
	return &ShardCalculator{
		config: config,
	}
}

// GetDatabaseIndex 获取数据库索引
func (sc *ShardCalculator) GetDatabaseIndex(shardKey int64) int {
	return int(shardKey % int64(sc.config.DatabaseCount))
}

// GetTableIndex 获取表索引
func (sc *ShardCalculator) GetTableIndex(shardKey int64) int {
	return int((shardKey / int64(sc.config.DatabaseCount)) % int64(sc.config.TableCount))
}

// GetDatabaseName 获取数据库名称
func (sc *ShardCalculator) GetDatabaseName(shardKey int64) string {
	dbIndex := sc.GetDatabaseIndex(shardKey)
	if dbIndex < len(sc.config.DatabaseConfigs) {
		return sc.config.DatabaseConfigs[dbIndex].Name
	}
	return fmt.Sprintf("interaction_db_%d", dbIndex)
}

// GetTableName 获取表名称
func (sc *ShardCalculator) GetTableName(shardKey int64, tablePrefix string) string {
	tableIndex := sc.GetTableIndex(shardKey)
	return fmt.Sprintf("%s_%d", tablePrefix, tableIndex)
}

// GetShardInfo 获取分片信息
func (sc *ShardCalculator) GetShardInfo(shardKey int64, tablePrefix string) (string, string) {
	dbName := sc.GetDatabaseName(shardKey)
	tableName := sc.GetTableName(shardKey, tablePrefix)
	return dbName, tableName
}
