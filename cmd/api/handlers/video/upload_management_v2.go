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

// GetUploadProgressV2 获取上传进度
func GetUploadProgressV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishCompleteParam // 复用UUID参数结构
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
	if resp, err := rpc.GetUploadProgressV2(ctx, &videos.VideoPublishProgressRequestV2{
		UserId:            UserId,
		UploadSessionUuid: VideoPublish.Uuid,
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}

// ResumeUploadV2 恢复上传
func ResumeUploadV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishCompleteParam // 复用UUID参数结构
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
	if resp, err := rpc.ResumeUploadV2(ctx, &videos.VideoPublishResumeRequestV2{
		UserId:            UserId,
		UploadSessionUuid: VideoPublish.Uuid,
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
