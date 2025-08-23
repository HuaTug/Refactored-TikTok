package rpc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/config/jaeger"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/kitex_gen/videos/videoservice"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/retry"
	etcd "github.com/kitex-contrib/registry-etcd"
)

var VideoClient videoservice.Client

func InitVideoRpc() {
	config.Init()
	r, err := etcd.NewEtcdResolver([]string{config.ConfigInfo.Etcd.Addr})
	if err != nil {
		hlog.Info(err)
	}
	suite, closer := jaeger.NewClientTracer().Init("Video")
	defer closer.Close()
	c, err := videoservice.NewClient(
		"Video",
		client.WithMuxConnection(3),                       // mux
		client.WithRPCTimeout(60*time.Second),             // rpc timeout increased for video processing
		client.WithConnectTimeout(50*time.Second),         // conn timeout
		client.WithFailureRetry(retry.NewFailurePolicy()), // retry
		client.WithResolver(r),                            // resolver
		client.WithSuite(suite),
	)
	if err != nil {
		hlog.Info(err)
	}
	VideoClient = c
}

func FeedList(ctx context.Context, req *videos.VideoFeedListRequestV2) (resp *videos.VideoFeedListResponseV2, err error) {
	resp, err = VideoClient.VideoFeedListV2(ctx, req)
	if resp == nil {
		resp = &videos.VideoFeedListResponseV2{}
	}
	if resp.Base == nil {
		resp.Base = &base.Status{}
	}
	if err != nil {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to provide FeedService!"
		return resp, err
	}
	return resp, nil
}

func VideoFeedList(ctx context.Context, req *videos.VideoFeedListRequestV2) (resp *videos.VideoFeedListResponseV2, err error) {
	resp, err = VideoClient.VideoFeedListV2(ctx, req)
	if resp == nil {
		resp = &videos.VideoFeedListResponseV2{}
	}
	if resp.Base == nil {
		resp.Base = &base.Status{}
	}
	if err != nil {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to provide VideoFeedList Service!"
		return resp, err
	}
	return resp, err
}

func VideoSearch(ctx context.Context, req *videos.VideoSearchRequestV2) (resp *videos.VideoSearchResponseV2, err error) {
	resp, err = VideoClient.VideoSearchV2(ctx, req)
	if resp == nil {
		resp = &videos.VideoSearchResponseV2{}
	}
	if resp.Base == nil {
		resp.Base = &base.Status{}
	}
	if err != nil {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to provide VideoSearch Service!"
		return resp, err
	}
	return resp, err
}

func VideoVisit(ctx context.Context, req *videos.VideoVisitRequestV2) (resp *videos.VideoVisitResponseV2, err error) {
	resp, err = VideoClient.VideoVisitV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func VideoPopular(ctx context.Context, req *videos.VideoPopularRequestV2) (resp *videos.VideoPopularResponseV2, err error) {
	resp, err = VideoClient.VideoPopularV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func VideoDelete(ctx context.Context, req *videos.VideoDeleteRequestV2) (resp *videos.VideoDeleteResponseV2, err error) {
	resp, err = VideoClient.VideoDeleteV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func VideoInfo(ctx context.Context, req *videos.VideoInfoRequestV2) (resp *videos.VideoInfoResponseV2, err error) {
	resp, err = VideoClient.VideoInfoV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func VideoIdList(ctx context.Context, pageNum, pageSize int64) (resp *videos.VideoFeedListResponseV2, err error) {
	req := &videos.VideoFeedListRequestV2{
		PageNum:  pageNum,
		PageSize: pageSize,
	}
	resp, err = VideoClient.VideoFeedListV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func UpdateVideoVisitCount(ctx context.Context, req *videos.UpdateVisitCountRequestV2) (resp *videos.UpdateVisitCountResponseV2, err error) {
	resp, err = VideoClient.UpdateVisitCountV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func UpdateVideoCommentCount(ctx context.Context, req *videos.UpdateVideoCommentCountRequestV2) (resp *videos.UpdateVideoCommentCountResponseV2, err error) {
	resp, err = VideoClient.UpdateVideoCommentCountV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func UpdateVideoLikeCount(ctx context.Context, req *videos.UpdateLikeCountRequestV2) (resp *videos.UpdateLikeCountResponseV2, err error) {
	resp, err = VideoClient.UpdateVideoLikeCountV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func UpdateVideoHisLikeCount(ctx context.Context, videoId, hisLikeCount int64) error {
	// V2版本中这个功能可能被合并到其他接口中，这里暂时保留一个简化版本
	req := &videos.UpdateLikeCountRequestV2{
		VideoId:   videoId,
		LikeCount: hisLikeCount,
	}
	_, err := VideoClient.UpdateVideoLikeCountV2(ctx, req)
	return err
}

func GetVideoVisitCount(ctx context.Context, req *videos.GetVideoVisitCountRequestV2) (resp *videos.GetVideoVisitCountResponseV2, err error) {
	resp, err = VideoClient.GetVideoVisitCountV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func GetVideoVisitCountInRedis(ctx context.Context, videoId string) (int64, error) {
	// 将string类型的videoId转换为int64
	videoIdInt, err := strconv.ParseInt(videoId, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid video ID format: %v", err)
	}

	req := &videos.GetVideoVisitCountRequestV2{
		VideoId:   videoIdInt,
		CountType: "redis",
	}
	resp, err := VideoClient.GetVideoVisitCountV2(ctx, req)
	if err != nil {
		return 0, err
	}
	return resp.VisitCount, nil
}

func VideoStream(ctx context.Context, req *videos.StreamVideoRequestV2) (resp *videos.StreamVideoResponseV2, err error) {
	resp, err = VideoClient.StreamVideoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func CreateFavorite(ctx context.Context, req *videos.CreateFavoriteRequestV2) (resp *videos.CreateFavoriteResponseV2, err error) {
	resp, err = VideoClient.CreateFavoriteV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func GetFavoroteList(ctx context.Context, req *videos.GetFavoriteListRequestV2) (resp *videos.GetFavoriteListResponseV2, err error) {
	resp, err = VideoClient.GetFavoriteListV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func GetFavoriteVideoList(ctx context.Context, req *videos.GetFavoriteVideoListRequestV2) (resp *videos.GetFavoriteVideoListResponseV2, err error) {
	resp, err = VideoClient.GetFavoriteVideoListV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func GetVideoFromFavorite(ctx context.Context, userId, videoId int64) (*videos.VideoInfoResponseV2, error) {
	req := &videos.VideoInfoRequestV2{
		VideoId:          videoId,
		RequestingUserId: userId,
		IncludeAnalytics: false,
	}
	resp, err := VideoClient.VideoInfoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func AddFavoriteVideo(ctx context.Context, req *videos.AddFavoriteVideoRequestV2) (resp *videos.AddFavoriteVideoResponseV2, err error) {
	resp, err = VideoClient.AddFavoriteVideoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func SharedVideo(ctx context.Context, req *videos.SharedVideoRequestV2) (resp *videos.SharedVideoResponseV2, err error) {
	resp, err = VideoClient.SharedVideoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func DeleteFavorite(ctx context.Context, req *videos.DeleteFavoriteRequestV2) (resp *videos.DeleteFavoriteResponseV2, err error) {
	resp, err = VideoClient.DeleteFavoriteV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func DeleteVideoFromFavortie(ctx context.Context, req *videos.DeleteVideoFromFavoriteRequestV2) (resp *videos.DeleteVideoFromFavoriteResponseV2, err error) {
	resp, err = VideoClient.DeleteVideoFromFavoriteV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func RecommendVideo(ctx context.Context, req *videos.RecommendVideoRequestV2) (resp *videos.RecommendVideoResponseV2, err error) {
	resp, err = VideoClient.RecommendVideoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

// ========== V2版本RPC方法 ==========

func VideoPublishStartV2(ctx context.Context, req *videos.VideoPublishStartRequestV2) (resp *videos.VideoPublishStartResponseV2, err error) {
	resp = new(videos.VideoPublishStartResponseV2)
	resp, err = VideoClient.VideoPublishStartV2(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to start video publish V2: %v", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("received nil response from VideoPublishStartV2")
	}
	return resp, err
}

func VideoPublishUploadingV2(ctx context.Context, req *videos.VideoPublishUploadingRequestV2) (resp *videos.VideoPublishUploadingResponseV2, err error) {
	resp = new(videos.VideoPublishUploadingResponseV2)
	resp, err = VideoClient.VideoPublishUploadingV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func VideoPublishCompleteV2(ctx context.Context, req *videos.VideoPublishCompleteRequestV2) (resp *videos.VideoPublishCompleteResponseV2, err error) {
	resp, err = VideoClient.VideoPublishCompleteV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func VideoPublishCancelV2(ctx context.Context, req *videos.VideoPublishCancelRequestV2) (resp *videos.VideoPublishCancelResponseV2, err error) {
	resp, err = VideoClient.VideoPublishCancelV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func GetUploadProgressV2(ctx context.Context, req *videos.VideoPublishProgressRequestV2) (resp *videos.VideoPublishProgressResponseV2, err error) {
	resp, err = VideoClient.GetUploadProgressV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func ResumeUploadV2(ctx context.Context, req *videos.VideoPublishResumeRequestV2) (resp *videos.VideoPublishResumeResponseV2, err error) {
	resp, err = VideoClient.ResumeUploadV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func ManageVideoHeatV2(ctx context.Context, req *videos.VideoHeatManagementRequest) (resp *videos.VideoHeatManagementResponse, err error) {
	resp, err = VideoClient.ManageVideoHeatV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func ManageUserQuotaV2(ctx context.Context, req *videos.UserQuotaManagementRequest) (resp *videos.UserQuotaManagementResponse, err error) {
	resp, err = VideoClient.ManageUserQuotaV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func BatchOperateVideosV2(ctx context.Context, req *videos.BatchVideoOperationRequest) (resp *videos.BatchVideoOperationResponse, err error) {
	resp, err = VideoClient.BatchOperateVideosV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func TranscodeVideoV2(ctx context.Context, req *videos.VideoTranscodingRequest) (resp *videos.VideoTranscodingResponse, err error) {
	resp, err = VideoClient.TranscodeVideoV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}

func GetVideoAnalyticsV2(ctx context.Context, req *videos.VideoAnalyticsRequest) (resp *videos.VideoAnalyticsResponse, err error) {
	resp, err = VideoClient.GetVideoAnalyticsV2(ctx, req)
	if err != nil {
		return resp, err
	}
	return resp, err
}
