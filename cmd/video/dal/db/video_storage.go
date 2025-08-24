package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// VideoStorageMapping 视频存储映射模型
type VideoStorageMapping struct {
	ID                int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID            int64      `json:"user_id" gorm:"not null;index:idx_user_video"`
	VideoID           int64      `json:"video_id" gorm:"not null;uniqueIndex:uk_video_id;index:idx_user_video"`
	SourcePath        string     `json:"source_path" gorm:"not null;size:512"`
	ProcessedPaths    JSON       `json:"processed_paths" gorm:"type:json"`
	ThumbnailPaths    JSON       `json:"thumbnail_paths" gorm:"type:json"`
	AnimatedCoverPath string     `json:"animated_cover_path" gorm:"size:512"`
	MetadataPath      string     `json:"metadata_path" gorm:"size:512"`
	StorageStatus     string     `json:"storage_status" gorm:"default:uploading;index:idx_storage_status"`
	HotStorage        bool       `json:"hot_storage" gorm:"default:false;index:idx_hot_storage"`
	BucketName        string     `json:"bucket_name" gorm:"default:tiktok-user-content;size:128"`
	AccessCount       int64      `json:"access_count" gorm:"default:0"`
	LastAccessedAt    *time.Time `json:"last_accessed_at" gorm:"index:idx_last_accessed"`
	PlayCount         int64      `json:"play_count" gorm:"default:0"`
	DownloadCount     int64      `json:"download_count" gorm:"default:0"`
	FileSize          int64      `json:"file_size"`
	Duration          int        `json:"duration"`
	ResolutionWidth   int        `json:"resolution_width"`
	ResolutionHeight  int        `json:"resolution_height"`
	Format            string     `json:"format" gorm:"default:mp4;size:16"`
	Codec             string     `json:"codec" gorm:"size:32"`
	Bitrate           int        `json:"bitrate"`
	CreatedAt         time.Time  `json:"created_at" gorm:"autoCreateTime;index:idx_created_at"`
	UpdatedAt         time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// JSON 自定义JSON类型
type JSON map[string]interface{}

// Value 实现 driver.Valuer 接口
func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现 sql.Scanner 接口
func (j *JSON) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSON)
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("cannot scan into JSON")
	}

	return json.Unmarshal(bytes, j)
}

// TableName 表名
func (VideoStorageMapping) TableName() string {
	return "video_storage_mapping"
}

// UserStorageQuota 用户存储配额模型
type UserStorageQuota struct {
	ID                 int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID             int64      `json:"user_id" gorm:"not null;uniqueIndex"`
	MaxStorageBytes    int64      `json:"max_storage_bytes" gorm:"default:10737418240"` // 10GB
	MaxVideoCount      int        `json:"max_video_count" gorm:"default:1000"`
	MaxVideoDuration   int        `json:"max_video_duration" gorm:"default:600"`    // 10分钟
	MaxVideoSize       int64      `json:"max_video_size" gorm:"default:1073741824"` // 1GB
	UsedStorageBytes   int64      `json:"used_storage_bytes" gorm:"default:0"`
	VideoCount         int        `json:"video_count" gorm:"default:0"`
	DraftCount         int        `json:"draft_count" gorm:"default:0"`
	QuotaExceeded      bool       `json:"quota_exceeded" gorm:"default:false;index:idx_quota_exceeded"`
	WarningSent        bool       `json:"warning_sent" gorm:"default:false"`
	QuotaLevel         string     `json:"quota_level" gorm:"default:basic;index:idx_quota_level"`
	TotalUploadBytes   int64      `json:"total_upload_bytes" gorm:"default:0"`
	TotalDownloadBytes int64      `json:"total_download_bytes" gorm:"default:0"`
	LastUploadAt       *time.Time `json:"last_upload_at" gorm:"index:idx_last_upload"`
	CreatedAt          time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (UserStorageQuota) TableName() string {
	return "user_storage_quota"
}

// VideoAccessLog 视频访问日志模型
type VideoAccessLog struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	VideoID        int64     `json:"video_id" gorm:"not null;index:idx_video_id"`
	UserID         *int64    `json:"user_id" gorm:"index:idx_user_id"`
	AccessType     string    `json:"access_type" gorm:"not null;index:idx_access_type"`
	IPAddress      string    `json:"ip_address" gorm:"size:45"`
	UserAgent      string    `json:"user_agent" gorm:"size:512"`
	DeviceType     string    `json:"device_type" gorm:"default:unknown;index:idx_device_type"`
	Quality        string    `json:"quality" gorm:"size:16"`
	DurationPlayed int       `json:"duration_played" gorm:"default:0"`
	CompletionRate float64   `json:"completion_rate" gorm:"type:decimal(5,2);default:0.00"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime;index:idx_created_at"`
}

func (VideoAccessLog) TableName() string {
	return "video_access_log"
}

// CreateVideoStorageMapping 创建视频存储映射
func CreateVideoStorageMapping(ctx context.Context, mapping *VideoStorageMapping) error {
	err := DB.WithContext(ctx).Create(mapping).Error
	if err != nil {
		hlog.Errorf("Failed to create video storage mapping: %v", err)
		return err
	}
	return nil
}

// GetVideoStorageMapping 根据video_id获取存储映射
func GetVideoStorageMapping(ctx context.Context, videoID int64) (*VideoStorageMapping, error) {
	var mapping VideoStorageMapping
	err := DB.WithContext(ctx).Where("video_id = ?", videoID).First(&mapping).Error
	if err != nil {
		hlog.Errorf("Failed to get video storage mapping for video %d: %v", videoID, err)
		return nil, err
	}
	return &mapping, nil
}

// GetVideoStorageMappingByUserAndVideo 根据用户ID和视频ID获取存储映射
func GetVideoStorageMappingByUserAndVideo(ctx context.Context, userID, videoID int64) (*VideoStorageMapping, error) {
	var mapping VideoStorageMapping
	err := DB.WithContext(ctx).Where("user_id = ? AND video_id = ?", userID, videoID).First(&mapping).Error
	if err != nil {
		hlog.Errorf("Failed to get video storage mapping for user %d video %d: %v", userID, videoID, err)
		return nil, err
	}
	return &mapping, nil
}

// UpdateVideoStorageMapping 更新视频存储映射
func UpdateVideoStorageMapping(ctx context.Context, mapping *VideoStorageMapping) error {
	err := DB.WithContext(ctx).Save(mapping).Error
	if err != nil {
		hlog.Errorf("Failed to update video storage mapping: %v", err)
		return err
	}
	return nil
}

// UpdateVideoAccessStats 更新视频访问统计
func UpdateVideoAccessStats(ctx context.Context, videoID int64, accessType string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"access_count":     DB.Raw("access_count + 1"),
		"last_accessed_at": now,
	}

	switch accessType {
	case "view", "play":
		updates["play_count"] = DB.Raw("play_count + 1")
	case "download":
		updates["download_count"] = DB.Raw("download_count + 1")
	}

	err := DB.WithContext(ctx).Model(&VideoStorageMapping{}).
		Where("video_id = ?", videoID).
		Updates(updates).Error

	if err != nil {
		hlog.Errorf("Failed to update video access stats for video %d: %v", videoID, err)
		return err
	}
	return nil
}

// GetUserVideos 获取用户的所有视频存储映射
func GetUserVideos(ctx context.Context, userID int64, limit, offset int) ([]*VideoStorageMapping, error) {
	var mappings []*VideoStorageMapping
	err := DB.WithContext(ctx).
		Where("user_id = ? AND storage_status = ?", userID, "completed").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&mappings).Error

	if err != nil {
		hlog.Errorf("Failed to get user videos for user %d: %v", userID, err)
		return nil, err
	}
	return mappings, nil
}

// GetHotVideos 获取热门视频（根据访问量）
func GetHotVideos(ctx context.Context, limit int) ([]*VideoStorageMapping, error) {
	var mappings []*VideoStorageMapping
	err := DB.WithContext(ctx).
		Where("storage_status = ? AND hot_storage = ?", "completed", true).
		Order("access_count DESC, play_count DESC").
		Limit(limit).
		Find(&mappings).Error

	if err != nil {
		hlog.Errorf("Failed to get hot videos: %v", err)
		return nil, err
	}
	return mappings, nil
}

// PromoteVideoToHotStorage 将视频提升到热点存储
func PromoteVideoToHotStorage(ctx context.Context, videoID int64) error {
	err := DB.WithContext(ctx).Model(&VideoStorageMapping{}).
		Where("video_id = ?", videoID).
		Update("hot_storage", true).Error

	if err != nil {
		hlog.Errorf("Failed to promote video %d to hot storage: %v", videoID, err)
		return err
	}
	return nil
}

// GetUserStorageQuota 获取用户存储配额
func GetUserStorageQuota(ctx context.Context, userID int64) (*UserStorageQuota, error) {
	var quota UserStorageQuota
	err := DB.WithContext(ctx).Where("user_id = ?", userID).First(&quota).Error
	if err != nil {
		if err.Error() == "record not found" {
			// 如果没有配额记录，创建默认配额
			defaultQuota := &UserStorageQuota{
				UserID:           userID,
				MaxStorageBytes:  10737418240, // 10GB
				MaxVideoCount:    1000,
				MaxVideoDuration: 600,        // 10分钟
				MaxVideoSize:     1073741824, // 1GB
				UsedStorageBytes: 0,
				VideoCount:       0,
				DraftCount:       0,
				QuotaExceeded:    false,
				WarningSent:      false,

				QuotaLevel:         "basic",
				TotalUploadBytes:   0,
				TotalDownloadBytes: 0,
				LastUploadAt:       nil,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}

			createErr := DB.WithContext(ctx).Create(defaultQuota).Error
			if createErr != nil {
				hlog.Errorf("Failed to create default user storage quota for user %d: %v", userID, createErr)
				return nil, createErr
			}

			hlog.Infof("Created default storage quota for user %d", userID)
			return defaultQuota, nil
		}
		hlog.Errorf("Failed to get user storage quota for user %d: %v", userID, err)
		return nil, err
	}
	return &quota, nil
}

// CreateUserStorageQuota 创建用户存储配额
func CreateUserStorageQuota(ctx context.Context, quota *UserStorageQuota) error {
	err := DB.WithContext(ctx).Create(quota).Error
	if err != nil {
		hlog.Errorf("Failed to create user storage quota: %v", err)
		return err
	}
	return nil
}

// UpdateUserStorageQuota 更新用户存储配额
func UpdateUserStorageQuota(ctx context.Context, quota *UserStorageQuota) error {
	err := DB.WithContext(ctx).Save(quota).Error
	if err != nil {
		hlog.Errorf("Failed to update user storage quota: %v", err)
		return err
	}
	return nil
}

// UpdateUserStorageUsage 更新用户存储使用量
func UpdateUserStorageUsage(ctx context.Context, userID int64, sizeBytes int64, videoCount int) error {
	err := DB.WithContext(ctx).Model(&UserStorageQuota{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"used_storage_bytes": DB.Raw("used_storage_bytes + ?", sizeBytes),
			"video_count":        DB.Raw("video_count + ?", videoCount),
			"last_upload_at":     time.Now(),
		}).Error

	if err != nil {
		hlog.Errorf("Failed to update user storage usage for user %d: %v", userID, err)
		return err
	}
	return nil
}

// LogVideoAccess 记录视频访问日志
func LogVideoAccess(ctx context.Context, log *VideoAccessLog) error {
	err := DB.WithContext(ctx).Create(log).Error
	if err != nil {
		hlog.Errorf("Failed to log video access: %v", err)
		return err
	}
	return nil
}

// GetVideoAccessStats 获取视频访问统计（24小时内）
func GetVideoAccessStats(ctx context.Context, videoID int64) (map[string]int64, error) {
	var results []struct {
		AccessType string `json:"access_type"`
		Count      int64  `json:"count"`
	}

	since := time.Now().Add(-24 * time.Hour)
	err := DB.WithContext(ctx).Model(&VideoAccessLog{}).
		Select("access_type, COUNT(*) as count").
		Where("video_id = ? AND created_at > ?", videoID, since).
		Group("access_type").
		Find(&results).Error

	if err != nil {
		hlog.Errorf("Failed to get video access stats for video %d: %v", videoID, err)
		return nil, err
	}

	stats := make(map[string]int64)
	for _, result := range results {
		stats[result.AccessType] = result.Count
	}

	return stats, nil
}

// GetVideosNeedingHotPromotion 获取需要提升到热点存储的视频
func GetVideosNeedingHotPromotion(ctx context.Context, minAccessCount int64) ([]*VideoStorageMapping, error) {
	var mappings []*VideoStorageMapping
	since := time.Now().Add(-24 * time.Hour)

	err := DB.WithContext(ctx).
		Where("storage_status = ? AND hot_storage = ? AND access_count >= ? AND last_accessed_at > ?",
			"completed", false, minAccessCount, since).
		Order("access_count DESC").
		Limit(100). // 限制一次处理的数量
		Find(&mappings).Error

	if err != nil {
		hlog.Errorf("Failed to get videos needing hot promotion: %v", err)
		return nil, err
	}
	return mappings, nil
}

// CleanupOldAccessLogs 清理旧的访问日志（保留最近30天）
func CleanupOldAccessLogs(ctx context.Context) error {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	err := DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&VideoAccessLog{}).Error
	if err != nil {
		hlog.Errorf("Failed to cleanup old access logs: %v", err)
		return err
	}
	return nil
}
