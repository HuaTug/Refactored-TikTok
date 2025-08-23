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

func VideoPublishStartV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishStartParam
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
	if resp, err := rpc.VideoPublishStartV2(ctx, &videos.VideoPublishStartRequestV2{
		UserId:           UserId,
		Title:            VideoPublish.Title,
		Description:      VideoPublish.Description,
		Tags:             []string{VideoPublish.LabName}, // 将lab_name转换为tags数组
		Category:         VideoPublish.Category,
		Privacy:          "public", // 默认公开，可根据Open字段调整
		ChunkTotalNumber: int32(VideoPublish.ChunkTotalNumber),
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
