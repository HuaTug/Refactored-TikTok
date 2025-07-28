package agent

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// PerceptionModule 感知模块
type PerceptionModule struct {
	config *AgentConfig
	stats  *PerceptionStats
}

// PerceptionStats 感知模块统计信息
type PerceptionStats struct {
	TotalAnalyses     int64     `json:"total_analyses"`
	BoredDetections   int64     `json:"bored_detections"`
	EngagedDetections int64     `json:"engaged_detections"`
	LastAnalysisTime  time.Time `json:"last_analysis_time"`
}

// NewPerceptionModule 创建感知模块
func NewPerceptionModule(config *AgentConfig) *PerceptionModule {
	return &PerceptionModule{
		config: config,
		stats:  &PerceptionStats{},
	}
}

// AnalyzeUserState 分析用户状态
func (pm *PerceptionModule) AnalyzeUserState(
	ctx context.Context,
	userID int64,
	behaviorSequence *BehaviorSequence,
	userProfile *UserProfile,
	userContext *UserContext,
) (*UserState, error) {

	pm.stats.TotalAnalyses++
	pm.stats.LastAnalysisTime = time.Now()

	// 分析参与度
	engagementLevel := pm.analyzeEngagement(behaviorSequence)

	// 分析探索度
	explorationLevel := pm.analyzeExploration(behaviorSequence)

	// 提取当前兴趣
	currentInterests := pm.extractCurrentInterests(behaviorSequence)

	// 更新统计
	if engagementLevel == EngagementBored {
		pm.stats.BoredDetections++
	} else if engagementLevel >= EngagementEngaged {
		pm.stats.EngagedDetections++
	}

	userState := &UserState{
		UserID:           userID,
		EngagementLevel:  engagementLevel,
		ExplorationLevel: explorationLevel,
		CurrentInterests: currentInterests,
		LongTermProfile:  userProfile,
		Context:          userContext,
	}

	hlog.Infof("User state analysis completed for user %d: engagement=%v, exploration=%v, interests=%v",
		userID, engagementLevel, explorationLevel, currentInterests)

	return userState, nil
}

// analyzeEngagement 分析用户参与度
func (pm *PerceptionModule) analyzeEngagement(behaviorSequence *BehaviorSequence) EngagementLevel {
	if len(behaviorSequence.Behaviors) == 0 {
		return EngagementCasual
	}

	// 计算平均完播率
	totalCompletionRate := 0.0
	viewCount := 0
	skipCount := 0
	interactionCount := 0

	for _, behavior := range behaviorSequence.Behaviors {
		switch behavior.BehaviorType {
		case BehaviorView:
			totalCompletionRate += behavior.CompletionRate
			viewCount++
		case BehaviorSkip:
			skipCount++
		case BehaviorLike, BehaviorShare, BehaviorComment, BehaviorFollow:
			interactionCount++
		}
	}

	if viewCount == 0 {
		return EngagementCasual
	}

	avgCompletionRate := totalCompletionRate / float64(viewCount)
	skipRate := float64(skipCount) / float64(len(behaviorSequence.Behaviors))
	interactionRate := float64(interactionCount) / float64(viewCount)

	// 判断参与度
	if avgCompletionRate < pm.config.BoredThreshold && skipRate > 0.7 {
		return EngagementBored
	} else if avgCompletionRate > pm.config.DeepInterestThreshold && interactionRate > 0.3 {
		return EngagementImmersed
	} else if avgCompletionRate > 0.5 && interactionRate > 0.1 {
		return EngagementEngaged
	}

	return EngagementCasual
}

// analyzeExploration 分析用户探索度
func (pm *PerceptionModule) analyzeExploration(behaviorSequence *BehaviorSequence) ExplorationLevel {
	if len(behaviorSequence.Behaviors) == 0 {
		return ExplorationMixed
	}

	// 统计不同类型内容的观看
	categoryCount := make(map[string]int)
	topicCount := make(map[string]int)

	for _, behavior := range behaviorSequence.Behaviors {
		if behavior.BehaviorType == BehaviorView {
			// 这里需要从视频信息中获取分类和主题
			// 简化处理，使用视频ID的模运算来模拟不同分类
			category := fmt.Sprintf("category_%d", behavior.VideoID%10)
			topic := fmt.Sprintf("topic_%d", behavior.VideoID%20)

			categoryCount[category]++
			topicCount[topic]++
		}
	}

	// 计算多样性指数
	categoryDiversity := pm.calculateDiversity(categoryCount)
	topicDiversity := pm.calculateDiversity(topicCount)

	avgDiversity := (categoryDiversity + topicDiversity) / 2

	if avgDiversity > 0.8 {
		return ExplorationDiverse
	} else if avgDiversity < 0.3 {
		return ExplorationFocused
	}

	return ExplorationMixed
}

// calculateDiversity 计算多样性指数（使用香农熵）
func (pm *PerceptionModule) calculateDiversity(counts map[string]int) float64 {
	if len(counts) <= 1 {
		return 0.0
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	if total == 0 {
		return 0.0
	}

	entropy := 0.0
	for _, count := range counts {
		if count > 0 {
			p := float64(count) / float64(total)
			entropy -= p * math.Log2(p)
		}
	}

	// 归一化到0-1范围
	maxEntropy := math.Log2(float64(len(counts)))
	if maxEntropy == 0 {
		return 0.0
	}

	return entropy / maxEntropy
}

// extractCurrentInterests 提取当前兴趣
func (pm *PerceptionModule) extractCurrentInterests(behaviorSequence *BehaviorSequence) []string {
	interestScores := make(map[string]float64)

	for _, behavior := range behaviorSequence.Behaviors {
		// 根据行为类型和完播率计算兴趣分数
		score := pm.calculateInterestScore(behavior)

		// 简化处理，使用视频ID模拟主题
		topic := fmt.Sprintf("topic_%d", behavior.VideoID%20)
		interestScores[topic] += score
	}

	// 排序并返回前N个兴趣
	type interestPair struct {
		topic string
		score float64
	}

	var interests []interestPair
	for topic, score := range interestScores {
		interests = append(interests, interestPair{topic, score})
	}

	sort.Slice(interests, func(i, j int) bool {
		return interests[i].score > interests[j].score
	})

	// 返回前5个兴趣
	var result []string
	maxInterests := 5
	if len(interests) < maxInterests {
		maxInterests = len(interests)
	}

	for i := 0; i < maxInterests; i++ {
		if interests[i].score > 0.1 { // 过滤低分兴趣
			result = append(result, interests[i].topic)
		}
	}

	return result
}

// calculateInterestScore 计算兴趣分数
func (pm *PerceptionModule) calculateInterestScore(behavior Behavior) float64 {
	baseScore := 0.0

	switch behavior.BehaviorType {
	case BehaviorView:
		baseScore = behavior.CompletionRate
	case BehaviorLike:
		baseScore = 2.0
	case BehaviorShare:
		baseScore = 3.0
	case BehaviorComment:
		baseScore = 4.0
	case BehaviorFollow:
		baseScore = 5.0
	case BehaviorRewatch:
		baseScore = 3.0
	case BehaviorProfileView:
		baseScore = 2.5
	case BehaviorSkip:
		baseScore = -0.5
	}

	// 根据交互深度调整分数
	depthMultiplier := 1.0 + float64(behavior.InteractionDepth)*0.2

	return baseScore * depthMultiplier
}

// DetectBehaviorPatterns 检测行为模式
func (pm *PerceptionModule) DetectBehaviorPatterns(behaviorSequence *BehaviorSequence) []string {
	var patterns []string

	if len(behaviorSequence.Behaviors) < 3 {
		return patterns
	}

	// 检测快速滑动模式
	if pm.detectRapidScrolling(behaviorSequence) {
		patterns = append(patterns, "rapid_scrolling")
	}

	// 检测深度浏览模式
	if pm.detectDeepBrowsing(behaviorSequence) {
		patterns = append(patterns, "deep_browsing")
	}

	// 检测重复观看模式
	if pm.detectRepeatViewing(behaviorSequence) {
		patterns = append(patterns, "repeat_viewing")
	}

	// 检测社交互动模式
	if pm.detectSocialInteraction(behaviorSequence) {
		patterns = append(patterns, "social_interaction")
	}

	return patterns
}

// detectRapidScrolling 检测快速滑动
func (pm *PerceptionModule) detectRapidScrolling(behaviorSequence *BehaviorSequence) bool {
	skipCount := 0
	totalBehaviors := len(behaviorSequence.Behaviors)

	for _, behavior := range behaviorSequence.Behaviors {
		if behavior.BehaviorType == BehaviorSkip ||
			(behavior.BehaviorType == BehaviorView && behavior.CompletionRate < 0.1) {
			skipCount++
		}
	}

	return float64(skipCount)/float64(totalBehaviors) > 0.6
}

// detectDeepBrowsing 检测深度浏览
func (pm *PerceptionModule) detectDeepBrowsing(behaviorSequence *BehaviorSequence) bool {
	deepViewCount := 0

	for _, behavior := range behaviorSequence.Behaviors {
		if behavior.BehaviorType == BehaviorView && behavior.CompletionRate > 0.8 {
			deepViewCount++
		}
	}

	return deepViewCount >= 3
}

// detectRepeatViewing 检测重复观看
func (pm *PerceptionModule) detectRepeatViewing(behaviorSequence *BehaviorSequence) bool {
	videoViews := make(map[int64]int)

	for _, behavior := range behaviorSequence.Behaviors {
		if behavior.BehaviorType == BehaviorView || behavior.BehaviorType == BehaviorRewatch {
			videoViews[behavior.VideoID]++
		}
	}

	for _, count := range videoViews {
		if count > 1 {
			return true
		}
	}

	return false
}

// detectSocialInteraction 检测社交互动
func (pm *PerceptionModule) detectSocialInteraction(behaviorSequence *BehaviorSequence) bool {
	socialCount := 0

	for _, behavior := range behaviorSequence.Behaviors {
		if behavior.BehaviorType == BehaviorLike ||
			behavior.BehaviorType == BehaviorShare ||
			behavior.BehaviorType == BehaviorComment ||
			behavior.BehaviorType == BehaviorFollow {
			socialCount++
		}
	}

	return socialCount >= 2
}

// GetStats 获取感知模块统计信息
func (pm *PerceptionModule) GetStats() *PerceptionStats {
	return pm.stats
}

// UpdateConfig 更新配置
func (pm *PerceptionModule) UpdateConfig(config *AgentConfig) error {
	pm.config = config
	return nil
}
