package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/oss"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// StreamProxyParam 视频流代理参数
type StreamProxyParam struct {
	Path     string `query:"path" form:"path"`
	VideoId  string `query:"video_id" form:"video_id"`
	Duration string `query:"duration" form:"duration"`
	Quality  string `query:"quality" form:"quality"` // 新增：视频质量参数
}

// VideoStreamProxy 视频流代理处理器 - 支持TikTok存储架构
func VideoStreamProxy(ctx context.Context, c *app.RequestContext) {
	var params StreamProxyParam
	if err := c.BindAndValidate(&params); err != nil {
		hlog.Errorf("Failed to bind stream proxy params: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 处理不同类型的视频URL
	var targetURL string
	var err error

	if params.VideoId != "" {
		// 优先尝试TikTok新架构
		userAgent := string(c.GetHeader("User-Agent"))
		targetURL, err = generateTikTokVideoURL(params.VideoId, userAgent, params.Quality)
		if err != nil {
			hlog.Warnf("Failed to generate TikTok URL for video %s: %v, falling back to legacy", params.VideoId, err)

			// 回退到旧格式
			targetURL, err = generateMinIOVideoURL(params.VideoId)
			if err != nil {
				hlog.Errorf("Failed to generate video URL for video %s: %v", params.VideoId, err)
				c.JSON(consts.StatusInternalServerError, map[string]interface{}{
					"error": "Failed to get video URL",
				})
				return
			}
		}

		// 记录访问日志（异步）
		go func() {
			videoIDInt, _ := strconv.ParseInt(params.VideoId, 10, 64)
			logVideoAccess(context.Background(), videoIDInt, c)
		}()

	} else if params.Path != "" {
		// 直接使用提供的路径
		if strings.HasPrefix(params.Path, "http") {
			targetURL = params.Path
		} else {
			// 假设是MinIO路径，构建完整URL
			targetURL = fmt.Sprintf("http://localhost:9091/browser/video/%s", params.Path)
		}
	} else {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"error": "Missing video_id or path parameter",
		})
		return
	}

	hlog.Infof("Proxying video stream from: %s", targetURL)

	// 创建HTTP请求到目标URL
	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		hlog.Errorf("Failed to create request: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to create proxy request",
		})
		return
	}

	// 复制原始请求的Range头（支持分段下载）
	if rangeHeader := string(c.GetHeader("Range")); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		hlog.Errorf("Failed to fetch video: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to fetch video",
		})
		return
	}
	defer resp.Body.Close()

	// 设置响应头
	c.Header("Content-Type", "video/mp4")
	c.Header("Accept-Ranges", "bytes")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Range, Content-Range")
	c.Header("Cache-Control", "public, max-age=3600")

	// 复制响应头
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		c.Header("Content-Length", contentLength)
	}
	if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
		c.Header("Content-Range", contentRange)
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.Header("ETag", etag)
	}
	if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
		c.Header("Last-Modified", lastModified)
	}

	// 设置状态码
	c.Status(resp.StatusCode)

	// 流式传输视频数据
	_, err = io.Copy(c, resp.Body)
	if err != nil {
		hlog.Errorf("Failed to stream video data: %v", err)
		return
	}

	hlog.Infof("Successfully streamed video from: %s", targetURL)
}

// generateMinIOVideoURL 为指定的video_id生成MinIO预签名URL（兼容旧格式）
func generateMinIOVideoURL(videoId string) (string, error) {
	if videoId == "" {
		return "", fmt.Errorf("video_id cannot be empty")
	}

	// 兼容旧存储格式：video/{video_id}/video.mp4
	objectName := fmt.Sprintf("video/%s/video.mp4", videoId)
	hlog.Infof("Generating presigned URL for legacy format: %s", objectName)

	presignedURL, err := oss.GeneratePreUrl("video", objectName, videoId)
	if err != nil {
		hlog.Errorf("Failed to generate presigned URL for video %s: %v", videoId, err)
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	hlog.Infof("Generated presigned URL: %s", presignedURL)

	// 解析URL移除可能的查询参数
	parsedURL, err := url.Parse(presignedURL)
	if err != nil {
		hlog.Warnf("Failed to parse presigned URL, using original: %v", err)
		return presignedURL, nil // 如果解析失败，返回原URL
	}

	return parsedURL.String(), nil
}

// generateTikTokVideoURL 为指定的video_id生成TikTok风格的预签名URL
func generateTikTokVideoURL(videoId, userAgent, quality string) (string, error) {
	if videoId == "" {
		return "", fmt.Errorf("video_id cannot be empty")
	}

	videoIDInt, err := strconv.ParseInt(videoId, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid video_id format: %v", err)
	}

	// 从数据库查询存储映射
	storageMapping, err := db.GetVideoStorageMapping(context.Background(), videoIDInt)
	if err != nil {
		hlog.Warnf("Failed to get storage mapping for video %d: %v", videoIDInt, err)
		return "", fmt.Errorf("video not found in TikTok storage: %v", err)
	}

	tikTokStorage := oss.NewTikTokStorage()

	// 获取最优视频路径
	objectPath, err := tikTokStorage.GetOptimalVideoPath(storageMapping.UserID, videoIDInt, userAgent, quality)
	if err != nil {
		return "", fmt.Errorf("failed to get optimal video path: %v", err)
	}

	// 检查是否在热点存储
	var bucketName string
	if storageMapping.HotStorage {
		bucketName = oss.BUCKET_CACHE_HOT
		hlog.Infof("Using hot storage for video %d", videoIDInt)
	} else {
		bucketName = oss.BUCKET_USER_CONTENT
		hlog.Infof("Using standard storage for video %d", videoIDInt)
	}

	// 生成预签名URL
	presignedURL, err := tikTokStorage.GeneratePresignedURL(bucketName, objectPath, 30*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %v", err)
	}

	hlog.Infof("Generated TikTok presigned URL: %s", presignedURL)
	return presignedURL, nil
}

// logVideoAccess 记录视频访问日志
func logVideoAccess(ctx context.Context, videoID int64, c *app.RequestContext) {
	// 解析设备类型
	userAgent := string(c.GetHeader("User-Agent"))
	deviceType := parseDeviceType(userAgent)

	// 获取用户IP
	ipAddress := getClientIP(c)

	// 创建访问日志
	accessLog := &db.VideoAccessLog{
		VideoID:    videoID,
		AccessType: "view",
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		DeviceType: deviceType,
		Quality:    string(c.Query("quality")),
	}

	// 异步记录到数据库
	if err := db.LogVideoAccess(ctx, accessLog); err != nil {
		hlog.Warnf("Failed to log video access: %v", err)
	}

	// 更新视频访问统计
	if err := db.UpdateVideoAccessStats(ctx, videoID, "view"); err != nil {
		hlog.Warnf("Failed to update video access stats: %v", err)
	}
}

// parseDeviceType 解析设备类型
func parseDeviceType(userAgent string) string {
	userAgent = strings.ToLower(userAgent)

	if strings.Contains(userAgent, "mobile") || strings.Contains(userAgent, "android") || strings.Contains(userAgent, "iphone") {
		return "mobile"
	} else if strings.Contains(userAgent, "tablet") || strings.Contains(userAgent, "ipad") {
		return "tablet"
	} else if strings.Contains(userAgent, "mozilla") || strings.Contains(userAgent, "chrome") || strings.Contains(userAgent, "safari") {
		return "desktop"
	}

	return "unknown"
}

// getClientIP 获取客户端IP
func getClientIP(c *app.RequestContext) string {
	// 优先从代理头中获取真实IP
	if ip := string(c.GetHeader("X-Forwarded-For")); ip != "" {
		if ips := strings.Split(ip, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if ip := string(c.GetHeader("X-Real-IP")); ip != "" {
		return ip
	}

	return c.ClientIP()
}

// VideoThumbnailProxy 视频缩略图代理处理器
func VideoThumbnailProxy(ctx context.Context, c *app.RequestContext) {
	videoId := c.Query("video_id")
	size := c.Query("size") // small, medium, large

	if videoId == "" {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"error": "Missing video_id parameter",
		})
		return
	}

	if size == "" {
		size = "medium" // 默认中等尺寸
	}

	videoIDInt, err := strconv.ParseInt(videoId, 10, 64)
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"error": "Invalid video_id format",
		})
		return
	}

	// 从数据库查询存储映射
	storageMapping, err := db.GetVideoStorageMapping(ctx, videoIDInt)
	if err != nil {
		hlog.Errorf("Failed to get storage mapping for video %d: %v", videoIDInt, err)
		c.JSON(consts.StatusNotFound, map[string]interface{}{
			"error": "Video not found",
		})
		return
	}

	tikTokStorage := oss.NewTikTokStorage()
	thumbnailPath := tikTokStorage.GetThumbnailPath(storageMapping.UserID, videoIDInt, size)

	// 生成预签名URL
	presignedURL, err := tikTokStorage.GeneratePresignedURL(oss.BUCKET_USER_CONTENT, thumbnailPath, 30*time.Minute)
	if err != nil {
		hlog.Errorf("Failed to generate thumbnail URL: %v", err)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to generate thumbnail URL",
		})
		return
	}

	// 重定向到缩略图URL
	c.Redirect(consts.StatusFound, []byte(presignedURL))
}

// VideoMetadataProxy 视频元数据代理处理器
func VideoMetadataProxy(ctx context.Context, c *app.RequestContext) {
	videoId := c.Query("video_id")
	if videoId == "" {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"error": "Missing video_id parameter",
		})
		return
	}

	videoIDInt, err := strconv.ParseInt(videoId, 10, 64)
	if err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"error": "Invalid video_id format",
		})
		return
	}

	// 从数据库查询存储映射
	storageMapping, err := db.GetVideoStorageMapping(ctx, videoIDInt)
	if err != nil {
		hlog.Errorf("Failed to get storage mapping for video %d: %v", videoIDInt, err)
		c.JSON(consts.StatusNotFound, map[string]interface{}{
			"error": "Video not found",
		})
		return
	}

	// 构建元数据响应
	metadata := map[string]interface{}{
		"video_id": videoIDInt,
		"user_id":  storageMapping.UserID,
		"duration": storageMapping.Duration,
		"resolution": map[string]interface{}{
			"width":  storageMapping.ResolutionWidth,
			"height": storageMapping.ResolutionHeight,
		},
		"format":              storageMapping.Format,
		"codec":               storageMapping.Codec,
		"bitrate":             storageMapping.Bitrate,
		"file_size":           storageMapping.FileSize,
		"storage_status":      storageMapping.StorageStatus,
		"hot_storage":         storageMapping.HotStorage,
		"access_count":        storageMapping.AccessCount,
		"play_count":          storageMapping.PlayCount,
		"created_at":          storageMapping.CreatedAt,
		"available_qualities": []string{"480", "720", "1080"},
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data":    metadata,
	})
}
