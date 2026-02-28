package service

import (
	"context"
	"math"
	"strings"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/internal/model"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// ============================================================
// RecommendationBridge
// 连接业务流（上传、观看、点赞等）与推荐系统数据表。
// 所有方法均为"尽力而为"，失败只打日志不阻塞主流程。
// ============================================================

// OnVideoPublished 视频发布成功后调用。
// 初始化 video_features、tag_video_mappings、category_video_stats、author_scores。
func OnVideoPublished(ctx context.Context, videoID, userID int64, title, description, tags, category string) {
	go func() {
		bgCtx := context.Background()

		// 1. 创建 video_features 初始行
		now := time.Now()
		freshnessScore := 10.0 // 新发布的视频新鲜度满分
		feature := &model.VideoFeature{
			VideoID:         videoID,
			QualityScore:    5.0, // 初始中等质量分
			PopularityScore: 1.0, // 初始热度
			FreshnessScore:  freshnessScore,
			CTR:             0,
			FinishRate:      0,
			LikeRate:        0,
			CommentRate:     0,
			ShareRate:       0,
			FavoriteRate:    0,
			InteractScore:   0,
			ExposureCount:   0,
			ClickCount:      0,
			AuthorScore:     0,
			IsHighQuality:   0,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		// 根据标题/描述长度给一个基础质量分加成
		if len(description) > 50 {
			feature.QualityScore += 1.0
		}
		if len(title) > 5 {
			feature.QualityScore += 0.5
		}

		if err := db.CreateOrUpdateVideoFeature(bgCtx, feature); err != nil {
			hlog.Warnf("[RecBridge] Failed to create video_features for video %d: %v", videoID, err)
		} else {
			hlog.Infof("[RecBridge] Created video_features for video %d (quality=%.1f, freshness=%.1f)", videoID, feature.QualityScore, freshnessScore)
		}

		// 2. 写入 tag_video_mappings
		if tags != "" {
			tagList := splitTags(tags)
			var mappings []*model.TagVideoMapping
			for _, tag := range tagList {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				mappings = append(mappings, &model.TagVideoMapping{
					TagName: tag,
					VideoID: videoID,
					Weight:  1.0,
				})
			}
			if len(mappings) > 0 {
				if err := db.BatchSaveTagVideoMappings(bgCtx, mappings); err != nil {
					hlog.Warnf("[RecBridge] Failed to save tag mappings for video %d: %v", videoID, err)
				} else {
					hlog.Infof("[RecBridge] Saved %d tag mappings for video %d", len(mappings), videoID)
				}
			}
		}

		// 3. 更新 category_video_stats
		if category != "" {
			if err := db.IncrementCategoryVideoCount(bgCtx, category); err != nil {
				hlog.Warnf("[RecBridge] Failed to increment category count for '%s': %v", category, err)
			}
		}

		// 4. 更新 author_scores（发布活跃度）
		authorScore, _ := db.GetAuthorScore(bgCtx, userID)
		if authorScore == nil {
			authorScore = &model.AuthorScore{
				AuthorID:       userID,
				QualityScore:   5.0,
				ActivityScore:  1.0,
				InfluenceScore: 0,
				GrowthScore:    1.0,
				OverallScore:   6.0,
			}
		} else {
			authorScore.ActivityScore = math.Min(authorScore.ActivityScore+0.5, 10.0)
			authorScore.OverallScore = (authorScore.QualityScore + authorScore.ActivityScore + authorScore.InfluenceScore + authorScore.GrowthScore) / 4.0
		}
		if err := db.CreateOrUpdateAuthorScore(bgCtx, authorScore); err != nil {
			hlog.Warnf("[RecBridge] Failed to update author score for user %d: %v", userID, err)
		}

		hlog.Infof("[RecBridge] Video %d publish pipeline completed", videoID)
	}()
}

// OnVideoViewed 用户观看视频时调用。
// 更新 video_features 的曝光/点击、user_profiles 的观看计数。
func OnVideoViewed(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()

		// 增加视频曝光和点击
		if err := db.IncrementVideoExposure(bgCtx, videoID); err != nil {
			hlog.Warnf("[RecBridge] Failed to increment exposure for video %d: %v", videoID, err)
		}
		if err := db.IncrementVideoClick(bgCtx, videoID); err != nil {
			hlog.Warnf("[RecBridge] Failed to increment click for video %d: %v", videoID, err)
		}

		// 更新用户画像观看计数
		if userID > 0 {
			ensureUserProfile(bgCtx, userID)
			if err := db.IncrementUserProfileCounter(bgCtx, userID, "total_view_count", 1); err != nil {
				hlog.Warnf("[RecBridge] Failed to increment view count for user %d: %v", userID, err)
			}
		}
	}()
}

// OnVideoLiked 用户点赞视频时调用。
func OnVideoLiked(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScore(bgCtx, videoID)
		if userID > 0 {
			ensureUserProfile(bgCtx, userID)
			if err := db.IncrementUserProfileCounter(bgCtx, userID, "total_like_count", 1); err != nil {
				hlog.Warnf("[RecBridge] Failed to increment like count for user %d: %v", userID, err)
			}
		}
	}()
}

// OnVideoCommented 用户评论视频时调用。
func OnVideoCommented(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScore(bgCtx, videoID)
		if userID > 0 {
			ensureUserProfile(bgCtx, userID)
			if err := db.IncrementUserProfileCounter(bgCtx, userID, "total_comment_count", 1); err != nil {
				hlog.Warnf("[RecBridge] Failed to increment comment count for user %d: %v", userID, err)
			}
		}
	}()
}

// OnVideoShared 用户分享视频时调用。
func OnVideoShared(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScore(bgCtx, videoID)
		if userID > 0 {
			ensureUserProfile(bgCtx, userID)
			if err := db.IncrementUserProfileCounter(bgCtx, userID, "total_share_count", 1); err != nil {
				hlog.Warnf("[RecBridge] Failed to increment share count for user %d: %v", userID, err)
			}
		}
	}()
}

// OnVideoFavorited 用户收藏视频时调用。
func OnVideoFavorited(ctx context.Context, videoID, userID int64) {
	go func() {
		bgCtx := context.Background()
		updateVideoInteractScore(bgCtx, videoID)
	}()
}

// ============================================================
// 内部辅助函数
// ============================================================

// updateVideoInteractScore 根据 videos 表的计数重新计算 video_features 的互动分和热度分。
func updateVideoInteractScore(ctx context.Context, videoID int64) {
	video, err := db.GetVideoInfo(ctx, videoID)
	if err != nil || video == nil {
		return
	}

	likes := float64(video.LikesCount)
	comments := float64(video.CommentCount)
	shares := float64(video.ShareCount)
	visits := float64(video.VisitCount)

	interactScore := likes*3 + comments*5 + shares*8
	popularityScore := interactScore + visits

	// 计算各项率
	metrics := map[string]interface{}{
		"interact_score":   interactScore,
		"popularity_score": popularityScore,
		"updated_at":       time.Now(),
	}
	if visits > 0 {
		metrics["like_rate"] = likes / visits
		metrics["comment_rate"] = comments / visits
		metrics["share_rate"] = shares / visits
	}

	if err := db.UpdateVideoFeatureMetrics(ctx, videoID, metrics); err != nil {
		hlog.Warnf("[RecBridge] Failed to update interact score for video %d: %v", videoID, err)
	}
}

// ensureUserProfile 确保 user_profiles 行存在（不存在则创建初始行）。
func ensureUserProfile(ctx context.Context, userID int64) {
	profile, _ := db.GetUserProfile(ctx, userID)
	if profile != nil {
		return
	}
	now := time.Now()
	newProfile := &model.UserProfile{
		UserID:     userID,
		UserLevel:  1,
		LastActiveAt: &now,
	}
	if err := db.CreateOrUpdateUserProfile(ctx, newProfile); err != nil {
		hlog.Warnf("[RecBridge] Failed to create user profile for user %d: %v", userID, err)
	}
}

// splitTags 分割标签字符串（支持逗号和中文逗号）。
func splitTags(tags string) []string {
	tags = strings.ReplaceAll(tags, "，", ",")
	return strings.Split(tags, ",")
}

// BackfillVideoFeatures 启动时检查并补建缺失的 video_features 行。
// 确保所有已有视频都能进入推荐池。
func BackfillVideoFeatures() {
	go func() {
		ctx := context.Background()
		now := time.Now()

		// 查出所有视频
		var allVideos []struct {
			VideoID     int64  `gorm:"column:video_id"`
			Title       string `gorm:"column:title"`
			Description string `gorm:"column:description"`
			LabelNames  string `gorm:"column:label_names"`
			Category    string `gorm:"column:category"`
			UserID      int64  `gorm:"column:user_id"`
			VisitCount  int64  `gorm:"column:visit_count"`
			LikesCount  int64  `gorm:"column:likes_count"`
			CommentCount int64 `gorm:"column:comment_count"`
			ShareCount  int64  `gorm:"column:share_count"`
		}
		if err := db.DB.WithContext(ctx).Table("videos").Find(&allVideos).Error; err != nil {
			hlog.Warnf("[RecBridge] Backfill: failed to query videos: %v", err)
			return
		}

		backfilled := 0
		for _, v := range allVideos {
			// 检查是否已有 video_features 行
			existing, _ := db.GetVideoFeature(ctx, v.VideoID)
			if existing != nil {
				continue
			}

			// 计算互动分
			interactScore := float64(v.LikesCount)*3 + float64(v.CommentCount)*5 + float64(v.ShareCount)*8
			popularityScore := interactScore + float64(v.VisitCount)
			qualityScore := 5.0
			if len(v.Description) > 50 {
				qualityScore += 1.0
			}
			if len(v.Title) > 5 {
				qualityScore += 0.5
			}

			feature := &model.VideoFeature{
				VideoID:         v.VideoID,
				QualityScore:    qualityScore,
				PopularityScore: popularityScore,
				FreshnessScore:  5.0, // 已有视频给中等新鲜度
				InteractScore:   interactScore,
				ExposureCount:   v.VisitCount,
				ClickCount:      v.VisitCount,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			if v.VisitCount > 0 {
				feature.LikeRate = float64(v.LikesCount) / float64(v.VisitCount)
				feature.CommentRate = float64(v.CommentCount) / float64(v.VisitCount)
				feature.ShareRate = float64(v.ShareCount) / float64(v.VisitCount)
				feature.CTR = float64(v.VisitCount) / float64(v.VisitCount+10) // 平滑 CTR
			}

			if err := db.CreateOrUpdateVideoFeature(ctx, feature); err != nil {
				hlog.Warnf("[RecBridge] Backfill: failed for video %d: %v", v.VideoID, err)
				continue
			}

			// 同时补建标签映射
			if v.LabelNames != "" {
				tagList := splitTags(v.LabelNames)
				var mappings []*model.TagVideoMapping
				for _, tag := range tagList {
					tag = strings.TrimSpace(tag)
					if tag == "" {
						continue
					}
					mappings = append(mappings, &model.TagVideoMapping{
						TagName: tag,
						VideoID: v.VideoID,
						Weight:  1.0,
					})
				}
				if len(mappings) > 0 {
					db.BatchSaveTagVideoMappings(ctx, mappings)
				}
			}

			// 补建分类统计
			if v.Category != "" {
				db.IncrementCategoryVideoCount(ctx, v.Category)
			}

			backfilled++
		}

		if backfilled > 0 {
			hlog.Infof("[RecBridge] Backfilled video_features for %d existing videos", backfilled)
		} else {
			hlog.Info("[RecBridge] All videos already have video_features entries")
		}
	}()
}
