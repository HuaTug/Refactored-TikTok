package db

import (
	"fmt"
	"sync"
)

type ShardConfig struct {
	DatabaseCount   int
	TableCount      int
	DatabaseConfigs []DatabaseConfig
}

type DatabaseConfig struct {
	Name            string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime int
}

type ShardCalculator struct {
	config *ShardConfig
	mu     sync.RWMutex
}

func NewShardCalculator(config *ShardConfig) *ShardCalculator {
	if config == nil {
		panic("ShardConfig cannot be nil")
	}
	if config.DatabaseCount <= 0 {
		panic("DatabaseCount must be greater than 0")
	}
	if config.TableCount <= 0 {
		panic("TableCount must be greater than 0")
	}
	return &ShardCalculator{
		config: config,
	}
}

func (sc *ShardCalculator) GetDatabaseIndex(shardKey int64) int {
	if sc.config.DatabaseCount <= 0 {
		return 0
	}
	return int(shardKey % int64(sc.config.DatabaseCount))
}

func (sc *ShardCalculator) GetTableIndex(shardKey int64) int {
	if sc.config.DatabaseCount <= 0 || sc.config.TableCount <= 0 {
		return 0
	}
	return int((shardKey / int64(sc.config.DatabaseCount)) % int64(sc.config.TableCount))
}

func (sc *ShardCalculator) GetDatabaseName(shardKey int64) string {
	dbIndx := sc.GetDatabaseIndex(shardKey)
	if dbIndx < len(sc.config.DatabaseConfigs) {
		return sc.config.DatabaseConfigs[dbIndx].Name
	}
	return ""
}

func (sc *ShardCalculator) GetTableName(shardKey int64, tablePrefix string) string {
	tableIndex := sc.GetTableIndex(shardKey)
	return fmt.Sprintf("%s_%d", tablePrefix, tableIndex)
}

func (sc *ShardCalculator) GetShardInfo(shardKey int64, tablePrefix string) (string, string) {
	dbName := sc.GetDatabaseName(shardKey)
	tableName := sc.GetTableName(shardKey, tablePrefix)
	return dbName, tableName
}
