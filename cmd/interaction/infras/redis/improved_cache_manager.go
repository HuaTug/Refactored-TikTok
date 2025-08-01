package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis"
)

// ImprovedCacheManager 改进的缓存管理器
type ImprovedCacheManager struct {
	client        *redis.Client
	defaultExpiry time.Duration
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Value     interface{} `json:"value"`
	Version   int64       `json:"version"`
	Timestamp int64       `json:"timestamp"`
}

// NewImprovedCacheManager 创建改进的缓存管理器
func NewImprovedCacheManager(client *redis.Client) *ImprovedCacheManager {
	return &ImprovedCacheManager{
		client:        client,
		defaultExpiry: 24 * time.Hour,
	}
}

// SetWithVersion 带版本控制的设置缓存
func (icm *ImprovedCacheManager) SetWithVersion(ctx context.Context, key string, value interface{}, version int64, expiry time.Duration) error {
	entry := CacheEntry{
		Value:     value,
		Version:   version,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	if expiry == 0 {
		expiry = icm.defaultExpiry
	}

	return icm.client.Set(key, data, expiry).Err()
}

// GetWithVersion 带版本控制的获取缓存
func (icm *ImprovedCacheManager) GetWithVersion(ctx context.Context, key string) (*CacheEntry, error) {
	data, err := icm.client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	return &entry, nil
}

// AtomicIncrement 原子递增操作
func (icm *ImprovedCacheManager) AtomicIncrement(ctx context.Context, key string, delta int64) (int64, error) {
	script := `
		local current = redis.call('GET', KEYS[1])
		if current == false then
			current = 0
		else
			current = tonumber(current)
		end
		local new_value = current + ARGV[1]
		redis.call('SET', KEYS[1], new_value)
		redis.call('EXPIRE', KEYS[1], ARGV[2])
		return new_value
	`

	result, err := icm.client.Eval(script, []string{key}, delta, int64(icm.defaultExpiry.Seconds())).Result()
	if err != nil {
		return 0, err
	}

	return result.(int64), nil
}

// AtomicDecrement 原子递减操作
func (icm *ImprovedCacheManager) AtomicDecrement(ctx context.Context, key string, delta int64) (int64, error) {
	return icm.AtomicIncrement(ctx, key, -delta)
}

// BatchSet 批量设置缓存
func (icm *ImprovedCacheManager) BatchSet(ctx context.Context, entries map[string]interface{}, expiry time.Duration) error {
	pipe := icm.client.TxPipeline()

	for key, value := range entries {
		entry := CacheEntry{
			Value:     value,
			Version:   time.Now().UnixNano(),
			Timestamp: time.Now().Unix(),
		}

		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("failed to marshal entry for key %s: %w", key, err)
		}

		if expiry == 0 {
			expiry = icm.defaultExpiry
		}

		pipe.Set(key, data, expiry)
	}

	_, err := pipe.Exec()
	return err
}

// BatchGet 批量获取缓存
func (icm *ImprovedCacheManager) BatchGet(ctx context.Context, keys []string) (map[string]*CacheEntry, error) {
	pipe := icm.client.Pipeline()

	cmds := make(map[string]*redis.StringCmd)
	for _, key := range keys {
		cmds[key] = pipe.Get(key)
	}

	_, err := pipe.Exec()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	results := make(map[string]*CacheEntry)
	for key, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				results[key] = nil
				continue
			}
			return nil, err
		}

		var entry CacheEntry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			hlog.Errorf("Failed to unmarshal cache entry for key %s: %v", key, err)
			results[key] = nil
			continue
		}

		results[key] = &entry
	}

	return results, nil
}

// InvalidatePattern 按模式失效缓存
func (icm *ImprovedCacheManager) InvalidatePattern(ctx context.Context, pattern string) error {
	keys, err := icm.client.Keys(pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	return icm.client.Del(keys...).Err()
}

// SetVideoLikeCountWithConsistency 设置视频点赞数（带一致性保证）
func (icm *ImprovedCacheManager) SetVideoLikeCountWithConsistency(ctx context.Context, videoID int64, count int64, version int64) error {
	key := fmt.Sprintf("video_like_count:%d", videoID)
	return icm.SetWithVersion(ctx, key, count, version, time.Hour)
}

// GetVideoLikeCountWithConsistency 获取视频点赞数（带一致性保证）
func (icm *ImprovedCacheManager) GetVideoLikeCountWithConsistency(ctx context.Context, videoID int64) (int64, int64, error) {
	key := fmt.Sprintf("video_like_count:%d", videoID)
	entry, err := icm.GetWithVersion(ctx, key)
	if err != nil {
		return 0, 0, err
	}

	if entry == nil {
		return 0, 0, nil
	}

	count, ok := entry.Value.(float64) // JSON unmarshaling converts numbers to float64
	if !ok {
		return 0, 0, fmt.Errorf("invalid count type in cache")
	}

	return int64(count), entry.Version, nil
}

// IncrVideoLikeCountWithConsistency 递增视频点赞数（带一致性保证）
func (icm *ImprovedCacheManager) IncrVideoLikeCountWithConsistency(ctx context.Context, videoID int64) (int64, error) {
	key := fmt.Sprintf("video_like_count:%d", videoID)
	return icm.AtomicIncrement(ctx, key, 1)
}

// DecrVideoLikeCountWithConsistency 递减视频点赞数（带一致性保证）
func (icm *ImprovedCacheManager) DecrVideoLikeCountWithConsistency(ctx context.Context, videoID int64) (int64, error) {
	key := fmt.Sprintf("video_like_count:%d", videoID)
	return icm.AtomicDecrement(ctx, key, 1)
}

// SetCommentLikeCountWithConsistency 设置评论点赞数（带一致性保证）
func (icm *ImprovedCacheManager) SetCommentLikeCountWithConsistency(ctx context.Context, commentID int64, count int64, version int64) error {
	key := fmt.Sprintf("comment_like_count:%d", commentID)
	return icm.SetWithVersion(ctx, key, count, version, time.Hour)
}

// GetCommentLikeCountWithConsistency 获取评论点赞数（带一致性保证）
func (icm *ImprovedCacheManager) GetCommentLikeCountWithConsistency(ctx context.Context, commentID int64) (int64, int64, error) {
	key := fmt.Sprintf("comment_like_count:%d", commentID)
	entry, err := icm.GetWithVersion(ctx, key)
	if err != nil {
		return 0, 0, err
	}

	if entry == nil {
		return 0, 0, nil
	}

	count, ok := entry.Value.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("invalid count type in cache")
	}

	return int64(count), entry.Version, nil
}

// IncrCommentLikeCountWithConsistency 递增评论点赞数（带一致性保证）
func (icm *ImprovedCacheManager) IncrCommentLikeCountWithConsistency(ctx context.Context, commentID int64) (int64, error) {
	key := fmt.Sprintf("comment_like_count:%d", commentID)
	return icm.AtomicIncrement(ctx, key, 1)
}

// DecrCommentLikeCountWithConsistency 递减评论点赞数（带一致性保证）
func (icm *ImprovedCacheManager) DecrCommentLikeCountWithConsistency(ctx context.Context, commentID int64) (int64, error) {
	key := fmt.Sprintf("comment_like_count:%d", commentID)
	return icm.AtomicDecrement(ctx, key, 1)
}

// SetUserLikeStatus 设置用户点赞状态
func (icm *ImprovedCacheManager) SetUserLikeStatus(ctx context.Context, userID, resourceID int64, resourceType string, liked bool) error {
	key := fmt.Sprintf("user_like_status:%s:%d:%d", resourceType, userID, resourceID)
	status := "0"
	if liked {
		status = "1"
	}
	return icm.client.Set(key, status, time.Hour).Err()
}

// GetUserLikeStatus 获取用户点赞状态
func (icm *ImprovedCacheManager) GetUserLikeStatus(ctx context.Context, userID, resourceID int64, resourceType string) (bool, error) {
	key := fmt.Sprintf("user_like_status:%s:%d:%d", resourceType, userID, resourceID)
	result, err := icm.client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}

	return result == "1", nil
}

// BatchSetUserLikeStatus 批量设置用户点赞状态
func (icm *ImprovedCacheManager) BatchSetUserLikeStatus(ctx context.Context, statuses map[string]bool, resourceType string) error {
	pipe := icm.client.TxPipeline()

	for statusKey, liked := range statuses {
		key := fmt.Sprintf("user_like_status:%s:%s", resourceType, statusKey)
		status := "0"
		if liked {
			status = "1"
		}
		pipe.Set(key, status, time.Hour)
	}

	_, err := pipe.Exec()
	return err
}

// RefreshCache 刷新缓存
func (icm *ImprovedCacheManager) RefreshCache(ctx context.Context, key string, refreshFunc func() (interface{}, error)) error {
	// 使用分布式锁防止缓存击穿
	lockKey := fmt.Sprintf("refresh_lock:%s", key)
	lockScript := `
		if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'EX', ARGV[2]) then
			return 1
		else
			return 0
		end
	`

	lockValue := fmt.Sprintf("%d", time.Now().UnixNano())
	locked, err := icm.client.Eval(lockScript, []string{lockKey}, lockValue, 30).Result()
	if err != nil {
		return err
	}

	if locked.(int64) == 0 {
		// 获取锁失败，等待其他进程刷新
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	defer func() {
		// 释放锁
		unlockScript := `
			if redis.call('GET', KEYS[1]) == ARGV[1] then
				return redis.call('DEL', KEYS[1])
			else
				return 0
			end
		`
		icm.client.Eval(unlockScript, []string{lockKey}, lockValue)
	}()

	// 刷新数据
	value, err := refreshFunc()
	if err != nil {
		return err
	}

	// 更新缓存
	return icm.SetWithVersion(ctx, key, value, time.Now().UnixNano(), icm.defaultExpiry)
}

// HealthCheck 健康检查
func (icm *ImprovedCacheManager) HealthCheck(ctx context.Context) error {
	return icm.client.Ping().Err()
}

// GetCacheStats 获取缓存统计信息
func (icm *ImprovedCacheManager) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	info, err := icm.client.Info("memory").Result()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	stats["memory_info"] = info

	// 获取键数量
	dbSize, err := icm.client.DBSize().Result()
	if err != nil {
		return nil, err
	}
	stats["total_keys"] = dbSize

	return stats, nil
}

// WarmupCache 预热缓存
func (icm *ImprovedCacheManager) WarmupCache(ctx context.Context, warmupFunc func(context.Context) error) error {
	hlog.Info("Starting cache warmup...")
	start := time.Now()

	err := warmupFunc(ctx)
	if err != nil {
		hlog.Errorf("Cache warmup failed: %v", err)
		return err
	}

	duration := time.Since(start)
	hlog.Infof("Cache warmup completed in %v", duration)
	return nil
}
