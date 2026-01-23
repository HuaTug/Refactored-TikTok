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

// VideoVisitV2 handles video visit requests
func VideoVisitV2(ctx context.Context, c *app.RequestContext) {
	var req videos.VideoVisitRequestV2

	// 获取当前用户ID（可能未登录）
	fromId, _ := c.Get("user_id")
	if uid, ok := fromId.(int64); ok {
		req.FromId = uid
	}

	// 获取视频ID
	if err := c.BindJSON(&req); err != nil {
		hlog.Error("VideoVisitV2 BindJSON error:", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	if req.VideoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 获取访问来源信息
	req.VisitSource = string(c.GetHeader("Referer"))
	if req.Context == nil {
		req.Context = make(map[string]string)
	}
	req.Context["user_agent"] = string(c.GetHeader("User-Agent"))
	req.Context["ip"] = c.ClientIP()

	resp, err := rpc.VideoVisitV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.VideoVisitV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// GetVideoVisitCountV2 handles get video visit count requests
func GetVideoVisitCountV2(ctx context.Context, c *app.RequestContext) {
	var req videos.GetVideoVisitCountRequestV2

	// 获取视频ID
	videoIdStr := c.Query("video_id")
	videoId, err := strconv.ParseInt(videoIdStr, 10, 64)
	if err != nil || videoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.VideoId = videoId

	// 获取计数类型（默认为visit_count）
	req.CountType = c.DefaultQuery("count_type", "visit_count")

	resp, err := rpc.GetVideoVisitCountV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.GetVideoVisitCountV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}
