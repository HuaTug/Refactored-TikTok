package handlers

import (
	"HuaTug.com/pkg/errno"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type Response struct {
	Code    int64       `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// SendResponse pack response
func SendResponse(c *app.RequestContext, err error, data interface{}) {
	Err := errno.ConvertErr(err)
	c.JSON(consts.StatusOK, Response{
		Code:    Err.ErrCode,
		Message: Err.ErrMsg,
		Data:    data,
	})
}

type RelationParam struct {
	ActionType int64 `form:"action_type" json:"action_type"`
	ToUserId   int64 `form:"to_user_id" json:"to_user_id"`
	UserId     int64 `form:"user_id" json:"user_id"`
}

type RelationPageParam struct {
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
	UserId   int64 `form:"user_id" json:"user_id"`
}
