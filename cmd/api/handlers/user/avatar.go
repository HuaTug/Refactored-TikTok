// Copyright 2021 CloudWeGo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package handlers

import (
	"context"
	"strconv"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/errno"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/common/utils"
)

type GetAvatarUploadUrlParam struct {
	FileExtension string `form:"file_extension" json:"file_extension" binding:"required"`
}

type UpdateAvatarParam struct {
	AvatarUrl string `form:"avatar_url" json:"avatar_url" binding:"required"`
}

// GetAvatarUploadUrl 获取头像上传预签名URL
func GetAvatarUploadUrl(ctx context.Context, c *app.RequestContext) {
	var param GetAvatarUploadUrlParam
	if err := c.BindAndValidate(&param); err != nil {
		hlog.Error("GetAvatarUploadUrl: bind and validate error: ", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 从token获取用户ID
	userIdInterface, exists := c.Get("user_id")
	if !exists {
		hlog.Error("GetAvatarUploadUrl: user_id not found in context")
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}

	// 转换用户ID为int64
	var userId int64
	switch v := userIdInterface.(type) {
	case int64:
		userId = v
	case float64:
		userId = int64(v)
	case string:
		var err error
		userId, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			hlog.Error("GetAvatarUploadUrl: invalid user_id format: ", err)
			SendResponse(c, errno.ParamErr, nil)
			return
		}
	default:
		hlog.Error("GetAvatarUploadUrl: unsupported user_id type")
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	hlog.Infof("GetAvatarUploadUrl: userId=%d, file_extension=%s", userId, param.FileExtension)

	// 创建带有user_id的context
	rpcCtx := context.WithValue(ctx, "user_id", userId)

	// 调用RPC服务
	resp, err := rpc.GetAvatarUploadUrl(rpcCtx, &users.GetAvatarUploadUrlRequest{
		FileExtension: param.FileExtension,
	})

	if err != nil {
		hlog.Error("GetAvatarUploadUrl: rpc.GetAvatarUploadUrl error: ", err)
		SendResponse(c, err, nil)
		return
	}

	c.JSON(200, utils.H{
		"code":       resp.Base.Code,
		"message":    resp.Base.Msg,
		"upload_url": resp.UploadUrl,
		"access_url": resp.AccessUrl,
		"expires_in": resp.ExpiresIn,
	})
}

// UpdateAvatar 更新用户头像
func UpdateAvatar(ctx context.Context, c *app.RequestContext) {
	var param UpdateAvatarParam
	if err := c.BindAndValidate(&param); err != nil {
		hlog.Error("UpdateAvatar: bind and validate error: ", err)
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	// 从token获取用户ID
	userIdInterface, exists := c.Get("user_id")
	if !exists {
		hlog.Error("UpdateAvatar: user_id not found in context")
		SendResponse(c, errno.AuthorizationFailedErr, nil)
		return
	}

	// 转换用户ID为int64
	var userId int64
	switch v := userIdInterface.(type) {
	case int64:
		userId = v
	case float64:
		userId = int64(v)
	case string:
		var err error
		userId, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			hlog.Error("UpdateAvatar: invalid user_id format: ", err)
			SendResponse(c, errno.ParamErr, nil)
			return
		}
	default:
		hlog.Error("UpdateAvatar: unsupported user_id type")
		SendResponse(c, errno.ParamErr, nil)
		return
	}

	hlog.Infof("UpdateAvatar: userId=%d, avatar_url=%s", userId, param.AvatarUrl)

	// 创建带有user_id的context
	rpcCtx := context.WithValue(ctx, "user_id", userId)

	// 调用RPC服务
	resp, err := rpc.UpdateAvatar(rpcCtx, &users.UpdateAvatarRequest{
		AvatarUrl: param.AvatarUrl,
	})

	if err != nil {
		hlog.Error("UpdateAvatar: rpc.UpdateAvatar error: ", err)
		SendResponse(c, err, nil)
		return
	}

	SendResponse(c, nil, utils.H{
		"user": resp.User,
	})
}
