package service

import (
	"context"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
	"github.com/pkg/errors"
)

type VideoListService struct {
	ctx context.Context
}

func NewVideoListService(ctx context.Context) *VideoListService {
	return &VideoListService{ctx: ctx}
}

func (v *VideoListService) VideoList(req *videos.VideoFeedListRequestV2) (video []*base.Video, count int64, err error) {
	if video, count, err = db.Videolist(v.ctx, req); err != nil {
		return video, count, errors.WithMessage(err, "dao.VideoList failed")
	}

	// Fix cover URLs: replace fake thumbnail URLs with video URLs
	for _, vid := range video {
		vid.CoverUrl = fixCoverUrl(vid.CoverUrl, vid.VideoUrl)
	}

	return video, count, err
}

func (v *VideoListService) VideoInfo(req *videos.VideoInfoRequestV2) (data *base.Video, err error) {
	data, err = db.GetVideoInfo(v.ctx, req.VideoId)
	if err != nil {
		return nil, err
	}

	// Fix cover URL for single video info
	data.CoverUrl = fixCoverUrl(data.CoverUrl, data.VideoUrl)

	return data, nil
}
