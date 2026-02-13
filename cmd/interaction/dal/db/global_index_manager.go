// Package db provides global index management for sharded databases.
// Global indexes enable efficient lookups by non-sharding keys (e.g., comment_id).
package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/pkg/infra/cache"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/gomodule/redigo/redis"
	"gorm.io/gorm"
)

// GlobalIndexManager 全局索引管理器
// 用于管理分库分表场景下的全局索引，支持通过非分片键快速定位数据
type GlobalIndexManager struct {
	shardingManager *ShardingManager
	mu              sync.RWMutex
}

// ShardInfo 分片信息
type ShardInfo struct {
	DBIndex    int   `json:"db_index"`
	TableIndex int   `json:"table_index"`
	ShardKey   int64 `json:"shard_key"`
}

// IndexEntry 索引条目
type IndexEntry struct {
	IndexKey   string    `json:"index_key"`   // 索引键类型（如 "comment_id", "user_id"）
	IndexValue int64     `json:"index_value"` // 索引值
	ShardInfo  ShardInfo `json:"shard_info"`  // 分片信息
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GlobalIndexTable 全局索引表结构（MySQL）
type GlobalIndexTable struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	IndexKey   string    `gorm:"column:index_key;size:64;index:idx_key_value"`
	IndexValue int64     `gorm:"column:index_value;index:idx_key_value"`
	DBIndex    int       `gorm:"column:db_index"`
	TableIndex int       `gorm:"column:table_index"`
	ShardKey   int64     `gorm:"column:shard_key"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 指定表名
func (GlobalIndexTable) TableName() string {
	return "global_index"
}

// NewGlobalIndexManager 创建全局索引管理器
func NewGlobalIndexManager(shardingManager *ShardingManager) *GlobalIndexManager {
	return &GlobalIndexManager{
		shardingManager: shardingManager,
	}
}

// getIndexCacheKey 获取索引缓存键
func (gim *GlobalIndexManager) getIndexCacheKey(indexKey string, indexValue int64) string {
	return fmt.Sprintf("global_index:%s:%d", indexKey, indexValue)
}

// CreateIndex 创建索引
func (gim *GlobalIndexManager) CreateIndex(ctx context.Context, indexKey string, indexValue int64, shardKey int64) error {
	// 计算分片信息
	dbIndex := gim.shardingManager.calculator.GetDatabaseIndex(shardKey)
	tableIndex := gim.shardingManager.calculator.GetTableIndex(shardKey)

	shardInfo := ShardInfo{
		DBIndex:    dbIndex,
		TableIndex: tableIndex,
		ShardKey:   shardKey,
	}

	// 1. 写入Redis缓存
	if err := gim.writeIndexToCache(ctx, indexKey, indexValue, shardInfo); err != nil {
		hlog.Warnf("Failed to write index to cache: %v", err)
	}

	// 2. 异步写入MySQL（持久化）
	go func() {
		if err := gim.writeIndexToMySQL(ctx, indexKey, indexValue, shardInfo); err != nil {
			hlog.Errorf("Failed to write index to MySQL: %v", err)
		}
	}()

	hlog.CtxInfof(ctx, "Created global index: %s=%d -> db_%d.table_%d (shardKey=%d)",
		indexKey, indexValue, dbIndex, tableIndex, shardKey)
	return nil
}

// writeIndexToCache 写入索引到Redis缓存
func (gim *GlobalIndexManager) writeIndexToCache(ctx context.Context, indexKey string, indexValue int64, shardInfo ShardInfo) error {
	conn := cache.GetRedis()
	defer conn.Close()

	cacheKey := gim.getIndexCacheKey(indexKey, indexValue)
	data, err := json.Marshal(shardInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal shard info: %w", err)
	}

	// 设置缓存，过期时间7天
	if _, err := conn.Do("SET", cacheKey, string(data), "EX", 7*24*3600); err != nil {
		return fmt.Errorf("failed to set index cache: %w", err)
	}

	return nil
}

// writeIndexToMySQL 写入索引到MySQL
func (gim *GlobalIndexManager) writeIndexToMySQL(ctx context.Context, indexKey string, indexValue int64, shardInfo ShardInfo) error {
	// 使用第一个主库存储全局索引表
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	indexEntry := &GlobalIndexTable{
		IndexKey:   indexKey,
		IndexValue: indexValue,
		DBIndex:    shardInfo.DBIndex,
		TableIndex: shardInfo.TableIndex,
		ShardKey:   shardInfo.ShardKey,
	}

	// 使用 ON DUPLICATE KEY UPDATE
	result := db.Where("index_key = ? AND index_value = ?", indexKey, indexValue).
		Assign(map[string]interface{}{
			"db_index":    shardInfo.DBIndex,
			"table_index": shardInfo.TableIndex,
			"shard_key":   shardInfo.ShardKey,
		}).
		FirstOrCreate(indexEntry)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert index: %w", result.Error)
	}

	return nil
}

// GetShardInfo 获取分片信息
func (gim *GlobalIndexManager) GetShardInfo(ctx context.Context, indexKey string, indexValue int64) (*ShardInfo, error) {
	// 1. 先从Redis缓存获取
	shardInfo, err := gim.getShardInfoFromCache(ctx, indexKey, indexValue)
	if err == nil && shardInfo != nil {
		return shardInfo, nil
	}

	// 2. 缓存未命中，从MySQL获取
	shardInfo, err = gim.getShardInfoFromMySQL(ctx, indexKey, indexValue)
	if err != nil {
		return nil, fmt.Errorf("failed to get shard info: %w", err)
	}

	// 3. 回写缓存
	if err := gim.writeIndexToCache(ctx, indexKey, indexValue, *shardInfo); err != nil {
		hlog.Warnf("Failed to write back to cache: %v", err)
	}

	return shardInfo, nil
}

// getShardInfoFromCache 从缓存获取分片信息
func (gim *GlobalIndexManager) getShardInfoFromCache(ctx context.Context, indexKey string, indexValue int64) (*ShardInfo, error) {
	conn := cache.GetRedis()
	defer conn.Close()

	cacheKey := gim.getIndexCacheKey(indexKey, indexValue)
	data, err := redis.String(conn.Do("GET", cacheKey))
	if err != nil {
		if err == redis.ErrNil {
			return nil, nil // 缓存未命中
		}
		return nil, fmt.Errorf("failed to get from cache: %w", err)
	}

	var shardInfo ShardInfo
	if err := json.Unmarshal([]byte(data), &shardInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal shard info: %w", err)
	}

	return &shardInfo, nil
}

// getShardInfoFromMySQL 从MySQL获取分片信息
func (gim *GlobalIndexManager) getShardInfoFromMySQL(ctx context.Context, indexKey string, indexValue int64) (*ShardInfo, error) {
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return nil, fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	var indexEntry GlobalIndexTable
	result := db.Where("index_key = ? AND index_value = ?", indexKey, indexValue).First(&indexEntry)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("index not found: %s=%d", indexKey, indexValue)
		}
		return nil, fmt.Errorf("failed to query index: %w", result.Error)
	}

	return &ShardInfo{
		DBIndex:    indexEntry.DBIndex,
		TableIndex: indexEntry.TableIndex,
		ShardKey:   indexEntry.ShardKey,
	}, nil
}

// DeleteIndex 删除索引
func (gim *GlobalIndexManager) DeleteIndex(ctx context.Context, indexKey string, indexValue int64) error {
	// 1. 删除Redis缓存
	conn := cache.GetRedis()
	defer conn.Close()

	cacheKey := gim.getIndexCacheKey(indexKey, indexValue)
	if _, err := conn.Do("DEL", cacheKey); err != nil {
		hlog.Warnf("Failed to delete index from cache: %v", err)
	}

	// 2. 删除MySQL记录
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	result := db.Where("index_key = ? AND index_value = ?", indexKey, indexValue).Delete(&GlobalIndexTable{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete index from MySQL: %w", result.Error)
	}

	hlog.CtxInfof(ctx, "Deleted global index: %s=%d", indexKey, indexValue)
	return nil
}

// BatchCreateIndex 批量创建索引
func (gim *GlobalIndexManager) BatchCreateIndex(ctx context.Context, entries []IndexEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// 批量写入缓存
	conn := cache.GetRedis()
	defer conn.Close()

	for _, entry := range entries {
		cacheKey := gim.getIndexCacheKey(entry.IndexKey, entry.IndexValue)
		data, _ := json.Marshal(entry.ShardInfo)
		conn.Do("SET", cacheKey, string(data), "EX", 7*24*3600)
	}

	// 批量写入MySQL
	go func() {
		dbs := gim.shardingManager.GetAllDatabases()
		if len(dbs) == 0 {
			return
		}

		var db *gorm.DB
		for _, d := range dbs {
			db = d
			break
		}

		indexTables := make([]GlobalIndexTable, len(entries))
		for i, entry := range entries {
			indexTables[i] = GlobalIndexTable{
				IndexKey:   entry.IndexKey,
				IndexValue: entry.IndexValue,
				DBIndex:    entry.ShardInfo.DBIndex,
				TableIndex: entry.ShardInfo.TableIndex,
				ShardKey:   entry.ShardInfo.ShardKey,
			}
		}

		if err := db.CreateInBatches(indexTables, 100).Error; err != nil {
			hlog.Errorf("Failed to batch create indexes: %v", err)
		}
	}()

	return nil
}

// GetIndexStats 获取索引统计信息
func (gim *GlobalIndexManager) GetIndexStats(ctx context.Context) (map[string]int64, error) {
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return nil, fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	var results []struct {
		IndexKey string
		Count    int64
	}

	if err := db.Model(&GlobalIndexTable{}).
		Select("index_key, COUNT(*) as count").
		Group("index_key").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get index stats: %w", err)
	}

	stats := make(map[string]int64)
	for _, r := range results {
		stats[r.IndexKey] = r.Count
	}

	return stats, nil
}

// RebuildIndex 重建索引（从分片表重建）
func (gim *GlobalIndexManager) RebuildIndex(ctx context.Context, indexKey string, getShardKeyFunc func(row interface{}) (int64, int64)) error {
	hlog.Infof("Starting index rebuild for: %s", indexKey)

	dbs := gim.shardingManager.GetAllDatabases()
	tableCount := gim.shardingManager.config.TableCount

	var wg sync.WaitGroup
	entries := make(chan IndexEntry, 1000)

	// 从所有分片收集数据
	for dbKey, db := range dbs {
		for tableIdx := 0; tableIdx < tableCount; tableIdx++ {
			wg.Add(1)
			go func(dbK string, d *gorm.DB, tIdx int) {
				defer wg.Done()
				tableName := fmt.Sprintf("comments_%d", tIdx)

				// 提取DB索引
				var dbIdx int
				fmt.Sscanf(dbK, "db_%d", &dbIdx)

				// 这里需要根据具体表结构查询
				// 示例：查询评论表的 comment_id 和 video_id
				var rows []map[string]interface{}
				if err := d.Table(tableName).Find(&rows).Error; err != nil {
					hlog.Warnf("Failed to scan table %s.%s: %v", dbK, tableName, err)
					return
				}

				for _, row := range rows {
					indexValue, shardKey := getShardKeyFunc(row)
					entries <- IndexEntry{
						IndexKey:   indexKey,
						IndexValue: indexValue,
						ShardInfo: ShardInfo{
							DBIndex:    dbIdx,
							TableIndex: tIdx,
							ShardKey:   shardKey,
						},
					}
				}
			}(dbKey, db, tableIdx)
		}
	}

	// 启动收集协程
	go func() {
		wg.Wait()
		close(entries)
	}()

	// 批量写入索引
	batch := make([]IndexEntry, 0, 100)
	for entry := range entries {
		batch = append(batch, entry)
		if len(batch) >= 100 {
			gim.BatchCreateIndex(ctx, batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		gim.BatchCreateIndex(ctx, batch)
	}

	hlog.Infof("Index rebuild completed for: %s", indexKey)
	return nil
}

// IndexKeyConstants 索引键常量
const (
	IndexKeyCommentID = "comment_id"
	IndexKeyUserID    = "user_id"
	IndexKeyVideoID   = "video_id"
)

// CreateCommentIndex 创建评论索引
func (gim *GlobalIndexManager) CreateCommentIndex(ctx context.Context, commentID int64, videoID int64) error {
	return gim.CreateIndex(ctx, IndexKeyCommentID, commentID, videoID)
}

// GetCommentShardInfo 获取评论分片信息
func (gim *GlobalIndexManager) GetCommentShardInfo(ctx context.Context, commentID int64) (*ShardInfo, error) {
	return gim.GetShardInfo(ctx, IndexKeyCommentID, commentID)
}

// WarmupCache 预热缓存
func (gim *GlobalIndexManager) WarmupCache(ctx context.Context, indexKey string, limit int) error {
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	var entries []GlobalIndexTable
	if err := db.Where("index_key = ?", indexKey).Limit(limit).Find(&entries).Error; err != nil {
		return fmt.Errorf("failed to load indexes: %w", err)
	}

	conn := cache.GetRedis()
	defer conn.Close()

	for _, entry := range entries {
		cacheKey := gim.getIndexCacheKey(entry.IndexKey, entry.IndexValue)
		shardInfo := ShardInfo{
			DBIndex:    entry.DBIndex,
			TableIndex: entry.TableIndex,
			ShardKey:   entry.ShardKey,
		}
		data, _ := json.Marshal(shardInfo)
		conn.Do("SET", cacheKey, string(data), "EX", 7*24*3600)
	}

	hlog.Infof("Warmed up %d cache entries for index key: %s", len(entries), indexKey)
	return nil
}

// Migrate 创建全局索引表
func (gim *GlobalIndexManager) Migrate(ctx context.Context) error {
	dbs := gim.shardingManager.GetAllDatabases()
	if len(dbs) == 0 {
		return fmt.Errorf("no database available")
	}

	var db *gorm.DB
	for _, d := range dbs {
		db = d
		break
	}

	// 创建全局索引表
	if err := db.AutoMigrate(&GlobalIndexTable{}); err != nil {
		return fmt.Errorf("failed to migrate global index table: %w", err)
	}

	// 创建复合索引
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_global_index_key_value
		ON global_index(index_key, index_value)
	`).Error; err != nil {
		hlog.Warnf("Failed to create composite index: %v", err)
	}

	hlog.Info("Global index table migrated successfully")
	return nil
}
