package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/interactions"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type BatchLikeStatusParam struct {
	VideoIds []int64 `json:"video_ids" form:"video_ids"`
}

type BatchLikeStatusResponse struct {
	LikeStatus map[int64]bool  `json:"like_status"`
	LikeCounts map[int64]int64 `json:"like_counts"`
}

// BatchLikeStatus 批量检查用户是否点赞了视频，同时返回点赞数
func BatchLikeStatus(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var param BatchLikeStatusParam

	if err = c.BindAndValidate(&param); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}

	// 初始化结果 map
	likeStatus := make(map[int64]bool)
	likeCounts := make(map[int64]int64)
	for _, vid := range param.VideoIds {
		likeStatus[vid] = false
		likeCounts[vid] = 0
	}

	if len(param.VideoIds) == 0 {
		SendResponse(c, errno.Success, BatchLikeStatusResponse{
			LikeStatus: likeStatus,
			LikeCounts: likeCounts,
		})
		return
	}

	// 调用 RPC 获取批量点赞状态
	resp, err := rpc.BatchLikeStatus(ctx, &interactions.BatchLikeStatusRequest{
		UserId:   UserId,
		VideoIds: param.VideoIds,
	})

	if err != nil {
		hlog.CtxErrorf(ctx, "BatchLikeStatus RPC failed: %v", err)
		// RPC 失败时返回默认值
		SendResponse(c, errno.Success, BatchLikeStatusResponse{
			LikeStatus: likeStatus,
			LikeCounts: likeCounts,
		})
		return
	}

	// 解析 RPC 响应
	if resp != nil && resp.LikeStatus != nil {
		likeStatus = resp.LikeStatus
	}

	// 点赞数需要单独获取（从 interaction 服务）
	// 暂时返回 0，后续可以扩展 RPC 接口返回点赞数
	SendResponse(c, errno.Success, BatchLikeStatusResponse{
		LikeStatus: likeStatus,
		LikeCounts: likeCounts,
	})
}
