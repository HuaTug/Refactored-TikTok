package service

import (
	"context"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// ============================================================
// Interaction 服务的推荐桥接
// 直接通过共享 DB 连接更新推荐相关表（video_features / user_profiles），
// 避免跨微服务 RPC 回调的复杂性。
// 所有方法均为异步 + best-effort，不阻塞主流程。
// ============================================================

// OnVideoLikedFromInteraction 点赞视频时更新推荐数据。
func OnVideoLikedFromInteraction(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScoreFromInteraction(bgCtx, videoID)
		if userID > 0 {
			ensureUserProfileFromInteraction(bgCtx, userID)
			if err := db.DB.WithContext(bgCtx).Exec(
				"UPDATE user_profiles SET total_like_count = total_like_count + 1, last_active_at = ? WHERE user_id = ?",
				time.Now(), userID,
			).Error; err != nil {
				hlog.Warnf("[RecBridge-Interaction] Failed to increment like count for user %d: %v", userID, err)
			}
		}
	}()
}

// OnVideoCommentedFromInteraction 评论视频时更新推荐数据。
func OnVideoCommentedFromInteraction(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScoreFromInteraction(bgCtx, videoID)
		if userID > 0 {
			ensureUserProfileFromInteraction(bgCtx, userID)
			if err := db.DB.WithContext(bgCtx).Exec(
				"UPDATE user_profiles SET total_comment_count = total_comment_count + 1, last_active_at = ? WHERE user_id = ?",
				time.Now(), userID,
			).Error; err != nil {
				hlog.Warnf("[RecBridge-Interaction] Failed to increment comment count for user %d: %v", userID, err)
			}
		}
	}()
}

// ============================================================
// 内部辅助函数
// ============================================================

// updateVideoInteractScoreFromInteraction 根据 videos 表计数更新 video_features 的互动分。
func updateVideoInteractScoreFromInteraction(ctx context.Context, videoID int64) {
	var result struct {
		LikesCount   int64 `gorm:"column:likes_count"`
		CommentCount int64 `gorm:"column:comment_count"`
		ShareCount   int64 `gorm:"column:share_count"`
		VisitCount   int64 `gorm:"column:visit_count"`
	}
	if err := db.DB.WithContext(ctx).Table("videos").
		Select("likes_count, comment_count, share_count, visit_count").
		Where("video_id = ?", videoID).
		Take(&result).Error; err != nil {
		hlog.Warnf("[RecBridge-Interaction] Failed to get video counts for %d: %v", videoID, err)
		return
	}

	likes := float64(result.LikesCount)
	comments := float64(result.CommentCount)
	shares := float64(result.ShareCount)
	visits := float64(result.VisitCount)

	interactScore := likes*3 + comments*5 + shares*8
	popularityScore := interactScore + visits

	updates := map[string]interface{}{
		"interact_score":   interactScore,
		"popularity_score": popularityScore,
		"updated_at":       time.Now(),
	}
	if visits > 0 {
		updates["like_rate"] = likes / visits
		updates["comment_rate"] = comments / visits
		updates["share_rate"] = shares / visits
	}

	if err := db.DB.WithContext(ctx).Table("video_features").
		Where("video_id = ?", videoID).
		Updates(updates).Error; err != nil {
		hlog.Warnf("[RecBridge-Interaction] Failed to update interact score for video %d: %v", videoID, err)
	}
}

// ensureUserProfileFromInteraction 确保 user_profiles 行存在。
func ensureUserProfileFromInteraction(ctx context.Context, userID int64) {
	var count int64
	db.DB.WithContext(ctx).Table("user_profiles").Where("user_id = ?", userID).Count(&count)
	if count > 0 {
		return
	}
	now := time.Now()
	if err := db.DB.WithContext(ctx).Exec(
		"INSERT IGNORE INTO user_profiles (user_id, user_level, last_active_at, created_at, updated_at) VALUES (?, 1, ?, ?, ?)",
		userID, now, now, now,
	).Error; err != nil {
		hlog.Warnf("[RecBridge-Interaction] Failed to create user profile for %d: %v", userID, err)
	}
}
