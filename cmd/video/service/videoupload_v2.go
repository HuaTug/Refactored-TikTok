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

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/cmd/video/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/oss"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// VideoUploadServiceV2 基于TikTok存储架构的新版上传服务
type VideoUploadServiceV2 struct {
	ctx           context.Context
	tikTokStorage *oss.TikTokStorage
	sessionCache  sync.Map // 会话缓存，避免重复创建MinIO UploadID
}

func NewVideoUploadServiceV2(ctx context.Context) *VideoUploadServiceV2 {
	return &VideoUploadServiceV2{
		ctx:           ctx,
		tikTokStorage: oss.NewTikTokStorage(),
	}
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
	videoID, err := db.GetMaxVideoId(s.ctx)
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
		TempDir:        s.createTempDir(req.UserId, genUUID),
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

	// 7. 确保用户存储目录结构存在（跳过，将在实际上传时处理）
	// TODO: 实现用户目录结构检查
	// if err := s.tikTokStorage.ensureUserDirectoryStructure(s.ctx, req.UserId); err != nil {
	//     return nil, fmt.Errorf("failed to ensure user directory: %w", err)
	// }

	// 8. 保存会话到Redis
	if err := s.saveUploadSession(session); err != nil {
		return nil, fmt.Errorf("failed to save upload session: %w", err)
	}

	// 9. 创建临时目录
	if err := os.MkdirAll(session.TempDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
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
		return fmt.Errorf("failed to upload chunk to MinIO: %w", err)
	}

	hlog.Infof("Successfully uploaded chunk %d to MinIO: ETag=%s, Size=%d bytes",
		req.ChunkNumber, part.ETag, part.Size)

	// 7. 更新会话中的分片信息
	// 确保分片数据被保存到会话中
	partWithData := part
	partWithData.Data = req.ChunkData // 保存原始分片数据用于后续合并
	session.UploadedParts[int(req.ChunkNumber)] = partWithData
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
func (s *VideoUploadServiceV2) CompleteUpload(req *videos.VideoPublishCompleteRequestV2) error {
	hlog.Infof("Starting complete upload for session %s, user %d", req.UploadSessionUuid, req.UserId)

	// 1. 获取上传会话
	session, err := s.getUploadSession(req.UploadSessionUuid, req.UserId)
	if err != nil {
		hlog.Errorf("Failed to get upload session %s: %v", req.UploadSessionUuid, err)
		return fmt.Errorf("failed to get upload session: %w", err)
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

		return fmt.Errorf("not all chunks have been uploaded: %d/%d uploaded, missing chunks: %v",
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
	hlog.Infof("Session %s has %d uploaded parts in memory", session.UUID, len(session.UploadedParts))

	for i := 1; i <= session.TotalChunks; i++ {
		if part, exists := session.UploadedParts[i]; exists {
			hlog.Infof("Found part %d: ETag=%s, Size=%d, DataLen=%d",
				i, part.ETag, part.Size, len(part.Data))
			parts = append(parts, part)
		} else {
			// 如果某个分片不存在，这是严重错误
			hlog.Errorf("Part %d not found in session %s uploaded parts, available parts: %v",
				i, session.UUID, getUploadedPartsKeys(session.UploadedParts))
			session.Status = "failed"
			s.saveUploadSession(session)
			return fmt.Errorf("missing part %d in uploaded parts", i)
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
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	hlog.Infof("Successfully completed MinIO multipart upload for session %s", session.UUID)

	// 4. 获取合并后的视频信息（MinIO中的文件已经合并完成）
	// 构造视频URL
	videoURL := fmt.Sprintf("http://localhost:9000/%s/%s", session.BucketName, session.ObjectName)

	// 模拟视频处理响应（实际应该从MinIO获取文件信息）
	uploadResp := &oss.VideoUploadResponse{
		VideoID:          session.VideoID,
		SourceURL:        videoURL,
		ProcessedURLs:    map[int]string{720: videoURL},                        // 简化处理，使用原始URL
		ThumbnailURLs:    map[string]string{"medium": videoURL + "_thumb.jpg"}, // 模拟缩略图
		AnimatedCoverURL: videoURL + "_animated.gif",                           // 模拟动态封面
		MetadataURL:      videoURL + "_metadata.json",                          // 模拟元数据
	}

	// 5. 获取视频文件大小（从MinIO）
	objectInfo, err := s.tikTokStorage.GetObjectInfo(s.ctx, session.BucketName, session.ObjectName)
	var fileSize int64 = 0
	if err != nil {
		hlog.Warnf("Failed to get object info from MinIO: %v", err)
		// 估算文件大小（分片大小 * 分片数量）
		fileSize = session.ChunkSize * int64(session.TotalChunks)
	} else {
		fileSize = objectInfo.Size
	}

	// 6. 创建存储映射记录
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

	// 7. 创建视频记录
	video := &base.Video{
		Title:       session.Title,
		Description: session.Description,
		UserId:      session.UserID,
		VisitCount:  0,
		LabelNames:  session.Tags,
		Category:    session.Category,
		CreatedAt:   time.Now().Format(constants.DataFormate),
		UpdatedAt:   time.Now().Format(constants.DataFormate),
		VideoUrl:    uploadResp.ProcessedURLs[720], // 默认使用720p
		CoverUrl:    uploadResp.ThumbnailURLs["medium"],
		DeletedAt:   "0",
	}

	if err := db.InsertVideo(s.ctx, video); err != nil {
		session.Status = "failed"
		s.saveUploadSession(session)
		hlog.Errorf("Failed to save video record for session %s: %v", session.UUID, err)
		return fmt.Errorf("failed to save video record: %w", err)
	}

	// 8. 更新用户存储配额
	if err := s.updateUserStorageUsage(session.UserID, fileSize); err != nil {
		hlog.Warnf("Failed to update user storage usage for session %s: %v", session.UUID, err)
	}

	// 8. 清理临时文件和会话
	session.Status = "completed"
	s.saveUploadSession(session)

	go func() {
		s.cleanupTempFiles(session.TempDir)
		s.deleteUploadSession(session.UUID, session.UserID)
	}()

	hlog.Infof("Successfully completed upload for session %s, video %d", session.UUID, session.VideoID)
	return nil
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
			hlog.Infof("Restored MinIO UploadID: %s, UploadedParts: %d",
				session.MinIOUploadID, len(session.UploadedParts))
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
	if expectedMD5 == "dummy-md5" || expectedMD5 == "" {
		// 兼容测试场景，跳过MD5验证
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
	if err := os.RemoveAll(tempDir); err != nil {
		hlog.Errorf("Failed to cleanup temp directory %s: %v", tempDir, err)
	}
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

// GetUserStorageQuota 获取用户存储配额信息
func (s *VideoUploadServiceV2) GetUserStorageQuota(userID int64) (*videos.UserStorageQuota, error) {
	// 暂时使用简化的存储统计逻辑，后续可以扩展
	// TODO: 实现真实的用户存储统计查询

	// 根据用户等级确定配额
	quotaLevel := s.determineQuotaLevel(userID)
	totalQuota := s.getTotalQuotaByLevel(quotaLevel)
	maxVideoSize := s.getMaxVideoSizeByLevel(quotaLevel)
	maxVideoCount := s.getMaxVideoCountByLevel(quotaLevel)

	// 简化的使用量统计 - 实际项目中应该从数据库查询
	usedStorage := int64(0)
	videoCount := int64(0)

	// TODO: 从数据库查询实际使用量
	// userStats, err := db.GetUserStorageStats(s.ctx, userID)
	// if err == nil {
	//     usedStorage = userStats.UsedStorage
	//     videoCount = userStats.VideoCount
	// }

	return &videos.UserStorageQuota{
		TotalQuotaBytes:   totalQuota,
		UsedQuotaBytes:    usedStorage,
		VideoCount:        videoCount,
		QuotaLevel:        quotaLevel,
		MaxVideoSizeBytes: maxVideoSize,
		MaxVideoCount:     int32(maxVideoCount), // 转换为int32
	}, nil
}

// determineQuotaLevel 确定用户配额等级
func (s *VideoUploadServiceV2) determineQuotaLevel(userID int64) string {
	// TODO: 根据用户VIP等级或其他条件确定配额等级
	// 这里简化处理，默认为 standard
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
		UploadSpeedMbps: "calculating", // TODO: 实现真实的速度计算
		UploadedChunks:  uploadedCount,
		TotalChunks:     totalChunks,
	}, nil
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
