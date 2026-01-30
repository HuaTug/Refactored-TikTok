package handlers

import (
	"context"
	"strings"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	jwt "HuaTug.com/pkg"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/logsystem"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func VideoPublishStartV2(ctx context.Context, c *app.RequestContext) {
	// 开始计时
	timer := logsystem.NewTimer()

	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishStartParam
	if err = c.BindAndValidate(&VideoPublish); err != nil {
		// 记录参数绑定失败的错误日志
		logsystem.LogBusinessError(ctx, c, "INVALID_PARAMS", err.Error())
		logsystem.SetErrorInfo(c, "INVALID_PARAMS", err.Error())
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		// 记录 JWT 认证失败的错误日志
		logsystem.LogBusinessError(ctx, c, "AUTH_FAILED", err.Error())
		logsystem.SetErrorInfo(c, "AUTH_FAILED", err.Error())
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
		// 设置用户ID到上下文
		logsystem.SetUserID(c, UserId)
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
		// 记录 RPC 调用失败的错误日志 -> 写入 Kafka -> ES
		logsystem.LogServiceError(ctx, c, "VideoPublishStartV2", "RPC_CALL_FAILED", err.Error(), timer.ElapsedMs())
		logsystem.LogErrorWithContext(ctx, c, "VideoPublishStartV2", "RPC_CALL_FAILED", "network", err.Error(), map[string]string{
			"user_id":            utils.Int64ToString(UserId),
			"title":              VideoPublish.Title,
			"chunk_total_number": utils.Int64ToString(VideoPublish.ChunkTotalNumber),
		})
		logsystem.SetErrorInfo(c, "RPC_CALL_FAILED", err.Error())
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		SendResponse(c, errno.Success, resp)
	}
}
