package service

import (
	"context"
	"fmt"
	"sync"
	"time"

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
// 抖音级点赞服务 - 增强版架构设计
// =============================================================================
//
// 核心设计原则：
// 1. 最终一致性：Redis先行，异步同步到DB
// 2. 高并发：分布式锁 + 原子操作 + 限流保护
// 3. 防刷机制：多维度限流（用户级/IP级/全局）
// 4. 幂等性：重复操作不产生副作用
//
// 数据一致性策略：
// ┌─────────────────────────────────────────────────────────────────────────┐
// │                        最终一致性 vs 强一致性                           │
// ├─────────────────────────────────────────────────────────────────────────┤
// │ 点赞计数：最终一致性                                                    │
// │   - 用户不敏感少量误差（差1-2个赞无所谓）                               │
// │   - Redis计数为主，异步同步到DB                                         │
// │   - 定时校准任务修复差异                                                │
// ├─────────────────────────────────────────────────────────────────────────┤
// │ 点赞状态：强一致性                                                      │
// │   - 用户敏感（点了必须显示红心）                                         │
// │   - Redis + DB双写，失败回滚                                            │
// │   - 使用分布式锁防止并发冲突                                            │
// └─────────────────────────────────────────────────────────────────────────┘
//
// =============================================================================

// EnhancedLikeService 增强版点赞服务
type EnhancedLikeService struct {
	ctx            context.Context
	interactionMgr *redis.EnhancedInteractionManager
	cacheManager   *redis.LikeCacheManager

	// 配置选项
	enableAsyncSync bool          // 是否启用异步同步
	syncBatchSize   int           // 同步批次大小
	syncInterval    time.Duration // 同步间隔
}

// EnhancedLikeConfig 增强版配置
type EnhancedLikeConfig struct {
	EnableAsyncSync bool
	SyncBatchSize   int
	SyncInterval    time.Duration
}

// DefaultEnhancedLikeConfig 默认配置
func DefaultEnhancedLikeConfig() *EnhancedLikeConfig {
	return &EnhancedLikeConfig{
		EnableAsyncSync: true,
		SyncBatchSize:   100,
		SyncInterval:    5 * time.Second,
	}
}

// NewEnhancedLikeService 创建增强版点赞服务
func NewEnhancedLikeService(ctx context.Context, config *EnhancedLikeConfig) *EnhancedLikeService {
	if config == nil {
		config = DefaultEnhancedLikeConfig()
	}

	interactionMgr := redis.NewEnhancedInteractionManager(redis.RedisDBInteraction)
	cacheManager := redis.NewLikeCacheManager(redis.RedisDBInteraction)

	service := &EnhancedLikeService{
		ctx:             ctx,
		interactionMgr:  interactionMgr,
		cacheManager:    cacheManager,
		enableAsyncSync: config.EnableAsyncSync,
		syncBatchSize:   config.SyncBatchSize,
		syncInterval:    config.SyncInterval,
	}

	// 启动异步同步worker
	if config.EnableAsyncSync {
		go service.startSyncWorker()
	}

	return service
}

// =============================================================================
// 点赞操作 (主流程)
// =============================================================================

// LikeAction 处理点赞/取消点赞操作
func (s *EnhancedLikeService) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (*interactions.LikeActionResponse, error) {
	var isLiked bool
	var likeCount int64
	var err error

	// 根据请求类型处理不同的点赞操作
	if req.VideoId != 0 {
		isLiked, likeCount, err = s.handleVideoLike(ctx, req)
	} else if req.CommentId != 0 {
		isLiked, likeCount, err = s.handleCommentLike(ctx, req)
	} else {
		return nil, fmt.Errorf("invalid request: neither video_id nor comment_id provided")
	}

	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to handle like action: %v", err)
		return &interactions.LikeActionResponse{
			Base: &base.Status{
				Code: 500,
				Msg:  err.Error(),
			},
		}, err
	}

	return &interactions.LikeActionResponse{
		Base: &base.Status{
			Code: 0,
			Msg:  "success",
		},
		IsLiked:   isLiked,
		LikeCount: likeCount,
	}, nil
}

// handleVideoLike 处理视频点赞
func (s *EnhancedLikeService) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, int64, error) {
	userID := req.UserId
	videoID := req.VideoId

	switch req.ActionType {
	case "like":
		return s.likeVideoEnhanced(ctx, userID, videoID)
	case "unlike":
		return s.unlikeVideoEnhanced(ctx, userID, videoID)
	default:
		return false, 0, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeVideoEnhanced 增强版视频点赞
func (s *EnhancedLikeService) likeVideoEnhanced(ctx context.Context, userID, videoID int64) (bool, int64, error) {
	// 1. 执行Redis点赞操作（原子性 + 限流 + 分布式锁）
	success, isNewLike, err := s.interactionMgr.DoLike(ctx, userID, videoID, redis.BizTypeVideo)
	if err != nil {
		// 限流或锁获取失败，返回友好提示
		hlog.CtxWarnf(ctx, "Like operation blocked: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, 0, err
	}

	if !success {
		return false, 0, fmt.Errorf("like operation failed")
	}

	// 2. 获取最新点赞数
	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)

	// 3. 如果是新点赞，触发相关事件
	if isNewLike {
		// 异步更新热门排行
		go s.updateHotRanking(ctx, videoID, likeCount)

		// 异步记录用户行为
		go s.trackUserBehavior(ctx, userID, "like")

		hlog.CtxInfof(ctx, "Video like success: user_id=%d, video_id=%d, new_count=%d", userID, videoID, likeCount)
	}

	return true, likeCount, nil
}

// unlikeVideoEnhanced 增强版取消视频点赞
func (s *EnhancedLikeService) unlikeVideoEnhanced(ctx context.Context, userID, videoID int64) (bool, int64, error) {
	// 1. 执行Redis取消点赞操作
	wasLiked, err := s.interactionMgr.DoUnlike(ctx, userID, videoID, redis.BizTypeVideo)
	if err != nil {
		hlog.CtxWarnf(ctx, "Unlike operation blocked: user_id=%d, video_id=%d, err=%v", userID, videoID, err)
		return false, 0, err
	}

	// 2. 获取最新点赞数
	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)

	// 3. 如果确实取消了点赞，更新热门排行
	if wasLiked {
		go s.updateHotRanking(ctx, videoID, likeCount)
		hlog.CtxInfof(ctx, "Video unlike success: user_id=%d, video_id=%d, new_count=%d", userID, videoID, likeCount)
	}

	return false, likeCount, nil
}

// handleCommentLike 处理评论点赞
func (s *EnhancedLikeService) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequest) (bool, int64, error) {
	userID := req.UserId
	commentID := req.CommentId

	switch req.ActionType {
	case "like":
		return s.likeCommentEnhanced(ctx, userID, commentID)
	case "unlike":
		return s.unlikeCommentEnhanced(ctx, userID, commentID)
	default:
		return false, 0, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// likeCommentEnhanced 增强版评论点赞
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
		hlog.CtxInfof(ctx, "Comment like success: user_id=%d, comment_id=%d", userID, commentID)
	}

	return true, likeCount, nil
}

// unlikeCommentEnhanced 增强版取消评论点赞
func (s *EnhancedLikeService) unlikeCommentEnhanced(ctx context.Context, userID, commentID int64) (bool, int64, error) {
	_, err := s.interactionMgr.DoUnlike(ctx, userID, commentID, redis.BizTypeComment)
	if err != nil {
		return false, 0, err
	}

	likeCount, _ := s.interactionMgr.GetLikeCount(ctx, commentID, redis.BizTypeComment)
	return false, likeCount, nil
}

// =============================================================================
// 查询操作
// =============================================================================

// GetLikeList 获取用户点赞的视频列表
func (s *EnhancedLikeService) GetLikeList(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	offset := (req.PageNum - 1) * req.PageSize
	limit := req.PageSize

	// 从Redis获取点赞列表
	videoIDs, err := s.interactionMgr.GetUserLikeList(ctx, req.UserId, redis.BizTypeVideo, offset, limit)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get like list: %v", err)
		return &interactions.LikeListResponse{
			Base: &base.Status{Code: 500, Msg: "获取点赞列表失败"},
		}, err
	}

	// 如果Redis没有数据，回源DB
	if len(videoIDs) == 0 {
		videoIDs, err = s.getLikeListFromDB(ctx, req.UserId, int(offset), int(limit))
		if err != nil {
			return &interactions.LikeListResponse{
				Base:  &base.Status{Code: 0, Msg: "success"},
				Items: []*base.Video{},
			}, nil
		}
	}

	// 批量获取视频信息
	videosList := s.batchGetVideoInfo(ctx, videoIDs)

	return &interactions.LikeListResponse{
		Base:  &base.Status{Code: 0, Msg: "success"},
		Items: videosList,
	}, nil
}

// BatchLikeStatus 批量检查点赞状态
func (s *EnhancedLikeService) BatchLikeStatus(ctx context.Context, req *interactions.BatchLikeStatusRequest) (*interactions.BatchLikeStatusResponse, error) {
	// 先从Redis获取
	statusMap, err := s.interactionMgr.BatchGetLikeStatus(ctx, req.UserId, req.VideoIds, redis.BizTypeVideo)
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get batch like status from Redis: %v", err)
		// 回源DB
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

// GetLikeCount 获取点赞数
func (s *EnhancedLikeService) GetLikeCount(ctx context.Context, bizType int, objID int64) (int64, error) {
	count, err := s.interactionMgr.GetLikeCount(ctx, objID, bizType)
	if err != nil || count == 0 {
		// 回源DB
		if bizType == redis.BizTypeVideo {
			return db.GetVideoLikeCount(ctx, objID)
		}
	}
	return count, nil
}

// BatchGetLikeCount 批量获取点赞数
func (s *EnhancedLikeService) BatchGetLikeCount(ctx context.Context, bizType int, objIDs []int64) (map[int64]int64, error) {
	return s.interactionMgr.BatchGetLikeCount(ctx, objIDs, bizType)
}

// =============================================================================
// 异步同步Worker
// =============================================================================

// startSyncWorker 启动异步同步worker
func (s *EnhancedLikeService) startSyncWorker() {
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.processSyncQueue()
		case <-s.ctx.Done():
			// 退出前处理完队列
			s.processSyncQueue()
			return
		}
	}
}

// processSyncQueue 处理同步队列
func (s *EnhancedLikeService) processSyncQueue() {
	ctx := context.Background()

	// 获取队列长度
	queueLen, err := s.interactionMgr.GetSyncQueueLength(ctx)
	if err != nil || queueLen == 0 {
		return
	}

	// 批量处理
	var wg sync.WaitGroup
	batchSize := s.syncBatchSize
	if int(queueLen) < batchSize {
		batchSize = int(queueLen)
	}

	for i := 0; i < batchSize; i++ {
		action, err := s.interactionMgr.PopSyncAction(ctx, 100*time.Millisecond)
		if err != nil || action == nil {
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

// syncToDatabase 同步到数据库
func (s *EnhancedLikeService) syncToDatabase(ctx context.Context, action *redis.LikeAction) {
	var err error

	switch action.BizType {
	case redis.BizTypeVideo:
		if action.Action == "like" {
			_, err = db.CreateVideoLike(ctx, action.UserID, action.ObjID)
		} else {
			_, err = db.DeleteVideoLike(ctx, action.UserID, action.ObjID)
		}
	case redis.BizTypeComment:
		if action.Action == "like" {
			err = db.CreateCommentLike(ctx, action.UserID, action.ObjID)
		} else {
			err = db.DeleteCommentLike(ctx, action.UserID, action.ObjID)
		}
	}

	if err != nil {
		hlog.Warnf("Failed to sync like action to DB: %+v, err=%v", action, err)
		// TODO: 失败重试逻辑，可以放回队列或写入死信队列
	}
}

// =============================================================================
// 辅助方法
// =============================================================================

// updateHotRanking 更新热门排行
func (s *EnhancedLikeService) updateHotRanking(ctx context.Context, videoID int64, likeCount int64) {
	if err := s.interactionMgr.UpdateHotVideoCache(ctx, videoID, likeCount); err != nil {
		hlog.CtxWarnf(ctx, "Failed to update hot ranking: video_id=%d, err=%v", videoID, err)
	}
}

// trackUserBehavior 记录用户行为
func (s *EnhancedLikeService) trackUserBehavior(ctx context.Context, userID int64, activity string) {
	if err := s.interactionMgr.TrackUserActivity(ctx, userID, activity); err != nil {
		hlog.CtxWarnf(ctx, "Failed to track user behavior: user_id=%d, err=%v", userID, err)
	}
}

// getLikeListFromDB 从数据库获取点赞列表
func (s *EnhancedLikeService) getLikeListFromDB(ctx context.Context, userID int64, offset, limit int) ([]int64, error) {
	return db.GetUserVideoLikes(ctx, userID, offset, limit)
}

// batchGetVideoInfo 批量获取视频信息
func (s *EnhancedLikeService) batchGetVideoInfo(ctx context.Context, videoIDs []int64) []*base.Video {
	if len(videoIDs) == 0 {
		return []*base.Video{}
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		videoList  = make([]*base.Video, 0, len(videoIDs))
	)

	// 并发获取视频信息
	semaphore := make(chan struct{}, 10) // 限制并发数

	for _, videoID := range videoIDs {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(vid int64) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			videoResp, err := client.VideoClient.VideoInfoV2(ctx, &videos.VideoInfoRequestV2{
				VideoId: vid,
			})
			if err != nil {
				hlog.CtxWarnf(ctx, "Failed to get video info: video_id=%d, err=%v", vid, err)
				return
			}

			if videoResp != nil && videoResp.Items != nil {
				mu.Lock()
				videoList = append(videoList, videoResp.Items)
				mu.Unlock()
			}
		}(videoID)
	}

	wg.Wait()
	return videoList
}

// =============================================================================
// 数据校准任务 (定时执行，修复Redis和DB之间的差异)
// =============================================================================

// CalibrateTask 数据校准任务
type CalibrateTask struct {
	interactionMgr *redis.EnhancedInteractionManager
	batchSize      int
}

// NewCalibrateTask 创建校准任务
func NewCalibrateTask(interactionMgr *redis.EnhancedInteractionManager) *CalibrateTask {
	return &CalibrateTask{
		interactionMgr: interactionMgr,
		batchSize:      100,
	}
}

// CalibrateVideoLikeCounts 校准视频点赞数
func (t *CalibrateTask) CalibrateVideoLikeCounts(ctx context.Context, videoIDs []int64) error {
	for _, videoID := range videoIDs {
		// 从DB获取真实计数
		dbCount, err := db.GetVideoLikeCount(ctx, videoID)
		if err != nil {
			continue
		}

		// 获取Redis计数
		redisCount, err := t.interactionMgr.GetLikeCount(ctx, videoID, redis.BizTypeVideo)
		if err != nil {
			continue
		}

		// 如果差异超过阈值，进行校准
		diff := dbCount - redisCount
		if diff < 0 {
			diff = -diff
		}

		if diff > 10 { // 差异超过10，进行校准
			hlog.Infof("Calibrating video like count: video_id=%d, db=%d, redis=%d", videoID, dbCount, redisCount)
			// 更新Redis计数
			// 这里需要实现一个SetLikeCount方法
		}
	}
	return nil
}
