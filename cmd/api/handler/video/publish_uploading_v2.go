package handlers

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	jwt "HuaTug.com/pkg/auth"
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

	// 获取上传的文件数据（前端已经分片，这里接收的是单个分片）
	file, err := c.FormFile("data")
	if err != nil {
		hlog.Errorf("Failed to get form file 'data': %v", err)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("missing or invalid file data")), nil)
		return
	}

	// 打开文件读取内容
	fileContent, err := file.Open()
	if err != nil {
		hlog.Errorf("Failed to open uploaded file: %v", err)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("failed to read file data")), nil)
		return
	}
	defer fileContent.Close()

	// 读取完整分片数据
	chunkData, err := io.ReadAll(fileContent)
	if err != nil {
		hlog.Errorf("Failed to read file content: %v", err)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("failed to read file content")), nil)
		return
	}

	// 获取分片编号（前端传递的是当前分片号，从1开始）
	chunkNumber := int(VideoPublish.ChunkNumber)
	if chunkNumber <= 0 {
		hlog.Errorf("Invalid chunk number: %d", chunkNumber)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("invalid chunk number: %d", chunkNumber)), nil)
		return
	}

	hlog.Infof("Received chunk %d for session %s: size=%d bytes, filename=%s",
		chunkNumber, VideoPublish.Uuid, len(chunkData), file.Filename)

	// 计算分片MD5
	chunkMd5 := fmt.Sprintf("%x", md5.Sum(chunkData))

	// 直接调用RPC上传这个分片（前端已经完成分片，后端只需转发）
	resp, err := rpc.VideoPublishUploadingV2(ctx, &videos.VideoPublishUploadingRequestV2{
		UserId:            UserId,
		UploadSessionUuid: VideoPublish.Uuid,
		ChunkNumber:       int32(chunkNumber),
		ChunkData:         chunkData,
		ChunkMd5:          chunkMd5,
		ChunkSize:         int64(len(chunkData)),
		ChunkOffset:       0, // 前端分片模式下不需要offset
	})

	if err != nil {
		hlog.Errorf("Failed to upload chunk %d for session %s: %v", chunkNumber, VideoPublish.Uuid, err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	hlog.Infof("Successfully uploaded chunk %d for session %s", chunkNumber, VideoPublish.Uuid)
	SendResponse(c, errno.Success, resp)
}
