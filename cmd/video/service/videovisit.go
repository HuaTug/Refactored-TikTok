package service

import (
	"context"
	"strconv"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/pkg/errors"
)

type VideoVisitService struct {
	ctx context.Context
}

func NewVideoVisitService(ctx context.Context) *VideoVisitService {
	return &VideoVisitService{
		ctx: ctx,
	}
}

// VideoVisit handles video visit and returns video info with related videos
func (s *VideoVisitService) VideoVisit(req *videos.VideoVisitRequestV2) (*videos.VideoVisitResponseV2, error) {
	resp := &videos.VideoVisitResponseV2{
		Base: &base.Status{},
	}

	// 获取视频信息
	video, err := db.GetVideoInfo(s.ctx, req.VideoId)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get video info")
	}
	if video == nil || video.VideoId == 0 {
		return nil, errors.New("video not found")
	}

	resp.Item = video

	// 记录浏览历史并增加浏览量（如果是登录用户）
	if req.FromId > 0 {
		// 添加观看历史（会自动增加浏览量）
		_, err := db.AddOrUpdateWatchHistory(s.ctx, req.FromId, req.VideoId, 0, 0)
		if err == nil {
			resp.ViewCounted = true
		}
	} else {
		// 匿名用户也增加浏览量
		if err := db.IncrementVisitCount(s.ctx, req.VideoId, 0); err == nil {
			resp.ViewCounted = true
		}
	}

	// TODO: 获取相关推荐视频（根据标签、分类等）
	// resp.RelatedVideos = getRelatedVideos(s.ctx, video)

	return resp, nil
}

// GetVideoVisitCount gets video visit count and detailed metrics
func (s *VideoVisitService) GetVideoVisitCount(req *videos.GetVideoVisitCountRequestV2) (*videos.GetVideoVisitCountResponseV2, error) {
	resp := &videos.GetVideoVisitCountResponseV2{
		Base:           &base.Status{},
		DetailedCounts: make(map[string]int64),
	}

	if req.CountType == "detailed" || req.CountType == "all" {
		// 获取详细计数
		counts, err := db.GetVideoDetailedCounts(s.ctx, req.VideoId)
		if err != nil {
			return nil, errors.WithMessage(err, "Failed to get video detailed counts")
		}
		resp.DetailedCounts = counts
		if visitCount, ok := counts["visit_count"]; ok {
			resp.VisitCount = visitCount
		}
	} else {
		// 仅获取浏览量
		count, err := db.GetVideoVisitCountById(s.ctx, req.VideoId)
		if err != nil {
			return nil, errors.WithMessage(err, "Failed to get video visit count")
		}
		resp.VisitCount = int64(count)
	}

	return resp, nil
}

// UpdateVisitCount updates video visit count (admin/internal use)
func (s *VideoVisitService) UpdateVisitCount(req *videos.UpdateVisitCountRequestV2) (int64, error) {
	// 更新浏览量
	if err := db.UpdateVideoVisit(s.ctx, req.VideoId, req.VisitCount); err != nil {
		return 0, errors.WithMessage(err, "Failed to update visit count")
	}

	// 获取更新后的总数
	count, err := db.GetVideoVisitCount(s.ctx, strconv.FormatInt(req.VideoId, 10))
	if err != nil {
		return req.VisitCount, nil
	}

	return count, nil
}
