package main

import (
	"context"
	"os"
	"sync"

	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/pkg/errors"
)

// InteractionServiceImpl implements the interaction RPC service.
type InteractionServiceImpl struct{}

// --- Singleton Enhanced Services ---

var (
	enhancedLikeSvc     *service.EnhancedLikeService
	enhancedLikeOnce    sync.Once
	enhancedCommentSvc  *service.EnhancedCommentService
	enhancedCommentOnce sync.Once
)

// getEnhancedLikeService returns a singleton EnhancedLikeService.
// 支持通过环境变量 LIKE_ASYNC_SYNC 控制同步/异步模式:
//   - LIKE_ASYNC_SYNC=true  (默认) → EDA异步模式: Redis-First + Async DB Sync
//   - LIKE_ASYNC_SYNC=false         → 同步直写模式: 直接写入MySQL
func getEnhancedLikeService(ctx context.Context) *service.EnhancedLikeService {
	enhancedLikeOnce.Do(func() {
		config := service.DefaultEnhancedLikeConfig()
		// 通过环境变量控制同步/异步模式 (用于性能对比测试)
		if v := os.Getenv("LIKE_ASYNC_SYNC"); v == "false" || v == "0" {
			config.EnableAsyncSync = false
			hlog.Info("EnhancedLikeService: SYNC mode (EnableAsyncSync=false) via env LIKE_ASYNC_SYNC")
		}
		enhancedLikeSvc = service.NewEnhancedLikeService(ctx, config)
		hlog.Infof("EnhancedLikeService initialized (singleton, async=%v)", config.EnableAsyncSync)
	})
	return enhancedLikeSvc
}

// getEnhancedCommentService returns a singleton EnhancedCommentService.
func getEnhancedCommentService(ctx context.Context) *service.EnhancedCommentService {
	enhancedCommentOnce.Do(func() {
		enhancedCommentSvc = service.NewEnhancedCommentService(ctx)
		hlog.Info("EnhancedCommentService initialized (singleton)")
	})
	return enhancedCommentSvc
}

// LikeAction handles like/unlike requests.
// Now backed by EnhancedLikeService (Redis-First + Async DB Sync).
func (s *InteractionServiceImpl) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (resp *interactions.LikeActionResponse, err error) {
	resp, err = getEnhancedLikeService(ctx).LikeAction(ctx, req)
	if err != nil {
		logServiceError(ctx, "LikeAction", err)
		if resp == nil {
			resp = &interactions.LikeActionResponse{
				Base: &base.Status{Code: consts.StatusInternalServerError, Msg: "内部服务错误"},
			}
		}
		return resp, err
	}
	return resp, nil
}

// LikeList returns the like list for a user.
func (s *InteractionServiceImpl) LikeList(ctx context.Context, req *interactions.LikeListRequest) (resp *interactions.LikeListResponse, err error) {
	resp, err = getEnhancedLikeService(ctx).GetLikeList(ctx, req)
	if err != nil {
		logServiceError(ctx, "LikeList", err)
		if resp == nil {
			resp = &interactions.LikeListResponse{
				Base: &base.Status{Code: consts.StatusBadRequest, Msg: "获取点赞列表失败"},
			}
		}
		return resp, err
	}
	return resp, nil
}

// BatchLikeStatus checks like status for multiple videos.
func (s *InteractionServiceImpl) BatchLikeStatus(ctx context.Context, req *interactions.BatchLikeStatusRequest) (resp *interactions.BatchLikeStatusResponse, err error) {
	if len(req.VideoIds) == 0 {
		return &interactions.BatchLikeStatusResponse{
			Base:       &base.Status{Code: consts.StatusOK, Msg: "success"},
			LikeStatus: make(map[int64]bool),
		}, nil
	}

	resp, err = getEnhancedLikeService(ctx).BatchLikeStatus(ctx, req)
	if err != nil {
		logServiceError(ctx, "BatchLikeStatus", err)
		if resp == nil {
			resp = &interactions.BatchLikeStatusResponse{
				Base:       &base.Status{Code: consts.StatusInternalServerError, Msg: "获取点赞状态失败"},
				LikeStatus: make(map[int64]bool),
			}
		}
		return resp, err
	}
	return resp, nil
}

// CreateComment creates a new comment.
// Now backed by EnhancedCommentService with rate limiting + spam detection.
func (s *InteractionServiceImpl) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) (resp *interactions.CreateCommentResponse, err error) {
	resp = &interactions.CreateCommentResponse{Base: &base.Status{}}

	if err := getEnhancedCommentService(ctx).CreateComment(ctx, req); err != nil {
		logServiceError(ctx, "CreateComment", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "创建评论失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "创建评论成功"
	return resp, nil
}

// ListComment returns paginated comments.
func (s *InteractionServiceImpl) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (resp *interactions.ListCommentResponse, err error) {
	resp, err = getEnhancedCommentService(ctx).ListComment(ctx, req)
	if err != nil {
		logServiceError(ctx, "ListComment", err)
		if resp == nil {
			resp = &interactions.ListCommentResponse{Base: &base.Status{}}
		}
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "获取评论列表失败"
		return resp, err
	}

	if resp.Base == nil {
		resp.Base = &base.Status{}
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取评论成功"
	return resp, nil
}

// DeleteComment deletes a comment with permission checks.
func (s *InteractionServiceImpl) DeleteComment(ctx context.Context, req *interactions.CommentDeleteRequest) (resp *interactions.CommentDeleteResponse, err error) {
	resp = &interactions.CommentDeleteResponse{Base: &base.Status{}}

	if err := getEnhancedCommentService(ctx).DeleteComment(ctx, req); err != nil {
		logServiceError(ctx, "DeleteComment", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "删除评论失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "删除评论成功"
	return resp, nil
}

// DeleteVideoInfo deletes all interaction data for a video.
// Falls back to the basic CommentService since it has the required method.
func (s *InteractionServiceImpl) DeleteVideoInfo(ctx context.Context, req *interactions.DeleteVideoInfoRequest) (resp *interactions.DeleteVideoInfoResponse, err error) {
	resp = &interactions.DeleteVideoInfoResponse{Base: &base.Status{}}

	if err := service.NewCommentService(ctx).NewDeleteVideoInfoEvent(req); err != nil {
		logServiceError(ctx, "DeleteVideoInfo", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "删除视频信息失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "删除视频信息成功"
	return resp, nil
}

// VideoPopularList returns the popular video list.
func (s *InteractionServiceImpl) VideoPopularList(ctx context.Context, req *interactions.VideoPopularListRequest) (resp *interactions.VideoPopularListResponse, err error) {
	resp = &interactions.VideoPopularListResponse{Base: &base.Status{}}

	list, err := service.NewCommentService(ctx).NewVideoPopularListEvent(req)
	if err != nil {
		logServiceError(ctx, "VideoPopularList", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "获取热门视频失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取热门视频成功"
	resp.Data = *list
	return resp, nil
}

// GetNotifications returns the notification list for a user.
func (s *InteractionServiceImpl) GetNotifications(ctx context.Context, req *interactions.GetNotificationsRequest) (resp *interactions.GetNotificationsResponse, err error) {
	resp = &interactions.GetNotificationsResponse{Base: &base.Status{}}

	notifications, totalCount, unreadCount, err := service.NewNotificationService(ctx).GetNotifications(req)
	if err != nil {
		logServiceError(ctx, "GetNotifications", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "获取通知列表失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取通知列表成功"
	resp.Notifications = notifications
	resp.TotalCount = totalCount
	resp.UnreadCount = unreadCount
	return resp, nil
}

// MarkNotificationRead marks notifications as read.
func (s *InteractionServiceImpl) MarkNotificationRead(ctx context.Context, req *interactions.MarkNotificationReadRequest) (resp *interactions.MarkNotificationReadResponse, err error) {
	resp = &interactions.MarkNotificationReadResponse{Base: &base.Status{}}

	markedCount, err := service.NewNotificationService(ctx).MarkNotificationRead(req)
	if err != nil {
		logServiceError(ctx, "MarkNotificationRead", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "标记通知失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "标记通知为已读成功"
	resp.MarkedCount = markedCount
	return resp, nil
}

// logServiceError logs the service error with cause and stack trace.
func logServiceError(ctx context.Context, method string, err error) {
	hlog.CtxErrorf(ctx, "service.%s failed, cause: %v", method, errors.Cause(err))
	hlog.CtxErrorf(ctx, "stack trace:\n%+v\n", err)
}
