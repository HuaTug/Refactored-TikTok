package service

import (
	"context"
	"sync"
	"time"

	"HuaTug.com/cmd/video/dal/db"
	redis "HuaTug.com/cmd/video/cache"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

var (
	syncOnce     sync.Once
	syncStopChan chan struct{}
)

// StartVisitCountSyncTask 启动浏览量同步任务
func StartVisitCountSyncTask() {
	syncOnce.Do(func() {
		syncStopChan = make(chan struct{})
		go runSyncTask()
		hlog.Info("Visit count sync task started")
	})
}

// StopVisitCountSyncTask 停止浏览量同步任务
func StopVisitCountSyncTask() {
	if syncStopChan != nil {
		close(syncStopChan)
	}
}

func runSyncTask() {
	// 每5分钟同步一次
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			syncVisitCounts()
		case <-syncStopChan:
			hlog.Info("Visit count sync task stopped")
			return
		}
	}
}

func syncVisitCounts() {
	ctx := context.Background()
	
	// 获取待同步的视频ID列表
	videoIds, err := redis.GetPendingSyncVideoIds(ctx)
	if err != nil {
		hlog.Errorf("Failed to get pending sync video ids: %v", err)
		return
	}

	if len(videoIds) == 0 {
		return
	}

	hlog.Infof("Starting to sync visit counts for %d videos", len(videoIds))
	
	successCount := 0
	failCount := 0

	for _, videoId := range videoIds {
		// 从Redis获取最新浏览量
		count, found, err := redis.GetVideoVisitCountCached(ctx, videoId)
		if err != nil || !found {
			continue
		}

		// 更新数据库
		if err := db.UpdateVideoVisit(ctx, videoId, count); err != nil {
			hlog.Warnf("Failed to sync visit count for video %d: %v", videoId, err)
			failCount++
			continue
		}

		// 从待同步队列中移除
		if err := redis.ClearSyncedVideoId(ctx, videoId); err != nil {
			hlog.Warnf("Failed to clear synced video id %d: %v", videoId, err)
		}

		successCount++
	}

	hlog.Infof("Visit count sync completed: %d success, %d failed", successCount, failCount)
}

// SyncSingleVideoVisitCount 同步单个视频的浏览量（用于实时性要求高的场景）
func SyncSingleVideoVisitCount(ctx context.Context, videoId int64) error {
	count, found, err := redis.GetVideoVisitCountCached(ctx, videoId)
	if err != nil || !found {
		return err
	}

	return db.UpdateVideoVisit(ctx, videoId, count)
}
