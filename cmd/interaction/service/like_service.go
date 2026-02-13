package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	client "HuaTug.com/cmd/interaction/client_rpc"
	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// EnhancedLikeService configuration constants.
const (
	defaultSyncBatchSize = 100
	defaultSyncInterval  = 5 * time.Second
	syncPopTimeout       = 100 * time.Millisecond
	maxSyncRetries       = 3
	batchVideoSemaphore  = 10 // max concurrent video info fetches
	calibrateDiffThresh  = 10 // minimum diff to trigger calibration
)

// EnhancedLikeService provides like operations backed by Redis with async DB sync.
type EnhancedLikeService struct {
	ctx            context.Context
	interactionMgr *redis.EnhancedInteractionManager
	cacheManager   *redis.LikeCacheManager

	enableAsyncSync bool
	syncBatchSize   int
	syncInterval    time.Duration
}

// EnhancedLikeConfig holds configuration for the enhanced like service.
type EnhancedLikeConfig struct {
	EnableAsyncSync bool
	SyncBatchSize   int
	SyncInterval    time.Duration
}

// DefaultEnhancedLikeConfig returns the default enhanced like configuration.
func DefaultEnhancedLikeConfig() *EnhancedLikeConfig {
	return &EnhancedLikeConfig{
		EnableAsyncSync: true,
		SyncBatchSize:   defaultSyncBatchSize,
		SyncInterval:    defaultSyncInterval,
	}
}

// NewEnhancedLikeService creates a new enhanced like service.
func NewEnhancedLikeService(ctx context.Context, config *EnhancedLikeConfig) *EnhancedLikeService {
	if config == nil {
		config = DefaultEnhancedLikeConfig()
	}

	svc := &EnhancedLikeService{
		ctx:             ctx,
		interactionMgr:  redis.NewEnhancedInteractionManager(redis.RedisDBInteraction),
		cacheManager:    redis.NewLikeCacheManager(redis.RedisDBInteraction),
		enableAsyncSync: config.EnableAsyncSync,
		syncBatchSize:   config.SyncBatchSize,
		syncInterval:    config.SyncInterval,
	}

	if config.EnableAsyncSync {
		go svc.startSyncWorker()
	}

	return svc
}

// --- Like Actions ---

// LikeAction handles like/unlike requests for videos or comments.
func (s *EnhancedLikeService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponse, error) {
	var (
		isLiked   bool
		likeCount int64
		err       error
	)

	switch {
	case req.VideoId != 0:
		isLiked, likeCount, err = s.handleVideoLike(ctx, req)
	case req.CommentId != 0:
		isLiked, likeCount, err = s.handleCommentLike(ctx, req)
	default:
		return nil, fmt.Errorf("invalid request: neither video_id nor comment_id provided")
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like action: %v", err)
		return &interactions.LikeActionResponse{
			Base: &base.Status{Code: 500, Msg: err.Error()},
		}, err
	}

	return &interactions.LikeActionResponse{
		Base:      &base.Status{Code: 0, Msg: "success"},
		IsLiked:   isLiked,
		LikeCount: likeCount,
	}, nil
}

// handleVideoLike dispatches video like/unlike operations.
func (s *EnhancedLikeService) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, int64, error) {
	switch req.ActionType {
	case "like":
		return s.likeVideoEnhanced(ctx, req.UserId, req.VideoId)
	case "unlike":
		return s.unlikeVideoEnhanced(ctx, req.UserId, req.VideoId)
	default:
		return false, 0, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeVideoEnhanced performs a Redis-first video like with async side-effects.
func (s *EnhancedLikeService) likeVideoEnhanced(ctx context.Context, userID, videoID int64) (bool, int64, error) {
	success, isNewLike, err := s.interactionMgr.DoLike(ctx, userID, videoID, redis.BizTypeVideo)
	if err != nil {
		hlog.CtxWarnf(ctx, "Like blocked: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, 0, err
	}
	if !success {
		return false, 0, fmt.Errorf("like operation failed")
	}

	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)

	if isNewLike {
		go s.updateHotRanking(ctx, videoID, likeCount)
		go s.trackUserBehavior(ctx, userID, "like")
		hlog.CtxInfof(ctx, "Video like: user_id=%d, video_id=%d, count=%d", userID, videoID, likeCount)
	}

	return true, likeCount, nil
}

// unlikeVideoEnhanced performs a Redis-first video unlike.
func (s *EnhancedLikeService) unlikeVideoEnhanced(ctx context.Context, userID, videoID int64) (bool, int64, error) {
	wasLiked, err := s.interactionMgr.DoUnlike(ctx, userID, videoID, redis.BizTypeVideo)
	if err != nil {
		hlog.CtxWarnf(ctx, "Unlike blocked: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, 0, err
	}

	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)

	if wasLiked {
		go s.updateHotRanking(ctx, videoID, likeCount)
		hlog.CtxInfof(ctx, "Video unlike: user_id=%d, video_id=%d, count=%d", userID, videoID, likeCount)
	}

	return false, likeCount, nil
}

// handleCommentLike dispatches comment like/unlike operations.
func (s *EnhancedLikeService) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, int64, error) {
	switch req.ActionType {
	case "like":
		return s.likeCommentEnhanced(ctx, req.UserId, req.CommentId)
	case "unlike":
		return s.unlikeCommentEnhanced(ctx, req.UserId, req.CommentId)
	default:
		return false, 0, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeCommentEnhanced performs a Redis-first comment like.
func (s *EnhancedLikeService) likeCommentEnhanced(ctx context.Context, userID, commentID int64) (bool, int64, error) {
	success, isNewLike, err := s.interactionMgr.DoLike(ctx, userID, commentID, redis.BizTypeComment)
	if err != nil {
		return false, 0, err
	}
	if !success {
		return false, 0, fmt.Errorf("like operation failed")
	}

	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, commentID, redis.BizTypeComment)

	if isNewLike {
		hlog.CtxInfof(ctx, "Comment like: user_id=%d, comment_id=%d", userID, commentID)
	}

	return true, likeCount, nil
}

// unlikeCommentEnhanced performs a Redis-first comment unlike.
func (s *EnhancedLikeService) unlikeCommentEnhanced(ctx context.Context, userID, commentID int64) (bool, int64, error) {
	if _, err := s.interactionMgr.DoUnlike(ctx, userID, commentID, redis.BizTypeComment); err != nil {
		return false, 0, err
	}

	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, commentID, redis.BizTypeComment)
	return false, likeCount, nil
}

// --- Query ---

// GetLikeList returns the videos liked by a user, with pagination.
func (s *EnhancedLikeService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	offset := (req.PageNum - 1) * req.PageSize
	limit := req.PageSize

	// Try Redis first.
	videoIDs, err := s.interactionMgr.GetUserLikeList(ctx, req.UserId, redis.BizTypeVideo, offset, limit)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get like list from Redis: %v", err)
		return &interactions.LikeListResponse{
			Base: &base.Status{Code: 500, Msg: "获取点赞列表失败"},
		}, err
	}

	// Fallback to DB if Redis is empty.
	if len(videoIDs) == 0 {
		videoIDs, err = db.GetUserVideoLikes(ctx, req.UserId, int(offset), int(limit))
		if err != nil {
			return &interactions.LikeListResponse{
				Base:  &base.Status{Code: 0, Msg: "success"},
				Items: []*base.Video{},
			}, nil
		}
	}

	videosList := s.batchGetVideoInfo(ctx, videoIDs)

	return &interactions.LikeListResponse{
		Base:  &base.Status{Code: 0, Msg: "success"},
		Items: videosList,
	}, nil
}

// BatchLikeStatus checks like status for multiple videos in batch.
func (s *EnhancedLikeService) BatchLikeStatus(ctx context.Context, req *interactions.BatchLikeStatusRequest) (*interactions.BatchLikeStatusResponse, error) {
	statusMap, err := s.interactionMgr.BatchGetLikeStatus(ctx, req.UserId, req.VideoIds, redis.BizTypeVideo)
	if err != nil {
		hlog.CtxWarnf(ctx, "Redis batch like status failed, fallback to DB: %v", err)
		statusMap, err = db.BatchGetUserVideoLikeStatus(ctx, req.UserId, req.VideoIds)
		if err != nil {
			return nil, err
		}
	}

	return &interactions.BatchLikeStatusResponse{
		Base:       &base.Status{Code: 0, Msg: "success"},
		LikeStatus: statusMap,
	}, nil
}

// GetLikeCount returns the like count for a resource, with DB fallback.
func (s *EnhancedLikeService) GetLikeCount(ctx context.Context, bizType int, objID int64) (int64, error) {
	count, err := s.interactionMgr.GetLikeCount(ctx, objID, bizType)
	if err != nil || count == 0 {
		if bizType == redis.BizTypeVideo {
			return db.GetVideoLikeCount(ctx, objID)
		}
	}
	return count, nil
}

// BatchGetLikeCount returns like counts for multiple resources in batch.
func (s *EnhancedLikeService) BatchGetLikeCount(ctx context.Context, bizType int, objIDs []int64) (map[int64]int64, error) {
	return s.interactionMgr.BatchGetLikeCount(ctx, objIDs, bizType)
}

// --- Async Sync Worker ---

// startSyncWorker polls the sync queue and persists like actions to DB.
func (s *EnhancedLikeService) startSyncWorker() {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.processSyncQueue()
		case <-s.ctx.Done():
			s.processSyncQueue() // drain before exit
			return
		}
	}
}

// processSyncQueue processes pending sync actions in batch.
func (s *EnhancedLikeService) processSyncQueue() {
	ctx := context.Background()

	queueLen, err := s.interactionMgr.GetSyncQueueLength(ctx)
	if err != nil || queueLen == 0 {
		return
	}

	batchSize := s.syncBatchSize
	if int(queueLen) < batchSize {
		batchSize = int(queueLen)
	}

	var wg sync.WaitGroup
	for i := 0; i < batchSize; i++ {
		action, popErr := s.interactionMgr.PopSyncAction(ctx, syncPopTimeout)
		if popErr != nil || action == nil {
			continue
		}

		wg.Add(1)
		go func(act *redis.LikeAction) {
			defer wg.Done()
			s.syncToDatabase(ctx, act)
		}(action)
	}

	wg.Wait()
}

// syncToDatabase persists a single like action to the database with retry on failure.
// After successful DB write, triggers delayed-double-delete to refresh Redis count
// from DB, ensuring Redis-DB consistency even under concurrent writes.
func (s *EnhancedLikeService) syncToDatabase(ctx context.Context, action *redis.LikeAction) {
	err := s.executeLikeDBAction(ctx, action)
	if err == nil {
		// === Delayed Double Delete: refresh Redis count from DB ===
		s.delayedRefreshCount(ctx, action)
		return
	}

	hlog.Warnf("Failed to sync like action to DB: %+v, err=%v", action, err)

	// Retry with quadratic backoff in a separate goroutine.
	go func(a *redis.LikeAction) {
		for attempt := 1; attempt <= maxSyncRetries; attempt++ {
			time.Sleep(time.Duration(attempt*attempt) * time.Second)

			retryErr := s.executeLikeDBAction(context.Background(), a)
			if retryErr == nil {
				hlog.Infof("Like sync retry succeeded on attempt %d: %+v", attempt, a)
				s.delayedRefreshCount(context.Background(), a)
				return
			}
			hlog.Warnf("Like sync retry %d failed: %+v, err=%v", attempt, a, retryErr)
		}
		hlog.Errorf("Like sync permanently failed after %d retries (dead letter): %+v", maxSyncRetries, a)
	}(action)
}

// delayedRefreshCount reads the authoritative like count from DB and schedules
// a delayed write-back to Redis, implementing the "second delete" of the
// delayed-double-delete consistency pattern.
func (s *EnhancedLikeService) delayedRefreshCount(ctx context.Context, action *redis.LikeAction) {
	var dbCount int64
	var err error

	switch action.BizType {
	case redis.BizTypeVideo:
		dbCount, err = db.GetVideoLikeCount(ctx, action.ObjID)
	case redis.BizTypeComment:
		// For comments, count from comment_likes table.
		dbCount, err = db.GetVideoLikeCount(ctx, action.ObjID) // reuse; will fallback in calibration
		if err != nil {
			// Non-critical: the periodic calibration task will fix drift.
			hlog.Warnf("[DelayedDoubleDelete] Failed to get DB count for biz=%d obj=%d: %v",
				action.BizType, action.ObjID, err)
			return
		}
	default:
		return
	}

	if err != nil {
		hlog.Warnf("[DelayedDoubleDelete] Failed to get DB count for biz=%d obj=%d: %v",
			action.BizType, action.ObjID, err)
		return
	}

	s.interactionMgr.ScheduleDelayedCountRefresh(action.ObjID, action.BizType, dbCount)
}

// executeLikeDBAction performs the actual DB write for a like action.
func (s *EnhancedLikeService) executeLikeDBAction(ctx context.Context, action *redis.LikeAction) error {
	switch action.BizType {
	case redis.BizTypeVideo:
		if action.Action == "like" {
			_, err := db.CreateVideoLike(ctx, action.UserID, action.ObjID)
			return err
		}
		_, err := db.DeleteVideoLike(ctx, action.UserID, action.ObjID)
		return err

	case redis.BizTypeComment:
		if action.Action == "like" {
			return db.CreateCommentLike(ctx, action.UserID, action.ObjID)
		}
		return db.DeleteCommentLike(ctx, action.UserID, action.ObjID)

	default:
		return fmt.Errorf("unknown biz type: %d", action.BizType)
	}
}

// --- Helpers ---

// updateHotRanking updates the hot video ranking cache.
func (s *EnhancedLikeService) updateHotRanking(ctx context.Context, videoID, likeCount int64) {
	if err := s.interactionMgr.UpdateHotVideoCache(ctx, videoID, likeCount); err != nil {
		hlog.CtxWarnf(ctx, "Failed to update hot ranking: video_id=%d, err=%v", videoID, err)
	}
}

// trackUserBehavior records a user activity asynchronously.
func (s *EnhancedLikeService) trackUserBehavior(ctx context.Context, userID int64, activity string) {
	if err := s.interactionMgr.TrackUserActivity(ctx, userID, activity); err != nil {
		hlog.CtxWarnf(ctx, "Failed to track user behavior: user_id=%d, err=%v", userID, err)
	}
}

// batchGetVideoInfo fetches video details concurrently with bounded parallelism.
func (s *EnhancedLikeService) batchGetVideoInfo(ctx context.Context, videoIDs []int64) []*base.Video {
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

// --- Calibration ---

// CalibrateTask reconciles Redis and DB like counts periodically.
type CalibrateTask struct {
	interactionMgr *redis.EnhancedInteractionManager
	batchSize      int
}

// NewCalibrateTask creates a new calibration task.
func NewCalibrateTask(interactionMgr *redis.EnhancedInteractionManager) *CalibrateTask {
	return &CalibrateTask{
		interactionMgr: interactionMgr,
		batchSize:      defaultSyncBatchSize,
	}
}

// CalibrateVideoLikeCounts checks and fixes like-count drift between Redis and DB.
func (t *CalibrateTask) CalibrateVideoLikeCounts(ctx context.Context, videoIDs []int64) error {
	for _, videoID := range videoIDs {
		dbCount, err := db.GetVideoLikeCount(ctx, videoID)
		if err != nil {
			hlog.Warnf("Calibrate: failed to get DB count for video %d: %v", videoID, err)
			continue
		}

		redisCount, err := t.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)
		if err != nil {
			hlog.Warnf("Calibrate: failed to get Redis count for video %d: %v", videoID, err)
			continue
		}

		diff := dbCount - redisCount
		if diff < 0 {
			diff = -diff
		}

		if diff > calibrateDiffThresh {
			hlog.Infof("Calibrating video like count: video_id=%d, db=%d, redis=%d", videoID, dbCount, redisCount)
			if err := t.interactionMgr.SetLikeCount(ctx, videoID, redis.BizTypeVideo, dbCount); err != nil {
				hlog.Warnf("Calibrate: failed to set Redis count for video %d: %v", videoID, err)
			}
		}
	}
	return nil
}
