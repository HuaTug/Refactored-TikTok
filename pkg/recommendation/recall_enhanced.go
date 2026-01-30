package recommendation

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ========================================
// 增强版召回策略（基于数据库）
// ========================================

// BaseRecallStrategy 基础召回策略
type BaseRecallStrategy struct {
	redis *redis.Client
	db    *gorm.DB
}

// topNVideosByScore 根据分数获取Top N视频
func (b *BaseRecallStrategy) topNVideosByScore(scores map[int64]float64, n int) []int64 {
	type videoScore struct {
		videoID int64
		score   float64
	}

	list := make([]videoScore, 0, len(scores))
	for vid, score := range scores {
		list = append(list, videoScore{vid, score})
	}

	// 排序
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

// ===== 1. 增强版协同过滤召回 =====

// EnhancedCFRecall 增强版协同过滤召回
type EnhancedCFRecall struct {
	BaseRecallStrategy
}

// NewCollaborativeFilteringRecall 创建协同过滤召回
func NewCollaborativeFilteringRecall(redisClient *redis.Client, db *gorm.DB) *EnhancedCFRecall {
	return &EnhancedCFRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (cf *EnhancedCFRecall) Name() string {
	return "collaborative_filtering"
}

func (cf *EnhancedCFRecall) Weight() float64 {
	return 0.3
}

func (cf *EnhancedCFRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 1. 尝试从缓存获取用户向量
	userEmbKey := fmt.Sprintf("user:embedding:%d", userID)
	userVec, err := cf.redis.Get(ctx, userEmbKey).Result()
	
	if err == nil && userVec != "" {
		// 使用向量相似度召回
		return cf.recallByEmbedding(ctx, userID, userVec, limit)
	}

	// 2. 回退到基于用户相似度的协同过滤
	return cf.recallBySimilarUsers(ctx, userID, limit)
}

func (cf *EnhancedCFRecall) recallByEmbedding(ctx context.Context, userID int64, userVec string, limit int) ([]int64, error) {
	// 从缓存获取相似用户
	key := fmt.Sprintf("user:similar:%d", userID)
	results, err := cf.redis.ZRevRangeWithScores(ctx, key, 0, int64(limit*2)).Result()
	if err != nil {
		return nil, err
	}

	videoScores := make(map[int64]float64)
	for _, z := range results {
		simUserID, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		similarity := z.Score

		// 获取相似用户喜欢的视频
		likesKey := fmt.Sprintf("user:likes:%d", simUserID)
		videos, _ := cf.redis.SMembers(ctx, likesKey).Result()

		for _, vidStr := range videos {
			vid, _ := strconv.ParseInt(vidStr, 10, 64)
			videoScores[vid] += similarity
		}
	}

	return cf.topNVideosByScore(videoScores, limit), nil
}

func (cf *EnhancedCFRecall) recallBySimilarUsers(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 基于用户行为相似度
	key := fmt.Sprintf("user:similar:%d", userID)
	results, err := cf.redis.ZRevRangeWithScores(ctx, key, 0, 49).Result()
	if err != nil {
		return nil, err
	}

	videoScores := make(map[int64]float64)
	for _, z := range results {
		simUserID, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		similarity := z.Score

		// 获取相似用户的行为视频
		likesKey := fmt.Sprintf("user:likes:%d", simUserID)
		videos, _ := cf.redis.SMembers(ctx, likesKey).Result()

		for _, vidStr := range videos {
			vid, _ := strconv.ParseInt(vidStr, 10, 64)
			videoScores[vid] += similarity
		}
	}

	return cf.topNVideosByScore(videoScores, limit), nil
}

// ===== 2. 增强版热门视频召回 =====

// EnhancedHotRecall 增强版热门召回
type EnhancedHotRecall struct {
	BaseRecallStrategy
}

// NewHotVideoRecall 创建热门视频召回
func NewHotVideoRecall(redisClient *redis.Client, db *gorm.DB) *EnhancedHotRecall {
	return &EnhancedHotRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (hv *EnhancedHotRecall) Name() string {
	return "hot_video"
}

func (hv *EnhancedHotRecall) Weight() float64 {
	return 0.2
}

func (hv *EnhancedHotRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	now := time.Now()
	videoSet := make(map[int64]float64)

	// 1. 1小时热榜 (权重最高)
	hourKey := fmt.Sprintf("hot:video:hour:%s", now.Format("2006010215"))
	hv.addHotVideos(ctx, hourKey, videoSet, 1.0, limit/3)

	// 2. 24小时热榜
	dayKey := fmt.Sprintf("hot:video:day:%s", now.Format("20060102"))
	hv.addHotVideos(ctx, dayKey, videoSet, 0.8, limit/3)

	// 3. 7天热榜
	weekKey := "hot:video:week"
	hv.addHotVideos(ctx, weekKey, videoSet, 0.6, limit/3)

	// 4. 实时热度榜（从video_hot_scores表）
	realtimeKey := "hot:video:realtime"
	hv.addHotVideos(ctx, realtimeKey, videoSet, 0.9, limit/4)

	return hv.topNVideosByScore(videoSet, limit), nil
}

func (hv *EnhancedHotRecall) addHotVideos(ctx context.Context, key string, videoSet map[int64]float64, weight float64, limit int) {
	results, err := hv.redis.ZRevRangeWithScores(ctx, key, 0, int64(limit)).Result()
	if err != nil {
		return
	}

	for _, z := range results {
		vid, _ := strconv.ParseInt(z.Member.(string), 10, 64)
		videoSet[vid] += z.Score * weight
	}
}

// ===== 3. 增强版内容召回 =====

// EnhancedContentRecall 增强版内容召回
type EnhancedContentRecall struct {
	BaseRecallStrategy
}

// NewContentBasedRecall 创建内容召回
func NewContentBasedRecall(redisClient *redis.Client, db *gorm.DB) *EnhancedContentRecall {
	return &EnhancedContentRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (cb *EnhancedContentRecall) Name() string {
	return "content_based"
}

func (cb *EnhancedContentRecall) Weight() float64 {
	return 0.25
}

func (cb *EnhancedContentRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	videoScores := make(map[int64]float64)

	// 1. 基于兴趣标签召回
	cb.recallByTags(ctx, userID, videoScores, limit)

	// 2. 基于分类偏好召回
	cb.recallByCategories(ctx, userID, videoScores, limit)

	// 3. 基于作者偏好召回
	cb.recallByAuthors(ctx, userID, videoScores, limit)

	return cb.topNVideosByScore(videoScores, limit), nil
}

func (cb *EnhancedContentRecall) recallByTags(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	// 获取用户兴趣标签
	interestsKey := fmt.Sprintf("user:interests:%d", userID)
	tags, err := cb.redis.ZRevRangeWithScores(ctx, interestsKey, 0, 9).Result()
	if err != nil {
		return
	}

	for _, tag := range tags {
		tagName := tag.Member.(string)
		tagWeight := tag.Score

		// 根据标签获取视频
		tagVideosKey := fmt.Sprintf("tag:videos:%s", tagName)
		videos, _ := cb.redis.ZRevRangeWithScores(ctx, tagVideosKey, 0, int64(limit)).Result()

		for _, v := range videos {
			vid, _ := strconv.ParseInt(v.Member.(string), 10, 64)
			videoScores[vid] += v.Score * tagWeight
		}
	}
}

func (cb *EnhancedContentRecall) recallByCategories(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	// 获取用户分类偏好
	categoryKey := fmt.Sprintf("user:category_prefer:%d", userID)
	categories, err := cb.redis.ZRevRangeWithScores(ctx, categoryKey, 0, 4).Result()
	if err != nil {
		return
	}

	for _, cat := range categories {
		catName := cat.Member.(string)
		catWeight := cat.Score

		// 根据分类获取视频
		catVideosKey := fmt.Sprintf("category:videos:%s", catName)
		videos, _ := cb.redis.ZRevRangeWithScores(ctx, catVideosKey, 0, int64(limit/2)).Result()

		for _, v := range videos {
			vid, _ := strconv.ParseInt(v.Member.(string), 10, 64)
			videoScores[vid] += v.Score * catWeight * 0.8
		}
	}
}

func (cb *EnhancedContentRecall) recallByAuthors(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	// 获取用户喜好的作者
	authorKey := fmt.Sprintf("user:author_prefer:%d", userID)
	authors, err := cb.redis.ZRevRangeWithScores(ctx, authorKey, 0, 19).Result()
	if err != nil {
		return
	}

	for _, author := range authors {
		authorID := author.Member.(string)
		authorWeight := author.Score

		// 获取作者的视频
		authorVideosKey := fmt.Sprintf("author:videos:%s", authorID)
		videos, _ := cb.redis.ZRevRange(ctx, authorVideosKey, 0, 4).Result()

		for _, vidStr := range videos {
			vid, _ := strconv.ParseInt(vidStr, 10, 64)
			videoScores[vid] += authorWeight * 0.5
		}
	}
}

// ===== 4. 增强版社交召回 =====

// EnhancedSocialRecall 增强版社交召回
type EnhancedSocialRecall struct {
	BaseRecallStrategy
}

// NewSocialRecall 创建社交召回
func NewSocialRecall(redisClient *redis.Client, db *gorm.DB) *EnhancedSocialRecall {
	return &EnhancedSocialRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (sr *EnhancedSocialRecall) Name() string {
	return "social"
}

func (sr *EnhancedSocialRecall) Weight() float64 {
	return 0.15
}

func (sr *EnhancedSocialRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	videoScores := make(map[int64]float64)

	// 1. 关注用户的最新视频
	sr.recallFromFollowing(ctx, userID, videoScores, limit)

	// 2. 好友（互相关注）喜欢的视频
	sr.recallFromFriends(ctx, userID, videoScores, limit)

	// 3. 同城/同校用户热门视频
	sr.recallFromNearby(ctx, userID, videoScores, limit)

	return sr.topNVideosByScore(videoScores, limit), nil
}

func (sr *EnhancedSocialRecall) recallFromFollowing(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	followKey := fmt.Sprintf("user:following:%d", userID)
	following, err := sr.redis.SMembers(ctx, followKey).Result()
	if err != nil {
		return
	}

	for _, authorIDStr := range following {
		// 获取关注用户的最新视频
		authorVideosKey := fmt.Sprintf("author:videos:%s", authorIDStr)
		videos, _ := sr.redis.ZRevRangeWithScores(ctx, authorVideosKey, 0, 4).Result()

		for _, v := range videos {
			vid, _ := strconv.ParseInt(v.Member.(string), 10, 64)
			videoScores[vid] += v.Score * 1.2 // 关注用户权重较高
		}
	}
}

func (sr *EnhancedSocialRecall) recallFromFriends(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	friendsKey := fmt.Sprintf("user:friends:%d", userID)
	friends, err := sr.redis.SMembers(ctx, friendsKey).Result()
	if err != nil {
		return
	}

	for _, friendIDStr := range friends {
		// 获取好友最近点赞的视频
		friendLikesKey := fmt.Sprintf("user:recent_likes:%s", friendIDStr)
		videos, _ := sr.redis.ZRevRange(ctx, friendLikesKey, 0, 2).Result()

		for _, vidStr := range videos {
			vid, _ := strconv.ParseInt(vidStr, 10, 64)
			videoScores[vid] += 0.8 // 好友推荐
		}
	}
}

func (sr *EnhancedSocialRecall) recallFromNearby(ctx context.Context, userID int64, videoScores map[int64]float64, limit int) {
	// 获取用户所在城市/学校
	userInfoKey := fmt.Sprintf("user:info:%d", userID)
	userInfo, err := sr.redis.HGetAll(ctx, userInfoKey).Result()
	if err != nil {
		return
	}

	// 同城热门
	if city, ok := userInfo["city"]; ok && city != "" {
		cityHotKey := fmt.Sprintf("hot:video:city:%s", city)
		videos, _ := sr.redis.ZRevRangeWithScores(ctx, cityHotKey, 0, int64(limit/4)).Result()
		for _, v := range videos {
			vid, _ := strconv.ParseInt(v.Member.(string), 10, 64)
			videoScores[vid] += v.Score * 0.5
		}
	}

	// 同校热门
	if school, ok := userInfo["school_id"]; ok && school != "" {
		schoolHotKey := fmt.Sprintf("hot:video:school:%s", school)
		videos, _ := sr.redis.ZRevRangeWithScores(ctx, schoolHotKey, 0, int64(limit/4)).Result()
		for _, v := range videos {
			vid, _ := strconv.ParseInt(v.Member.(string), 10, 64)
			videoScores[vid] += v.Score * 0.6
		}
	}
}

// ===== 5. 增强版新视频召回 =====

// EnhancedNewVideoRecall 增强版新视频召回
type EnhancedNewVideoRecall struct {
	BaseRecallStrategy
}

// NewNewVideoRecall 创建新视频召回
func NewNewVideoRecall(redisClient *redis.Client, db *gorm.DB) *EnhancedNewVideoRecall {
	return &EnhancedNewVideoRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (nv *EnhancedNewVideoRecall) Name() string {
	return "new_video"
}

func (nv *EnhancedNewVideoRecall) Weight() float64 {
	return 0.1
}

func (nv *EnhancedNewVideoRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	// 从时间线获取新视频
	key := "videos:timeline"
	results, err := nv.redis.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   fmt.Sprintf("%d", yesterday.Unix()),
		Max:   fmt.Sprintf("%d", now.Unix()),
		Count: int64(limit * 2),
	}).Result()

	if err != nil {
		return nil, err
	}

	// 结合用户偏好过滤
	userCategories := nv.getUserPreferredCategories(ctx, userID)
	
	videos := make([]int64, 0, limit)
	for _, vidStr := range results {
		vid, _ := strconv.ParseInt(vidStr, 10, 64)
		
		// 如果用户有偏好，优先返回匹配的新视频
		if len(userCategories) > 0 {
			videoCategory := nv.getVideoCategory(ctx, vid)
			if _, ok := userCategories[videoCategory]; ok {
				videos = append(videos, vid)
			}
		} else {
			videos = append(videos, vid)
		}

		if len(videos) >= limit {
			break
		}
	}

	return videos, nil
}

func (nv *EnhancedNewVideoRecall) getUserPreferredCategories(ctx context.Context, userID int64) map[string]bool {
	categoryKey := fmt.Sprintf("user:category_prefer:%d", userID)
	categories, err := nv.redis.ZRevRange(ctx, categoryKey, 0, 4).Result()
	if err != nil {
		return nil
	}

	result := make(map[string]bool)
	for _, cat := range categories {
		result[cat] = true
	}
	return result
}

func (nv *EnhancedNewVideoRecall) getVideoCategory(ctx context.Context, videoID int64) string {
	key := fmt.Sprintf("video:category:%d", videoID)
	cat, _ := nv.redis.Get(ctx, key).Result()
	return cat
}

// ===== 6. 相似视频召回 =====

// SimilarVideoRecall 相似视频召回（用于相关推荐）
type SimilarVideoRecall struct {
	BaseRecallStrategy
}

// NewSimilarVideoRecall 创建相似视频召回
func NewSimilarVideoRecall(redisClient *redis.Client, db *gorm.DB) *SimilarVideoRecall {
	return &SimilarVideoRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (sv *SimilarVideoRecall) Name() string {
	return "similar_video"
}

func (sv *SimilarVideoRecall) Weight() float64 {
	return 0.1
}

func (sv *SimilarVideoRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 获取用户最近观看的视频
	watchHistoryKey := fmt.Sprintf("user:watch_history:%d", userID)
	recentVideos, err := sv.redis.ZRevRange(ctx, watchHistoryKey, 0, 4).Result()
	if err != nil || len(recentVideos) == 0 {
		return nil, nil
	}

	videoScores := make(map[int64]float64)

	// 对每个最近观看的视频，获取相似视频
	for i, vidStr := range recentVideos {
		vid, _ := strconv.ParseInt(vidStr, 10, 64)
		weight := 1.0 - float64(i)*0.15 // 越近的视频权重越高

		// 从相似度缓存获取
		similarKey := fmt.Sprintf("video:similar:%d", vid)
		similar, _ := sv.redis.ZRevRangeWithScores(ctx, similarKey, 0, int64(limit/2)).Result()

		for _, s := range similar {
			simVid, _ := strconv.ParseInt(s.Member.(string), 10, 64)
			videoScores[simVid] += s.Score * weight
		}
	}

	return sv.topNVideosByScore(videoScores, limit), nil
}

// RecallByRefVideo 基于参考视频的相似召回
func (sv *SimilarVideoRecall) RecallByRefVideo(ctx context.Context, refVideoID int64, limit int) ([]int64, error) {
	similarKey := fmt.Sprintf("video:similar:%d", refVideoID)
	results, err := sv.redis.ZRevRange(ctx, similarKey, 0, int64(limit)).Result()
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

// ===== 7. 趋势视频召回 =====

// TrendingVideoRecall 趋势视频召回
type TrendingVideoRecall struct {
	BaseRecallStrategy
}

// NewTrendingVideoRecall 创建趋势视频召回
func NewTrendingVideoRecall(redisClient *redis.Client, db *gorm.DB) *TrendingVideoRecall {
	return &TrendingVideoRecall{
		BaseRecallStrategy: BaseRecallStrategy{
			redis: redisClient,
			db:    db,
		},
	}
}

func (tv *TrendingVideoRecall) Name() string {
	return "trending"
}

func (tv *TrendingVideoRecall) Weight() float64 {
	return 0.1
}

func (tv *TrendingVideoRecall) Recall(ctx context.Context, userID int64, limit int) ([]int64, error) {
	// 获取增长最快的视频（热度上升趋势）
	trendingKey := "trending:videos"
	results, err := tv.redis.ZRevRange(ctx, trendingKey, 0, int64(limit)).Result()
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
