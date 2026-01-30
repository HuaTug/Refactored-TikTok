package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
)

// =============================================================================
// 抖音级点赞评论系统 - Redis 架构设计
// =============================================================================
//
// 设计目标：
// 1. 高并发：支持百万级QPS的点赞操作
// 2. 低延迟：点赞响应 < 50ms
// 3. 数据一致性：采用最终一致性策略，异步同步到DB
// 4. 防刷保护：多维度限流 + 行为检测
//
// 核心架构：
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                           Client Request                                │
// └─────────────────────────┬───────────────────────────────────────────────┘
//                           ▼
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                      Rate Limiter (Lua Script)                          │
// │              滑动窗口限流 + 用户级 + IP级 + 全局级                        │
// └─────────────────────────┬───────────────────────────────────────────────┘
//                           ▼
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                    Distributed Lock (RedLock)                           │
// │              防止重复点赞/并发冲突，锁粒度：user:video                    │
// └─────────────────────────┬───────────────────────────────────────────────┘
//                           ▼
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                    Redis Write (Pipeline/Lua)                           │
// │              原子更新：用户点赞集合 + 计数器 + 时间线                      │
// └─────────────────────────┬───────────────────────────────────────────────┘
//                           ▼
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                    Async Queue (Kafka/RabbitMQ)                         │
// │              异步持久化到MySQL，支持失败重试                              │
// └─────────────────────────────────────────────────────────────────────────┘
//
// =============================================================================

// Redis Key设计规范 (仿B站/抖音架构)
const (
	// ========== 点赞相关 ==========
	// 用户点赞记录 (ZSet) - 用于查询用户点赞列表
	// Key: like:user:{user_id}:{biz_type}
	// Score: 点赞时间戳, Member: 资源ID
	LikeUserSetKey = "like:user:%d:%d"

	// 资源点赞用户 (ZSet) - 用于查询谁点赞了这个视频
	// Key: like:obj:{biz_type}:{obj_id}
	// Score: 点赞时间戳, Member: 用户ID
	LikeObjSetKey = "like:obj:%d:%d"

	// 点赞计数器 (Hash) - 用于快速获取点赞数
	// Key: like:count:{biz_type}
	// Field: obj_id, Value: count
	LikeCountHashKey = "like:count:%d"

	// 点赞状态位图 (Bitmap) - 用于快速判断是否点赞
	// Key: like:bitmap:{biz_type}:{obj_id_shard}
	// 使用分片减少单个bitmap大小
	LikeBitmapKey = "like:bitmap:%d:%d"

	// ========== 评论相关 ==========
	// 评论计数器
	CommentCountHashKey = "comment:count"
	// 评论热度排行 (ZSet)
	CommentHotRankKey = "comment:hot:%d" // video_id
	// 评论时间线 (ZSet)
	CommentTimelineKey = "comment:timeline:%d" // video_id

	// ========== 限流相关 ==========
	// 用户限流计数器 (滑动窗口)
	RateLimitUserKey = "ratelimit:user:%d:%s"
	// IP限流计数器
	RateLimitIPKey = "ratelimit:ip:%s:%s"
	// 全局限流计数器
	RateLimitGlobalKey = "ratelimit:global:%s"

	// ========== 分布式锁 ==========
	// 点赞操作锁
	LikeLockKey = "lock:like:%d:%d:%d" // biz_type:user_id:obj_id
	// 评论操作锁
	CommentLockKey = "lock:comment:%d:%d" // video_id:user_id

	// ========== 异步队列 ==========
	// 待同步队列 (List) - 用于异步持久化
	LikeSyncQueueKey    = "queue:like:sync"
	CommentSyncQueueKey = "queue:comment:sync"

	// ========== 缓存相关 ==========
	// 热点视频缓存预热标记
	HotVideoCacheKey = "cache:hot:video:%d"
	// 用户活跃度标记
	UserActiveKey = "active:user:%d"
)

// 业务类型常量
const (
	BizTypeVideo   = 1 // 视频
	BizTypeComment = 2 // 评论
	BizTypeReply   = 3 // 回复
)

// 限流配置
const (
	// 用户级限流：每秒最多10次点赞
	UserLikeRateLimit = 10
	UserLikeWindow    = time.Second

	// 用户级评论限流：每分钟最多10条
	UserCommentRateLimit = 10
	UserCommentWindow    = time.Minute

	// 分布式锁超时
	LockTimeout = 3 * time.Second
	LockRetry   = 3
)

// EnhancedInteractionManager 增强版交互管理器
type EnhancedInteractionManager struct {
	client      redis.Cmdable
	lockTimeout time.Duration
}

// NewEnhancedInteractionManager 创建增强版交互管理器
func NewEnhancedInteractionManager(client redis.Cmdable) *EnhancedInteractionManager {
	return &EnhancedInteractionManager{
		client:      client,
		lockTimeout: LockTimeout,
	}
}

// =============================================================================
// 分布式锁实现
// =============================================================================

// AcquireLock 获取分布式锁
// 使用 SET NX EX 实现简单的分布式锁
func (m *EnhancedInteractionManager) AcquireLock(ctx context.Context, key string, value string, ttl time.Duration) (bool, error) {
	result, err := m.client.SetNX(key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	return result, nil
}

// ReleaseLock 释放分布式锁
// 使用 Lua 脚本确保只释放自己持有的锁
func (m *EnhancedInteractionManager) ReleaseLock(ctx context.Context, key string, value string) error {
	lua := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	_, err := m.client.Eval(lua, []string{key}, value).Result()
	return err
}

// AcquireLikeLock 获取点赞操作锁
func (m *EnhancedInteractionManager) AcquireLikeLock(ctx context.Context, bizType int, userID, objID int64) (string, bool, error) {
	key := fmt.Sprintf(LikeLockKey, bizType, userID, objID)
	value := fmt.Sprintf("%d:%d", time.Now().UnixNano(), userID)

	for i := 0; i < LockRetry; i++ {
		acquired, err := m.AcquireLock(ctx, key, value, m.lockTimeout)
		if err != nil {
			return "", false, err
		}
		if acquired {
			return value, true, nil
		}
		// 短暂等待后重试
		time.Sleep(50 * time.Millisecond)
	}
	return "", false, nil
}

// ReleaseLikeLock 释放点赞操作锁
func (m *EnhancedInteractionManager) ReleaseLikeLock(ctx context.Context, bizType int, userID, objID int64, value string) error {
	key := fmt.Sprintf(LikeLockKey, bizType, userID, objID)
	return m.ReleaseLock(ctx, key, value)
}

// =============================================================================
// 限流实现 (滑动窗口算法)
// =============================================================================

// RateLimitResult 限流结果
type RateLimitResult struct {
	Allowed   bool  // 是否允许
	Remaining int64 // 剩余次数
	RetryAt   int64 // 下次重试时间 (Unix timestamp)
}

// CheckRateLimit 检查限流 (滑动窗口算法)
// 使用 Lua 脚本实现原子性的滑动窗口限流
func (m *EnhancedInteractionManager) CheckRateLimit(ctx context.Context, key string, limit int64, window time.Duration) (*RateLimitResult, error) {
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()

	// Lua 脚本实现滑动窗口
	lua := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window_ms = tonumber(ARGV[4])

		-- 移除过期的请求记录
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

		-- 获取当前窗口内的请求数
		local current = redis.call('ZCARD', key)

		if current < limit then
			-- 未超限，添加当前请求
			redis.call('ZADD', key, now, now .. ':' .. math.random())
			redis.call('PEXPIRE', key, window_ms)
			return {1, limit - current - 1, 0}
		else
			-- 已超限，返回最早请求的过期时间
			local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
			local retry_at = 0
			if #oldest >= 2 then
				retry_at = tonumber(oldest[2]) + window_ms
			end
			return {0, 0, retry_at}
		end
	`

	result, err := m.client.Eval(lua, []string{key}, now, windowStart, limit, window.Milliseconds()).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to check rate limit: %w", err)
	}

	arr, ok := result.([]interface{})
	if !ok || len(arr) != 3 {
		return nil, fmt.Errorf("unexpected rate limit result format")
	}

	allowed, _ := arr[0].(int64)
	remaining, _ := arr[1].(int64)
	retryAt, _ := arr[2].(int64)

	return &RateLimitResult{
		Allowed:   allowed == 1,
		Remaining: remaining,
		RetryAt:   retryAt,
	}, nil
}

// CheckUserLikeRateLimit 检查用户点赞限流
func (m *EnhancedInteractionManager) CheckUserLikeRateLimit(ctx context.Context, userID int64) (*RateLimitResult, error) {
	key := fmt.Sprintf(RateLimitUserKey, userID, "like")
	return m.CheckRateLimit(ctx, key, UserLikeRateLimit, UserLikeWindow)
}

// CheckUserCommentRateLimit 检查用户评论限流
func (m *EnhancedInteractionManager) CheckUserCommentRateLimit(ctx context.Context, userID int64) (*RateLimitResult, error) {
	key := fmt.Sprintf(RateLimitUserKey, userID, "comment")
	return m.CheckRateLimit(ctx, key, UserCommentRateLimit, UserCommentWindow)
}

// =============================================================================
// 点赞操作 (原子性 + 最终一致性)
// =============================================================================

// LikeAction 点赞操作类型
type LikeAction struct {
	UserID    int64     `json:"user_id"`
	ObjID     int64     `json:"obj_id"`
	BizType   int       `json:"biz_type"`
	Action    string    `json:"action"` // "like" or "unlike"
	Timestamp time.Time `json:"timestamp"`
}

// DoLike 执行点赞操作 (原子性保证)
// 返回：isSuccess, isNewLike (是否是新增点赞), error
func (m *EnhancedInteractionManager) DoLike(ctx context.Context, userID, objID int64, bizType int) (bool, bool, error) {
	// 1. 检查限流
	rateLimitResult, err := m.CheckUserLikeRateLimit(ctx, userID)
	if err != nil {
		return false, false, fmt.Errorf("rate limit check failed: %w", err)
	}
	if !rateLimitResult.Allowed {
		return false, false, fmt.Errorf("rate limit exceeded, retry after %d", rateLimitResult.RetryAt)
	}

	// 2. 获取分布式锁
	lockValue, acquired, err := m.AcquireLikeLock(ctx, bizType, userID, objID)
	if err != nil {
		return false, false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return false, false, fmt.Errorf("failed to acquire lock, please retry")
	}
	defer m.ReleaseLikeLock(ctx, bizType, userID, objID, lockValue)

	// 3. 使用 Lua 脚本原子执行点赞操作
	userSetKey := fmt.Sprintf(LikeUserSetKey, userID, bizType)
	objSetKey := fmt.Sprintf(LikeObjSetKey, bizType, objID)
	countHashKey := fmt.Sprintf(LikeCountHashKey, bizType)
	timestamp := float64(time.Now().Unix())

	lua := `
		local user_set_key = KEYS[1]
		local obj_set_key = KEYS[2]
		local count_hash_key = KEYS[3]
		local user_id = ARGV[1]
		local obj_id = ARGV[2]
		local timestamp = tonumber(ARGV[3])

		-- 检查是否已点赞
		local score = redis.call('ZSCORE', user_set_key, obj_id)
		if score then
			-- 已经点赞，返回 0
			return {0, 0}
		end

		-- 添加到用户点赞集合
		redis.call('ZADD', user_set_key, timestamp, obj_id)

		-- 添加到资源点赞用户集合
		redis.call('ZADD', obj_set_key, timestamp, user_id)

		-- 增加点赞计数
		local new_count = redis.call('HINCRBY', count_hash_key, obj_id, 1)

		-- 设置过期时间 (7天)
		redis.call('EXPIRE', user_set_key, 604800)
		redis.call('EXPIRE', obj_set_key, 604800)

		return {1, new_count}
	`

	result, err := m.client.Eval(lua, []string{userSetKey, objSetKey, countHashKey},
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(objID, 10),
		timestamp).Result()
	if err != nil {
		return false, false, fmt.Errorf("like operation failed: %w", err)
	}

	arr, ok := result.([]interface{})
	if !ok || len(arr) != 2 {
		return false, false, fmt.Errorf("unexpected result format")
	}

	isNewLike, _ := arr[0].(int64)

	// 4. 异步写入持久化队列
	if isNewLike == 1 {
		go m.pushToSyncQueue(ctx, &LikeAction{
			UserID:    userID,
			ObjID:     objID,
			BizType:   bizType,
			Action:    "like",
			Timestamp: time.Now(),
		})
	}

	return true, isNewLike == 1, nil
}

// DoUnlike 执行取消点赞操作 (原子性保证)
func (m *EnhancedInteractionManager) DoUnlike(ctx context.Context, userID, objID int64, bizType int) (bool, error) {
	// 1. 获取分布式锁
	lockValue, acquired, err := m.AcquireLikeLock(ctx, bizType, userID, objID)
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return false, fmt.Errorf("failed to acquire lock, please retry")
	}
	defer m.ReleaseLikeLock(ctx, bizType, userID, objID, lockValue)

	// 2. 使用 Lua 脚本原子执行取消点赞操作
	userSetKey := fmt.Sprintf(LikeUserSetKey, userID, bizType)
	objSetKey := fmt.Sprintf(LikeObjSetKey, bizType, objID)
	countHashKey := fmt.Sprintf(LikeCountHashKey, bizType)

	lua := `
		local user_set_key = KEYS[1]
		local obj_set_key = KEYS[2]
		local count_hash_key = KEYS[3]
		local user_id = ARGV[1]
		local obj_id = ARGV[2]

		-- 检查是否已点赞
		local score = redis.call('ZSCORE', user_set_key, obj_id)
		if not score then
			-- 未点赞，返回 0
			return 0
		end

		-- 从用户点赞集合移除
		redis.call('ZREM', user_set_key, obj_id)

		-- 从资源点赞用户集合移除
		redis.call('ZREM', obj_set_key, user_id)

		-- 减少点赞计数 (确保不为负)
		local current = tonumber(redis.call('HGET', count_hash_key, obj_id) or 0)
		if current > 0 then
			redis.call('HINCRBY', count_hash_key, obj_id, -1)
		end

		return 1
	`

	result, err := m.client.Eval(lua, []string{userSetKey, objSetKey, countHashKey},
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(objID, 10)).Result()
	if err != nil {
		return false, fmt.Errorf("unlike operation failed: %w", err)
	}

	wasLiked, _ := result.(int64)

	// 3. 异步写入持久化队列
	if wasLiked == 1 {
		go m.pushToSyncQueue(ctx, &LikeAction{
			UserID:    userID,
			ObjID:     objID,
			BizType:   bizType,
			Action:    "unlike",
			Timestamp: time.Now(),
		})
	}

	return wasLiked == 1, nil
}

// pushToSyncQueue 推送到异步同步队列
func (m *EnhancedInteractionManager) pushToSyncQueue(ctx context.Context, action *LikeAction) {
	data, err := json.Marshal(action)
	if err != nil {
		return
	}
	m.client.LPush(LikeSyncQueueKey, string(data))
}

// =============================================================================
// 查询操作
// =============================================================================

// IsLiked 检查用户是否已点赞
func (m *EnhancedInteractionManager) IsLiked(ctx context.Context, userID, objID int64, bizType int) (bool, error) {
	key := fmt.Sprintf(LikeUserSetKey, userID, bizType)
	score, err := m.client.ZScore(key, strconv.FormatInt(objID, 10)).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	return score > 0, nil
}

// GetLikeCount 获取点赞数
func (m *EnhancedInteractionManager) GetLikeCount(ctx context.Context, objID int64, bizType int) (int64, error) {
	key := fmt.Sprintf(LikeCountHashKey, bizType)
	countStr, err := m.client.HGet(key, strconv.FormatInt(objID, 10)).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	count, _ := strconv.ParseInt(countStr, 10, 64)
	return count, nil
}

// BatchGetLikeStatus 批量检查点赞状态
func (m *EnhancedInteractionManager) BatchGetLikeStatus(ctx context.Context, userID int64, objIDs []int64, bizType int) (map[int64]bool, error) {
	if len(objIDs) == 0 {
		return make(map[int64]bool), nil
	}

	key := fmt.Sprintf(LikeUserSetKey, userID, bizType)
	pipe := m.client.Pipeline()

	cmds := make(map[int64]*redis.FloatCmd)
	for _, objID := range objIDs {
		cmds[objID] = pipe.ZScore(key, strconv.FormatInt(objID, 10))
	}

	_, err := pipe.Exec()
	if err != nil && err != redis.Nil {
		return nil, err
	}

	result := make(map[int64]bool)
	for objID, cmd := range cmds {
		_, err := cmd.Result()
		result[objID] = err == nil
	}
	return result, nil
}

// BatchGetLikeCount 批量获取点赞数
func (m *EnhancedInteractionManager) BatchGetLikeCount(ctx context.Context, objIDs []int64, bizType int) (map[int64]int64, error) {
	if len(objIDs) == 0 {
		return make(map[int64]int64), nil
	}

	key := fmt.Sprintf(LikeCountHashKey, bizType)

	// 构建字段列表
	fields := make([]string, len(objIDs))
	for i, objID := range objIDs {
		fields[i] = strconv.FormatInt(objID, 10)
	}

	// 批量获取
	values, err := m.client.HMGet(key, fields...).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int64)
	for i, objID := range objIDs {
		if values[i] != nil {
			if countStr, ok := values[i].(string); ok {
				count, _ := strconv.ParseInt(countStr, 10, 64)
				result[objID] = count
			}
		} else {
			result[objID] = 0
		}
	}
	return result, nil
}

// GetUserLikeList 获取用户点赞列表
func (m *EnhancedInteractionManager) GetUserLikeList(ctx context.Context, userID int64, bizType int, offset, limit int64) ([]int64, error) {
	key := fmt.Sprintf(LikeUserSetKey, userID, bizType)

	// 按时间倒序获取
	members, err := m.client.ZRevRange(key, offset, offset+limit-1).Result()
	if err != nil {
		return nil, err
	}

	objIDs := make([]int64, 0, len(members))
	for _, member := range members {
		if objID, err := strconv.ParseInt(member, 10, 64); err == nil {
			objIDs = append(objIDs, objID)
		}
	}
	return objIDs, nil
}

// =============================================================================
// 热点数据处理
// =============================================================================

// UpdateHotVideoCache 更新热点视频缓存
func (m *EnhancedInteractionManager) UpdateHotVideoCache(ctx context.Context, videoID int64, likeCount int64) error {
	// 更新热门视频排行榜
	key := VideoPopularListKey
	score := float64(likeCount)

	_, err := m.client.ZAdd(key, redis.Z{
		Score:  score,
		Member: strconv.FormatInt(videoID, 10),
	}).Result()
	return err
}

// PrewarmHotVideo 预热热点视频数据
func (m *EnhancedInteractionManager) PrewarmHotVideo(ctx context.Context, videoID int64) error {
	// 标记为热点视频
	key := fmt.Sprintf(HotVideoCacheKey, videoID)
	return m.client.Set(key, "1", 24*time.Hour).Err()
}

// IsHotVideo 检查是否是热点视频
func (m *EnhancedInteractionManager) IsHotVideo(ctx context.Context, videoID int64) (bool, error) {
	key := fmt.Sprintf(HotVideoCacheKey, videoID)
	exists, err := m.client.Exists(key).Result()
	return exists > 0, err
}

// =============================================================================
// 异步同步消费者 (用于持久化到DB)
// =============================================================================

// PopSyncAction 从同步队列获取待处理的操作
func (m *EnhancedInteractionManager) PopSyncAction(ctx context.Context, timeout time.Duration) (*LikeAction, error) {
	result, err := m.client.BRPop(timeout, LikeSyncQueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 超时，队列为空
		}
		return nil, err
	}

	if len(result) < 2 {
		return nil, nil
	}

	var action LikeAction
	if err := json.Unmarshal([]byte(result[1]), &action); err != nil {
		return nil, err
	}

	return &action, nil
}

// GetSyncQueueLength 获取同步队列长度
func (m *EnhancedInteractionManager) GetSyncQueueLength(ctx context.Context) (int64, error) {
	return m.client.LLen(LikeSyncQueueKey).Result()
}

// =============================================================================
// 用户活跃度追踪
// =============================================================================

// TrackUserActivity 追踪用户活跃度
func (m *EnhancedInteractionManager) TrackUserActivity(ctx context.Context, userID int64, activity string) error {
	key := fmt.Sprintf(UserActiveKey, userID)
	pipe := m.client.TxPipeline()

	// 记录活跃时间
	pipe.HSet(key, activity, time.Now().Unix())
	pipe.HIncrBy(key, activity+"_count", 1)
	pipe.Expire(key, 24*time.Hour)

	_, err := pipe.Exec()
	return err
}

// GetUserActivityScore 获取用户活跃度分数
func (m *EnhancedInteractionManager) GetUserActivityScore(ctx context.Context, userID int64) (int64, error) {
	key := fmt.Sprintf(UserActiveKey, userID)

	// 获取各种活动计数
	result, err := m.client.HGetAll(key).Result()
	if err != nil {
		return 0, err
	}

	var score int64
	for k, v := range result {
		if k == "like_count" {
			count, _ := strconv.ParseInt(v, 10, 64)
			score += count * 1 // 点赞权重1
		} else if k == "comment_count" {
			count, _ := strconv.ParseInt(v, 10, 64)
			score += count * 3 // 评论权重3
		} else if k == "share_count" {
			count, _ := strconv.ParseInt(v, 10, 64)
			score += count * 5 // 分享权重5
		}
	}
	return score, nil
}
