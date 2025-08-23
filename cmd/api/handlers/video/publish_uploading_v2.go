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

func VideoPublishUploadingV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishUploadingParam
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
	if resp, err := rpc.VideoPublishUploadingV2(ctx, &videos.VideoPublishUploadingRequestV2{
		UserId:            UserId,
		UploadSessionUuid: VideoPublish.Uuid,
		ChunkNumber:       int32(VideoPublish.ChunkNumber),
		ChunkData:         []byte{VideoPublish.Data},
		ChunkMd5:          "", // 需要从请求中获取MD5
		ChunkSize:         int64(len([]byte{VideoPublish.Data})),
		ChunkOffset:       0, // 需要计算偏移量
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
