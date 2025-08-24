package handlers

import (
	"context"
	"strings"

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
	
	// 处理标签，将lab_name转换为标签数组
	var tags []string
	if VideoPublish.LabName != "" {
		// 按逗号分割标签
		tags = strings.Split(VideoPublish.LabName, ",")
		// 清理标签空格
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	// 根据Open字段设置隐私设置
	privacy := "public"
	switch VideoPublish.Open {
	case 0:
		privacy = "private"
	case 1:
		privacy = "public"
	case 2:
		privacy = "friends"
	}

	if resp, err := rpc.VideoPublishStartV2(ctx, &videos.VideoPublishStartRequestV2{
		UserId:           UserId,
		Title:            VideoPublish.Title,
		Description:      VideoPublish.Description,
		Tags:             tags,
		Category:         VideoPublish.Category,
		Privacy:          privacy,
		ChunkTotalNumber: int32(VideoPublish.ChunkTotalNumber),
	}); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}