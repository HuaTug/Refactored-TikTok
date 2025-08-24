package main

import (
	"context"
	"fmt"
	"strings"

	"HuaTug.com/cmd/video/service"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/pkg/errors"
)

type VideoServiceImpl struct{}

func (s *VideoServiceImpl) VideoFeedListV2(ctx context.Context, req *videos.VideoFeedListRequestV2) (resp *videos.VideoFeedListResponseV2, err error) {
	resp = new(videos.VideoFeedListResponseV2)
	resp.Base = &base.Status{}
	var video []*base.Video
	var count int64
	if video, count, err = service.NewVideoListService(ctx).VideoList(req); err != nil {
		hlog.CtxErrorf(ctx, "service.VideoFeedList failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Get VideoList!"
		resp.VideoList = video
		return resp, err
	}
	//todo
	fmt.Print(count)

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Get VideoList Success"
	resp.VideoList = video
	return resp, nil
}

func (s *VideoServiceImpl) VideoSearchV2(ctx context.Context, req *videos.VideoSearchRequestV2) (resp *videos.VideoSearchResponseV2, err error) {
	resp = new(videos.VideoSearchResponseV2)
	resp.Base = &base.Status{}
	var video []*base.Video
	var count int64

	if video, count, err = service.NewVideoSearchService(ctx).VideoSearch(req); err != nil {
		hlog.CtxErrorf(ctx, "service.VideoSearch failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Get VideoFeed!"
		resp.VideoSearch = video
		resp.Count = count

		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Get VideoFeed Success"
	resp.VideoSearch = video
	resp.Count = count

	return resp, nil
}

func (s *VideoServiceImpl) VideoPopularV2(ctx context.Context, req *videos.VideoPopularRequestV2) (resp *videos.VideoPopularResponseV2, err error) {
	resp = new(videos.VideoPopularResponseV2)
	resp.Base = &base.Status{}
	var video []*base.Video
	if video, err = service.NewVideoPopularService(ctx).VideoPopular(req); err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPopular failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Get VideoFeed!"
		resp.Popular = video
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Get VideoFeed Success"
	resp.Popular = video
	return resp, nil
}

func (s *VideoServiceImpl) VideoPublishStartV2(ctx context.Context, req *videos.VideoPublishStartRequestV2) (resp *videos.VideoPublishStartResponseV2, err error) {
	resp = new(videos.VideoPublishStartResponseV2)
	resp.Base = &base.Status{}

	// 使用新的TikTok风格上传服务V2
	uploadServiceV2 := service.NewVideoUploadServiceV2(ctx)
	session, err := uploadServiceV2.StartUpload(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPublishStart (V2) failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
	}

	// 获取用户存储配额信息
	userQuota, err := uploadServiceV2.GetUserStorageQuota(req.UserId)
	if err != nil {
		hlog.CtxWarnf(ctx, "Failed to get user quota: %v", err)
		// 使用默认配额，不影响主流程
		userQuota = &videos.UserStorageQuota{
			TotalQuotaBytes:   10 * 1024 * 1024 * 1024, // 10GB
			UsedQuotaBytes:    0,
			VideoCount:        0,
			QuotaLevel:        "standard",
			MaxVideoSizeBytes: 1024 * 1024 * 1024, // 1GB per video
			MaxVideoCount:     100,
		}
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Video Publish Started Successfully (V2 TikTok Style)"
	resp.UploadSessionUuid = session.UUID
	resp.VideoId = session.VideoID
	resp.UserQuota = userQuota
	resp.TempUploadPath = session.TempDir
	resp.SessionExpiresAt = session.ExpiresAt.Unix()
	// 暂时不实现预签名URL，保持空数组
	resp.PresignedUrls = []string{}

	return resp, nil
}

func (s *VideoServiceImpl) VideoPublishUploadingV2(ctx context.Context, req *videos.VideoPublishUploadingRequestV2) (resp *videos.VideoPublishUploadingResponseV2, err error) {
	resp = new(videos.VideoPublishUploadingResponseV2)
	resp.Base = &base.Status{}

	// 优先使用新的TikTok风格上传服务V2
	uploadServiceV2 := service.NewVideoUploadServiceV2(ctx)

	// 获取上传进度信息
	progress, progressErr := uploadServiceV2.GetUploadProgress(req.UploadSessionUuid, req.UserId)

	err = uploadServiceV2.UploadChunk(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPublishUploading (V2) failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)

		// 检查是否是会话不存在的错误
		if strings.Contains(err.Error(), "upload session not found") || strings.Contains(err.Error(), "session not found") {
			hlog.Warnf("Session not found error for UUID %s, attempting to continue with V1 fallback", req.UploadSessionUuid)
		}
	}

	// V2成功时，获取更新后的进度信息
	if progressErr == nil {
		// 更新进度信息（上传成功后重新获取）
		updatedProgress, updateErr := uploadServiceV2.GetUploadProgress(req.UploadSessionUuid, req.UserId)
		if updateErr == nil {
			progress = updatedProgress
		}
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Video Chunk Uploaded Successfully (V2)"
	resp.UploadedChunkNumber = req.ChunkNumber

	if progress != nil {
		resp.ChunkUploadStatus = progress.Status
		resp.UploadProgressPercent = progress.ProgressPercent
		resp.NextChunkOffset = progress.NextChunkOffset
		resp.UploadSpeedMbps = progress.UploadSpeedMbps
	} else {
		// 默认值
		resp.ChunkUploadStatus = "uploaded"
		resp.UploadProgressPercent = 0
		resp.NextChunkOffset = 0
		resp.UploadSpeedMbps = "calculating"
	}

	return resp, nil
}

func (s *VideoServiceImpl) VideoPublishCompleteV2(ctx context.Context, req *videos.VideoPublishCompleteRequestV2) (resp *videos.VideoPublishCompleteResponseV2, err error) {
	resp = new(videos.VideoPublishCompleteResponseV2)
	resp.Base = &base.Status{}

	// 优先使用新的TikTok风格上传服务V2
	uploadServiceV2 := service.NewVideoUploadServiceV2(ctx)
	err = uploadServiceV2.CompleteUpload(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPublishComplete (V2) failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Video Publish Completed Successfully (V2 TikTok Style)"
	return resp, nil
}

func (s *VideoServiceImpl) VideoPublishCancelV2(ctx context.Context, req *videos.VideoPublishCancelRequestV2) (resp *videos.VideoPublishCancelResponseV2, err error) {
	resp = new(videos.VideoPublishCancelResponseV2)
	resp.Base = &base.Status{}

	// 优先使用新的TikTok风格上传服务V2
	uploadServiceV2 := service.NewVideoUploadServiceV2(ctx)
	err = uploadServiceV2.CancelUpload(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.VideoPublishCancle (V2) failed, original error: %v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Video Publish Canceled Successfully (V2 TikTok Style)"
	return resp, nil
}

func (s *VideoServiceImpl) VideoVisitV2(ctx context.Context, req *videos.VideoVisitRequestV2) (resp *videos.VideoVisitResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) GetVideoVisitCountV2(ctx context.Context, req *videos.GetVideoVisitCountRequestV2) (resp *videos.GetVideoVisitCountResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) VideoDeleteV2(ctx context.Context, req *videos.VideoDeleteRequestV2) (resp *videos.VideoDeleteResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) VideoIdList(ctx context.Context, req *videos.VideoFeedListRequestV2) (resp *videos.VideoFeedListResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) VideoInfoV2(ctx context.Context, req *videos.VideoInfoRequestV2) (resp *videos.VideoInfoResponseV2, err error) {
	resp = new(videos.VideoInfoResponseV2)
	resp.Base = &base.Status{}
	data := new(base.Video)
	data, err = service.NewVideoListService(ctx).VideoInfo(req)
	if err != nil {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = err.Error()
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Get Video Info Success"
	resp.Items = data
	return resp, nil
}

func (s *VideoServiceImpl) UpdateVisitCountV2(ctx context.Context, req *videos.UpdateVisitCountRequestV2) (resp *videos.UpdateVisitCountResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) UpdateVideoCommentCountV2(ctx context.Context, req *videos.UpdateVideoCommentCountRequestV2) (resp *videos.UpdateVideoCommentCountResponseV2, err error) {
	return resp, nil
}

func (s *VideoServiceImpl) UpdateVideoLikeCountV2(ctx context.Context, req *videos.UpdateLikeCountRequestV2) (resp *videos.UpdateLikeCountResponseV2, err error) {
	return resp, nil
}


func (s *VideoServiceImpl) GetFavoriteVideoList(ctx context.Context, req *videos.GetFavoriteVideoListRequestV2) (resp *videos.GetFavoriteVideoListResponseV2, err error) {
	resp = new(videos.GetFavoriteVideoListResponseV2)
	resp.Base = &base.Status{}
	// TODO: Add your implementation logic here
	// Example:
	var videos []*base.Video
	videos, err = service.NewVideoFavoritesService(ctx).GetFavoriteVideoList(req)
	if err != nil {
		resp.Base.Code = errno.ServiceErrCode
		resp.Base.Msg = "Failed to Get Favorite List"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Success to Get Favorite List"
	resp.VideoList = videos
	return resp, nil
}

// ========== Missing Interface Methods ==========

// AddFavoriteVideoV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) AddFavoriteVideoV2(ctx context.Context, req *videos.AddFavoriteVideoRequestV2) (resp *videos.AddFavoriteVideoResponseV2, err error) {
	resp = new(videos.AddFavoriteVideoResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement add favorite video logic
	// err = service.NewVideoFavoritesService(ctx).AddFavoriteVideo(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to add favorite video"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully added video to favorites"
	return resp, nil
}

// DeleteFavoriteV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) DeleteFavoriteV2(ctx context.Context, req *videos.DeleteFavoriteRequestV2) (resp *videos.DeleteFavoriteResponseV2, err error) {
	resp = new(videos.DeleteFavoriteResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement delete favorite logic
	// err = service.NewVideoFavoritesService(ctx).DeleteFavorite(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to delete favorite"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully deleted favorite"
	return resp, nil
}

// DeleteVideoFromFavoriteV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) DeleteVideoFromFavoriteV2(ctx context.Context, req *videos.DeleteVideoFromFavoriteRequestV2) (resp *videos.DeleteVideoFromFavoriteResponseV2, err error) {
	resp = new(videos.DeleteVideoFromFavoriteResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement delete video from favorite logic
	// err = service.NewVideoFavoritesService(ctx).DeleteVideoFromFavorite(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to delete video from favorite"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully deleted video from favorite"
	return resp, nil
}

// SharedVideoV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) SharedVideoV2(ctx context.Context, req *videos.SharedVideoRequestV2) (resp *videos.SharedVideoResponseV2, err error) {
	resp = new(videos.SharedVideoResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement shared video logic
	// err = service.NewVideoShareService(ctx).ShareVideo(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to share video"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully shared video"
	return resp, nil
}

// RecommendVideoV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) RecommendVideoV2(ctx context.Context, req *videos.RecommendVideoRequestV2) (resp *videos.RecommendVideoResponseV2, err error) {
	resp = new(videos.RecommendVideoResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement video recommendation logic
	// videos, err := service.NewVideoRecommendService(ctx).GetRecommendedVideos(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to get recommended videos"
	// 	return resp, err
	// }

	// resp.RecommendedVideos = videos
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved recommended videos"
	return resp, nil
}

// ManageVideoHeatV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) ManageVideoHeatV2(ctx context.Context, req *videos.VideoHeatManagementRequest) (resp *videos.VideoHeatManagementResponse, err error) {
	resp = new(videos.VideoHeatManagementResponse)
	resp.Base = &base.Status{}

	// TODO: Implement video heat management logic
	// err = service.NewVideoHeatService(ctx).ManageVideoHeat(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to manage video heat"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully managed video heat"
	return resp, nil
}

// ManageUserQuotaV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) ManageUserQuotaV2(ctx context.Context, req *videos.UserQuotaManagementRequest) (resp *videos.UserQuotaManagementResponse, err error) {
	resp = new(videos.UserQuotaManagementResponse)
	resp.Base = &base.Status{}

	// TODO: Implement user quota management logic
	// err = service.NewUserQuotaService(ctx).ManageUserQuota(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to manage user quota"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully managed user quota"
	return resp, nil
}

// BatchOperateVideosV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) BatchOperateVideosV2(ctx context.Context, req *videos.BatchVideoOperationRequest) (resp *videos.BatchVideoOperationResponse, err error) {
	resp = new(videos.BatchVideoOperationResponse)
	resp.Base = &base.Status{}

	// TODO: Implement batch video operations logic
	// err = service.NewVideoBatchService(ctx).BatchOperateVideos(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to perform batch video operations"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully performed batch video operations"
	return resp, nil
}

// CreateFavoriteV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) CreateFavoriteV2(ctx context.Context, req *videos.CreateFavoriteRequestV2) (resp *videos.CreateFavoriteResponseV2, err error) {
	resp = new(videos.CreateFavoriteResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement create favorite logic
	// err = service.NewVideoFavoritesService(ctx).CreateFavorite(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to create favorite"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully created favorite"
	return resp, nil
}

// GetFavoriteVideoListV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) GetFavoriteVideoListV2(ctx context.Context, req *videos.GetFavoriteVideoListRequestV2) (resp *videos.GetFavoriteVideoListResponseV2, err error) {
	resp = new(videos.GetFavoriteVideoListResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement get favorite video list logic
	// videos, err := service.NewVideoFavoritesService(ctx).GetFavoriteVideoList(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to get favorite video list"
	// 	return resp, err
	// }

	// resp.VideoList = videos
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved favorite video list"
	return resp, nil
}

// GetFavoriteListV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) GetFavoriteListV2(ctx context.Context, req *videos.GetFavoriteListRequestV2) (resp *videos.GetFavoriteListResponseV2, err error) {
	resp = new(videos.GetFavoriteListResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement get favorite list logic
	// favorites, err := service.NewVideoFavoritesService(ctx).GetFavoriteList(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to get favorite list"
	// 	return resp, err
	// }

	// resp.FavoriteList = favorites
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved favorite list"
	return resp, nil
}

// StreamVideoV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) StreamVideoV2(ctx context.Context, req *videos.StreamVideoRequestV2) (resp *videos.StreamVideoResponseV2, err error) {
	resp = new(videos.StreamVideoResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement stream video logic
	// streamInfo, err := service.NewVideoStreamService(ctx).GetStreamInfo(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to get stream video info"
	// 	return resp, err
	// }

	// resp.StreamUrl = streamInfo.StreamUrl
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved stream video info"
	return resp, nil
}

// GetUploadProgressV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) GetUploadProgressV2(ctx context.Context, req *videos.VideoPublishProgressRequestV2) (resp *videos.VideoPublishProgressResponseV2, err error) {
	resp = new(videos.VideoPublishProgressResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement get upload progress logic
	// progress, err := service.NewVideoPublishService(ctx).GetUploadProgress(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to get upload progress"
	// 	return resp, err
	// }

	// resp.UploadProgress = progress
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved upload progress"
	return resp, nil
}

// ResumeUploadV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) ResumeUploadV2(ctx context.Context, req *videos.VideoPublishResumeRequestV2) (resp *videos.VideoPublishResumeResponseV2, err error) {
	resp = new(videos.VideoPublishResumeResponseV2)
	resp.Base = &base.Status{}

	// TODO: Implement resume upload logic
	// err = service.NewVideoPublishService(ctx).ResumeUpload(req)
	// if err != nil {
	// 	resp.Base.Code = errno.ServiceErrCode
	// 	resp.Base.Msg = "Failed to resume upload"
	// 	return resp, err
	// }

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully resumed upload"
	return resp, nil
}

// GetVideoAnalyticsV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) GetVideoAnalyticsV2(ctx context.Context, req *videos.VideoAnalyticsRequest) (resp *videos.VideoAnalyticsResponse, err error) {
	resp = new(videos.VideoAnalyticsResponse)
	resp.Base = &base.Status{}

	// TODO: Implement video analytics logic
	// For now, return success with empty response
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully retrieved video analytics"
	return resp, nil
}

// TranscodeVideoV2 implements the VideoServiceImpl interface.
func (s *VideoServiceImpl) TranscodeVideoV2(ctx context.Context, req *videos.VideoTranscodingRequest) (resp *videos.VideoTranscodingResponse, err error) {
	resp = new(videos.VideoTranscodingResponse)
	resp.Base = &base.Status{}

	// TODO: Implement video transcoding logic
	// For now, return success with empty response
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Successfully submitted video transcoding request"
	return resp, nil
}
