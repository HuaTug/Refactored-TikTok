package recommendation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// ===== 1. 协同过滤召回 =====
type CollaborativeFilteringRecall struct {
	redis *redis.Client
}

func (cf *CollaborativeFilteringRecall) Name() string {
	return "collaborative_filtering"
}

func (cf *CollaborativeFilteringRecall) Weight() float64 {
	return 0.3
}

func (cf *CollaborativeFilteringRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 基于用户-物品协同过滤
	// 1. 找到相似用户
	similarUsers, err := cf.findSimilarUsers(ctx, userID, 50)
	if err != nil {
		return nil, err
	}

	// 2. 获取相似用户喜欢的视频
	videoScores := make(map[int64]float64)
	for _, simUser := range similarUsers {
		videos, err := cf.getUserLikedVideos(ctx, simUser.UserID)
		if err != nil {
			continue
		}

		for _, vid := range videos {
			videoScores[vid] += simUser.Similarity
		}
	}

	// 3. 排序并返回top N
	return cf.topNVideos(videoScores, limit), nil
}

type SimilarUser struct {
	UserID     int64
	Similarity float64
}

func (cf *CollaborativeFilteringRecall) findSimilarUsers(ctx context.Context, userID int64, limit int) ([]SimilarUser, error) {
	// 使用 Redis Sorted Set 存储用户相似度
	key := fmt.Sprintf("user:similar:%d", userID)

	results, err := cf.redis.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	users := make([]SimilarUser, 0, len(results))
	for _, z := range results {
		uid, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		users = append(users, SimilarUser{
			UserID:     uid,
			Similarity: z.Score,
		})
	}

	return users, nil
}

func (cf *CollaborativeFilteringRecall) getUserLikedVideos(ctx context.Context, userID int64) ([]int64, error) {
	key := fmt.Sprintf("user:likes:%d", userID)

	members, err := cf.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	videos := make([]int64, 0, len(members))
	for _, m := range members {
		vid, _ := strconv.ParseInt(m, 10, 64)
		videos = append(videos, vid)
	}

	return videos, nil
}

func (cf *CollaborativeFilteringRecall) topNVideos(scores map[int64]float64, n int) []int64 {
	type videoScore struct {
		videoID int64
		score   float64
	}

	list := make([]videoScore, 0, len(scores))
	for vid, score := range scores {
		list = append(list, videoScore{vid, score})
	}

	// 简单排序
	for i := 0; i < len(list) && i < n; i++ {
		maxIdx := i
		for j := i + 1; j < len(list); j++ {
			if list[j].score > list[maxIdx].score {
				maxIdx = j
			}
		}
		list[i], list[maxIdx] = list[maxIdx], list[i]
	}

	result := make([]int64, 0, n)
	for i := 0; i < len(list) && i < n; i++ {
		result = append(result, list[i].videoID)
	}

	return result
}

// ===== 2. 热门视频召回 =====
type HotVideoRecall struct {
	redis *redis.Client
}

func (hv *HotVideoRecall) Name() string {
	return "hot_video"
}

func (hv *HotVideoRecall) Weight() float64 {
	return 0.2
}

func (hv *HotVideoRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 多时间窗口热门视频
	now := time.Now()

	// 1小时热榜
	hourKey := fmt.Sprintf("hot:video:hour:%s", now.Format("2006010215"))
	hourVideos, _ := hv.redis.ZRevRange(ctx, hourKey, 0, int64(limit/3)).Result()

	// 24小时热榜
	dayKey := fmt.Sprintf("hot:video:day:%s", now.Format("20060102"))
	dayVideos, _ := hv.redis.ZRevRange(ctx, dayKey, 0, int64(limit/3)).Result()

	// 7天热榜
	weekKey := "hot:video:week"
	weekVideos, _ := hv.redis.ZRevRange(ctx, weekKey, 0, int64(limit/3)).Result()

	// 合并去重
	videoSet := make(map[int64]bool)
	videos := make([]int64, 0, limit)

	for _, v := range append(append(hourVideos, dayVideos...), weekVideos...) {
		vid, _ := strconv.ParseInt(v, 10, 64)
		if !videoSet[vid] && len(videos) < limit {
			videoSet[vid] = true
			videos = append(videos, vid)
		}
	}

	return videos, nil
}

// ===== 3. 基于内容的召回 =====
type ContentBasedRecall struct {
	redis *redis.Client
}

func (cb *ContentBasedRecall) Name() string {
	return "content_based"
}

func (cb *ContentBasedRecall) Weight() float64 {
	return 0.25
}

func (cb *ContentBasedRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 1. 获取用户兴趣标签
	interestTags, err := cb.getUserInterestTags(ctx, userID, 5)
	if err != nil {
		return nil, err
	}

	// 2. 根据标签召回视频
	videoScores := make(map[int64]float64)
	for _, tag := range interestTags {
		key := fmt.Sprintf("tag:videos:%s", tag.Tag)
		videos, _ := cb.redis.ZRevRangeWithScores(ctx, key, 0, int64(limit)).Result()

		for _, z := range videos {
			vid, _ := strconv.ParseInt(z.Member.(string), 10, 64)
			videoScores[vid] += z.Score * tag.Weight
		}
	}

	// 3. 返回top N
	cf := &CollaborativeFilteringRecall{redis: cb.redis}
	return cf.topNVideos(videoScores, limit), nil
}

type InterestTag struct {
	Tag    string
	Weight float64
}

func (cb *ContentBasedRecall) getUserInterestTags(ctx context.Context, userID int64, limit int) ([]InterestTag, error) {
	key := fmt.Sprintf("user:interests:%d", userID)

	results, err := cb.redis.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}

	tags := make([]InterestTag, 0, len(results))
	for _, z := range results {
		tags = append(tags, InterestTag{
			Tag:    z.Member.(string),
			Weight: z.Score,
		})
	}

	return tags, nil
}

// ===== 4. 社交关系召回 =====
type SocialRecall struct {
	redis *redis.Client
}

func (sr *SocialRecall) Name() string {
	return "social"
}

func (sr *SocialRecall) Weight() float64 {
	return 0.15
}

func (sr *SocialRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 1. 获取关注列表
	followKey := fmt.Sprintf("user:following:%d", userID)
	following, err := sr.redis.SMembers(ctx, followKey).Result()
	if err != nil {
		return nil, err
	}

	// 2. 获取关注用户的最新视频
	videos := make([]int64, 0, limit)
	for _, authorIDStr := range following {
		authorID, _ := strconv.ParseInt(authorIDStr, 10, 64)

		authorVideosKey := fmt.Sprintf("author:videos:%d", authorID)
		authorVideos, _ := sr.redis.ZRevRange(ctx, authorVideosKey, 0, 3).Result()

		for _, vidStr := range authorVideos {
			vid, _ := strconv.ParseInt(vidStr, 10, 64)
			videos = append(videos, vid)
			if len(videos) >= limit {
				return videos, nil
			}
		}
	}

	return videos, nil
}

// ===== 5. 新视频探索召回 =====
type NewVideoRecall struct {
	redis *redis.Client
}

func (nv *NewVideoRecall) Name() string {
	return "new_video"
}

func (nv *NewVideoRecall) Weight() float64 {
	return 0.1
}

func (nv *NewVideoRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 最近24小时发布的新视频
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	key := "videos:timeline"

	results, err := nv.redis.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%d", yesterday.Unix()),
		Max:   fmt.Sprintf("%d", now.Unix()),
		Count: int64(limit),
	}).Result()

	if err != nil {
		return nil, err
	}

	videos := make([]int64, 0, len(results))
	for _, vidStr := range results {
		vid, _ := strconv.ParseInt(vidStr, 10, 64)
		videos = append(videos, vid)
	}

	return videos, nil
}
