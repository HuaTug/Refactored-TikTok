package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/interactions"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// CommentItemWithLike 在原始 Comment 基础上附加 is_liked 字段
type CommentItemWithLike struct {
	*base.Comment
	IsLiked bool `json:"is_liked"`
}

// ListCommentWithLikeResponse 带点赞状态的评论列表响应
type ListCommentWithLikeResponse struct {
	Base  *base.Status           `json:"base"`
	Items []*CommentItemWithLike `json:"items"`
}

func ListComment(ctx context.Context, c *app.RequestContext) {
	var err error
	var Comment ListCommentParam
	if err = c.BindAndValidate(&Comment); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if err := c.Bind(&Comment); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
	}

	// Set default sort type to "hot" if not specified
	if Comment.SortType == "" {
		Comment.SortType = "hot"
	}

	resp, err := rpc.ListComment(ctx, &interactions.ListCommentRequest{
		VideoId:   Comment.VideoId,
		CommentId: Comment.CommentId,
		PageNum:   Comment.PageNum,
		PageSize:  Comment.PageSize,
		SortType:  Comment.SortType,
	})
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 获取当前用户 ID，用于查询评论点赞状态
	var userID int64
	if v, jwtErr := jwt.ConvertJWTPayloadToString(ctx, c); jwtErr == nil {
		userID = utils.Transfer(v)
	}

	// 如果有用户登录，批量查询评论点赞状态
	items := make([]*CommentItemWithLike, 0, len(resp.Items))
	if userID > 0 && len(resp.Items) > 0 && redis.RedisDBInteraction != nil {
		commentIDs := make([]int64, 0, len(resp.Items))
		for _, item := range resp.Items {
			commentIDs = append(commentIDs, item.CommentId)
		}

		enhancedMgr := redis.NewEnhancedInteractionManager(redis.RedisDBInteraction)
		likeStatusMap, statusErr := enhancedMgr.BatchGetLikeStatus(ctx, userID, commentIDs, redis.BizTypeComment)
		if statusErr != nil {
			hlog.CtxWarnf(ctx, "Failed to batch get comment like status: %v", statusErr)
			likeStatusMap = make(map[int64]bool)
		}

		for _, item := range resp.Items {
			items = append(items, &CommentItemWithLike{
				Comment: item,
				IsLiked: likeStatusMap[item.CommentId],
			})
		}
	} else {
		for _, item := range resp.Items {
			items = append(items, &CommentItemWithLike{
				Comment: item,
				IsLiked: false,
			})
		}
	}

	SendResponse(c, errno.Success, ListCommentWithLikeResponse{
		Base:  resp.Base,
		Items: items,
	})
}
