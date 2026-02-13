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

func DeleteVideoFromFavorite(ctx context.Context, c *app.RequestContext) {
	var DeleteVideo DeleteVideoFromFavoriteParam
	var err error
	var v interface{}
	var UserId int64

	if err = c.BindAndValidate(&DeleteVideo); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	} else {
		UserId = utils.Transfer(v)
	}

	// video_id 必须提供
	if DeleteVideo.VideoId == 0 {
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	resp, err := rpc.DeleteVideoFromFavortie(ctx, &videos.DeleteVideoFromFavoriteRequestV2{
		UserId:       UserId,
		VideoId:      DeleteVideo.VideoId,
		FavoriteId:   DeleteVideo.FavoriteId, // 可以为0，表示从所有收藏夹中删除
		RemoveReason: "user_request",
	})
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	SendResponse(c, errno.Success, resp)
}
