package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ========================================
// 增强版排序模型
// ========================================

// EnhancedRankingModel 增强版排序模型
type EnhancedRankingModel struct {
	redis   *redis.Client
	db      *gorm.DB
	weights FeatureWeights
}

// FeatureWeights 特征权重配置
type FeatureWeights struct {
	// 用户特征权重
	UserActiveLevel   float64 `json:"user_active_level"`
	UserAvgWatchTime  float64 `json:"user_avg_watch_time"`
	UserInteractRate  float64 `json:"user_interact_rate"`

	// 视频质量权重
	VideoQualityScore float64 `json:"video_quality_score"`
	VideoCTR          float64 `json:"video_ctr"`
	VideoFinishRate   float64 `json:"video_finish_rate"`
	VideoLikeRate     float64 `json:"video_like_rate"`
	VideoCommentRate  float64 `json:"video_comment_rate"`
	VideoShareRate    float64 `json:"video_share_rate"`

	// 交叉特征权重
	UserAuthorAffinity float64 `json:"user_author_affinity"`
	UserCategoryMatch  float64 `json:"user_category_match"`
	UserTagOverlap     float64 `json:"user_tag_overlap"`

	// 新鲜度和热度权重
	VideoFreshness    float64 `json:"video_freshness"`
	RealtimeHotScore  float64 `json:"realtime_hot_score"`
	TrendingScore     float64 `json:"trending_score"`

	// 上下文权重
	TimeMatch      float64 `json:"time_match"`
	DeviceType     float64 `json:"device_type"`
	NetworkQuality float64 `json:"network_quality"`

	// 作者权重
	AuthorQuality   float64 `json:"author_quality"`
	AuthorInfluence float64 `json:"author_influence"`
}

// DefaultFeatureWeights 默认特征权重
func DefaultFeatureWeights() FeatureWeights {
	return FeatureWeights{
		// 用户特征
		UserActiveLevel:  0.03,
		UserAvgWatchTime: 0.02,
		UserInteractRate: 0.03,

		// 视频质量（核心指标）
		VideoQualityScore: 0.12,
		VideoCTR:          0.10,
		VideoFinishRate:   0.15,
		VideoLikeRate:     0.06,
		VideoCommentRate:  0.04,
		VideoShareRate:    0.05,

		// 交叉特征（个性化关键）
		UserAuthorAffinity: 0.10,
		UserCategoryMatch:  0.08,
		UserTagOverlap:     0.06,

		// 时效性
		VideoFreshness:   0.07,
		RealtimeHotScore: 0.05,
		TrendingScore:    0.04,

		// 上下文
		TimeMatch:      0.02,
		DeviceType:     0.01,
		NetworkQuality: 0.01,

		// 作者
		AuthorQuality:   0.03,
		AuthorInfluence: 0.03,
	}
}

// NewEnhancedRankingModel 创建增强版排序模型
func NewEnhancedRankingModel(redisClient *redis.Client, db *gorm.DB) *EnhancedRankingModel {
	return &EnhancedRankingModel{
		redis:   redisClient,
		db:      db,
		weights: DefaultFeatureWeights(),
	}
}

// SetWeights 设置权重
func (ltr *EnhancedRankingModel) SetWeights(weights FeatureWeights) {
	ltr.weights = weights
}

// Rank 对候选视频进行精排
func (ltr *EnhancedRankingModel) Rank(ctx context.Context, userID int64, videoIDs []int64) ([]ScoredVideo, error) {
	if len(videoIDs) == 0 {
		return nil, nil
	}

	// 1. 批量获取用户画像
	userProfile := ltr.getUserProfile(ctx, userID)

	// 2. 批量获取视频特征
	videoFeatures := ltr.batchGetVideoFeatures(ctx, videoIDs)

	// 3. 并行计算分数
	scoredVideos := make([]ScoredVideo, len(videoIDs))
	var wg sync.WaitGroup

	batchSize := 50
	for i := 0; i < len(videoIDs); i += batchSize {
		end := i + batchSize
		if end > len(videoIDs) {
			end = len(videoIDs)
		}

		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for j := start; j < end; j++ {
				videoID := videoIDs[j]
				feature := videoFeatures[videoID]
				features := ltr.extractFeatures(ctx, userProfile, feature, videoID)
				score := ltr.calculateScore(features)

				scoredVideos[j] = ScoredVideo{
					VideoID:  videoID,
					Score:    score,
					Features: features,
					Reasons:  ltr.generateReasons(features),
				}
			}
		}(i, end)
	}

	wg.Wait()

	// 4. 按分数降序排序
	sort.Slice(scoredVideos, func(i, j int) bool {
		return scoredVideos[i].Score > scoredVideos[j].Score
	})

	return scoredVideos, nil
}

// getUserProfile 获取用户画像
func (ltr *EnhancedRankingModel) getUserProfile(ctx context.Context, userID int64) *UserProfileData {
	profile := &UserProfileData{
		UserID:           userID,
		InterestTags:     make(map[string]float64),
		CategoryPrefer:   make(map[string]float64),
		AuthorPrefer:     make(map[int64]float64),
	}

	// 从 Redis 获取用户画像
	if ltr.redis == nil {
		return profile
	}
	pipe := ltr.redis.Pipeline()
	
	interestsCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:interests:%d", userID), 0, 19)
	categoryCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:category_prefer:%d", userID), 0, 9)
	authorCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:author_prefer:%d", userID), 0, 49)
	statsCmd := pipe.HGetAll(ctx, fmt.Sprintf("user:stats:%d", userID))
	
	pipe.Exec(ctx)

	// 解析兴趣标签
	if interests, err := interestsCmd.Result(); err == nil {
		for _, z := range interests {
			profile.InterestTags[z.Member.(string)] = z.Score
		}
	}

	// 解析分类偏好
	if categories, err := categoryCmd.Result(); err == nil {
		for _, z := range categories {
			profile.CategoryPrefer[z.Member.(string)] = z.Score
		}
	}

	// 解析作者偏好
	if authors, err := authorCmd.Result(); err == nil {
		for _, z := range authors {
			authorID, _ := strconv.ParseInt(z.Member.(string), 10, 64)
			profile.AuthorPrefer[authorID] = z.Score
		}
	}

	// 解析用户统计
	if stats, err := statsCmd.Result(); err == nil {
		profile.ActiveLevel, _ = strconv.ParseFloat(stats["active_level"], 64)
		profile.AvgWatchTime, _ = strconv.ParseFloat(stats["avg_watch_time"], 64)
		profile.InteractRate, _ = strconv.ParseFloat(stats["interact_rate"], 64)
	}

	return profile
}

// UserProfileData 用户画像数据
type UserProfileData struct {
	UserID         int64
	InterestTags   map[string]float64
	CategoryPrefer map[string]float64
	AuthorPrefer   map[int64]float64
	ActiveLevel    float64
	AvgWatchTime   float64
	InteractRate   float64
}

// VideoFeatureData 视频特征数据
type VideoFeatureData struct {
	VideoID        int64
	AuthorID       int64
	Category       string
	Tags           []string
	Duration       int64
	QualityScore   float64
	CTR            float64
	FinishRate     float64
	LikeRate       float64
	CommentRate    float64
	ShareRate      float64
	HotScore       float64
	TrendingScore  float64
	AuthorScore    float64
	CreatedAt      time.Time
}

// batchGetVideoFeatures 批量获取视频特征
func (ltr *EnhancedRankingModel) batchGetVideoFeatures(ctx context.Context, videoIDs []int64) map[int64]*VideoFeatureData {
	result := make(map[int64]*VideoFeatureData)

	if ltr.redis == nil {
		for _, vid := range videoIDs {
			result[vid] = ltr.getDefaultVideoFeature(vid)
		}
		return result
	}

	// 使用 Pipeline 批量获取
	pipe := ltr.redis.Pipeline()
	cmds := make(map[int64]*redis.StringCmd)

	for _, vid := range videoIDs {
		key := fmt.Sprintf("video:feature:%d", vid)
		cmds[vid] = pipe.Get(ctx, key)
	}

	pipe.Exec(ctx)

	// 解析结果
	for vid, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			// 缓存未命中，使用默认值
			result[vid] = ltr.getDefaultVideoFeature(vid)
			continue
		}

		var feature VideoFeatureData
		if err := json.Unmarshal([]byte(data), &feature); err != nil {
			result[vid] = ltr.getDefaultVideoFeature(vid)
			continue
		}
		result[vid] = &feature
	}

	return result
}

// getDefaultVideoFeature 获取默认视频特征
func (ltr *EnhancedRankingModel) getDefaultVideoFeature(videoID int64) *VideoFeatureData {
	return &VideoFeatureData{
		VideoID:      videoID,
		QualityScore: 0.5,
		CTR:          0.05,
		FinishRate:   0.3,
		LikeRate:     0.03,
		CommentRate:  0.01,
		ShareRate:    0.005,
		HotScore:     0.1,
	}
}

// extractFeatures 提取特征
func (ltr *EnhancedRankingModel) extractFeatures(ctx context.Context, userProfile *UserProfileData, videoFeature *VideoFeatureData, videoID int64) map[string]float64 {
	features := make(map[string]float64)

	// === 用户特征 ===
	features["user_active_level"] = ltr.normalizeActiveLevel(userProfile.ActiveLevel)
	features["user_avg_watch_time"] = ltr.normalizeWatchTime(userProfile.AvgWatchTime)
	features["user_interact_rate"] = userProfile.InteractRate

	// === 视频特征 ===
	if videoFeature != nil {
		features["video_quality_score"] = videoFeature.QualityScore
		features["video_ctr"] = videoFeature.CTR
		features["video_finish_rate"] = videoFeature.FinishRate
		features["video_like_rate"] = videoFeature.LikeRate
		features["video_comment_rate"] = videoFeature.CommentRate
		features["video_share_rate"] = videoFeature.ShareRate
		features["video_duration"] = ltr.normalizeDuration(videoFeature.Duration)
		features["video_freshness"] = ltr.calculateFreshness(videoFeature.CreatedAt)
		features["realtime_hot_score"] = videoFeature.HotScore
		features["trending_score"] = videoFeature.TrendingScore
		features["author_quality"] = videoFeature.AuthorScore

		// === 交叉特征 ===
		features["user_author_affinity"] = ltr.calculateAuthorAffinity(userProfile, videoFeature.AuthorID)
		features["user_category_match"] = ltr.calculateCategoryMatch(userProfile, videoFeature.Category)
		features["user_tag_overlap"] = ltr.calculateTagOverlap(userProfile, videoFeature.Tags)
	} else {
		// 默认值
		features["video_quality_score"] = 0.5
		features["video_ctr"] = 0.05
		features["video_finish_rate"] = 0.3
		features["video_like_rate"] = 0.03
		features["video_comment_rate"] = 0.01
		features["video_share_rate"] = 0.005
		features["video_duration"] = 0.5
		features["video_freshness"] = 0.5
		features["realtime_hot_score"] = 0.3
		features["trending_score"] = 0.2
		features["author_quality"] = 0.5
		features["user_author_affinity"] = 0.3
		features["user_category_match"] = 0.3
		features["user_tag_overlap"] = 0.2
	}

	// === 上下文特征 ===
	features["time_match"] = ltr.calculateTimeMatch()
	features["device_type"] = 1.0 // 默认移动端
	features["network_quality"] = 0.9

	return features
}

// calculateScore 计算综合分数
func (ltr *EnhancedRankingModel) calculateScore(features map[string]float64) float64 {
	score := 0.0

	// 用户特征
	score += ltr.weights.UserActiveLevel * features["user_active_level"]
	score += ltr.weights.UserAvgWatchTime * features["user_avg_watch_time"]
	score += ltr.weights.UserInteractRate * features["user_interact_rate"]

	// 视频质量
	score += ltr.weights.VideoQualityScore * features["video_quality_score"]
	score += ltr.weights.VideoCTR * features["video_ctr"] * 10 // CTR通常较小，放大
	score += ltr.weights.VideoFinishRate * features["video_finish_rate"]
	score += ltr.weights.VideoLikeRate * features["video_like_rate"] * 10
	score += ltr.weights.VideoCommentRate * features["video_comment_rate"] * 20
	score += ltr.weights.VideoShareRate * features["video_share_rate"] * 30

	// 交叉特征
	score += ltr.weights.UserAuthorAffinity * features["user_author_affinity"]
	score += ltr.weights.UserCategoryMatch * features["user_category_match"]
	score += ltr.weights.UserTagOverlap * features["user_tag_overlap"]

	// 时效性
	score += ltr.weights.VideoFreshness * features["video_freshness"]
	score += ltr.weights.RealtimeHotScore * features["realtime_hot_score"]
	score += ltr.weights.TrendingScore * features["trending_score"]

	// 上下文
	score += ltr.weights.TimeMatch * features["time_match"]
	score += ltr.weights.DeviceType * features["device_type"]
	score += ltr.weights.NetworkQuality * features["network_quality"]

	// 作者
	score += ltr.weights.AuthorQuality * features["author_quality"]

	// 应用 sigmoid 激活函数
	score = 1.0 / (1.0 + math.Exp(-score*5))

	return score
}

// generateReasons 生成推荐理由
func (ltr *EnhancedRankingModel) generateReasons(features map[string]float64) []string {
	reasons := make([]string, 0)

	// 根据特征值生成个性化理由
	if features["user_author_affinity"] > 0.6 {
		reasons = append(reasons, "你关注的创作者")
	}

	if features["video_finish_rate"] > 0.7 {
		reasons = append(reasons, "高完播率内容")
	}

	if features["realtime_hot_score"] > 0.7 {
		reasons = append(reasons, "正在热播")
	}

	if features["trending_score"] > 0.6 {
		reasons = append(reasons, "热度上升中")
	}

	if features["user_category_match"] > 0.7 {
		reasons = append(reasons, "你可能感兴趣")
	}

	if features["video_freshness"] > 0.8 {
		reasons = append(reasons, "最新发布")
	}

	if features["video_like_rate"] > 0.08 {
		reasons = append(reasons, "高赞内容")
	}

	if features["user_tag_overlap"] > 0.6 {
		reasons = append(reasons, "根据你的喜好推荐")
	}

	if features["author_quality"] > 0.8 {
		reasons = append(reasons, "优质创作者")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "为你推荐")
	}

	// 限制理由数量
	if len(reasons) > 2 {
		reasons = reasons[:2]
	}

	return reasons
}

// ========================================
// 特征计算辅助函数
// ========================================

func (ltr *EnhancedRankingModel) normalizeActiveLevel(level float64) float64 {
	// 将活跃等级归一化到 [0, 1]
	return math.Min(level/10.0, 1.0)
}

func (ltr *EnhancedRankingModel) normalizeWatchTime(watchTime float64) float64 {
	// 将观看时长归一化，假设理想值为 60 秒
	return math.Min(watchTime/60.0, 1.0)
}

func (ltr *EnhancedRankingModel) normalizeDuration(duration int64) float64 {
	// 视频时长归一化，60秒为最优
	optimal := 60.0
	d := float64(duration)
	if d < optimal {
		return d / optimal
	}
	// 超过最优时长，分数下降
	return optimal / d
}

func (ltr *EnhancedRankingModel) calculateFreshness(createdAt time.Time) float64 {
	// 新鲜度计算：时间衰减
	hoursOld := time.Since(createdAt).Hours()

	// 指数衰减：半衰期为 24 小时
	halfLife := 24.0
	freshness := math.Pow(0.5, hoursOld/halfLife)

	return freshness
}

func (ltr *EnhancedRankingModel) calculateAuthorAffinity(userProfile *UserProfileData, authorID int64) float64 {
	if score, ok := userProfile.AuthorPrefer[authorID]; ok {
		return math.Min(score/10.0, 1.0)
	}
	return 0.1 // 默认亲和度
}

func (ltr *EnhancedRankingModel) calculateCategoryMatch(userProfile *UserProfileData, category string) float64 {
	if category == "" {
		return 0.2
	}
	if score, ok := userProfile.CategoryPrefer[category]; ok {
		return math.Min(score/10.0, 1.0)
	}
	return 0.2 // 默认匹配度
}

func (ltr *EnhancedRankingModel) calculateTagOverlap(userProfile *UserProfileData, videoTags []string) float64 {
	if len(videoTags) == 0 || len(userProfile.InterestTags) == 0 {
		return 0.2
	}

	// 计算标签重叠度
	totalWeight := 0.0
	matchWeight := 0.0

	for tag, weight := range userProfile.InterestTags {
		totalWeight += weight
		for _, videoTag := range videoTags {
			if tag == videoTag {
				matchWeight += weight
				break
			}
		}
	}

	if totalWeight == 0 {
		return 0.2
	}

	return matchWeight / totalWeight
}

func (ltr *EnhancedRankingModel) calculateTimeMatch() float64 {
	// 基于当前时段的匹配度
	hour := time.Now().Hour()

	// 晚高峰时段
	if hour >= 18 && hour <= 23 {
		return 1.0
	}
	// 午间时段
	if hour >= 12 && hour <= 14 {
		return 0.9
	}
	// 早间时段
	if hour >= 7 && hour <= 9 {
		return 0.8
	}
	// 其他时段
	return 0.7
}

// ========================================
// Thompson Sampling (探索与利用平衡)
// ========================================

// ThompsonSamplingRanker Thompson Sampling 排序器
type ThompsonSamplingRanker struct {
	redis *redis.Client
}

// NewThompsonSamplingRanker 创建 Thompson Sampling 排序器
func NewThompsonSamplingRanker(redisClient *redis.Client) *ThompsonSamplingRanker {
	return &ThompsonSamplingRanker{redis: redisClient}
}

// SelectWithExploration 使用 Thompson Sampling 选择视频（平衡探索和利用）
func (tsr *ThompsonSamplingRanker) SelectWithExploration(ctx context.Context, scoredVideos []ScoredVideo, explorationRatio float64) []ScoredVideo {
	if len(scoredVideos) == 0 {
		return scoredVideos
	}

	explorationCount := int(float64(len(scoredVideos)) * explorationRatio)
	if explorationCount == 0 {
		return scoredVideos
	}

	// 分离利用和探索
	exploitation := scoredVideos[:len(scoredVideos)-explorationCount]
	exploration := scoredVideos[len(scoredVideos)-explorationCount:]

	// 对探索部分使用 Thompson Sampling
	for i := range exploration {
		alpha, beta := tsr.getVideoStats(ctx, exploration[i].VideoID)
		exploration[i].Score = tsr.sampleBeta(alpha, beta)
	}

	// 重新排序探索部分
	sort.Slice(exploration, func(i, j int) bool {
		return exploration[i].Score > exploration[j].Score
	})

	// 均匀插入探索内容
	result := make([]ScoredVideo, 0, len(scoredVideos))
	interval := len(exploitation) / (len(exploration) + 1)
	
	explorationIdx := 0
	for i, video := range exploitation {
		result = append(result, video)
		if interval > 0 && (i+1)%interval == 0 && explorationIdx < len(exploration) {
			result = append(result, exploration[explorationIdx])
			explorationIdx++
		}
	}

	// 添加剩余探索内容
	for ; explorationIdx < len(exploration); explorationIdx++ {
		result = append(result, exploration[explorationIdx])
	}

	return result
}

func (tsr *ThompsonSamplingRanker) getVideoStats(ctx context.Context, videoID int64) (float64, float64) {
	// 从 Redis 获取视频的 CTR 统计
	key := fmt.Sprintf("video:ctr_stats:%d", videoID)
	stats, err := tsr.redis.HGetAll(ctx, key).Result()
	if err != nil {
		return 1.0, 1.0 // 默认先验
	}

	clicks, _ := strconv.ParseFloat(stats["clicks"], 64)
	impressions, _ := strconv.ParseFloat(stats["impressions"], 64)

	// Beta 分布参数
	alpha := clicks + 1
	beta := impressions - clicks + 1

	return alpha, beta
}

func (tsr *ThompsonSamplingRanker) sampleBeta(alpha, beta float64) float64 {
	// 简化的 Beta 分布期望值（实际应使用随机采样）
	// E[Beta(α, β)] = α / (α + β)
	return alpha / (alpha + beta)
}
