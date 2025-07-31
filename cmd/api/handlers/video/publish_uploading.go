package handlers

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/cmd/video/infras/redis"

	"HuaTug.com/kitex_gen/videos"
	jwt "HuaTug.com/pkg"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func VideoPublishUploading(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64
	var VideoPublish VideoPublishUploadingParam
	if err = c.BindAndValidate(&VideoPublish); err != nil {
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
	Data, err := c.FormFile("data")
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 获取Redis中的chunk信息
	res, err := redis.GetChunkInfo(fmt.Sprint(UserId), VideoPublish.Uuid)
	if err != nil {
		hlog.Error("Failed to get chunk info from Redis:", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 检查Redis返回的数据是否为空
	if len(res) == 0 {
		hlog.Error("Chunk info not found in Redis for uuid:", VideoPublish.Uuid)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("chunk info not found for uuid: %s", VideoPublish.Uuid)), nil)
		return
	}

	// 检查第一个元素是否为空
	if res[0] == "" {
		hlog.Error("Chunk total number is empty in Redis")
		SendResponse(c, errno.ConvertErr(fmt.Errorf("chunk total number is empty")), nil)
		return
	}

	// 解析总分片数
	chunkTotalNumber, err := strconv.ParseInt(res[0], 10, 64)
	if err != nil {
		hlog.Error("Failed to parse chunk total number:", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 验证分片编号的合理性
	if VideoPublish.ChunkNumber <= 0 || VideoPublish.ChunkNumber > chunkTotalNumber {
		hlog.Error("Invalid chunk number:", VideoPublish.ChunkNumber, "total:", chunkTotalNumber)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("invalid chunk number: %d, should be between 1 and %d", VideoPublish.ChunkNumber, chunkTotalNumber)), nil)
		return
	}

	// 改进的分片大小计算逻辑
	fileSize := Data.Size
	var chunkSize int64
	var offset int64

	if chunkTotalNumber > 1 {
		// 计算每个分片的大小（除了最后一个分片）
		baseChunkSize := fileSize / chunkTotalNumber
		remainder := fileSize % chunkTotalNumber

		if VideoPublish.ChunkNumber < chunkTotalNumber {
			// 前面的分片使用基本大小
			chunkSize = baseChunkSize
			if VideoPublish.ChunkNumber <= remainder {
				chunkSize++ // 前remainder个分片多分配1字节
			}
			offset = (VideoPublish.ChunkNumber-1)*baseChunkSize + min(VideoPublish.ChunkNumber-1, remainder)
		} else {
			// 最后一个分片包含剩余的所有数据
			offset = (VideoPublish.ChunkNumber-1)*baseChunkSize + remainder
			chunkSize = fileSize - offset
		}
	} else {
		// 只有一个分片，使用整个文件
		chunkSize = fileSize
		offset = 0
	}

	// 验证计算结果的合理性
	if chunkSize <= 0 {
		hlog.Error("Invalid chunk size calculated:", chunkSize)
		SendResponse(c, errno.ConvertErr(fmt.Errorf("invalid chunk size: %d", chunkSize)), nil)
		return
	}

	if offset+chunkSize > fileSize {
		hlog.Warn("Chunk size exceeds file boundary, adjusting...")
		chunkSize = fileSize - offset
	}

	file, err := Data.Open()
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	defer file.Close()

	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		hlog.Error("Failed to seek file:", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	buffer := make([]byte, chunkSize)
	bytesRead, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		hlog.Error("Failed to read file chunk:", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	chunkFileName := fmt.Sprintf("%s_part_%d", VideoPublish.FileName, VideoPublish.ChunkNumber)
	md5hash := utils.GetBytesMD5(buffer[:bytesRead])

	hlog.Info("Processing chunk: ", VideoPublish.ChunkNumber, " of: ", chunkTotalNumber,
		" offset: ", offset, " size: ", bytesRead, " filename: ", chunkFileName)

	resp, err := rpc.VideoPublishUploading(ctx, &videos.VideoPublishUploadingRequest{
		UserId:      UserId,
		Data:        buffer[:bytesRead],
		Filename:    chunkFileName,
		IsM3u8:      VideoPublish.Is_M3U8,
		Md5:         md5hash,
		Uuid:        VideoPublish.Uuid,
		ChunkNumber: VideoPublish.ChunkNumber,
	})
	if err != nil {
		hlog.Error("RPC call failed:", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	SendResponse(c, errno.Success, resp)
}

// min 函数用于获取两个int64的最小值
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
