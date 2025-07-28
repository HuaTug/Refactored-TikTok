package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// ActionModule 行动模块
type ActionModule struct {
	config              *AgentConfig
	videoService        VideoService
	userService         UserService
	recommendationCache *RecommendationCache
	stats               *ActionStats
}

// ActionStats 行动模块统计信息
type ActionStats struct {
	TotalRecommendations  int64                        `json:"total_recommendations"`
	RecommendationsByMode map[RecommendationMode]int64 `json:"recommendations_by_mode"`
	AverageResponseTime   time.Duration                `json:"average_response_time"`
	CacheHitRate          float64                      `json:"cache_hit_rate"`
	LastActionTime        time.Time                    `json:"last_action_time"`
}

// VideoService 视频服务接口
type VideoService interface {
	GetVideosByCategory(ctx context.Context, category string, limit int) ([]Video, error)
	GetTrendingVideos(ctx context.Context, limit int) ([]Video, error)
	GetPersonalizedVideos(ctx context.Context, userID int64, interests []string, limit int) ([]Video, error)
	GetSimilarVideos(ctx context.Context, videoID int64, limit int) ([]Video, error)
	GetVideosByTags(ctx context.Context, tags []string, limit int) ([]Video, error)
}

// UserService 用户服务接口
type UserService interface {
	GetUserInteractionHistory(ctx context.Context, userID int64, days int) ([]UserInteraction, error)
	GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)
	UpdateUserInterests(ctx context.Context, userID int64, interests []string) error
}

// Video 视频信息
type Video struct {
	VideoID      int64                  `json:"video_id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Category     string                 `json:"category"`
	Tags         []string               `json:"tags"`
	AuthorID     int64                  `json:"author_id"`
	AuthorName   string                 `json:"author_name"`
	Duration     time.Duration          `json:"duration"`
	ViewCount    int64                  `json:"view_count"`
	LikeCount    int64                  `json:"like_count"`
	ShareCount   int64                  `json:"share_count"`
	CommentCount int64                  `json:"comment_count"`
	CreatedAt    time.Time              `json:"created_at"`
	Quality      VideoQuality           `json:"quality"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// VideoQuality 视频质量评估
type VideoQuality struct {
	OverallScore     float64 `json:"overall_score"`     // 总体评分 0-1
	ContentQuality   float64 `json:"content_quality"`   // 内容质量
	TechnicalQuality float64 `json:"technical_quality"` // 技术质量
	EngagementRate   float64 `json:"engagement_rate"`   // 参与度
	FreshnessScore   float64 `json:"freshness_score"`   // 新鲜度
}

// UserInteraction 用户交互记录
type UserInteraction struct {
	UserID           int64         `json:"user_id"`
	VideoID          int64         `json:"video_id"`
	InteractionType  BehaviorType  `json:"interaction_type"`
	Duration         time.Duration `json:"duration"`
	CompletionRate   float64       `json:"completion_rate"`
	Timestamp        time.Time     `json:"timestamp"`
	DeviceType       string        `json:"device_type"`
	NetworkCondition string        `json:"network_condition"`
}

// RecommendationCache 推荐缓存
type RecommendationCache struct {
	cache     map[string]*CacheEntry
	ttl       time.Duration
	maxSize   int
	hitCount  int64
	missCount int64
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Videos    []RecommendedVideo `json:"videos"`
	Timestamp time.Time          `json:"timestamp"`
	UserState *UserState         `json:"user_state"`
}

// NewActionModule 创建行动模块
func NewActionModule(config *AgentConfig) *ActionModule {
	return &ActionModule{
		config:              config,
		recommendationCache: NewRecommendationCache(time.Hour, 1000),
		stats: &ActionStats{
			RecommendationsByMode: make(map[RecommendationMode]int64),
		},
	}
}

// SetServices 设置外部服务
func (am *ActionModule) SetServices(videoService VideoService, userService UserService) {
	am.videoService = videoService
	am.userService = userService
}

// NewRecommendationCache 创建推荐缓存
func NewRecommendationCache(ttl time.Duration, maxSize int) *RecommendationCache {
	return &RecommendationCache{
		cache:   make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// ExecuteRecommendation 执行推荐
func (am *ActionModule) ExecuteRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) (*ActionResult, error) {

	startTime := time.Now()
	defer func() {
		am.stats.LastActionTime = time.Now()
		am.stats.AverageResponseTime = time.Since(startTime)
		am.stats.TotalRecommendations++
		am.stats.RecommendationsByMode[decision.RecommendationMode]++
	}()

	hlog.Infof("Executing recommendation for user %d with mode %v",
		userState.UserID, decision.RecommendationMode)

	// 检查缓存
	if cachedResult := am.checkCache(userState, decision); cachedResult != nil {
		am.stats.CacheHitRate = float64(am.recommendationCache.hitCount) /
			float64(am.recommendationCache.hitCount+am.recommendationCache.missCount)
		return cachedResult, nil
	}

	// 根据决策模式执行不同的推荐策略
	var videos []RecommendedVideo
	var err error

	switch decision.RecommendationMode {
	case ModeRegular:
		videos, err = am.executeRegularRecommendation(ctx, userState, decision)
	case ModeHotExplore:
		videos, err = am.executeHotExploreRecommendation(ctx, userState, decision)
	case ModeDeepDive:
		videos, err = am.executeDeepDiveRecommendation(ctx, userState, decision)
	case ModeNewContent:
		videos, err = am.executeNewContentRecommendation(ctx, userState, decision)
	case ModePersonalized:
		videos, err = am.executePersonalizedRecommendation(ctx, userState, decision)
	case ModeDiversified:
		videos, err = am.executeDiversifiedRecommendation(ctx, userState, decision)
	default:
		videos, err = am.executeRegularRecommendation(ctx, userState, decision)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to execute recommendation: %w", err)
	}

	// 应用业务规则过滤
	videos = am.applyBusinessFilters(ctx, userState, videos)

	// 重新排序和评分
	videos = am.reRankVideos(ctx, userState, videos, decision)

	// 限制数量
	maxCount := am.getMaxRecommendationCount(decision)
	if len(videos) > maxCount {
		videos = videos[:maxCount]
	}

	result := &ActionResult{
		RecommendedVideos: videos,
		Mode:              decision.RecommendationMode,
		Parameters:        decision.Parameters,
		Timestamp:         time.Now(),
	}

	// 缓存结果
	am.cacheResult(userState, decision, result)

	hlog.Infof("Recommendation executed successfully: %d videos recommended", len(videos))
	return result, nil
}

// executeRegularRecommendation 执行常规推荐
func (am *ActionModule) executeRegularRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.DefaultRecommendCount
	var allVideos []Video

	// 基于用户兴趣获取视频
	if len(userState.CurrentInterests) > 0 {
		videos, err := am.videoService.GetPersonalizedVideos(
			ctx, userState.UserID, userState.CurrentInterests, count)
		if err == nil {
			allVideos = append(allVideos, videos...)
		}
	}

	// 如果个性化视频不足，补充热门视频
	if len(allVideos) < count {
		trending, err := am.videoService.GetTrendingVideos(ctx, count-len(allVideos))
		if err == nil {
			allVideos = append(allVideos, trending...)
		}
	}

	return am.convertToRecommendedVideos(allVideos, "regular_recommendation"), nil
}

// executeHotExploreRecommendation 执行热点探索推荐
func (am *ActionModule) executeHotExploreRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.HotTopicCount

	// 获取热门视频
	trendingVideos, err := am.videoService.GetTrendingVideos(ctx, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending videos: %w", err)
	}

	return am.convertToRecommendedVideos(trendingVideos, "hot_explore"), nil
}

// executeDeepDiveRecommendation 执行深度挖掘推荐
func (am *ActionModule) executeDeepDiveRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.DeepDiveCount
	var allVideos []Video

	// 基于用户最感兴趣的话题深度挖掘
	if len(userState.CurrentInterests) > 0 {
		mainInterest := userState.CurrentInterests[0] // 假设第一个是主要兴趣

		// 获取相关标签的视频
		videos, err := am.videoService.GetVideosByTags(ctx, []string{mainInterest}, count)
		if err == nil {
			allVideos = append(allVideos, videos...)
		}

		// 获取同类别视频
		categoryVideos, err := am.videoService.GetVideosByCategory(ctx, mainInterest, count/2)
		if err == nil {
			allVideos = append(allVideos, categoryVideos...)
		}
	}

	// 如果基于兴趣没有获取到足够视频，使用用户画像
	if len(allVideos) < count && userState.LongTermProfile != nil {
		for topic, weight := range userState.LongTermProfile.PreferredTopics {
			if weight > 0.7 { // 高权重话题
				videos, err := am.videoService.GetVideosByCategory(ctx, topic, count/3)
				if err == nil {
					allVideos = append(allVideos, videos...)
				}
				if len(allVideos) >= count {
					break
				}
			}
		}
	}

	// 如果还是不够，补充高质量视频
	if len(allVideos) < count {
		trending, err := am.videoService.GetTrendingVideos(ctx, count-len(allVideos))
		if err == nil {
			allVideos = append(allVideos, trending...)
		}
	}

	return am.convertToRecommendedVideos(allVideos, "deep_dive"), nil
}

// executeNewContentRecommendation 执行新内容推荐
func (am *ActionModule) executeNewContentRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.DefaultRecommendCount

	// 这里简化实现，实际应该获取最新发布的视频
	videos, err := am.videoService.GetTrendingVideos(ctx, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get new content: %w", err)
	}

	// 过滤出最新的视频（这里简化处理）
	var newVideos []Video
	cutoffTime := time.Now().Add(-24 * time.Hour) // 24小时内的视频
	for _, video := range videos {
		if video.CreatedAt.After(cutoffTime) {
			newVideos = append(newVideos, video)
		}
	}

	return am.convertToRecommendedVideos(newVideos, "new_content"), nil
}

// executePersonalizedRecommendation 执行个性化推荐
func (am *ActionModule) executePersonalizedRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.DefaultRecommendCount
	var allVideos []Video

	// 基于用户当前兴趣的个性化推荐
	if len(userState.CurrentInterests) > 0 {
		videos, err := am.videoService.GetPersonalizedVideos(
			ctx, userState.UserID, userState.CurrentInterests, count)
		if err == nil {
			allVideos = append(allVideos, videos...)
		}
	}

	// 如果当前兴趣不够，使用用户画像
	if len(allVideos) < count && userState.LongTermProfile != nil {
		for topic, weight := range userState.LongTermProfile.PreferredTopics {
			if weight > 0.5 { // 中等权重以上的话题
				videos, err := am.videoService.GetVideosByCategory(ctx, topic, count/4)
				if err == nil {
					allVideos = append(allVideos, videos...)
				}
				if len(allVideos) >= count {
					break
				}
			}
		}
	}

	// 如果还是不够，补充热门内容
	if len(allVideos) < count {
		trending, err := am.videoService.GetTrendingVideos(ctx, count-len(allVideos))
		if err == nil {
			allVideos = append(allVideos, trending...)
		}
	}

	return am.convertToRecommendedVideos(allVideos, "personalized"), nil
}

// executeDiversifiedRecommendation 执行多样化推荐
func (am *ActionModule) executeDiversifiedRecommendation(
	ctx context.Context,
	userState *UserState,
	decision *DecisionResult,
) ([]RecommendedVideo, error) {

	count := am.config.DefaultRecommendCount
	var allVideos []Video

	// 多样化策略：从不同类别获取视频
	categories := []string{"entertainment", "education", "music", "sports", "technology"}
	videosPerCategory := count / len(categories)

	for _, category := range categories {
		videos, err := am.videoService.GetVideosByCategory(ctx, category, videosPerCategory)
		if err == nil {
			allVideos = append(allVideos, videos...)
		}
	}

	// 随机打乱顺序
	rand.Shuffle(len(allVideos), func(i, j int) {
		allVideos[i], allVideos[j] = allVideos[j], allVideos[i]
	})

	return am.convertToRecommendedVideos(allVideos, "diversified"), nil
}

// convertToRecommendedVideos 转换为推荐视频格式
func (am *ActionModule) convertToRecommendedVideos(videos []Video, reason string) []RecommendedVideo {
	var recommendedVideos []RecommendedVideo

	for _, video := range videos {
		score := am.calculateVideoScore(&video)

		recommendedVideo := RecommendedVideo{
			VideoID:     video.VideoID,
			Title:       video.Title,
			Description: video.Description,
			Category:    video.Category,
			Tags:        video.Tags,
			Score:       score,
			Reason:      reason,
			Metadata: map[string]interface{}{
				"author_id":     video.AuthorID,
				"author_name":   video.AuthorName,
				"view_count":    video.ViewCount,
				"like_count":    video.LikeCount,
				"duration":      video.Duration.Seconds(),
				"quality_score": video.Quality.OverallScore,
			},
		}

		recommendedVideos = append(recommendedVideos, recommendedVideo)
	}

	return recommendedVideos
}

// calculateVideoScore 计算视频评分
func (am *ActionModule) calculateVideoScore(video *Video) float64 {
	// 基础质量分
	qualityScore := video.Quality.OverallScore

	// 参与度分数
	engagementScore := 0.0
	if video.ViewCount > 0 {
		engagementScore = float64(video.LikeCount+video.ShareCount+video.CommentCount) / float64(video.ViewCount)
	}

	// 新鲜度分数
	daysSinceCreated := time.Since(video.CreatedAt).Hours() / 24
	freshnessScore := math.Max(0, 1.0-daysSinceCreated/30.0) // 30天内的视频有新鲜度加分

	// 综合评分
	finalScore := qualityScore*0.4 + engagementScore*0.4 + freshnessScore*0.2

	return math.Min(1.0, finalScore)
}

// applyBusinessFilters 应用业务规则过滤
func (am *ActionModule) applyBusinessFilters(
	ctx context.Context,
	userState *UserState,
	videos []RecommendedVideo,
) []RecommendedVideo {

	var filteredVideos []RecommendedVideo

	for _, video := range videos {
		// 过滤低质量视频
		if video.Score < 0.3 {
			continue
		}

		// 根据用户等级过滤内容
		if am.shouldFilterByUserLevel(userState, &video) {
			continue
		}

		// 过滤重复推荐
		if am.isRecentlyRecommended(userState.UserID, video.VideoID) {
			continue
		}

		filteredVideos = append(filteredVideos, video)
	}

	return filteredVideos
}

// shouldFilterByUserLevel 根据用户等级判断是否需要过滤
func (am *ActionModule) shouldFilterByUserLevel(userState *UserState, video *RecommendedVideo) bool {
	// 这里可以根据用户的消费能力等级过滤内容
	// 简化实现
	return false
}

// isRecentlyRecommended 检查是否最近已推荐过
func (am *ActionModule) isRecentlyRecommended(userID int64, videoID int64) bool {
	// 这里应该检查推荐历史
	// 简化实现
	return false
}

// reRankVideos 重新排序视频
func (am *ActionModule) reRankVideos(
	ctx context.Context,
	userState *UserState,
	videos []RecommendedVideo,
	decision *DecisionResult,
) []RecommendedVideo {

	// 根据用户状态调整排序权重
	for i := range videos {
		adjustedScore := videos[i].Score

		// 根据参与度调整
		switch userState.EngagementLevel {
		case EngagementBored:
			// 无聊用户偏好短视频和娱乐内容
			if videos[i].Category == "entertainment" {
				adjustedScore *= 1.2
			}
		case EngagementImmersed:
			// 沉浸用户偏好深度内容
			if videos[i].Category == "education" {
				adjustedScore *= 1.3
			}
		}

		// 根据探索度调整
		switch userState.ExplorationLevel {
		case ExplorationDiverse:
			// 多样化用户给新类别加分
			if !am.isUserFamiliarCategory(userState, videos[i].Category) {
				adjustedScore *= 1.1
			}
		}

		videos[i].Score = adjustedScore
	}

	// 按评分排序
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Score > videos[j].Score
	})

	return videos
}

// isUserFamiliarCategory 检查用户是否熟悉该类别
func (am *ActionModule) isUserFamiliarCategory(userState *UserState, category string) bool {
	for _, interest := range userState.CurrentInterests {
		if interest == category {
			return true
		}
	}
	return false
}

// getMaxRecommendationCount 获取最大推荐数量
func (am *ActionModule) getMaxRecommendationCount(decision *DecisionResult) int {
	switch decision.RecommendationMode {
	case ModeHotExplore:
		return 15 // 热点探索可以多推荐一些
	case ModeDeepDive:
		return 8 // 深度挖掘推荐较少但质量高
	default:
		return 10
	}
}

// checkCache 检查缓存
func (am *ActionModule) checkCache(userState *UserState, decision *DecisionResult) *ActionResult {
	cacheKey := am.generateCacheKey(userState, decision)

	if entry, exists := am.recommendationCache.cache[cacheKey]; exists {
		if time.Since(entry.Timestamp) < am.recommendationCache.ttl {
			am.recommendationCache.hitCount++
			return &ActionResult{
				RecommendedVideos: entry.Videos,
				Mode:              decision.RecommendationMode,
				Parameters:        decision.Parameters,
				Timestamp:         entry.Timestamp,
			}
		} else {
			// 缓存过期，删除
			delete(am.recommendationCache.cache, cacheKey)
		}
	}

	am.recommendationCache.missCount++
	return nil
}

// cacheResult 缓存结果
func (am *ActionModule) cacheResult(userState *UserState, decision *DecisionResult, result *ActionResult) {
	cacheKey := am.generateCacheKey(userState, decision)

	// 如果缓存已满，删除最旧的条目
	if len(am.recommendationCache.cache) >= am.recommendationCache.maxSize {
		am.evictOldestCacheEntry()
	}

	am.recommendationCache.cache[cacheKey] = &CacheEntry{
		Videos:    result.RecommendedVideos,
		Timestamp: result.Timestamp,
		UserState: userState,
	}
}

// generateCacheKey 生成缓存键
func (am *ActionModule) generateCacheKey(userState *UserState, decision *DecisionResult) string {
	return fmt.Sprintf("user_%d_mode_%d_eng_%d_exp_%d",
		userState.UserID,
		decision.RecommendationMode,
		userState.EngagementLevel,
		userState.ExplorationLevel)
}

// evictOldestCacheEntry 删除最旧的缓存条目
func (am *ActionModule) evictOldestCacheEntry() {
	oldestKey := ""
	oldestTime := time.Now()

	for key, entry := range am.recommendationCache.cache {
		if entry.Timestamp.Before(oldestTime) {
			oldestTime = entry.Timestamp
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(am.recommendationCache.cache, oldestKey)
	}
}

// GetStats 获取统计信息
func (am *ActionModule) GetStats() *ActionStats {
	return am.stats
}

// UpdateConfig 更新配置
func (am *ActionModule) UpdateConfig(config *AgentConfig) error {
	am.config = config
	return nil
}

// ClearCache 清空缓存
func (am *ActionModule) ClearCache() {
	am.recommendationCache.cache = make(map[string]*CacheEntry)
	am.recommendationCache.hitCount = 0
	am.recommendationCache.missCount = 0
}
