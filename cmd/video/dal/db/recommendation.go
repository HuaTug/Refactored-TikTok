package db

import (
	"context"
	"encoding/json"
	"time"

	"HuaTug.com/cmd/model"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ========================================
// User Profile Operations (用户画像操作)
// ========================================

// GetUserProfile 获取用户画像
func GetUserProfile(ctx context.Context, userId int64) (*model.UserProfile, error) {
	var profile model.UserProfile
	err := DB.WithContext(ctx).Where("user_id = ?", userId).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get user profile")
	}
	return &profile, nil
}

// CreateOrUpdateUserProfile 创建或更新用户画像
func CreateOrUpdateUserProfile(ctx context.Context, profile *model.UserProfile) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		UpdateAll: true,
	}).Create(profile).Error
}

// UpdateUserProfileInterests 更新用户兴趣标签
func UpdateUserProfileInterests(ctx context.Context, userId int64, interestTags map[string]float64) error {
	tagsJson, err := json.Marshal(interestTags)
	if err != nil {
		return errors.WithMessage(err, "Failed to marshal interest tags")
	}

	rawMessage := json.RawMessage(tagsJson)
	return DB.WithContext(ctx).Model(&model.UserProfile{}).
		Where("user_id = ?", userId).
		Update("interest_tags", &rawMessage).Error
}

// UpdateUserProfileCategories 更新用户分类偏好
func UpdateUserProfileCategories(ctx context.Context, userId int64, categoryPreference map[string]float64) error {
	catJson, err := json.Marshal(categoryPreference)
	if err != nil {
		return errors.WithMessage(err, "Failed to marshal category preference")
	}

	rawMessage := json.RawMessage(catJson)
	return DB.WithContext(ctx).Model(&model.UserProfile{}).
		Where("user_id = ?", userId).
		Update("category_preference", &rawMessage).Error
}

// UpdateUserProfileStats 更新用户行为统计
func UpdateUserProfileStats(ctx context.Context, userId int64, stats map[string]interface{}) error {
	return DB.WithContext(ctx).Model(&model.UserProfile{}).
		Where("user_id = ?", userId).
		Updates(stats).Error
}

// IncrementUserProfileCounter 增加用户画像计数器
func IncrementUserProfileCounter(ctx context.Context, userId int64, field string, delta int64) error {
	return DB.WithContext(ctx).Model(&model.UserProfile{}).
		Where("user_id = ?", userId).
		UpdateColumn(field, gorm.Expr(field+" + ?", delta)).Error
}

// BatchGetUserProfiles 批量获取用户画像
func BatchGetUserProfiles(ctx context.Context, userIds []int64) ([]*model.UserProfile, error) {
	var profiles []*model.UserProfile
	if len(userIds) == 0 {
		return profiles, nil
	}
	err := DB.WithContext(ctx).Where("user_id IN ?", userIds).Find(&profiles).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to batch get user profiles")
	}
	return profiles, nil
}

// ========================================
// Video Feature Operations (视频特征操作)
// ========================================

// GetVideoFeature 获取视频特征
func GetVideoFeature(ctx context.Context, videoId int64) (*model.VideoFeature, error) {
	var feature model.VideoFeature
	err := DB.WithContext(ctx).Where("video_id = ?", videoId).First(&feature).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get video feature")
	}
	return &feature, nil
}

// CreateOrUpdateVideoFeature 创建或更新视频特征
func CreateOrUpdateVideoFeature(ctx context.Context, feature *model.VideoFeature) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}},
		UpdateAll: true,
	}).Create(feature).Error
}

// BatchGetVideoFeatures 批量获取视频特征
func BatchGetVideoFeatures(ctx context.Context, videoIds []int64) ([]*model.VideoFeature, error) {
	var features []*model.VideoFeature
	if len(videoIds) == 0 {
		return features, nil
	}
	err := DB.WithContext(ctx).Where("video_id IN ?", videoIds).Find(&features).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to batch get video features")
	}
	return features, nil
}

// GetHighQualityVideos 获取高质量视频列表
func GetHighQualityVideos(ctx context.Context, limit int) ([]*model.VideoFeature, error) {
	var features []*model.VideoFeature
	err := DB.WithContext(ctx).
		Where("is_high_quality = 1").
		Order("quality_score DESC").
		Limit(limit).
		Find(&features).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get high quality videos")
	}
	return features, nil
}

// GetVideosByPopularity 按热度获取视频
func GetVideosByPopularity(ctx context.Context, limit int) ([]*model.VideoFeature, error) {
	var features []*model.VideoFeature
	err := DB.WithContext(ctx).
		Order("popularity_score DESC").
		Limit(limit).
		Find(&features).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get videos by popularity")
	}
	return features, nil
}

// GetVideosByCTR 按点击率获取视频
func GetVideosByCTR(ctx context.Context, minExposure int64, limit int) ([]*model.VideoFeature, error) {
	var features []*model.VideoFeature
	err := DB.WithContext(ctx).
		Where("exposure_count >= ?", minExposure).
		Order("ctr DESC").
		Limit(limit).
		Find(&features).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get videos by CTR")
	}
	return features, nil
}

// UpdateVideoFeatureMetrics 更新视频特征指标
func UpdateVideoFeatureMetrics(ctx context.Context, videoId int64, metrics map[string]interface{}) error {
	return DB.WithContext(ctx).Model(&model.VideoFeature{}).
		Where("video_id = ?", videoId).
		Updates(metrics).Error
}

// IncrementVideoExposure 增加视频曝光次数
func IncrementVideoExposure(ctx context.Context, videoId int64) error {
	return DB.WithContext(ctx).Model(&model.VideoFeature{}).
		Where("video_id = ?", videoId).
		UpdateColumn("exposure_count", gorm.Expr("exposure_count + 1")).Error
}

// IncrementVideoClick 增加视频点击次数并更新CTR
func IncrementVideoClick(ctx context.Context, videoId int64) error {
	return DB.WithContext(ctx).Model(&model.VideoFeature{}).
		Where("video_id = ?", videoId).
		Updates(map[string]interface{}{
			"click_count": gorm.Expr("click_count + 1"),
			"ctr":         gorm.Expr("CASE WHEN exposure_count > 0 THEN (click_count + 1) / exposure_count ELSE 0 END"),
		}).Error
}

// ========================================
// Video Embedding Operations (视频向量操作)
// ========================================

// GetVideoEmbedding 获取视频向量
func GetVideoEmbedding(ctx context.Context, videoId int64, embeddingType string) (*model.VideoEmbedding, error) {
	var embedding model.VideoEmbedding
	err := DB.WithContext(ctx).
		Where("video_id = ? AND embedding_type = ?", videoId, embeddingType).
		First(&embedding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get video embedding")
	}
	return &embedding, nil
}

// SaveVideoEmbedding 保存视频向量
func SaveVideoEmbedding(ctx context.Context, embedding *model.VideoEmbedding) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}, {Name: "embedding_type"}},
		UpdateAll: true,
	}).Create(embedding).Error
}

// BatchGetVideoEmbeddings 批量获取视频向量
func BatchGetVideoEmbeddings(ctx context.Context, videoIds []int64, embeddingType string) ([]*model.VideoEmbedding, error) {
	var embeddings []*model.VideoEmbedding
	if len(videoIds) == 0 {
		return embeddings, nil
	}
	err := DB.WithContext(ctx).
		Where("video_id IN ? AND embedding_type = ?", videoIds, embeddingType).
		Find(&embeddings).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to batch get video embeddings")
	}
	return embeddings, nil
}

// ========================================
// User Embedding Operations (用户向量操作)
// ========================================

// GetUserEmbedding 获取用户向量
func GetUserEmbedding(ctx context.Context, userId int64, embeddingType string) (*model.UserEmbedding, error) {
	var embedding model.UserEmbedding
	err := DB.WithContext(ctx).
		Where("user_id = ? AND embedding_type = ?", userId, embeddingType).
		First(&embedding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get user embedding")
	}
	return &embedding, nil
}

// SaveUserEmbedding 保存用户向量
func SaveUserEmbedding(ctx context.Context, embedding *model.UserEmbedding) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "embedding_type"}},
		UpdateAll: true,
	}).Create(embedding).Error
}

// ========================================
// Video Similarity Operations (视频相似度操作)
// ========================================

// GetSimilarVideos 获取相似视频
func GetSimilarVideos(ctx context.Context, videoId int64, similarityType string, limit int) ([]*model.VideoSimilarity, error) {
	var similarities []*model.VideoSimilarity
	query := DB.WithContext(ctx).Where("video_id = ?", videoId)
	if similarityType != "" {
		query = query.Where("similarity_type = ?", similarityType)
	}
	err := query.Order("similarity_score DESC").Limit(limit).Find(&similarities).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get similar videos")
	}
	return similarities, nil
}

// SaveVideoSimilarity 保存视频相似度
func SaveVideoSimilarity(ctx context.Context, similarity *model.VideoSimilarity) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}, {Name: "similar_video_id"}, {Name: "similarity_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"similarity_score"}),
	}).Create(similarity).Error
}

// BatchSaveVideoSimilarities 批量保存视频相似度
func BatchSaveVideoSimilarities(ctx context.Context, similarities []*model.VideoSimilarity) error {
	if len(similarities) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}, {Name: "similar_video_id"}, {Name: "similarity_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"similarity_score"}),
	}).CreateInBatches(similarities, 100).Error
}

// ========================================
// Recommendation Exposure Operations (推荐曝光操作)
// ========================================

// RecordExposure 记录推荐曝光
func RecordExposure(ctx context.Context, exposure *model.RecommendationExposure) error {
	return DB.WithContext(ctx).Create(exposure).Error
}

// BatchRecordExposures 批量记录推荐曝光
func BatchRecordExposures(ctx context.Context, exposures []*model.RecommendationExposure) error {
	if len(exposures) == 0 {
		return nil
	}
	return DB.WithContext(ctx).CreateInBatches(exposures, 100).Error
}

// UpdateExposureFeedback 更新曝光反馈
func UpdateExposureFeedback(ctx context.Context, userId, videoId int64, feedback map[string]interface{}) error {
	return DB.WithContext(ctx).Model(&model.RecommendationExposure{}).
		Where("user_id = ? AND video_id = ?", userId, videoId).
		Order("exposure_time DESC").
		Limit(1).
		Updates(feedback).Error
}

// GetUserRecentExposures 获取用户最近曝光的视频ID
func GetUserRecentExposures(ctx context.Context, userId int64, hours int, limit int) ([]int64, error) {
	var videoIds []int64
	cutoffTime := time.Now().Add(-time.Duration(hours) * time.Hour)
	err := DB.WithContext(ctx).Model(&model.RecommendationExposure{}).
		Where("user_id = ? AND exposure_time >= ?", userId, cutoffTime).
		Order("exposure_time DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get recent exposures")
	}
	return videoIds, nil
}

// GetExposureStats 获取曝光统计
func GetExposureStats(ctx context.Context, videoId int64, startTime, endTime time.Time) (map[string]interface{}, error) {
	var stats struct {
		TotalExposures int64   `gorm:"column:total_exposures"`
		TotalClicks    int64   `gorm:"column:total_clicks"`
		TotalLikes     int64   `gorm:"column:total_likes"`
		AvgWatchTime   float64 `gorm:"column:avg_watch_time"`
		AvgCompletion  float64 `gorm:"column:avg_completion"`
	}

	err := DB.WithContext(ctx).Model(&model.RecommendationExposure{}).
		Select(`
			COUNT(*) as total_exposures,
			SUM(is_clicked) as total_clicks,
			SUM(is_liked) as total_likes,
			AVG(watch_duration) as avg_watch_time,
			AVG(completion_rate) as avg_completion
		`).
		Where("video_id = ? AND exposure_time BETWEEN ? AND ?", videoId, startTime, endTime).
		Scan(&stats).Error

	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get exposure stats")
	}

	return map[string]interface{}{
		"total_exposures": stats.TotalExposures,
		"total_clicks":    stats.TotalClicks,
		"total_likes":     stats.TotalLikes,
		"avg_watch_time":  stats.AvgWatchTime,
		"avg_completion":  stats.AvgCompletion,
		"ctr":             float64(stats.TotalClicks) / float64(stats.TotalExposures+1),
	}, nil
}

// ========================================
// Negative Feedback Operations (负反馈操作)
// ========================================

// RecordNegativeFeedback 记录负反馈
func RecordNegativeFeedback(ctx context.Context, feedback *model.NegativeFeedback) error {
	return DB.WithContext(ctx).Create(feedback).Error
}

// GetUserNegativeFeedbacks 获取用户负反馈
func GetUserNegativeFeedbacks(ctx context.Context, userId int64, targetType int8) ([]*model.NegativeFeedback, error) {
	var feedbacks []*model.NegativeFeedback
	query := DB.WithContext(ctx).Where("user_id = ?", userId)
	if targetType > 0 {
		query = query.Where("target_type = ?", targetType)
	}
	// 排除已过期的
	query = query.Where("expire_at IS NULL OR expire_at > ?", time.Now())
	err := query.Find(&feedbacks).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get user negative feedbacks")
	}
	return feedbacks, nil
}

// GetUserBlockedVideoIds 获取用户屏蔽的视频ID列表
func GetUserBlockedVideoIds(ctx context.Context, userId int64) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.NegativeFeedback{}).
		Where("user_id = ? AND target_type = ? AND (expire_at IS NULL OR expire_at > ?)",
			userId, model.NegativeFeedbackTargetVideo, time.Now()).
		Pluck("target_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get blocked video ids")
	}
	return videoIds, nil
}

// GetUserBlockedAuthorIds 获取用户屏蔽的作者ID列表
func GetUserBlockedAuthorIds(ctx context.Context, userId int64) ([]int64, error) {
	var authorIds []int64
	err := DB.WithContext(ctx).Model(&model.NegativeFeedback{}).
		Where("user_id = ? AND target_type = ? AND (expire_at IS NULL OR expire_at > ?)",
			userId, model.NegativeFeedbackTargetAuthor, time.Now()).
		Pluck("target_id", &authorIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get blocked author ids")
	}
	return authorIds, nil
}

// DeleteNegativeFeedback 删除负反馈
func DeleteNegativeFeedback(ctx context.Context, userId int64, targetType int8, targetId int64) error {
	return DB.WithContext(ctx).
		Where("user_id = ? AND target_type = ? AND target_id = ?", userId, targetType, targetId).
		Delete(&model.NegativeFeedback{}).Error
}

// ========================================
// Video Hot Score Operations (视频热度操作)
// ========================================

// GetVideoHotScore 获取视频热度
func GetVideoHotScore(ctx context.Context, videoId int64, timeWindow string) (*model.VideoHotScore, error) {
	var hotScore model.VideoHotScore
	err := DB.WithContext(ctx).
		Where("video_id = ? AND time_window = ?", videoId, timeWindow).
		First(&hotScore).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get video hot score")
	}
	return &hotScore, nil
}

// CreateOrUpdateVideoHotScore 创建或更新视频热度
func CreateOrUpdateVideoHotScore(ctx context.Context, hotScore *model.VideoHotScore) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}, {Name: "time_window"}},
		UpdateAll: true,
	}).Create(hotScore).Error
}

// GetHotVideosByWindow 获取指定时间窗口的热门视频列表
func GetHotVideosByWindow(ctx context.Context, timeWindow string, limit int) ([]*model.VideoHotScore, error) {
	var hotScores []*model.VideoHotScore
	err := DB.WithContext(ctx).
		Where("time_window = ?", timeWindow).
		Order("hot_score DESC").
		Limit(limit).
		Find(&hotScores).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get hot videos")
	}
	return hotScores, nil
}

// GetHotVideoIds 获取热门视频ID列表
func GetHotVideoIds(ctx context.Context, timeWindow string, limit int) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.VideoHotScore{}).
		Where("time_window = ?", timeWindow).
		Order("hot_score DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get hot video ids")
	}
	return videoIds, nil
}

// BatchUpdateVideoHotScores 批量更新视频热度
func BatchUpdateVideoHotScores(ctx context.Context, hotScores []*model.VideoHotScore) error {
	if len(hotScores) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "video_id"}, {Name: "time_window"}},
		UpdateAll: true,
	}).CreateInBatches(hotScores, 100).Error
}

// UpdateHotScoreRanks 更新热度排名
func UpdateHotScoreRanks(ctx context.Context, timeWindow string) error {
	// 使用子查询更新排名
	sql := `
		UPDATE video_hot_scores vhs
		JOIN (
			SELECT video_id, 
				   ROW_NUMBER() OVER (ORDER BY hot_score DESC) as new_rank
			FROM video_hot_scores
			WHERE time_window = ?
		) ranked ON vhs.video_id = ranked.video_id AND vhs.time_window = ?
		SET vhs.rank = ranked.new_rank
	`
	return DB.WithContext(ctx).Exec(sql, timeWindow, timeWindow).Error
}

// ========================================
// Author Score Operations (作者评分操作)
// ========================================

// GetAuthorScore 获取作者评分
func GetAuthorScore(ctx context.Context, authorId int64) (*model.AuthorScore, error) {
	var score model.AuthorScore
	err := DB.WithContext(ctx).Where("author_id = ?", authorId).First(&score).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get author score")
	}
	return &score, nil
}

// CreateOrUpdateAuthorScore 创建或更新作者评分
func CreateOrUpdateAuthorScore(ctx context.Context, score *model.AuthorScore) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "author_id"}},
		UpdateAll: true,
	}).Create(score).Error
}

// BatchGetAuthorScores 批量获取作者评分
func BatchGetAuthorScores(ctx context.Context, authorIds []int64) ([]*model.AuthorScore, error) {
	var scores []*model.AuthorScore
	if len(authorIds) == 0 {
		return scores, nil
	}
	err := DB.WithContext(ctx).Where("author_id IN ?", authorIds).Find(&scores).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to batch get author scores")
	}
	return scores, nil
}

// GetTopAuthors 获取优质作者列表
func GetTopAuthors(ctx context.Context, limit int) ([]*model.AuthorScore, error) {
	var scores []*model.AuthorScore
	err := DB.WithContext(ctx).
		Order("overall_score DESC").
		Limit(limit).
		Find(&scores).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get top authors")
	}
	return scores, nil
}

// ========================================
// Tag Video Mapping Operations (标签映射操作)
// ========================================

// GetVideosByTag 根据标签获取视频
func GetVideosByTag(ctx context.Context, tagName string, limit int) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.TagVideoMapping{}).
		Where("tag_name = ?", tagName).
		Order("weight DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get videos by tag")
	}
	return videoIds, nil
}

// GetVideoTags 获取视频的标签
func GetVideoTags(ctx context.Context, videoId int64) ([]*model.TagVideoMapping, error) {
	var mappings []*model.TagVideoMapping
	err := DB.WithContext(ctx).
		Where("video_id = ?", videoId).
		Order("weight DESC").
		Find(&mappings).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get video tags")
	}
	return mappings, nil
}

// SaveTagVideoMapping 保存标签视频映射
func SaveTagVideoMapping(ctx context.Context, mapping *model.TagVideoMapping) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tag_name"}, {Name: "video_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"weight"}),
	}).Create(mapping).Error
}

// BatchSaveTagVideoMappings 批量保存标签视频映射
func BatchSaveTagVideoMappings(ctx context.Context, mappings []*model.TagVideoMapping) error {
	if len(mappings) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tag_name"}, {Name: "video_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"weight"}),
	}).CreateInBatches(mappings, 100).Error
}

// ========================================
// Category Stats Operations (分类统计操作)
// ========================================

// GetCategoryStats 获取分类统计
func GetCategoryStats(ctx context.Context, category string) (*model.CategoryVideoStats, error) {
	var stats model.CategoryVideoStats
	err := DB.WithContext(ctx).Where("category = ?", category).First(&stats).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get category stats")
	}
	return &stats, nil
}

// GetAllCategoryStats 获取所有分类统计
func GetAllCategoryStats(ctx context.Context) ([]*model.CategoryVideoStats, error) {
	var stats []*model.CategoryVideoStats
	err := DB.WithContext(ctx).Order("hot_score DESC").Find(&stats).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get all category stats")
	}
	return stats, nil
}

// UpdateCategoryStats 更新分类统计
func UpdateCategoryStats(ctx context.Context, stats *model.CategoryVideoStats) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "category"}},
		UpdateAll: true,
	}).Create(stats).Error
}

// IncrementCategoryVideoCount 增加分类视频数
func IncrementCategoryVideoCount(ctx context.Context, category string) error {
	return DB.WithContext(ctx).Model(&model.CategoryVideoStats{}).
		Where("category = ?", category).
		Updates(map[string]interface{}{
			"total_videos":     gorm.Expr("total_videos + 1"),
			"daily_new_videos": gorm.Expr("daily_new_videos + 1"),
		}).Error
}

// ========================================
// User Video Interaction Operations (用户视频交互操作)
// ========================================

// GetUserVideoInteraction 获取用户视频交互详情
func GetUserVideoInteraction(ctx context.Context, userId, videoId int64) (*model.UserVideoInteraction, error) {
	var interaction model.UserVideoInteraction
	err := DB.WithContext(ctx).
		Where("user_id = ? AND video_id = ?", userId, videoId).
		First(&interaction).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get user video interaction")
	}
	return &interaction, nil
}

// CreateOrUpdateUserVideoInteraction 创建或更新用户视频交互
func CreateOrUpdateUserVideoInteraction(ctx context.Context, interaction *model.UserVideoInteraction) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "video_id"}},
		UpdateAll: true,
	}).Create(interaction).Error
}

// IncrementInteractionCounter 增加交互计数器
func IncrementInteractionCounter(ctx context.Context, userId, videoId int64, field string, delta int) error {
	now := time.Now()
	// 先尝试更新
	result := DB.WithContext(ctx).Model(&model.UserVideoInteraction{}).
		Where("user_id = ? AND video_id = ?", userId, videoId).
		Updates(map[string]interface{}{
			field:              gorm.Expr(field+" + ?", delta),
			"last_interact_at": now,
		})

	if result.RowsAffected == 0 {
		// 记录不存在，创建新记录
		interaction := &model.UserVideoInteraction{
			UserID:          userId,
			VideoID:         videoId,
			FirstInteractAt: now,
			LastInteractAt:  now,
		}
		// 设置对应字段
		switch field {
		case "impression_count":
			interaction.ImpressionCount = delta
		case "click_count":
			interaction.ClickCount = delta
		case "replay_count":
			interaction.ReplayCount = delta
		case "comment_count":
			interaction.CommentCount = delta
		}
		return DB.WithContext(ctx).Create(interaction).Error
	}
	return result.Error
}

// GetUserHighEngagementVideos 获取用户高互动视频
func GetUserHighEngagementVideos(ctx context.Context, userId int64, limit int) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.UserVideoInteraction{}).
		Where("user_id = ?", userId).
		Order("engagement_score DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get high engagement videos")
	}
	return videoIds, nil
}

// ========================================
// A/B Test Operations (A/B测试操作)
// ========================================

// GetRunningExperiments 获取运行中的实验
func GetRunningExperiments(ctx context.Context) ([]*model.ABTestExperiment, error) {
	var experiments []*model.ABTestExperiment
	err := DB.WithContext(ctx).Where("status = 1").Find(&experiments).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get running experiments")
	}
	return experiments, nil
}

// GetExperimentGroups 获取实验分组
func GetExperimentGroups(ctx context.Context, experimentId int64) ([]*model.ABTestGroup, error) {
	var groups []*model.ABTestGroup
	err := DB.WithContext(ctx).Where("experiment_id = ?", experimentId).Find(&groups).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get experiment groups")
	}
	return groups, nil
}

// GetUserExperimentAssignment 获取用户实验分配
func GetUserExperimentAssignment(ctx context.Context, userId, experimentId int64) (*model.UserABTestAssignment, error) {
	var assignment model.UserABTestAssignment
	err := DB.WithContext(ctx).
		Where("user_id = ? AND experiment_id = ?", userId, experimentId).
		First(&assignment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get user experiment assignment")
	}
	return &assignment, nil
}

// AssignUserToExperiment 将用户分配到实验
func AssignUserToExperiment(ctx context.Context, assignment *model.UserABTestAssignment) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "experiment_id"}},
		DoNothing: true,
	}).Create(assignment).Error
}

// ========================================
// Bloom Filter Operations (布隆过滤器操作)
// ========================================

// GetUserBloomFilter 获取用户布隆过滤器
func GetUserBloomFilter(ctx context.Context, userId int64) (*model.RecommendationBloomFilter, error) {
	var filter model.RecommendationBloomFilter
	err := DB.WithContext(ctx).Where("user_id = ?", userId).First(&filter).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.WithMessage(err, "Failed to get user bloom filter")
	}
	return &filter, nil
}

// SaveUserBloomFilter 保存用户布隆过滤器
func SaveUserBloomFilter(ctx context.Context, filter *model.RecommendationBloomFilter) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		UpdateAll: true,
	}).Create(filter).Error
}

// ========================================
// Utility Functions (工具函数)
// ========================================

// GetNewVideos 获取新发布的视频
func GetNewVideos(ctx context.Context, hours int, limit int) ([]int64, error) {
	var videoIds []int64
	cutoffTime := time.Now().Add(-time.Duration(hours) * time.Hour)
	err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("created_at >= ? AND deleted_at IS NULL AND audit_status = 1 AND open = 1", cutoffTime).
		Order("created_at DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get new videos")
	}
	return videoIds, nil
}

// GetVideosByCategory 按分类获取视频
func GetVideosByCategory(ctx context.Context, category string, limit int) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("category = ? AND deleted_at IS NULL AND audit_status = 1 AND open = 1", category).
		Order("created_at DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get videos by category")
	}
	return videoIds, nil
}

// GetUserLikedVideos 获取用户点赞的视频
func GetUserLikedVideos(ctx context.Context, userId int64, limit int) ([]int64, error) {
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.VideoLike{}).
		Where("user_id = ? AND deleted_at IS NULL", userId).
		Order("created_at DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get user liked videos")
	}
	return videoIds, nil
}

// GetUserFollowingAuthorVideos 获取用户关注作者的视频
func GetUserFollowingAuthorVideos(ctx context.Context, followingIds []int64, limit int) ([]int64, error) {
	if len(followingIds) == 0 {
		return nil, nil
	}
	var videoIds []int64
	err := DB.WithContext(ctx).Model(&model.Video{}).
		Where("user_id IN ? AND deleted_at IS NULL AND audit_status = 1 AND open = 1", followingIds).
		Order("created_at DESC").
		Limit(limit).
		Pluck("video_id", &videoIds).Error
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get following author videos")
	}
	return videoIds, nil
}

// CleanupExpiredExposures 清理过期的曝光记录
func CleanupExpiredExposures(ctx context.Context, days int) (int64, error) {
	cutoffTime := time.Now().AddDate(0, 0, -days)
	result := DB.WithContext(ctx).
		Where("exposure_time < ?", cutoffTime).
		Delete(&model.RecommendationExposure{})
	if result.Error != nil {
		return 0, errors.WithMessage(result.Error, "Failed to cleanup expired exposures")
	}
	hlog.Infof("Cleaned up %d expired exposure records", result.RowsAffected)
	return result.RowsAffected, nil
}

// CleanupExpiredNegativeFeedbacks 清理过期的负反馈
func CleanupExpiredNegativeFeedbacks(ctx context.Context) (int64, error) {
	result := DB.WithContext(ctx).
		Where("expire_at IS NOT NULL AND expire_at < ?", time.Now()).
		Delete(&model.NegativeFeedback{})
	if result.Error != nil {
		return 0, errors.WithMessage(result.Error, "Failed to cleanup expired negative feedbacks")
	}
	return result.RowsAffected, nil
}
