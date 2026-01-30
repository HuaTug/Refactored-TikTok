package recommendation

import (
	"context"
	"math"
	"sync"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/cmd/video/dal/db"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/gorm"
)

// =====================================================
// 视频热度计算服务
// 定时计算视频热度分数，支持多时间窗口
// =====================================================

// TimeWindow 时间窗口定义
type TimeWindow struct {
	Name     string        // 窗口名称: 1h/6h/24h/7d
	Duration time.Duration // 时间跨度
	Weight   float64       // 窗口权重（用于综合热度计算）
}

// HotScoreConfig 热度计算配置
type HotScoreConfig struct {
	// 时间窗口配置
	TimeWindows []TimeWindow

	// 各指标权重
	ViewWeight     float64 // 播放量权重
	LikeWeight     float64 // 点赞权重
	CommentWeight  float64 // 评论权重
	ShareWeight    float64 // 分享权重
	FavoriteWeight float64 // 收藏权重

	// 时间衰减参数
	DecayHalfLife time.Duration // 半衰期

	// 质量因子权重
	QualityBonus float64 // 优质内容加成

	// 计算参数
	BatchSize       int           // 批量处理大小
	CalculateWorker int           // 计算并发数
	UpdateInterval  time.Duration // 更新间隔
}

// DefaultHotScoreConfig 默认热度配置
func DefaultHotScoreConfig() *HotScoreConfig {
	return &HotScoreConfig{
		TimeWindows: []TimeWindow{
			{Name: "1h", Duration: time.Hour, Weight: 0.4},
			{Name: "6h", Duration: 6 * time.Hour, Weight: 0.3},
			{Name: "24h", Duration: 24 * time.Hour, Weight: 0.2},
			{Name: "7d", Duration: 7 * 24 * time.Hour, Weight: 0.1},
		},
		ViewWeight:      1.0,
		LikeWeight:      3.0,
		CommentWeight:   5.0,
		ShareWeight:     8.0,
		FavoriteWeight:  4.0,
		DecayHalfLife:   6 * time.Hour,
		QualityBonus:    1.5,
		BatchSize:       500,
		CalculateWorker: 4,
		UpdateInterval:  5 * time.Minute,
	}
}

// VideoHotScoreService 视频热度计算服务
type VideoHotScoreService struct {
	config    *HotScoreConfig
	db        *gorm.DB
	stopCh    chan struct{}
	isRunning bool
	mutex     sync.RWMutex
}

// NewVideoHotScoreService 创建热度计算服务
func NewVideoHotScoreService(config *HotScoreConfig, database *gorm.DB) *VideoHotScoreService {
	if config == nil {
		config = DefaultHotScoreConfig()
	}
	return &VideoHotScoreService{
		config: config,
		db:     database,
		stopCh: make(chan struct{}),
	}
}

// Start 启动热度计算定时任务
func (s *VideoHotScoreService) Start() {
	s.mutex.Lock()
	if s.isRunning {
		s.mutex.Unlock()
		return
	}
	s.isRunning = true
	s.mutex.Unlock()

	hlog.Info("[HotScoreService] Starting video hot score calculation service...")

	// 启动时先执行一次完整计算
	go func() {
		s.CalculateAllHotScores(context.Background())
	}()

	// 定时执行
	ticker := time.NewTicker(s.config.UpdateInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.CalculateAllHotScores(context.Background())
			case <-s.stopCh:
				ticker.Stop()
				hlog.Info("[HotScoreService] Hot score calculation service stopped")
				return
			}
		}
	}()
}

// Stop 停止服务
func (s *VideoHotScoreService) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isRunning {
		return
	}
	close(s.stopCh)
	s.isRunning = false
}

// CalculateAllHotScores 计算所有视频的热度分数
func (s *VideoHotScoreService) CalculateAllHotScores(ctx context.Context) {
	startTime := time.Now()
	hlog.Info("[HotScoreService] Starting hot score calculation...")

	// 获取所有公开且审核通过的视频
	videoIds, err := s.getActiveVideoIds(ctx)
	if err != nil {
		hlog.Errorf("[HotScoreService] Failed to get active video ids: %v", err)
		return
	}

	if len(videoIds) == 0 {
		hlog.Info("[HotScoreService] No active videos to calculate")
		return
	}

	hlog.Infof("[HotScoreService] Found %d active videos to calculate", len(videoIds))

	// 分批处理
	totalProcessed := 0
	for i := 0; i < len(videoIds); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(videoIds) {
			end = len(videoIds)
		}
		batch := videoIds[i:end]

		processed, err := s.calculateBatchHotScores(ctx, batch)
		if err != nil {
			hlog.Errorf("[HotScoreService] Error calculating batch %d-%d: %v", i, end, err)
			continue
		}
		totalProcessed += processed
	}

	// 更新排名
	for _, tw := range s.config.TimeWindows {
		if err := db.UpdateHotScoreRanks(ctx, tw.Name); err != nil {
			hlog.Errorf("[HotScoreService] Failed to update ranks for %s: %v", tw.Name, err)
		}
	}

	elapsed := time.Since(startTime)
	hlog.Infof("[HotScoreService] Hot score calculation completed. Processed: %d videos, Time: %v", totalProcessed, elapsed)
}

// getActiveVideoIds 获取所有活跃视频ID
func (s *VideoHotScoreService) getActiveVideoIds(ctx context.Context) ([]int64, error) {
	var videoIds []int64
	err := s.db.WithContext(ctx).Model(&model.Video{}).
		Where("deleted_at IS NULL AND audit_status = 1 AND open = 1").
		Pluck("video_id", &videoIds).Error
	return videoIds, err
}

// calculateBatchHotScores 批量计算热度分数
func (s *VideoHotScoreService) calculateBatchHotScores(ctx context.Context, videoIds []int64) (int, error) {
	// 获取视频详情
	videos, err := s.getVideoDetails(ctx, videoIds)
	if err != nil {
		return 0, err
	}

	// 获取视频特征（如果有）
	featureMap := s.getVideoFeatureMap(ctx, videoIds)

	// 获取时间窗口内的互动增量数据
	interactionDeltas := s.getInteractionDeltas(ctx, videoIds)

	// 计算每个视频在每个时间窗口的热度
	var hotScores []*model.VideoHotScore
	now := time.Now()

	for _, video := range videos {
		for _, tw := range s.config.TimeWindows {
			hotScore := s.calculateSingleVideoHotScore(video, tw, now, featureMap, interactionDeltas)
			hotScores = append(hotScores, hotScore)
		}
	}

	// 批量保存
	if len(hotScores) > 0 {
		if err := db.BatchUpdateVideoHotScores(ctx, hotScores); err != nil {
			return 0, err
		}
	}

	return len(videos), nil
}

// VideoDetail 视频详情结构
type VideoDetail struct {
	VideoId        int64
	UserId         int64
	VisitCount     uint64
	LikesCount     uint64
	CommentCount   uint64
	ShareCount     uint64
	FavoritesCount uint64
	CreatedAt      time.Time
	Duration       uint
}

// getVideoDetails 获取视频详情
func (s *VideoHotScoreService) getVideoDetails(ctx context.Context, videoIds []int64) ([]*VideoDetail, error) {
	var videos []*VideoDetail
	err := s.db.WithContext(ctx).Model(&model.Video{}).
		Select("video_id, user_id, visit_count, likes_count, comment_count, share_count, favorites_count, created_at, duration").
		Where("video_id IN ?", videoIds).
		Find(&videos).Error
	return videos, err
}

// getVideoFeatureMap 获取视频特征映射
func (s *VideoHotScoreService) getVideoFeatureMap(ctx context.Context, videoIds []int64) map[int64]*model.VideoFeature {
	featureMap := make(map[int64]*model.VideoFeature)
	features, err := db.BatchGetVideoFeatures(ctx, videoIds)
	if err != nil {
		hlog.Warnf("[HotScoreService] Failed to get video features: %v", err)
		return featureMap
	}
	for _, f := range features {
		featureMap[f.VideoID] = f
	}
	return featureMap
}

// InteractionDelta 互动增量数据
type InteractionDelta struct {
	VideoID      int64
	TimeWindow   string
	ViewDelta    int64
	LikeDelta    int64
	CommentDelta int64
	ShareDelta   int64
}

// getInteractionDeltas 获取时间窗口内的互动增量
// 这里通过 recommendation_exposures 表来统计
func (s *VideoHotScoreService) getInteractionDeltas(ctx context.Context, videoIds []int64) map[string]map[int64]*InteractionDelta {
	deltas := make(map[string]map[int64]*InteractionDelta)
	now := time.Now()

	for _, tw := range s.config.TimeWindows {
		windowStart := now.Add(-tw.Duration)
		deltas[tw.Name] = make(map[int64]*InteractionDelta)

		// 从曝光记录统计互动增量
		type ExposureStat struct {
			VideoID      int64 `gorm:"column:video_id"`
			ClickCount   int64 `gorm:"column:click_count"`
			LikeCount    int64 `gorm:"column:like_count"`
			CommentCount int64 `gorm:"column:comment_count"`
			ShareCount   int64 `gorm:"column:share_count"`
		}

		var stats []ExposureStat
		s.db.WithContext(ctx).Model(&model.RecommendationExposure{}).
			Select(`
				video_id,
				COUNT(*) as click_count,
				SUM(is_liked) as like_count,
				SUM(is_commented) as comment_count,
				SUM(is_shared) as share_count
			`).
			Where("video_id IN ? AND exposure_time >= ? AND is_clicked = 1", videoIds, windowStart).
			Group("video_id").
			Find(&stats)

		for _, stat := range stats {
			deltas[tw.Name][stat.VideoID] = &InteractionDelta{
				VideoID:      stat.VideoID,
				TimeWindow:   tw.Name,
				ViewDelta:    stat.ClickCount,
				LikeDelta:    stat.LikeCount,
				CommentDelta: stat.CommentCount,
				ShareDelta:   stat.ShareCount,
			}
		}
	}

	return deltas
}

// calculateSingleVideoHotScore 计算单个视频的热度分数
func (s *VideoHotScoreService) calculateSingleVideoHotScore(
	video *VideoDetail,
	tw TimeWindow,
	now time.Time,
	featureMap map[int64]*model.VideoFeature,
	interactionDeltas map[string]map[int64]*InteractionDelta,
) *model.VideoHotScore {
	windowStart := now.Add(-tw.Duration)

	// 基础热度计算（使用总量数据作为基础）
	baseScore := s.calculateBaseScore(video)

	// 时间窗口内的增量热度
	deltaScore := 0.0
	if delta, ok := interactionDeltas[tw.Name][video.VideoId]; ok {
		deltaScore = float64(delta.ViewDelta)*s.config.ViewWeight +
			float64(delta.LikeDelta)*s.config.LikeWeight +
			float64(delta.CommentDelta)*s.config.CommentWeight +
			float64(delta.ShareDelta)*s.config.ShareWeight
	}

	// 时间衰减因子
	videoAge := now.Sub(video.CreatedAt)
	decayFactor := s.calculateDecayFactor(videoAge)

	// 质量加成
	qualityBonus := 1.0
	if feature, ok := featureMap[video.VideoId]; ok {
		if feature.IsHighQuality == 1 {
			qualityBonus = s.config.QualityBonus
		}
		// 根据完播率额外加成
		if feature.FinishRate > 0.5 {
			qualityBonus *= (1 + feature.FinishRate*0.5)
		}
	}

	// 综合热度分数
	// 基础分 * 衰减因子 + 增量分 * 质量加成
	hotScore := (baseScore*0.3*decayFactor + deltaScore*0.7) * qualityBonus

	// 使用对数压缩避免数值过大
	if hotScore > 0 {
		hotScore = math.Log10(hotScore+1) * 100
	}

	return &model.VideoHotScore{
		VideoID:      video.VideoId,
		TimeWindow:   tw.Name,
		ViewCount:    int64(video.VisitCount),
		LikeCount:    int64(video.LikesCount),
		CommentCount: int64(video.CommentCount),
		ShareCount:   int64(video.ShareCount),
		HotScore:     hotScore,
		WindowStart:  windowStart,
		WindowEnd:    now,
		UpdatedAt:    now,
	}
}

// calculateBaseScore 计算基础热度分
func (s *VideoHotScoreService) calculateBaseScore(video *VideoDetail) float64 {
	return float64(video.VisitCount)*s.config.ViewWeight +
		float64(video.LikesCount)*s.config.LikeWeight +
		float64(video.CommentCount)*s.config.CommentWeight +
		float64(video.ShareCount)*s.config.ShareWeight +
		float64(video.FavoritesCount)*s.config.FavoriteWeight
}

// calculateDecayFactor 计算时间衰减因子
// 使用指数衰减模型: f(t) = e^(-λt), 其中 λ = ln(2) / 半衰期
func (s *VideoHotScoreService) calculateDecayFactor(age time.Duration) float64 {
	if age <= 0 {
		return 1.0
	}
	lambda := math.Log(2) / float64(s.config.DecayHalfLife)
	factor := math.Exp(-lambda * float64(age))
	// 最低保留10%的权重
	if factor < 0.1 {
		factor = 0.1
	}
	return factor
}

// CalculateVideoHotScore 手动计算单个视频热度（实时接口）
func (s *VideoHotScoreService) CalculateVideoHotScore(ctx context.Context, videoId int64) error {
	videos, err := s.getVideoDetails(ctx, []int64{videoId})
	if err != nil || len(videos) == 0 {
		return err
	}

	featureMap := s.getVideoFeatureMap(ctx, []int64{videoId})
	interactionDeltas := s.getInteractionDeltas(ctx, []int64{videoId})

	now := time.Now()
	var hotScores []*model.VideoHotScore

	for _, tw := range s.config.TimeWindows {
		hotScore := s.calculateSingleVideoHotScore(videos[0], tw, now, featureMap, interactionDeltas)
		hotScores = append(hotScores, hotScore)
	}

	return db.BatchUpdateVideoHotScores(ctx, hotScores)
}

// GetTopHotVideos 获取热门视频排行榜
func (s *VideoHotScoreService) GetTopHotVideos(ctx context.Context, timeWindow string, limit int) ([]int64, error) {
	return db.GetHotVideoIds(ctx, timeWindow, limit)
}

// GetVideoHotRank 获取视频热度排名
func (s *VideoHotScoreService) GetVideoHotRank(ctx context.Context, videoId int64, timeWindow string) (int, float64, error) {
	hotScore, err := db.GetVideoHotScore(ctx, videoId, timeWindow)
	if err != nil {
		return 0, 0, err
	}
	if hotScore == nil {
		return 0, 0, nil
	}
	return hotScore.Rank, hotScore.HotScore, nil
}

// =====================================================
// 热度趋势分析
// =====================================================

// HotTrend 热度趋势
type HotTrend struct {
	VideoID     int64   `json:"video_id"`
	CurrentRank int     `json:"current_rank"`
	TrendScore  float64 `json:"trend_score"`  // 趋势分数 (正数上升，负数下降)
	TrendType   string  `json:"trend_type"`   // rising/stable/falling/new
}

// GetTrendingVideos 获取趋势视频（上升最快的视频）
func (s *VideoHotScoreService) GetTrendingVideos(ctx context.Context, limit int) ([]*HotTrend, error) {
	// 比较 1h 和 24h 的热度变化
	hotScores1h, err := db.GetHotVideosByWindow(ctx, "1h", 200)
	if err != nil {
		return nil, err
	}

	hotScores24h, err := db.GetHotVideosByWindow(ctx, "24h", 200)
	if err != nil {
		return nil, err
	}

	// 构建24h热度映射
	score24hMap := make(map[int64]float64)
	for _, hs := range hotScores24h {
		score24hMap[hs.VideoID] = hs.HotScore
	}

	// 计算趋势分数
	var trends []*HotTrend
	for i, hs := range hotScores1h {
		trend := &HotTrend{
			VideoID:     hs.VideoID,
			CurrentRank: i + 1,
		}

		score24h, exists := score24hMap[hs.VideoID]
		if !exists || score24h == 0 {
			// 新晋热门
			trend.TrendType = "new"
			trend.TrendScore = hs.HotScore
		} else {
			// 计算增长率
			growthRate := (hs.HotScore - score24h) / score24h * 100
			trend.TrendScore = growthRate

			if growthRate > 20 {
				trend.TrendType = "rising"
			} else if growthRate < -20 {
				trend.TrendType = "falling"
			} else {
				trend.TrendType = "stable"
			}
		}

		trends = append(trends, trend)
	}

	// 按趋势分数排序
	sortTrendsByScore(trends)

	// 只返回上升的视频
	var risingTrends []*HotTrend
	for _, t := range trends {
		if t.TrendType == "rising" || t.TrendType == "new" {
			risingTrends = append(risingTrends, t)
			if len(risingTrends) >= limit {
				break
			}
		}
	}

	return risingTrends, nil
}

// sortTrendsByScore 按趋势分数排序
func sortTrendsByScore(trends []*HotTrend) {
	for i := 0; i < len(trends)-1; i++ {
		for j := i + 1; j < len(trends); j++ {
			if trends[j].TrendScore > trends[i].TrendScore {
				trends[i], trends[j] = trends[j], trends[i]
			}
		}
	}
}

// =====================================================
// 分类热度统计
// =====================================================

// CategoryHotStats 分类热度统计
type CategoryHotStats struct {
	Category   string  `json:"category"`
	HotScore   float64 `json:"hot_score"`
	VideoCount int     `json:"video_count"`
	TrendScore float64 `json:"trend_score"`
}

// GetCategoryHotStats 获取分类热度统计
func (s *VideoHotScoreService) GetCategoryHotStats(ctx context.Context) ([]*CategoryHotStats, error) {
	type CategoryStat struct {
		Category   string  `gorm:"column:category"`
		TotalScore float64 `gorm:"column:total_score"`
		VideoCount int     `gorm:"column:video_count"`
	}

	var stats []CategoryStat
	err := s.db.WithContext(ctx).
		Table("video_hot_scores vhs").
		Select("v.category, SUM(vhs.hot_score) as total_score, COUNT(DISTINCT vhs.video_id) as video_count").
		Joins("JOIN videos v ON vhs.video_id = v.video_id").
		Where("vhs.time_window = '24h' AND v.deleted_at IS NULL AND v.category IS NOT NULL AND v.category != ''").
		Group("v.category").
		Order("total_score DESC").
		Find(&stats).Error

	if err != nil {
		return nil, err
	}

	var result []*CategoryHotStats
	for _, stat := range stats {
		result = append(result, &CategoryHotStats{
			Category:   stat.Category,
			HotScore:   stat.TotalScore,
			VideoCount: stat.VideoCount,
		})
	}

	return result, nil
}

// =====================================================
// 实时热度更新接口
// =====================================================

// IncrementVideoInteraction 增量更新视频互动（可用于实时更新）
func (s *VideoHotScoreService) IncrementVideoInteraction(ctx context.Context, videoId int64, actionType string) error {
	// 获取当前视频热度
	for _, tw := range s.config.TimeWindows {
		hotScore, err := db.GetVideoHotScore(ctx, videoId, tw.Name)
		if err != nil {
			continue
		}

		if hotScore == nil {
			// 如果不存在，创建新记录
			hotScore = &model.VideoHotScore{
				VideoID:     videoId,
				TimeWindow:  tw.Name,
				WindowStart: time.Now().Add(-tw.Duration),
				WindowEnd:   time.Now(),
				UpdatedAt:   time.Now(),
			}
		}

		// 根据动作类型增加计数
		var deltaScore float64
		switch actionType {
		case "view":
			hotScore.ViewCount++
			deltaScore = s.config.ViewWeight
		case "like":
			hotScore.LikeCount++
			deltaScore = s.config.LikeWeight
		case "comment":
			hotScore.CommentCount++
			deltaScore = s.config.CommentWeight
		case "share":
			hotScore.ShareCount++
			deltaScore = s.config.ShareWeight
		}

		// 更新热度分
		hotScore.HotScore += deltaScore
		hotScore.UpdatedAt = time.Now()

		if err := db.CreateOrUpdateVideoHotScore(ctx, hotScore); err != nil {
			hlog.Warnf("[HotScoreService] Failed to update hot score for video %d: %v", videoId, err)
		}
	}

	return nil
}

// =====================================================
// 定时任务调度器
// =====================================================

// HotScoreScheduler 热度计算调度器
type HotScoreScheduler struct {
	service *VideoHotScoreService
	jobs    []*ScheduledJob
	stopCh  chan struct{}
}

// ScheduledJob 定时任务
type ScheduledJob struct {
	Name     string
	Interval time.Duration
	Task     func(ctx context.Context)
}

// NewHotScoreScheduler 创建调度器
func NewHotScoreScheduler(service *VideoHotScoreService) *HotScoreScheduler {
	scheduler := &HotScoreScheduler{
		service: service,
		stopCh:  make(chan struct{}),
	}

	// 注册定时任务
	scheduler.jobs = []*ScheduledJob{
		{
			Name:     "hot_score_calculation",
			Interval: 5 * time.Minute,
			Task:     service.CalculateAllHotScores,
		},
		{
			Name:     "clean_old_scores",
			Interval: 24 * time.Hour,
			Task:     scheduler.cleanOldHotScores,
		},
		{
			Name:     "update_category_stats",
			Interval: 1 * time.Hour,
			Task:     scheduler.updateCategoryStats,
		},
	}

	return scheduler
}

// Start 启动调度器
func (s *HotScoreScheduler) Start() {
	hlog.Info("[HotScoreScheduler] Starting scheduler...")

	for _, job := range s.jobs {
		go s.runJob(job)
	}
}

// Stop 停止调度器
func (s *HotScoreScheduler) Stop() {
	close(s.stopCh)
	hlog.Info("[HotScoreScheduler] Scheduler stopped")
}

// runJob 运行单个任务
func (s *HotScoreScheduler) runJob(job *ScheduledJob) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	// 先执行一次
	hlog.Infof("[HotScoreScheduler] Running job: %s", job.Name)
	job.Task(context.Background())

	for {
		select {
		case <-ticker.C:
			hlog.Infof("[HotScoreScheduler] Running scheduled job: %s", job.Name)
			job.Task(context.Background())
		case <-s.stopCh:
			return
		}
	}
}

// cleanOldHotScores 清理旧的热度记录
func (s *HotScoreScheduler) cleanOldHotScores(ctx context.Context) {
	// 保留最近7天的数据
	cutoffTime := time.Now().AddDate(0, 0, -7)
	s.service.db.WithContext(ctx).
		Where("updated_at < ?", cutoffTime).
		Delete(&model.VideoHotScore{})

	hlog.Info("[HotScoreScheduler] Cleaned old hot scores")
}

// updateCategoryStats 更新分类统计
func (s *HotScoreScheduler) updateCategoryStats(ctx context.Context) {
	stats, err := s.service.GetCategoryHotStats(ctx)
	if err != nil {
		hlog.Errorf("[HotScoreScheduler] Failed to get category stats: %v", err)
		return
	}

	for _, stat := range stats {
		categoryStats := &model.CategoryVideoStats{
			Category:    stat.Category,
			TotalVideos: int64(stat.VideoCount),
			HotScore:    stat.HotScore,
			UpdatedAt:   time.Now(),
		}
		if err := db.UpdateCategoryStats(ctx, categoryStats); err != nil {
			hlog.Warnf("[HotScoreScheduler] Failed to update category stats for %s: %v", stat.Category, err)
		}
	}

	hlog.Infof("[HotScoreScheduler] Updated %d category stats", len(stats))
}
