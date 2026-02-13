package handlers

import (
	"context"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
)

// HotVideoRankingParam 热门视频排行榜请求参数
type HotVideoRankingParam struct {
	Limit     int64  `form:"limit" json:"limit"`           // 返回数量，默认20
	TimeRange string `form:"time_range" json:"time_range"` // 时间范围: day, week, month
	Category  string `form:"category" json:"category"`     // 分类
}

// GetHotVideoRankingV2 获取热门视频排行榜
func GetHotVideoRankingV2(ctx context.Context, c *app.RequestContext) {
	var err error

	// 解析参数
	var param HotVideoRankingParam
	if err = c.BindAndValidate(&param); err != nil {
		param = HotVideoRankingParam{}
	}

	// 设置默认值
	if param.Limit <= 0 || param.Limit > 100 {
		param.Limit = 20
	}
	if param.TimeRange == "" {
		param.TimeRange = "day"
	}

	// 复用现有的 VideoPopular RPC 调用
	resp, err := rpc.VideoPopular(ctx, &videos.VideoPopularRequestV2{
		PageNum:   1,
		PageSize:  param.Limit,
		TimeRange: param.TimeRange,
		Category:  param.Category,
	})

	if err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}

	SendResponse(c, errno.Success, resp)
}

