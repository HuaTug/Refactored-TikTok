package handlers

import (
	"context"
	"crypto/md5"
	"fmt"

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

	// 获取上传的文件数据
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

	// 读取完整文件数据
	fileData := make([]byte, file.Size)
	_, err = fileContent.Read(fileData)
	if err != nil {
		hlog.Errorf("Failed to read file content: %v", err)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("failed to read file content")), nil)
		return
	}

	hlog.Infof("Successfully read file data: size=%d bytes, filename=%s, chunk_num=%d",
		len(fileData), file.Filename, VideoPublish.ChunkNumber)

	// 根据chunk_num对文件进行分片处理
	chunkNum := int(VideoPublish.ChunkNumber)
	if chunkNum <= 0 {
		hlog.Errorf("Invalid chunk number: %d", chunkNum)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("invalid chunk number: %d", chunkNum)), nil)
		return
	}

	// 计算每个分片的大小
	fileSize := int64(len(fileData))
	chunkSize := fileSize / int64(chunkNum)
	if fileSize%int64(chunkNum) != 0 {
		chunkSize++ // 处理不能整除的情况
	}

	hlog.Infof("File will be split into %d chunks, each chunk size: %d bytes", chunkNum, chunkSize)

	// 依次处理每个分片并上传到MinIO
	var allErrors []error
	for i := 1; i <= chunkNum; i++ {
		// 计算当前分片的数据范围
		startOffset := int64(i-1) * chunkSize
		endOffset := startOffset + chunkSize
		if endOffset > fileSize {
			endOffset = fileSize
		}

		// 提取当前分片数据
		chunkData := fileData[startOffset:endOffset]

		// 计算分片MD5
		chunkMd5 := fmt.Sprintf("%x", md5.Sum(chunkData))

		hlog.Infof("Processing chunk %d/%d: offset=%d-%d, size=%d bytes",
			i, chunkNum, startOffset, endOffset, len(chunkData))

		// 调用RPC上传当前分片
		_, err := rpc.VideoPublishUploadingV2(ctx, &videos.VideoPublishUploadingRequestV2{
			UserId:            UserId,
			UploadSessionUuid: VideoPublish.Uuid,
			ChunkNumber:       int32(i),
			ChunkData:         chunkData,
			ChunkMd5:          chunkMd5,
			ChunkSize:         int64(len(chunkData)),
			ChunkOffset:       startOffset,
		})

		if err != nil {
			hlog.Errorf("Failed to upload chunk %d/%d: %v", i, chunkNum, err)
			allErrors = append(allErrors, fmt.Errorf("chunk %d failed: %v", i, err))
			// 继续尝试上传其他分片，而不是立即返回
		} else {
			hlog.Infof("Successfully uploaded chunk %d/%d", i, chunkNum)
		}
	}

	// 检查是否有上传失败的分片
	if len(allErrors) > 0 {
		hlog.Errorf("Some chunks failed to upload: %v", allErrors)
		// 返回第一个错误，但在日志中记录所有错误
		SendResponse(c, errno.ConvertErr(allErrors[0]), nil)
		return
	}

	// 所有分片都上传成功
	hlog.Infof("All %d chunks uploaded successfully for session %s", chunkNum, VideoPublish.Uuid)

	// 返回成功响应
	resp := map[string]interface{}{
		"message":      "All chunks uploaded successfully",
		"total_chunks": chunkNum,
		"file_size":    fileSize,
	}
	SendResponse(c, errno.Success, resp)
}
