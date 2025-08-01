package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/gorm"
)

// DataConsistencyService 数据一致性检查服务
type DataConsistencyService struct {
	ctx           context.Context
	cancel        context.CancelFunc
	db            *gorm.DB
	cacheManager  *redis.ImprovedCacheManager
	checkInterval time.Duration
	isRunning     bool
}

// ConsistencyCheckResult 一致性检查结果
type ConsistencyCheckResult struct {
	ResourceType  string      `json:"resource_type"`
	ResourceID    int64       `json:"resource_id"`
	CacheValue    interface{} `json:"cache_value"`
	DatabaseValue interface{} `json:"database_value"`
	IsConsistent  bool        `json:"is_consistent"`
	Difference    string      `json:"difference"`
	CheckTime     time.Time   `json:"check_time"`
}

// NewDataConsistencyService 创建数据一致性检查服务
func NewDataConsistencyService(db *gorm.DB, cacheManager *redis.ImprovedCacheManager) *DataConsistencyService {
	ctx, cancel := context.WithCancel(context.Background())

	return &DataConsistencyService{
		ctx:           ctx,
		cancel:        cancel,
		db:            db,
		cacheManager:  cacheManager,
		checkInterval: 5 * time.Minute, // 每5分钟检查一次
		isRunning:     false,
	}
}

// Start 启动一致性检查服务
func (dcs *DataConsistencyService) Start() error {
	if dcs.isRunning {
		return fmt.Errorf("consistency check service is already running")
	}

	dcs.isRunning = true
	hlog.Info("Starting data consistency check service...")

	// 启动定期检查
	go dcs.runPeriodicCheck()

	// 启动热点数据检查
	go dcs.runHotDataCheck()

	hlog.Info("Data consistency check service started")
	return nil
}

// Stop 停止一致性检查服务
func (dcs *DataConsistencyService) Stop() error {
	if !dcs.isRunning {
		return nil
	}

	dcs.isRunning = false
	dcs.cancel()

	hlog.Info("Data consistency check service stopped")
	return nil
}

// runPeriodicCheck 运行定期检查
func (dcs *DataConsistencyService) runPeriodicCheck() {
	ticker := time.NewTicker(dcs.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dcs.ctx.Done():
			hlog.Info("Periodic consistency check stopped")
			return
		case <-ticker.C:
			dcs.performConsistencyCheck()
		}
	}
}

// runHotDataCheck 运行热点数据检查
func (dcs *DataConsistencyService) runHotDataCheck() {
	ticker := time.NewTicker(time.Minute) // 每分钟检查热点数据
	defer ticker.Stop()

	for {
		select {
		case <-dcs.ctx.Done():
			hlog.Info("Hot data consistency check stopped")
			return
		case <-ticker.C:
			dcs.checkHotVideoData()
		}
	}
}

// performConsistencyCheck 执行一致性检查
func (dcs *DataConsistencyService) performConsistencyCheck() {
	hlog.Info("Starting consistency check...")
	start := time.Now()

	// 检查视频点赞数据
	videoResults := dcs.checkVideoLikeConsistency()

	// 检查评论点赞数据
	commentResults := dcs.checkCommentLikeConsistency()

	// 记录检查结果
	totalChecked := len(videoResults) + len(commentResults)
	inconsistentCount := 0

	for _, result := range videoResults {
		if !result.IsConsistent {
			inconsistentCount++
			dcs.handleInconsistency(&result)
		}
		dcs.saveCheckResult(&result)
	}

	for _, result := range commentResults {
		if !result.IsConsistent {
			inconsistentCount++
			dcs.handleInconsistency(&result)
		}
		dcs.saveCheckResult(&result)
	}

	duration := time.Since(start)
	hlog.Infof("Consistency check completed: checked=%d, inconsistent=%d, duration=%v",
		totalChecked, inconsistentCount, duration)
}

// checkVideoLikeConsistency 检查视频点赞数据一致性
func (dcs *DataConsistencyService) checkVideoLikeConsistency() []ConsistencyCheckResult {
	var results []ConsistencyCheckResult

	// 获取最近活跃的视频ID列表
	videoIDs := dcs.getActiveVideoIDs(100) // 检查最近100个活跃视频

	for _, videoID := range videoIDs {
		result := dcs.checkSingleVideoConsistency(videoID)
		results = append(results, result)
	}

	return results
}

// checkCommentLikeConsistency 检查评论点赞数据一致性
func (dcs *DataConsistencyService) checkCommentLikeConsistency() []ConsistencyCheckResult {
	var results []ConsistencyCheckResult

	// 获取最近活跃的评论ID列表
	commentIDs := dcs.getActiveCommentIDs(100) // 检查最近100个活跃评论

	for _, commentID := range commentIDs {
		result := dcs.checkSingleCommentConsistency(commentID)
		results = append(results, result)
	}

	return results
}

// checkSingleVideoConsistency 检查单个视频的一致性
func (dcs *DataConsistencyService) checkSingleVideoConsistency(videoID int64) ConsistencyCheckResult {
	result := ConsistencyCheckResult{
		ResourceType: "video",
		ResourceID:   videoID,
		CheckTime:    time.Now(),
	}

	// 从缓存获取点赞数
	cacheCount, _, err := dcs.cacheManager.GetVideoLikeCountWithConsistency(dcs.ctx, videoID)
	if err != nil {
		hlog.Errorf("Failed to get video like count from cache: %v", err)
		cacheCount = -1 // 标记为缓存获取失败
	}
	result.CacheValue = cacheCount

	// 从数据库获取点赞数
	var dbCount int64
	err = dcs.db.Model(&model.VideoLike{}).Where("video_id = ?", videoID).Count(&dbCount).Error
	if err != nil {
		hlog.Errorf("Failed to get video like count from database: %v", err)
		dbCount = -1 // 标记为数据库获取失败
	}
	result.DatabaseValue = dbCount

	// 比较一致性
	if cacheCount == -1 || dbCount == -1 {
		result.IsConsistent = false
		result.Difference = "Failed to retrieve data from cache or database"
	} else if cacheCount == dbCount {
		result.IsConsistent = true
	} else {
		result.IsConsistent = false
		result.Difference = fmt.Sprintf("Cache: %d, Database: %d, Diff: %d",
			cacheCount, dbCount, cacheCount-dbCount)
	}

	return result
}

// checkSingleCommentConsistency 检查单个评论的一致性
func (dcs *DataConsistencyService) checkSingleCommentConsistency(commentID int64) ConsistencyCheckResult {
	result := ConsistencyCheckResult{
		ResourceType: "comment",
		ResourceID:   commentID,
		CheckTime:    time.Now(),
	}

	// 从缓存获取点赞数
	cacheCount, _, err := dcs.cacheManager.GetCommentLikeCountWithConsistency(dcs.ctx, commentID)
	if err != nil {
		hlog.Errorf("Failed to get comment like count from cache: %v", err)
		cacheCount = -1
	}
	result.CacheValue = cacheCount

	// 从数据库获取点赞数
	var dbCount int64
	err = dcs.db.Model(&model.CommentLike{}).Where("comment_id = ?", commentID).Count(&dbCount).Error
	if err != nil {
		hlog.Errorf("Failed to get comment like count from database: %v", err)
		dbCount = -1
	}
	result.DatabaseValue = dbCount

	// 比较一致性
	if cacheCount == -1 || dbCount == -1 {
		result.IsConsistent = false
		result.Difference = "Failed to retrieve data from cache or database"
	} else if cacheCount == dbCount {
		result.IsConsistent = true
	} else {
		result.IsConsistent = false
		result.Difference = fmt.Sprintf("Cache: %d, Database: %d, Diff: %d",
			cacheCount, dbCount, cacheCount-dbCount)
	}

	return result
}

// handleInconsistency 处理数据不一致
func (dcs *DataConsistencyService) handleInconsistency(result *ConsistencyCheckResult) {
	hlog.Warnf("Data inconsistency detected: %s %d - %s",
		result.ResourceType, result.ResourceID, result.Difference)

	// 自动修复策略：以数据库为准更新缓存
	switch result.ResourceType {
	case "video":
		dcs.fixVideoLikeConsistency(result.ResourceID, result.DatabaseValue.(int64))
	case "comment":
		dcs.fixCommentLikeConsistency(result.ResourceID, result.DatabaseValue.(int64))
	}
}

// fixVideoLikeConsistency 修复视频点赞数据一致性
func (dcs *DataConsistencyService) fixVideoLikeConsistency(videoID int64, correctCount int64) {
	if correctCount < 0 {
		return // 数据库查询失败，不进行修复
	}

	err := dcs.cacheManager.SetVideoLikeCountWithConsistency(dcs.ctx, videoID, correctCount, time.Now().UnixNano())
	if err != nil {
		hlog.Errorf("Failed to fix video like consistency: %v", err)
		return
	}

	hlog.Infof("Fixed video like consistency: video_id=%d, count=%d", videoID, correctCount)
}

// fixCommentLikeConsistency 修复评论点赞数据一致性
func (dcs *DataConsistencyService) fixCommentLikeConsistency(commentID int64, correctCount int64) {
	if correctCount < 0 {
		return
	}

	err := dcs.cacheManager.SetCommentLikeCountWithConsistency(dcs.ctx, commentID, correctCount, time.Now().UnixNano())
	if err != nil {
		hlog.Errorf("Failed to fix comment like consistency: %v", err)
		return
	}

	hlog.Infof("Fixed comment like consistency: comment_id=%d, count=%d", commentID, correctCount)
}

// saveCheckResult 保存检查结果
func (dcs *DataConsistencyService) saveCheckResult(result *ConsistencyCheckResult) {
	cacheValueJSON, _ := json.Marshal(result.CacheValue)
	dbValueJSON, _ := json.Marshal(result.DatabaseValue)

	checkRecord := &model.DataConsistencyCheck{
		CheckType:     "like_count",
		ResourceType:  result.ResourceType,
		ResourceID:    result.ResourceID,
		CacheValue:    string(cacheValueJSON),
		DatabaseValue: string(dbValueJSON),
		IsConsistent:  result.IsConsistent,
		Difference:    result.Difference,
		CheckTime:     result.CheckTime,
		CreatedAt:     time.Now(),
	}

	if !result.IsConsistent {
		// 记录修复时间
		now := time.Now()
		checkRecord.FixedAt = &now
	}

	if err := dcs.db.Create(checkRecord).Error; err != nil {
		hlog.Errorf("Failed to save consistency check result: %v", err)
	}
}

// getActiveVideoIDs 获取活跃视频ID列表
func (dcs *DataConsistencyService) getActiveVideoIDs(limit int) []int64 {
	var videoIDs []int64

	// 从最近的点赞记录中获取活跃视频
	err := dcs.db.Model(&model.VideoLike{}).
		Select("DISTINCT video_id").
		Where("created_at > ?", time.Now().Add(-24*time.Hour)). // 最近24小时
		Order("created_at DESC").
		Limit(limit).
		Pluck("video_id", &videoIDs).Error

	if err != nil {
		hlog.Errorf("Failed to get active video IDs: %v", err)
		return []int64{}
	}

	return videoIDs
}

// getActiveCommentIDs 获取活跃评论ID列表
func (dcs *DataConsistencyService) getActiveCommentIDs(limit int) []int64 {
	var commentIDs []int64

	// 从最近的点赞记录中获取活跃评论
	err := dcs.db.Model(&model.CommentLike{}).
		Select("DISTINCT comment_id").
		Where("created_at > ?", time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").
		Limit(limit).
		Pluck("comment_id", &commentIDs).Error

	if err != nil {
		hlog.Errorf("Failed to get active comment IDs: %v", err)
		return []int64{}
	}

	return commentIDs
}

// checkHotVideoData 检查热点视频数据
func (dcs *DataConsistencyService) checkHotVideoData() {
	// 获取热点视频（点赞数较多的视频）
	var hotVideos []struct {
		VideoID   int64 `gorm:"column:video_id"`
		LikeCount int64 `gorm:"column:like_count"`
	}

	err := dcs.db.Model(&model.VideoLike{}).
		Select("video_id, COUNT(*) as like_count").
		Group("video_id").
		Having("COUNT(*) > ?", 100). // 点赞数超过100的视频
		Order("like_count DESC").
		Limit(20). // 检查前20个热点视频
		Find(&hotVideos).Error

	if err != nil {
		hlog.Errorf("Failed to get hot videos: %v", err)
		return
	}

	for _, video := range hotVideos {
		result := dcs.checkSingleVideoConsistency(video.VideoID)
		if !result.IsConsistent {
			hlog.Warnf("Hot video inconsistency detected: video_id=%d", video.VideoID)
			dcs.handleInconsistency(&result)
		}
	}
}

// GetConsistencyReport 获取一致性报告
func (dcs *DataConsistencyService) GetConsistencyReport(ctx context.Context, hours int) (*ConsistencyReport, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	var checks []model.DataConsistencyCheck
	err := dcs.db.Where("check_time > ?", since).Find(&checks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get consistency checks: %w", err)
	}

	report := &ConsistencyReport{
		TimeRange:   fmt.Sprintf("Last %d hours", hours),
		TotalChecks: len(checks),
		GeneratedAt: time.Now(),
	}

	for _, check := range checks {
		if check.IsConsistent {
			report.ConsistentCount++
		} else {
			report.InconsistentCount++
			report.InconsistentItems = append(report.InconsistentItems, InconsistentItem{
				ResourceType: check.ResourceType,
				ResourceID:   check.ResourceID,
				Difference:   check.Difference,
				CheckTime:    check.CheckTime,
				Fixed:        check.FixedAt != nil,
			})
		}
	}

	if report.TotalChecks > 0 {
		report.ConsistencyRate = float64(report.ConsistentCount) / float64(report.TotalChecks) * 100
	}

	return report, nil
}

// ConsistencyReport 一致性报告
type ConsistencyReport struct {
	TimeRange         string             `json:"time_range"`
	TotalChecks       int                `json:"total_checks"`
	ConsistentCount   int                `json:"consistent_count"`
	InconsistentCount int                `json:"inconsistent_count"`
	ConsistencyRate   float64            `json:"consistency_rate"`
	InconsistentItems []InconsistentItem `json:"inconsistent_items"`
	GeneratedAt       time.Time          `json:"generated_at"`
}

// InconsistentItem 不一致项目
type InconsistentItem struct {
	ResourceType string    `json:"resource_type"`
	ResourceID   int64     `json:"resource_id"`
	Difference   string    `json:"difference"`
	CheckTime    time.Time `json:"check_time"`
	Fixed        bool      `json:"fixed"`
}

// ManualCheck 手动检查指定资源的一致性
func (dcs *DataConsistencyService) ManualCheck(ctx context.Context, resourceType string, resourceID int64) (*ConsistencyCheckResult, error) {
	var result ConsistencyCheckResult

	switch resourceType {
	case "video":
		result = dcs.checkSingleVideoConsistency(resourceID)
	case "comment":
		result = dcs.checkSingleCommentConsistency(resourceID)
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	// 保存检查结果
	dcs.saveCheckResult(&result)

	// 如果不一致，自动修复
	if !result.IsConsistent {
		dcs.handleInconsistency(&result)
	}

	return &result, nil
}

// CleanupOldRecords 清理旧的检查记录
func (dcs *DataConsistencyService) CleanupOldRecords(ctx context.Context, days int) error {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	result := dcs.db.Where("created_at < ?", cutoff).Delete(&model.DataConsistencyCheck{})
	if result.Error != nil {
		return fmt.Errorf("failed to cleanup old records: %w", result.Error)
	}

	hlog.Infof("Cleaned up %d old consistency check records", result.RowsAffected)
	return nil
}
