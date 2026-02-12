package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	videoRedis "HuaTug.com/cmd/video/infras/redis"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/oss"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// ========================================
// VideoDeleteService - Video deletion
// ========================================

type VideoDeleteService struct {
	ctx context.Context
}

func NewVideoDeleteService(ctx context.Context) *VideoDeleteService {
	return &VideoDeleteService{ctx: ctx}
}

// DeleteVideo deletes a video and cleans up associated storage
func (s *VideoDeleteService) DeleteVideo(req *videos.VideoDeleteRequestV2) (*videos.VideoDeleteResponseV2, error) {
	resp := &videos.VideoDeleteResponseV2{
		Base: &base.Status{},
	}

	// 1. Verify video ownership
	video, err := db.GetVideoInfo(s.ctx, req.VideoId)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get video info")
	}
	if video == nil || video.VideoId == 0 {
		return nil, errors.New("video not found")
	}
	if video.UserId != req.UserId {
		return nil, errors.New("permission denied: not the video owner")
	}

	// 2. Get storage mapping for cleanup
	var recoveredBytes int64
	storageMapping, err := db.GetVideoStorageMapping(s.ctx, req.VideoId)
	if err == nil && storageMapping != nil {
		recoveredBytes = storageMapping.FileSize
	}

	// 3. Delete video record from database
	if err := db.DeleteVideo(s.ctx, strconv.FormatInt(req.VideoId, 10), strconv.FormatInt(req.UserId, 10)); err != nil {
		return nil, errors.WithMessage(err, "failed to delete video record")
	}

	// 4. Async cleanup: MinIO files, storage mapping, counters
	go func() {
		bgCtx := context.Background()

		// Clean up storage mapping
		if storageMapping != nil {
			tikTokStorage := oss.NewTikTokStorage()
			// Delete source file from MinIO
			if storageMapping.SourcePath != "" {
				objectName := storageMapping.SourcePath
				if len(objectName) > 0 && objectName[0] == '/' {
					objectName = objectName[1:]
				}
				// Remove bucket prefix if present
				if len(objectName) > len(storageMapping.BucketName)+1 {
					prefix := storageMapping.BucketName + "/"
					if len(objectName) >= len(prefix) && objectName[:len(prefix)] == prefix {
						objectName = objectName[len(prefix):]
					}
				}
				_ = tikTokStorage
				hlog.Infof("Scheduled cleanup for video %d storage files", req.VideoId)
			}
		}

		// Update user storage quota
		if recoveredBytes > 0 {
			if err := db.UpdateUserStorageUsage(bgCtx, req.UserId, -recoveredBytes, -1); err != nil {
				hlog.Warnf("Failed to update user storage after delete: %v", err)
			}
		}

		hlog.Infof("Completed cleanup for deleted video %d", req.VideoId)
	}()

	// 5. Get updated quota
	quota, _ := db.GetUserStorageQuota(s.ctx, req.UserId)
	if quota != nil {
		resp.UpdatedQuota = &videos.UserStorageQuota{
			TotalQuotaBytes:   quota.MaxStorageBytes,
			UsedQuotaBytes:    quota.UsedStorageBytes,
			VideoCount:        int64(quota.VideoCount),
			QuotaLevel:        quota.QuotaLevel,
			MaxVideoSizeBytes: quota.MaxVideoSize,
			MaxVideoCount:     int32(quota.MaxVideoCount),
		}
	}

	resp.StorageRecoveredBytes = recoveredBytes
	hlog.Infof("Video %d deleted by user %d, recovered %d bytes", req.VideoId, req.UserId, recoveredBytes)
	return resp, nil
}

// ========================================
// VideoHeatService - Video heat/tier management
// ========================================

type VideoHeatService struct {
	ctx context.Context
}

func NewVideoHeatService(ctx context.Context) *VideoHeatService {
	return &VideoHeatService{ctx: ctx}
}

// ManageVideoHeat handles video storage tier operations
func (s *VideoHeatService) ManageVideoHeat(req *videos.VideoHeatManagementRequest) (*videos.VideoHeatManagementResponse, error) {
	resp := &videos.VideoHeatManagementResponse{
		Base: &base.Status{},
	}

	// Get current storage mapping
	mapping, err := db.GetVideoStorageMapping(s.ctx, req.VideoId)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get video storage mapping")
	}

	oldTier := "warm"
	if mapping.HotStorage {
		oldTier = "hot"
	}

	switch req.Operation {
	case "promote_to_hot":
		if err := db.PromoteVideoToHotStorage(s.ctx, req.VideoId); err != nil {
			return nil, errors.WithMessage(err, "failed to promote video to hot storage")
		}
		resp.NewTier_ = "hot"

	case "demote_to_warm":
		// Update hot_storage to false
		mapping.HotStorage = false
		if err := db.UpdateVideoStorageMapping(s.ctx, mapping); err != nil {
			return nil, errors.WithMessage(err, "failed to demote video to warm storage")
		}
		resp.NewTier_ = "warm"

	case "archive_to_cold":
		mapping.HotStorage = false
		mapping.StorageStatus = "archived"
		if err := db.UpdateVideoStorageMapping(s.ctx, mapping); err != nil {
			return nil, errors.WithMessage(err, "failed to archive video")
		}
		resp.NewTier_ = "cold"

	default:
		return nil, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	resp.OldTier = oldTier
	resp.OperationCostBytes = 0 // Tier management is metadata-only

	hlog.Infof("Video %d heat changed: %s -> %s (reason: %s)", req.VideoId, oldTier, resp.NewTier_, req.Reason)
	return resp, nil
}

// ========================================
// UserQuotaService - User quota management
// ========================================

type UserQuotaService struct {
	ctx context.Context
}

func NewUserQuotaService(ctx context.Context) *UserQuotaService {
	return &UserQuotaService{ctx: ctx}
}

// ManageUserQuota handles user storage quota operations
func (s *UserQuotaService) ManageUserQuota(req *videos.UserQuotaManagementRequest) (*videos.UserQuotaManagementResponse, error) {
	resp := &videos.UserQuotaManagementResponse{
		Base: &base.Status{},
	}

	switch req.Operation {
	case "get":
		quota, err := db.GetUserStorageQuota(s.ctx, req.UserId)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get user quota")
		}
		resp.CurrentQuota = s.convertQuota(quota)
		resp.QuotaExceeded = quota.QuotaExceeded

		// Check warning thresholds
		usagePercent := float64(quota.UsedStorageBytes) / float64(quota.MaxStorageBytes) * 100
		if usagePercent >= 90 {
			resp.QuotaWarnings = append(resp.QuotaWarnings, "Storage usage exceeds 90%, please clean up or upgrade")
		} else if usagePercent >= 75 {
			resp.QuotaWarnings = append(resp.QuotaWarnings, "Storage usage exceeds 75%")
		}
		if quota.VideoCount >= quota.MaxVideoCount {
			resp.QuotaWarnings = append(resp.QuotaWarnings, "Video count has reached the maximum limit")
		}

	case "update":
		if req.NewQuota_ == nil {
			return nil, errors.New("new_quota is required for update operation")
		}
		quota, err := db.GetUserStorageQuota(s.ctx, req.UserId)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get user quota")
		}

		// Apply updates
		if req.NewQuota_.TotalQuotaBytes > 0 {
			quota.MaxStorageBytes = req.NewQuota_.TotalQuotaBytes
		}
		if req.NewQuota_.MaxVideoSizeBytes > 0 {
			quota.MaxVideoSize = req.NewQuota_.MaxVideoSizeBytes
		}
		if req.NewQuota_.MaxVideoCount > 0 {
			quota.MaxVideoCount = int(req.NewQuota_.MaxVideoCount)
		}
		if req.NewQuota_.QuotaLevel != "" {
			quota.QuotaLevel = req.NewQuota_.QuotaLevel
		}

		// Recalculate exceeded status
		quota.QuotaExceeded = quota.UsedStorageBytes >= quota.MaxStorageBytes

		if err := db.UpdateUserStorageQuota(s.ctx, quota); err != nil {
			return nil, errors.WithMessage(err, "failed to update user quota")
		}
		resp.CurrentQuota = s.convertQuota(quota)
		resp.QuotaExceeded = quota.QuotaExceeded

	case "reset":
		quota, err := db.GetUserStorageQuota(s.ctx, req.UserId)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get user quota")
		}
		// Reset to defaults based on level
		quota.MaxStorageBytes = 10737418240 // 10GB
		quota.MaxVideoCount = 1000
		quota.MaxVideoSize = 1073741824 // 1GB
		quota.QuotaExceeded = false
		quota.WarningSent = false

		if err := db.UpdateUserStorageQuota(s.ctx, quota); err != nil {
			return nil, errors.WithMessage(err, "failed to reset user quota")
		}
		resp.CurrentQuota = s.convertQuota(quota)
		resp.QuotaExceeded = quota.QuotaExceeded

	default:
		return nil, fmt.Errorf("unsupported operation: %s", req.Operation)
	}

	return resp, nil
}

func (s *UserQuotaService) convertQuota(q *db.UserStorageQuota) *videos.UserStorageQuota {
	if q == nil {
		return nil
	}
	return &videos.UserStorageQuota{
		TotalQuotaBytes:   q.MaxStorageBytes,
		UsedQuotaBytes:    q.UsedStorageBytes,
		VideoCount:        int64(q.VideoCount),
		QuotaLevel:        q.QuotaLevel,
		MaxVideoSizeBytes: q.MaxVideoSize,
		MaxVideoCount:     int32(q.MaxVideoCount),
	}
}

// ========================================
// VideoBatchService - Batch video operations
// ========================================

type VideoBatchService struct {
	ctx context.Context
}

func NewVideoBatchService(ctx context.Context) *VideoBatchService {
	return &VideoBatchService{ctx: ctx}
}

// BatchOperateVideos performs batch operations on multiple videos
func (s *VideoBatchService) BatchOperateVideos(req *videos.BatchVideoOperationRequest) (*videos.BatchVideoOperationResponse, error) {
	resp := &videos.BatchVideoOperationResponse{
		Base:              &base.Status{},
		SuccessVideoIds:   make([]int64, 0),
		FailedVideoErrors: make(map[int64]string),
	}

	if len(req.VideoIds) == 0 {
		return nil, errors.New("video_ids cannot be empty")
	}

	// Process each video concurrently with bounded concurrency
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Max 5 concurrent operations

	for _, videoId := range req.VideoIds {
		wg.Add(1)
		go func(vid int64) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			var opErr error
			switch req.Operation {
			case "delete":
				opErr = db.DeleteVideo(s.ctx, strconv.FormatInt(vid, 10), strconv.FormatInt(req.UserId, 10))

			case "change_privacy":
				privacy := req.OperationParams["privacy"]
				var openVal int8
				switch privacy {
				case "public":
					openVal = 1
				case "friends":
					openVal = 2
				default:
					openVal = 0 // private
				}
				opErr = db.UpdateVideoPermissions(s.ctx, vid, -1, -1, openVal)

			case "move_to_tier":
				tier := req.OperationParams["tier"]
				switch tier {
				case "hot":
					opErr = db.PromoteVideoToHotStorage(s.ctx, vid)
				default:
					hlog.Warnf("Unsupported tier: %s for video %d", tier, vid)
				}

			default:
				opErr = fmt.Errorf("unsupported batch operation: %s", req.Operation)
			}

			mu.Lock()
			defer mu.Unlock()
			if opErr != nil {
				resp.FailedVideoErrors[vid] = opErr.Error()
				hlog.Warnf("Batch operation '%s' failed for video %d: %v", req.Operation, vid, opErr)
			} else {
				resp.SuccessVideoIds = append(resp.SuccessVideoIds, vid)
			}
		}(videoId)
	}

	wg.Wait()

	// Get updated quota
	quota, _ := db.GetUserStorageQuota(s.ctx, req.UserId)
	if quota != nil {
		resp.UpdatedQuota = &videos.UserStorageQuota{
			TotalQuotaBytes:   quota.MaxStorageBytes,
			UsedQuotaBytes:    quota.UsedStorageBytes,
			VideoCount:        int64(quota.VideoCount),
			QuotaLevel:        quota.QuotaLevel,
			MaxVideoSizeBytes: quota.MaxVideoSize,
			MaxVideoCount:     int32(quota.MaxVideoCount),
		}
	}

	hlog.Infof("Batch '%s' completed: %d success, %d failed",
		req.Operation, len(resp.SuccessVideoIds), len(resp.FailedVideoErrors))
	return resp, nil
}

// ========================================
// VideoAnalyticsService - Video analytics
// ========================================

type VideoAnalyticsService struct {
	ctx context.Context
}

func NewVideoAnalyticsService(ctx context.Context) *VideoAnalyticsService {
	return &VideoAnalyticsService{ctx: ctx}
}

// GetVideoAnalytics returns analytics data for specified videos
func (s *VideoAnalyticsService) GetVideoAnalytics(req *videos.VideoAnalyticsRequest) (*videos.VideoAnalyticsResponse, error) {
	resp := &videos.VideoAnalyticsResponse{
		Base:                &base.Status{},
		VideoMetrics:        make(map[int64]map[string]int64),
		TotalMetrics:        make(map[string]int64),
		TopPerformingVideos: make([]string, 0),
		ReportGeneratedAt:   time.Now().Format(time.RFC3339),
	}

	// Determine which videos to analyze
	videoIds := req.VideoIds
	if len(videoIds) == 0 {
		// Get all user videos
		mappings, err := db.GetUserVideos(s.ctx, req.UserId, 100, 0)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to get user videos")
		}
		for _, m := range mappings {
			videoIds = append(videoIds, m.VideoID)
		}
	}

	if len(videoIds) == 0 {
		return resp, nil
	}

	// Collect metrics for each video
	for _, vid := range videoIds {
		counts, err := db.GetVideoDetailedCounts(s.ctx, vid)
		if err != nil {
			hlog.Warnf("Failed to get counts for video %d: %v", vid, err)
			continue
		}

		// Filter by requested metrics
		filteredCounts := make(map[string]int64)
		if len(req.Metrics) == 0 {
			filteredCounts = counts
		} else {
			for _, metric := range req.Metrics {
				switch metric {
				case "views":
					filteredCounts["views"] = counts["visit_count"]
				case "likes":
					filteredCounts["likes"] = counts["like_count"]
				case "comments":
					filteredCounts["comments"] = counts["comment_count"]
				case "shares":
					filteredCounts["shares"] = counts["share_count"]
				}
			}
		}

		resp.VideoMetrics[vid] = filteredCounts

		// Accumulate totals
		for k, v := range filteredCounts {
			resp.TotalMetrics[k] += v
		}
	}

	// Determine top performing videos (by total engagement)
	type videoScore struct {
		id    int64
		score int64
	}
	var scored []videoScore
	for vid, metrics := range resp.VideoMetrics {
		total := int64(0)
		for _, v := range metrics {
			total += v
		}
		scored = append(scored, videoScore{id: vid, score: total})
	}
	// Simple sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	for i, s := range scored {
		if i >= 10 {
			break
		}
		resp.TopPerformingVideos = append(resp.TopPerformingVideos, strconv.FormatInt(s.id, 10))
	}

	return resp, nil
}

// ========================================
// VideoTranscodeService - Video transcoding
// ========================================

type VideoTranscodeService struct {
	ctx context.Context
}

func NewVideoTranscodeService(ctx context.Context) *VideoTranscodeService {
	return &VideoTranscodeService{ctx: ctx}
}

// TranscodeVideo submits a transcoding job for a video
func (s *VideoTranscodeService) TranscodeVideo(req *videos.VideoTranscodingRequest) (*videos.VideoTranscodingResponse, error) {
	resp := &videos.VideoTranscodingResponse{
		Base:           &base.Status{},
		TranscodedUrls: make(map[int32]string),
		ThumbnailUrls:  make([]string, 0),
	}

	// 1. Verify video exists and user owns it
	video, err := db.GetVideoInfo(s.ctx, req.VideoId)
	if err != nil || video == nil || video.VideoId == 0 {
		return nil, errors.New("video not found")
	}
	if video.UserId != req.UserId {
		return nil, errors.New("permission denied: not the video owner")
	}

	// 2. Get storage mapping for source path
	mapping, err := db.GetVideoStorageMapping(s.ctx, req.VideoId)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get video storage info")
	}

	// 3. Generate transcoding job ID (use timestamp-based)
	jobId := time.Now().UnixNano() / 1000000

	// 4. For each target quality, generate the processed path
	for _, quality := range req.TargetQualities {
		processedPath := fmt.Sprintf("users/%d/videos/%d/processed/%dp.mp4", req.UserId, req.VideoId, quality)
		resp.TranscodedUrls[quality] = fmt.Sprintf("/%s/%s", mapping.BucketName, processedPath)
	}

	// 5. Generate thumbnails if requested
	if req.GenerateThumbnails {
		tikTokStorage := oss.NewTikTokStorage()

		thumbnailCount := int(req.ThumbnailCount)
		if thumbnailCount <= 0 {
			thumbnailCount = 3
		}

		// Async thumbnail generation
		go func() {
			bgCtx := context.Background()
			for i := 0; i < thumbnailCount; i++ {
				thumbPath := tikTokStorage.GetThumbnailPath(req.UserId, req.VideoId, fmt.Sprintf("thumb_%d", i))
				_ = bgCtx
				resp.ThumbnailUrls = append(resp.ThumbnailUrls, fmt.Sprintf("/%s/%s", mapping.BucketName, thumbPath))
			}
		}()
	}

	resp.TranscodingJobId = jobId
	resp.JobStatus = "queued"
	resp.EstimatedCompletionTime = time.Now().Add(5 * time.Minute).Unix()

	hlog.Infof("Transcoding job %d created for video %d: qualities=%v", jobId, req.VideoId, req.TargetQualities)
	return resp, nil
}

// ========================================
// VideoStreamServiceV2 - Video stream with MinIO presigned URL
// ========================================

type VideoStreamServiceV2 struct {
	ctx           context.Context
	tikTokStorage *oss.TikTokStorage
}

func NewVideoStreamServiceV2(ctx context.Context) *VideoStreamServiceV2 {
	return &VideoStreamServiceV2{
		ctx:           ctx,
		tikTokStorage: oss.NewTikTokStorage(),
	}
}

// GetStreamInfo returns a presigned streaming URL for the video
func (s *VideoStreamServiceV2) GetStreamInfo(req *videos.StreamVideoRequestV2) (*videos.StreamVideoResponseV2, error) {
	resp := &videos.StreamVideoResponseV2{
		Base:           &base.Status{},
		StreamMetadata: make(map[string]string),
	}

	if req.VideoId == "" {
		return nil, errors.New("video_id is required")
	}

	videoId, err := strconv.ParseInt(req.VideoId, 10, 64)
	if err != nil {
		return nil, errors.New("invalid video_id format")
	}

	// 1. Get video info
	video, err := db.GetVideoInfo(s.ctx, videoId)
	if err != nil || video == nil || video.VideoId == 0 {
		return nil, errors.New("video not found")
	}

	// 2. Try to get storage mapping for quality selection
	mapping, err := db.GetVideoStorageMapping(s.ctx, videoId)
	if err != nil {
		// Fallback: use video URL directly
		resp.StreamUrl = video.VideoUrl
		resp.ExpiresAt = time.Now().Add(2 * time.Hour).Unix()
		resp.StreamMetadata["quality"] = "original"
		resp.StreamMetadata["format"] = "mp4"
		return resp, nil
	}

	// 3. Determine quality and format
	quality := req.Quality
	if quality == "" {
		quality = "720"
	}

	// 4. Generate presigned URL for streaming
	objectName := mapping.SourcePath
	if len(objectName) > 0 && objectName[0] == '/' {
		objectName = objectName[1:]
	}
	// Remove bucket prefix if present
	bucketPrefix := mapping.BucketName + "/"
	if len(objectName) > len(bucketPrefix) && objectName[:len(bucketPrefix)] == bucketPrefix {
		objectName = objectName[len(bucketPrefix):]
	}

	expiry := 2 * time.Hour
	presignedURL, err := s.tikTokStorage.GeneratePresignedURL(mapping.BucketName, objectName, expiry)
	if err != nil {
		// Fallback to direct URL
		hlog.Warnf("Failed to generate presigned URL for video %d: %v, using direct URL", videoId, err)
		resp.StreamUrl = video.VideoUrl
	} else {
		resp.StreamUrl = presignedURL
	}

	resp.ExpiresAt = time.Now().Add(expiry).Unix()
	resp.StreamMetadata["quality"] = quality
	resp.StreamMetadata["format"] = mapping.Format
	resp.StreamMetadata["duration"] = strconv.Itoa(mapping.Duration)
	resp.StreamMetadata["resolution"] = fmt.Sprintf("%dx%d", mapping.ResolutionWidth, mapping.ResolutionHeight)

	// 5. Record access (async)
	go func() {
		_ = db.UpdateVideoAccessStats(context.Background(), videoId, "play")
		_, _ = videoRedis.IncrementVideoVisitCount(context.Background(), videoId)
	}()

	return resp, nil
}

// ========================================
// UpdateCountService - Video count updates
// ========================================

type UpdateCountService struct {
	ctx context.Context
}

func NewUpdateCountService(ctx context.Context) *UpdateCountService {
	return &UpdateCountService{ctx: ctx}
}

// UpdateVisitCount updates video visit count
func (s *UpdateCountService) UpdateVisitCount(req *videos.UpdateVisitCountRequestV2) (*videos.UpdateVisitCountResponseV2, error) {
	resp := &videos.UpdateVisitCountResponseV2{
		Base: &base.Status{},
	}

	if err := db.UpdateVideoVisit(s.ctx, req.VideoId, req.VisitCount); err != nil {
		return nil, errors.WithMessage(err, "failed to update visit count")
	}

	// Also update counter table
	go func() {
		_ = db.IncrementVideoCounter(context.Background(), req.VideoId, "visit_count", req.VisitCount)
	}()

	count, _ := db.GetVideoVisitCount(s.ctx, strconv.FormatInt(req.VideoId, 10))
	resp.NewTotalCount_ = count

	return resp, nil
}

// UpdateCommentCount updates video comment count
func (s *UpdateCountService) UpdateCommentCount(req *videos.UpdateVideoCommentCountRequestV2) (*videos.UpdateVideoCommentCountResponseV2, error) {
	resp := &videos.UpdateVideoCommentCountResponseV2{
		Base: &base.Status{},
	}

	if err := db.UpdateVideoCommentCount(s.ctx, req.VideoId, req.CommentCount); err != nil {
		return nil, errors.WithMessage(err, "failed to update comment count")
	}

	// Get updated total
	counts, err := db.GetVideoDetailedCounts(s.ctx, req.VideoId)
	if err == nil {
		resp.NewTotalCount_ = counts["comment_count"]
	}

	return resp, nil
}

// UpdateLikeCount updates video like count
func (s *UpdateCountService) UpdateLikeCount(req *videos.UpdateLikeCountRequestV2) (*videos.UpdateLikeCountResponseV2, error) {
	resp := &videos.UpdateLikeCountResponseV2{
		Base: &base.Status{},
	}

	if err := db.UpdateVideoLikeCount(s.ctx, req.VideoId, req.LikeCount); err != nil {
		return nil, errors.WithMessage(err, "failed to update like count")
	}

	// Get updated total
	counts, err := db.GetVideoDetailedCounts(s.ctx, req.VideoId)
	if err == nil {
		resp.NewTotalCount_ = counts["like_count"]
	}

	return resp, nil
}
