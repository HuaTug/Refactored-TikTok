package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
}

// StartUpload 开始上传流程（TikTok风格）
func (s *VideoUploadServiceV2) StartUpload(req *videos.VideoPublishStartRequestV2) (*UploadSession, error) {
	// 1. 参数验证
	if req.Title == "" || req.ChunkTotalNumber <= 0 {
		return nil, errno.RequestErr
	}

	// 2. 检查用户存储配额
	if err := s.checkUserStorageQuota(req.UserId); err != nil {
		return nil, fmt.Errorf("storage quota exceeded: %w", err)
	}

	// 3. 生成video_id
	videoID, err := db.GetMaxVideoId(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate video_id: %w", err)
	}

	//redis.NewOptimizedUploadManager(s.ctx).CreateUploadSession(req.UserId, req.Title, req.Description, req.Category, req.LabName, req.ChunkTotalNumber)

	// 4. 创建上传会话
	session := &UploadSession{
		UUID:           s.generateUUID(),
		UserID:         req.UserId,
		VideoID:        parseVideoID(videoID),
		Title:          req.Title,
		Description:    req.Description,
		Category:       req.Category,
		Tags:           strings.Join(req.Tags, ","),
		TotalChunks:    int(req.ChunkTotalNumber),
		UploadedChunks: make([]bool, req.ChunkTotalNumber),
		TempDir:        s.createTempDir(req.UserId, videoID),
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(24 * time.Hour), // 24小时过期
		Status:         "pending",
	}

	// 5. 确保用户存储目录结构存在（跳过，将在实际上传时处理）
	// TODO: 实现用户目录结构检查
	// if err := s.tikTokStorage.ensureUserDirectoryStructure(s.ctx, req.UserId); err != nil {
	//     return nil, fmt.Errorf("failed to ensure user directory: %w", err)
	// }

	// 6. 保存会话到Redis
	if err := s.saveUploadSession(session); err != nil {
		return nil, fmt.Errorf("failed to save upload session: %w", err)
	}

	// 7. 创建临时目录
	if err := os.MkdirAll(session.TempDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	hlog.Infof("Started upload session %s for user %d, video %d", session.UUID, req.UserId, session.VideoID)
	return session, nil
}

// UploadChunk 上传分片（高性能优化版）
func (s *VideoUploadServiceV2) UploadChunk(req *videos.VideoPublishUploadingRequestV2) error {
	hlog.Infof("Starting upload chunk %d for session %s", req.ChunkNumber, req.UploadSessionUuid)

	// 1. 基本参数验证（无需查询Redis）
	if req.ChunkNumber <= 0 {
		return fmt.Errorf("invalid chunk number %d", req.ChunkNumber)
	}

	// 2. 验证分片数据
	if !s.verifyChunk(req.ChunkData, req.ChunkMd5) {
		return errors.New("chunk verification failed")
	}

	// 3. 快速构建临时目录路径（避免getUploadSession调用）
	uid := strconv.FormatInt(req.UserId, 10)
	tempDir := s.createTempDir(req.UserId, req.UploadSessionUuid)
	chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.tmp", req.ChunkNumber))

	// 4. 保存分片到临时目录
	if err := s.saveChunkFile(chunkPath, req.ChunkData); err != nil {
		return fmt.Errorf("failed to save chunk: %w", err)
	}

	// 5. 更新Redis中的分片状态（这是唯一必需的Redis操作）
	if err := redis.UpdateChunkUploadStatus(s.ctx, req.UploadSessionUuid, uid, int64(req.ChunkNumber)); err != nil {
		return fmt.Errorf("failed to update chunk status in Redis: %w", err)
	}

	hlog.Infof("Successfully uploaded chunk %d for session %s", req.ChunkNumber, req.UploadSessionUuid)
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

	hlog.Infof("Retrieved session %s: %d total chunks, local status: %d/%d uploaded",
		session.UUID, session.TotalChunks, s.countUploadedChunks(session.UploadedChunks), session.TotalChunks)

	// 2. 验证所有分片都已上传（增强检查）
	allUploaded := s.allChunksUploaded(session)
	if !allUploaded {
		// 详细的错误信息
		uploadedCount := s.countUploadedChunks(session.UploadedChunks)
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

	hlog.Infof("All chunks verified for session %s, proceeding with merge", session.UUID)

	session.Status = "processing"
	s.saveUploadSession(session)

	// 3. 合并分片
	mergedFilePath := filepath.Join(session.TempDir, "merged_video.mp4")
	hlog.Infof("Merging chunks for session %s to %s", session.UUID, mergedFilePath)

	if err := s.mergeChunks(session.TempDir, mergedFilePath, session.TotalChunks); err != nil {
		session.Status = "failed"
		s.saveUploadSession(session)
		hlog.Errorf("Failed to merge chunks for session %s: %v", session.UUID, err)
		return fmt.Errorf("failed to merge chunks: %w", err)
	}

	hlog.Infof("Successfully merged chunks for session %s", session.UUID)

	// 4. 使用TikTok存储架构上传
	uploadReq := &oss.VideoUploadRequest{
		UserID:      session.UserID,
		VideoID:     session.VideoID,
		Title:       session.Title,
		Description: session.Description,
		Category:    session.Category,
		Tags:        []string{session.Tags},
		Privacy:     "public",
		FilePath:    mergedFilePath,
		FileSize:    s.getFileSize(mergedFilePath),
		Duration:    s.getVideoDuration(mergedFilePath),
		Resolution:  s.getVideoResolution(mergedFilePath),
	}

	hlog.Infof("Uploading merged video for session %s to TikTok storage", session.UUID)
	uploadResp, err := s.tikTokStorage.UploadVideoTikTokStyle(s.ctx, uploadReq)
	if err != nil {
		session.Status = "failed"
		s.saveUploadSession(session)
		hlog.Errorf("Failed to upload to TikTok storage for session %s: %v", session.UUID, err)
		return fmt.Errorf("failed to upload to TikTok storage: %w", err)
	}

	// 5. 创建存储映射记录
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
		FileSize:          uploadReq.FileSize,
		Duration:          int(uploadReq.Duration),
		ResolutionWidth:   uploadReq.Resolution.Width,
		ResolutionHeight:  uploadReq.Resolution.Height,
		Format:            "mp4",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := db.CreateVideoStorageMapping(s.ctx, storageMapping); err != nil {
		hlog.Errorf("Failed to create storage mapping for session %s: %v", session.UUID, err)
		// 不阻塞主流程，可以后续补偿
	}

	// 6. 创建视频记录
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

	// 7. 更新用户存储配额
	if err := s.updateUserStorageUsage(session.UserID, uploadReq.FileSize); err != nil {
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

func (s *VideoUploadServiceV2) createTempDir(userID int64, videoID string) string {
	return filepath.Join("/tmp", "tiktok_upload", fmt.Sprintf("%d_%s", userID, videoID))
}

func (s *VideoUploadServiceV2) saveUploadSession(session *UploadSession) error {
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

	hlog.Infof("Successfully saved upload session %s to Redis", session.UUID)
	return nil
}

func (s *VideoUploadServiceV2) getUploadSession(uuid string, userID int64) (*UploadSession, error) {
	uid := strconv.FormatInt(userID, 10)

	// 获取基本信息
	info, err := redis.GetChunkInfo(uid, uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk info: %w", err)
	}

	totalChunks, err := strconv.Atoi(info[0])
	if err != nil {
		return nil, fmt.Errorf("invalid total chunks: %s", info[0])
	}

	// 使用V2专用方法获取真实的上传状态
	uploadedChunks, err := redis.GetUploadedChunksStatus(s.ctx, uuid, uid)
	if err != nil {
		hlog.Warnf("Failed to get uploaded chunks status for session %s: %v", uuid, err)
		// 如果获取状态失败，创建默认的false切片
		uploadedChunks = make([]bool, totalChunks)
	}

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
	}

	hlog.Infof("Retrieved upload session %s: %d/%d chunks uploaded", uuid, s.countUploadedChunks(uploadedChunks), totalChunks)
	return session, nil
}

func (s *VideoUploadServiceV2) deleteUploadSession(uuid string, userID int64) error {
	// TODO: 实现Redis上传会话删除
	return redis.DeleteVideoEvent(s.ctx, uuid, strconv.FormatInt(userID, 10))
}

func (s *VideoUploadServiceV2) verifyChunk(data []byte, expectedMD5 string) bool {
	// 实现MD5验证
	return true // 简化实现
}

func (s *VideoUploadServiceV2) saveChunkFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
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

func (s *VideoUploadServiceV2) mergeChunks(tempDir, outputPath string, totalChunks int) error {
	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	for i := 1; i <= totalChunks; i++ {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.tmp", i))
		chunk, err := os.Open(chunkPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(output, chunk)
		chunk.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *VideoUploadServiceV2) getFileSize(filePath string) int64 {
	stat, err := os.Stat(filePath)
	if err != nil {
		return 0
	}
	return stat.Size()
}

func (s *VideoUploadServiceV2) getVideoDuration(filePath string) int64 {
	// TODO: 使用ffmpeg获取视频时长
	return 120 // 默认120秒
}

func (s *VideoUploadServiceV2) getVideoResolution(filePath string) oss.VideoResolution {
	// TODO: 使用ffmpeg获取视频分辨率
	return oss.VideoResolution{Width: 1280, Height: 720}
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
