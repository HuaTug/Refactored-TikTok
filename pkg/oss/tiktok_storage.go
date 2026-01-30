package oss

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/minio/minio-go/v7"
)

// 存储桶常量（已迁移到storage_config.go，保留此引用以保持兼容性）
// 旧的常量定义已移至 storage_config.go

// TikTokStorage 新的存储服务
type TikTokStorage struct {
	client *minio.Client
}

// VideoUploadRequest 视频上传请求
type VideoUploadRequest struct {
	UserID      int64           `json:"user_id"`
	VideoID     int64           `json:"video_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Tags        []string        `json:"tags"`
	Privacy     string          `json:"privacy"` // public, private, friends
	FilePath    string          `json:"file_path"`
	FileSize    int64           `json:"file_size"`
	Duration    int64           `json:"duration"`
	Resolution  VideoResolution `json:"resolution"`
}

type VideoResolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VideoUploadResponse 视频上传响应
type VideoUploadResponse struct {
	VideoID          int64             `json:"video_id"`
	SourceURL        string            `json:"source_url"`
	ProcessedURLs    map[int]string    `json:"processed_urls"`
	ThumbnailURLs    map[string]string `json:"thumbnail_urls"`
	AnimatedCoverURL string            `json:"animated_cover_url"`
	MetadataURL      string            `json:"metadata_url"`
}

// VideoMetadata 视频元数据
type VideoMetadata struct {
	UserID            int64             `json:"user_id"`
	VideoID           int64             `json:"video_id"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	Category          string            `json:"category"`
	Tags              []string          `json:"tags"`
	Privacy           string            `json:"privacy"`
	Duration          int64             `json:"duration"`
	Resolution        VideoResolution   `json:"resolution"`
	SourcePath        string            `json:"source_path"`
	ProcessedPaths    map[int]string    `json:"processed_paths"`
	ThumbnailPaths    map[string]string `json:"thumbnail_paths"`
	AnimatedCoverPath string            `json:"animated_cover_path"`
	UploadedAt        time.Time         `json:"uploaded_at"`
}

// VideoStoragePath 视频存储路径
type VideoStoragePath struct {
	UserID    int64  `json:"user_id"`
	VideoID   int64  `json:"video_id"`
	CreatedAt string `json:"created_at"`
}

// MinIOObjectPart MinIO分片信息
type MinIOObjectPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
	Data       []byte `json:"data"` // 分片数据，序列化到JSON以支持会话恢复
}

// NewTikTokStorage 创建新的存储服务实例
func NewTikTokStorage() *TikTokStorage {
	return &TikTokStorage{
		client: minioClient,
	}
}

// 初始化存储桶 - 使用统一的存储桶配置
func (ts *TikTokStorage) InitializeBuckets(ctx context.Context) error {
	if ts.client == nil {
		return fmt.Errorf("MinIO client is not initialized")
	}

	buckets := GetAllBuckets()

	hlog.Infof("Starting to initialize %d TikTok storage buckets", len(buckets))

	for i, bucketName := range buckets {
		hlog.Infof("Checking bucket %d/%d: %s", i+1, len(buckets), bucketName)

		exists, err := ts.client.BucketExists(ctx, bucketName)
		if err != nil {
			hlog.Errorf("Failed to check if bucket %s exists: %v", bucketName, err)
			return fmt.Errorf("check bucket %s error: %w", bucketName, err)
		}

		if !exists {
			hlog.Infof("Bucket %s does not exist, creating...", bucketName)
			err = ts.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
				Region: "us-east-1",
			})
			if err != nil {
				hlog.Errorf("Failed to create bucket %s: %v", bucketName, err)
				return fmt.Errorf("create bucket %s error: %w", bucketName, err)
			}
			hlog.Infof("Successfully created bucket: %s", bucketName)
		} else {
			hlog.Infof("Bucket %s already exists", bucketName)
		}
	}

	hlog.Info("All TikTok storage buckets initialized successfully")
	return nil
}

// UploadVideoTikTokStyle 按TikTok风格上传视频
func (ts *TikTokStorage) UploadVideoTikTokStyle(ctx context.Context, req *VideoUploadRequest) (*VideoUploadResponse, error) {
	// 1. 确保用户目录结构存在
	if err := ts.ensureUserDirectoryStructure(ctx, req.UserID); err != nil {
		return nil, fmt.Errorf("failed to ensure user directory: %w", err)
	}

	// 2. 上传原始文件
	sourcePath := ts.getSourceVideoPath(req.UserID, req.VideoID)
	if err := ts.uploadFile(ctx, BUCKET_USER_CONTENT, sourcePath, req.FilePath); err != nil {
		return nil, fmt.Errorf("failed to upload source video: %w", err)
	}

	// 3. 生成多分辨率版本的路径（实际转码需要额外的视频处理服务）
	qualities := []int{480, 720, 1080}
	processedPaths := make(map[int]string)

	for _, quality := range qualities {
		processedPath := ts.getProcessedVideoPath(req.UserID, req.VideoID, quality)
		processedPaths[quality] = processedPath

		// TODO: 集成视频转码服务
		// 目前先复制原始文件作为处理后的文件
		if err := ts.copyObject(ctx, BUCKET_USER_CONTENT, sourcePath, BUCKET_USER_CONTENT, processedPath); err != nil {
			hlog.Warnf("Failed to create processed version %dp: %v", quality, err)
		}
	}

	// 4. 生成缩略图（简化版，实际需要视频处理服务）
	thumbnailPaths := ts.generateThumbnailPaths(req.UserID, req.VideoID)

	// 5. 动态封面路径
	animatedCoverPath := ts.getAnimatedCoverPath(req.UserID, req.VideoID)

	// 6. 保存元数据
	metadata := &VideoMetadata{
		UserID:            req.UserID,
		VideoID:           req.VideoID,
		Title:             req.Title,
		Description:       req.Description,
		Category:          req.Category,
		Tags:              req.Tags,
		Privacy:           req.Privacy,
		Duration:          req.Duration,
		Resolution:        req.Resolution,
		SourcePath:        sourcePath,
		ProcessedPaths:    processedPaths,
		ThumbnailPaths:    thumbnailPaths,
		AnimatedCoverPath: animatedCoverPath,
		UploadedAt:        time.Now(),
	}

	metadataPath := ts.getVideoMetadataPath(req.UserID, req.VideoID)
	if err := ts.uploadMetadata(ctx, BUCKET_USER_CONTENT, metadataPath, metadata); err != nil {
		return nil, fmt.Errorf("failed to upload metadata: %w", err)
	}

	// 7. 构建响应
	response := &VideoUploadResponse{
		VideoID:          req.VideoID,
		SourceURL:        ts.generateURL(BUCKET_USER_CONTENT, sourcePath),
		ProcessedURLs:    ts.generateURLsForProcessed(processedPaths),
		ThumbnailURLs:    ts.generateURLsForThumbnails(thumbnailPaths),
		AnimatedCoverURL: ts.generateURL(BUCKET_USER_CONTENT, animatedCoverPath),
		MetadataURL:      ts.generateURL(BUCKET_USER_CONTENT, metadataPath),
	}

	hlog.Infof("Successfully uploaded video %d for user %d", req.VideoID, req.UserID)
	return response, nil
}

// 路径生成方法 - 使用统一路径模板
func (ts *TikTokStorage) getSourceVideoPath(userID, videoID int64) string {
	return fmt.Sprintf(VIDEO_SOURCE_TEMPLATE, userID, videoID)
}

func (ts *TikTokStorage) getProcessedVideoPath(userID, videoID int64, quality int) string {
	return fmt.Sprintf(VIDEO_PROCESSED_TEMPLATE, userID, videoID, quality)
}

func (ts *TikTokStorage) GetThumbnailPath(userID, videoID int64, size string) string {
	return fmt.Sprintf(VIDEO_THUMBNAIL_TEMPLATE, userID, videoID, size)
}

func (ts *TikTokStorage) getAnimatedCoverPath(userID, videoID int64) string {
	return fmt.Sprintf(VIDEO_ANIMATED_COVER_TEMPLATE, userID, videoID)
}

func (ts *TikTokStorage) getVideoMetadataPath(userID, videoID int64) string {
	return fmt.Sprintf(VIDEO_METADATA_TEMPLATE, userID, videoID)
}

func (ts *TikTokStorage) getUserAvatarPath(userID int64, size string) string {
	return fmt.Sprintf(USER_AVATAR_TEMPLATE, userID, size, ".jpg")
}

func (ts *TikTokStorage) getUserBackgroundPath(userID int64) string {
	return fmt.Sprintf(USER_BACKGROUND_TEMPLATE, userID)
}

// 生成缩略图路径映射
func (ts *TikTokStorage) generateThumbnailPaths(userID, videoID int64) map[string]string {
	sizes := []string{"small", "medium", "large"}
	paths := make(map[string]string)

	for _, size := range sizes {
		paths[size] = ts.GetThumbnailPath(userID, videoID, size)
	}

	return paths
}

// 确保用户目录结构存在 - 使用统一路径模板
func (ts *TikTokStorage) ensureUserDirectoryStructure(ctx context.Context, userID int64) error {
	directories := []string{
		fmt.Sprintf(USER_AVATAR_DIR_TEMPLATE, userID),
		fmt.Sprintf(USER_BACKGROUND_DIR_TEMPLATE, userID),
		fmt.Sprintf(USER_VIDEOS_DIR_TEMPLATE, userID),
		fmt.Sprintf(USER_DRAFTS_DIR_TEMPLATE, userID),
	}

	for _, dir := range directories {
		markerPath := filepath.Join(dir, ".directory_marker")
		if err := ts.uploadEmptyFile(ctx, BUCKET_USER_CONTENT, markerPath); err != nil {
			return fmt.Errorf("failed to create directory marker %s: %w", dir, err)
		}
	}

	return nil
}

// 上传文件
func (ts *TikTokStorage) uploadFile(ctx context.Context, bucketName, objectName, filePath string) error {
	_, err := ts.client.FPutObject(ctx, bucketName, objectName, filePath, minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	return err
}

// 上传空文件（用于创建目录标记）
func (ts *TikTokStorage) uploadEmptyFile(ctx context.Context, bucketName, objectName string) error {
	_, err := ts.client.PutObject(ctx, bucketName, objectName, bytes.NewReader([]byte{}), 0, minio.PutObjectOptions{})
	return err
}

// 复制对象
func (ts *TikTokStorage) copyObject(ctx context.Context, srcBucket, srcObject, dstBucket, dstObject string) error {
	src := minio.CopySrcOptions{
		Bucket: srcBucket,
		Object: srcObject,
	}

	dst := minio.CopyDestOptions{
		Bucket: dstBucket,
		Object: dstObject,
	}

	_, err := ts.client.CopyObject(ctx, dst, src)
	return err
}

// 上传元数据
func (ts *TikTokStorage) uploadMetadata(ctx context.Context, bucketName, objectName string, metadata *VideoMetadata) error {
	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	_, err = ts.client.PutObject(ctx, bucketName, objectName, bytes.NewReader(jsonData), int64(len(jsonData)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	return err
}

// 生成URL - Use MinIO API URL format for direct access
func (ts *TikTokStorage) generateURL(bucketName, objectName string) string {
	return fmt.Sprintf("%s/%s/%s", GetMinIOEndpoint(), bucketName, objectName)
}

// 为处理后的视频生成URL映射
func (ts *TikTokStorage) generateURLsForProcessed(processedPaths map[int]string) map[int]string {
	urls := make(map[int]string)
	for quality, path := range processedPaths {
		urls[quality] = ts.generateURL(BUCKET_USER_CONTENT, path)
	}
	return urls
}

// 为缩略图生成URL映射
func (ts *TikTokStorage) generateURLsForThumbnails(thumbnailPaths map[string]string) map[string]string {
	urls := make(map[string]string)
	for size, path := range thumbnailPaths {
		urls[size] = ts.generateURL(BUCKET_USER_CONTENT, path)
	}
	return urls
}

// 获取用户所有视频
func (ts *TikTokStorage) GetUserVideos(ctx context.Context, userID int64, limit, offset int) ([]*VideoMetadata, error) {
	prefix := fmt.Sprintf("users/%d/videos/", userID)

	objectCh := ts.client.ListObjects(ctx, BUCKET_USER_CONTENT, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	})

	var videoMetadataList []*VideoMetadata
	videoIDPattern := regexp.MustCompile(`users/(\d+)/videos/(\d+)/`)

	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}

		// 解析视频ID并获取元数据
		if matches := videoIDPattern.FindStringSubmatch(object.Key); len(matches) == 3 {
			videoID, _ := strconv.ParseInt(matches[2], 10, 64)
			metadataPath := ts.getVideoMetadataPath(userID, videoID)

			metadata, err := ts.getVideoMetadata(ctx, metadataPath)
			if err != nil {
				hlog.Warnf("Failed to get metadata for video %d: %v", videoID, err)
				continue
			}

			videoMetadataList = append(videoMetadataList, metadata)
		}
	}

	// 分页处理
	start := offset
	end := offset + limit
	if start > len(videoMetadataList) {
		return []*VideoMetadata{}, nil
	}
	if end > len(videoMetadataList) {
		end = len(videoMetadataList)
	}

	return videoMetadataList[start:end], nil
}

// 获取视频元数据
func (ts *TikTokStorage) getVideoMetadata(ctx context.Context, metadataPath string) (*VideoMetadata, error) {
	object, err := ts.client.GetObject(ctx, BUCKET_USER_CONTENT, metadataPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer object.Close()

	data, err := io.ReadAll(object)
	if err != nil {
		return nil, err
	}

	var metadata VideoMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// 热度分层存储：将热门视频提升到热点缓存 - 使用统一路径模板
func (ts *TikTokStorage) PromoteToHotStorage(ctx context.Context, userID, videoID int64) error {
	sourcePath := ts.getProcessedVideoPath(userID, videoID, 720)
	hotPath := fmt.Sprintf(HOT_VIDEO_TEMPLATE, userID, videoID)

	return ts.copyObject(ctx, BUCKET_USER_CONTENT, sourcePath, BUCKET_CACHE_HOT, hotPath)
}

// 检查视频是否在热点存储中 - 使用统一路径模板
func (ts *TikTokStorage) IsInHotStorage(ctx context.Context, userID, videoID int64) (bool, error) {
	hotPath := fmt.Sprintf(HOT_VIDEO_TEMPLATE, userID, videoID)

	_, err := ts.client.StatObject(ctx, BUCKET_CACHE_HOT, hotPath, minio.StatObjectOptions{})
	if err != nil {
		// 如果错误是对象不存在，返回false
		if strings.Contains(err.Error(), "NoSuchKey") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// 生成预签名URL用于Stream代理
func (ts *TikTokStorage) GeneratePresignedURL(bucketName, objectName string, expiry time.Duration) (string, error) {
	presignedURL, err := ts.client.PresignedGetObject(context.Background(), bucketName, objectName, expiry, nil)
	if err != nil {
		return "", err
	}

	return presignedURL.String(), nil
}

// UploadUserAvatar 上传用户头像（TikTok风格）- 使用统一路径模板
func (ts *TikTokStorage) UploadUserAvatar(ctx context.Context, userID int64, data []byte, contentType string) (map[string]string, error) {
	// 确保用户目录存在
	if err := ts.ensureUserDirectoryStructure(ctx, userID); err != nil {
		return nil, err
	}

	// 删除旧头像
	ts.deleteUserAvatars(ctx, userID)

	var suffix string
	switch contentType {
	case "image/jpeg", "image/jpg":
		suffix = ".jpg"
	case "image/png":
		suffix = ".png"
	default:
		return nil, fmt.Errorf("unsupported image format: %s", contentType)
	}

	// 生成不同尺寸的头像路径
	sizes := []string{"small", "medium", "large"}
	avatarURLs := make(map[string]string)

	for _, size := range sizes {
		avatarPath := fmt.Sprintf(USER_AVATAR_TEMPLATE, userID, size, suffix)

		_, err := ts.client.PutObject(ctx, BUCKET_USER_CONTENT, avatarPath, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upload avatar %s: %w", size, err)
		}

		avatarURLs[size] = ts.generateURL(BUCKET_USER_CONTENT, avatarPath)
	}

	hlog.Infof("Successfully uploaded avatar for user %d", userID)
	return avatarURLs, nil
}

// 删除用户旧头像
func (ts *TikTokStorage) deleteUserAvatars(ctx context.Context, userID int64) {
	sizes := []string{"small", "medium", "large"}
	extensions := []string{".jpg", ".jpeg", ".png"}

	for _, size := range sizes {
		for _, ext := range extensions {
			avatarPath := fmt.Sprintf("users/%d/profile/avatar/avatar_%s%s", userID, size, ext)
			err := ts.client.RemoveObject(ctx, BUCKET_USER_CONTENT, avatarPath, minio.RemoveObjectOptions{})
			if err != nil {
				hlog.Warnf("Failed to delete old avatar %s: %v", avatarPath, err)
			}
		}
	}
}

// GetOptimalVideoPath 根据设备类型和网络状况选择最优视频路径 - 使用统一路径模板
func (ts *TikTokStorage) GetOptimalVideoPath(userID, videoID int64, userAgent string, quality string) (string, error) {
	// 检查是否在热点存储
	inHotStorage, err := ts.IsInHotStorage(context.Background(), userID, videoID)
	if err != nil {
		hlog.Warnf("Failed to check hot storage for video %d: %v", videoID, err)
	}

	// 选择合适的分辨率
	selectedQuality := ts.selectOptimalQuality(userAgent, quality)

	var objectPath string
	if inHotStorage {
		objectPath = fmt.Sprintf(HOT_VIDEO_TEMPLATE, userID, videoID, selectedQuality)
	} else {
		objectPath = ts.getProcessedVideoPath(userID, videoID, selectedQuality)
	}

	return objectPath, nil
}

// 智能分辨率选择
func (ts *TikTokStorage) selectOptimalQuality(userAgent, requestedQuality string) int {
	if requestedQuality != "" {
		if quality, err := strconv.Atoi(requestedQuality); err == nil {
			return quality
		}
	}

	// 根据User-Agent判断设备类型
	if strings.Contains(userAgent, "Mobile") {
		return 480 // 移动设备默认480p
	}

	return 720 // 桌面设备默认720p
}

// CreateMultipartUpload 初始化分片上传，返回 UploadID
func (ts *TikTokStorage) CreateMultipartUpload(ctx context.Context, bucketName, objectName, contentType string) (string, error) {
	// MinIO Go SDK v7 采用了不同的方式
	// 我们直接使用一个唯一的uploadID，实际的multipart由PutObject处理
	uploadID := fmt.Sprintf("native_%d_%s", time.Now().UnixNano(), strings.ReplaceAll(objectName, "/", "_"))

	hlog.Infof("已初始化分片上传会话，Bucket: %s, Object: %s, UploadID: %s", bucketName, objectName, uploadID)
	return uploadID, nil
}

// UploadPart 上传单个分片 - 直接上传到MinIO临时目录
func (ts *TikTokStorage) UploadPart(ctx context.Context, bucketName, objectName, uploadID string, partNumber int, data io.Reader, partSize int64) (MinIOObjectPart, error) {
	// 将数据读取到内存中以计算MD5
	partData := make([]byte, partSize)
	_, err := io.ReadFull(data, partData)
	if err != nil {
		return MinIOObjectPart{}, fmt.Errorf("读取分片数据失败: %v", err)
	}

	// 计算ETag (MD5)
	etag := fmt.Sprintf("%x", md5.Sum(partData))

	// 构建临时分片的对象名称
	tempPartObjectName := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE+"part_%d", uploadID, partNumber)

	// 直接上传分片到MinIO临时目录
	reader := bytes.NewReader(partData)
	uploadInfo, err := ts.client.PutObject(ctx, bucketName, tempPartObjectName, reader, partSize, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		hlog.Errorf("分片 %d 上传到MinIO失败: %v", partNumber, err)
		return MinIOObjectPart{}, fmt.Errorf("分片上传失败: %v", err)
	}

	hlog.Infof("分片 %d 已上传到MinIO临时目录，ETag: %s, Size: %d bytes, Path: %s",
		partNumber, uploadInfo.ETag, partSize, tempPartObjectName)

	return MinIOObjectPart{
		PartNumber: partNumber,
		ETag:       etag,
		Size:       partSize,
		Data:       nil, // 不在内存中保存数据，合并时从MinIO读取
	}, nil
}

// CompleteMultipartUpload 完成分片合并 - 从MinIO临时目录读取分片后合并
func (ts *TikTokStorage) CompleteMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string, parts []MinIOObjectPart) error {
	hlog.Infof("开始合并 %d 个分片", len(parts))

	// 按分片号排序
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	// 创建缓冲区来存储完整文件
	var fullData bytes.Buffer

	// 按顺序从MinIO读取分片数据并拼接
	for _, part := range parts {
		// 构建临时分片的对象名称
		tempPartObjectName := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE+"part_%d", uploadID, part.PartNumber)

		hlog.Infof("读取分片 %d: %s, 预期大小: %d", part.PartNumber, tempPartObjectName, part.Size)

		// 从MinIO读取分片数据
		obj, err := ts.client.GetObject(ctx, bucketName, tempPartObjectName, minio.GetObjectOptions{})
		if err != nil {
			hlog.Errorf("获取分片 %d 失败: %v", part.PartNumber, err)
			return fmt.Errorf("获取分片 %d 失败: %v", part.PartNumber, err)
		}

		partData, err := io.ReadAll(obj)
		obj.Close()
		if err != nil {
			hlog.Errorf("读取分片 %d 数据失败: %v", part.PartNumber, err)
			return fmt.Errorf("读取分片 %d 数据失败: %v", part.PartNumber, err)
		}

		if len(partData) == 0 {
			return fmt.Errorf("分片 %d 数据为空", part.PartNumber)
		}

		fullData.Write(partData)
		hlog.Infof("已读取分片 %d: ETag=%s, Size=%d", part.PartNumber, part.ETag, len(partData))
	}

	hlog.Infof("所有分片已读取完成，总大小: %d bytes", fullData.Len())

	// 使用PutObject上传完整文件
	reader := bytes.NewReader(fullData.Bytes())
	uploadInfo, err := ts.client.PutObject(ctx, bucketName, objectName, reader, int64(fullData.Len()), minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	if err != nil {
		hlog.Errorf("MinIO PutObject失败: %v", err)
		return fmt.Errorf("MinIO文件上传失败: %v", err)
	}

	hlog.Infof("✅ MinIO文件上传成功，文件: %s/%s，大小: %d bytes，ETag: %s",
		bucketName, objectName, uploadInfo.Size, uploadInfo.ETag)

	// 清理临时分片文件
	go func() {
		for _, part := range parts {
			tempPartObjectName := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE+"part_%d", uploadID, part.PartNumber)
			if err := ts.client.RemoveObject(context.Background(), bucketName, tempPartObjectName, minio.RemoveObjectOptions{}); err != nil {
				hlog.Warnf("清理临时分片 %s 失败: %v", tempPartObjectName, err)
			} else {
				hlog.Infof("已清理临时分片: %s", tempPartObjectName)
			}
		}
	}()

	return nil
}

// AbortMultipartUpload 取消分片上传，清理临时分片文件
func (ts *TikTokStorage) AbortMultipartUpload(ctx context.Context, bucketName, objectName, uploadID string) error {
	// 列出临时分片目录下的所有对象并删除
	prefix := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE, uploadID)
	objectsCh := ts.client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var deleteCount int
	for obj := range objectsCh {
		if obj.Err != nil {
			hlog.Warnf("列出临时分片对象失败: %v", obj.Err)
			continue
		}
		if err := ts.client.RemoveObject(ctx, bucketName, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			hlog.Warnf("删除临时分片 %s 失败: %v", obj.Key, err)
		} else {
			deleteCount++
		}
	}

	hlog.Infof("已取消分片上传并清理 %d 个临时分片，Bucket: %s, Object: %s, UploadID: %s",
		deleteCount, bucketName, objectName, uploadID)
	return nil
}

// ListParts 列出已上传的分片 - 使用统一路径模板
func (ts *TikTokStorage) ListParts(ctx context.Context, bucketName, objectName, uploadID string) ([]MinIOObjectPart, error) {
	// 列出临时分片目录下的所有对象
	prefix := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE, uploadID)
	objectsCh := ts.client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var parts []MinIOObjectPart
	for object := range objectsCh {
		if object.Err != nil {
			return nil, fmt.Errorf("列出分片失败: %v", object.Err)
		}

		// 从对象名中提取分片编号
		partName := strings.TrimPrefix(object.Key, prefix)
		if strings.HasPrefix(partName, "part_") {
			partNumberStr := strings.TrimPrefix(partName, "part_")
			partNumber, err := strconv.Atoi(partNumberStr)
			if err != nil {
				continue // 跳过无效的分片名
			}

			parts = append(parts, MinIOObjectPart{
				PartNumber: partNumber,
				ETag:       object.ETag,
				Size:       object.Size,
			})
		}
	}

	return parts, nil
}

// CalculateOptimalChunkSize 计算最优分片大小
func (ts *TikTokStorage) CalculateOptimalChunkSize(fileSize int64, maxChunks int) int64 {
	const minChunkSize = 5 * 1024 * 1024   // 5MB - MinIO最小分片大小
	const maxChunkSize = 100 * 1024 * 1024 // 100MB - 推荐最大分片大小

	// 基于文件大小和最大分片数计算分片大小
	calculatedSize := fileSize / int64(maxChunks)

	// 确保分片大小在合理范围内
	if calculatedSize < minChunkSize {
		return minChunkSize
	}
	if calculatedSize > maxChunkSize {
		return maxChunkSize
	}

	return calculatedSize
}

// GenerateVideoObjectName 生成视频对象名称（已弃用，请使用getSourceVideoPath）
// 保留此方法以向后兼容
func (ts *TikTokStorage) GenerateVideoObjectName(userID, videoID int64) string {
	// 现在使用统一的路径模板
	return fmt.Sprintf(VIDEO_SOURCE_TEMPLATE, userID, videoID)
}

// GetObjectInfo 获取对象信息
func (ts *TikTokStorage) GetObjectInfo(ctx context.Context, bucketName, objectName string) (*minio.ObjectInfo, error) {
	objectInfo, err := ts.client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取对象信息失败: %v", err)
	}

	return &objectInfo, nil
}

// cleanupTempParts 清理临时分片文件 - 使用统一路径模板
func (ts *TikTokStorage) cleanupTempParts(ctx context.Context, bucketName, uploadID string, maxParts int) {
	prefix := fmt.Sprintf(TEMP_PART_DIR_TEMPLATE, uploadID)

	// 如果maxParts为0，清理所有分片
	if maxParts == 0 {
		// 列出所有临时分片
		objectsCh := ts.client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		})

		for object := range objectsCh {
			if object.Err != nil {
				hlog.Errorf("列出临时分片失败: %v", object.Err)
				continue
			}

			err := ts.client.RemoveObject(ctx, bucketName, object.Key, minio.RemoveObjectOptions{})
			if err != nil {
				hlog.Errorf("删除临时分片 %s 失败: %v", object.Key, err)
			}
		}
	} else {
		// 删除指定数量的分片
		for i := 1; i <= maxParts; i++ {
			tempObjectName := fmt.Sprintf(TEMP_PART_TEMPLATE, uploadID, i)
			err := ts.client.RemoveObject(ctx, bucketName, tempObjectName, minio.RemoveObjectOptions{})
			if err != nil {
				hlog.Errorf("删除临时分片 %s 失败: %v", tempObjectName, err)
			}
		}
	}

	hlog.Infof("已清理上传ID %s 的临时分片", uploadID)
}

// ============== 缩略图生成和上传功能 ==============

// GenerateAndUploadThumbnails 从视频文件生成多尺寸缩略图并上传
// videoFile: 本地视频文件路径
// userID: 用户ID
// videoID: 视频ID
// timeOffset: 时间偏移（秒），-1表示自动选择最佳帧
func (ts *TikTokStorage) GenerateAndUploadThumbnails(ctx context.Context, videoFile string, userID, videoID int64, timeOffset int) (map[string]string, error) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("thumbnails_%d_%d", userID, videoID))
	defer os.RemoveAll(tempDir) // 清理临时文件

	// 生成多尺寸缩略图
	sizes := []utils.ThumbnailSize{
		utils.ThumbnailSmall,
		utils.ThumbnailMedium,
		utils.ThumbnailLarge,
	}

	thumbnailFiles, err := utils.GetVideoThumbnails(videoFile, tempDir, timeOffset, sizes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate thumbnails: %w", err)
	}

	// 上传所有缩略图
	thumbnailURLs := make(map[string]string)

	for size, filePath := range thumbnailFiles {
		// 生成存储路径
		thumbnailPath := ts.GetThumbnailPath(userID, videoID, size)

		// 读取文件内容
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read thumbnail %s: %w", size, err)
		}

		// 上传到存储
		_, err = ts.client.PutObject(
			ctx,
			BUCKET_USER_CONTENT,
			thumbnailPath,
			bytes.NewReader(data),
			int64(len(data)),
			minio.PutObjectOptions{
				ContentType: "image/jpeg",
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to upload thumbnail %s: %w", size, err)
		}

		// 生成访问URL
		thumbnailURLs[size] = ts.generateURL(BUCKET_USER_CONTENT, thumbnailPath)

		hlog.Infof("Successfully uploaded thumbnail %s for video %d", size, videoID)
	}

	return thumbnailURLs, nil
}

// GenerateAndUploadAnimatedCover 从视频生成动态封面（GIF）并上传
// videoFile: 本地视频文件路径
// userID: 用户ID
// videoID: 视频ID
// startTime: 开始时间（秒）
// duration: 持续时间（秒）
func (ts *TikTokStorage) GenerateAndUploadAnimatedCover(ctx context.Context, videoFile string, userID, videoID int64, startTime, duration int) (string, error) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("animated_cover_%d_%d", userID, videoID))
	defer os.RemoveAll(tempDir)

	// 生成动态封面
	animatedCoverFile, err := utils.GetAnimatedCover(videoFile, tempDir, startTime, duration, 5, 320)
	if err != nil {
		return "", fmt.Errorf("failed to generate animated cover: %w", err)
	}

	// 读取文件内容
	data, err := os.ReadFile(animatedCoverFile)
	if err != nil {
		return "", fmt.Errorf("failed to read animated cover: %w", err)
	}

	// 生成存储路径
	animatedCoverPath := ts.getAnimatedCoverPath(userID, videoID)

	// 上传到存储
	_, err = ts.client.PutObject(
		ctx,
		BUCKET_USER_CONTENT,
		animatedCoverPath,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{
			ContentType: "image/gif",
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload animated cover: %w", err)
	}

	// 生成访问URL
	url := ts.generateURL(BUCKET_USER_CONTENT, animatedCoverPath)

	hlog.Infof("Successfully uploaded animated cover for video %d", videoID)
	return url, nil
}

// ExtractBestThumbnail 从视频自动选择最佳帧作为缩略图
// videoFile: 本地视频文件路径
// userID: 用户ID
// videoID: 视频ID
// 返回中等尺寸缩略图URL
func (ts *TikTokStorage) ExtractBestThumbnail(ctx context.Context, videoFile string, userID, videoID int64) (string, error) {
	// 自动选择最佳帧（从视频中间位置抽取）
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("best_thumbnail_%d_%d", userID, videoID))
	defer os.RemoveAll(tempDir)

	// 生成中等尺寸缩略图
	thumbnailFile, err := utils.GetVideoThumbnail(videoFile, tempDir, -1, 320, 180)
	if err != nil {
		return "", fmt.Errorf("failed to extract best thumbnail: %w", err)
	}

	// 读取文件内容
	data, err := os.ReadFile(thumbnailFile)
	if err != nil {
		return "", fmt.Errorf("failed to read thumbnail: %w", err)
	}

	// 生成存储路径
	thumbnailPath := ts.GetThumbnailPath(userID, videoID, "medium")

	// 上传到存储
	_, err = ts.client.PutObject(
		ctx,
		BUCKET_USER_CONTENT,
		thumbnailPath,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{
			ContentType: "image/jpeg",
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload thumbnail: %w", err)
	}

	// 生成访问URL
	url := ts.generateURL(BUCKET_USER_CONTENT, thumbnailPath)

	hlog.Infof("Successfully extracted and uploaded best thumbnail for video %d", videoID)
	return url, nil
}

// DownloadFile 从MinIO下载文件到本地路径
func (ts *TikTokStorage) DownloadFile(ctx context.Context, bucketName, objectName, localPath string) error {
	// 获取对象
	object, err := ts.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to get object from MinIO: %w", err)
	}
	defer object.Close()

	// 创建本地文件
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	// 复制数据
	_, err = io.Copy(localFile, object)
	if err != nil {
		return fmt.Errorf("failed to copy data to local file: %w", err)
	}

	return nil
}

// UploadBytes 上传字节数据到MinIO
func (ts *TikTokStorage) UploadBytes(ctx context.Context, bucketName, objectName string, data []byte, contentType string) error {
	reader := bytes.NewReader(data)
	_, err := ts.client.PutObject(ctx, bucketName, objectName, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload bytes to MinIO: %w", err)
	}
	return nil
}
