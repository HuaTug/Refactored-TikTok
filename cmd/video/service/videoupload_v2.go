package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	redis "HuaTug.com/cmd/video/cache"
	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/oss"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// VideoUploadServiceV2 基于TikTok存储架构的新版上传服务
type VideoUploadServiceV2 struct {
	ctx           context.Context
	tikTokStorage *oss.TikTokStorage
	sessionCache  sync.Map      // 会话缓存，避免重复创建MinIO UploadID
	cleanupStopCh chan struct{} // 停止清理任务的通道
}

func NewVideoUploadServiceV2(ctx context.Context) *VideoUploadServiceV2 {
	service := &VideoUploadServiceV2{
		ctx:           ctx,
		tikTokStorage: oss.NewTikTokStorage(),
		cleanupStopCh: make(chan struct{}),
	}
	// 启动后台清理任务
	go service.startCleanupTask()
	return service
}

// UploadSession 上传会话
type UploadSession struct {
	UUID           string    `json:"uuid"`
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Category       string    `json:"category"`
	Tags           string    `json:"tags"`
	TotalChunks    int       `json:"total_chunks"`
	UploadedChunks []bool    `json:"uploaded_chunks"`
	TempDir        string    `json:"temp_dir"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Status         string    `json:"status"` // pending, uploading, processing, completed, failed

	// MinIO 分片上传相关字段
	MinIOUploadID string                      `json:"minio_upload_id"`
	BucketName    string                      `json:"bucket_name"`
	ObjectName    string                      `json:"object_name"`
	ContentType   string                      `json:"content_type"`
	UploadedParts map[int]oss.MinIOObjectPart `json:"uploaded_parts"` // partNumber -> ObjectPart
	ChunkSize     int64                       `json:"chunk_size"`     // 每个分片的大小
}

// StartUpload 开始上传流程（TikTok风格）
func (s *VideoUploadServiceV2) StartUpload(req *videos.VideoPublishStartRequestV2) (*UploadSession, error) {
	hlog.Infof("Starting video upload for user %d: title='%s', chunks=%d", req.UserId, req.Title, req.ChunkTotalNumber)

	// 1. 参数验证
	if req.Title == "" || req.ChunkTotalNumber <= 0 {
		hlog.Errorf("Invalid upload request: title='%s', chunks=%d", req.Title, req.ChunkTotalNumber)
		return nil, errno.RequestErr
	}

	// 2. 检查用户存储配额
	if err := s.checkUserStorageQuota(req.UserId); err != nil {
		hlog.Errorf("Storage quota check failed for user %d: %v", req.UserId, err)
		return nil, fmt.Errorf("storage quota exceeded: %w", err)
	}

	// 3. 生成video_id
	videoID, err := db.GetMaxVideoId(s.ctx, req.UserId)
	if err != nil {
		hlog.Errorf("Failed to generate video_id for user %d: %v", req.UserId, err)
		return nil, fmt.Errorf("failed to generate video_id: %w", err)
	}

	// 4. 创建上传会话（先生成UUID，统一以UUID作为临时目录标识）
	genUUID := s.generateUUID()
	hlog.Infof("Generated upload session UUID: %s for user %d", genUUID, req.UserId)

	// 5. 初始化MinIO分片上传
	bucketName := oss.BUCKET_USER_CONTENT
	objectName := s.tikTokStorage.GenerateVideoObjectName(req.UserId, parseVideoID(videoID))
	contentType := "video/mp4"

	hlog.Infof("Initializing MinIO multipart upload: bucket=%s, object=%s", bucketName, objectName)
	minioUploadID, err := s.tikTokStorage.CreateMultipartUpload(s.ctx, bucketName, objectName, contentType)
	if err != nil {
		hlog.Errorf("Failed to create MinIO multipart upload for user %d: %v", req.UserId, err)
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}

	// 6. 计算最优分片大小（假设每个分片5MB）
	chunkSize := int64(5 * 1024 * 1024) // 5MB

	// 7. 创建临时目录
	tempDir := s.createTempDir(req.UserId, genUUID)
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		hlog.Errorf("Failed to create temp directory %s: %v", tempDir, err)
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	session := &UploadSession{
		UUID:           genUUID,
		UserID:         req.UserId,
		VideoID:        parseVideoID(videoID),
		Title:          req.Title,
		Description:    req.Description,
		Category:       req.Category,
		Tags:           strings.Join(req.Tags, ","),
		TotalChunks:    int(req.ChunkTotalNumber),
		UploadedChunks: make([]bool, req.ChunkTotalNumber),
		TempDir:        tempDir,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour), // 24小时过期
		Status:         "pending",

		// MinIO 分片上传相关字段
		MinIOUploadID: minioUploadID,
		BucketName:    bucketName,
		ObjectName:    objectName,
		ContentType:   contentType,
		UploadedParts: make(map[int]oss.MinIOObjectPart),
		ChunkSize:     chunkSize,
	}

	// 7. Ensure user directory structure exists in MinIO
	if err := s.tikTokStorage.EnsureUserDirectoryStructure(s.ctx, req.UserId); err != nil {
		hlog.Warnf("Failed to ensure user directory structure for user %d: %v (non-blocking)", req.UserId, err)
		// Non-blocking: directory will be created on first upload if missing
	}

	// 8. 保存会话到Redis
	if err := s.saveUploadSession(session); err != nil {
		// 保存失败时立即清理临时目录
		go s.cleanupTempFiles(tempDir)
		return nil, fmt.Errorf("failed to save upload session: %w", err)
	}

	hlog.Infof("Started upload session %s for user %d, video %d, MinIO UploadID: %s",
		session.UUID, req.UserId, session.VideoID, session.MinIOUploadID)
	return session, nil
}

// UploadChunk 上传分片（MinIO分片上传版本）
func (s *VideoUploadServiceV2) UploadChunk(req *videos.VideoPublishUploadingRequestV2) error {
	hlog.Infof("Starting MinIO chunk upload %d for session %s (size: %d bytes)",
		req.ChunkNumber, req.UploadSessionUuid, len(req.ChunkData))

	// 1. 基本参数验证
	if req.ChunkNumber <= 0 {
		hlog.Errorf("Invalid chunk number %d for session %s", req.ChunkNumber, req.UploadSessionUuid)
		return fmt.Errorf("invalid chunk number %d", req.ChunkNumber)
	}

	if len(req.ChunkData) == 0 {
		hlog.Errorf("Empty chunk data for chunk %d, session %s", req.ChunkNumber, req.UploadSessionUuid)
		return fmt.Errorf("empty chunk data for chunk %d", req.ChunkNumber)
	}

	// 2. 验证分片数据
	if !s.verifyChunk(req.ChunkData, req.ChunkMd5) {
		hlog.Errorf("Chunk verification failed for chunk %d, session %s", req.ChunkNumber, req.UploadSessionUuid)
		return errors.New("chunk verification failed")
	}

	// 3. 获取上传会话信息（需要MinIO相关参数）
	session, err := s.getUploadSession(req.UploadSessionUuid, req.UserId)
	if err != nil {
		hlog.Errorf("Failed to get upload session %s for user %d: %v", req.UploadSessionUuid, req.UserId, err)
		return fmt.Errorf("failed to get upload session: %w", err)
	}

	// 4. 如果会话恢复但MinIO UploadID缺失，重新创建MinIO上传会话
	if session.MinIOUploadID == "" {
		hlog.Infof("MinIO UploadID missing for session %s, creating new multipart upload", req.UploadSessionUuid)

		// 重新创建MinIO分片上传
		minioUploadID, err := s.tikTokStorage.CreateMultipartUpload(s.ctx, session.BucketName, session.ObjectName, session.ContentType)
		if err != nil {
			hlog.Errorf("Failed to recreate MinIO multipart upload for session %s: %v", req.UploadSessionUuid, err)
			return fmt.Errorf("failed to recreate multipart upload: %w", err)
		}

		session.MinIOUploadID = minioUploadID
		hlog.Infof("Recreated MinIO UploadID for session %s: %s", req.UploadSessionUuid, minioUploadID)

		// 更新会话并保存到内存中，避免重复创建
		if err := s.saveUploadSession(session); err != nil {
			hlog.Warnf("Failed to save session after recreating MinIO UploadID: %v", err)
		}

		// 重要：尝试从MinIO恢复已上传的分片信息
		if parts, err := s.tikTokStorage.ListParts(s.ctx, session.BucketName, session.ObjectName, session.MinIOUploadID); err == nil {
			for _, part := range parts {
				session.UploadedParts[part.PartNumber] = part
			}
			hlog.Infof("Recovered %d existing parts for session %s", len(parts), req.UploadSessionUuid)
		} else {
			hlog.Warnf("Failed to list existing parts for new UploadID: %v", err)
		}
	} // 5. 检查分片是否已经上传过
	if int(req.ChunkNumber) <= len(session.UploadedChunks) && session.UploadedChunks[req.ChunkNumber-1] {
		hlog.Infof("Chunk %d already uploaded for session %s, skipping", req.ChunkNumber, req.UploadSessionUuid)
		return nil
	}

	// 6. 直接上传分片到MinIO
	chunkReader := bytes.NewReader(req.ChunkData)
	chunkSize := int64(len(req.ChunkData))

	part, err := s.tikTokStorage.UploadPart(
		s.ctx,
		session.BucketName,
		session.ObjectName,
		session.MinIOUploadID,
		int(req.ChunkNumber),
		chunkReader,
		chunkSize,
	)
	if err != nil {
		hlog.Errorf("Failed to upload chunk %d to MinIO for session %s: %v", req.ChunkNumber, req.UploadSessionUuid, err)
		// 如果这是第一个分片且上传失败，可能是会话配置错误，标记会话为失败
		if req.ChunkNumber == 1 {
			session.Status = "failed"
			s.saveUploadSession(session)
			// 异步清理临时目录
			go s.cleanupTempFiles(session.TempDir)
			go s.deleteUploadSession(session.UUID, session.UserID)
		}
		return fmt.Errorf("failed to upload chunk to MinIO: %w", err)
	}

	hlog.Infof("Successfully uploaded chunk %d to MinIO: ETag=%s, Size=%d bytes",
		req.ChunkNumber, part.ETag, part.Size)

	// 7. 更新会话中的分片信息（不保存原始数据，分片已在MinIO临时目录中）
	session.UploadedParts[int(req.ChunkNumber)] = part
	if int(req.ChunkNumber) <= len(session.UploadedChunks) {
		session.UploadedChunks[req.ChunkNumber-1] = true // 数组是0索引的
	}

	// 8. 保存更新后的会话
	if err := s.saveUploadSession(session); err != nil {
		hlog.Errorf("Failed to update session after chunk upload (non-blocking): %v", err)
		// 不阻塞主流程，分片已经上传成功
	}

	// 9. 更新Redis中的分片状态
	uid := strconv.FormatInt(req.UserId, 10)
	if err := redis.UpdateChunkUploadStatus(s.ctx, req.UploadSessionUuid, uid, int64(req.ChunkNumber)); err != nil {
		hlog.Errorf("Failed to update chunk status in Redis (non-blocking): %v", err)
		// 不阻塞主流程
	}

	hlog.Infof("Successfully uploaded MinIO chunk %d for session %s, ETag: %s",
		req.ChunkNumber, req.UploadSessionUuid, part.ETag)
	return nil
}

// CompleteUpload 完成上传（TikTok风格处理）
func (s *VideoUploadServiceV2) CompleteUpload(req *videos.VideoPublishCompleteRequestV2) (*videos.VideoPublishCompleteResponseV2, error) {
	hlog.Infof("Starting complete upload for session %s, user %d", req.UploadSessionUuid, req.UserId)

	// 1. 获取上传会话
	session, err := s.getUploadSession(req.UploadSessionUuid, req.UserId)
	if err != nil {
		hlog.Errorf("Failed to get upload session %s: %v", req.UploadSessionUuid, err)
		return nil, fmt.Errorf("failed to get upload session: %w", err)
	}

	uploadedCount := s.countUploadedChunks(session.UploadedChunks)
	hlog.Infof("Retrieved session %s: %d total chunks, local status: %d/%d uploaded",
		session.UUID, session.TotalChunks, uploadedCount, session.TotalChunks)

	// 2. 验证所有分片都已上传（增强检查）
	allUploaded := s.allChunksUploaded(session)
	if !allUploaded {
		// 详细的错误信息
		hlog.Errorf("Not all chunks uploaded for session %s: %d/%d chunks uploaded",
			session.UUID, uploadedCount, session.TotalChunks)

		// 打印缺失的分片信息
		var missingChunks []int
		for i, uploaded := range session.UploadedChunks {
			if !uploaded {
				missingChunks = append(missingChunks, i+1)
			}
		}
		hlog.Errorf("Missing chunks for session %s: %v", session.UUID, missingChunks)

		return nil, fmt.Errorf("not all chunks have been uploaded: %d/%d uploaded, missing chunks: %v",
			uploadedCount, session.TotalChunks, missingChunks)
	}

	hlog.Infof("All chunks verified for session %s, proceeding with MinIO merge", session.UUID)

	// 3. 更新会话状态为处理中
	session.Status = "processing"
	if err := s.saveUploadSession(session); err != nil {
		hlog.Warnf("Failed to update session status to processing: %v", err)
	}

	// 4. 使用MinIO合并分片
	hlog.Infof("Merging MinIO chunks for session %s", session.UUID)

	// 准备分片列表（确保按顺序排列）
	var parts []oss.MinIOObjectPart
	hlog.Infof("Session %s has %d uploaded parts in session", session.UUID, len(session.UploadedParts))

	for i := 1; i <= session.TotalChunks; i++ {
		if part, exists := session.UploadedParts[i]; exists {
			hlog.Infof("Found part %d: ETag=%s, Size=%d", i, part.ETag, part.Size)
			parts = append(parts, part)
		} else {
			// 如果某个分片不存在，这是严重错误
			hlog.Errorf("Part %d not found in session %s uploaded parts, available parts: %v",
				i, session.UUID, getUploadedPartsKeys(session.UploadedParts))
			session.Status = "failed"
			s.saveUploadSession(session)
			return nil, fmt.Errorf("missing part %d in uploaded parts", i)
		}
	}

	hlog.Infof("Prepared %d parts for MinIO merge operation", len(parts))

	// 执行MinIO分片合并
	err = s.tikTokStorage.CompleteMultipartUpload(
		s.ctx,
		session.BucketName,
		session.ObjectName,
		session.MinIOUploadID,
		parts,
	)
	if err != nil {
		session.Status = "failed"
		s.saveUploadSession(session)
		hlog.Errorf("Failed to complete MinIO multipart upload for session %s: %v", session.UUID, err)
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	hlog.Infof("Successfully completed MinIO multipart upload for session %s", session.UUID)

	// 4. 获取合并后的视频信息（MinIO中的文件已经合并完成）
	// 构造视频URL - 使用相对路径，前端通过 Vite 代理访问 MinIO
	videoURL := fmt.Sprintf("/%s/%s", session.BucketName, session.ObjectName)

	// 5. 同步生成视频缩略图并上传到 MinIO
	thumbnailURL := videoURL // 默认使用视频URL作为封面
	thumbnailURLs := map[string]string{"medium": videoURL}

	// 同步生成缩略图
	thumbURL, err := s.generateAndUploadThumbnail(session.UserID, session.VideoID, session.BucketName, session.ObjectName)
	if err != nil {
		hlog.Warnf("Failed to generate thumbnail for video %d: %v, using video URL as cover", session.VideoID, err)
	} else {
		hlog.Infof("Successfully generated thumbnail for video %d: %s", session.VideoID, thumbURL)
		thumbnailURL = thumbURL
		thumbnailURLs["medium"] = thumbURL
	}

	// 视频处理响应
	uploadResp := &oss.VideoUploadResponse{
		VideoID:          session.VideoID,
		SourceURL:        videoURL,
		ProcessedURLs:    map[int]string{720: videoURL},
		ThumbnailURLs:    thumbnailURLs,
		AnimatedCoverURL: videoURL,
		MetadataURL:      "",
	}

	// 6. 获取视频文件大小（从MinIO）
	objectInfo, err := s.tikTokStorage.GetObjectInfo(s.ctx, session.BucketName, session.ObjectName)
	var fileSize int64 = 0
	if err != nil {
		hlog.Warnf("Failed to get object info from MinIO: %v", err)
		// 估算文件大小（分片大小 * 分片数量）
		fileSize = session.ChunkSize * int64(session.TotalChunks)
	} else {
		fileSize = objectInfo.Size
	}

	// 7. 创建存储映射记录
	storageMapping := &db.VideoStorageMapping{
		UserID:            session.UserID,
		VideoID:           session.VideoID,
		SourcePath:        uploadResp.SourceURL,
		ProcessedPaths:    s.convertProcessedPaths(uploadResp.ProcessedURLs),
		ThumbnailPaths:    s.convertThumbnailPaths(uploadResp.ThumbnailURLs),
		AnimatedCoverPath: uploadResp.AnimatedCoverURL,
		MetadataPath:      uploadResp.MetadataURL,
		StorageStatus:     "completed",
		HotStorage:        false,
		BucketName:        oss.BUCKET_USER_CONTENT,
		FileSize:          fileSize,
		Duration:          120,  // 默认120秒，后续可以通过视频分析获取
		ResolutionWidth:   1280, // 默认分辨率
		ResolutionHeight:  720,
		Format:            "mp4",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := db.CreateVideoStorageMapping(s.ctx, storageMapping); err != nil {
		hlog.Errorf("Failed to create storage mapping for session %s: %v", session.UUID, err)
		// 不阻塞主流程，可以后续补偿
	}

	// 8. 创建视频记录
	video := &base.Video{
		Title:       session.Title,
		Description: session.Description,
		UserId:      session.UserID,
		VisitCount:  0,
		LabelNames:  session.Tags,
		Category:    session.Category,
		CreatedAt:   time.Now().Format(constants.DataFormate),
		UpdatedAt:   time.Now().Format(constants.DataFormate),
		VideoUrl:    uploadResp.ProcessedURLs[720],
		CoverUrl:    thumbnailURL, // 使用生成的缩略图URL
	}

	if err := db.InsertVideo(s.ctx, video); err != nil {
		session.Status = "failed"
		s.saveUploadSession(session)
		hlog.Errorf("Failed to save video record for session %s: %v", session.UUID, err)
		return nil, fmt.Errorf("failed to save video record: %w", err)
	}

	// 8. 更新用户存储配额
	if err := s.updateUserStorageUsage(session.UserID, fileSize); err != nil {
		hlog.Warnf("Failed to update user storage usage for session %s: %v", session.UUID, err)
	}

	// 9. 更新用户视频数
	if err := db.DB.WithContext(s.ctx).Exec(
		"UPDATE users SET video_count = video_count + 1 WHERE user_id = ?", session.UserID,
	).Error; err != nil {
		hlog.Warnf("Failed to update user video_count for session %s: %v", session.UUID, err)
	}

	// 10. 接入推荐系统：初始化视频特征、标签映射、分类统计、作者评分
	OnVideoPublished(s.ctx, session.VideoID, session.UserID, session.Title, session.Description, session.Tags, session.Category)

	// 8. 清理临时文件和会话
	session.Status = "completed"
	s.saveUploadSession(session)

	go func() {
		s.cleanupTempFiles(session.TempDir)
		s.deleteUploadSession(session.UUID, session.UserID)
	}()

	hlog.Infof("Successfully completed upload for session %s, video %d", session.UUID, session.VideoID)

	// 获取用户存储配额信息
	quota, err := s.GetUserStorageQuota(session.UserID)
	if err != nil {
		hlog.Warnf("Failed to get user storage quota: %v", err)
		quota = &videos.UserStorageQuota{} // 返回空配额
	}

	// 返回完整的响应对象
	resp := &videos.VideoPublishCompleteResponseV2{
		Base: &base.Status{
			Code: 200,
			Msg:  "Video Publish Completed Successfully (V2 TikTok Style)",
		},
		VideoId:            session.VideoID,
		VideoSourceUrl:     uploadResp.SourceURL,
		ProcessedVideoUrls: convertToInt32Map(uploadResp.ProcessedURLs),
		ThumbnailUrls:      uploadResp.ThumbnailURLs,
		AnimatedCoverUrl:   uploadResp.AnimatedCoverURL,
		MetadataUrl:        uploadResp.MetadataURL,
		ProcessingStatus:   "completed",
		ProcessingJobId:    0, // 暂时没有处理任务ID
		UpdatedQuota:       quota,
	}
	return resp, nil
}

// CancelUpload 取消上传
func (s *VideoUploadServiceV2) CancelUpload(req *videos.VideoPublishCancelRequestV2) error {
	session, err := s.getUploadSession(req.UploadSessionUuid, req.UserId)
	if err != nil {
		return fmt.Errorf("failed to get upload session: %w", err)
	}

	// 取消MinIO分片上传
	if session.MinIOUploadID != "" {
		err = s.tikTokStorage.AbortMultipartUpload(
			s.ctx,
			session.BucketName,
			session.ObjectName,
			session.MinIOUploadID,
		)
		if err != nil {
			hlog.Errorf("Failed to abort MinIO multipart upload for session %s: %v", session.UUID, err)
			// 不阻塞主流程
		} else {
			hlog.Infof("Successfully aborted MinIO multipart upload for session %s", session.UUID)
		}
	}

	session.Status = "cancelled"
	s.saveUploadSession(session)

	// 异步清理
	go func() {
		s.cleanupTempFiles(session.TempDir)
		s.deleteUploadSession(session.UUID, session.UserID)
	}()

	hlog.Infof("Cancelled upload session %s", session.UUID)
	return nil
}

// 辅助方法
func (s *VideoUploadServiceV2) checkUserStorageQuota(userID int64) error {
	quota, err := db.GetUserStorageQuota(s.ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user storage quota: %w", err)
	}

	if quota.QuotaExceeded {
		return errors.New("storage quota exceeded")
	}

	return nil
}

func (s *VideoUploadServiceV2) generateUUID() string {
	// 实现UUID生成逻辑 - 使用时间戳和随机数./scripts/start_services.sh
	return fmt.Sprintf("v2_%d_%d", time.Now().UnixNano()/1000000, time.Now().Nanosecond()%1000000)
}

// createTempDir 使用与V1一致的会话目录结构：{uid}_{uuid}
func (s *VideoUploadServiceV2) createTempDir(userID int64, uuid string) string {
	// 放在工作目录，便于与现有逻辑共享和排查
	return fmt.Sprintf("%d_%s", userID, uuid)
}

func (s *VideoUploadServiceV2) saveUploadSession(session *UploadSession) error {
	// 更新内存缓存
	cacheKey := fmt.Sprintf("%d:%s", session.UserID, session.UUID)
	s.sessionCache.Store(cacheKey, session)

	// 使用专门的V2版本Redis方法创建或更新会话
	err := redis.CreateVideoEventV2(s.ctx,
		session.Title,
		session.Description,
		strconv.FormatInt(session.UserID, 10),
		session.UUID,
		strconv.Itoa(session.TotalChunks),
		session.Tags,
		session.Category)
	if err != nil {
		hlog.Errorf("Failed to save upload session %s: %v", session.UUID, err)
		return fmt.Errorf("failed to save upload session: %w", err)
	}

	// 保存完整的会话状态到Redis（包括MinIO UploadID和UploadedParts）
	sessionKey := fmt.Sprintf("video_session:%s:%d", session.UUID, session.UserID)
	sessionData, err := json.Marshal(session)
	if err != nil {
		hlog.Errorf("Failed to marshal session data: %v", err)
	} else {
		if err := redis.SaveSessionData(s.ctx, sessionKey, string(sessionData)); err != nil {
			hlog.Errorf("Failed to save session data to Redis: %v", err)
		}
	}

	hlog.Infof("MinIO UploadID for session %s: %s", session.UUID, session.MinIOUploadID)
	hlog.Infof("Successfully saved upload session %s to Redis and cache", session.UUID)
	return nil
}

func (s *VideoUploadServiceV2) getUploadSession(uuid string, userID int64) (*UploadSession, error) {
	uid := strconv.FormatInt(userID, 10)

	// 首先检查内存缓存
	cacheKey := fmt.Sprintf("%d:%s", userID, uuid)
	if cached, ok := s.sessionCache.Load(cacheKey); ok {
		if session, ok := cached.(*UploadSession); ok {
			hlog.Infof("Retrieved session %s from cache: %d/%d chunks uploaded",
				uuid, s.countUploadedChunks(session.UploadedChunks), session.TotalChunks)
			return session, nil
		}
	}

	// 获取基本信息
	info, err := redis.GetChunkInfo(uid, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk info: %w", err)
	}

	totalChunks, err := strconv.Atoi(info[0])
	if err != nil {
		return nil, fmt.Errorf("invalid total chunks: %s", info[0])
	}

	// 创建默认的上传状态（所有分片都未上传）
	uploadedChunks := make([]bool, totalChunks)

	session := &UploadSession{
		UUID:           uuid,
		UserID:         userID,
		Title:          info[1],
		Description:    info[2],
		Tags:           info[3],
		Category:       info[4],
		TotalChunks:    totalChunks,
		Status:         "uploading",
		UploadedChunks: uploadedChunks,
		TempDir:        s.createTempDir(userID, uuid),

		// MinIO相关字段：初始化为空，稍后会设置
		MinIOUploadID: "",
		BucketName:    oss.BUCKET_USER_CONTENT,
		ObjectName:    s.tikTokStorage.GenerateVideoObjectName(userID, 0), // VideoID未知，使用0
		ContentType:   "video/mp4",
		UploadedParts: make(map[int]oss.MinIOObjectPart), // 空的分片映射
		ChunkSize:     5 * 1024 * 1024,                   // 默认5MB
	}

	// 尝试从Redis恢复MinIO会话状态
	sessionKey := fmt.Sprintf("video_session:%s:%d", uuid, userID)
	if sessionData, err := redis.GetSessionData(s.ctx, sessionKey); err == nil && sessionData != "" {
		hlog.Infof("Found existing session data in Redis for %s", uuid)
		var savedSession UploadSession
		if err := json.Unmarshal([]byte(sessionData), &savedSession); err == nil {
			session.MinIOUploadID = savedSession.MinIOUploadID
			session.UploadedParts = savedSession.UploadedParts
			session.VideoID = savedSession.VideoID
			if savedSession.ObjectName != "" {
				session.ObjectName = savedSession.ObjectName
			}
			// 恢复 UploadedChunks 数组状态
			if len(savedSession.UploadedChunks) == totalChunks {
				session.UploadedChunks = savedSession.UploadedChunks
			}
			hlog.Infof("Restored MinIO UploadID: %s, UploadedParts: %d, UploadedChunks: %d/%d",
				session.MinIOUploadID, len(session.UploadedParts),
				s.countUploadedChunks(session.UploadedChunks), totalChunks)
		} else {
			hlog.Errorf("Failed to unmarshal session data: %v", err)
		}
	}

	// 存储到缓存
	s.sessionCache.Store(cacheKey, session)

	hlog.Infof("Retrieved upload session %s: %d/%d chunks uploaded", uuid, s.countUploadedChunks(uploadedChunks), totalChunks)
	return session, nil
}

func (s *VideoUploadServiceV2) deleteUploadSession(uuid string, userID int64) error {
	// 清理内存缓存
	cacheKey := fmt.Sprintf("%d:%s", userID, uuid)
	s.sessionCache.Delete(cacheKey)

	// 删除Redis中的会话
	return redis.DeleteVideoEvent(s.ctx, uuid, strconv.FormatInt(userID, 10))
}

func (s *VideoUploadServiceV2) verifyChunk(data []byte, expectedMD5 string) bool {
	if expectedMD5 == "" {
		return true
	}

	// 计算实际的MD5
	hash := md5.Sum(data)
	actualMD5 := fmt.Sprintf("%x", hash)

	isValid := actualMD5 == expectedMD5
	if !isValid {
		hlog.Errorf("Chunk MD5 verification failed: expected=%s, actual=%s", expectedMD5, actualMD5)
	}

	return isValid
}

func (s *VideoUploadServiceV2) allChunksUploaded(session *UploadSession) bool {
	// 使用Redis验证，确保数据一致性
	uid := strconv.FormatInt(session.UserID, 10)
	hlog.Info("Sessin.UUID and Uid is", session.UUID, uid)
	allUploaded, err := redis.IsAllChunksUploadedV2(s.ctx, session.UUID, uid)
	if err != nil {
		hlog.Errorf("Failed to check chunks status from Redis for session %s: %v", session.UUID, err)
		// 降级到本地检查
		for _, uploaded := range session.UploadedChunks {
			if !uploaded {
				return false
			}
		}
		return true
	}

	hlog.Infof("Session %s all chunks uploaded check: %v", session.UUID, allUploaded)
	return allUploaded
}

func (s *VideoUploadServiceV2) convertProcessedPaths(urls map[int]string) db.JSON {
	result := make(db.JSON)
	for quality, url := range urls {
		result[strconv.Itoa(quality)] = url
	}
	return result
}

func (s *VideoUploadServiceV2) convertThumbnailPaths(urls map[string]string) db.JSON {
	result := make(db.JSON)
	for size, url := range urls {
		result[size] = url
	}
	return result
}

func (s *VideoUploadServiceV2) updateUserStorageUsage(userID int64, fileSize int64) error {
	return db.UpdateUserStorageUsage(s.ctx, userID, fileSize, 1)
}

func (s *VideoUploadServiceV2) cleanupTempFiles(tempDir string) {
	// 检查目录是否存在
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		hlog.Infof("Temp directory %s does not exist, skipping cleanup", tempDir)
		return
	}

	// 尝试删除目录
	if err := os.RemoveAll(tempDir); err != nil {
		hlog.Errorf("Failed to cleanup temp directory %s: %v", tempDir, err)
		return
	}

	hlog.Infof("Successfully cleaned up temp directory: %s", tempDir)
}

func parseVideoID(vid string) int64 {
	id, _ := strconv.ParseInt(vid, 10, 64)
	return id
}

// getUploadedPartsKeys 获取已上传分片的编号列表
func getUploadedPartsKeys(parts map[int]oss.MinIOObjectPart) []int {
	keys := make([]int, 0, len(parts))
	for k := range parts {
		keys = append(keys, k)
	}
	return keys
}

// GetUserStorageQuota retrieves real user storage quota from database
func (s *VideoUploadServiceV2) GetUserStorageQuota(userID int64) (*videos.UserStorageQuota, error) {
	// 1. Try to get quota from database first
	dbQuota, err := db.GetUserStorageQuota(s.ctx, userID)
	if err == nil && dbQuota != nil {
		return &videos.UserStorageQuota{
			TotalQuotaBytes:   dbQuota.MaxStorageBytes,
			UsedQuotaBytes:    dbQuota.UsedStorageBytes,
			VideoCount:        int64(dbQuota.VideoCount),
			QuotaLevel:        dbQuota.QuotaLevel,
			MaxVideoSizeBytes: dbQuota.MaxVideoSize,
			MaxVideoCount:     int32(dbQuota.MaxVideoCount),
		}, nil
	}

	hlog.Warnf("Failed to get quota from DB for user %d: %v, using default values", userID, err)

	// 2. Fallback: determine quota from user level with defaults
	quotaLevel := s.determineQuotaLevel(userID)
	totalQuota := s.getTotalQuotaByLevel(quotaLevel)
	maxVideoSize := s.getMaxVideoSizeByLevel(quotaLevel)
	maxVideoCount := s.getMaxVideoCountByLevel(quotaLevel)

	// 3. Try to get actual usage from storage mappings
	usedStorage := int64(0)
	videoCount := int64(0)
	userVideos, err := db.GetUserVideos(s.ctx, userID, 10000, 0)
	if err == nil {
		videoCount = int64(len(userVideos))
		for _, v := range userVideos {
			usedStorage += v.FileSize
		}
	}

	return &videos.UserStorageQuota{
		TotalQuotaBytes:   totalQuota,
		UsedQuotaBytes:    usedStorage,
		VideoCount:        videoCount,
		QuotaLevel:        quotaLevel,
		MaxVideoSizeBytes: maxVideoSize,
		MaxVideoCount:     int32(maxVideoCount),
	}, nil
}

// determineQuotaLevel determines user quota level based on DB record or video count
func (s *VideoUploadServiceV2) determineQuotaLevel(userID int64) string {
	// Try to get quota level from database
	dbQuota, err := db.GetUserStorageQuota(s.ctx, userID)
	if err == nil && dbQuota != nil && dbQuota.QuotaLevel != "" {
		return dbQuota.QuotaLevel
	}

	// Fallback: determine by user's video count as a heuristic
	userVideos, err := db.GetUserVideos(s.ctx, userID, 1, 0)
	if err == nil {
		count := len(userVideos)
		switch {
		case count >= 500:
			return "enterprise"
		case count >= 100:
			return "premium"
		case count >= 10:
			return "standard"
		default:
			return "basic"
		}
	}

	return "standard"
}

// getTotalQuotaByLevel 根据等级获取总配额
func (s *VideoUploadServiceV2) getTotalQuotaByLevel(level string) int64 {
	switch level {
	case "basic":
		return 5 * 1024 * 1024 * 1024 // 5GB
	case "standard":
		return 10 * 1024 * 1024 * 1024 // 10GB
	case "premium":
		return 50 * 1024 * 1024 * 1024 // 50GB
	case "enterprise":
		return 200 * 1024 * 1024 * 1024 // 200GB
	default:
		return 10 * 1024 * 1024 * 1024 // 默认10GB
	}
}

// getMaxVideoSizeByLevel 根据等级获取单个视频最大大小
func (s *VideoUploadServiceV2) getMaxVideoSizeByLevel(level string) int64 {
	switch level {
	case "basic":
		return 500 * 1024 * 1024 // 500MB
	case "standard":
		return 1024 * 1024 * 1024 // 1GB
	case "premium":
		return 5 * 1024 * 1024 * 1024 // 5GB
	case "enterprise":
		return 10 * 1024 * 1024 * 1024 // 10GB
	default:
		return 1024 * 1024 * 1024 // 默认1GB
	}
}

// getMaxVideoCountByLevel 根据等级获取最大视频数量
func (s *VideoUploadServiceV2) getMaxVideoCountByLevel(level string) int64 {
	switch level {
	case "basic":
		return 50
	case "standard":
		return 100
	case "premium":
		return 500
	case "enterprise":
		return 2000
	default:
		return 100 // 默认100个
	}
}

// UploadProgressInfo 上传进度信息
type UploadProgressInfo struct {
	Status          string  `json:"status"`
	ProgressPercent float64 `json:"progress_percent"`
	NextChunkOffset int64   `json:"next_chunk_offset"`
	UploadSpeedMbps string  `json:"upload_speed_mbps"`
	UploadedChunks  int     `json:"uploaded_chunks"`
	TotalChunks     int     `json:"total_chunks"`
}

// GetUploadProgress 获取上传进度信息
func (s *VideoUploadServiceV2) GetUploadProgress(sessionUUID string, userID int64) (*UploadProgressInfo, error) {
	// 从Redis获取会话信息
	session, err := s.getUploadSession(sessionUUID, userID)
	if err != nil {
		hlog.Warnf("Failed to get session %s: %v", sessionUUID, err)
		// 返回基本进度信息，不阻塞主流程
		return &UploadProgressInfo{
			Status:          "uploading",
			ProgressPercent: 0,
			NextChunkOffset: 0,
			UploadSpeedMbps: "calculating",
		}, nil
	}

	// 计算已上传分片数
	uploadedCount := s.countUploadedChunks(session.UploadedChunks)
	totalChunks := session.TotalChunks

	// 计算进度百分比
	progressPercent := float64(uploadedCount) / float64(totalChunks) * 100

	// 计算下一个分片偏移量（简化实现）
	nextChunkOffset := int64(uploadedCount + 1)

	// 判断状态
	status := "uploading"
	if uploadedCount == totalChunks {
		status = "completed"
	} else if uploadedCount == 0 {
		status = "pending"
	}

	return &UploadProgressInfo{
		Status:          status,
		ProgressPercent: progressPercent,
		NextChunkOffset: nextChunkOffset,
		UploadSpeedMbps: s.calculateUploadSpeed(session),
		UploadedChunks:  uploadedCount,
		TotalChunks:     totalChunks,
	}, nil
}

// calculateUploadSpeed estimates upload speed based on session progress and elapsed time
func (s *VideoUploadServiceV2) calculateUploadSpeed(session *UploadSession) string {
	if session == nil || session.CreatedAt.IsZero() {
		return "N/A"
	}

	elapsed := time.Since(session.CreatedAt).Seconds()
	if elapsed <= 0 {
		return "N/A"
	}

	uploadedCount := s.countUploadedChunks(session.UploadedChunks)
	if uploadedCount == 0 {
		return "0.00"
	}

	// Estimate total bytes uploaded: uploaded chunks * chunk size
	uploadedBytes := float64(uploadedCount) * float64(session.ChunkSize)

	// Convert to Mbps (megabits per second)
	speedMbps := (uploadedBytes * 8) / (elapsed * 1024 * 1024)

	return fmt.Sprintf("%.2f", speedMbps)
}

// countUploadedChunks 计算已上传分片数量
func (s *VideoUploadServiceV2) countUploadedChunks(uploadedChunks []bool) int {
	count := 0
	for _, uploaded := range uploadedChunks {
		if uploaded {
			count++
		}
	}
	return count
}

// convertToInt32Map 将map[int]string转换为map[int32]string
func convertToInt32Map(m map[int]string) map[int32]string {
	result := make(map[int32]string)
	for k, v := range m {
		result[int32(k)] = v
	}
	return result
}

// startCleanupTask 启动后台清理任务，定期清理过期的临时目录
func (s *VideoUploadServiceV2) startCleanupTask() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时执行一次清理
	defer ticker.Stop()

	hlog.Info("Started temp directory cleanup task (runs every hour)")

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredTempDirs()
		case <-s.cleanupStopCh:
			hlog.Info("Stopped temp directory cleanup task")
			return
		case <-s.ctx.Done():
			hlog.Info("Context cancelled, stopping cleanup task")
			return
		}
	}
}

// cleanupExpiredTempDirs 清理过期的临时目录
func (s *VideoUploadServiceV2) cleanupExpiredTempDirs() {
	hlog.Info("Starting cleanup of expired temp directories")

	// 读取当前工作目录
	entries, err := os.ReadDir(".")
	if err != nil {
		hlog.Errorf("Failed to read current directory for cleanup: %v", err)
		return
	}

	now := time.Now()
	cleanedCount := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// 跳过非临时目录（临时目录格式：{uid}_v2_{timestamp}_{random} 或 {uid}_{uuid}）
		name := entry.Name()
		if !s.isTempDir(name) {
			continue
		}

		// 获取目录信息
		info, err := entry.Info()
		if err != nil {
			hlog.Warnf("Failed to get info for directory %s: %v", name, err)
			continue
		}

		// 检查是否过期（超过24小时未修改）
		if now.Sub(info.ModTime()) > 24*time.Hour {
			if err := os.RemoveAll(name); err != nil {
				hlog.Errorf("Failed to remove expired temp directory %s: %v", name, err)
			} else {
				hlog.Infof("Cleaned up expired temp directory: %s (age: %v)", name, now.Sub(info.ModTime()))
				cleanedCount++
			}
		}
	}

	hlog.Infof("Cleanup completed: cleaned %d expired temp directories", cleanedCount)
}

// isTempDir 判断是否是临时目录
func (s *VideoUploadServiceV2) isTempDir(name string) bool {
	// 匹配格式：{uid}_v2_{timestamp}_{random} 或 {uid}_{uuid}
	// 例如：1_v2_1769001157405_457000 或 1_uuid123

	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return false
	}

	// 第一部分应该是数字（用户ID）
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return false
	}

	// 检查第二部分是否包含 "v2" 或者是 UUID 格式
	if parts[1] == "v2" {
		return len(parts) >= 3 // 至少有三部分：uid_v2_...
	}

	// 简单的UUID检查：长度大于5且包含数字和字母
	if len(parts[1]) > 5 {
		return true
	}

	return false
}

// StopCleanupTask 停止清理任务（可在服务关闭时调用）
func (s *VideoUploadServiceV2) StopCleanupTask() {
	close(s.cleanupStopCh)
}

// generateAndUploadThumbnail 从MinIO下载视频，生成缩略图，然后上传到MinIO（使用service的ctx）
func (s *VideoUploadServiceV2) generateAndUploadThumbnail(userID, videoID int64, bucketName, objectName string) (string, error) {
	return s.generateAndUploadThumbnailAsync(s.ctx, userID, videoID, bucketName, objectName)
}

// generateAndUploadThumbnailAsync 从MinIO下载视频，生成缩略图，然后上传到MinIO（使用传入的ctx，适用于异步调用）
func (s *VideoUploadServiceV2) generateAndUploadThumbnailAsync(ctx context.Context, userID, videoID int64, bucketName, objectName string) (string, error) {
	hlog.Infof("Starting thumbnail generation for video %d", videoID)

	// 1. 创建临时目录
	tempDir := fmt.Sprintf("/tmp/thumbnail_%d_%d", userID, videoID)
	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir) // 清理临时目录

	// 2. 从MinIO下载视频到临时文件
	tempVideoPath := fmt.Sprintf("%s/video.mp4", tempDir)
	if err := s.tikTokStorage.DownloadFile(ctx, bucketName, objectName, tempVideoPath); err != nil {
		return "", fmt.Errorf("failed to download video from MinIO: %w", err)
	}
	hlog.Infof("Downloaded video to temp path: %s", tempVideoPath)

	// 3. 使用FFmpeg生成缩略图
	thumbnailPath, err := utils.GetVideoThumbnail(tempVideoPath, tempDir, 1, 320, 180) // 在第1秒截取，320x180尺寸
	if err != nil {
		return "", fmt.Errorf("failed to generate thumbnail: %w", err)
	}
	hlog.Infof("Generated thumbnail at: %s", thumbnailPath)

	// 4. 读取缩略图文件
	thumbnailData, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return "", fmt.Errorf("failed to read thumbnail file: %w", err)
	}

	// 5. 构造缩略图在MinIO中的路径
	// 格式: users/{userID}/videos/{videoID}/thumbnails/thumb_medium.jpg
	thumbnailObjectName := fmt.Sprintf(oss.VIDEO_THUMBNAIL_TEMPLATE, userID, videoID, "medium")

	// 6. 上传缩略图到MinIO
	if err := s.tikTokStorage.UploadBytes(ctx, bucketName, thumbnailObjectName, thumbnailData, "image/jpeg"); err != nil {
		return "", fmt.Errorf("failed to upload thumbnail to MinIO: %w", err)
	}

	// 7. 返回缩略图URL
	thumbnailURL := fmt.Sprintf("/%s/%s", bucketName, thumbnailObjectName)
	hlog.Infof("Uploaded thumbnail to MinIO: %s", thumbnailURL)

	return thumbnailURL, nil
}
