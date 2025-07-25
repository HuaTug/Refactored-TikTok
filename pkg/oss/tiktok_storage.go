package oss

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/minio/minio-go/v7"
)

// 存储桶常量
const (
	BUCKET_USER_CONTENT  = "tiktok-user-content"  // 用户生成内容
	BUCKET_SYSTEM_ASSETS = "tiktok-system-assets" // 系统资源
	BUCKET_CACHE_HOT     = "tiktok-cache-hot"     // 热点缓存
	BUCKET_CACHE_WARM    = "tiktok-cache-warm"    // 温数据缓存
	BUCKET_CACHE_COLD    = "tiktok-cache-cold"    // 冷数据存储
	BUCKET_ANALYTICS     = "tiktok-analytics"     // 分析数据
)

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

// NewTikTokStorage 创建新的存储服务实例
func NewTikTokStorage() *TikTokStorage {
	return &TikTokStorage{
		client: minioClient,
	}
}

// 初始化存储桶
func (ts *TikTokStorage) InitializeBuckets(ctx context.Context) error {
	buckets := []string{
		BUCKET_USER_CONTENT,
		BUCKET_SYSTEM_ASSETS,
		BUCKET_CACHE_HOT,
		BUCKET_CACHE_WARM,
		BUCKET_CACHE_COLD,
		BUCKET_ANALYTICS,
	}

	for _, bucketName := range buckets {
		exists, err := ts.client.BucketExists(ctx, bucketName)
		if err != nil {
			return fmt.Errorf("check bucket %s error: %w", bucketName, err)
		}

		if !exists {
			err = ts.client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
				Region: "us-east-1",
			})
			if err != nil {
				return fmt.Errorf("create bucket %s error: %w", bucketName, err)
			}
			hlog.Infof("Created bucket: %s", bucketName)
		}
	}

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

// 路径生成方法
func (ts *TikTokStorage) getSourceVideoPath(userID, videoID int64) string {
	return fmt.Sprintf("users/%d/videos/%d/source/original.mp4", userID, videoID)
}

func (ts *TikTokStorage) getProcessedVideoPath(userID, videoID int64, quality int) string {
	return fmt.Sprintf("users/%d/videos/%d/processed/video_%dp.mp4", userID, videoID, quality)
}

func (ts *TikTokStorage) GetThumbnailPath(userID, videoID int64, size string) string {
	return fmt.Sprintf("users/%d/videos/%d/thumbnails/thumb_%s.jpg", userID, videoID, size)
}

func (ts *TikTokStorage) getAnimatedCoverPath(userID, videoID int64) string {
	return fmt.Sprintf("users/%d/videos/%d/thumbnails/animated_cover.gif", userID, videoID)
}

func (ts *TikTokStorage) getVideoMetadataPath(userID, videoID int64) string {
	return fmt.Sprintf("users/%d/videos/%d/metadata/info.json", userID, videoID)
}

func (ts *TikTokStorage) getUserAvatarPath(userID int64, size string) string {
	return fmt.Sprintf("users/%d/profile/avatar/avatar_%s.jpg", userID, size)
}

func (ts *TikTokStorage) getUserBackgroundPath(userID int64) string {
	return fmt.Sprintf("users/%d/profile/background/bg_image.jpg", userID)
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

// 确保用户目录结构存在
func (ts *TikTokStorage) ensureUserDirectoryStructure(ctx context.Context, userID int64) error {
	directories := []string{
		fmt.Sprintf("users/%d/profile/avatar/", userID),
		fmt.Sprintf("users/%d/profile/background/", userID),
		fmt.Sprintf("users/%d/videos/", userID),
		fmt.Sprintf("users/%d/drafts/", userID),
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

// 生成URL
func (ts *TikTokStorage) generateURL(bucketName, objectName string) string {
	return fmt.Sprintf("http://localhost:9091/browser/%s/%s", bucketName, objectName)
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

// 热度分层存储：将热门视频提升到热点缓存
func (ts *TikTokStorage) PromoteToHotStorage(ctx context.Context, userID, videoID int64) error {
	sourcePath := ts.getProcessedVideoPath(userID, videoID, 720)
	hotPath := fmt.Sprintf("hot/users/%d/videos/%d/video_720p.mp4", userID, videoID)

	return ts.copyObject(ctx, BUCKET_USER_CONTENT, sourcePath, BUCKET_CACHE_HOT, hotPath)
}

// 检查视频是否在热点存储中
func (ts *TikTokStorage) IsInHotStorage(ctx context.Context, userID, videoID int64) (bool, error) {
	hotPath := fmt.Sprintf("hot/users/%d/videos/%d/video_720p.mp4", userID, videoID)

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

// UploadUserAvatar 上传用户头像（TikTok风格）
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
		avatarPath := fmt.Sprintf("users/%d/profile/avatar/avatar_%s%s", userID, size, suffix)

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

// GetOptimalVideoPath 根据设备类型和网络状况选择最优视频路径
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
		objectPath = fmt.Sprintf("hot/users/%d/videos/%d/video_%dp.mp4", userID, videoID, selectedQuality)
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
