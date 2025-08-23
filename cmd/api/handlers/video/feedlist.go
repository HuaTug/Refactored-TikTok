package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func FeedService(ctx context.Context, c *app.RequestContext) {
	var err error
	var FeedList FeedListParam
	if err = c.Bind(&FeedList); err != nil {
		hlog.Info(err)
		SendResponse(c, errno.ConvertErr(err), nil)
	}
	resp, err := rpc.FeedList(ctx, &videos.VideoFeedListRequestV2{
		PageNum:        1,
		PageSize:       20,
		CategoryFilter: "",
		PrivacyFilter:  "public",
		TagFilters:     []string{},
	})
	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	SendResponse(c, errno.Success, resp)
}
