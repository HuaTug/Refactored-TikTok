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

func VideoPublishCompleteV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishCompleteParam
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
	// 最终文件的MD5和Size由视频服务在合并后计算，这里无需在网关侧读本地文件
	finalFileMd5 := ""
	var finalFileSize int64 = 0

	if resp, err := rpc.VideoPublishCompleteV2(ctx, &videos.VideoPublishCompleteRequestV2{
		UserId:            UserId,
		UploadSessionUuid: VideoPublish.Uuid,
		FinalFileMd5:      finalFileMd5,
		FinalFileSize:     finalFileSize,
		EnableTranscoding: true,
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
