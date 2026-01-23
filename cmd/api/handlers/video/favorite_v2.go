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

// CreateFavoriteV2 handles create favorite requests (V2)
func CreateFavoriteV2(ctx context.Context, c *app.RequestContext) {
	var req videos.CreateFavoriteRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 绑定请求参数
	if err := c.BindJSON(&req); err != nil {
		hlog.Error("CreateFavoriteV2 BindJSON error:", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	if req.Name == "" {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	resp, err := rpc.CreateFavoriteV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.CreateFavoriteV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// GetFavoriteListV2 handles get favorite list requests (V2)
func GetFavoriteListV2(ctx context.Context, c *app.RequestContext) {
	var req videos.GetFavoriteListRequestV2

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

	// 获取隐私过滤器
	req.PrivacyFilter = c.Query("privacy_filter")

	resp, err := rpc.GetFavoriteListV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.GetFavoriteListV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// GetFavoriteVideoListV2 handles get favorite video list requests (V2)
func GetFavoriteVideoListV2(ctx context.Context, c *app.RequestContext) {
	var req videos.GetFavoriteVideoListRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取收藏夹ID
	favoriteIdStr := c.Query("favorite_id")
	favoriteId, err := strconv.ParseInt(favoriteIdStr, 10, 64)
	if err != nil || favoriteId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.FavoriteId = favoriteId

	// 获取分页参数
	pageNum, _ := strconv.ParseInt(c.DefaultQuery("page_num", "1"), 10, 64)
	pageSize, _ := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 64)
	req.PageNum = pageNum
	req.PageSize = pageSize

	// 获取排序参数
	req.SortBy = c.DefaultQuery("sort_by", "created_at DESC")

	resp, err := rpc.GetFavoriteVideoListV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.GetFavoriteVideoListV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// AddFavoriteVideoV2 handles add video to favorite requests (V2)
func AddFavoriteVideoV2(ctx context.Context, c *app.RequestContext) {
	var req videos.AddFavoriteVideoRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 绑定请求参数
	if err := c.BindJSON(&req); err != nil {
		hlog.Error("AddFavoriteVideoV2 BindJSON error:", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	if req.FavoriteId == 0 || req.VideoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	resp, err := rpc.AddFavoriteVideoV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.AddFavoriteVideoV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// DeleteFavoriteV2 handles delete favorite requests (V2)
func DeleteFavoriteV2(ctx context.Context, c *app.RequestContext) {
	var req videos.DeleteFavoriteRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取收藏夹ID
	favoriteIdStr := c.Query("favorite_id")
	favoriteId, err := strconv.ParseInt(favoriteIdStr, 10, 64)
	if err != nil || favoriteId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.FavoriteId = favoriteId

	// 获取删除原因（可选）
	req.DeleteReason = c.Query("delete_reason")

	resp, err := rpc.DeleteFavoriteV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.DeleteFavoriteV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}

// DeleteVideoFromFavoriteV2 handles delete video from favorite requests (V2)
func DeleteVideoFromFavoriteV2(ctx context.Context, c *app.RequestContext) {
	var req videos.DeleteVideoFromFavoriteRequestV2

	// 获取当前用户ID
	userId, exists := c.Get("user_id")
	if !exists {
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}
	req.UserId = userId.(int64)

	// 获取收藏夹ID和视频ID
	favoriteIdStr := c.Query("favorite_id")
	favoriteId, err := strconv.ParseInt(favoriteIdStr, 10, 64)
	if err != nil || favoriteId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.FavoriteId = favoriteId

	videoIdStr := c.Query("video_id")
	videoId, err := strconv.ParseInt(videoIdStr, 10, 64)
	if err != nil || videoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}
	req.VideoId = videoId

	// 获取删除原因（可选）
	req.RemoveReason = c.Query("remove_reason")

	resp, err := rpc.DeleteVideoFromFavoriteV2(ctx, &req)
	if err != nil {
		hlog.Error("rpc.DeleteVideoFromFavoriteV2 error:", err)
		SendResponse(c, errno.RpcErr, nil)
		return
	}

	c.JSON(consts.StatusOK, resp)
}
