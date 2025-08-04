package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/kitex_gen/videos"

	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// LikeActionService 新版点赞服务，使用优化后的Redis设计
type LikeActionService struct {
	ctx          context.Context
	cacheManager *redis.LikeCacheManager
	producer     *mq.Producer
}

// NewLikeActionService 创建新版点赞服务实例
func NewLikeActionService(ctx context.Context, producer *mq.Producer) *LikeActionService {
	cacheManager := redis.NewLikeCacheManager(redis.RedisDBInteraction)
	return &LikeActionService{
		ctx:          ctx,
		cacheManager: cacheManager,
		producer:     producer,
	}
}

// LikeAction 处理点赞/取消点赞操作
func (service *LikeActionService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponse, error) {
	var isLiked bool
	var err error

	// 根据请求类型处理不同的点赞操作
	if req.VideoId != 0 {
		isLiked, err = service.handleVideoLike(ctx, req)
	} else if req.CommentId != 0 {
		isLiked, err = service.handleCommentLike(ctx, req)
	} else {
		return nil, fmt.Errorf("invalid request: neither video_id nor comment_id provided")
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like action: %v", err)
		return nil, err
	}

	return &interactions.LikeActionResponse{
		Base: &base.Status{
			Code: 0,
			Msg:  "success",
		},
		IsLiked: isLiked,
	}, nil
}

// handleVideoLike 处理视频点赞操作
func (service *LikeActionService) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	switch req.ActionType {
	case "like":
		// 检查是否已经点赞
		isLiked, err := service.cacheManager.IsVideoLikedByUser(ctx, req.UserId, req.VideoId)
		if err != nil {
			return false, fmt.Errorf("failed to check like status: %w", err)
		}
		if isLiked {
			return true, nil // 已经点赞，直接返回
		}

		// 添加点赞记录到缓存
		if err := service.cacheManager.AddUserLike(ctx, req.UserId, redis.BusinessTypeVideo, req.VideoId); err != nil {
			return false, fmt.Errorf("failed to add like to cache: %w", err)
		}

		event := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    req.VideoId,
			CommentID:  req.CommentId,
			ActionType: req.ActionType,
			EventType:  "video_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}
		service.producer.PublishLikeEvent(ctx, event)

		return true, nil

	case "unlike":
		// 检查是否已经点赞
		isLiked, err := service.cacheManager.IsVideoLikedByUser(ctx, req.UserId, req.VideoId)
		if err != nil {
			return false, fmt.Errorf("failed to check like status: %w", err)
		}
		if !isLiked {
			return false, nil // 没有点赞，直接返回
		}

		// 从缓存移除点赞记录
		if err := service.cacheManager.RemoveUserLike(ctx, req.UserId, redis.BusinessTypeVideo, req.VideoId); err != nil {
			return false, fmt.Errorf("failed to remove like from cache: %w", err)
		}

		event := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    req.VideoId,
			CommentID:  req.CommentId,
			ActionType: req.ActionType,
			EventType:  "video_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}
		service.producer.PublishLikeEvent(ctx, event)
		return false, nil

	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// handleCommentLike 处理评论点赞操作
func (service *LikeActionService) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	switch req.ActionType {
	case "like":
		// 检查是否已经点赞
		isLiked, err := service.cacheManager.IsCommentLikedByUser(ctx, req.UserId, req.CommentId)
		if err != nil {
			return false, fmt.Errorf("failed to check like status: %w", err)
		}
		if isLiked {
			return true, nil // 已经点赞，直接返回
		}

		// 添加点赞记录到缓存
		if err := service.cacheManager.AddUserLike(ctx, req.UserId, redis.BusinessTypeComment, req.CommentId); err != nil {
			return false, fmt.Errorf("failed to add like to cache: %w", err)
		}

		// 发布评论点赞事件，由事件驱动同步服务处理数据库操作
		event := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    req.VideoId,
			CommentID:  req.CommentId,
			ActionType: req.ActionType,
			EventType:  "comment_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}
		service.producer.PublishLikeEvent(ctx, event)

		return true, nil

	case "unlike":
		// 检查是否已经点赞
		isLiked, err := service.cacheManager.IsCommentLikedByUser(ctx, req.UserId, req.CommentId)
		if err != nil {
			return false, fmt.Errorf("failed to check like status: %w", err)
		}
		if !isLiked {
			return false, nil // 没有点赞，直接返回
		}

		// 从缓存移除点赞记录
		if err := service.cacheManager.RemoveUserLike(ctx, req.UserId, redis.BusinessTypeComment, req.CommentId); err != nil {
			return false, fmt.Errorf("failed to remove like from cache: %w", err)
		}

		// 发布评论取消点赞事件，由事件驱动同步服务处理数据库操作
		event := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    req.VideoId,
			CommentID:  req.CommentId,
			ActionType: req.ActionType,
			EventType:  "comment_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}
		service.producer.PublishLikeEvent(ctx, event)
		return true, nil

	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// GetLikeList 获取用户点赞的视频列表
func (service *LikeActionService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	// 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	// 根据用户ID获取其点赞的视频列表
	offset := (req.PageNum - 1) * req.PageSize
	limit := req.PageSize

	// 获取用户点赞的视频ID列表
	videoIDs, err := service.cacheManager.GetUserLikeHistory(ctx, req.UserId, redis.BusinessTypeVideo, offset, limit)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get user like history: %v", err)
		return &interactions.LikeListResponse{
			Base: &base.Status{
				Code: 500,
				Msg:  "获取点赞列表失败",
			},
			Items: nil,
		}, err
	}

	// 如果没有点赞记录，直接返回空列表
	if len(videoIDs) == 0 {
		return &interactions.LikeListResponse{
			Base: &base.Status{
				Code: 0,
				Msg:  "success",
			},
			Items: []*base.Video{},
		}, nil
	}

	// 批量获取视频详细信息
	videosList := make([]*base.Video, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		// 调用视频服务获取视频详情
		videoResp, err := client.VideoClient.VideoInfo(ctx, &videos.VideoInfoRequest{
			VideoId: videoID,
		})
		if err != nil {
			hlog.CtxWarnf(ctx, "Failed to get video info for ID %d: %v", videoID, err)
			continue
		}

		if videoResp != nil && videoResp.Items != nil {
			videosList = append(videosList, videoResp.Items)
		}
	}

	return &interactions.LikeListResponse{
		Base: &base.Status{
			Code: 0,
			Msg:  "success",
		},
		Items: videosList,
	}, nil
}

// GetUserLikeHistory 获取用户点赞历史
func (service *LikeActionService) GetUserLikeHistory(ctx context.Context, userID, businessID int64, offset, limit int64) ([]int64, error) {
	return service.cacheManager.GetUserLikeHistory(ctx, userID, businessID, offset, limit)
}

// GetLikeCount 获取点赞数
func (service *LikeActionService) GetLikeCount(ctx context.Context, businessID, messageID int64) (int64, error) {
	count, err := service.cacheManager.GetCountCache(ctx, businessID, messageID)
	if err != nil {
		return 0, err
	}
	return count.LikeCount, nil
}

// BatchGetLikeCount 批量获取点赞数
func (service *LikeActionService) BatchGetLikeCount(ctx context.Context, businessID int64, messageIDs []int64) (map[int64]int64, error) {
	countMap, err := service.cacheManager.BatchGetCountCache(ctx, businessID, messageIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int64)
	for messageID, count := range countMap {
		result[messageID] = count.LikeCount
	}

	return result, nil
}

// BatchCheckUserLikes 批量检查用户点赞状态
func (service *LikeActionService) BatchCheckUserLikes(ctx context.Context, userID, businessID int64, messageIDs []int64) (map[int64]bool, error) {
	return service.cacheManager.BatchCheckUserLikes(ctx, userID, businessID, messageIDs)
}

// === 异步数据库操作 ===

// saveLikeToDB 异步保存点赞记录到数据库（已弃用 - 由事件驱动同步处理）
func (service *LikeActionService) saveLikeToDB(userID, videoID int64, behaviorType string) {
	// 注意：此方法已弃用，现在由EventDrivenSyncService统一处理user_behaviors表的插入
	// 避免重复插入问题
	hlog.Infof("saveLikeToDB called but handled by EventDrivenSyncService - user_id: %d, video_id: %d, type: %s",
		userID, videoID, behaviorType)
}

// deleteLikeFromDB 异步从数据库删除点赞记录（已弃用 - 由事件驱动同步处理）
func (service *LikeActionService) deleteLikeFromDB(userID, videoID int64, behaviorType string) {
	// 注意：此方法已弃用，现在由EventDrivenSyncService统一处理user_behaviors表的删除
	// 避免重复操作问题
	hlog.Infof("deleteLikeFromDB called but handled by EventDrivenSyncService - user_id: %d, video_id: %d, type: %s",
		userID, videoID, behaviorType)
}

// saveCommentLikeToDB 异步保存评论点赞记录到数据库（已弃用 - 由事件驱动同步处理）
func (service *LikeActionService) saveCommentLikeToDB(userID, commentID int64, behaviorType string) {
	// 注意：此方法已弃用，现在由EventDrivenSyncService统一处理user_behaviors表的插入
	// 避免重复插入问题
	hlog.Infof("saveCommentLikeToDB called but handled by EventDrivenSyncService - user_id: %d, comment_id: %d, type: %s",
		userID, commentID, behaviorType)
}

// deleteCommentLikeFromDB 异步从数据库删除评论点赞记录（已弃用 - 由事件驱动同步处理）
func (service *LikeActionService) deleteCommentLikeFromDB(userID, commentID int64, behaviorType string) {
	// 注意：此方法已弃用，现在由EventDrivenSyncService统一处理user_behaviors表的删除
	// 避免重复操作问题
	hlog.Infof("deleteCommentLikeFromDB called but handled by EventDrivenSyncService - user_id: %d, comment_id: %d, type: %s",
		userID, commentID, behaviorType)
}

// SyncCacheWithDB 同步缓存与数据库数据
func (service *LikeActionService) SyncCacheWithDB(ctx context.Context, businessID, messageID int64) error {
	// 从数据库获取真实的点赞数据
	// 更新缓存中的计数和用户列表

	// 这里需要根据实际的数据库结构来实现
	hlog.CtxInfof(ctx, "Syncing cache with DB for business:%d, message:%d", businessID, messageID)

	return nil
}

func (service *LikeActionService) SendLikeNotification(ctx context.Context, fromUserID, targetID int64, targetType string) {
	var toUserID int64
	var content string

	if targetType == "video" {
		video, err := db.GetVideoInfo(ctx, targetID)
		if err != nil {
			hlog.Errorf("Failed to get video info for notification: %v", err)
			return
		}

		toUserID = video.UserId
		userName, err := client.GetUserInfo(ctx, &users.GetUserInfoRequest{
			UserId: fromUserID,
		})
		if err != nil {
			hlog.Errorf("Failed to get userinfo for notification: %v", err)
			return
		}

		content = fmt.Sprintf("%v 赞了你的视频", userName)
	} else if targetType == "comment" {
		comment, err := db.GetCommentInfo(ctx, targetID)
		if err != nil {
			hlog.Errorf("Failed to get comment info for notification: %v", err)
			return
		}
		toUserID = comment.UserId
		userName, err := client.GetUserInfo(ctx, &users.GetUserInfoRequest{
			UserId: fromUserID,
		})
		if err != nil {
			hlog.Errorf("Failed to get userinfo for notification: %v", err)
			return
		}

		content = fmt.Sprintf("%v 赞了你的评论", userName)
	}

	if fromUserID == toUserID {
		return
	}

	notificationEvent := &mq.NotificationEvent{
		UserID:           toUserID,
		FromUserID:       fromUserID,
		NotificationType: "like",
		TargetID:         targetID,
		Content:          content,
		Timestamp:        time.Now().Unix(),
		EventID:          uuid.New().String(),
	}

	if err := service.producer.PublishNotificationEvent(ctx, notificationEvent); err != nil {
		hlog.Errorf("Failed to publish notification event: %v", err)
	}
}
