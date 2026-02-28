package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func CreateFavoriteVideo(ctx context.Context, c *app.RequestContext) {
	var CreateFavorite CreateFavoriteParam
	var err error
	var v interface{}
	var UserId int64

	if err = c.Bind(&CreateFavorite); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}

	// 前端发送 title，后端字段为 name，做兼容处理
	name := CreateFavorite.Name
	if name == "" {
		name = CreateFavorite.Title
	}

	// 前端发送 coverImage，后端字段为 cover_url，做兼容处理
	coverUrl := CreateFavorite.CoverUrl
	if coverUrl == "" {
		coverUrl = CreateFavorite.CoverImage
	}

	// 前端 showStatus: "0"=公开, "1"=私密
	privacy := "private"
	if CreateFavorite.ShowStatus == "0" {
		privacy = "public"
	}

	resp, err := rpc.CreateFavorite(ctx, &videos.CreateFavoriteRequestV2{
		UserId:      UserId,
		Name:        name,
		Description: CreateFavorite.Description,
		CoverUrl:    coverUrl,
		Privacy:     privacy,
		Tags:        []string{},
	})
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	SendResponse(c, errno.Success, resp)
}
