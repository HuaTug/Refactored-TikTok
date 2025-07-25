package main

import (
	"context"

	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/pkg/errors"
)

type InteractionServiceImpl struct {
	producer *mq.Producer
}

// 全局生产者实例，在main.go中初始化
var globalProducer *mq.Producer

func SetGlobalProducer(producer *mq.Producer) {
	globalProducer = producer
}

func (s *InteractionServiceImpl) LikeAction(ctx context.Context, req *interactions.LikeActionRequest) (resp *interactions.LikeActionResponse, err error) {
	resp = new(interactions.LikeActionResponse)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	// err = service.NewVideoVisitService(ctx).RecordVisit(req)
	err = service.NewLikeActionService(ctx).NewLikeActionEvent(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.LikeAction failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Like Video !"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Video Liked Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) LikeList(ctx context.Context, req *interactions.LikeListRequest) (resp *interactions.LikeListResponse, err error) {
	// TODO: Add your implementation logic here
	// Example:
	resp = new(interactions.LikeListResponse)
	resp, err = service.NewLikeActionService(ctx).NewLikeListEvent(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.LikeListEvent failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to ListLike_Video!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "ListLike_Video Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) CreateComment(ctx context.Context, req *interactions.CreateCommentRequest) (resp *interactions.CreateCommentResponse, err error) {
	resp = new(interactions.CreateCommentResponse)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	err = service.NewCommentService(ctx).CreateComment(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.CreateComment failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Create Comment!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Create Comment Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) ListComment(ctx context.Context, req *interactions.ListCommentRequest) (resp *interactions.ListCommentResponse, err error) {
	// TODO: Add your implementation logic here
	// Example:
	resp, err = service.NewCommentService(ctx).ListComment(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.ListComment failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to List Comment Visit!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "ListComment Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) DeleteComment(ctx context.Context, req *interactions.CommentDeleteRequest) (resp *interactions.CommentDeleteResponse, err error) {
	resp = new(interactions.CommentDeleteResponse)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	err = service.NewCommentService(ctx).NewDeleteEvent(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.DeleteEvent failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Delete Comment"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Delete Comment Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) DeleteVideoInfo(ctx context.Context, req *interactions.DeleteVideoInfoRequest) (resp *interactions.DeleteVideoInfoResponse, err error) {

	resp = new(interactions.DeleteVideoInfoResponse)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	err = service.NewCommentService(ctx).NewDeleteVideoInfoEvent(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.DeleteVideoInfo failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Delete VideoInfo!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Delete VideoInfo Successfully"
	return resp, nil
}

func (s *InteractionServiceImpl) VideoPopularList(ctx context.Context, req *interactions.VideoPopularListRequest) (resp *interactions.VideoPopularListResponse, err error) {

	resp = new(interactions.VideoPopularListResponse)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	temp := new([]string)
	temp, err = service.NewCommentService(ctx).NewVideoPopularListEvent(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPopular failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Show VideoPopular Visit!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Show VideoPopular Successfully"
	resp.Data = *temp
	return resp, nil
}

// ========== V2版本API实现 ==========

func (s *InteractionServiceImpl) LikeActionV2(ctx context.Context, req *interactions.LikeActionRequestV2) (resp *interactions.LikeActionResponseV2, err error) {
	// 使用全局producer实例
	likeServiceV2 := service.NewLikeActionServiceV2(ctx, globalProducer)

	resp, err = likeServiceV2.LikeActionV2(ctx, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.LikeActionV2 failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		if resp == nil {
			resp = &interactions.LikeActionResponseV2{
				Base: &base.Status{
					Code: consts.StatusInternalServerError,
					Msg:  "内部服务错误",
				},
			}
		}
		return resp, err
	}

	return resp, nil
}

// ========== 通知功能实现 ==========

func (s *InteractionServiceImpl) GetNotifications(ctx context.Context, req *interactions.GetNotificationsRequest) (resp *interactions.GetNotificationsResponse, err error) {
	resp = &interactions.GetNotificationsResponse{
		Base: &base.Status{},
	}

	// TODO: 实现获取通知列表的逻辑
	// 这里需要从数据库或缓存中查询用户的通知

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取通知列表成功"
	resp.Notifications = []*interactions.NotificationInfo{} // 暂时返回空列表
	resp.TotalCount = 0
	resp.UnreadCount = 0

	return resp, nil
}

func (s *InteractionServiceImpl) MarkNotificationRead(ctx context.Context, req *interactions.MarkNotificationReadRequest) (resp *interactions.MarkNotificationReadResponse, err error) {
	resp = &interactions.MarkNotificationReadResponse{
		Base: &base.Status{},
	}

	// TODO: 实现标记通知为已读的逻辑

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "标记通知为已读成功"
	resp.MarkedCount = int64(len(req.NotificationIds))

	return resp, nil
}
