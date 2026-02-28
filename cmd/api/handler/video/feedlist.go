package handlers

import (
	"context"
	"strconv"

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

	// 从查询参数获取分页信息，有默认值兜底
	var pageSize int64 = 10
	var pageNum int64 = 1

	if ps := c.Query("page_size"); ps != "" {
		if v, e := strconv.ParseInt(ps, 10, 64); e == nil && v > 0 {
			pageSize = v
		}
	}
	if pn := c.Query("page_num"); pn != "" {
		if v, e := strconv.ParseInt(pn, 10, 64); e == nil && v > 0 {
			pageNum = v
		}
	}

	resp, err := rpc.FeedList(ctx, &videos.VideoFeedListRequestV2{
		PageNum:        pageNum,
		PageSize:       pageSize,
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
