package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
)

// Business types for like system
const (
	BusinessTypeVideo   = 1 // 视频业务
	BusinessTypeComment = 2 // 评论业务
)

// Redis key templates following the new design pattern
const (
	// 计数缓存 Key：count:{business_id}:{message_id}
	// 存储该内容的点赞数(like_count)和点踩数(dislike_count)
	CountCacheKeyTemplate = "count:%d:%d"

	// 用户点赞列表 Key：user:likes:{mid}:{business_id}
	// ZSet结构存储用户在该业务下点赞过的内容ID和点赞时间
	UserLikesKeyTemplate = "user:likes:%d:%d"

	// 内容点赞用户列表 Key：content:likes:{business_id}:{message_id}
	// ZSet结构存储点赞该内容的用户ID和点赞时间
	ContentLikesKeyTemplate = "content:likes:%d:%d"
)

// LikeCacheManager 新版点赞缓存管理器
type LikeCacheManager struct {
	client     redis.Cmdable
	defaultTTL time.Duration
	// 新增并发安全设置
	lockTimeout time.Duration
}

// NewLikeCacheManager 创建新版点赞缓存管理器
func NewLikeCacheManager(client redis.Cmdable) *LikeCacheManager {
	return &LikeCacheManager{
		client:      client,
		defaultTTL:  24 * time.Hour,  // 默认缓存时间为24小时
		lockTimeout: 5 * time.Second, // 锁超时时间为5秒
	}
}

// CountCache 计数缓存结构
type CountCache struct {
	LikeCount    int64 `json:"like_count"`
	DislikeCount int64 `json:"dislike_count"`
}

// 设置乐观锁
func (lcm *LikeCacheManager) SetWithVersion(ctx context.Context, key string, value interface{}, version int64, ttl time.Duration) error {
	entry := struct {
		Value     interface{} `json:"value"`
		Version   int64       `json:"version"`
		Timestamp int64       `json:"timestamp"`
	}{
		Value:     value,
		Version:   version,
		Timestamp: time.Now().Unix(),
	}

	b, _ := json.Marshal(entry)
	if ttl == 0 {
		ttl = lcm.defaultTTL
	}
	return lcm.client.Set(key, b, ttl).Err()
}

func (lcm *LikeCacheManager) GetWithVersion(ctx context.Context, key string) (interface{}, int64, error) {
	val, err := lcm.client.Get(key).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get value with version: %w", err)
	}

	var entry struct {
		Value     interface{} `json:"value"`
		Version   int64       `json:"version"`
		Timestamp int64       `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal value with version: %w", err)
	}

	return entry.Value, entry.Version, nil
}

func (lcm *LikeCacheManager) AtomicIncrement(ctx context.Context, key string, delta int64) (int64, error) {
	lua := `
		local v = redis.call('GET',KEYS[1])
		v = v and tonumber(v) or 0
		v = v + ARGV[1]
		redis.call('SET',KEYS[1],v,'EX',ARGV[2])
		return v
	`

	res, err := lcm.client.Eval(lua, []string{key}, delta, int64(lcm.defaultTTL.Seconds())).Result()
	if err != nil {
		return 0, err
	}
	return res.(int64), nil
}

func (lcm *LikeCacheManager) AtomicDecrement(ctx context.Context, key string, delta int64) (int64, error) {
	return lcm.AtomicIncrement(ctx, key, -delta)
}

func (lcm *LikeCacheManager) BatchSet(ctx context.Context, kv map[string]interface{}, ttl time.Duration) error {
	pipe := lcm.client.TxPipeline()
	for k, v := range kv {
		lcm.SetWithVersion(ctx, k, v, time.Now().UnixNano(), ttl)
	}

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to batch set values: %w", err)
	}
	return nil
}

func (lcm *LikeCacheManager) RefreshCache(ctx context.Context, key string, refreshFn func() (interface{}, error)) error {
	lockKey := "refresh_lock:" + key
	lockVal := fmt.Sprintf("%d", time.Now().UnixNano())
	ok, _ := lcm.client.SetNX(lockKey, lockVal, lcm.lockTimeout).Result()
	if !ok {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	defer lcm.client.Eval(`
		if redis.call('GET',KEYS[1]) == ARGV[1] then
			redis.call('DEL',KEYS[1])
		end
	`, []string{lockKey}, lockVal)

	val, err := refreshFn()
	if err != nil {
		return fmt.Errorf("failed to refresh cache: %w", err)
	}
	return lcm.SetWithVersion(ctx, key, val, time.Now().UnixNano(), lcm.defaultTTL)
}

func (lcm *LikeCacheManager) HealthCheck(ctx context.Context) error {
	return lcm.client.Ping().Err()
}

func (lcm *LikeCacheManager) GetCacheStats(ctx context.Context) (map[string]interface{}, error) {
	info, _ := lcm.client.Info("memory").Result()
	size, _ := lcm.client.DBSize().Result()
	return map[string]interface{}{
		"memory_info": info,
		"total_keys":  size,
	}, nil
}

func (lcm *LikeCacheManager) ParseEntry(cmd *redis.StringCmd) (interface{}, int64, error) {
	str, err := cmd.Result()
	if err != nil {
		return nil, 0, err
	}
	var e struct {
		Value   interface{} `json:"value"`
		Version int64       `json:"version"`
	}
	_ = json.Unmarshal([]byte(str), &e)
	return e.Value, e.Version, nil
}

// === 计数缓存相关操作 ===

// GetCountCache 获取内容的点赞/点踩计数
func (lcm *LikeCacheManager) GetCountCache(ctx context.Context, businessID, messageID int64) (*CountCache, error) {
	key := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)

	result, err := lcm.client.HMGet(key, "like_count", "dislike_count").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get count cache: %w", err)
	}

	count := &CountCache{}

	// 解析like_count
	if result[0] != nil {
		if likeStr, ok := result[0].(string); ok {
			if likeCount, err := strconv.ParseInt(likeStr, 10, 64); err == nil {
				count.LikeCount = likeCount
			}
		}
	}

	// 解析dislike_count
	if result[1] != nil {
		if dislikeStr, ok := result[1].(string); ok {
			if dislikeCount, err := strconv.ParseInt(dislikeStr, 10, 64); err == nil {
				count.DislikeCount = dislikeCount
			}
		}
	}
	return count, nil
}

// SetCountCache 设置内容的点赞/点踩计数
func (lcm *LikeCacheManager) SetCountCache(ctx context.Context, businessID, messageID int64, count *CountCache) error {
	key := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)

	pipe := lcm.client.TxPipeline()
	pipe.HMSet(key, map[string]interface{}{
		"like_count":    count.LikeCount,
		"dislike_count": count.DislikeCount,
	})
	pipe.Expire(key, lcm.defaultTTL)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to set count cache: %w", err)
	}
	return nil
}

// IncrementLikeCount 增加点赞数
func (lcm *LikeCacheManager) IncrementLikeCount(ctx context.Context, businessID, messageID int64, delta int64) error {
	key := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)

	pipe := lcm.client.TxPipeline()
	pipe.HIncrBy(key, "like_count", delta)
	pipe.Expire(key, lcm.defaultTTL)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to increment like count: %w", err)
	}

	return nil
}

// IncrementDislikeCount 增加点踩数
func (lcm *LikeCacheManager) IncrementDislikeCount(ctx context.Context, businessID, messageID int64, delta int64) error {
	key := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)

	pipe := lcm.client.TxPipeline()
	pipe.HIncrBy(key, "dislike_count", delta)
	pipe.Expire(key, lcm.defaultTTL)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to increment dislike count: %w", err)
	}

	return nil
}

// === 用户点赞列表相关操作 ===

// AddUserLike 添加用户点赞记录
func (lcm *LikeCacheManager) AddUserLike(ctx context.Context, userID, businessID, messageID int64) error {
	userLikesKey := fmt.Sprintf(UserLikesKeyTemplate, userID, businessID)
	contentLikesKey := fmt.Sprintf(ContentLikesKeyTemplate, businessID, messageID)
	timestamp := float64(time.Now().Unix())

	pipe := lcm.client.TxPipeline()

	// 添加到用户点赞列表
	pipe.ZAdd(userLikesKey, redis.Z{
		Score:  timestamp,
		Member: messageID,
	})
	pipe.Expire(userLikesKey, lcm.defaultTTL)

	// 添加到内容点赞用户列表
	pipe.ZAdd(contentLikesKey, redis.Z{
		Score:  timestamp,
		Member: userID,
	})
	pipe.Expire(contentLikesKey, lcm.defaultTTL)

	// 增加点赞计数
	countKey := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)
	pipe.HIncrBy(countKey, "like_count", 1)
	pipe.Expire(countKey, lcm.defaultTTL)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to add user like: %w", err)
	}

	return nil
}

// RemoveUserLike 移除用户点赞记录
func (lcm *LikeCacheManager) RemoveUserLike(ctx context.Context, userID, businessID, messageID int64) error {
	userLikesKey := fmt.Sprintf(UserLikesKeyTemplate, userID, businessID)
	contentLikesKey := fmt.Sprintf(ContentLikesKeyTemplate, businessID, messageID)

	pipe := lcm.client.TxPipeline()

	// 从用户点赞列表移除
	pipe.ZRem(userLikesKey, messageID)

	// 从内容点赞用户列表移除
	pipe.ZRem(contentLikesKey, userID)

	// 减少点赞计数
	countKey := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)
	pipe.HIncrBy(countKey, "like_count", -1)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to remove user like: %w", err)
	}

	return nil
}

// IsUserLiked 检查用户是否点赞了某内容
func (lcm *LikeCacheManager) IsUserLiked(ctx context.Context, userID, businessID, messageID int64) (bool, error) {
	key := fmt.Sprintf(UserLikesKeyTemplate, userID, businessID)

	score, err := lcm.client.ZScore(key, strconv.FormatInt(messageID, 10)).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil // 未点赞
		}
		return false, fmt.Errorf("failed to check user like status: %w", err)
	}

	return score > 0, nil
}

// GetUserLikeHistory 获取用户点赞历史（分页）
func (lcm *LikeCacheManager) GetUserLikeHistory(ctx context.Context, userID, businessID int64, offset, limit int64) ([]int64, error) {
	key := fmt.Sprintf(UserLikesKeyTemplate, userID, businessID)

	// 按时间倒序获取（最新的在前）
	result, err := lcm.client.ZRevRange(key, offset, offset+limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user like history: %w", err)
	}

	messageIDs := make([]int64, 0, len(result))
	for _, item := range result {
		if messageID, err := strconv.ParseInt(item, 10, 64); err == nil {
			messageIDs = append(messageIDs, messageID)
		}
	}

	return messageIDs, nil
}

// GetContentLikeUsers 获取点赞某内容的用户列表（分页）
func (lcm *LikeCacheManager) GetContentLikeUsers(ctx context.Context, businessID, messageID int64, offset, limit int64) ([]int64, error) {
	key := fmt.Sprintf(ContentLikesKeyTemplate, businessID, messageID)

	// 按时间倒序获取（最新的在前）
	result, err := lcm.client.ZRevRange(key, offset, offset+limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get content like users: %w", err)
	}

	userIDs := make([]int64, 0, len(result))
	for _, item := range result {
		if userID, err := strconv.ParseInt(item, 10, 64); err == nil {
			userIDs = append(userIDs, userID)
		}
	}

	return userIDs, nil
}

// === 批量操作 ===

// BatchGetCountCache 批量获取计数缓存
func (lcm *LikeCacheManager) BatchGetCountCache(ctx context.Context, businessID int64, messageIDs []int64) (map[int64]*CountCache, error) {
	if len(messageIDs) == 0 {
		return make(map[int64]*CountCache), nil
	}

	pipe := lcm.client.Pipeline()

	// 批量获取所有key的数据
	cmds := make(map[int64]*redis.SliceCmd)
	for _, messageID := range messageIDs {
		key := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)
		cmds[messageID] = pipe.HMGet(key, "like_count", "dislike_count")
	}

	_, err := pipe.Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to batch get count cache: %w", err)
	}

	// 解析结果
	result := make(map[int64]*CountCache)
	for messageID, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil {
			continue // 跳过错误的key
		}

		count := &CountCache{}

		// 解析like_count
		if values[0] != nil {
			if likeStr, ok := values[0].(string); ok {
				if likeCount, err := strconv.ParseInt(likeStr, 10, 64); err == nil {
					count.LikeCount = likeCount
				}
			}
		}

		// 解析dislike_count
		if values[1] != nil {
			if dislikeStr, ok := values[1].(string); ok {
				if dislikeCount, err := strconv.ParseInt(dislikeStr, 10, 64); err == nil {
					count.DislikeCount = dislikeCount
				}
			}
		}

		result[messageID] = count
	}

	return result, nil
}

// BatchCheckUserLikes 批量检查用户点赞状态
func (lcm *LikeCacheManager) BatchCheckUserLikes(ctx context.Context, userID, businessID int64, messageIDs []int64) (map[int64]bool, error) {
	if len(messageIDs) == 0 {
		return make(map[int64]bool), nil
	}

	key := fmt.Sprintf(UserLikesKeyTemplate, userID, businessID)

	pipe := lcm.client.Pipeline()
	cmds := make(map[int64]*redis.FloatCmd)

	for _, messageID := range messageIDs {
		cmds[messageID] = pipe.ZScore(key, strconv.FormatInt(messageID, 10))
	}

	_, err := pipe.Exec()
	if err != nil {
		return nil, fmt.Errorf("failed to batch check user likes: %w", err)
	}

	result := make(map[int64]bool)
	for messageID, cmd := range cmds {
		_, err := cmd.Result()
		result[messageID] = err == nil // 如果没有错误，说明存在（已点赞）
	}

	return result, nil
}

// === 清理操作 ===

// DeleteContentLikeCache 删除内容相关的所有点赞缓存
func (lcm *LikeCacheManager) DeleteContentLikeCache(ctx context.Context, businessID, messageID int64) error {
	countKey := fmt.Sprintf(CountCacheKeyTemplate, businessID, messageID)
	contentLikesKey := fmt.Sprintf(ContentLikesKeyTemplate, businessID, messageID)

	pipe := lcm.client.TxPipeline()
	pipe.Del(countKey)
	pipe.Del(contentLikesKey)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to delete content like cache: %w", err)
	}

	return nil
}

// === 便捷方法 ===

// GetVideoLikeCount 获取视频点赞数（便捷方法）
func (lcm *LikeCacheManager) GetVideoLikeCount(ctx context.Context, videoID int64) (int64, error) {
	count, err := lcm.GetCountCache(ctx, BusinessTypeVideo, videoID)
	if err != nil {
		return 0, err
	}
	return count.LikeCount, nil
}

// GetCommentLikeCount 获取评论点赞数（便捷方法）
func (lcm *LikeCacheManager) GetCommentLikeCount(ctx context.Context, commentID int64) (int64, error) {
	count, err := lcm.GetCountCache(ctx, BusinessTypeComment, commentID)
	if err != nil {
		return 0, err
	}
	return count.LikeCount, nil
}

// IsVideoLikedByUser 检查用户是否点赞了视频（便捷方法）
func (lcm *LikeCacheManager) IsVideoLikedByUser(ctx context.Context, userID, videoID int64) (bool, error) {
	return lcm.IsUserLiked(ctx, userID, BusinessTypeVideo, videoID)
}

// IsCommentLikedByUser 检查用户是否点赞了评论（便捷方法）
func (lcm *LikeCacheManager) IsCommentLikedByUser(ctx context.Context, userID, commentID int64) (bool, error) {
	return lcm.IsUserLiked(ctx, userID, BusinessTypeComment, commentID)
}

// === 带一致性检查的方法 ===

// SetVideoLikeCountWithConsistency 设置视频点赞数（带版本一致性检查）
func (lcm *LikeCacheManager) SetVideoLikeCountWithConsistenc(ctx context.Context, videoID int64, count int64, version int64) error {
	countCache := &CountCache{
		LikeCount:    count,
		DislikeCount: 0, // 如果需要保留原有的踩数，可以先获取
	}

	// 首先尝试获取现有的数据以保留DislikeCount
	existingCount, err := lcm.GetCountCache(ctx, BusinessTypeVideo, videoID)
	if err == nil && existingCount != nil {
		countCache.DislikeCount = existingCount.DislikeCount
	}

	key := fmt.Sprintf(CountCacheKeyTemplate, BusinessTypeVideo, videoID)
	return lcm.SetWithVersion(ctx, key, countCache, version, lcm.defaultTTL)
}

// SetCommentLikeCountWithConsistency 设置评论点赞数（带版本一致性检查）
func (lcm *LikeCacheManager) SetCommentLikeCountWithConsistenc(ctx context.Context, commentID int64, count int64, version int64) error {
	countCache := &CountCache{
		LikeCount:    count,
		DislikeCount: 0, // 如果需要保留原有的踩数，可以先获取
	}

	// 首先尝试获取现有的数据以保留DislikeCount
	existingCount, err := lcm.GetCountCache(ctx, BusinessTypeComment, commentID)
	if err == nil && existingCount != nil {
		countCache.DislikeCount = existingCount.DislikeCount
	}

	key := fmt.Sprintf(CountCacheKeyTemplate, BusinessTypeComment, commentID)
	return lcm.SetWithVersion(ctx, key, countCache, version, lcm.defaultTTL)
}
