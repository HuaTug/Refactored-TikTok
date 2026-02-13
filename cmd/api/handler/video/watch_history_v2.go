package handlers

import (
	"context"
	"strconv"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// GetWatchHistoryV2 handles get watch history requests
func GetWatchHistoryV2(ctx context.Context, c *app.RequestContext) {
	var req videos.GetWatchHistoryRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取分页参数
	pageNum, _ := strconv.ParseInt(c.DefaultQuery("page_num", "1"), 10, 64)
	pageSize, _ := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 64)
	req.PageNum = pageNum
	req.PageSize = pageSize

	// 获取日期过滤器
	req.DateFilter = c.DefaultQuery("date_filter", "all")

	resp, err := rpc.GetWatchHistoryV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.GetWatchHistoryV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// AddWatchHistoryV2 handles add watch history requests
func AddWatchHistoryV2(ctx context.Context, c *app.RequestContext) {
	var req videos.AddWatchHistoryRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 绑定请求参数
	if err := c.BindJSON(&req); err != nil {
		hlog.Error("AddWatchHistoryV2 BindJSON error:", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	if req.VideoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	resp, err := rpc.AddWatchHistoryV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.AddWatchHistoryV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// ClearWatchHistoryV2 handles clear watch history requests
func ClearWatchHistoryV2(ctx context.Context, c *app.RequestContext) {
	var req videos.ClearWatchHistoryRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取日期范围参数
	req.DateRange = c.DefaultQuery("date_range", "all")

	resp, err := rpc.ClearWatchHistoryV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.ClearWatchHistoryV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// DeleteWatchHistoryItemV2 handles delete watch history item requests
func DeleteWatchHistoryItemV2(ctx context.Context, c *app.RequestContext) {
	var req videos.DeleteWatchHistoryItemRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取视频ID
	videoIdStr := c.Query("video_id")
	videoId, err := strconv.ParseInt(videoIdStr, 10, 64)
	if err != nil || videoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.VideoId = videoId

	resp, err := rpc.DeleteWatchHistoryItemV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.DeleteWatchHistoryItemV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}
