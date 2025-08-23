package client

import (
	"context"
	"time"

	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/kitex_gen/videos/videoservice"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/retry"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	etcd "github.com/kitex-contrib/registry-etcd"
)

var VideoClient videoservice.Client

func InitVideoRpc() {
	r, err := etcd.NewEtcdResolver([]string{"localhost:2379"})
	if err != nil {
		hlog.Info(err)
	}

	c, err := videoservice.NewClient(
		"Video",
		/* 		client.WithMiddleware(middleware.CommonMiddleware),
		   		client.WithInstanceMW(middleware.ClientMiddleware), */
		client.WithMuxConnection(1),                       // mux
		client.WithRPCTimeout(30*time.Second),             // rpc timeout
		client.WithConnectTimeout(50*time.Second),         // conn timeout
		client.WithFailureRetry(retry.NewFailurePolicy()), // retry
		//client.WithSuite(trace.NewDefaultClientSuite()),   // tracer
		client.WithResolver(r), // resolver
		client.WithClientBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "Interaction"}),
	)
	if err != nil {
		hlog.Info(err)
	}
	VideoClient = c
}

func VideoIdList(ctx context.Context, req *videos.VideoFeedListRequestV2) (resp *videos.VideoFeedListResponseV2, err error) {
	resp, err = VideoClient.VideoFeedListV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

func GetVideoVisitCountInRedis(ctx context.Context, req *videos.GetVideoVisitCountRequestV2) (resp *videos.GetVideoVisitCountResponseV2, err error) {
	resp, err = VideoClient.GetVideoVisitCountV2(ctx, req)
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

func UpdateVisitCount(ctx context.Context, req *videos.UpdateVisitCountRequestV2) (resp *videos.UpdateVisitCountResponseV2, err error) {
	resp, err = VideoClient.UpdateVisitCountV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}

// NOTE: UpdateVideoHisLikeCount was removed in V2 API
// func UpdateVideoHisLikeCount(ctx context.Context, req *videos.UpdateVideoHisLikeCountRequest) (resp *videos.UpdateVideoHisLikeCountResponse, err error) {
// 	resp, err = VideoClient.UpdateVideoHisLikeCount(ctx, req)
// 	if err != nil {
// 		return resp, nil
// 	}
// 	return resp, err
// }

func VideoInfo(ctx context.Context, req *videos.VideoInfoRequestV2) (resp *videos.VideoInfoResponseV2, err error) {
	resp, err = VideoClient.VideoInfoV2(ctx, req)
	if err != nil {
		return resp, nil
	}
	return resp, err
}
