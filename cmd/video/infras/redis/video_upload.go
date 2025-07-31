package redis

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis"
)

func GetVideoDBKeys() ([]string, error) {
	keys, err := redisDBVideoUpload.Keys(`*`).Result()
	if err != nil {
		return nil, err
	}
	return keys, err
}
func GetChunkInfo(uid, uuid string) ([]string, error) {
	key := "l:" + uid + ":" + uuid
	v, err := redisDBVideoUpload.LRange(key, 0, -1).Result()
	if err != nil {
		hlog.Error("Redis LRange failed for key:", key, "error:", err)
		return nil, err
	}

	// 检查数据是否存在
	if len(v) == 0 {
		hlog.Error("No chunk info found for key:", key)
		return nil, fmt.Errorf("no chunk info found for uid:%s uuid:%s", uid, uuid)
	}

	// 验证数据格式 - 应该至少包含 [chunkTotalNumber, title, description]
	if len(v) < 3 {
		hlog.Error("Incomplete chunk info for key:", key, "data:", v)
		return nil, fmt.Errorf("incomplete chunk info: expected at least 3 fields, got %d", len(v))
	}

	// 验证第一个字段（chunkTotalNumber）是否为有效数字
	if v[0] == "" {
		hlog.Error("Empty chunk total number for key:", key)
		return nil, fmt.Errorf("empty chunk total number")
	}

	// 尝试解析数字以验证有效性
	if _, err := strconv.ParseInt(v[0], 10, 64); err != nil {
		hlog.Error("Invalid chunk total number for key:", key, "value:", v[0], "error:", err)
		return nil, fmt.Errorf("invalid chunk total number: %s", v[0])
	}

	hlog.Info("Retrieved chunk info for key:", key, "total chunks:", v[0])
	return v, nil
}

// 在执行删除时 使用管道实现批量操作 减少网络开销
func DelVideoDBKeys(keys []string) error {
	pipe := redisDBVideoUpload.TxPipeline()
	for _, key := range keys {
		pipe.Del(key)
	}
	if _, err := pipe.Exec(); err != nil {
		return err
	}
	return nil
}

func NewVideoEvent(ctx context.Context, title, description, uid, uuid, chuckTotalNumber, lable_name, category string) (string, error) {
	exist, err := redisDBVideoUpload.Exists("l:" + uid + ":" + uuid).Result()
	if err != nil {
		return ``, err
	}
	if exist != 0 {
		return ``, errno.UserNotExistErr
	}
	hlog.Info("Creating new video event with UUID:", uuid)
	if _, err := redisDBVideoUpload.RPush("l:"+uid+":"+uuid, chuckTotalNumber, title, description, lable_name, category).Result(); err != nil {
		return ``, err
	}
	return uuid, nil
}

// 这段代码利用了位图 用来标记上传的视频切片是否成功
func DoneChunkEvent(ctx context.Context, uuid, uid string, chunk int64) error {
	bitrecord, err := redisDBVideoUpload.GetBit("b:"+uid+":"+uuid, chunk).Result()
	if err != nil {
		hlog.Error("Failed to get bit from Redis:", err) // 记录错误
		return err
	}
	if bitrecord == 1 {
		return errors.New("Information already exists")
	}
	if _, err = redisDBVideoUpload.SetBit("b:"+uid+":"+uuid, chunk, 1).Result(); err != nil {
		hlog.Info("SetBit Error:", err)
		return err
	}
	hlog.Info("SetBit Success")
	return nil
}

// 这段代码是用来检查所有视频分片是否都已经被记录
func IsChunkAllRecorded(ctx context.Context, uuid, uid string) (bool, error) {
	// 由于这段代码使用的是Result,即其会返回一个字符串切片
	r, err := redisDBVideoUpload.LRange("l:"+uid+":"+uuid, 0, 0).Result()
	if err != nil {
		return false, err
	}
	chunkTotalNumber, _ := strconv.ParseInt(r[0], 10, 64)
	recordNumber, err := redisDBVideoUpload.BitCount("b:"+uid+":"+uuid, &redis.BitCount{
		Start: 0,
		End:   chunkTotalNumber - 1,
	}).Result()
	if err != nil {
		return false, err
	}
	return chunkTotalNumber == recordNumber, nil
}

func RecordM3U8Filename(ctx context.Context, uuid, uid, filename string) error {
	exist, err := redisDBVideoUpload.Exists("l:" + uid + ":" + uuid).Result()
	if err != nil {
		hlog.Info("First err")
		return err
	}
	if exist == 0 {
		hlog.Info("Second err")
		return errno.RequestErr
	}
	fLen, err := redisDBVideoUpload.LLen("l:" + uid + ":" + uuid).Result()
	if err != nil {
		hlog.Info("Thrid err")
		return err
	}
	if fLen == 4 {
		hlog.Info(errors.New("判断长度出错"))
		return errno.RequestErr
	}
	if _, err := redisDBVideoUpload.RPush("l:"+uid+":"+uuid, filename).Result(); err != nil {
		hlog.Info("Fifth err")
		return err
	}
	return nil
}

func GetM3U8Filename(ctx context.Context, uuid, uid string) (string, error) {
	if filename, err := redisDBVideoUpload.LRange("l:"+uid+":"+uuid, 3, 3).Result(); err != nil || filename[0] == `` {
		return ``, errno.RequestErr
	} else {
		return filename[0], nil
	}
}

func FinishVideoEvent(ctx context.Context, uuid, uid string) ([]string, error) {
	//这是表示在完成视频分片合成后 将视频的相关信息取出
	info, err := redisDBVideoUpload.LRange("l:"+uid+":"+uuid, 1, 2).Result()
	if err != nil {
		return nil, err
	}
	return info, nil
}

func DeleteVideoEvent(ctx context.Context, uuid, uid string) error {
	hlog.Info("l:" + uid + ":" + uuid)
	pipe := redisDBVideoUpload.TxPipeline()
	pipe.Del("l:" + uid + ":" + uuid)
	pipe.Del("b:" + uid + ":" + uuid)
	if _, err := pipe.Exec(); err != nil {
		return err
	}
	return nil
}

// ==== V2 版本专用的Redis方法 ====

// GetUploadedChunksStatus 获取已上传分片的状态（V2版本专用）
func GetUploadedChunksStatus(ctx context.Context, uuid, uid string) ([]bool, error) {
	// 首先获取总分片数
	info, err := GetChunkInfo(uid, uuid)
	if err != nil {
		return nil, err
	}

	totalChunks, err := strconv.ParseInt(info[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid total chunks: %s", info[0])
	}

	// 创建状态切片
	uploadedChunks := make([]bool, totalChunks)

	// 从Redis BitMap中获取每个分片的状态
	// 注意：分片编号从1开始，但数组索引从0开始
	hlog.Infof("DEBUG: Checking BitMap for key: b:%s:%s", uid, uuid)
	for i := int64(1); i <= totalChunks; i++ {
		bit, err := redisDBVideoUpload.GetBit("b:"+uid+":"+uuid, i).Result()
		if err != nil {
			hlog.Warnf("Failed to get bit %d for session %s: %v", i, uuid, err)
			continue
		}
		hlog.Infof("DEBUG: Bit %d value: %d", i, bit)
		// i是分片编号（1-based），i-1是数组索引（0-based）
		uploadedChunks[i-1] = (bit == 1)
	}

	uploadedCount := countTrueBits(uploadedChunks)
	hlog.Infof("Retrieved upload status for session %s: uploaded %d/%d chunks", uuid, uploadedCount, totalChunks)
	return uploadedChunks, nil
}

// UpdateChunkUploadStatus 更新分片上传状态（V2版本专用）
func UpdateChunkUploadStatus(ctx context.Context, uuid, uid string, chunkNumber int64) error {
	hlog.Infof("DEBUG: UpdateChunkUploadStatus called for session %s, chunk %d", uuid, chunkNumber)

	// 检查会话是否存在
	exist, err := redisDBVideoUpload.Exists("l:" + uid + ":" + uuid).Result()
	if err != nil {
		hlog.Errorf("DEBUG: Failed to check session existence: %v", err)
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if exist == 0 {
		hlog.Errorf("DEBUG: Upload session not found: %s", uuid)
		return fmt.Errorf("upload session not found: %s", uuid)
	}
	hlog.Infof("DEBUG: Session exists, proceeding with bit update")

	// 检查分片是否已经上传
	bitKey := "b:" + uid + ":" + uuid
	hlog.Infof("DEBUG: Checking bit at key: %s, position: %d", bitKey, chunkNumber)
	bitrecord, err := redisDBVideoUpload.GetBit(bitKey, chunkNumber).Result()
	if err != nil {
		hlog.Errorf("DEBUG: Failed to get chunk status: %v", err)
		return fmt.Errorf("failed to get chunk status: %w", err)
	}
	hlog.Infof("DEBUG: Current bit value: %d", bitrecord)

	if bitrecord == 1 {
		hlog.Warnf("Chunk %d already uploaded for session %s", chunkNumber, uuid)
		return nil // 不返回错误，允许重复上传
	}

	// 设置分片状态
	hlog.Infof("DEBUG: Setting bit at key: %s, position: %d", bitKey, chunkNumber)
	if _, err = redisDBVideoUpload.SetBit(bitKey, chunkNumber, 1).Result(); err != nil {
		hlog.Errorf("DEBUG: Failed to set chunk status: %v", err)
		return fmt.Errorf("failed to set chunk status: %w", err)
	}

	// 验证设置是否成功
	bitrecord, err = redisDBVideoUpload.GetBit(bitKey, chunkNumber).Result()
	if err != nil {
		hlog.Errorf("DEBUG: Failed to verify chunk status: %v", err)
		return fmt.Errorf("test failed to get chunk status: %w", err)
	}
	hlog.Info("test:", bitrecord)
	hlog.Infof("DEBUG: Bit successfully set, verified value: %d", bitrecord)

	hlog.Infof("Updated chunk %d status for session %s", chunkNumber, uuid)
	return nil
}

// IsAllChunksUploadedV2 检查所有分片是否都已上传（V2版本专用）
func IsAllChunksUploadedV2(ctx context.Context, uuid, uid string) (bool, error) {
	hlog.Infof("DEBUG: IsAllChunksUploadedV2 called for session %s", uuid)

	// 获取总分片数
	info, err := GetChunkInfo(uid, uuid)
	if err != nil {
		hlog.Errorf("DEBUG: Failed to get chunk info: %v", err)
		return false, err
	}

	chunkTotalNumber, err := strconv.ParseInt(info[0], 10, 64)
	if err != nil {
		hlog.Errorf("DEBUG: Invalid total chunks: %s", info[0])
		return false, fmt.Errorf("invalid total chunks: %s", info[0])
	}
	hlog.Infof("DEBUG: Total chunks to check: %d", chunkTotalNumber)

	// 计算已上传的分片数
	// 注意：分片编号从1开始，需要检查位置1到chunkTotalNumber
	recordNumber := int64(0)
	bitKey := "b:" + uid + ":" + uuid
	hlog.Infof("DEBUG: Checking BitMap at key: %s", bitKey)

	for i := int64(1); i <= chunkTotalNumber; i++ {
		bit, err := redisDBVideoUpload.GetBit(bitKey, i).Result()
		if err != nil {
			hlog.Warnf("Failed to get bit %d for session %s: %v", i, uuid, err)
			continue
		}
		hlog.Infof("DEBUG: Bit %d value: %d", i, bit)
		if bit == 1 {
			recordNumber++
		}
	}

	allUploaded := chunkTotalNumber == recordNumber
	hlog.Infof("DEBUG: Found %d/%d chunks uploaded", recordNumber, chunkTotalNumber)
	hlog.Infof("Session %s: %d/%d chunks uploaded, all complete: %v", uuid, recordNumber, chunkTotalNumber, allUploaded)

	return allUploaded, nil
}

// CreateVideoEventV2 创建视频上传事件（V2版本专用，支持自定义UUID）
func CreateVideoEventV2(ctx context.Context, title, description, uid, customUUID, chunkTotalNumber, labelName, category string) error {
	// 检查UUID是否已存在
	exist, err := redisDBVideoUpload.Exists("l:" + uid + ":" + customUUID).Result()
	if err != nil {
		return fmt.Errorf("failed to check UUID existence: %w", err)
	}

	if exist != 0 {
		hlog.Warnf("Video event already exists for UUID %s, updating", customUUID)
		// 如果已存在，删除旧记录
		if err := DeleteVideoEvent(ctx, customUUID, uid); err != nil {
			hlog.Warnf("Failed to delete existing event: %v", err)
		}
	}

	// 创建新的视频事件
	if _, err := redisDBVideoUpload.RPush("l:"+uid+":"+customUUID, chunkTotalNumber, title, description, labelName, category).Result(); err != nil {
		return fmt.Errorf("failed to create video event: %w", err)
	}

	hlog.Infof("Created video event V2 for UUID %s, total chunks: %s", customUUID, chunkTotalNumber)
	return nil
}

// GetUploadSessionInfoV2 获取上传会话完整信息（V2版本专用）
func GetUploadSessionInfoV2(ctx context.Context, uuid, uid string) (map[string]interface{}, error) {
	// 获取基本信息
	info, err := GetChunkInfo(uid, uuid)
	if err != nil {
		return nil, err
	}

	// 获取上传状态
	uploadedChunks, err := GetUploadedChunksStatus(ctx, uuid, uid)
	if err != nil {
		return nil, err
	}

	totalChunks, _ := strconv.Atoi(info[0])
	uploadedCount := countTrueBits(uploadedChunks)

	sessionInfo := map[string]interface{}{
		"uuid":            uuid,
		"total_chunks":    totalChunks,
		"uploaded_chunks": uploadedChunks,
		"uploaded_count":  uploadedCount,
		"progress":        float64(uploadedCount) / float64(totalChunks) * 100,
		"title":           info[1],
		"description":     info[2],
		"label_name":      info[3],
		"category":        info[4],
		"is_complete":     uploadedCount == totalChunks,
	}

	return sessionInfo, nil
}

// 辅助函数：计算布尔切片中true的数量
func countTrueBits(bits []bool) int {
	count := 0
	for _, bit := range bits {
		if bit {
			count++
		}
	}
	return count
}
