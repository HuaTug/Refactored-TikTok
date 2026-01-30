package handlers

import (
	"context"
	"net/http"
	"strconv"

	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// HotRankingHandler 热度排行榜处理器
type HotRankingHandler struct{}

// NewHotRankingHandler 创建热度排行处理器
func NewHotRankingHandler() *HotRankingHandler {
	return &HotRankingHandler{}
}

// GetHotRanking 获取热门视频排行榜
// GET /api/video/hot/ranking?time_window=24h&limit=50
func (h *HotRankingHandler) GetHotRanking(ctx context.Context, c *app.RequestContext) {
	// 获取参数
	timeWindow := c.DefaultQuery("time_window", "24h")
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}

	// 验证时间窗口
	validWindows := map[string]bool{"1h": true, "6h": true, "24h": true, "7d": true}
	if !validWindows[timeWindow] {
		timeWindow = "24h"
	}

	// 获取热度服务
	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	// 获取热门视频ID列表
	videoIds, err := service.GetTopHotVideos(ctx, timeWindow, limit)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to get hot videos: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to get hot ranking",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"time_window": timeWindow,
			"video_ids":   videoIds,
			"count":       len(videoIds),
		},
	})
}

// GetTrendingVideos 获取趋势视频（上升最快）
// GET /api/video/hot/trending?limit=20
func (h *HotRankingHandler) GetTrendingVideos(ctx context.Context, c *app.RequestContext) {
	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		limit = 20
	}

	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	trends, err := service.GetTrendingVideos(ctx, limit)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to get trending videos: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to get trending videos",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"trends": trends,
			"count":  len(trends),
		},
	})
}

// GetVideoHotRank 获取视频热度排名
// GET /api/video/hot/rank/:video_id?time_window=24h
func (h *HotRankingHandler) GetVideoHotRank(ctx context.Context, c *app.RequestContext) {
	videoIdStr := c.Param("video_id")
	videoId, err := strconv.ParseInt(videoIdStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"code":    400,
			"message": "Invalid video_id",
		})
		return
	}

	timeWindow := c.DefaultQuery("time_window", "24h")

	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	rank, score, err := service.GetVideoHotRank(ctx, videoId, timeWindow)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to get video rank: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to get video rank",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"video_id":    videoId,
			"time_window": timeWindow,
			"rank":        rank,
			"hot_score":   score,
		},
	})
}

// GetCategoryHotStats 获取分类热度统计
// GET /api/video/hot/categories
func (h *HotRankingHandler) GetCategoryHotStats(ctx context.Context, c *app.RequestContext) {
	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	stats, err := service.GetCategoryHotStats(ctx)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to get category stats: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to get category hot stats",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"categories": stats,
			"count":      len(stats),
		},
	})
}

// RefreshVideoHotScore 刷新单个视频热度（管理员接口）
// POST /api/video/hot/refresh/:video_id
func (h *HotRankingHandler) RefreshVideoHotScore(ctx context.Context, c *app.RequestContext) {
	videoIdStr := c.Param("video_id")
	videoId, err := strconv.ParseInt(videoIdStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"code":    400,
			"message": "Invalid video_id",
		})
		return
	}

	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	err = service.CalculateVideoHotScore(ctx, videoId)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to refresh video hot score: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to refresh video hot score",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
		"data": map[string]interface{}{
			"video_id": videoId,
			"status":   "refreshed",
		},
	})
}

// RecordInteraction 记录视频互动（用于实时热度更新）
// POST /api/video/hot/interaction
// Body: {"video_id": 123, "action_type": "view|like|comment|share"}
func (h *HotRankingHandler) RecordInteraction(ctx context.Context, c *app.RequestContext) {
	var req struct {
		VideoID    int64  `json:"video_id"`
		ActionType string `json:"action_type"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"code":    400,
			"message": "Invalid request body",
		})
		return
	}

	// 验证动作类型
	validActions := map[string]bool{"view": true, "like": true, "comment": true, "share": true}
	if !validActions[req.ActionType] {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"code":    400,
			"message": "Invalid action_type",
		})
		return
	}

	service := recommendation.GetHotScoreService()
	if service == nil {
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Hot score service not available",
		})
		return
	}

	err := service.IncrementVideoInteraction(ctx, req.VideoID, req.ActionType)
	if err != nil {
		hlog.Errorf("[HotRankingHandler] Failed to record interaction: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"code":    500,
			"message": "Failed to record interaction",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": "success",
	})
}
