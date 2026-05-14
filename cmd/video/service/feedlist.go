package service

import (
	"context"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"
)

type FeedListService struct {
	ctx context.Context
}

func NewFeedListService(ctx context.Context) *FeedListService {
	return &FeedListService{ctx: ctx}
}

// FeedList 视频流接口
func (v *FeedListService) FeedList(req *videos.VideoFeedListRequestV2) (res []*base.Video, err error) {
	// Convert V2 request to match database interface
	// Note: The LastTime field is not available in V2, need to handle this differently
	res, err = db.GetAllFeedList(v.ctx, req)
	if err != nil {
		return nil, err
	}

	VideoFiles = res
	return res, nil
}
