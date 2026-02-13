package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/internal/model"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/gorm"
)

// Consistency service configuration constants.
const (
	defaultConsistencyInterval = 5 * time.Minute
	defaultConsistencyBatch    = 100
	defaultConsistencyRetries  = 3
	maxInconsistencyLogSize    = 1000
	inconsistencyLogTrimSize   = 500
)

// EnhancedConsistencyService periodically checks and fixes data consistency
// between Redis cache and the database.
type EnhancedConsistencyService struct {
	db           *gorm.DB
	cacheManager *redis.LikeCacheManager
	producer     mq.MessageProducer
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex

	checkInterval    time.Duration
	batchSize        int
	maxRetries       int
	inconsistencyLog []InconsistencyRecord
}

// InconsistencyRecord records a detected cache-DB inconsistency.
type InconsistencyRecord struct {
	ResourceType  string     `json:"resource_type"` // "video" or "comment"
	ResourceID    int64      `json:"resource_id"`
	CacheValue    int64      `json:"cache_value"`
	DatabaseValue int64      `json:"database_value"`
	Difference    int64      `json:"difference"`
	DetectedAt    time.Time  `json:"detected_at"`
	FixedAt       *time.Time `json:"fixed_at,omitempty"`
	Status        string     `json:"status"` // detected / fixing / fixed / failed
}

// ConsistencyReport summarizes a single consistency check run.
type ConsistencyReport struct {
	CheckTime         time.Time             `json:"check_time"`
	TotalChecked      int                   `json:"total_checked"`
	InconsistentCount int                   `json:"inconsistent_count"`
	FixedCount        int                   `json:"fixed_count"`
	FailedCount       int                   `json:"failed_count"`
	InconsistencyRate float64               `json:"inconsistency_rate"`
	Details           []InconsistencyRecord `json:"details"`
}

// NewEnhancedConsistencyService creates a new consistency service.
func NewEnhancedConsistencyService(db *gorm.DB, cacheManager *redis.LikeCacheManager, producer mq.MessageProducer) *EnhancedConsistencyService {
	ctx, cancel := context.WithCancel(context.Background())
	return &EnhancedConsistencyService{
		db:               db,
		cacheManager:     cacheManager,
		producer:         producer,
		ctx:              ctx,
		cancel:           cancel,
		checkInterval:    defaultConsistencyInterval,
		batchSize:        defaultConsistencyBatch,
		maxRetries:       defaultConsistencyRetries,
		inconsistencyLog: make([]InconsistencyRecord, 0),
	}
}

// Start launches the periodic consistency checker.
func (s *EnhancedConsistencyService) Start() {
	hlog.Info("[ConsistencyService] Started")

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			hlog.Info("[ConsistencyService] Stopped")
			return
		case <-ticker.C:
			if err := s.runCheck(); err != nil {
				hlog.Errorf("[ConsistencyService] Check failed: %v", err)
			}
		}
	}
}

// Stop stops the consistency service.
func (s *EnhancedConsistencyService) Stop() {
	s.cancel()
}

// runCheck performs a full consistency check across videos and comments.
func (s *EnhancedConsistencyService) runCheck() error {
	hlog.Info("[ConsistencyService] Starting check...")

	report := &ConsistencyReport{
		CheckTime: time.Now(),
		Details:   make([]InconsistencyRecord, 0),
	}

	if err := s.checkVideoLikes(report); err != nil {
		hlog.Errorf("[ConsistencyService] Video like check failed: %v", err)
	}
	if err := s.checkCommentLikes(report); err != nil {
		hlog.Errorf("[ConsistencyService] Comment like check failed: %v", err)
	}

	report.InconsistentCount = len(report.Details)
	if report.TotalChecked > 0 {
		report.InconsistencyRate = float64(report.InconsistentCount) / float64(report.TotalChecked)
	}

	s.logReport(report)

	hlog.Infof("[ConsistencyService] Check completed: checked=%d, inconsistent=%d, rate=%.2f%%",
		report.TotalChecked, report.InconsistentCount, report.InconsistencyRate*100)

	return nil
}

// checkVideoLikes checks like-count consistency for videos in batches.
// The authoritative source is the DB; the Redis cache is the secondary store.
func (s *EnhancedConsistencyService) checkVideoLikes(report *ConsistencyReport) error {
	offset := 0
	for {
		var videos []model.Video
		if err := s.db.Offset(offset).Limit(s.batchSize).Find(&videos).Error; err != nil {
			return fmt.Errorf("failed to fetch videos: %w", err)
		}
		if len(videos) == 0 {
			break
		}

		for _, video := range videos {
			report.TotalChecked++

			// Read the authoritative count from the video_likes relation table
			var dbCount int64
			if err := s.db.Model(&model.VideoLike{}).Where("video_id = ? AND deleted_at IS NULL", video.VideoId).Count(&dbCount).Error; err != nil {
				hlog.Warnf("Failed to get DB like count for video %d: %v", video.VideoId, err)
				continue
			}

			// Read the Redis cache count (from the enhanced Hash key)
			cacheCount, err := s.cacheManager.GetVideoLikeCount(s.ctx, video.VideoId)
			if err != nil {
				hlog.Warnf("Failed to get cache count for video %d: %v", video.VideoId, err)
				continue
			}

			if cacheCount == dbCount {
				continue
			}

			record := InconsistencyRecord{
				ResourceType:  "video",
				ResourceID:    video.VideoId,
				CacheValue:    cacheCount,
				DatabaseValue: dbCount,
				Difference:    cacheCount - dbCount,
				DetectedAt:    time.Now(),
				Status:        "detected",
			}

			// Fix direction: DB -> Redis (DB is authority)
			if err := s.fixVideoLike(video.VideoId, dbCount); err != nil {
				hlog.Errorf("Failed to fix video %d: %v", video.VideoId, err)
				record.Status = "failed"
				report.FailedCount++
			} else {
				now := time.Now()
				record.Status = "fixed"
				record.FixedAt = &now
				report.FixedCount++
			}

			report.Details = append(report.Details, record)
			s.recordInconsistency(record)
		}

		offset += s.batchSize
	}

	return nil
}

// checkCommentLikes checks like-count consistency for comments in batches.
// The authoritative source is the DB; the Redis cache is the secondary store.
func (s *EnhancedConsistencyService) checkCommentLikes(report *ConsistencyReport) error {
	offset := 0
	for {
		var comments []model.Comment
		if err := s.db.Offset(offset).Limit(s.batchSize).Find(&comments).Error; err != nil {
			return fmt.Errorf("failed to fetch comments: %w", err)
		}
		if len(comments) == 0 {
			break
		}

		for _, comment := range comments {
			report.TotalChecked++

			// Read the authoritative count from comment_likes table
			var dbCount int64
			if err := s.db.Model(&model.CommentLike{}).Where("comment_id = ? AND deleted_at IS NULL", comment.CommentId).Count(&dbCount).Error; err != nil {
				hlog.Warnf("Failed to get DB like count for comment %d: %v", comment.CommentId, err)
				continue
			}

			cacheCount, err := s.cacheManager.GetCommentLikeCount(s.ctx, comment.CommentId)
			if err != nil {
				hlog.Warnf("Failed to get cache count for comment %d: %v", comment.CommentId, err)
				continue
			}

			if cacheCount == dbCount {
				continue
			}

			record := InconsistencyRecord{
				ResourceType:  "comment",
				ResourceID:    comment.CommentId,
				CacheValue:    cacheCount,
				DatabaseValue: dbCount,
				Difference:    cacheCount - dbCount,
				DetectedAt:    time.Now(),
				Status:        "detected",
			}

			// Fix direction: DB -> Redis (DB is authority)
			if err := s.fixCommentLike(comment.CommentId, dbCount); err != nil {
				hlog.Errorf("Failed to fix comment %d: %v", comment.CommentId, err)
				record.Status = "failed"
				report.FailedCount++
			} else {
				now := time.Now()
				record.Status = "fixed"
				record.FixedAt = &now
				report.FixedCount++
			}

			report.Details = append(report.Details, record)
			s.recordInconsistency(record)
		}

		offset += s.batchSize
	}

	return nil
}

// fixVideoLike updates the Redis cache to match the authoritative DB count.
// Consistency direction: DB -> Redis (DB is the source of truth).
func (s *EnhancedConsistencyService) fixVideoLike(videoID, dbCount int64) error {
	// Read fresh count from the video_likes table (not the denormalized video.likes_count)
	// to ensure we use the most accurate value.
	var freshCount int64
	err := s.db.Model(&model.VideoLike{}).Where("video_id = ? AND deleted_at IS NULL", videoID).Count(&freshCount).Error
	if err != nil {
		return fmt.Errorf("failed to get fresh video like count: %w", err)
	}

	// Update the Redis Hash counter from DB
	key := fmt.Sprintf("like:count:%d", redis.BizTypeVideo)
	if err := redis.RedisDBInteraction.HSet(key, fmt.Sprintf("%d", videoID), fmt.Sprintf("%d", freshCount)).Err(); err != nil {
		return fmt.Errorf("failed to fix Redis cache for video %d: %w", videoID, err)
	}

	// Also sync the denormalized count in the video table
	if err := s.db.Model(&model.Video{}).Where("video_id = ?", videoID).Update("likes_count", freshCount).Error; err != nil {
		hlog.Warnf("[ConsistencyService] Failed to sync denormalized likes_count for video %d: %v", videoID, err)
	}

	hlog.Infof("[ConsistencyService] Fixed video %d: set Redis count to %d (from DB)", videoID, freshCount)
	return nil
}

// fixCommentLike updates the Redis cache to match the authoritative DB count.
// Consistency direction: DB -> Redis (DB is the source of truth).
func (s *EnhancedConsistencyService) fixCommentLike(commentID, dbCount int64) error {
	// Read fresh count from the comment_likes table
	var freshCount int64
	err := s.db.Model(&model.CommentLike{}).Where("comment_id = ? AND deleted_at IS NULL", commentID).Count(&freshCount).Error
	if err != nil {
		return fmt.Errorf("failed to get fresh comment like count: %w", err)
	}

	// Update the Redis Hash counter from DB
	key := fmt.Sprintf("like:count:%d", redis.BizTypeComment)
	if err := redis.RedisDBInteraction.HSet(key, fmt.Sprintf("%d", commentID), fmt.Sprintf("%d", freshCount)).Err(); err != nil {
		return fmt.Errorf("failed to fix Redis cache for comment %d: %w", commentID, err)
	}

	// Also sync the denormalized count in the comment table
	if err := s.db.Model(&model.Comment{}).Where("comment_id = ?", commentID).Update("like_count", freshCount).Error; err != nil {
		hlog.Warnf("[ConsistencyService] Failed to sync denormalized like_count for comment %d: %v", commentID, err)
	}

	hlog.Infof("[ConsistencyService] Fixed comment %d: set Redis count to %d (from DB)", commentID, freshCount)
	return nil
}

// recordInconsistency appends a record to the in-memory log, trimming if necessary.
func (s *EnhancedConsistencyService) recordInconsistency(record InconsistencyRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inconsistencyLog = append(s.inconsistencyLog, record)
	if len(s.inconsistencyLog) > maxInconsistencyLogSize {
		s.inconsistencyLog = s.inconsistencyLog[len(s.inconsistencyLog)-inconsistencyLogTrimSize:]
	}
}

// logReport logs a consistency report summary.
func (s *EnhancedConsistencyService) logReport(report *ConsistencyReport) {
	hlog.Infof("[ConsistencyReport] Time=%s Checked=%d Inconsistent=%d Fixed=%d Failed=%d Rate=%.2f%%",
		report.CheckTime.Format("2006-01-02 15:04:05"),
		report.TotalChecked, report.InconsistentCount,
		report.FixedCount, report.FailedCount,
		report.InconsistencyRate*100)

	for _, d := range report.Details {
		hlog.Infof("[ConsistencyReport] %s %d: cache=%d db=%d diff=%d status=%s",
			d.ResourceType, d.ResourceID, d.CacheValue, d.DatabaseValue, d.Difference, d.Status)
	}
}

// GetConsistencyReport returns a copy of the inconsistency log.
func (s *EnhancedConsistencyService) GetConsistencyReport() []InconsistencyRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]InconsistencyRecord, len(s.inconsistencyLog))
	copy(result, s.inconsistencyLog)
	return result
}

// HealthCheck returns the service health status.
func (s *EnhancedConsistencyService) HealthCheck() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recentCount := 0
	for _, r := range s.inconsistencyLog {
		if time.Since(r.DetectedAt) < time.Hour {
			recentCount++
		}
	}

	return map[string]interface{}{
		"service_status":         "running",
		"check_interval":         s.checkInterval.String(),
		"total_inconsistencies":  len(s.inconsistencyLog),
		"recent_inconsistencies": recentCount,
		"last_check":             time.Now().Format("2006-01-02 15:04:05"),
	}
}
