package service

import (
	"context"
	"strconv"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/pkg/infra/cache"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type VideoPopularService struct {
	ctx context.Context
}

func NewVideoPopularService(ctx context.Context) *VideoPopularService {
	return &VideoPopularService{ctx: ctx}
}

func (v *VideoPopularService) VideoPopular(req *videos.VideoPopularRequestV2) (video []*base.Video, err error) {
	// 设置默认分页参数
	limit := int(req.PageSize)
	if limit <= 0 || limit > 10 {
		limit = 10 // 热门榜最多10条
	}

	// 1. 先尝试从Redis缓存获取
	res, err := cache.RangeList("Rank")
	if err == nil && len(res) > 0 {
		// 缓存命中，从缓存构建结果
		for i := 0; i < len(res) && i < limit; i++ {
			s, err := strconv.Atoi(res[i])
			if err != nil {
				hlog.Warnf("Convert failed for rank item %s: %v", res[i], err)
				continue
			}
			temp, err := db.FindVideo(v.ctx, int64(s))
			if err != nil {
				hlog.Warnf("FindVideo failed for video %d: %v", s, err)
				continue
			}
			video = append(video, temp)
		}
		if len(video) > 0 {
			return video, nil
		}
	}

	// 2. 缓存为空或失败，从数据库按点赞数获取热门视频
	hlog.Info("Redis rank cache miss, fetching hot videos from database")
	hotVideos, err := db.GetHotVideosByLikes(v.ctx, limit)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get hot videos from database")
	}

	// 3. 构建返回结果并更新缓存
	for _, dbVideo := range hotVideos {
		videoItem := &base.Video{
			VideoId:    dbVideo.VideoId,
			Title:      dbVideo.Title,
			VideoUrl:   dbVideo.VideoUrl,
			CoverUrl:   dbVideo.CoverUrl,
			LikesCount: int64(dbVideo.LikesCount),
			VisitCount: int64(dbVideo.VisitCount),
			UserId:     dbVideo.UserId,
			CreatedAt:  dbVideo.CreatedAt.Format(time.RFC3339),
		}
		video = append(video, videoItem)

		// 同步更新Redis缓存（以点赞数为分数）
		if err := cache.RangeAdd(int64(dbVideo.LikesCount), dbVideo.VideoId); err != nil {
			hlog.Warnf("Failed to update rank cache for video %d: %v", dbVideo.VideoId, err)
		}
	}

	return video, nil
}
