package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/config"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	goredisv8 "github.com/go-redis/redis/v8"
)

// ============================================================
// Interaction 服务的推荐桥接
// 直接通过共享 DB 连接更新推荐相关表（video_features / user_profiles），
// 同时把行为打到 Redis user:recent_actions:{uid} ZSET，
// 让 RecommendationAgent 的实时画像层能实时感知。
// 所有方法均为异步 + best-effort，不阻塞主流程。
// ============================================================

// realtimeRedisClient 单独连一个 v8 client 到 DB=0（recommendation 包用的 DB），
// 不复用 interaction 的 DB=1 客户端。
var (
	realtimeRedisOnce sync.Once
	realtimeRedis     *goredisv8.Client
)

// realtimeRecentActionKey: 必须和 pkg/recommendation/realtime_state.go 里的
// realtimeActionKey() 保持一致：fmt.Sprintf("user:recent_actions:%d", userID)
func realtimeRecentActionKey(userID int64) string {
	return fmt.Sprintf("user:recent_actions:%d", userID)
}

// realtimeMaxActions: 必须 ≥ pkg/recommendation 里的 MaxActions（默认 50）。
const realtimeMaxActions = 50
const realtimeActionTTL = 7 * 24 * time.Hour

// realtimeUserAction 与 pkg/recommendation.UserAction 的 JSON tag 完全对齐。
type realtimeUserAction struct {
	VideoID    int64   `json:"video_id"`
	ActionType string  `json:"action_type"`
	Timestamp  int64   `json:"timestamp"`
	Duration   int     `json:"duration"`
	Progress   float64 `json:"progress"`
	Category   string  `json:"category"`
	Tags       string  `json:"tags"`
}

// getRealtimeRedis 懒加载一个 v8 client，连接 DB=0。
func getRealtimeRedis() *goredisv8.Client {
	realtimeRedisOnce.Do(func() {
		realtimeRedis = goredisv8.NewClient(&goredisv8.Options{
			Addr:     config.ConfigInfo.Redis.Addr,
			Password: config.ConfigInfo.Redis.Password,
			DB:       0,
		})
		if _, err := realtimeRedis.Ping(context.Background()).Result(); err != nil {
			hlog.Warnf("[RecBridge-Interaction] realtime redis ping failed: %v", err)
		} else {
			hlog.Info("[RecBridge-Interaction] realtime redis connected (DB=0)")
		}
	})
	return realtimeRedis
}

// recordRealtimeAction 把一条行为写入 user:recent_actions:{uid} ZSET，
// 并裁剪到最近 MaxActions 条。失败仅打日志，不阻塞调用方。
func recordRealtimeAction(ctx context.Context, userID int64, action realtimeUserAction) {
	rdb := getRealtimeRedis()
	if rdb == nil {
		return
	}

	data, err := json.Marshal(action)
	if err != nil {
		hlog.Warnf("[RecBridge-Interaction] marshal action failed: %v", err)
		return
	}

	key := realtimeRecentActionKey(userID)
	pipe := rdb.Pipeline()
	pipe.ZAdd(ctx, key, &goredisv8.Z{
		Score:  float64(action.Timestamp),
		Member: string(data),
	})
	// 只保留最新 N 条
	pipe.ZRemRangeByRank(ctx, key, 0, int64(-realtimeMaxActions-1))
	pipe.Expire(ctx, key, realtimeActionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		hlog.Warnf("[RecBridge-Interaction] zadd realtime action failed: %v", err)
	}
}

// loadVideoMeta 给一条 action 补上 category/tags（标签等查不到时容忍空）。
func loadVideoMeta(ctx context.Context, videoID int64) (category, tags string) {
	var v struct {
		Category   string `gorm:"column:category"`
		LabelNames string `gorm:"column:label_names"`
	}
	if err := db.DB.WithContext(ctx).Table("videos").
		Select("category, label_names").
		Where("video_id = ?", videoID).
		Take(&v).Error; err != nil {
		return "", ""
	}
	return v.Category, v.LabelNames
}

// writeUserBehavior 落盘行为流水（训练数据来源之一）。
func writeUserBehavior(ctx context.Context, userID, videoID int64, behaviorType string) {
	now := time.Now()
	if err := db.DB.WithContext(ctx).Exec(
		"INSERT INTO user_behaviors (user_id, video_id, behavior_type, behavior_time, created_at) VALUES (?, ?, ?, ?, ?)",
		userID, videoID, behaviorType, now, now,
	).Error; err != nil {
		hlog.Warnf("[RecBridge-Interaction] insert user_behaviors failed: user=%d video=%d type=%s err=%v",
			userID, videoID, behaviorType, err)
	}
}

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

			// --- 新增：同步写入 user_behaviors + Redis 实时行为流 ---
			writeUserBehavior(bgCtx, userID, videoID, "like")
			category, tags := loadVideoMeta(bgCtx, videoID)
			recordRealtimeAction(bgCtx, userID, realtimeUserAction{
				VideoID:    videoID,
				ActionType: "like",
				Timestamp:  time.Now().UnixMilli(),
				// like 这种动作没有显式 progress/duration，用 0.95/0 让 Agent 视为 deep interaction
				Progress: 0.95,
				Duration: 0,
				Category: category,
				Tags:     tags,
			})
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

			// --- 新增：同步写入 user_behaviors + Redis 实时行为流 ---
			writeUserBehavior(bgCtx, userID, videoID, "comment")
			category, tags := loadVideoMeta(bgCtx, videoID)
			recordRealtimeAction(bgCtx, userID, realtimeUserAction{
				VideoID:    videoID,
				ActionType: "comment",
				Timestamp:  time.Now().UnixMilli(),
				Progress:   0.85,
				Duration:   0,
				Category:   category,
				Tags:       tags,
			})
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
