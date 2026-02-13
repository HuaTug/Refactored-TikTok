package handlers

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/users"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func LoginUser(ctx context.Context, c *app.RequestContext) {
	var loginVar LoginParam
	var err error
	if err := c.Bind(&loginVar); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	resp := new(users.LoginUserResponse)
	hlog.Info(loginVar.UserName, ",", loginVar.PassWord, ",", loginVar.Email)
	resp, err = rpc.LoginUser(ctx, &users.LoginUserResquest{
		UserName: loginVar.UserName,
		Password: loginVar.PassWord,
		Email:    loginVar.Email,
	})

	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	// 只有登录成功时才生成 token
	if resp.Base.Code == consts.StatusOK {
		// 设置 user_id 以便生成 token
		c.Set("user_id", resp.User.UserId)

		// 生成 Access Token
		accessToken, _, _ := jwt.AccessTokenJwtMiddleware.TokenGenerator(resp.User.UserId)

		// 生成 Refresh Token
		refreshToken, _, _ := jwt.RefreshTokenJwtMiddleware.TokenGenerator(jwt.PayloadIdentityData{
			Uid: fmt.Sprint(resp.User.UserId),
		})

		resp.Base.Msg = "Login Success"
		resp.Token = accessToken
		resp.RefreshToken = refreshToken
	}

	SendResponse(c, errno.Success, resp)
}
