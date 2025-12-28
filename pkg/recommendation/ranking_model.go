package recommendation

import (
	"context"
	"math"
	"sort"

	"github.com/go-redis/redis/v8"
)

// LearningToRankModel Learning to Rank 排序模型
type LearningToRankModel struct {
	redis *redis.Client
}

func NewLearningToRankModel(redisClient *redis.Client) *LearningToRankModel {
	return &LearningToRankModel{
		redis: redisClient,
	}
}

// Rank 对候选视频进行精排
func (ltr *LearningToRankModel) Rank(ctx context.Context, userID int64, videoIDs []int64) ([]ScoredVideo, error) {
	scoredVideos := make([]ScoredVideo, 0, len(videoIDs))

	for _, videoID := range videoIDs {
		// 计算综合分数
		features := ltr.extractFeatures(ctx, userID, videoID)
		score := ltr.calculateScore(features)

		scoredVideos = append(scoredVideos, ScoredVideo{
			VideoID:  videoID,
			Score:    score,
			Features: features,
			Reasons:  ltr.generateReasons(features),
		})
	}

	// 按分数降序排序
	sort.Slice(scoredVideos, func(i, j int) bool {
		return scoredVideos[i].Score > scoredVideos[j].Score
	})

	return scoredVideos, nil
}

// extractFeatures 提取特征
func (ltr *LearningToRankModel) extractFeatures(ctx context.Context, userID, videoID int64) map[string]float64 {
	features := make(map[string]float64)

	// === 用户特征 ===
	// 这里应该从数据库或缓存获取,简化示例
	features["user_active_level"] = 0.8    // 用户活跃度
	features["user_avg_watch_time"] = 45.0 // 平均观看时长(秒)
	features["user_interact_rate"] = 0.15  // 互动率

	// === 视频特征 ===
	features["video_quality_score"] = 0.85 // 内容质量分
	features["video_duration"] = 60.0      // 视频时长
	features["video_freshness"] = 0.9      // 新鲜度(时间衰减)
	features["video_ctr"] = 0.12           // 点击率
	features["video_finish_rate"] = 0.65   // 完播率
	features["video_like_rate"] = 0.08     // 点赞率
	features["video_comment_rate"] = 0.03  // 评论率
	features["video_share_rate"] = 0.02    // 分享率

	// === 交叉特征 ===
	features["user_author_affinity"] = 0.7 // 用户对作者的亲和度
	features["user_category_match"] = 0.75 // 用户对分类的匹配度
	features["user_tag_overlap"] = 0.6     // 用户兴趣标签重叠度

	// === 上下文特征 ===
	features["time_match"] = 0.8      // 时间匹配度
	features["device_type"] = 1.0     // 设备类型(mobile=1, pc=0.8)
	features["network_quality"] = 0.9 // 网络质量

	// === 热度特征 ===
	features["realtime_hot_score"] = 0.7 // 实时热度
	features["trending_score"] = 0.65    // 趋势分

	return features
}

// calculateScore 计算综合分数
func (ltr *LearningToRankModel) calculateScore(features map[string]float64) float64 {
	// 使用加权线性模型 (实际应使用GBDT/XGBoost/DeepFM等)
	weights := map[string]float64{
		// 用户特征权重
		"user_active_level":   0.05,
		"user_avg_watch_time": 0.03,
		"user_interact_rate":  0.04,

		// 视频质量权重
		"video_quality_score": 0.15,
		"video_ctr":           0.12,
		"video_finish_rate":   0.18,
		"video_like_rate":     0.08,
		"video_comment_rate":  0.05,
		"video_share_rate":    0.06,

		// 交叉特征权重
		"user_author_affinity": 0.10,
		"user_category_match":  0.09,
		"user_tag_overlap":     0.07,

		// 新鲜度和热度权重
		"video_freshness":    0.08,
		"realtime_hot_score": 0.06,
		"trending_score":     0.05,

		// 上下文权重
		"time_match":      0.04,
		"device_type":     0.02,
		"network_quality": 0.03,
	}

	score := 0.0
	for feature, value := range features {
		if weight, ok := weights[feature]; ok {
			score += weight * ltr.normalize(value, feature)
		}
	}

	// 应用激活函数 (sigmoid)
	score = 1.0 / (1.0 + math.Exp(-score))

	return score
}

// normalize 特征归一化
func (ltr *LearningToRankModel) normalize(value float64, feature string) float64 {
	// 不同特征使用不同的归一化方法
	switch feature {
	case "video_duration":
		// 时长归一化到 [0, 1], 假设最优时长为60秒
		optimal := 60.0
		return 1.0 - math.Abs(value-optimal)/optimal

	case "user_avg_watch_time":
		// 观看时长归一化
		return math.Min(value/60.0, 1.0)

	default:
		// 大部分特征已经在 [0, 1] 范围内
		return value
	}
}

// generateReasons 生成推荐理由
func (ltr *LearningToRankModel) generateReasons(features map[string]float64) []string {
	reasons := make([]string, 0)

	// 根据特征生成个性化推荐理由
	if features["user_author_affinity"] > 0.7 {
		reasons = append(reasons, "你关注的作者发布的")
	}

	if features["video_finish_rate"] > 0.7 {
		reasons = append(reasons, "高完播率内容")
	}

	if features["realtime_hot_score"] > 0.8 {
		reasons = append(reasons, "正在热播")
	}

	if features["user_category_match"] > 0.75 {
		reasons = append(reasons, "你可能感兴趣的分类")
	}

	if features["video_freshness"] > 0.85 {
		reasons = append(reasons, "最新发布")
	}

	if features["video_like_rate"] > 0.1 {
		reasons = append(reasons, "高互动内容")
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "为你推荐")
	}

	return reasons
}

// ===== 多臂老虎机算法 (Exploration vs Exploitation) =====

// ThompsonSamplingBandit Thompson Sampling 算法
type ThompsonSamplingBandit struct {
	redis *redis.Client
}

// SelectVideo 使用 Thompson Sampling 选择视频 (平衡探索和利用)
func (tsb *ThompsonSamplingBandit) SelectVideo(ctx context.Context, candidates []int64, userID int64) int64 {
	// Thompson Sampling: 从 Beta 分布中采样
	// Beta(α, β) 其中 α = 成功次数 + 1, β = 失败次数 + 1

	maxSample := 0.0
	selectedVideo := candidates[0]

	for _, videoID := range candidates {
		// 获取视频的历史表现
		alpha, beta := tsb.getVideoStats(ctx, videoID)

		// 从 Beta 分布采样
		sample := tsb.sampleBeta(alpha, beta)

		if sample > maxSample {
			maxSample = sample
			selectedVideo = videoID
		}
	}

	return selectedVideo
}

func (tsb *ThompsonSamplingBandit) getVideoStats(ctx context.Context, videoID int64) (float64, float64) {
	// 从 Redis 获取视频的点击和曝光数据
	// 简化示例: 返回模拟数据
	return 10.0, 5.0 // alpha (clicks), beta (impressions - clicks)
}

func (tsb *ThompsonSamplingBandit) sampleBeta(alpha, beta float64) float64 {
	// 简化的 Beta 分布采样 (实际应使用专业库)
	// 这里使用期望值作为近似: E[Beta(α,β)] = α/(α+β)
	return alpha / (alpha + beta)
}
