package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ========================================
// 核心数据结构
// ========================================

// RecommendationEngine 推荐引擎
type RecommendationEngine struct {
	redis            *redis.Client
	db               *gorm.DB
	recallStrategies []RecallStrategy
	rankingModel     RankingModel
	config           *EngineConfig
	bloomFilter      *BloomFilterManager
	mu               sync.RWMutex
}

// EngineConfig 推荐引擎配置
type EngineConfig struct {
	// 召回配置
	MaxRecallCandidates int     `json:"max_recall_candidates"` // 最大召回候选数
	RecallMultiplier    float64 `json:"recall_multiplier"`     // 召回倍数

	// 排序配置
	RankBatchSize int `json:"rank_batch_size"` // 排序批次大小

	// 去重配置
	ExposureFilterHours int `json:"exposure_filter_hours"` // 曝光过滤时长(小时)
	BloomFilterSize     int `json:"bloom_filter_size"`     // 布隆过滤器大小

	// 多样性配置
	DiversityLambda float64 `json:"diversity_lambda"` // 多样性权重

	// 探索配置
	ExplorationRatio float64 `json:"exploration_ratio"` // 探索比例

	// A/B测试配置
	ABTestEnabled bool `json:"ab_test_enabled"` // 是否启用A/B测试
}

// DefaultEngineConfig 默认配置
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxRecallCandidates: 1000,
		RecallMultiplier:    3.0,
		RankBatchSize:       100,
		ExposureFilterHours: 72,
		BloomFilterSize:     10000,
		DiversityLambda:     0.7,
		ExplorationRatio:    0.1,
		ABTestEnabled:       true,
	}
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
	VideoID      int64              `json:"video_id"`
	Score        float64            `json:"score"`
	Reasons      []string           `json:"reasons"`
	Features     map[string]float64 `json:"features"`
	RecallSource string             `json:"recall_source"`
}

// UserProfile 用户画像
type UserProfile struct {
	UserID           int64              `json:"user_id"`
	InterestTags     map[string]float64 `json:"interest_tags"`
	CategoryPrefer   map[string]float64 `json:"category_prefer"`
	AuthorPrefer     map[int64]float64  `json:"author_prefer"`
	TimePrefer       []int              `json:"time_prefer"`
	AvgWatchDuration float64            `json:"avg_watch_duration"`
	InteractRate     float64            `json:"interact_rate"`
	UserLevel        int                `json:"user_level"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// VideoFeature 视频特征
type VideoFeature struct {
	VideoID         int64     `json:"video_id"`
	Tags            []string  `json:"tags"`
	Category        string    `json:"category"`
	AuthorID        int64     `json:"author_id"`
	Duration        int64     `json:"duration"`
	Quality         float64   `json:"quality"`
	Freshness       float64   `json:"freshness"`
	PopularityScore float64   `json:"popularity_score"`
	CTR             float64   `json:"ctr"`
	FinishRate      float64   `json:"finish_rate"`
	InteractScore   float64   `json:"interact_score"`
	AuthorScore     float64   `json:"author_score"`
	CreatedAt       time.Time `json:"created_at"`
}

// RecommendRequest 推荐请求
type RecommendRequest struct {
	UserID       int64             `json:"user_id"`
	Limit        int               `json:"limit"`
	RequestID    string            `json:"request_id"`
	RequestType  string            `json:"request_type"` // feed/related/search
	Context      map[string]string `json:"context"`      // 上下文信息
	ExcludeIDs   []int64           `json:"exclude_ids"`  // 排除的视频ID
	RefVideoID   int64             `json:"ref_video_id"` // 相关推荐的参考视频
}

// RecommendResponse 推荐响应
type RecommendResponse struct {
	Videos        []ScoredVideo     `json:"videos"`
	RequestID     string            `json:"request_id"`
	RecallStats   map[string]int    `json:"recall_stats"`
	CandidateSize int               `json:"candidate_size"`
	LatencyMs     int64             `json:"latency_ms"`
	ExperimentID  int64             `json:"experiment_id,omitempty"`
	GroupID       int64             `json:"group_id,omitempty"`
}

// ========================================
// 推荐引擎核心实现
// ========================================

// NewRecommendationEngine 创建推荐引擎
func NewRecommendationEngine(redisClient *redis.Client, db *gorm.DB) *RecommendationEngine {
	engine := &RecommendationEngine{
		redis:            redisClient,
		db:               db,
		recallStrategies: make([]RecallStrategy, 0),
		config:           DefaultEngineConfig(),
		bloomFilter:      NewBloomFilterManager(redisClient),
	}

	// 注册召回策略
	engine.RegisterStrategy(NewCollaborativeFilteringRecall(redisClient, db))
	engine.RegisterStrategy(NewHotVideoRecall(redisClient, db))
	engine.RegisterStrategy(NewContentBasedRecall(redisClient, db))
	engine.RegisterStrategy(NewSocialRecall(redisClient, db))
	engine.RegisterStrategy(NewNewVideoRecall(redisClient, db))
	engine.RegisterStrategy(NewSimilarVideoRecall(redisClient, db))

	// 设置排序模型
	engine.rankingModel = NewEnhancedRankingModel(redisClient, db)

	return engine
}

// RegisterStrategy 注册召回策略
func (re *RecommendationEngine) RegisterStrategy(strategy RecallStrategy) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.recallStrategies = append(re.recallStrategies, strategy)
}

// SetConfig 设置配置
func (re *RecommendationEngine) SetConfig(config *EngineConfig) {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.config = config
}

// Recommend 生成推荐列表（主入口）
func (re *RecommendationEngine) Recommend(ctx context.Context, req *RecommendRequest) (*RecommendResponse, error) {
	startTime := time.Now()

	response := &RecommendResponse{
		RequestID:   req.RequestID,
		RecallStats: make(map[string]int),
	}

	// 1. 多路召回
	candidates, recallStats := re.multiChannelRecall(ctx, req.UserID, req.Limit)
	response.RecallStats = recallStats
	response.CandidateSize = len(candidates)

	// 2. 过滤已曝光/排除的视频
	candidates = re.filterVideos(ctx, req.UserID, candidates, req.ExcludeIDs)

	// 3. 精排
	rankedVideos, err := re.rankingModel.Rank(ctx, req.UserID, candidates)
	if err != nil {
		return nil, err
	}

	// 4. 重排序 (多样性、新鲜度)
	rankedVideos = re.Rerank(rankedVideos, req.Limit)

	// 5. 探索性注入 (E&E)
	rankedVideos = re.injectExploration(ctx, req.UserID, rankedVideos, req.Limit)

	// 6. 截取最终结果
	if len(rankedVideos) > req.Limit {
		rankedVideos = rankedVideos[:req.Limit]
	}

	response.Videos = rankedVideos
	response.LatencyMs = time.Since(startTime).Milliseconds()

	// 7. 异步记录曝光
	go re.recordExposures(context.Background(), req.UserID, rankedVideos, req.RequestID)

	return response, nil
}

// RecommendSimple 简化版推荐（兼容旧接口）
func (re *RecommendationEngine) RecommendSimple(ctx context.Context, userID int64, limit int) ([]ScoredVideo, error) {
	req := &RecommendRequest{
		UserID:      userID,
		Limit:       limit,
		RequestID:   fmt.Sprintf("%d_%d", userID, time.Now().UnixNano()),
		RequestType: "feed",
	}
	resp, err := re.Recommend(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Videos, nil
}

// multiChannelRecall 多路召回
func (re *RecommendationEngine) multiChannelRecall(ctx context.Context, userID int64, targetLimit int) ([]int64, map[string]int) {
	candidateSet := make(map[int64]string) // video_id -> recall_source
	recallStats := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, strategy := range re.recallStrategies {
		wg.Add(1)
		go func(s RecallStrategy) {
			defer wg.Done()

			// 每路召回数量 = 目标数量 * 倍数 * 权重
			recallLimit := int(float64(targetLimit) * re.config.RecallMultiplier * s.Weight())
			if recallLimit < 10 {
				recallLimit = 10
			}

			videos, err := s.Recall(ctx, userID, recallLimit)
			if err != nil {
				return
			}

			mu.Lock()
			defer mu.Unlock()

			recallStats[s.Name()] = len(videos)
			for _, vid := range videos {
				if _, exists := candidateSet[vid]; !exists {
					candidateSet[vid] = s.Name()
				}
			}
		}(strategy)
	}

	wg.Wait()

	// 转为切片
	candidates := make([]int64, 0, len(candidateSet))
	for vid := range candidateSet {
		candidates = append(candidates, vid)
	}

	return candidates, recallStats
}

// filterVideos 过滤视频
func (re *RecommendationEngine) filterVideos(ctx context.Context, userID int64, candidates []int64, excludeIDs []int64) []int64 {
	// 构建排除集合
	excludeSet := make(map[int64]bool)
	for _, id := range excludeIDs {
		excludeSet[id] = true
	}

	// 获取用户最近曝光的视频
	exposedVideos := re.getRecentExposures(ctx, userID)
	for _, id := range exposedVideos {
		excludeSet[id] = true
	}

	// 获取用户负反馈的视频
	blockedVideos := re.getBlockedVideos(ctx, userID)
	for _, id := range blockedVideos {
		excludeSet[id] = true
	}

	// 过滤
	filtered := make([]int64, 0, len(candidates))
	for _, vid := range candidates {
		if !excludeSet[vid] {
			filtered = append(filtered, vid)
		}
	}

	return filtered
}

// Rerank 重排序,保证多样性
func (re *RecommendationEngine) Rerank(videos []ScoredVideo, limit int) []ScoredVideo {
	if len(videos) <= limit {
		return videos
	}

	// 使用 MMR (Maximal Marginal Relevance) 算法
	selected := make([]ScoredVideo, 0, limit)
	remaining := make([]ScoredVideo, len(videos))
	copy(remaining, videos)

	lambda := re.config.DiversityLambda

	// 选择分数最高的作为第一个
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].Score > remaining[j].Score
	})
	selected = append(selected, remaining[0])
	remaining = remaining[1:]

	for len(selected) < limit && len(remaining) > 0 {
		maxMMRScore := -math.MaxFloat64
		maxIdx := -1

		for i, video := range remaining {
			// 计算与已选视频的最大相似度
			maxSim := 0.0
			for _, s := range selected {
				sim := re.calculateSimilarity(video, s)
				if sim > maxSim {
					maxSim = sim
				}
			}

			// MMR 分数 = λ * 相关性 - (1-λ) * 最大相似度
			mmrScore := lambda*video.Score - (1-lambda)*maxSim

			if mmrScore > maxMMRScore {
				maxMMRScore = mmrScore
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

// injectExploration 注入探索性内容
func (re *RecommendationEngine) injectExploration(ctx context.Context, userID int64, videos []ScoredVideo, limit int) []ScoredVideo {
	explorationCount := int(float64(limit) * re.config.ExplorationRatio)
	if explorationCount == 0 {
		return videos
	}

	// 获取探索性视频（新视频、冷启动内容）
	explorationVideos := re.getExplorationVideos(ctx, userID, explorationCount)
	if len(explorationVideos) == 0 {
		return videos
	}

	// 在结果中均匀插入探索性内容
	result := make([]ScoredVideo, 0, len(videos)+len(explorationVideos))
	explorationIdx := 0
	interval := len(videos) / (len(explorationVideos) + 1)

	for i, video := range videos {
		result = append(result, video)
		// 每隔一定间隔插入探索性内容
		if interval > 0 && (i+1)%interval == 0 && explorationIdx < len(explorationVideos) {
			result = append(result, explorationVideos[explorationIdx])
			explorationIdx++
		}
	}

	// 添加剩余的探索性内容
	for ; explorationIdx < len(explorationVideos); explorationIdx++ {
		result = append(result, explorationVideos[explorationIdx])
	}

	return result
}

// getExplorationVideos 获取探索性视频
func (re *RecommendationEngine) getExplorationVideos(ctx context.Context, userID int64, limit int) []ScoredVideo {
	// 从新视频池中随机选取
	key := "exploration:video:pool"
	members, err := re.redis.SRandMemberN(ctx, key, int64(limit)).Result()
	if err != nil || len(members) == 0 {
		return nil
	}

	videos := make([]ScoredVideo, 0, len(members))
	for _, m := range members {
		var vid int64
		fmt.Sscanf(m, "%d", &vid)
		videos = append(videos, ScoredVideo{
			VideoID:      vid,
			Score:        0.5, // 探索性内容给一个中等分数
			Reasons:      []string{"发现更多精彩"},
			RecallSource: "exploration",
		})
	}

	return videos
}

// getRecentExposures 获取用户最近曝光的视频
func (re *RecommendationEngine) getRecentExposures(ctx context.Context, userID int64) []int64 {
	key := fmt.Sprintf("user:exposures:%d", userID)
	members, err := re.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil
	}

	videoIds := make([]int64, 0, len(members))
	for _, m := range members {
		var vid int64
		fmt.Sscanf(m, "%d", &vid)
		videoIds = append(videoIds, vid)
	}

	return videoIds
}

// getBlockedVideos 获取用户屏蔽的视频
func (re *RecommendationEngine) getBlockedVideos(ctx context.Context, userID int64) []int64 {
	key := fmt.Sprintf("user:blocked:videos:%d", userID)
	members, err := re.redis.SMembers(ctx, key).Result()
	if err != nil {
		return nil
	}

	videoIds := make([]int64, 0, len(members))
	for _, m := range members {
		var vid int64
		fmt.Sscanf(m, "%d", &vid)
		videoIds = append(videoIds, vid)
	}

	return videoIds
}

// recordExposures 记录曝光
func (re *RecommendationEngine) recordExposures(ctx context.Context, userID int64, videos []ScoredVideo, requestID string) {
	if len(videos) == 0 {
		return
	}

	// 1. 更新Redis曝光集合（用于快速去重）
	key := fmt.Sprintf("user:exposures:%d", userID)
	pipe := re.redis.Pipeline()

	members := make([]interface{}, len(videos))
	for i, v := range videos {
		members[i] = fmt.Sprintf("%d", v.VideoID)
	}
	pipe.SAdd(ctx, key, members...)
	pipe.Expire(ctx, key, time.Duration(re.config.ExposureFilterHours)*time.Hour)
	pipe.Exec(ctx)

	// 2. 异步写入数据库曝光记录表（用于效果分析）
	// 这里通过消息队列或直接批量插入
}

// FilterWatched 过滤已观看视频（兼容旧接口）
func (re *RecommendationEngine) FilterWatched(ctx context.Context, userID int64, videos []ScoredVideo) []ScoredVideo {
	watchedKey := fmt.Sprintf("user:watch_history:%d", userID)

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
	// 根据用户行为实时更新画像
	// action: view, like, comment, share, finish, dislike

	// 1. 更新用户兴趣标签
	if err := re.updateUserInterests(ctx, userID, videoID, action); err != nil {
		return err
	}

	// 2. 更新用户分类偏好
	if err := re.updateUserCategoryPrefer(ctx, userID, videoID, action); err != nil {
		return err
	}

	// 3. 更新用户作者偏好
	if err := re.updateUserAuthorPrefer(ctx, userID, videoID, action); err != nil {
		return err
	}

	return nil
}

// updateUserInterests 更新用户兴趣标签
func (re *RecommendationEngine) updateUserInterests(ctx context.Context, userID, videoID int64, action string) error {
	// 获取视频标签
	videoTagsKey := fmt.Sprintf("video:tags:%d", videoID)
	tags, err := re.redis.SMembers(ctx, videoTagsKey).Result()
	if err != nil || len(tags) == 0 {
		return nil
	}

	// 根据行为类型确定权重
	weight := getActionWeight(action)

	// 更新用户兴趣标签
	userInterestsKey := fmt.Sprintf("user:interests:%d", userID)
	pipe := re.redis.Pipeline()
	for _, tag := range tags {
		pipe.ZIncrBy(ctx, userInterestsKey, weight, tag)
	}
	pipe.Expire(ctx, userInterestsKey, 30*24*time.Hour) // 30天过期
	_, err = pipe.Exec(ctx)

	return err
}

// updateUserCategoryPrefer 更新用户分类偏好
func (re *RecommendationEngine) updateUserCategoryPrefer(ctx context.Context, userID, videoID int64, action string) error {
	// 获取视频分类
	videoCategoryKey := fmt.Sprintf("video:category:%d", videoID)
	category, err := re.redis.Get(ctx, videoCategoryKey).Result()
	if err != nil || category == "" {
		return nil
	}

	weight := getActionWeight(action)

	// 更新用户分类偏好
	userCategoryKey := fmt.Sprintf("user:category_prefer:%d", userID)
	return re.redis.ZIncrBy(ctx, userCategoryKey, weight, category).Err()
}

// updateUserAuthorPrefer 更新用户作者偏好
func (re *RecommendationEngine) updateUserAuthorPrefer(ctx context.Context, userID, videoID int64, action string) error {
	// 获取视频作者
	videoAuthorKey := fmt.Sprintf("video:author:%d", videoID)
	authorIDStr, err := re.redis.Get(ctx, videoAuthorKey).Result()
	if err != nil || authorIDStr == "" {
		return nil
	}

	weight := getActionWeight(action)

	// 更新用户作者偏好
	userAuthorKey := fmt.Sprintf("user:author_prefer:%d", userID)
	return re.redis.ZIncrBy(ctx, userAuthorKey, weight, authorIDStr).Err()
}

// getActionWeight 获取行为权重
func getActionWeight(action string) float64 {
	weights := map[string]float64{
		"view":    0.1,
		"finish":  0.3,
		"like":    0.5,
		"comment": 0.6,
		"share":   0.8,
		"favorite": 0.7,
		"dislike": -0.5,
	}

	if w, ok := weights[action]; ok {
		return w
	}
	return 0.1
}

// ========================================
// 布隆过滤器管理
// ========================================

// BloomFilterManager 布隆过滤器管理器
type BloomFilterManager struct {
	redis *redis.Client
}

// NewBloomFilterManager 创建布隆过滤器管理器
func NewBloomFilterManager(redisClient *redis.Client) *BloomFilterManager {
	return &BloomFilterManager{redis: redisClient}
}

// Add 添加元素
func (bf *BloomFilterManager) Add(ctx context.Context, userID, videoID int64) error {
	key := fmt.Sprintf("bf:user:exposed:%d", userID)
	// 使用 Redis 的位图操作模拟布隆过滤器
	hashes := bf.getHashes(videoID)
	pipe := bf.redis.Pipeline()
	for _, h := range hashes {
		pipe.SetBit(ctx, key, int64(h%100000), 1)
	}
	pipe.Expire(ctx, key, 7*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

// Contains 检查元素是否存在
func (bf *BloomFilterManager) Contains(ctx context.Context, userID, videoID int64) (bool, error) {
	key := fmt.Sprintf("bf:user:exposed:%d", userID)
	hashes := bf.getHashes(videoID)

	for _, h := range hashes {
		bit, err := bf.redis.GetBit(ctx, key, int64(h%100000)).Result()
		if err != nil || bit == 0 {
			return false, err
		}
	}
	return true, nil
}

// getHashes 计算多个哈希值
func (bf *BloomFilterManager) getHashes(videoID int64) []uint32 {
	// 简化版哈希函数，实际应使用 MurmurHash 等
	h1 := uint32(videoID % 65537)
	h2 := uint32((videoID * 31) % 65537)
	h3 := uint32((videoID * 37) % 65537)
	return []uint32{h1, h2, h3}
}

// ========================================
// 辅助函数
// ========================================

func formatWatchHistoryKey(userID int64) string {
	return fmt.Sprintf("user:watch_history:%d", userID)
}

// GetUserProfile 获取用户画像
func (re *RecommendationEngine) GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error) {
	profile := &UserProfile{
		UserID:         userID,
		InterestTags:   make(map[string]float64),
		CategoryPrefer: make(map[string]float64),
		AuthorPrefer:   make(map[int64]float64),
	}

	// 获取兴趣标签
	interestsKey := fmt.Sprintf("user:interests:%d", userID)
	interests, err := re.redis.ZRevRangeWithScores(ctx, interestsKey, 0, 19).Result()
	if err == nil {
		for _, z := range interests {
			profile.InterestTags[z.Member.(string)] = z.Score
		}
	}

	// 获取分类偏好
	categoryKey := fmt.Sprintf("user:category_prefer:%d", userID)
	categories, err := re.redis.ZRevRangeWithScores(ctx, categoryKey, 0, 9).Result()
	if err == nil {
		for _, z := range categories {
			profile.CategoryPrefer[z.Member.(string)] = z.Score
		}
	}

	// 获取作者偏好
	authorKey := fmt.Sprintf("user:author_prefer:%d", userID)
	authors, err := re.redis.ZRevRangeWithScores(ctx, authorKey, 0, 49).Result()
	if err == nil {
		for _, z := range authors {
			var authorID int64
			fmt.Sscanf(z.Member.(string), "%d", &authorID)
			profile.AuthorPrefer[authorID] = z.Score
		}
	}

	return profile, nil
}

// GetVideoFeature 获取视频特征
func (re *RecommendationEngine) GetVideoFeature(ctx context.Context, videoID int64) (*VideoFeature, error) {
	key := fmt.Sprintf("video:feature:%d", videoID)
	data, err := re.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var feature VideoFeature
	if err := json.Unmarshal([]byte(data), &feature); err != nil {
		return nil, err
	}

	return &feature, nil
}

// CacheVideoFeature 缓存视频特征
func (re *RecommendationEngine) CacheVideoFeature(ctx context.Context, feature *VideoFeature) error {
	key := fmt.Sprintf("video:feature:%d", feature.VideoID)
	data, err := json.Marshal(feature)
	if err != nil {
		return err
	}
	return re.redis.Set(ctx, key, data, 24*time.Hour).Err()
}
