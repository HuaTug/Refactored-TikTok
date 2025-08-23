package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	jwt "HuaTug.com/pkg"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// BatchOperateVideosV2 批量操作视频
func BatchOperateVideosV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoDeleteParam // 复用VideoId参数结构，实际应该有专门的批量操作参数
	if err = c.BindAndValidate(&VideoPublish); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}
	if resp, err := rpc.BatchOperateVideosV2(ctx, &videos.BatchVideoOperationRequest{
		UserId:   UserId,
		VideoIds: []int64{VideoPublish.VideoId}, // 简化处理，实际应支持多个视频ID
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}

// TranscodeVideoV2 转码视频
func TranscodeVideoV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoDeleteParam // 复用VideoId参数结构
	if err = c.BindAndValidate(&VideoPublish); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}
	if resp, err := rpc.TranscodeVideoV2(ctx, &videos.VideoTranscodingRequest{
		UserId:  UserId,
		VideoId: VideoPublish.VideoId,
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}

// GetVideoAnalyticsV2 获取视频分析数据
func GetVideoAnalyticsV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoDeleteParam // 复用VideoId参数结构
	if err = c.BindAndValidate(&VideoPublish); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}
	if resp, err := rpc.GetVideoAnalyticsV2(ctx, &videos.VideoAnalyticsRequest{
		UserId:   UserId,
		VideoIds: []int64{VideoPublish.VideoId}, // 简化处理，实际应支持多个视频ID
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
