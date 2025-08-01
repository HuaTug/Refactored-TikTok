package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/cmd/model"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

type LikeActionServiceV2 struct {
	ctx      context.Context
	producer *mq.Producer
}

func NewLikeActionServiceV2(ctx context.Context, producer *mq.Producer) *LikeActionServiceV2 {
	return &LikeActionServiceV2{
		ctx:      ctx,
		producer: producer,
	}
}

// LikeActionV2 实现异步点赞逻辑
func (service *LikeActionServiceV2) LikeActionV2(ctx context.Context, req *interactions.LikeActionRequestV2) (*interactions.LikeActionResponseV2, error) {
	resp := &interactions.LikeActionResponseV2{
		Base: &base.Status{},
	}

	// 1. 立即处理点赞关系记录（同步操作）
	var err error
	var isLiked bool

	if req.VideoId != 0 {
		// 视频点赞
		isLiked, err = service.handleVideoLike(ctx, req)
		if err != nil {
			hlog.CtxErrorf(ctx, "Failed to handle video like: %v", err)
			resp.Base.Code = 500
			resp.Base.Msg = "处理点赞失败"
			return resp, err
		}

		// 2. 异步发送点赞事件到消息队列
		likeEvent := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    req.VideoId,
			CommentID:  0,
			ActionType: req.ActionType,
			EventType:  "video_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}

		if err := service.producer.PublishLikeEvent(ctx, likeEvent); err != nil {
			hlog.CtxWarnf(ctx, "Failed to publish like event: %v", err)
			// 注意：消息队列发送失败不影响用户操作的成功返回
		}

		// 异步发送通知事件
		if req.ActionType == "like" {
			// 获取视频作者信息来发送通知
			go service.sendLikeNotification(ctx, req.UserId, req.VideoId, "video")
		}

		// 3. 从Redis获取最新点赞数
		resp.LikeCount, _ = redis.GetVideoLikeCount(req.VideoId)

	} else if req.CommentId != 0 {
		// 评论点赞
		isLiked, err = service.handleCommentLike(ctx, req)
		if err != nil {
			hlog.CtxErrorf(ctx, "Failed to handle comment like: %v", err)
			resp.Base.Code = 500
			resp.Base.Msg = "处理点赞失败"
			return resp, err
		}

		// 发送点赞事件
		likeEvent := &mq.LikeEvent{
			UserID:     req.UserId,
			VideoID:    0,
			CommentID:  req.CommentId,
			ActionType: req.ActionType,
			EventType:  "comment_like",
			Timestamp:  time.Now().Unix(),
			EventID:    uuid.New().String(),
		}

		if err := service.producer.PublishLikeEvent(ctx, likeEvent); err != nil {
			hlog.CtxWarnf(ctx, "Failed to publish like event: %v", err)
		}

		// 异步发送通知事件
		if req.ActionType == "like" {
			go service.sendLikeNotification(ctx, req.UserId, req.CommentId, "comment")
		}

		// 获取最新点赞数
		resp.LikeCount, _ = redis.GetCommentLikeCount(req.CommentId)
	} else {
		resp.Base.Code = 400
		resp.Base.Msg = "请求参数错误"
		return resp, errno.RequestErr
	}

	// 4. 返回成功响应
	resp.Base.Code = 200
	resp.Base.Msg = "操作成功"
	resp.IsLiked = isLiked

	return resp, nil
}

// 处理视频点赞逻辑
func (service *LikeActionServiceV2) handleVideoLike(ctx context.Context, req *interactions.LikeActionRequestV2) (bool, error) {
	switch req.ActionType {
	case "like":
		// 添加点赞记录到Redis
		if err := redis.AppendVideoLikeInfo(req.VideoId, req.UserId); err != nil {
			return false, err
		}

		// 异步添加到数据库
		go func() {
			like := &model.UserBehavior{
				UserId:       req.UserId,
				VideoId:      req.VideoId,
				BehaviorType: "like",
				BehaviorTime: time.Now().Format(constants.DataFormate),
			}
			if err := db.AddUserLikeBehavior(context.Background(), like); err != nil {
				hlog.Errorf("Failed to save like behavior to DB: %v", err)
			}
		}()

		return true, nil

	case "unlike":
		// 从Redis移除点赞记录
		if err := redis.RemoveVideoLikeInfo(req.VideoId, req.UserId); err != nil {
			return false, err
		}

		// 异步从数据库删除
		go func() {
			if err := db.DeleteUserLikeBehavior(context.Background(), req.UserId, req.VideoId, "like"); err != nil {
				hlog.Errorf("Failed to delete like behavior from DB: %v", err)
			}
		}()

		return false, nil

	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// 处理评论点赞逻辑
func (service *LikeActionServiceV2) handleCommentLike(ctx context.Context, req *interactions.LikeActionRequestV2) (bool, error) {
	switch req.ActionType {
	case "like":
		if err := redis.AppendCommentLikeInfo(req.CommentId, req.UserId); err != nil {
			return false, err
		}
		return true, nil

	case "unlike":
		if err := redis.RemoveCommentLikeInfo(req.CommentId, req.UserId); err != nil {
			return false, err
		}
		return false, nil

	default:
		return false, fmt.Errorf("invalid action type: %s", req.ActionType)
	}
}

// LikeListV2 获取用户点赞的视频列表
func (service *LikeActionServiceV2) LikeListV2(ctx context.Context, req *interactions.LikeListRequest) (*interactions.LikeListResponse, error) {
	resp := &interactions.LikeListResponse{
		Base: &base.Status{},
	}

	// 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = constants.DefaultLimit
	}

	// 从数据库获取用户点赞的视频ID列表
	list, err := db.GetVideoLikeListByUserId(ctx, req.UserId, req.PageNum, req.PageSize)
	if err != nil {
		hlog.CtxErrorf(ctx, "Failed to get video like list: %v", err)
		resp.Base.Code = 500
		resp.Base.Msg = "获取点赞列表失败"
		return resp, err
	}

	hlog.CtxInfof(ctx, "Got like list for user %d: %v", req.UserId, list)

	// 如果没有点赞记录，直接返回空列表
	if list == nil || len(*list) == 0 {
		resp.Base.Code = 200
		resp.Base.Msg = "获取点赞列表成功"
		resp.Items = []*base.Video{}
		return resp, nil
	}

	// 批量获取视频信息
	res := make([]*base.Video, 0, len(*list))
	for _, item := range *list {
		vid, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			hlog.CtxWarnf(ctx, "Invalid video ID: %s", item)
			continue
		}

		// 调用视频服务获取视频详情
		videoResp, err := client.VideoInfo(ctx, &videos.VideoInfoRequest{VideoId: vid})
		if err != nil {
			hlog.CtxWarnf(ctx, "Failed to get video info for ID %d: %v", vid, err)
			continue
		}

		if videoResp != nil && videoResp.Items != nil {
			res = append(res, videoResp.Items)
		}
	}

	resp.Items = res
	resp.Base.Code = 200
	resp.Base.Msg = "获取点赞列表成功"
	return resp, nil
}

// 发送点赞通知
func (service *LikeActionServiceV2) sendLikeNotification(ctx context.Context, fromUserID, targetID int64, targetType string) {
	// 这里需要查询目标的作者ID
	var toUserID int64
	var content string

	if targetType == "video" {
		// 查询视频作者
		video, err := db.GetVideoInfo(ctx, targetID)
		if err != nil {
			hlog.Errorf("Failed to get video info for notification: %v", err)
			return
		}
		toUserID = video.UserId
		content = "赞了你的视频"
	} else if targetType == "comment" {
		// 查询评论作者
		comment, err := db.GetCommentInfo(ctx, targetID)
		if err != nil {
			hlog.Errorf("Failed to get comment info for notification: %v", err)
			return
		}
		toUserID = comment.UserId
		content = "赞了你的评论"
	}

	// 不给自己发通知
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
