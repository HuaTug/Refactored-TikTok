// Package response provides unified HTTP response utilities for all API handlers.
// This package consolidates the common response handling logic that was previously
// duplicated across multiple handler packages.
package response

import (
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// Response represents the standard API response structure.
type Response struct {
	Code    int64       `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// SendResponse packs and sends a standardized JSON response.
// It converts the error to an errno.Errno type and extracts the error code and message.
func SendResponse(c *app.RequestContext, err error, data interface{}) {
	Err := errno.ConvertErr(err)
	c.JSON(consts.StatusOK, Response{
		Code:    Err.ErrCode,
		Message: Err.ErrMsg,
		Data:    data,
	})
}

// SendSuccess sends a successful response with the given data.
func SendSuccess(c *app.RequestContext, data interface{}) {
	SendResponse(c, nil, data)
}

// SendError sends an error response.
func SendError(c *app.RequestContext, err error) {
	SendResponse(c, err, nil)
}

// PaginatedResponse represents a paginated API response.
type PaginatedResponse struct {
	Code    int64       `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	Total   int64       `json:"total"`
	Page    int64       `json:"page"`
	Size    int64       `json:"size"`
}

// SendPaginatedResponse sends a paginated response.
func SendPaginatedResponse(c *app.RequestContext, err error, data interface{}, total, page, size int64) {
	Err := errno.ConvertErr(err)
	c.JSON(consts.StatusOK, PaginatedResponse{
		Code:    Err.ErrCode,
		Message: Err.ErrMsg,
		Data:    data,
		Total:   total,
		Page:    page,
		Size:    size,
	})
}
