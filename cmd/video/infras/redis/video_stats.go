package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

const (
	// Key patterns
	videoVisitCountKey     = "video:visit:%d"          // 视频浏览量 key pattern
	videoVisitDailyKey     = "video:visit:daily:%s:%d" // 每日浏览量 key pattern (date:videoId)
	userWatchHistoryKey    = "user:watch:history:%d"   // 用户观看历史 sorted set
	videoHotScoreKey       = "video:hot:score"         // 视频热度排行榜 sorted set
	visitCountSyncQueueKey = "video:visit:sync:queue"  // 待同步到数据库的队列

	// TTL settings
	visitCountTTL      = 24 * time.Hour  // 浏览量缓存24小时
	watchHistoryTTL    = 7 * 24 * time.Hour // 观看历史缓存7天
	hotScoreTTL        = 1 * time.Hour   // 热度分数1小时
	visitDailyTTL      = 48 * time.Hour  // 每日统计保留48小时
)

// IncrementVideoVisitCount 增加视频浏览量（使用Redis缓存）
func IncrementVideoVisitCount(ctx context.Context, videoId int64) (int64, error) {
	key := fmt.Sprintf(videoVisitCountKey, videoId)
	
	// INCR 原子操作增加浏览量
	count, err := redisDBVideoInfo.Incr(key).Result()
	if err != nil {
		hlog.Errorf("Failed to increment visit count in Redis for video %d: %v", videoId, err)
		return 0, err
	}
	
	// 设置/延长过期时间
	redisDBVideoInfo.Expire(key, visitCountTTL)
	
	// 同时更新每日统计
	dailyKey := fmt.Sprintf(videoVisitDailyKey, time.Now().Format("2006-01-02"), videoId)
	redisDBVideoInfo.Incr(dailyKey)
	redisDBVideoInfo.Expire(dailyKey, visitDailyTTL)
	
	// 更新热度分数（基于浏览量的简单热度算法）
	UpdateVideoHotScore(ctx, videoId, 1.0) // 每次浏览加1分
	
	// 将videoId添加到待同步队列（批量同步到数据库）
	redisDBVideoInfo.SAdd(visitCountSyncQueueKey, videoId)
	
	hlog.Debugf("Video %d visit count incremented to %d", videoId, count)
	return count, nil
}

// GetVideoVisitCountCached 获取视频浏览量（优先从Redis获取）- 使用新的缓存key
func GetVideoVisitCountCached(ctx context.Context, videoId int64) (int64, bool, error) {
	key := fmt.Sprintf(videoVisitCountKey, videoId)
	
	countStr, err := redisDBVideoInfo.Get(key).Result()
	if err != nil {
		// Redis中不存在，需要从数据库加载
		return 0, false, nil
	}
	
	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return 0, false, err
	}
	
	return count, true, nil
}

// SetVideoVisitCount 设置视频浏览量（从数据库同步到Redis）
func SetVideoVisitCount(ctx context.Context, videoId int64, count int64) error {
	key := fmt.Sprintf(videoVisitCountKey, videoId)
	return redisDBVideoInfo.Set(key, count, visitCountTTL).Err()
}

// AddToWatchHistory 添加观看历史到Redis
func AddToWatchHistory(ctx context.Context, userId, videoId int64) error {
	key := fmt.Sprintf(userWatchHistoryKey, userId)
	
	// 使用 ZADD，score为时间戳，实现按时间排序
	score := float64(time.Now().Unix())
	
	_, err := redisDBVideoInfo.ZAdd(key, struct {
		Score  float64
		Member interface{}
	}{
		Score:  score,
		Member: videoId,
	}).Result()
	
	if err != nil {
		hlog.Errorf("Failed to add watch history to Redis for user %d, video %d: %v", userId, videoId, err)
		return err
	}
	
	// 设置/延长过期时间
	redisDBVideoInfo.Expire(key, watchHistoryTTL)
	
	// 保持最近500条记录，删除旧的
	redisDBVideoInfo.ZRemRangeByRank(key, 0, -501)
	
	return nil
}

// GetWatchHistoryFromCache 从Redis获取观看历史
func GetWatchHistoryFromCache(ctx context.Context, userId int64, offset, limit int64) ([]int64, error) {
	key := fmt.Sprintf(userWatchHistoryKey, userId)
	
	// 按分数倒序获取（最新的在前）
	results, err := redisDBVideoInfo.ZRevRange(key, offset, offset+limit-1).Result()
	if err != nil {
		return nil, err
	}
	
	videoIds := make([]int64, 0, len(results))
	for _, r := range results {
		id, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			continue
		}
		videoIds = append(videoIds, id)
	}
	
	return videoIds, nil
}

// IsVideoInWatchHistory 检查视频是否在观看历史中
func IsVideoInWatchHistory(ctx context.Context, userId, videoId int64) (bool, error) {
	key := fmt.Sprintf(userWatchHistoryKey, userId)
	
	_, err := redisDBVideoInfo.ZScore(key, strconv.FormatInt(videoId, 10)).Result()
	if err != nil {
		return false, nil // 不存在
	}
	
	return true, nil
}

// RemoveFromWatchHistory 从观看历史中移除
func RemoveFromWatchHistory(ctx context.Context, userId, videoId int64) error {
	key := fmt.Sprintf(userWatchHistoryKey, userId)
	return redisDBVideoInfo.ZRem(key, videoId).Err()
}

// ClearWatchHistoryCache 清空用户观看历史缓存
func ClearWatchHistoryCache(ctx context.Context, userId int64) error {
	key := fmt.Sprintf(userWatchHistoryKey, userId)
	return redisDBVideoInfo.Del(key).Err()
}

// UpdateVideoHotScore 更新视频热度分数
func UpdateVideoHotScore(ctx context.Context, videoId int64, scoreDelta float64) error {
	_, err := redisDBVideoInfo.ZIncrBy(videoHotScoreKey, scoreDelta, strconv.FormatInt(videoId, 10)).Result()
	if err != nil {
		return err
	}
	
	// 设置过期时间
	redisDBVideoInfo.Expire(videoHotScoreKey, hotScoreTTL)
	return nil
}

// GetHotVideos 获取热门视频列表
func GetHotVideos(ctx context.Context, limit int64) ([]int64, error) {
	results, err := redisDBVideoInfo.ZRevRange(videoHotScoreKey, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}
	
	videoIds := make([]int64, 0, len(results))
	for _, r := range results {
		id, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			continue
		}
		videoIds = append(videoIds, id)
	}
	
	return videoIds, nil
}

// GetPendingSyncVideoIds 获取待同步的视频ID列表
func GetPendingSyncVideoIds(ctx context.Context) ([]int64, error) {
	results, err := redisDBVideoInfo.SMembers(visitCountSyncQueueKey).Result()
	if err != nil {
		return nil, err
	}
	
	videoIds := make([]int64, 0, len(results))
	for _, r := range results {
		id, err := strconv.ParseInt(r, 10, 64)
		if err != nil {
			continue
		}
		videoIds = append(videoIds, id)
	}
	
	return videoIds, nil
}

// ClearSyncedVideoId 从待同步队列中移除已同步的视频ID
func ClearSyncedVideoId(ctx context.Context, videoId int64) error {
	return redisDBVideoInfo.SRem(visitCountSyncQueueKey, videoId).Err()
}

// BatchGetVideoVisitCounts 批量获取视频浏览量
func BatchGetVideoVisitCounts(ctx context.Context, videoIds []int64) (map[int64]int64, error) {
	if len(videoIds) == 0 {
		return make(map[int64]int64), nil
	}
	
	pipe := redisDBVideoInfo.Pipeline()
	cmds := make(map[int64]*struct {
		Cmd interface{}
	})
	
	for _, id := range videoIds {
		key := fmt.Sprintf(videoVisitCountKey, id)
		cmd := pipe.Get(key)
		cmds[id] = &struct{ Cmd interface{} }{Cmd: cmd}
	}
	
	_, err := pipe.Exec()
	if err != nil && err.Error() != "redis: nil" {
		hlog.Warnf("Batch get visit counts partial error: %v", err)
	}
	
	result := make(map[int64]int64)
	for id := range cmds {
		key := fmt.Sprintf(videoVisitCountKey, id)
		countStr, err := redisDBVideoInfo.Get(key).Result()
		if err == nil {
			if count, err := strconv.ParseInt(countStr, 10, 64); err == nil {
				result[id] = count
			}
		}
	}
	
	return result, nil
}

// GetDailyVideoVisitCount 获取视频某天的浏览量
func GetDailyVideoVisitCount(ctx context.Context, videoId int64, date string) (int64, error) {
	key := fmt.Sprintf(videoVisitDailyKey, date, videoId)
	
	countStr, err := redisDBVideoInfo.Get(key).Result()
	if err != nil {
		return 0, nil // 不存在返回0
	}
	
	return strconv.ParseInt(countStr, 10, 64)
}
