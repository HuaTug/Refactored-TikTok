package recommendation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"HuaTug.com/internal/model"
	"HuaTug.com/cmd/video/dal/db"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/gorm"
)

// Action type constants for interaction increment.
const (
	ActionView    = "view"
	ActionLike    = "like"
	ActionComment = "comment"
	ActionShare   = "share"
)

// Score calculation constants.
const (
	baseScoreWeight  = 0.3 // Weight of base score in composite hot score
	deltaScoreWeight = 0.7 // Weight of delta score in composite hot score

	logCompressionFactor = 100.0 // Multiplier after log10 compression
	minDecayFactor       = 0.1   // Minimum decay factor to retain at least 10% weight
	finishRateThreshold  = 0.5   // Completion rate threshold for quality bonus
	finishRateBonusMul   = 0.5   // Multiplier for finish rate bonus

	trendQueryLimit   = 200  // Max videos to query for trend analysis
	trendRisingPct    = 20.0 // Growth rate (%) threshold for "rising"
	trendFallingPct   = -20.0
	cleanupRetainDays = 7 // Days to retain hot score records
)

// Trend type labels.
const (
	TrendTypeRising  = "rising"
	TrendTypeStable  = "stable"
	TrendTypeFalling = "falling"
	TrendTypeNew     = "new"
)

// --- Type Definitions ---

// TimeWindow defines a time window for hot score calculation.
type TimeWindow struct {
	Name     string        // Window name: 1h/6h/24h/7d
	Duration time.Duration // Time span
	Weight   float64       // Window weight for composite score
}

// HotScoreConfig holds configuration for hot score calculation.
type HotScoreConfig struct {
	TimeWindows []TimeWindow

	ViewWeight     float64
	LikeWeight     float64
	CommentWeight  float64
	ShareWeight    float64
	FavoriteWeight float64

	DecayHalfLife time.Duration // Half-life for exponential decay
	QualityBonus  float64       // Multiplier for high-quality content

	BatchSize       int
	CalculateWorker int
	UpdateInterval  time.Duration
}

// VideoDetail holds video detail info for hot score calculation.
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

// InteractionDelta holds interaction increments within a time window.
type InteractionDelta struct {
	VideoID      int64
	TimeWindow   string
	ViewDelta    int64
	LikeDelta    int64
	CommentDelta int64
	ShareDelta   int64
}

// HotTrend represents a video's trending information.
type HotTrend struct {
	VideoID     int64   `json:"video_id"`
	CurrentRank int     `json:"current_rank"`
	TrendScore  float64 `json:"trend_score"`
	TrendType   string  `json:"trend_type"` // rising/stable/falling/new
}

// CategoryHotStats holds hot score statistics per category.
type CategoryHotStats struct {
	Category   string  `json:"category"`
	HotScore   float64 `json:"hot_score"`
	VideoCount int     `json:"video_count"`
	TrendScore float64 `json:"trend_score"`
}

// --- Default Configuration ---

// DefaultHotScoreConfig returns the default hot score configuration.
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

// --- VideoHotScoreService ---

// VideoHotScoreService calculates and manages video hot scores periodically.
type VideoHotScoreService struct {
	config    *HotScoreConfig
	db        *gorm.DB
	stopCh    chan struct{}
	isRunning bool
	mu        sync.RWMutex
}

// NewVideoHotScoreService creates a new hot score service.
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

// Start launches the periodic hot score calculation.
// NOTE: This only starts the ticker-based loop. The HotScoreScheduler should NOT
// register a duplicate hot_score_calculation job; it is handled here.
func (s *VideoHotScoreService) Start() {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.mu.Unlock()

	hlog.Info("[HotScoreService] Starting video hot score calculation service...")

	go func() {
		// Run an initial calculation, then enter ticker loop.
		s.CalculateAllHotScores(context.Background())

		ticker := time.NewTicker(s.config.UpdateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.CalculateAllHotScores(context.Background())
			case <-s.stopCh:
				hlog.Info("[HotScoreService] Hot score calculation service stopped")
				return
			}
		}
	}()
}

// Stop gracefully stops the service.
func (s *VideoHotScoreService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}
	close(s.stopCh)
	s.isRunning = false
}

// --- Core Calculation ---

// CalculateAllHotScores recalculates hot scores for all active videos.
func (s *VideoHotScoreService) CalculateAllHotScores(ctx context.Context) {
	startTime := time.Now()
	hlog.Info("[HotScoreService] Starting hot score calculation...")

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

	totalProcessed := 0
	for i := 0; i < len(videoIds); i += s.config.BatchSize {
		end := min(i+s.config.BatchSize, len(videoIds))
		batch := videoIds[i:end]

		processed, batchErr := s.calculateBatchHotScores(ctx, batch)
		if batchErr != nil {
			hlog.Errorf("[HotScoreService] Error calculating batch [%d, %d): %v", i, end, batchErr)
			continue
		}
		totalProcessed += processed
	}

	// Update rankings per time window.
	for _, tw := range s.config.TimeWindows {
		if rankErr := db.UpdateHotScoreRanks(ctx, tw.Name); rankErr != nil {
			hlog.Errorf("[HotScoreService] Failed to update ranks for %s: %v", tw.Name, rankErr)
		}
	}

	hlog.Infof("[HotScoreService] Hot score calculation completed. Processed: %d videos, Time: %v",
		totalProcessed, time.Since(startTime))
}

// getActiveVideoIds returns IDs of all public, audit-passed, non-deleted videos.
func (s *VideoHotScoreService) getActiveVideoIds(ctx context.Context) ([]int64, error) {
	var videoIds []int64
	err := s.db.WithContext(ctx).Model(&model.Video{}).
		Where("deleted_at IS NULL AND audit_status = 1 AND open = 1").
		Pluck("video_id", &videoIds).Error
	return videoIds, err
}

// calculateBatchHotScores computes hot scores for a batch of video IDs.
func (s *VideoHotScoreService) calculateBatchHotScores(ctx context.Context, videoIds []int64) (int, error) {
	videos, err := s.getVideoDetails(ctx, videoIds)
	if err != nil {
		return 0, err
	}

	featureMap := s.getVideoFeatureMap(ctx, videoIds)
	interactionDeltas := s.getInteractionDeltas(ctx, videoIds)

	now := time.Now()
	var hotScores []*model.VideoHotScore

	for _, video := range videos {
		for _, tw := range s.config.TimeWindows {
			hs := s.computeHotScore(video, tw, now, featureMap, interactionDeltas)
			hotScores = append(hotScores, hs)
		}
	}

	if len(hotScores) > 0 {
		if saveErr := db.BatchUpdateVideoHotScores(ctx, hotScores); saveErr != nil {
			return 0, saveErr
		}
	}

	return len(videos), nil
}

// getVideoDetails fetches video details from DB for the given IDs.
func (s *VideoHotScoreService) getVideoDetails(ctx context.Context, videoIds []int64) ([]*VideoDetail, error) {
	var videos []*VideoDetail
	err := s.db.WithContext(ctx).Model(&model.Video{}).
		Select("video_id, user_id, visit_count, likes_count, comment_count, share_count, favorites_count, created_at, duration").
		Where("video_id IN ?", videoIds).
		Find(&videos).Error
	return videos, err
}

// getVideoFeatureMap returns a map of VideoFeature keyed by video ID.
func (s *VideoHotScoreService) getVideoFeatureMap(ctx context.Context, videoIds []int64) map[int64]*model.VideoFeature {
	featureMap := make(map[int64]*model.VideoFeature, len(videoIds))
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

// getInteractionDeltas queries interaction increments per time window from exposure records.
func (s *VideoHotScoreService) getInteractionDeltas(ctx context.Context, videoIds []int64) map[string]map[int64]*InteractionDelta {
	deltas := make(map[string]map[int64]*InteractionDelta, len(s.config.TimeWindows))
	now := time.Now()

	type exposureStat struct {
		VideoID      int64 `gorm:"column:video_id"`
		ClickCount   int64 `gorm:"column:click_count"`
		LikeCount    int64 `gorm:"column:like_count"`
		CommentCount int64 `gorm:"column:comment_count"`
		ShareCount   int64 `gorm:"column:share_count"`
	}

	for _, tw := range s.config.TimeWindows {
		windowStart := now.Add(-tw.Duration)
		windowDeltas := make(map[int64]*InteractionDelta)

		var stats []exposureStat
		err := s.db.WithContext(ctx).Model(&model.RecommendationExposure{}).
			Select(`
				video_id,
				COUNT(*) as click_count,
				SUM(is_liked) as like_count,
				SUM(is_commented) as comment_count,
				SUM(is_shared) as share_count
			`).
			Where("video_id IN ? AND exposure_time >= ? AND is_clicked = 1", videoIds, windowStart).
			Group("video_id").
			Find(&stats).Error
		if err != nil {
			hlog.Warnf("[HotScoreService] Failed to get interaction deltas for window %s: %v", tw.Name, err)
			deltas[tw.Name] = windowDeltas
			continue
		}

		for _, stat := range stats {
			windowDeltas[stat.VideoID] = &InteractionDelta{
				VideoID:      stat.VideoID,
				TimeWindow:   tw.Name,
				ViewDelta:    stat.ClickCount,
				LikeDelta:    stat.LikeCount,
				CommentDelta: stat.CommentCount,
				ShareDelta:   stat.ShareCount,
			}
		}
		deltas[tw.Name] = windowDeltas
	}

	return deltas
}

// --- Single Video Score Computation ---

// computeHotScore calculates the composite hot score for a single video + time window.
func (s *VideoHotScoreService) computeHotScore(
	video *VideoDetail,
	tw TimeWindow,
	now time.Time,
	featureMap map[int64]*model.VideoFeature,
	interactionDeltas map[string]map[int64]*InteractionDelta,
) *model.VideoHotScore {

	baseScore := s.calculateBaseScore(video)
	deltaScore := s.calculateDeltaScore(tw.Name, video.VideoId, interactionDeltas)

	videoAge := now.Sub(video.CreatedAt)
	decayFactor := s.calculateDecayFactor(videoAge)
	qualityMul := s.calculateQualityMultiplier(video.VideoId, featureMap)

	// Composite: base * decay + delta, scaled by quality.
	hotScore := (baseScore*baseScoreWeight*decayFactor + deltaScore*deltaScoreWeight) * qualityMul

	// Log compression to prevent extreme values.
	if hotScore > 0 {
		hotScore = math.Log10(hotScore+1) * logCompressionFactor
	}

	return &model.VideoHotScore{
		VideoID:      video.VideoId,
		TimeWindow:   tw.Name,
		ViewCount:    int64(video.VisitCount),
		LikeCount:    int64(video.LikesCount),
		CommentCount: int64(video.CommentCount),
		ShareCount:   int64(video.ShareCount),
		HotScore:     hotScore,
		WindowStart:  now.Add(-tw.Duration),
		WindowEnd:    now,
		UpdatedAt:    now,
	}
}

// calculateBaseScore computes the weighted sum of all-time interaction counts.
func (s *VideoHotScoreService) calculateBaseScore(video *VideoDetail) float64 {
	return float64(video.VisitCount)*s.config.ViewWeight +
		float64(video.LikesCount)*s.config.LikeWeight +
		float64(video.CommentCount)*s.config.CommentWeight +
		float64(video.ShareCount)*s.config.ShareWeight +
		float64(video.FavoritesCount)*s.config.FavoriteWeight
}

// calculateDeltaScore computes the weighted sum of incremental interactions in a time window.
func (s *VideoHotScoreService) calculateDeltaScore(windowName string, videoId int64, deltas map[string]map[int64]*InteractionDelta) float64 {
	windowDeltas, ok := deltas[windowName]
	if !ok {
		return 0
	}
	delta, ok := windowDeltas[videoId]
	if !ok {
		return 0
	}
	return float64(delta.ViewDelta)*s.config.ViewWeight +
		float64(delta.LikeDelta)*s.config.LikeWeight +
		float64(delta.CommentDelta)*s.config.CommentWeight +
		float64(delta.ShareDelta)*s.config.ShareWeight
}

// calculateDecayFactor returns the exponential time-decay factor.
// Formula: f(t) = e^(-λt), λ = ln(2) / half-life. Clamped to minDecayFactor.
func (s *VideoHotScoreService) calculateDecayFactor(age time.Duration) float64 {
	if age <= 0 {
		return 1.0
	}
	lambda := math.Log(2) / float64(s.config.DecayHalfLife)
	factor := math.Exp(-lambda * float64(age))
	if factor < minDecayFactor {
		return minDecayFactor
	}
	return factor
}

// calculateQualityMultiplier returns the quality bonus multiplier for a video.
func (s *VideoHotScoreService) calculateQualityMultiplier(videoId int64, featureMap map[int64]*model.VideoFeature) float64 {
	feature, ok := featureMap[videoId]
	if !ok {
		return 1.0
	}

	multiplier := 1.0
	if feature.IsHighQuality == 1 {
		multiplier = s.config.QualityBonus
	}
	if feature.FinishRate > finishRateThreshold {
		multiplier *= (1 + feature.FinishRate*finishRateBonusMul)
	}
	return multiplier
}

// --- Public API ---

// CalculateVideoHotScore recalculates hot scores for a single video (real-time API).
func (s *VideoHotScoreService) CalculateVideoHotScore(ctx context.Context, videoId int64) error {
	videos, err := s.getVideoDetails(ctx, []int64{videoId})
	if err != nil {
		return err
	}
	if len(videos) == 0 {
		return fmt.Errorf("video %d not found", videoId)
	}

	featureMap := s.getVideoFeatureMap(ctx, []int64{videoId})
	interactionDeltas := s.getInteractionDeltas(ctx, []int64{videoId})

	now := time.Now()
	hotScores := make([]*model.VideoHotScore, 0, len(s.config.TimeWindows))
	for _, tw := range s.config.TimeWindows {
		hs := s.computeHotScore(videos[0], tw, now, featureMap, interactionDeltas)
		hotScores = append(hotScores, hs)
	}

	return db.BatchUpdateVideoHotScores(ctx, hotScores)
}

// GetTopHotVideos returns the top hot video IDs for a given time window.
func (s *VideoHotScoreService) GetTopHotVideos(ctx context.Context, timeWindow string, limit int) ([]int64, error) {
	return db.GetHotVideoIds(ctx, timeWindow, limit)
}

// GetVideoHotRank returns the rank and score of a video in a given time window.
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

// --- Trending Analysis ---

// GetTrendingVideos returns videos with the fastest-rising hot scores.
func (s *VideoHotScoreService) GetTrendingVideos(ctx context.Context, limit int) ([]*HotTrend, error) {
	hotScores1h, err := db.GetHotVideosByWindow(ctx, "1h", trendQueryLimit)
	if err != nil {
		return nil, err
	}

	hotScores24h, err := db.GetHotVideosByWindow(ctx, "24h", trendQueryLimit)
	if err != nil {
		return nil, err
	}

	score24hMap := make(map[int64]float64, len(hotScores24h))
	for _, hs := range hotScores24h {
		score24hMap[hs.VideoID] = hs.HotScore
	}

	trends := make([]*HotTrend, 0, len(hotScores1h))
	for i, hs := range hotScores1h {
		trend := &HotTrend{
			VideoID:     hs.VideoID,
			CurrentRank: i + 1,
		}

		prev, exists := score24hMap[hs.VideoID]
		if !exists || prev == 0 {
			trend.TrendType = TrendTypeNew
			trend.TrendScore = hs.HotScore
		} else {
			growthRate := (hs.HotScore - prev) / prev * 100
			trend.TrendScore = growthRate
			trend.TrendType = classifyTrend(growthRate)
		}

		trends = append(trends, trend)
	}

	// Sort descending by trend score.
	sort.Slice(trends, func(i, j int) bool {
		return trends[i].TrendScore > trends[j].TrendScore
	})

	// Filter to rising/new only.
	risingTrends := make([]*HotTrend, 0, limit)
	for _, t := range trends {
		if t.TrendType == TrendTypeRising || t.TrendType == TrendTypeNew {
			risingTrends = append(risingTrends, t)
			if len(risingTrends) >= limit {
				break
			}
		}
	}

	return risingTrends, nil
}

// classifyTrend returns the trend label based on growth rate percentage.
func classifyTrend(growthRate float64) string {
	switch {
	case growthRate > trendRisingPct:
		return TrendTypeRising
	case growthRate < trendFallingPct:
		return TrendTypeFalling
	default:
		return TrendTypeStable
	}
}

// --- Category Stats ---

// GetCategoryHotStats returns hot score statistics grouped by video category.
func (s *VideoHotScoreService) GetCategoryHotStats(ctx context.Context) ([]*CategoryHotStats, error) {
	type categoryStat struct {
		Category   string  `gorm:"column:category"`
		TotalScore float64 `gorm:"column:total_score"`
		VideoCount int     `gorm:"column:video_count"`
	}

	var stats []categoryStat
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

	result := make([]*CategoryHotStats, 0, len(stats))
	for _, stat := range stats {
		result = append(result, &CategoryHotStats{
			Category:   stat.Category,
			HotScore:   stat.TotalScore,
			VideoCount: stat.VideoCount,
		})
	}

	return result, nil
}

// --- Real-time Interaction Update ---

// IncrementVideoInteraction updates a video's hot score incrementally upon user interaction.
func (s *VideoHotScoreService) IncrementVideoInteraction(ctx context.Context, videoId int64, actionType string) error {
	weight, err := s.actionWeight(actionType)
	if err != nil {
		return err
	}

	var firstErr error
	for _, tw := range s.config.TimeWindows {
		if updateErr := s.applyInteractionIncrement(ctx, videoId, tw, actionType, weight); updateErr != nil {
			hlog.Warnf("[HotScoreService] Failed to update hot score for video %d window %s: %v",
				videoId, tw.Name, updateErr)
			if firstErr == nil {
				firstErr = updateErr
			}
		}
	}

	return firstErr
}

// actionWeight returns the configured weight for a given action type.
func (s *VideoHotScoreService) actionWeight(actionType string) (float64, error) {
	switch actionType {
	case ActionView:
		return s.config.ViewWeight, nil
	case ActionLike:
		return s.config.LikeWeight, nil
	case ActionComment:
		return s.config.CommentWeight, nil
	case ActionShare:
		return s.config.ShareWeight, nil
	default:
		return 0, fmt.Errorf("unknown action type: %s", actionType)
	}
}

// applyInteractionIncrement applies a single interaction increment to a hot score record.
func (s *VideoHotScoreService) applyInteractionIncrement(
	ctx context.Context,
	videoId int64,
	tw TimeWindow,
	actionType string,
	weight float64,
) error {
	hotScore, err := db.GetVideoHotScore(ctx, videoId, tw.Name)
	if err != nil {
		return err
	}

	if hotScore == nil {
		hotScore = &model.VideoHotScore{
			VideoID:     videoId,
			TimeWindow:  tw.Name,
			WindowStart: time.Now().Add(-tw.Duration),
			WindowEnd:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}

	switch actionType {
	case ActionView:
		hotScore.ViewCount++
	case ActionLike:
		hotScore.LikeCount++
	case ActionComment:
		hotScore.CommentCount++
	case ActionShare:
		hotScore.ShareCount++
	}

	hotScore.HotScore += weight
	hotScore.UpdatedAt = time.Now()

	return db.CreateOrUpdateVideoHotScore(ctx, hotScore)
}

// --- Scheduler ---

// HotScoreScheduler manages periodic auxiliary jobs (cleanup, category stats).
// NOTE: The primary hot_score_calculation is handled by VideoHotScoreService.Start(),
// so it is NOT registered here to avoid duplicate execution.
type HotScoreScheduler struct {
	service *VideoHotScoreService
	jobs    []*ScheduledJob
	stopCh  chan struct{}
}

// ScheduledJob represents a periodic task.
type ScheduledJob struct {
	Name     string
	Interval time.Duration
	Task     func(ctx context.Context)
}

// NewHotScoreScheduler creates a scheduler with auxiliary jobs only.
func NewHotScoreScheduler(service *VideoHotScoreService) *HotScoreScheduler {
	scheduler := &HotScoreScheduler{
		service: service,
		stopCh:  make(chan struct{}),
	}

	scheduler.jobs = []*ScheduledJob{
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

// Start launches all scheduled jobs in separate goroutines.
func (s *HotScoreScheduler) Start() {
	hlog.Info("[HotScoreScheduler] Starting scheduler...")
	for _, job := range s.jobs {
		go s.runJob(job)
	}
}

// Stop signals all jobs to stop.
func (s *HotScoreScheduler) Stop() {
	close(s.stopCh)
	hlog.Info("[HotScoreScheduler] Scheduler stopped")
}

// runJob executes a job immediately, then on each tick until stopped.
func (s *HotScoreScheduler) runJob(job *ScheduledJob) {
	hlog.Infof("[HotScoreScheduler] Running initial job: %s", job.Name)
	job.Task(context.Background())

	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

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

// cleanOldHotScores removes hot score records older than cleanupRetainDays.
func (s *HotScoreScheduler) cleanOldHotScores(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -cleanupRetainDays)
	result := s.service.db.WithContext(ctx).
		Where("updated_at < ?", cutoff).
		Delete(&model.VideoHotScore{})
	if result.Error != nil {
		hlog.Errorf("[HotScoreScheduler] Failed to clean old hot scores: %v", result.Error)
		return
	}
	hlog.Infof("[HotScoreScheduler] Cleaned old hot scores, rows affected: %d", result.RowsAffected)
}

// updateCategoryStats refreshes category-level aggregated statistics.
func (s *HotScoreScheduler) updateCategoryStats(ctx context.Context) {
	stats, err := s.service.GetCategoryHotStats(ctx)
	if err != nil {
		hlog.Errorf("[HotScoreScheduler] Failed to get category stats: %v", err)
		return
	}

	for _, stat := range stats {
		catStats := &model.CategoryVideoStats{
			Category:    stat.Category,
			TotalVideos: int64(stat.VideoCount),
			HotScore:    stat.HotScore,
			UpdatedAt:   time.Now(),
		}
		if updateErr := db.UpdateCategoryStats(ctx, catStats); updateErr != nil {
			hlog.Warnf("[HotScoreScheduler] Failed to update category stats for %s: %v",
				stat.Category, updateErr)
		}
	}

	hlog.Infof("[HotScoreScheduler] Updated %d category stats", len(stats))
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
