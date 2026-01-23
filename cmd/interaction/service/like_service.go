package service

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// =============================================================================
// 点赞服务架构设计
// =============================================================================
//
// 设计原则：
// 1. 点赞是简单的 CRUD 操作，不需要消息队列
// 2. 数据一致性：先写数据库，成功后再更新缓存（Cache-Aside 模式）
// 3. 缓存失败不影响主流程，但会记录日志
//
// 数据流：
//   用户点赞 → 检查状态(缓存优先) → 写入数据库(事务) → 更新缓存 → 返回结果
//
// 数据存储：
// - MySQL video_likes 表：持久化存储，source of truth
// - Redis 缓存：
//   - user:likes:{user_id}:{business_type} (Set)：用户点赞列表
//   - count:{business_type}:{resource_id} (String)：点赞计数
//
// =============================================================================

// LikeActionService 点赞服务
type LikeActionService struct {
	ctx          context.Context
	cacheManager *redis.LikeCacheManager
}

// NewLikeActionService 创建点赞服务实例（不再依赖 MQ Producer）
func NewLikeActionService(ctx context.Context) *LikeActionService {
	cacheManager := redis.NewLikeCacheManager(redis.RedisDBInteraction)
	return &LikeActionService{
		ctx:          ctx,
		cacheManager: cacheManager,
	}
}

// =============================================================================
// 点赞操作
// =============================================================================

// LikeAction 处理点赞/取消点赞操作
func (s *LikeActionService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponse, error) {
	var isLiked bool
	var err error

	// 根据请求类型处理不同的点赞操作
	if req.VideoId != 0 {
		isLiked, err = s.handleVideoLike(ctx, req)
	} else if req.CommentId != 0 {
		isLiked, err = s.handleCommentLike(ctx, req)
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
func (s *LikeActionService) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	userID := req.UserId
	videoID := req.VideoId

	switch req.ActionType {
	case "like":
		return s.likeVideo(ctx, userID, videoID)
	case "unlike":
		return s.unlikeVideo(ctx, userID, videoID)
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeVideo 点赞视频
func (s *LikeActionService) likeVideo(ctx context.Context, userID, videoID int64) (bool, error) {
	// 直接写入数据库（DB 层会处理幂等逻辑）
	created, err := db.CreateVideoLike(ctx, userID, videoID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to create video like in DB: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, fmt.Errorf("failed to save like to database: %w", err)
	}

	// 只有真正创建了记录才更新缓存
	if created {
		s.updateCacheAfterLike(ctx, userID, videoID)
		hlog.CtxInfof(ctx, "Video like success: user_id=%d, video_id=%d", userID, videoID)
	}

	return true, nil
}

// unlikeVideo 取消点赞视频
func (s *LikeActionService) unlikeVideo(ctx context.Context, userID, videoID int64) (bool, error) {
	// 直接从数据库删除（DB 层会处理幂等逻辑）
	deleted, err := db.DeleteVideoLike(ctx, userID, videoID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to delete video like from DB: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, fmt.Errorf("failed to remove like from database: %w", err)
	}

	// 只有真正删除了记录才更新缓存
	if deleted {
		s.updateCacheAfterUnlike(ctx, userID, videoID)
		hlog.CtxInfof(ctx, "Video unlike success: user_id=%d, video_id=%d", userID, videoID)
	}

	return false, nil
}

// =============================================================================
// 缓存操作（辅助方法）
// =============================================================================

// updateCacheAfterLike 点赞后更新缓存
func (s *LikeActionService) updateCacheAfterLike(ctx context.Context, userID, videoID int64) {
	// 更新用户点赞列表
	if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
		hlog.CtxWarnf(ctx, "Failed to add user like to cache: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
	}

	// 更新点赞计数
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeVideo, videoID, 1); err != nil {
		hlog.CtxWarnf(ctx, "Failed to increment like count in cache: video_id=%d, err=%v", videoID, err)
	}
}

// updateCacheAfterUnlike 取消点赞后更新缓存
func (s *LikeActionService) updateCacheAfterUnlike(ctx context.Context, userID, videoID int64) {
	// 从用户点赞列表移除
	if err := s.cacheManager.RemoveUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
		hlog.CtxWarnf(ctx, "Failed to remove user like from cache: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
	}

	// 更新点赞计数
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeVideo, videoID, -1); err != nil {
		hlog.CtxWarnf(ctx, "Failed to decrement like count in cache: video_id=%d, err=%v", videoID, err)
	}
}

// checkVideoLikeStatus 检查用户是否点赞了视频
// 查询顺序：Redis缓存 → MySQL数据库（缓存miss时回填）
func (s *LikeActionService) checkVideoLikeStatus(ctx context.Context, userID, videoID int64) (bool, error) {
	// 1. 先查 Redis 缓存
	isLiked, err := s.cacheManager.IsVideoLikedByUser(ctx, userID, videoID)
	if err == nil && isLiked {
		return true, nil
	}

	// 2. 缓存中没找到，查数据库
	like, err := db.GetVideoLikeByUserAndVideo(ctx, userID, videoID)
	if err != nil {
		// 数据库查询出错或记录不存在，视为未点赞
		return false, nil
	}

	// 3. 数据库中有记录，回填缓存
	if like != nil {
		if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
			hlog.CtxWarnf(ctx, "Failed to backfill like cache: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		}
		return true, nil
	}

	return false, nil
}

// =============================================================================
// 评论点赞（简化实现，与视频点赞逻辑类似）
// =============================================================================

// handleCommentLike 处理评论点赞操作
func (s *LikeActionService) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	userID := req.UserId
	commentID := req.CommentId

	switch req.ActionType {
	case "like":
		return s.likeComment(ctx, userID, commentID)
	case "unlike":
		return s.unlikeComment(ctx, userID, commentID)
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeComment 点赞评论
func (s *LikeActionService) likeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	// 1. 检查是否已点赞
	isLiked, err := s.cacheManager.IsCommentLikedByUser(ctx, userID, commentID)
	if err == nil && isLiked {
		return true, nil // 已经点赞，幂等返回
	}

	// 2. 写入数据库
	if err := db.CreateCommentLike(ctx, userID, commentID); err != nil {
		hlog.CtxErrorf(ctx, "Failed to create comment like in DB: user_id=%d, comment_id=%d, err=%v", userID, commentID, err)
		return false, fmt.Errorf("failed to save comment like to database: %w", err)
	}

	// 3. 更新缓存
	if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeComment, commentID); err != nil {
		hlog.CtxWarnf(ctx, "Failed to add comment like to cache: err=%v", err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeComment, commentID, 1); err != nil {
		hlog.CtxWarnf(ctx, "Failed to increment comment like count: err=%v", err)
	}

	return true, nil
}

// unlikeComment 取消点赞评论
func (s *LikeActionService) unlikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	// 1. 检查是否已点赞
	isLiked, err := s.cacheManager.IsCommentLikedByUser(ctx, userID, commentID)
	if err != nil || !isLiked {
		return false, nil // 没有点赞，幂等返回
	}

	// 2. 从数据库删除
	if err := db.DeleteCommentLike(ctx, userID, commentID); err != nil {
		hlog.CtxErrorf(ctx, "Failed to delete comment like from DB: err=%v", err)
		return false, fmt.Errorf("failed to remove comment like from database: %w", err)
	}

	// 3. 更新缓存
	if err := s.cacheManager.RemoveUserLike(ctx, userID, redis.BusinessTypeComment, commentID); err != nil {
		hlog.CtxWarnf(ctx, "Failed to remove comment like from cache: err=%v", err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeComment, commentID, -1); err != nil {
		hlog.CtxWarnf(ctx, "Failed to decrement comment like count: err=%v", err)
	}

	return false, nil
}

// =============================================================================
// 查询接口
// =============================================================================

// GetLikeList 获取用户点赞的视频列表
func (s *LikeActionService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	// 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	offset := (req.PageNum - 1) * req.PageSize
	limit := req.PageSize

	// 获取用户点赞的视频ID列表（从缓存获取）
	videoIDs, err := s.cacheManager.GetUserLikeHistory(ctx, req.UserId, redis.BusinessTypeVideo, offset, limit)
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
		videoResp, err := client.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{
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

// GetLikeCount 获取点赞数
func (s *LikeActionService) GetLikeCount(ctx context.Context, businessID, messageID int64) (int64, error) {
	count, err := s.cacheManager.GetCountCache(ctx, businessID, messageID)
	if err != nil {
		// 缓存miss，从数据库查询
		if businessID == redis.BusinessTypeVideo {
			dbCount, dbErr := db.GetVideoLikeCount(ctx, messageID)
			if dbErr == nil {
				return dbCount, nil
			}
		}
		return 0, err
	}
	return count.LikeCount, nil
}

// BatchGetLikeCount 批量获取点赞数
func (s *LikeActionService) BatchGetLikeCount(ctx context.Context, businessID int64, messageIDs []int64) (map[int64]int64, error) {
	countMap, err := s.cacheManager.BatchGetCountCache(ctx, businessID, messageIDs)
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
func (s *LikeActionService) BatchCheckUserLikes(ctx context.Context, userID, businessID int64, messageIDs []int64) (map[int64]bool, error) {
	// 初始化 result map
	result := make(map[int64]bool)

	// 先从缓存批量查询
	cacheResult, err := s.cacheManager.BatchCheckUserLikes(ctx, userID, businessID, messageIDs)
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to batch check likes from cache, fallback to DB: %v", err)
	} else if cacheResult != nil {
		result = cacheResult
	}

	// 对于缓存中没有找到的，从数据库查询
	if businessID == redis.BusinessTypeVideo {
		missingIDs := make([]int64, 0)
		for _, id := range messageIDs {
			if _, ok := result[id]; !ok {
				missingIDs = append(missingIDs, id)
			}
		}

		if len(missingIDs) > 0 {
			dbResult, dbErr := db.BatchGetUserVideoLikeStatus(ctx, userID, missingIDs)
			if dbErr == nil {
				for id, isLiked := range dbResult {
					result[id] = isLiked
					// 回填缓存
					if isLiked {
						_ = s.cacheManager.AddUserLike(ctx, userID, businessID, id)
					}
				}
			}
		}
	}

	return result, nil
}

// GetUserLikeHistory 获取用户点赞历史
func (s *LikeActionService) GetUserLikeHistory(ctx context.Context, userID, businessID int64, offset, limit int64) ([]int64, error) {
	return s.cacheManager.GetUserLikeHistory(ctx, userID, businessID, offset, limit)
}
