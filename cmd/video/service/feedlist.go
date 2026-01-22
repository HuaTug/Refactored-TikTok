package service

import (
	"context"
	"strings"

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

	// Fix cover URLs: replace fake thumbnail URLs with video URLs
	for _, video := range res {
		video.CoverUrl = fixCoverUrl(video.CoverUrl, video.VideoUrl)
	}

	VideoFiles = res
	return res, nil
}

// fixCoverUrl fixes cover URLs that point to non-existent thumbnail files
// If cover_url ends with _thumb.jpg or other fake suffixes, replace it with video URL
func fixCoverUrl(coverUrl, videoUrl string) string {
	if coverUrl == "" {
		return videoUrl
	}

	// Check if cover URL is a fake thumbnail path (ends with _thumb.jpg, _animated.gif, etc.)
	fakeSuffixes := []string{"_thumb.jpg", "_animated.gif", "_metadata.json"}
	for _, suffix := range fakeSuffixes {
		if strings.HasSuffix(coverUrl, suffix) {
			// Return video URL as fallback cover
			return videoUrl
		}
	}

	return coverUrl
}
