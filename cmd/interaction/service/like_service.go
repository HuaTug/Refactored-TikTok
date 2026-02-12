package service

import (
	"context"
	"fmt"
	"sync"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// LikeActionService implements the Cache-Aside pattern: write DB first, then update cache.
type LikeActionService struct {
	ctx          context.Context
	cacheManager *redis.LikeCacheManager
}

// NewLikeActionService creates a like service instance.
func NewLikeActionService(ctx context.Context) *LikeActionService {
	return &LikeActionService{
		ctx:          ctx,
		cacheManager: redis.NewLikeCacheManager(redis.RedisDBInteraction),
	}
}

// --- Like/Unlike ---

// LikeAction handles like/unlike for videos or comments.
func (s *LikeActionService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponse, error) {
	var (
		isLiked bool
		err     error
	)

	switch {
	case req.VideoId != 0:
		isLiked, err = s.handleVideoLike(ctx, req)
	case req.CommentId != 0:
		isLiked, err = s.handleCommentLike(ctx, req)
	default:
		return nil, fmt.Errorf("invalid request: neither video_id nor comment_id provided")
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like action: %v", err)
		return nil, err
	}

	return &interactions.LikeActionResponse{
		Base:    &base.Status{Code: 0, Msg: "success"},
		IsLiked: isLiked,
	}, nil
}

// handleVideoLike dispatches video like/unlike.
func (s *LikeActionService) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	switch req.ActionType {
	case "like":
		return s.likeVideo(ctx, req.UserId, req.VideoId)
	case "unlike":
		return s.unlikeVideo(ctx, req.UserId, req.VideoId)
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeVideo creates a video like record in DB, then updates cache.
func (s *LikeActionService) likeVideo(ctx context.Context, userID, videoID int64) (bool, error) {
	created, err := db.CreateVideoLike(ctx, userID, videoID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to create video like: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, fmt.Errorf("failed to save like: %w", err)
	}

	if created {
		s.updateCacheAfterLike(ctx, userID, videoID)
		hlog.CtxInfof(ctx, "Video like: user_id=%d, video_id=%d", userID, videoID)
	}

	return true, nil
}

// unlikeVideo soft-deletes a video like record in DB, then updates cache.
func (s *LikeActionService) unlikeVideo(ctx context.Context, userID, videoID int64) (bool, error) {
	deleted, err := db.DeleteVideoLike(ctx, userID, videoID)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to delete video like: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, fmt.Errorf("failed to remove like: %w", err)
	}

	if deleted {
		s.updateCacheAfterUnlike(ctx, userID, videoID)
		hlog.CtxInfof(ctx, "Video unlike: user_id=%d, video_id=%d", userID, videoID)
	}

	return false, nil
}

// --- Cache Helpers ---

// updateCacheAfterLike adds user like to cache and increments count.
func (s *LikeActionService) updateCacheAfterLike(ctx context.Context, userID, videoID int64) {
	if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
		hlog.CtxWarnf(ctx, "Cache: add user like failed: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeVideo, videoID, 1); err != nil {
		hlog.CtxWarnf(ctx, "Cache: increment like count failed: video_id=%d, err=%v", videoID, err)
	}
}

// updateCacheAfterUnlike removes user like from cache and decrements count.
func (s *LikeActionService) updateCacheAfterUnlike(ctx context.Context, userID, videoID int64) {
	if err := s.cacheManager.RemoveUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
		hlog.CtxWarnf(ctx, "Cache: remove user like failed: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeVideo, videoID, -1); err != nil {
		hlog.CtxWarnf(ctx, "Cache: decrement like count failed: video_id=%d, err=%v", videoID, err)
	}
}

// checkVideoLikeStatus checks if a user has liked a video (cache → DB → backfill).
func (s *LikeActionService) checkVideoLikeStatus(ctx context.Context, userID, videoID int64) (bool, error) {
	isLiked, err := s.cacheManager.IsVideoLikedByUser(ctx, userID, videoID)
	if err == nil && isLiked {
		return true, nil
	}

	like, err := db.GetVideoLikeByUserAndVideo(ctx, userID, videoID)
	if err != nil {
		return false, nil
	}
	if like == nil {
		return false, nil
	}

	// Backfill cache.
	if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeVideo, videoID); err != nil {
		hlog.CtxWarnf(ctx, "Cache backfill failed: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
	}
	return true, nil
}

// --- Comment Like ---

// handleCommentLike dispatches comment like/unlike.
func (s *LikeActionService) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, error) {
	switch req.ActionType {
	case "like":
		return s.likeComment(ctx, req.UserId, req.CommentId)
	case "unlike":
		return s.unlikeComment(ctx, req.UserId, req.CommentId)
	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeComment creates a comment like (idempotent).
func (s *LikeActionService) likeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	isLiked, err := s.cacheManager.IsCommentLikedByUser(ctx, userID, commentID)
	if err == nil && isLiked {
		return true, nil // already liked
	}

	if err := db.CreateCommentLike(ctx, userID, commentID); err != nil {
		hlog.CtxErrorf(ctx, "Failed to create comment like: user_id=%d, comment_id=%d, err=%v", userID, commentID, err)
		return false, fmt.Errorf("failed to save comment like: %w", err)
	}

	if err := s.cacheManager.AddUserLike(ctx, userID, redis.BusinessTypeComment, commentID); err != nil {
		hlog.CtxWarnf(ctx, "Cache: add comment like failed: err=%v", err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeComment, commentID, 1); err != nil {
		hlog.CtxWarnf(ctx, "Cache: increment comment like count failed: err=%v", err)
	}

	return true, nil
}

// unlikeComment deletes a comment like (idempotent).
func (s *LikeActionService) unlikeComment(ctx context.Context, userID, commentID int64) (bool, error) {
	isLiked, err := s.cacheManager.IsCommentLikedByUser(ctx, userID, commentID)
	if err != nil || !isLiked {
		return false, nil // not liked
	}

	if err := db.DeleteCommentLike(ctx, userID, commentID); err != nil {
		hlog.CtxErrorf(ctx, "Failed to delete comment like: err=%v", err)
		return false, fmt.Errorf("failed to remove comment like: %w", err)
	}

	if err := s.cacheManager.RemoveUserLike(ctx, userID, redis.BusinessTypeComment, commentID); err != nil {
		hlog.CtxWarnf(ctx, "Cache: remove comment like failed: err=%v", err)
	}
	if err := s.cacheManager.IncrementLikeCount(ctx, redis.BusinessTypeComment, commentID, -1); err != nil {
		hlog.CtxWarnf(ctx, "Cache: decrement comment like count failed: err=%v", err)
	}

	return false, nil
}

// --- Query ---

// GetLikeList returns the user's liked videos with pagination.
func (s *LikeActionService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	offset := (req.PageNum - 1) * req.PageSize
	limit := req.PageSize

	videoIDs, err := s.cacheManager.GetUserLikeHistory(ctx, req.UserId, redis.BusinessTypeVideo, offset, limit)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get user like history: %v", err)
		return &interactions.LikeListResponse{
			Base:  &base.Status{Code: 500, Msg: "获取点赞列表失败"},
			Items: nil,
		}, err
	}

	if len(videoIDs) == 0 {
		return &interactions.LikeListResponse{
			Base:  &base.Status{Code: 0, Msg: "success"},
			Items: []*base.Video{},
		}, nil
	}

	// Fetch video info concurrently (bounded parallelism).
	videosList := s.batchGetVideoInfo(ctx, videoIDs)

	return &interactions.LikeListResponse{
		Base:  &base.Status{Code: 0, Msg: "success"},
		Items: videosList,
	}, nil
}

// batchGetVideoInfo fetches video details concurrently.
func (s *LikeActionService) batchGetVideoInfo(ctx context.Context, videoIDs []int64) []*base.Video {
	if len(videoIDs) == 0 {
		return []*base.Video{}
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		videoList = make([]*base.Video, 0, len(videoIDs))
	)

	semaphore := make(chan struct{}, batchVideoSemaphore)

	for _, vid := range videoIDs {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(id int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			resp, err := client.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{VideoId: id})
			if err != nil {
				hlog.CtxWarnf(ctx, "Failed to get video info: video_id=%d, err=%v", id, err)
				return
			}
			if resp != nil && resp.Items != nil {
				mu.Lock()
				videoList = append(videoList, resp.Items)
				mu.Unlock()
			}
		}(vid)
	}

	wg.Wait()
	return videoList
}

// GetLikeCount returns the like count for a resource, with DB fallback.
func (s *LikeActionService) GetLikeCount(ctx context.Context, businessID, messageID int64) (int64, error) {
	count, err := s.cacheManager.GetCountCache(ctx, businessID, messageID)
	if err != nil {
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

// BatchGetLikeCount returns like counts for multiple resources.
func (s *LikeActionService) BatchGetLikeCount(ctx context.Context, businessID int64, messageIDs []int64) (map[int64]int64, error) {
	countMap, err := s.cacheManager.BatchGetCountCache(ctx, businessID, messageIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int64, len(countMap))
	for messageID, count := range countMap {
		result[messageID] = count.LikeCount
	}
	return result, nil
}

// BatchCheckUserLikes checks like status for multiple resources (cache → DB backfill).
func (s *LikeActionService) BatchCheckUserLikes(ctx context.Context, userID, businessID int64, messageIDs []int64) (map[int64]bool, error) {
	result := make(map[int64]bool, len(messageIDs))

	cacheResult, err := s.cacheManager.BatchCheckUserLikes(ctx, userID, businessID, messageIDs)
	if err != nil {
		hlog.CtxWarnf(ctx, "Batch check likes from cache failed, fallback to DB: %v", err)
	} else if cacheResult != nil {
		result = cacheResult
	}

	// Find IDs not in cache and query DB.
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
					if isLiked {
						_ = s.cacheManager.AddUserLike(ctx, userID, businessID, id)
					}
				}
			}
		}
	}

	return result, nil
}

// GetUserLikeHistory returns the user's like history IDs.
func (s *LikeActionService) GetUserLikeHistory(ctx context.Context, userID, businessID int64, offset, limit int64) ([]int64, error) {
	return s.cacheManager.GetUserLikeHistory(ctx, userID, businessID, offset, limit)
}
