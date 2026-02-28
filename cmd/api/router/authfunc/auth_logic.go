package authfunc

import (
	"context"

	handlers "HuaTug.com/cmd/api/handler/interaction"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
)

func Auth() []app.HandlerFunc {
	return append(make([]app.HandlerFunc, 0),
		DoubleTokenAuthFunc(),
	)
}

// OptionalAuth 可选认证：如果有有效 token 则解析出 userID 并设置到 context，
// 没有 token 或 token 无效时静默通过，不中断请求。
// 用于不强制登录但需要个性化数据（如评论点赞状态）的接口。
func OptionalAuth() []app.HandlerFunc {
	return []app.HandlerFunc{OptionalTokenAuthFunc()}
}

func DoubleTokenAuthFunc() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !jwt.IsAccessTokenAvailable(ctx, c) {
			if !jwt.IsRefreshTokenAvailable(ctx, c) {
				handlers.SendResponse(c, errno.ConvertErr(errno.TokenInvailedErr), nil)
				c.Abort()
				return
			}
			//此时表示refresh-token并未过期 在生成一个新的access-token
			//resp:=new(Res)

			//ToDo
			jwt.GenerateAccessToken(ctx, c)

		}
		c.Next(ctx)
	}
}

// OptionalTokenAuthFunc 尝试解析 Access-Token，成功则设置 identity，
// 失败则静默跳过，不阻断请求链。
func OptionalTokenAuthFunc() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 尝试解析 token，成功时 IsAccessTokenAvailable 会自动 c.Set IdentityKey
		jwt.IsAccessTokenAvailable(ctx, c)
		c.Next(ctx)
	}
}
