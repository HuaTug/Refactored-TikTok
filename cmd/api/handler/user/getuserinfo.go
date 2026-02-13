package handlers

import (
	"context"
	"strconv"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/users"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// GetUserInfoParam 获取用户信息参数
type GetUserInfoParam struct {
	UserId int64 `form:"user_id" query:"user_id"`
}

func GetUserInfo(ctx context.Context, c *app.RequestContext) {
	var userId int64

	// 优先从请求参数获取 user_id
	userIdStr := c.Query("user_id")
	if userIdStr != "" {
		parsedId, err := strconv.ParseInt(userIdStr, 10, 64)
		if err == nil && parsedId > 0 {
			userId = parsedId
			hlog.Infof("[GetUserInfo] 从请求参数获取 userId: %d", userId)
		}
	}

	// 如果请求参数中没有 user_id，则从 JWT 获取当前登录用户
	if userId == 0 {
		if v, err := jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
			SendResponse(c, errno.ConvertErr(err), nil)
			return
		} else {
			userId = utils.Transfer(v)
			hlog.Infof("[GetUserInfo] 从 JWT 获取 userId: %d", userId)
		}
	}

	resp, err := rpc.GetUserInfo(ctx, &users.GetUserInfoRequest{
		UserId: userId,
	})
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), resp)
		return
	}
	SendResponse(c, errno.Success, resp)
}
