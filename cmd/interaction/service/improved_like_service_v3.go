package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	redisClient "github.com/go-redis/redis"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ImprovedLikeService 改进的点赞服务
type ImprovedLikeService struct {
	ctx          context.Context
	producer     *mq.Producer
	database     *gorm.DB
	cacheManager *redis.ImprovedCacheManager
	syncService  *EventDrivenSyncService
	rateLimiter  *RateLimiter
}

// RateLimiter 限流器
type RateLimiter struct {
	redis *redisClient.Client
}

// NewImprovedLikeService 创建改进的点赞服务
func NewImprovedLikeService(ctx context.Context, producer *mq.Producer, database *gorm.DB, cacheManager *redis.ImprovedCacheManager) *ImprovedLikeService {
	syncService := NewEventDrivenSyncService(producer, database)

	return &ImprovedLikeService{
		ctx:          ctx,
		producer:     producer,
		database:     database,
		cacheManager: cacheManager,
		syncService:  syncService,
		rateLimiter:  &RateLimiter{},
	}
}

// LikeAction 点赞操作（改进版）
func (s *ImprovedLikeService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponseV2, error) {
	resp := &interactions.LikeActionResponseV2{
		Base: &base.Status{},
	}

	// 1. 参数验证
	if err := s.validateLikeRequest(req); err != nil {
		resp.Base.Code = 400
		resp.Base.Msg = err.Error()
		return resp, errno.RequestErr
	}

	// 2. 限流检查
	if err := s.checkRateLimit(ctx, req.UserId); err != nil {
		resp.Base.Code = 429
		resp.Base.Msg = "操作过于频繁，请稍后再试"
		return resp, err
	}

	// 3. 处理点赞操作
	var isLiked bool
	var err error

	if req.VideoId != 0 {
		isLiked, err = s.handleVideoLikeImproved(ctx, req)
	} else if req.CommentId != 0 {
		isLiked, err = s.handleCommentLikeImproved(ctx, req)
	} else {
		resp.Base.Code = 400
		resp.Base.Msg = "请求参数错误"
		return resp, errno.RequestErr
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like action: %v", err)
		resp.Base.Code = 500
		resp.Base.Msg = "处理点赞失败"
		return resp, err
	}

	// 4. 返回成功响应
	resp.Base.Code = 200
	resp.Base.Msg = "操作成功"
	resp.IsLiked = isLiked

	// 5. 获取最新计数
	if req.VideoId != 0 {
		resp.LikeCount, _ = s.cacheManager.IncrVideoLikeCountWithConsistency(ctx, req.VideoId)
	} else if req.CommentId != 0 {
		resp.LikeCount, _ = s.cacheManager.IncrCommentLikeCountWithConsistency(ctx, req.CommentId)
	}

	return resp, nil
}

// handleVideoLikeImproved 处理视频点赞（改进版）
func (s *ImprovedLikeService) handleVideoLikeImproved(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	// 1. 检查当前点赞状态
	currentStatus, err := s.cacheManager.GetUserLikeStatus(ctx, req.UserId, req.VideoId, "video")
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get user like status from cache: %v", err)
		// 从数据库查询
		currentStatus, err = s.getUserVideoLikeStatusFromDB(ctx, req.UserId, req.VideoId)
		if err != nil {
			return false, fmt.Errorf("failed to get user like status: %w", err)
		}
	}

	var targetStatus bool
	switch req.ActionType {
	case "like":
		if currentStatus {
			return true, nil // 已经点赞，直接返回
		}
		targetStatus = true
	case "unlike":
		if !currentStatus {
			return false, nil // 已经取消点赞，直接返回
		}
		targetStatus = false
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}

	// 2. 立即更新Redis缓存（提供快速响应）
	if err := s.updateVideoLikeCache(ctx, req, targetStatus); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update video like cache: %v", err)
		// 缓存更新失败不阻塞主流程
	}

	// 3. 发布同步事件到消息队列（异步处理数据库操作）
	syncEvent := &SyncEvent{
		EventID:        uuid.New().String(),
		EventType:      "video_like",
		ResourceType:   "video",
		ResourceID:     req.VideoId,
		UserID:         req.UserId,
		ActionType:     req.ActionType,
		Timestamp:      time.Now().Unix(),
		Priority:       1, // 中等优先级
		MaxRetries:     3,
		IdempotencyKey: s.generateIdempotencyKey("video_like", req.UserId, req.VideoId, req.ActionType),
	}

	if err := s.syncService.PublishSyncEvent(ctx, syncEvent); err != nil {
		hlog.CtxWarnf(ctx, "Failed to publish sync event: %v", err)
		// 消息队列发送失败，尝试直接处理
		if err := s.processVideoLikeDirectly(ctx, req, targetStatus); err != nil {
			return false, fmt.Errorf("failed to process video like: %w", err)
		}
	}

	// 4. 发送通知事件（如果是点赞操作）
	if req.ActionType == "like" {
		go s.sendLikeNotification(ctx, req.UserId, req.VideoId, "video")
	}

	return targetStatus, nil
}

// handleCommentLikeImproved 处理评论点赞（改进版）
func (s *ImprovedLikeService) handleCommentLikeImproved(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	// 1. 检查当前点赞状态
	currentStatus, err := s.cacheManager.GetUserLikeStatus(ctx, req.UserId, req.CommentId, "comment")
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get user comment like status from cache: %v", err)
		currentStatus, err = s.getUserCommentLikeStatusFromDB(ctx, req.UserId, req.CommentId)
		if err != nil {
			return false, fmt.Errorf("failed to get user comment like status: %w", err)
		}
	}

	var targetStatus bool
	switch req.ActionType {
	case "like":
		if currentStatus {
			return true, nil
		}
		targetStatus = true
	case "unlike":
		if !currentStatus {
			return false, nil
		}
		targetStatus = false
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}

	// 2. 立即更新Redis缓存
	if err := s.updateCommentLikeCache(ctx, req, targetStatus); err != nil {
		hlog.CtxErrorf(ctx, "Failed to update comment like cache: %v", err)
	}

	// 3. 发布同步事件
	syncEvent := &SyncEvent{
		EventID:        uuid.New().String(),
		EventType:      "comment_like",
		ResourceType:   "comment",
		ResourceID:     req.CommentId,
		UserID:         req.UserId,
		ActionType:     req.ActionType,
		Timestamp:      time.Now().Unix(),
		Priority:       1,
		MaxRetries:     3,
		IdempotencyKey: s.generateIdempotencyKey("comment_like", req.UserId, req.CommentId, req.ActionType),
	}

	if err := s.syncService.PublishSyncEvent(ctx, syncEvent); err != nil {
		hlog.CtxWarnf(ctx, "Failed to publish sync event: %v", err)
		if err := s.processCommentLikeDirectly(ctx, req, targetStatus); err != nil {
			return false, fmt.Errorf("failed to process comment like: %w", err)
		}
	}

	// 4. 发送通知事件
	if req.ActionType == "like" {
		go s.sendLikeNotification(ctx, req.UserId, req.CommentId, "comment")
	}

	return targetStatus, nil
}

// updateVideoLikeCache 更新视频点赞缓存
func (s *ImprovedLikeService) updateVideoLikeCache(ctx context.Context, req *interactions.LikeActionRequest, liked bool) error {
	// 1. 更新用户点赞状态
	if err := s.cacheManager.SetUserLikeStatus(ctx, req.UserId, req.VideoId, "video", liked); err != nil {
		return err
	}

	// 2. 更新点赞计数
	if liked {
		_, err := s.cacheManager.IncrVideoLikeCountWithConsistency(ctx, req.VideoId)
		return err
	} else {
		_, err := s.cacheManager.DecrVideoLikeCountWithConsistency(ctx, req.VideoId)
		return err
	}
}

// updateCommentLikeCache 更新评论点赞缓存
func (s *ImprovedLikeService) updateCommentLikeCache(ctx context.Context, req *interactions.LikeActionRequest, liked bool) error {
	// 1. 更新用户点赞状态
	if err := s.cacheManager.SetUserLikeStatus(ctx, req.UserId, req.CommentId, "comment", liked); err != nil {
		return err
	}

	// 2. 更新点赞计数
	if liked {
		_, err := s.cacheManager.IncrCommentLikeCountWithConsistency(ctx, req.CommentId)
		return err
	} else {
		_, err := s.cacheManager.DecrCommentLikeCountWithConsistency(ctx, req.CommentId)
		return err
	}
}

// processVideoLikeDirectly 直接处理视频点赞（同步方式）
func (s *ImprovedLikeService) processVideoLikeDirectly(ctx context.Context, req *interactions.LikeActionRequest, liked bool) error {
	return s.database.Transaction(func(tx *gorm.DB) error {
		if liked {
			// 创建点赞记录
			videoLike := &model.VideoLike{
				UserId:    req.UserId,
				VideoId:   req.VideoId,
				CreatedAt: time.Now().Format(constants.DataFormate),
			}
			if err := tx.Create(videoLike).Error; err != nil {
				return fmt.Errorf("failed to create video like: %w", err)
			}

			// 创建用户行为记录
			behavior := &model.UserBehavior{
				UserId:       req.UserId,
				VideoId:      req.VideoId,
				BehaviorType: "like",
				BehaviorTime: time.Now().Format(constants.DataFormate),
			}
			if err := tx.Create(behavior).Error; err != nil {
				return fmt.Errorf("failed to create user behavior: %w", err)
			}
		} else {
			// 删除点赞记录
			if err := tx.Where("user_id = ? AND video_id = ?", req.UserId, req.VideoId).Delete(&model.VideoLike{}).Error; err != nil {
				return fmt.Errorf("failed to delete video like: %w", err)
			}

			// 删除用户行为记录
			if err := tx.Where("user_id = ? AND video_id = ? AND behavior_type = ?", req.UserId, req.VideoId, "like").Delete(&model.UserBehavior{}).Error; err != nil {
				return fmt.Errorf("failed to delete user behavior: %w", err)
			}
		}
		return nil
	})
}

// processCommentLikeDirectly 直接处理评论点赞（同步方式）
func (s *ImprovedLikeService) processCommentLikeDirectly(ctx context.Context, req *interactions.LikeActionRequest, liked bool) error {
	return s.database.Transaction(func(tx *gorm.DB) error {
		if liked {
			commentLike := &model.CommentLike{
				UserId:    req.UserId,
				CommentId: req.CommentId,
				CreatedAt: time.Now().Format(constants.DataFormate),
			}
			if err := tx.Create(commentLike).Error; err != nil {
				return fmt.Errorf("failed to create comment like: %w", err)
			}
		} else {
			if err := tx.Where("user_id = ? AND comment_id = ?", req.UserId, req.CommentId).Delete(&model.CommentLike{}).Error; err != nil {
				return fmt.Errorf("failed to delete comment like: %w", err)
			}
		}
		return nil
	})
}

// getUserVideoLikeStatusFromDB 从数据库获取用户视频点赞状态
func (s *ImprovedLikeService) getUserVideoLikeStatusFromDB(ctx context.Context, userID, videoID int64) (bool, error) {
	var count int64
	err := s.database.Model(&model.VideoLike{}).Where("user_id = ? AND video_id = ?", userID, videoID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// getUserCommentLikeStatusFromDB 从数据库获取用户评论点赞状态
func (s *ImprovedLikeService) getUserCommentLikeStatusFromDB(ctx context.Context, userID, commentID int64) (bool, error) {
	var count int64
	err := s.database.Model(&model.CommentLike{}).Where("user_id = ? AND comment_id = ?", userID, commentID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// validateLikeRequest 验证点赞请求
func (s *ImprovedLikeService) validateLikeRequest(req *interactions.LikeActionRequest) error {
	if req.UserId <= 0 {
		return fmt.Errorf("invalid user id")
	}

	if req.VideoId <= 0 && req.CommentId <= 0 {
		return fmt.Errorf("video id or comment id is required")
	}

	if req.VideoId > 0 && req.CommentId > 0 {
		return fmt.Errorf("cannot like both video and comment in one request")
	}

	if req.ActionType != "like" && req.ActionType != "unlike" {
		return fmt.Errorf("invalid action type: %s", req.ActionType)
	}

	return nil
}

// checkRateLimit 检查限流
func (s *ImprovedLikeService) checkRateLimit(ctx context.Context, userID int64) error {
	// 简化实现，实际应该使用Redis实现滑动窗口限流
	key := fmt.Sprintf("like_rate_limit:%d", userID)

	// 每分钟最多60次操作
	count, err := redis.GetCommentRateLimit(key)
	if err != nil {
		return nil // 限流检查失败不阻塞主流程
	}

	if count >= 60 {
		return fmt.Errorf("rate limit exceeded")
	}

	// 增加计数
	redis.IncrementCommentRateLimit(key, 60)
	return nil
}

// generateIdempotencyKey 生成幂等性键
func (s *ImprovedLikeService) generateIdempotencyKey(eventType string, userID, resourceID int64, actionType string) string {
	return fmt.Sprintf("%s_%d_%d_%s_%d", eventType, userID, resourceID, actionType, time.Now().Unix()/60) // 按分钟分组
}

// sendLikeNotification 发送点赞通知
func (s *ImprovedLikeService) sendLikeNotification(ctx context.Context, userID, resourceID int64, resourceType string) {
	notificationEvent := &mq.NotificationEvent{
		SenderID:         userID,
		Type:             "like",
		VideoID:          resourceID,
		CommentID:        resourceID,
		Content:          fmt.Sprintf("用户点赞了您的%s", resourceType),
		Timestamp:        time.Now().Unix(),
		EventID:          uuid.New().String(),
		NotificationType: "like",
		TargetID:         resourceID,
	}

	if err := s.producer.PublishNotificationEvent(ctx, notificationEvent); err != nil {
		hlog.CtxWarnf(ctx, "Failed to send like notification: %v", err)
	}
}

// GetLikeList 获取用户点赞列表（改进版）
func (s *ImprovedLikeService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	resp := &interactions.LikeListResponse{
		Base: &base.Status{},
	}

	// 参数验证
	if req.UserId <= 0 {
		resp.Base.Code = 400
		resp.Base.Msg = "invalid user id"
		return resp, errno.RequestErr
	}

	// 从数据库查询用户点赞的视频列表
	var videoLikes []model.VideoLike
	offset := (req.PageNum - 1) * req.PageSize

	err := s.database.Where("user_id = ?", req.UserId).
		Order("created_at DESC").
		Offset(int(offset)).
		Limit(int(req.PageSize)).
		Find(&videoLikes).Error

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get user like list: %v", err)
		resp.Base.Code = 500
		resp.Base.Msg = "获取点赞列表失败"
		return resp, err
	}

	// 提取视频ID列表
	videoIDs := make([]int64, len(videoLikes))
	for i, like := range videoLikes {
		videoIDs[i] = like.VideoId
	}

	resp.Base.Code = 200
	resp.Base.Msg = "获取成功"
	// 注意：LikeListResponse使用Items字段，需要转换为Video对象
	// 这里暂时返回空的Items，实际应该根据videoIDs获取Video对象
	resp.Items = []*base.Video{}

	return resp, nil
}

// GetVideoLikeCount 获取视频点赞数（改进版）
func (s *ImprovedLikeService) GetVideoLikeCount(ctx context.Context, videoID int64) (int64, error) {
	// 先从缓存获取
	count, version, err := s.cacheManager.GetVideoLikeCountWithConsistency(ctx, videoID)
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get video like count from cache: %v", err)
	} else if count > 0 || version > 0 {
		return count, nil
	}

	// 缓存未命中，从数据库查询
	var dbCount int64
	err = s.database.Model(&model.VideoLike{}).Where("video_id = ?", videoID).Count(&dbCount).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get video like count from database: %w", err)
	}

	// 更新缓存
	go func() {
		s.cacheManager.SetVideoLikeCountWithConsistency(context.Background(), videoID, dbCount, time.Now().UnixNano())
	}()

	return dbCount, nil
}

// GetCommentLikeCount 获取评论点赞数（改进版）
func (s *ImprovedLikeService) GetCommentLikeCount(ctx context.Context, commentID int64) (int64, error) {
	// 先从缓存获取
	count, version, err := s.cacheManager.GetCommentLikeCountWithConsistency(ctx, commentID)
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get comment like count from cache: %v", err)
	} else if count > 0 || version > 0 {
		return count, nil
	}

	// 缓存未命中，从数据库查询
	var dbCount int64
	err = s.database.Model(&model.CommentLike{}).Where("comment_id = ?", commentID).Count(&dbCount).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get comment like count from database: %w", err)
	}

	// 更新缓存
	go func() {
		s.cacheManager.SetCommentLikeCountWithConsistency(context.Background(), commentID, dbCount, time.Now().UnixNano())
	}()

	return dbCount, nil
}

// StartSyncService 启动同步服务
func (s *ImprovedLikeService) StartSyncService() error {
	return s.syncService.Start()
}

// StopSyncService 停止同步服务
func (s *ImprovedLikeService) StopSyncService() error {
	return s.syncService.Stop()
}

// HealthCheck 健康检查
func (s *ImprovedLikeService) HealthCheck(ctx context.Context) error {
	// 检查缓存连接
	if err := s.cacheManager.HealthCheck(ctx); err != nil {
		return fmt.Errorf("cache health check failed: %w", err)
	}

	// 检查数据库连接
	sqlDB, err := s.database.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
