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

	// 接入推荐系统：更新视频曝光/点击、用户画像
	OnVideoViewed(s.ctx, req.VideoId, req.FromId)

	// Get related videos by same author or category
	relatedVideos, err := getRelatedVideos(s.ctx, video)
	if err == nil && len(relatedVideos) > 0 {
		resp.RelatedVideos = relatedVideos
	}

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

// getRelatedVideos fetches related videos by same author or hot videos as fallback
func getRelatedVideos(ctx context.Context, video *base.Video) ([]*base.Video, error) {
	const relatedLimit = 10

	// 1. Get videos from the same author (exclude current video)
	authorVideos, _, err := db.GetUserVideoList(ctx, video.UserId, 1, relatedLimit+1)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get author videos")
	}

	var related []*base.Video
	for _, v := range authorVideos {
		if v.VideoId != video.VideoId {
			related = append(related, v)
		}
		if len(related) >= relatedLimit {
			break
		}
	}

	// 2. If not enough, supplement with hot videos
	if len(related) < relatedLimit {
		remaining := relatedLimit - len(related)
		hotVideos, err := db.GetHotVideosByLikes(ctx, remaining+5)
		if err == nil {
			existingIds := make(map[int64]bool)
			existingIds[video.VideoId] = true
			for _, v := range related {
				existingIds[v.VideoId] = true
			}
			for _, hv := range hotVideos {
				if existingIds[hv.VideoId] {
					continue
				}
				related = append(related, &base.Video{
					VideoId:    hv.VideoId,
					Title:      hv.Title,
					VideoUrl:   hv.VideoUrl,
					CoverUrl:   hv.CoverUrl,
					LikesCount: int64(hv.LikesCount),
					VisitCount: int64(hv.VisitCount),
					UserId:     hv.UserId,
				})
				if len(related) >= relatedLimit {
					break
				}
			}
		}
	}

	return related, nil
}
