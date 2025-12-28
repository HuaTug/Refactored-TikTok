package recommendation

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-redis/redis/v8"
)

// RecommendationEngine 推荐引擎
type RecommendationEngine struct {
	redis            *redis.Client
	recallStrategies []RecallStrategy
	rankingModel     RankingModel
}

// RecallStrategy 召回策略接口
type RecallStrategy interface {
	Name() string
	Recall(ctx context.Context, userID int64, limit int) ([]int64, error)
	Weight() float64
}

// RankingModel 排序模型接口
type RankingModel interface {
	Rank(ctx context.Context, userID int64, videoIDs []int64) ([]ScoredVideo, error)
}

// ScoredVideo 带分数的视频
type ScoredVideo struct {
	VideoID  int64
	Score    float64
	Reasons  []string // 推荐理由
	Features map[string]float64
}

// UserProfile 用户画像
type UserProfile struct {
	UserID           int64
	InterestTags     map[string]float64 // 兴趣标签权重
	CategoryPrefer   map[string]float64 // 分类偏好
	AuthorPrefer     map[int64]float64  // 作者偏好
	TimePrefer       []int              // 活跃时段
	AvgWatchDuration float64            // 平均观看时长
	InteractRate     float64            // 互动率
	UpdatedAt        time.Time
}

// VideoFeature 视频特征
type VideoFeature struct {
	VideoID         int64
	Tags            []string
	Category        string
	AuthorID        int64
	Duration        int64
	Quality         float64 // 内容质量分
	Freshness       float64 // 新鲜度
	PopularityScore float64 // 热度分
	CTR             float64 // 点击率
	FinishRate      float64 // 完播率
	InteractScore   float64 // 互动分
	CreatedAt       time.Time
}

// NewRecommendationEngine 创建推荐引擎
func NewRecommendationEngine(redisClient *redis.Client) *RecommendationEngine {
	engine := &RecommendationEngine{
		redis:            redisClient,
		recallStrategies: make([]RecallStrategy, 0),
	}

	// 注册召回策略
	engine.RegisterStrategy(&CollaborativeFilteringRecall{redis: redisClient})
	engine.RegisterStrategy(&HotVideoRecall{redis: redisClient})
	engine.RegisterStrategy(&ContentBasedRecall{redis: redisClient})
	engine.RegisterStrategy(&SocialRecall{redis: redisClient})
	engine.RegisterStrategy(&NewVideoRecall{redis: redisClient})

	// 设置排序模型
	engine.rankingModel = NewLearningToRankModel(redisClient)

	return engine
}

// RegisterStrategy 注册召回策略
func (re *RecommendationEngine) RegisterStrategy(strategy RecallStrategy) {
	re.recallStrategies = append(re.recallStrategies, strategy)
}

// Recommend 生成推荐列表
func (re *RecommendationEngine) Recommend(ctx context.Context, userID int64, limit int) ([]ScoredVideo, error) {
	// 1. 多路召回
	candidateVideos := make(map[int64]bool)

	for _, strategy := range re.recallStrategies {
		recallLimit := int(float64(limit) * 3 * strategy.Weight()) // 召回3倍候选
		videos, err := strategy.Recall(ctx, userID, recallLimit)
		if err != nil {
			continue // 单路召回失败不影响整体
		}

		for _, vid := range videos {
			candidateVideos[vid] = true
		}
	}

	// 转为切片
	candidates := make([]int64, 0, len(candidateVideos))
	for vid := range candidateVideos {
		candidates = append(candidates, vid)
	}

	// 2. 精排
	rankedVideos, err := re.rankingModel.Rank(ctx, userID, candidates)
	if err != nil {
		return nil, err
	}

	// 3. 重排序 (多样性、新鲜度)
	finalResults := re.Rerank(rankedVideos, limit)

	// 4. 过滤已观看
	finalResults = re.FilterWatched(ctx, userID, finalResults)

	if len(finalResults) > limit {
		finalResults = finalResults[:limit]
	}

	return finalResults, nil
}

// Rerank 重排序,保证多样性
func (re *RecommendationEngine) Rerank(videos []ScoredVideo, limit int) []ScoredVideo {
	// MMR (Maximal Marginal Relevance) 算法
	// 平衡相关性和多样性

	if len(videos) <= limit {
		return videos
	}

	selected := make([]ScoredVideo, 0, limit)
	remaining := videos
	lambda := 0.7 // 相关性权重

	// 选择分数最高的作为第一个
	selected = append(selected, remaining[0])
	remaining = remaining[1:]

	for len(selected) < limit && len(remaining) > 0 {
		maxScore := -math.MaxFloat64
		maxIdx := -1

		for i, video := range remaining {
			// 计算与已选视频的最小相似度
			minSim := 1.0
			for _, s := range selected {
				sim := re.calculateSimilarity(video, s)
				if sim < minSim {
					minSim = sim
				}
			}

			// MMR 分数 = λ * 相关性 - (1-λ) * 相似度
			mmrScore := lambda*video.Score - (1-lambda)*minSim

			if mmrScore > maxScore {
				maxScore = mmrScore
				maxIdx = i
			}
		}

		if maxIdx >= 0 {
			selected = append(selected, remaining[maxIdx])
			remaining = append(remaining[:maxIdx], remaining[maxIdx+1:]...)
		}
	}

	return selected
}

// calculateSimilarity 计算两个视频的相似度
func (re *RecommendationEngine) calculateSimilarity(v1, v2 ScoredVideo) float64 {
	// 基于特征向量计算余弦相似度
	dotProduct := 0.0
	norm1 := 0.0
	norm2 := 0.0

	for key := range v1.Features {
		if val2, ok := v2.Features[key]; ok {
			dotProduct += v1.Features[key] * val2
		}
		norm1 += v1.Features[key] * v1.Features[key]
	}

	for _, val := range v2.Features {
		norm2 += val * val
	}

	if norm1 == 0 || norm2 == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// FilterWatched 过滤已观看视频
func (re *RecommendationEngine) FilterWatched(ctx context.Context, userID int64, videos []ScoredVideo) []ScoredVideo {
	// 从 Redis 获取用户观看历史
	watchedKey := formatWatchHistoryKey(userID)

	filtered := make([]ScoredVideo, 0, len(videos))
	for _, video := range videos {
		watched, err := re.redis.SIsMember(ctx, watchedKey, video.VideoID).Result()
		if err != nil || !watched {
			filtered = append(filtered, video)
		}
	}

	return filtered
}

// UpdateUserProfile 更新用户画像
func (re *RecommendationEngine) UpdateUserProfile(ctx context.Context, userID int64, action string, videoID int64) error {
	// 根据用户行为更新画像
	// action: view, like, comment, share, finish

	// 实现用户画像实时更新逻辑
	return nil
}

func formatWatchHistoryKey(userID int64) string {
	return fmt.Sprintf("user:watch_history:%d", userID)
}
