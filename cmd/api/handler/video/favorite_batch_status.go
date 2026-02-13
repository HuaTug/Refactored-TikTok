package handlers

import (
	"context"

	"HuaTug.com/cmd/api/dal"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type BatchFavoriteStatusParam struct {
	VideoIds []int64 `json:"video_ids"`
}

// BatchFavoriteStatus 批量查询用户收藏状态
func BatchFavoriteStatus(ctx context.Context, c *app.RequestContext) {
	var req BatchFavoriteStatusParam
	var err error
	var userId int64

	if err = c.BindAndValidate(&req); err != nil {
		hlog.Errorf("BatchFavoriteStatus bind error: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 使用与其他 handler 一致的方式获取 user_id
	userIdFromContext, exists := c.Get("user_id")
	if !exists {
		// 降级到 JWT 方式
		var v interface{}
		if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
			SendResponse(c, errno.ConvertErr(err), nil)
			return
		}
		userId = utils.Transfer(v)
		hlog.Infof("BatchFavoriteStatus: userId from JWT = %d", userId)
	} else {
		userId = userIdFromContext.(int64)
		hlog.Infof("BatchFavoriteStatus: userId from context = %d", userId)
	}

	if len(req.VideoIds) == 0 {
		SendResponse(c, errno.Success, map[string]interface{}{
			"favorite_status": make(map[int64]bool),
		})
		return
	}

	// 批量查询收藏状态
	favoriteStatus, err := dal.BatchCheckUserFavorites(ctx, userId, req.VideoIds)
	if err != nil {
		hlog.Errorf("BatchCheckUserFavorites error: %v", err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	SendResponse(c, errno.Success, map[string]interface{}{
		"favorite_status": favoriteStatus,
	})
}
