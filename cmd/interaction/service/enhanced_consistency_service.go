package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/gorm"
)

// EnhancedConsistencyService 增强的数据一致性服务
type EnhancedConsistencyService struct {
	db           *gorm.DB
	cacheManager *redis.LikeCacheManager
	producer     mq.MessageProducer
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex

	// 配置参数
	checkInterval    time.Duration
	batchSize        int
	maxRetries       int
	inconsistencyLog []InconsistencyRecord
}

// InconsistencyRecord 不一致记录
type InconsistencyRecord struct {
	ResourceType  string     `json:"resource_type"` // video, comment
	ResourceID    int64      `json:"resource_id"`
	CacheValue    int64      `json:"cache_value"`
	DatabaseValue int64      `json:"database_value"`
	Difference    int64      `json:"difference"`
	DetectedAt    time.Time  `json:"detected_at"`
	FixedAt       *time.Time `json:"fixed_at,omitempty"`
	Status        string     `json:"status"` // detected, fixing, fixed, failed
}

// ConsistencyReport 一致性报告
type ConsistencyReport struct {
	CheckTime         time.Time             `json:"check_time"`
	TotalChecked      int                   `json:"total_checked"`
	InconsistentCount int                   `json:"inconsistent_count"`
	FixedCount        int                   `json:"fixed_count"`
	FailedCount       int                   `json:"failed_count"`
	InconsistencyRate float64               `json:"inconsistency_rate"`
	Details           []InconsistencyRecord `json:"details"`
}

// NewEnhancedConsistencyService 创建增强的一致性服务
func NewEnhancedConsistencyService(db *gorm.DB, cacheManager *redis.LikeCacheManager, producer mq.MessageProducer) *EnhancedConsistencyService {
	ctx, cancel := context.WithCancel(context.Background())

	return &EnhancedConsistencyService{
		db:               db,
		cacheManager:     cacheManager,
		producer:         producer,
		ctx:              ctx,
		cancel:           cancel,
		checkInterval:    5 * time.Minute, // 每5分钟检查一次
		batchSize:        100,             // 每批处理100条记录
		maxRetries:       3,               // 最大重试3次
		inconsistencyLog: make([]InconsistencyRecord, 0),
	}
}

// Start 启动一致性检查服务
func (ecs *EnhancedConsistencyService) Start() {
	hlog.Info("Enhanced consistency service started")

	ticker := time.NewTicker(ecs.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ecs.ctx.Done():
			hlog.Info("Enhanced consistency service stopped")
			return
		case <-ticker.C:
			if err := ecs.performConsistencyCheck(); err != nil {
				hlog.Errorf("Consistency check failed: %v", err)
			}
		}
	}
}

// Stop 停止一致性检查服务
func (ecs *EnhancedConsistencyService) Stop() {
	ecs.cancel()
}

// performConsistencyCheck 执行一致性检查
func (ecs *EnhancedConsistencyService) performConsistencyCheck() error {
	hlog.Info("Starting consistency check...")

	report := &ConsistencyReport{
		CheckTime: time.Now(),
		Details:   make([]InconsistencyRecord, 0),
	}

	// 检查视频点赞数一致性
	if err := ecs.checkVideoLikeConsistency(report); err != nil {
		hlog.Errorf("Video like consistency check failed: %v", err)
	}

	// 检查评论点赞数一致性
	if err := ecs.checkCommentLikeConsistency(report); err != nil {
		hlog.Errorf("Comment like consistency check failed: %v", err)
	}

	// 计算统计信息
	report.InconsistentCount = len(report.Details)
	if report.TotalChecked > 0 {
		report.InconsistencyRate = float64(report.InconsistentCount) / float64(report.TotalChecked)
	}

	// 记录报告
	ecs.logConsistencyReport(report)

	hlog.Infof("Consistency check completed: checked=%d, inconsistent=%d, rate=%.2f%%",
		report.TotalChecked, report.InconsistentCount, report.InconsistencyRate*100)

	return nil
}

// checkVideoLikeConsistency 检查视频点赞数一致性
func (ecs *EnhancedConsistencyService) checkVideoLikeConsistency(report *ConsistencyReport) error {
	// 分批获取视频数据
	offset := 0
	for {
		var videos []model.Video
		if err := ecs.db.Offset(offset).Limit(ecs.batchSize).Find(&videos).Error; err != nil {
			return fmt.Errorf("failed to fetch videos: %w", err)
		}

		if len(videos) == 0 {
			break
		}

		for _, video := range videos {
			report.TotalChecked++

			// 获取缓存中的点赞数
			cacheCount, err := ecs.cacheManager.GetVideoLikeCount(ecs.ctx, video.VideoId)
			if err != nil {
				hlog.Warnf("Failed to get cache count for video %d: %v", video.VideoId, err)
				continue
			}

			// 比较数据库和缓存的值
			if cacheCount != video.LikeCount {
				record := InconsistencyRecord{
					ResourceType:  "video",
					ResourceID:    video.VideoId,
					CacheValue:    cacheCount,
					DatabaseValue: video.LikeCount,
					Difference:    cacheCount - video.LikeCount,
					DetectedAt:    time.Now(),
					Status:        "detected",
				}

				report.Details = append(report.Details, record)

				// 尝试修复不一致
				if err := ecs.fixVideoLikeInconsistency(video.VideoId, cacheCount, video.LikeCount); err != nil {
					hlog.Errorf("Failed to fix video %d inconsistency: %v", video.VideoId, err)
					record.Status = "failed"
					report.FailedCount++
				} else {
					record.Status = "fixed"
					now := time.Now()
					record.FixedAt = &now
					report.FixedCount++
				}

				// 记录不一致
				ecs.recordInconsistency(record)
			}
		}

		offset += ecs.batchSize
	}

	return nil
}

// checkCommentLikeConsistency 检查评论点赞数一致性
func (ecs *EnhancedConsistencyService) checkCommentLikeConsistency(report *ConsistencyReport) error {
	// 分批获取评论数据
	offset := 0
	for {
		var comments []model.Comment
		if err := ecs.db.Offset(offset).Limit(ecs.batchSize).Find(&comments).Error; err != nil {
			return fmt.Errorf("failed to fetch comments: %w", err)
		}

		if len(comments) == 0 {
			break
		}

		for _, comment := range comments {
			report.TotalChecked++

			// 获取缓存中的点赞数
			cacheCount, err := ecs.cacheManager.GetCommentLikeCount(ecs.ctx, comment.CommentId)
			if err != nil {
				hlog.Warnf("Failed to get cache count for comment %d: %v", comment.CommentId, err)
				continue
			}

			// 比较数据库和缓存的值
			if cacheCount != comment.LikeCount {
				record := InconsistencyRecord{
					ResourceType:  "comment",
					ResourceID:    comment.CommentId,
					CacheValue:    cacheCount,
					DatabaseValue: comment.LikeCount,
					Difference:    cacheCount - comment.LikeCount,
					DetectedAt:    time.Now(),
					Status:        "detected",
				}

				report.Details = append(report.Details, record)

				// 尝试修复不一致
				if err := ecs.fixCommentLikeInconsistency(comment.CommentId, cacheCount, comment.LikeCount); err != nil {
					hlog.Errorf("Failed to fix comment %d inconsistency: %v", comment.CommentId, err)
					record.Status = "failed"
					report.FailedCount++
				} else {
					record.Status = "fixed"
					now := time.Now()
					record.FixedAt = &now
					report.FixedCount++
				}

				// 记录不一致
				ecs.recordInconsistency(record)
			}
		}

		offset += ecs.batchSize
	}

	return nil
}

// fixVideoLikeInconsistency 修复视频点赞数不一致
func (ecs *EnhancedConsistencyService) fixVideoLikeInconsistency(videoID, cacheCount, dbCount int64) error {
	// 策略：以缓存为准更新数据库（因为缓存是实时更新的）
	if err := ecs.db.Model(&model.Video{}).
		Where("video_id = ?", videoID).
		Update("like_count", cacheCount).Error; err != nil {
		return fmt.Errorf("failed to update video like count: %w", err)
	}

	hlog.Infof("Fixed video %d like count: %d -> %d", videoID, dbCount, cacheCount)
	return nil
}

// fixCommentLikeInconsistency 修复评论点赞数不一致
func (ecs *EnhancedConsistencyService) fixCommentLikeInconsistency(commentID, cacheCount, dbCount int64) error {
	// 策略：以缓存为准更新数据库（因为缓存是实时更新的）
	if err := ecs.db.Model(&model.Comment{}).
		Where("comment_id = ?", commentID).
		Update("like_count", cacheCount).Error; err != nil {
		return fmt.Errorf("failed to update comment like count: %w", err)
	}

	hlog.Infof("Fixed comment %d like count: %d -> %d", commentID, dbCount, cacheCount)
	return nil
}

// recordInconsistency 记录不一致情况
func (ecs *EnhancedConsistencyService) recordInconsistency(record InconsistencyRecord) {
	ecs.mu.Lock()
	defer ecs.mu.Unlock()

	ecs.inconsistencyLog = append(ecs.inconsistencyLog, record)

	// 保持日志大小在合理范围内
	if len(ecs.inconsistencyLog) > 1000 {
		ecs.inconsistencyLog = ecs.inconsistencyLog[len(ecs.inconsistencyLog)-500:]
	}
}

// logConsistencyReport 记录一致性报告
func (ecs *EnhancedConsistencyService) logConsistencyReport(report *ConsistencyReport) {
	hlog.Infof("=== Consistency Report ===")
	hlog.Infof("Check Time: %s", report.CheckTime.Format("2006-01-02 15:04:05"))
	hlog.Infof("Total Checked: %d", report.TotalChecked)
	hlog.Infof("Inconsistent Count: %d", report.InconsistentCount)
	hlog.Infof("Fixed Count: %d", report.FixedCount)
	hlog.Infof("Failed Count: %d", report.FailedCount)
	hlog.Infof("Inconsistency Rate: %.2f%%", report.InconsistencyRate*100)

	if len(report.Details) > 0 {
		hlog.Infof("=== Inconsistency Details ===")
		for _, detail := range report.Details {
			hlog.Infof("%s %d: cache=%d, db=%d, diff=%d, status=%s",
				detail.ResourceType, detail.ResourceID,
				detail.CacheValue, detail.DatabaseValue,
				detail.Difference, detail.Status)
		}
	}
}

// GetConsistencyReport 获取一致性报告
func (ecs *EnhancedConsistencyService) GetConsistencyReport() []InconsistencyRecord {
	ecs.mu.RLock()
	defer ecs.mu.RUnlock()

	// 返回副本
	result := make([]InconsistencyRecord, len(ecs.inconsistencyLog))
	copy(result, ecs.inconsistencyLog)
	return result
}

// HealthCheck 健康检查
func (ecs *EnhancedConsistencyService) HealthCheck() map[string]interface{} {
	ecs.mu.RLock()
	defer ecs.mu.RUnlock()

	recentInconsistencies := 0
	for _, record := range ecs.inconsistencyLog {
		if time.Since(record.DetectedAt) < time.Hour {
			recentInconsistencies++
		}
	}

	return map[string]interface{}{
		"service_status":         "running",
		"check_interval":         ecs.checkInterval.String(),
		"total_inconsistencies":  len(ecs.inconsistencyLog),
		"recent_inconsistencies": recentInconsistencies,
		"last_check":             time.Now().Format("2006-01-02 15:04:05"),
	}
}
