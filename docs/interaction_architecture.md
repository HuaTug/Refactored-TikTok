# 抖音级点赞评论系统 - 架构设计文档

## 概述

本文档详细描述了基于抖音交互逻辑设计的高并发点赞评论系统，涵盖了并发问题解决方案、数据一致性策略和Redis架构设计。

## 目录

- [1. 系统架构](#1-系统架构)
- [2. 数据一致性策略](#2-数据一致性策略)
- [3. Redis设计](#3-redis设计)
- [4. 并发控制](#4-并发控制)
- [5. 性能优化](#5-性能优化)
- [6. 测试方案](#6-测试方案)

---

## 1. 系统架构

### 1.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Client (APP/Web)                                │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │ HTTP/RPC
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           API Gateway (Hertz)                                │
│                         - JWT验证 - 参数校验                                  │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │ Kitex RPC
                                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Interaction Service (点赞/评论)                         │
├───────────────┬─────────────────┬─────────────────┬─────────────────────────┤
│  Rate Limiter │   Dist Lock     │   Biz Logic     │   Async Worker          │
│  (滑动窗口)    │   (RedLock)     │   (业务逻辑)     │   (异步同步)             │
└───────┬───────┴────────┬────────┴────────┬────────┴────────┬────────────────┘
        │                │                 │                 │
        ▼                ▼                 ▼                 ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                           Redis Cluster                                    │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐      │
│  │  点赞状态    │ │  计数缓存    │ │  限流计数    │ │  同步队列    │        │
│  │  (ZSet)      │ │  (Hash)      │ │  (ZSet)      │ │  (List)      │        │
│  └──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘      │
└───────────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼ 异步同步
┌───────────────────────────────────────────────────────────────────────────┐
│                            MySQL (主从)                                    │
│          video_likes | comment_likes | comments | videos                   │
└───────────────────────────────────────────────────────────────────────────┘
```

### 1.2 核心组件

| 组件 | 职责 | 实现文件 |
|------|------|----------|
| Rate Limiter | 滑动窗口限流 | `interaction_enhanced.go` |
| Dist Lock | 分布式锁 | `interaction_enhanced.go` |
| Like Service | 点赞业务逻辑 | `like_service_enhanced.go` |
| Comment Service | 评论业务逻辑 | `comment_service_enhanced.go` |
| Async Worker | 异步DB同步 | `like_service_enhanced.go` |

---

## 2. 数据一致性策略

### 2.1 一致性模型选择

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       数据一致性策略矩阵                                      │
├──────────────────┬────────────────────┬────────────────────────────────────┤
│       场景       │    一致性级别       │           原因                      │
├──────────────────┼────────────────────┼────────────────────────────────────┤
│   点赞状态       │   强一致性          │ 用户敏感：点赞后必须立即显示红心     │
│   (是否点赞)     │                    │ 实现：Redis+DB双写，失败回滚         │
├──────────────────┼────────────────────┼────────────────────────────────────┤
│   点赞计数       │   最终一致性        │ 用户不敏感：差1-2个赞无感知          │
│   (多少人点赞)   │                    │ 实现：Redis先行，异步同步DB          │
├──────────────────┼────────────────────┼────────────────────────────────────┤
│   评论内容       │   强一致性          │ 用户敏感：评论后必须立即可见         │
│                  │                    │ 实现：同步写DB，成功后更新缓存       │
├──────────────────┼────────────────────┼────────────────────────────────────┤
│   评论计数       │   最终一致性        │ 用户不敏感：评论数少量误差可接受     │
│                  │                    │ 实现：缓存计数，定时校准             │
└──────────────────┴────────────────────┴────────────────────────────────────┘
```

### 2.2 点赞状态的强一致性实现

```go
// 点赞操作流程 (强一致性)
func DoLike(ctx, userID, videoID) error {
    // 1. 获取分布式锁
    lock := AcquireLock("like:user:video", timeout)
    defer lock.Release()
    
    // 2. 检查是否已点赞 (Redis)
    if IsLiked(userID, videoID) {
        return nil // 幂等返回
    }
    
    // 3. 原子更新Redis (Lua脚本)
    // - 添加到用户点赞集合
    // - 添加到视频点赞用户集合
    // - 增加点赞计数
    err := AtomicLike(userID, videoID)
    if err != nil {
        return err
    }
    
    // 4. 异步同步到DB (最终一致性)
    go PushToSyncQueue(LikeAction{userID, videoID, "like"})
    
    return nil
}
```

### 2.3 数据校准机制

```go
// 定时校准任务 (每小时执行)
func CalibrateTask() {
    // 1. 获取热门视频列表
    hotVideos := GetHotVideos(limit=1000)
    
    // 2. 比对Redis和DB的点赞数
    for _, videoID := range hotVideos {
        redisCount := GetRedisLikeCount(videoID)
        dbCount := GetDBLikeCount(videoID)
        
        diff := abs(redisCount - dbCount)
        
        // 3. 差异超过阈值，进行校准
        if diff > 10 {
            // 以DB为准，更新Redis
            SetRedisLikeCount(videoID, dbCount)
            log.Info("Calibrated video %d: redis=%d, db=%d", videoID, redisCount, dbCount)
        }
    }
}
```

---

## 3. Redis设计

### 3.1 Key设计规范

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Redis Key 设计规范                                  │
├──────────────────┬─────────────────┬───────────────────────────────────────┤
│       Key        │      类型       │              说明                      │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ like:user:{uid}:{biz} │   ZSet     │ 用户点赞列表                           │
│                  │                 │ Score: 时间戳, Member: 资源ID          │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ like:obj:{biz}:{oid}  │   ZSet     │ 资源被点赞用户列表                      │
│                  │                 │ Score: 时间戳, Member: 用户ID          │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ like:count:{biz} │   Hash          │ 点赞计数汇总                           │
│                  │                 │ Field: 资源ID, Value: 计数             │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ ratelimit:user:{uid}:{action} │ ZSet │ 用户限流计数                       │
│                  │                 │ 滑动窗口算法                           │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ lock:like:{biz}:{uid}:{oid} │ String │ 点赞操作分布式锁                   │
│                  │                 │ 防止并发冲突                           │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│ queue:like:sync  │   List          │ 待同步队列                             │
│                  │                 │ 异步持久化到DB                          │
└──────────────────┴─────────────────┴───────────────────────────────────────┘
```

### 3.2 数据结构示例

```redis
# 用户点赞列表
ZADD like:user:12345:1 1706688000 10001  # 用户12345在时间戳1706688000点赞了视频10001
ZADD like:user:12345:1 1706688100 10002  # 用户12345在时间戳1706688100点赞了视频10002

# 视频被点赞用户列表
ZADD like:obj:1:10001 1706688000 12345   # 视频10001在时间戳1706688000被用户12345点赞
ZADD like:obj:1:10001 1706688200 67890   # 视频10001在时间戳1706688200被用户67890点赞

# 点赞计数汇总
HSET like:count:1 10001 2                # 视频10001有2个点赞
HSET like:count:1 10002 100              # 视频10002有100个点赞

# 限流计数 (滑动窗口)
ZADD ratelimit:user:12345:like 1706688000001 "1706688000001:rand1"  # 记录时间戳
ZADD ratelimit:user:12345:like 1706688000100 "1706688000100:rand2"

# 分布式锁
SET lock:like:1:12345:10001 "1706688000:12345" EX 3 NX

# 同步队列
LPUSH queue:like:sync '{"user_id":12345,"obj_id":10001,"action":"like"}'
```

### 3.3 原子操作Lua脚本

```lua
-- 点赞操作 Lua 脚本 (保证原子性)
local user_set_key = KEYS[1]    -- like:user:{uid}:{biz}
local obj_set_key = KEYS[2]     -- like:obj:{biz}:{oid}
local count_hash_key = KEYS[3]  -- like:count:{biz}
local user_id = ARGV[1]
local obj_id = ARGV[2]
local timestamp = tonumber(ARGV[3])

-- 检查是否已点赞
local score = redis.call('ZSCORE', user_set_key, obj_id)
if score then
    -- 已点赞，返回 0
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
```

---

## 4. 并发控制

### 4.1 分布式锁设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          分布式锁策略                                        │
├──────────────────┬─────────────────┬───────────────────────────────────────┤
│       场景       │    锁粒度        │           实现方式                     │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   点赞操作       │  user:obj       │ SET NX EX + Lua释放                   │
│                  │                 │ 防止同一用户对同一资源并发点赞         │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   评论创建       │  user:video     │ SET NX EX                             │
│                  │                 │ 防止同一用户对同一视频重复评论         │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   计数更新       │  不需要锁        │ 使用 Lua 脚本原子操作                 │
│                  │                 │ HINCRBY 本身是原子的                   │
└──────────────────┴─────────────────┴───────────────────────────────────────┘
```

### 4.2 限流策略

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          多维度限流策略                                      │
├──────────────────┬─────────────────┬───────────────────────────────────────┤
│       维度       │    限制          │           实现                         │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   用户级         │  10次/秒 (点赞)  │ 滑动窗口算法                          │
│                  │  10次/分 (评论)  │ Redis ZSet 存储时间戳                 │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   IP级           │  100次/秒        │ 防止刷接口攻击                        │
│                  │                 │ 在API Gateway层实现                    │
├──────────────────┼─────────────────┼───────────────────────────────────────┤
│   全局级         │  10万QPS         │ 保护后端服务不被压垮                   │
│                  │                 │ 令牌桶算法                             │
└──────────────────┴─────────────────┴───────────────────────────────────────┘
```

### 4.3 滑动窗口限流实现

```lua
-- 滑动窗口限流 Lua 脚本
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
    return {1, limit - current - 1, 0}  -- 允许, 剩余次数, 重试时间
else
    -- 已超限，返回最早请求的过期时间
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry_at = 0
    if #oldest >= 2 then
        retry_at = tonumber(oldest[2]) + window_ms
    end
    return {0, 0, retry_at}  -- 拒绝, 剩余0, 重试时间
end
```

---

## 5. 性能优化

### 5.1 批量操作优化

```go
// 批量获取点赞状态 (减少网络往返)
func BatchGetLikeStatus(userID int64, objIDs []int64, bizType int) (map[int64]bool, error) {
    key := fmt.Sprintf(LikeUserSetKey, userID, bizType)
    
    pipe := redis.Pipeline()
    cmds := make(map[int64]*redis.FloatCmd)
    
    for _, objID := range objIDs {
        cmds[objID] = pipe.ZScore(key, strconv.FormatInt(objID, 10))
    }
    
    pipe.Exec()
    
    result := make(map[int64]bool)
    for objID, cmd := range cmds {
        _, err := cmd.Result()
        result[objID] = err == nil
    }
    return result, nil
}
```

### 5.2 热点数据处理

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          热点数据处理策略                                    │
├──────────────────┬──────────────────────────────────────────────────────────┤
│   本地缓存       │ 使用 Go 的 sync.Map 缓存热点视频的点赞数                  │
│                  │ TTL: 1秒，减少 Redis 访问                                 │
├──────────────────┼──────────────────────────────────────────────────────────┤
│   缓存预热       │ 定时任务预热 Top 1000 热门视频到 Redis                    │
│                  │ 避免缓存击穿                                              │
├──────────────────┼──────────────────────────────────────────────────────────┤
│   热点散列       │ 对超高流量视频的点赞计数进行分片                           │
│                  │ 如: like:count:hot:10001:{0-9}                            │
└──────────────────┴──────────────────────────────────────────────────────────┘
```

### 5.3 异步处理优化

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          异步处理架构                                        │
└─────────────────────────────┬───────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐     ┌───────────────┐     ┌───────────────┐
│  点赞操作     │     │  异步队列     │     │  同步Worker   │
│  (Redis)      │ ──▶ │  (List)       │ ──▶ │  (批量写DB)   │
│  < 10ms       │     │  缓冲区       │     │  每5秒 batch  │
└───────────────┘     └───────────────┘     └───────────────┘
```

---

## 6. 测试方案

### 6.1 单元测试

```go
// 文件: interaction_enhanced_test.go

func TestDoLike(t *testing.T) {
    manager := NewEnhancedInteractionManager(redisClient)
    
    // 测试正常点赞
    success, isNew, err := manager.DoLike(ctx, 12345, 10001, BizTypeVideo)
    assert.NoError(t, err)
    assert.True(t, success)
    assert.True(t, isNew)
    
    // 测试重复点赞 (幂等)
    success, isNew, err = manager.DoLike(ctx, 12345, 10001, BizTypeVideo)
    assert.NoError(t, err)
    assert.True(t, success)
    assert.False(t, isNew) // 不是新点赞
}

func TestRateLimit(t *testing.T) {
    manager := NewEnhancedInteractionManager(redisClient)
    
    // 快速连续请求11次
    for i := 0; i < 11; i++ {
        result, _ := manager.CheckUserLikeRateLimit(ctx, 12345)
        if i < 10 {
            assert.True(t, result.Allowed)
        } else {
            assert.False(t, result.Allowed) // 第11次应该被限流
        }
    }
}

func TestDistributedLock(t *testing.T) {
    manager := NewEnhancedInteractionManager(redisClient)
    
    // 获取锁
    value, acquired, err := manager.AcquireLikeLock(ctx, BizTypeVideo, 12345, 10001)
    assert.NoError(t, err)
    assert.True(t, acquired)
    assert.NotEmpty(t, value)
    
    // 尝试再次获取同一个锁 (应该失败)
    _, acquired2, _ := manager.AcquireLikeLock(ctx, BizTypeVideo, 12345, 10001)
    assert.False(t, acquired2)
    
    // 释放锁
    err = manager.ReleaseLikeLock(ctx, BizTypeVideo, 12345, 10001, value)
    assert.NoError(t, err)
}
```

### 6.2 并发测试

```go
func TestConcurrentLike(t *testing.T) {
    manager := NewEnhancedInteractionManager(redisClient)
    
    var wg sync.WaitGroup
    successCount := int32(0)
    
    // 100个并发点赞同一个视频
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(userID int64) {
            defer wg.Done()
            success, _, err := manager.DoLike(ctx, userID, 10001, BizTypeVideo)
            if err == nil && success {
                atomic.AddInt32(&successCount, 1)
            }
        }(int64(i))
    }
    
    wg.Wait()
    
    // 验证点赞数
    count, _ := manager.GetLikeCount(ctx, 10001, BizTypeVideo)
    assert.Equal(t, int64(100), count)
}
```

### 6.3 压力测试

```bash
# 使用 wrk 进行压力测试
wrk -t12 -c400 -d30s -s like.lua http://localhost:8080/v1/action/like

# like.lua 脚本
wrk.method = "POST"
wrk.body   = '{"video_id": 10001, "action_type": "like"}'
wrk.headers["Content-Type"] = "application/json"
wrk.headers["Authorization"] = "Bearer <token>"

# 预期结果:
# - QPS > 10000
# - P99 延迟 < 100ms
# - 错误率 < 0.1%
```

### 6.4 一致性测试

```go
func TestEventualConsistency(t *testing.T) {
    manager := NewEnhancedInteractionManager(redisClient)
    likeService := NewEnhancedLikeService(ctx, nil)
    
    // 1. 执行点赞
    likeService.LikeAction(ctx, &LikeActionRequest{
        UserId:     12345,
        VideoId:    10001,
        ActionType: "like",
    })
    
    // 2. 立即检查Redis (应该有)
    isLiked, _ := manager.IsLiked(ctx, 12345, 10001, BizTypeVideo)
    assert.True(t, isLiked)
    
    // 3. 等待异步同步
    time.Sleep(10 * time.Second)
    
    // 4. 检查DB (应该有)
    dbLike, _ := db.GetVideoLikeByUserAndVideo(ctx, 12345, 10001)
    assert.NotNil(t, dbLike)
}
```

---

## 文件清单

| 文件 | 描述 |
|------|------|
| `cmd/interaction/infras/redis/interaction_enhanced.go` | 增强版Redis交互管理器 |
| `cmd/interaction/service/like_service_enhanced.go` | 增强版点赞服务 |
| `cmd/interaction/service/comment_service_enhanced.go` | 增强版评论服务 |
| `docs/interaction_architecture.md` | 本架构文档 |

---

## 参考资料

1. [抖音架构演进](https://mp.weixin.qq.com/s/...)
2. [B站点赞系统设计](https://mp.weixin.qq.com/s/...)
3. [Redis分布式锁最佳实践](https://redis.io/topics/distlock)
4. [滑动窗口限流算法](https://developer.aliyun.com/article/...)

---

*文档版本: v1.0*  
*最后更新: 2026-01-30*
