package service

import (
	"context"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/videos"

	"github.com/pkg/errors"
)

type WatchHistoryService struct {
	ctx context.Context
}

func NewWatchHistoryService(ctx context.Context) *WatchHistoryService {
	return &WatchHistoryService{
		ctx: ctx,
	}
}

// GetWatchHistory gets user's watch history with video details
func (s *WatchHistoryService) GetWatchHistory(req *videos.GetWatchHistoryRequestV2) (*videos.GetWatchHistoryResponseV2, error) {
	resp := &videos.GetWatchHistoryResponseV2{
		Base:        &base.Status{},
		HistoryList: make([]*videos.WatchHistoryItem, 0),
	}

	history, videoList, total, err := db.GetWatchHistoryWithVideos(s.ctx, req.UserId, req.PageNum, req.PageSize, req.DateFilter)
	if err != nil {
		return nil, errors.WithMessage(err, "Failed to get watch history")
	}

	// 构建视频ID到视频信息的映射
	videoMap := make(map[int64]*base.Video)
	for _, v := range videoList {
		videoMap[v.VideoId] = v
	}

	// 构建响应
	for _, h := range history {
		item := &videos.WatchHistoryItem{
			HistoryId:      h.UserVideoWatchHistoryId,
			VideoId:        h.VideoId,
			UserId:         h.UserId,
			WatchDuration:  int32(h.WatchDuration),
			CompletionRate: h.CompletionRate,
			WatchTime:      h.WatchTime.Format(time.RFC3339),
		}
		if video, ok := videoMap[h.VideoId]; ok {
			item.VideoInfo = video
		}
		resp.HistoryList = append(resp.HistoryList, item)
	}

	resp.TotalCount = total
	resp.HasMore = int64(req.PageNum)*int64(req.PageSize) < total

	return resp, nil
}

// AddWatchHistory adds or updates watch history record
func (s *WatchHistoryService) AddWatchHistory(req *videos.AddWatchHistoryRequestV2) (bool, error) {
	isNew, err := db.AddOrUpdateWatchHistory(s.ctx, req.UserId, req.VideoId, uint(req.WatchDuration), req.CompletionRate)
	if err != nil {
		return false, errors.WithMessage(err, "Failed to add watch history")
	}
	// 推荐桥接：把带 progress 的观看行为推送给 RealtimeStateService（异步、不阻塞）
	if req.UserId > 0 {
		OnVideoViewedWithProgress(s.ctx, req.VideoId, req.UserId,
			float64(req.CompletionRate), int(req.WatchDuration))
	}
	return isNew, nil
}

// ClearWatchHistory clears user's watch history
func (s *WatchHistoryService) ClearWatchHistory(req *videos.ClearWatchHistoryRequestV2) (int64, error) {
	dateRange := req.DateRange
	if dateRange == "" {
		dateRange = "all"
	}

	count, err := db.ClearUserWatchHistoryByDate(s.ctx, req.UserId, dateRange)
	if err != nil {
		return 0, errors.WithMessage(err, "Failed to clear watch history")
	}
	return count, nil
}

// DeleteWatchHistoryItem deletes a specific watch history item
func (s *WatchHistoryService) DeleteWatchHistoryItem(req *videos.DeleteWatchHistoryItemRequestV2) error {
	if err := db.DeleteWatchHistoryItem(s.ctx, req.UserId, req.VideoId); err != nil {
		return errors.WithMessage(err, "Failed to delete watch history item")
	}
	return nil
}
