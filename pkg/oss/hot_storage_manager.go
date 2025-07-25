package oss

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/minio/minio-go/v7"
)

// HotStorageManager 热度存储管理器
type HotStorageManager struct {
	tikTokStorage *TikTokStorage
}

// NewHotStorageManager 创建热度存储管理器
func NewHotStorageManager() *HotStorageManager {
	return &HotStorageManager{
		tikTokStorage: NewTikTokStorage(),
	}
}

// HotPromotionConfig 热度提升配置
type HotPromotionConfig struct {
	MinAccessCount     int64         // 最小访问次数阈值
	MinPlayCount       int64         // 最小播放次数阈值
	CheckInterval      time.Duration // 检查间隔
	PromotionBatchSize int           // 每次处理的视频数量
}

// DefaultHotPromotionConfig 默认热度提升配置
var DefaultHotPromotionConfig = &HotPromotionConfig{
	MinAccessCount:     1000,      // 访问次数超过1000
	MinPlayCount:       500,       // 播放次数超过500
	CheckInterval:      time.Hour, // 每小时检查一次
	PromotionBatchSize: 50,        // 每次处理50个视频
}

// StartHotStorageManager 启动热度存储管理器
func (hsm *HotStorageManager) StartHotStorageManager(ctx context.Context, config *HotPromotionConfig) {
	if config == nil {
		config = DefaultHotPromotionConfig
	}

	hlog.Infof("Starting hot storage manager with config: %+v", config)

	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			hlog.Info("Hot storage manager stopped")
			return
		case <-ticker.C:
			hsm.processHotPromotion(ctx, config)
		}
	}
}

// processHotPromotion 处理热度提升
func (hsm *HotStorageManager) processHotPromotion(ctx context.Context, config *HotPromotionConfig) {
	hlog.Info("Starting hot promotion process")

	// 获取需要提升的视频
	candidates, err := db.GetVideosNeedingHotPromotion(ctx, config.MinAccessCount)
	if err != nil {
		hlog.Errorf("Failed to get videos needing hot promotion: %v", err)
		return
	}

	if len(candidates) == 0 {
		hlog.Info("No videos need hot promotion")
		return
	}

	hlog.Infof("Found %d videos candidates for hot promotion", len(candidates))

	promoted := 0
	for i, video := range candidates {
		if i >= config.PromotionBatchSize {
			break
		}

		// 检查播放次数
		if video.PlayCount < config.MinPlayCount {
			continue
		}

		// 提升到热点存储
		if err := hsm.PromoteVideoToHot(ctx, video.UserID, video.VideoID); err != nil {
			hlog.Errorf("Failed to promote video %d to hot storage: %v", video.VideoID, err)
			continue
		}

		promoted++
		hlog.Infof("Promoted video %d (user %d) to hot storage", video.VideoID, video.UserID)
	}

	hlog.Infof("Hot promotion process completed: promoted %d videos", promoted)
}

// PromoteVideoToHot 将视频提升到热点存储
func (hsm *HotStorageManager) PromoteVideoToHot(ctx context.Context, userID, videoID int64) error {
	// 1. 获取视频存储映射
	storageMapping, err := db.GetVideoStorageMapping(ctx, videoID)
	if err != nil {
		return fmt.Errorf("failed to get storage mapping: %w", err)
	}

	if storageMapping.HotStorage {
		hlog.Infof("Video %d is already in hot storage", videoID)
		return nil
	}

	// 2. 复制视频到热点存储桶
	if err := hsm.tikTokStorage.PromoteToHotStorage(ctx, userID, videoID); err != nil {
		return fmt.Errorf("failed to copy to hot storage: %w", err)
	}

	// 3. 更新数据库标记
	if err := db.PromoteVideoToHotStorage(ctx, videoID); err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	hlog.Infof("Successfully promoted video %d to hot storage", videoID)
	return nil
}

// DemoteVideoFromHot 从热点存储降级视频
func (hsm *HotStorageManager) DemoteVideoFromHot(ctx context.Context, videoID int64) error {
	// 1. 获取视频存储映射
	storageMapping, err := db.GetVideoStorageMapping(ctx, videoID)
	if err != nil {
		return fmt.Errorf("failed to get storage mapping: %w", err)
	}

	if !storageMapping.HotStorage {
		hlog.Infof("Video %d is not in hot storage", videoID)
		return nil
	}

	// 2. 从热点存储桶删除
	hotPath := fmt.Sprintf("hot/users/%d/videos/%d/video_720p.mp4", storageMapping.UserID, videoID)
	err = hsm.tikTokStorage.client.RemoveObject(ctx, BUCKET_CACHE_HOT, hotPath, minio.RemoveObjectOptions{})
	if err != nil {
		hlog.Warnf("Failed to remove from hot storage (may not exist): %v", err)
	}

	// 3. 更新数据库标记
	err = db.DB.WithContext(ctx).Model(&db.VideoStorageMapping{}).
		Where("video_id = ?", videoID).
		Update("hot_storage", false).Error
	if err != nil {
		return fmt.Errorf("failed to update database: %w", err)
	}

	hlog.Infof("Successfully demoted video %d from hot storage", videoID)
	return nil
}

// CleanupColdVideos 清理冷视频（从热点存储中移除访问量低的视频）
func (hsm *HotStorageManager) CleanupColdVideos(ctx context.Context) error {
	hlog.Info("Starting cold video cleanup")

	// 获取热点存储中访问量低的视频（24小时内访问次数少于100）
	since := time.Now().Add(-24 * time.Hour)
	var coldVideos []*db.VideoStorageMapping

	err := db.DB.WithContext(ctx).
		Where("hot_storage = ? AND last_accessed_at < ? AND access_count < ?",
			true, since, 100).
		Find(&coldVideos).Error
	if err != nil {
		return fmt.Errorf("failed to get cold videos: %w", err)
	}

	if len(coldVideos) == 0 {
		hlog.Info("No cold videos to cleanup")
		return nil
	}

	hlog.Infof("Found %d cold videos to cleanup", len(coldVideos))

	for _, video := range coldVideos {
		if err := hsm.DemoteVideoFromHot(ctx, video.VideoID); err != nil {
			hlog.Errorf("Failed to demote cold video %d: %v", video.VideoID, err)
			continue
		}
	}

	hlog.Infof("Cold video cleanup completed: demoted %d videos", len(coldVideos))
	return nil
}

// GetHotStorageStats 获取热点存储统计信息
func (hsm *HotStorageManager) GetHotStorageStats(ctx context.Context) (*HotStorageStats, error) {
	var stats HotStorageStats

	// 统计热点存储中的视频数量
	err := db.DB.WithContext(ctx).Model(&db.VideoStorageMapping{}).
		Where("hot_storage = ?", true).
		Count(&stats.HotVideoCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count hot videos: %w", err)
	}

	// 统计总视频数量
	err = db.DB.WithContext(ctx).Model(&db.VideoStorageMapping{}).
		Where("storage_status = ?", "completed").
		Count(&stats.TotalVideoCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count total videos: %w", err)
	}

	// 计算热点存储使用率
	if stats.TotalVideoCount > 0 {
		stats.HotStorageRatio = float64(stats.HotVideoCount) / float64(stats.TotalVideoCount) * 100
	}

	// 获取最近24小时的访问统计
	since := time.Now().Add(-24 * time.Hour)

	// 热点存储访问次数
	var hotAccessCount int64
	err = db.DB.WithContext(ctx).Model(&db.VideoAccessLog{}).
		Joins("JOIN video_storage_mapping ON video_access_log.video_id = video_storage_mapping.video_id").
		Where("video_storage_mapping.hot_storage = ? AND video_access_log.created_at > ?", true, since).
		Count(&hotAccessCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count hot access: %w", err)
	}
	stats.HotAccessCount24h = hotAccessCount

	// 总访问次数
	var totalAccessCount int64
	err = db.DB.WithContext(ctx).Model(&db.VideoAccessLog{}).
		Where("created_at > ?", since).
		Count(&totalAccessCount).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count total access: %w", err)
	}
	stats.TotalAccessCount24h = totalAccessCount

	// 计算热点存储命中率
	if totalAccessCount > 0 {
		stats.HotStorageHitRatio = float64(hotAccessCount) / float64(totalAccessCount) * 100
	}

	return &stats, nil
}

// HotStorageStats 热点存储统计信息
type HotStorageStats struct {
	HotVideoCount       int64   `json:"hot_video_count"`        // 热点存储视频数量
	TotalVideoCount     int64   `json:"total_video_count"`      // 总视频数量
	HotStorageRatio     float64 `json:"hot_storage_ratio"`      // 热点存储占比（%）
	HotAccessCount24h   int64   `json:"hot_access_count_24h"`   // 24小时内热点存储访问次数
	TotalAccessCount24h int64   `json:"total_access_count_24h"` // 24小时内总访问次数
	HotStorageHitRatio  float64 `json:"hot_storage_hit_ratio"`  // 热点存储命中率（%）
}

// StartCleanupWorker 启动清理工作者
func (hsm *HotStorageManager) StartCleanupWorker(ctx context.Context) {
	hlog.Info("Starting cleanup worker")

	// 每4小时执行一次冷视频清理
	cleanupTicker := time.NewTicker(4 * time.Hour)
	defer cleanupTicker.Stop()

	// 每天执行一次访问日志清理
	logCleanupTicker := time.NewTicker(24 * time.Hour)
	defer logCleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			hlog.Info("Cleanup worker stopped")
			return
		case <-cleanupTicker.C:
			if err := hsm.CleanupColdVideos(ctx); err != nil {
				hlog.Errorf("Cold video cleanup failed: %v", err)
			}
		case <-logCleanupTicker.C:
			if err := db.CleanupOldAccessLogs(ctx); err != nil {
				hlog.Errorf("Access log cleanup failed: %v", err)
			}
		}
	}
}
