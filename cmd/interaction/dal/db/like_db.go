package db

import (
	"context"
	"time"

	"HuaTug.com/internal/model"
	"gorm.io/gorm"
)

// GetVideoLikeByUserAndVideo 检查用户是否点赞了某视频
func GetVideoLikeByUserAndVideo(ctx context.Context, userID, videoID int64) (*model.VideoLike, error) {
	var like model.VideoLike
	err := DB.WithContext(ctx).
		Where("user_id = ? AND video_id = ? AND deleted_at IS NULL", userID, videoID).
		First(&like).Error
	if err != nil {
		return nil, err
	}
	return &like, nil
}

// CreateVideoLike 创建视频点赞记录（事务：插入点赞记录 + 更新视频点赞计数）
// 返回值：created 表示是否真正创建/恢复了记录（用于判断是否需要更新缓存）
func CreateVideoLike(ctx context.Context, userID, videoID int64) (created bool, err error) {
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 检查是否已存在（包括软删除的记录）
		var existing model.VideoLike
		findErr := tx.Unscoped().
			Where("user_id = ? AND video_id = ?", userID, videoID).
			First(&existing).Error

		if findErr == nil {
			// 记录存在
			if existing.DeletedAt != nil {
				// 软删除状态，恢复记录
				if err := tx.Model(&existing).Updates(map[string]interface{}{
					"deleted_at": nil,
					"created_at": time.Now(),
				}).Error; err != nil {
					return err
				}
				// 更新视频点赞计数
				if err := tx.Model(&model.Video{}).
					Where("video_id = ?", videoID).
					UpdateColumn("likes_count", gorm.Expr("likes_count + ?", 1)).Error; err != nil {
					return err
				}
				created = true
			}
			// 已经存在且未删除，created = false
			return nil
		}

		// 记录不存在，创建新记录
		like := &model.VideoLike{
			UserId:    userID,
			VideoId:   videoID,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(like).Error; err != nil {
			return err
		}
		// 更新视频点赞计数
		if err := tx.Model(&model.Video{}).
			Where("video_id = ?", videoID).
			UpdateColumn("likes_count", gorm.Expr("likes_count + ?", 1)).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// DeleteVideoLike 删除视频点赞记录（事务：软删除点赞记录 + 更新视频点赞计数）
// 返回值：deleted 表示是否真正删除了记录
func DeleteVideoLike(ctx context.Context, userID, videoID int64) (deleted bool, err error) {
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 检查记录是否存在且未删除
		var existing model.VideoLike
		findErr := tx.Where("user_id = ? AND video_id = ? AND deleted_at IS NULL", userID, videoID).
			First(&existing).Error
		if findErr != nil {
			// 记录不存在或已删除，直接返回成功（幂等）
			return nil
		}

		// 软删除点赞记录
		if err := tx.Where("user_id = ? AND video_id = ?", userID, videoID).
			Delete(&model.VideoLike{}).Error; err != nil {
			return err
		}

		// 减少视频表的点赞计数（确保不小于0）
		if err := tx.Model(&model.Video{}).
			Where("video_id = ? AND likes_count > 0", videoID).
			UpdateColumn("likes_count", gorm.Expr("likes_count - ?", 1)).Error; err != nil {
			return err
		}

		deleted = true
		return nil
	})
	return deleted, err
}

// GetUserVideoLikes 获取用户点赞的视频ID列表
func GetUserVideoLikes(ctx context.Context, userID int64, offset, limit int) ([]int64, error) {
	var likes []model.VideoLike
	err := DB.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&likes).Error
	if err != nil {
		return nil, err
	}

	videoIDs := make([]int64, len(likes))
	for i, like := range likes {
		videoIDs[i] = like.VideoId
	}
	return videoIDs, nil
}

// GetCommentLikeByUserAndComment 检查用户是否点赞了某评论
func GetCommentLikeByUserAndComment(ctx context.Context, userID, commentID int64) (*model.CommentLike, error) {
	var like model.CommentLike
	err := DB.WithContext(ctx).
		Where("user_id = ? AND comment_id = ? AND deleted_at IS NULL", userID, commentID).
		First(&like).Error
	if err != nil {
		return nil, err
	}
	return &like, nil
}

// CreateCommentLike 创建评论点赞记录
func CreateCommentLike(ctx context.Context, userID, commentID int64) error {
	like := &model.CommentLike{
		UserId:    userID,
		CommentId: commentID,
		CreatedAt: time.Now(),
	}

	// 先检查是否已存在（包括软删除的记录）
	var existing model.CommentLike
	err := DB.WithContext(ctx).Unscoped().
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		First(&existing).Error

	if err == nil {
		// 记录存在，如果是软删除状态则恢复
		if existing.DeletedAt != nil {
			return DB.WithContext(ctx).Model(&existing).Updates(map[string]interface{}{
				"deleted_at": nil,
				"created_at": time.Now(),
			}).Error
		}
		// 已经存在且未删除，直接返回成功
		return nil
	}

	// 记录不存在，创建新记录
	return DB.WithContext(ctx).Create(like).Error
}

// DeleteCommentLike 删除评论点赞记录（软删除）
func DeleteCommentLike(ctx context.Context, userID, commentID int64) error {
	return DB.WithContext(ctx).
		Where("user_id = ? AND comment_id = ?", userID, commentID).
		Delete(&model.CommentLike{}).Error
}

// GetHotVideosByLikes 获取热门视频排行榜（按点赞数排序）
func GetHotVideosByLikes(ctx context.Context, limit int) ([]model.Video, error) {
	var videos []model.Video
	err := DB.WithContext(ctx).
		Where("deleted_at IS NULL").
		Order("likes_count DESC, visit_count DESC, created_at DESC").
		Limit(limit).
		Find(&videos).Error
	if err != nil {
		return nil, err
	}
	return videos, nil
}

// GetVideoLikeCount 从数据库获取视频点赞数
func GetVideoLikeCount(ctx context.Context, videoID int64) (int64, error) {
	var count int64
	err := DB.WithContext(ctx).
		Model(&model.VideoLike{}).
		Where("video_id = ? AND deleted_at IS NULL", videoID).
		Count(&count).Error
	return count, err
}

// BatchGetUserVideoLikeStatus 批量检查用户是否点赞了多个视频
func BatchGetUserVideoLikeStatus(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	if len(videoIDs) == 0 {
		return make(map[int64]bool), nil
	}

	var likes []model.VideoLike
	err := DB.WithContext(ctx).
		Where("user_id = ? AND video_id IN ? AND deleted_at IS NULL", userID, videoIDs).
		Find(&likes).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int64]bool)
	for _, videoID := range videoIDs {
		result[videoID] = false
	}
	for _, like := range likes {
		result[like.VideoId] = true
	}
	return result, nil
}

// HotVideoRankItem 热门视频排行项
type HotVideoRankItem struct {
	Rank        int       `json:"rank"`
	VideoId     int64     `json:"video_id"`
	Title       string    `json:"title"`
	CoverUrl    string    `json:"cover_url"`
	PlayUrl     string    `json:"play_url"`
	LikesCount  int64     `json:"likes_count"`
	VisitCount  int64     `json:"visit_count"`
	AuthorId    int64     `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetHotVideoRanking 获取热门视频排行榜（带详细信息）
func GetHotVideoRanking(ctx context.Context, limit int) ([]HotVideoRankItem, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	var videos []model.Video
	err := DB.WithContext(ctx).
		Where("deleted_at IS NULL AND likes_count > 0").
		Order("likes_count DESC, visit_count DESC, created_at DESC").
		Limit(limit).
		Find(&videos).Error
	if err != nil {
		return nil, err
	}

	items := make([]HotVideoRankItem, len(videos))
	for i, video := range videos {
		items[i] = HotVideoRankItem{
			Rank:       i + 1,
			VideoId:    video.VideoId,
			Title:      video.Title,
			CoverUrl:   video.CoverUrl,
			PlayUrl:    video.VideoUrl,
			LikesCount: int64(video.LikesCount),
			VisitCount: int64(video.VisitCount),
			AuthorId:   video.UserId,
			CreatedAt:  video.CreatedAt,
		}
	}
	return items, nil
}
