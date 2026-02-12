package dal

import (
	"context"
	"fmt"
	"sync"

	"HuaTug.com/config"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	DB   *gorm.DB
	once sync.Once
)

// FavoritesVideos 收藏视频关联表 (只定义查询需要的字段)
type FavoritesVideos struct {
	ID         int64 `gorm:"primaryKey"`
	UserId     int64 `gorm:"column:user_id"`
	VideoId    int64 `gorm:"column:video_id"`
	FavoriteId int64 `gorm:"column:favorite_id"`
}

func (FavoritesVideos) TableName() string {
	return "favorites_videos"
}

// InitDB 初始化数据库连接
func InitDB() {
	once.Do(func() {
		var err error
		dsn := config.ConfigInfo.Mysql.Username + ":" + config.ConfigInfo.Mysql.Password + "@tcp(" + config.ConfigInfo.Mysql.Addr + ")/" + config.ConfigInfo.Mysql.Database + "?charset=utf8mb4&parseTime=True&loc=Local"

		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err != nil {
			hlog.Fatalf("Failed to connect to database: %v", err)
		}

		sqlDB, err := DB.DB()
		if err != nil {
			hlog.Fatalf("Failed to get underlying sql.DB: %v", err)
		}

		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetMaxIdleConns(10)

		hlog.Info("API Database initialized successfully")
	})
}

// BatchCheckUserFavorites 批量检查用户是否收藏了视频
func BatchCheckUserFavorites(ctx context.Context, userId int64, videoIds []int64) (map[int64]bool, error) {
	result := make(map[int64]bool)

	if DB == nil {
		return result, fmt.Errorf("database not initialized")
	}

	// 初始化所有 videoId 为 false
	for _, vid := range videoIds {
		result[vid] = false
	}

	if len(videoIds) == 0 {
		return result, nil
	}

	hlog.Infof("BatchCheckUserFavorites: userId=%d, videoIds=%v", userId, videoIds)

	// 查询用户收藏的视频
	var favoriteRecords []FavoritesVideos
	if err := DB.WithContext(ctx).
		Where("user_id = ? AND video_id IN ?", userId, videoIds).
		Find(&favoriteRecords).Error; err != nil {
		return result, fmt.Errorf("failed to batch check user favorites: %w", err)
	}

	hlog.Infof("BatchCheckUserFavorites: found %d records", len(favoriteRecords))
	for _, record := range favoriteRecords {
		hlog.Infof("BatchCheckUserFavorites: record user_id=%d, video_id=%d, favorite_id=%d", 
			record.UserId, record.VideoId, record.FavoriteId)
	}

	// 标记已收藏的视频
	for _, record := range favoriteRecords {
		result[record.VideoId] = true
	}

	return result, nil
}
