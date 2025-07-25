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

package main

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/cmd/user/service"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/users"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/pkg/errors"
)

// UserServiceImpl implements the last service interface defined in the IDL.
type UserServiceImpl struct{}

// CreateUser implements the UserServiceImpl interface.
func (s *UserServiceImpl) CreateUser(ctx context.Context, req *users.CreateUserRequest) (resp *users.CreateUserResponse, err error) {
	resp = new(users.CreateUserResponse)
	resp.Base = &base.Status{}

	// 参数验证
	if len(req.UserName) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "用户名不能为空"
		return resp, nil
	}

	if len(req.Password) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "密码不能为空"
		return resp, nil
	}

	// 密码长度验证
	if len(req.Password) < 6 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "密码长度不能少于6位"
		return resp, nil
	}

	err = service.NewCreateUserService(ctx).CreateUser(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.CreateUser failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "创建用户失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "创建用户成功"
	return resp, nil
}

func (s *UserServiceImpl) UpdateUser(ctx context.Context, req *users.UpdateUserRequest) (resp *users.UpdateUserResponse, err error) {
	var user *base.User
	resp = new(users.UpdateUserResponse)
	resp.Base = &base.Status{}
	if err := service.NewUpdateUserService(ctx).UpdateUser(req); err != nil {
		hlog.CtxErrorf(ctx, "service.UpdateUser failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail To Update User"
		resp.Data = nil
		return resp, err
	}
	if user, err = db.GetUser(ctx, fmt.Sprint(req.UserId)); err != nil {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Get user info failed"
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Update User Success"
	resp.Data = user
	return resp, nil
}

func (s *UserServiceImpl) LoginUser(ctx context.Context, req *users.LoginUserResquest) (resp *users.LoginUserResponse, err error) {
	resp = new(users.LoginUserResponse)
	resp.Base = &base.Status{}

	// 参数验证
	if len(req.UserName) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "用户名不能为空"
		return resp, nil
	}

	if len(req.Password) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "密码不能为空"
		return resp, nil
	}

	var user *base.User
	var flag bool
	user, err, flag = service.NewLoginUserService(ctx).LoginUser(req)
	if err != nil || !flag || user == nil {
		hlog.CtxErrorf(ctx, "service.LoginUser failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusUnauthorized
		resp.Base.Msg = "用户名或密码错误"
		return resp, nil
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "登录成功"
	resp.User = user
	return resp, nil
}

func (s *UserServiceImpl) CheckUserExistsById(ctx context.Context, req *users.CheckUserExistsByIdRequst) (resp *users.CheckUserExistsByIdResponse, err error) {
	resp = new(users.CheckUserExistsByIdResponse)
	resp.Base = &base.Status{}
	resp.Exists, err = service.NewCheckUserExistsService(ctx).CheckUserExists(req)
	if err != nil || !resp.Exists {
		hlog.CtxErrorf(ctx, "service.CheckUserExists failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "User not exists!"
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "User exists!"
	return resp, nil
}

func (s *UserServiceImpl) QueryUser(ctx context.Context, req *users.QueryUserRequest) (resp *users.QueryUserResponse, err error) {
	resp = new(users.QueryUserResponse)
	resp.Base = &base.Status{}

	resp.Users, resp.Totoal, err = service.NewQueryUserService(ctx).QueryUserInfo(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.QueryUser failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "查询用户失败"
		return resp, err
	}

	if resp.Totoal == 0 {
		resp.Base.Code = consts.StatusOK
		resp.Base.Msg = "没有找到匹配的用户"
		return resp, nil
	}

	// 注意：由于我们已经从base.User中移除了password字段，所以不需要再隐藏密码
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "查询成功"
	return resp, nil
}

func (s *UserServiceImpl) GetUserInfo(ctx context.Context, req *users.GetUserInfoRequest) (resp *users.GetUserInfoResponse, err error) {
	resp = new(users.GetUserInfoResponse)
	resp.Base = &base.Status{}
	var user *base.User
	if user, err = service.NewGetUserInfoService(ctx).GetUserInfo(req.UserId); err != nil {
		hlog.CtxErrorf(ctx, "service.GetUserInfo failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "Fail to Get User Info!"
		return resp, err
	}
	resp.User = user
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "Get User Info Success!"
	return resp, nil
}

func (s *UserServiceImpl) DeleteUser(ctx context.Context, req *users.DeleteUserRequest) (resp *users.DeleteUserResponse, err error) {
	resp = new(users.DeleteUserResponse)
	resp.Base = &base.Status{}
	if err := service.NewDeleteUSerService(ctx).DeleteUser(req.UserId); err != nil {
		hlog.CtxErrorf(ctx, "service.DeleteUser failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Msg = "Fail to Delete User!"
		resp.Base.Code = 500
		return resp, err
	}
	resp.Base = &base.Status{}
	resp.Base.Code = 200
	resp.Base.Msg = "Delete User Success!"
	return resp, nil
}

func (s *UserServiceImpl) VerifyCode(ctx context.Context, req *users.VerifyCodeRequest) (resp *users.VerifyCodeResponse, err error) {
	resp = new(users.VerifyCodeResponse)
	resp.Base = &base.Status{}
	if err := service.NewVerifyCodeService(ctx).VerifyCode(req); err != nil {
		hlog.CtxErrorf(ctx, "service.VerifyCode failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Msg = "Fail to Verify Code!"
		resp.Base.Code = 500
		return resp, err
	}
	resp.Base = &base.Status{}
	resp.Base.Code = 200
	resp.Base.Msg = "Verify Code Success!"
	return resp, nil
}

func (s *UserServiceImpl) SendCode(ctx context.Context, req *users.SendCodeRequest) (resp *users.SendCodeResponse, err error) {
	resp = new(users.SendCodeResponse)
	resp.Base = &base.Status{}

	_, err = service.NewSendCodeService(ctx).SendCode(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.SendCode failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Msg = "发送验证码失败!"
		resp.Base.Code = consts.StatusBadRequest
		return resp, err
	}
	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "验证码发送成功"
	return resp, nil
}

// ========== V2版本API实现 ==========

func (s *UserServiceImpl) ForgotPassword(ctx context.Context, req *users.ForgotPasswordRequest) (resp *users.ForgotPasswordResponse, err error) {
	resp = new(users.ForgotPasswordResponse)
	resp.Base = &base.Status{}

	if len(req.Email) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "邮箱不能为空"
		return resp, nil
	}

	resetToken, err := service.NewForgotPasswordService(ctx).ForgotPassword(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.ForgotPassword failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "发送重置密码邮件失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "重置密码邮件已发送"
	resp.ResetToken = resetToken
	return resp, nil
}

func (s *UserServiceImpl) ResetPassword(ctx context.Context, req *users.ResetPasswordRequest) (resp *users.ResetPasswordResponse, err error) {
	resp = new(users.ResetPasswordResponse)
	resp.Base = &base.Status{}

	if len(req.ResetToken) == 0 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "重置令牌不能为空"
		return resp, nil
	}

	if len(req.NewPassword_) < 6 {
		resp.Base.Code = consts.StatusBadRequest
		resp.Base.Msg = "新密码长度不能少于6位"
		return resp, nil
	}

	err = service.NewResetPasswordService(ctx).ResetPassword(req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.ResetPassword failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "重置密码失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "密码重置成功"
	return resp, nil
}

func (s *UserServiceImpl) GetMyProfile(ctx context.Context, req *users.GetMyProfileRequest) (resp *users.GetMyProfileResponse, err error) {
	resp = new(users.GetMyProfileResponse)
	resp.Base = &base.Status{}

	// 从JWT token中获取用户ID (这里需要middleware支持)
	// 临时从context中获取用户ID，实际应该从JWT中解析
	userIdValue := ctx.Value("user_id")
	if userIdValue == nil {
		resp.Base.Code = consts.StatusUnauthorized
		resp.Base.Msg = "用户未登录"
		return resp, nil
	}

	userId, ok := userIdValue.(int64)
	if !ok {
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "用户ID格式错误"
		return resp, nil
	}

	user, err := service.NewGetUserInfoService(ctx).GetUserInfo(userId)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.GetMyProfile failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "获取用户信息失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取用户信息成功"
	resp.User = user
	return resp, nil
}

func (s *UserServiceImpl) GetAvatarUploadUrl(ctx context.Context, req *users.GetAvatarUploadUrlRequest) (resp *users.GetAvatarUploadUrlResponse, err error) {
	resp = new(users.GetAvatarUploadUrlResponse)
	resp.Base = &base.Status{}

	// 从JWT token中获取用户ID
	userIdValue := ctx.Value("user_id")
	if userIdValue == nil {
		resp.Base.Code = consts.StatusUnauthorized
		resp.Base.Msg = "用户未登录"
		return resp, nil
	}

	userId, ok := userIdValue.(int64)
	if !ok {
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "用户ID格式错误"
		return resp, nil
	}

	uploadUrl, accessUrl, expiresIn, err := service.NewAvatarUploadService(ctx).GetAvatarUploadUrl(userId, req.FileExtension)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.GetAvatarUploadUrl failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "获取上传URL失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "获取上传URL成功"
	resp.UploadUrl = uploadUrl
	resp.AccessUrl = accessUrl
	resp.ExpiresIn = expiresIn
	return resp, nil
}

func (s *UserServiceImpl) UpdateAvatar(ctx context.Context, req *users.UpdateAvatarRequest) (resp *users.UpdateAvatarResponse, err error) {
	resp = new(users.UpdateAvatarResponse)
	resp.Base = &base.Status{}

	// 从JWT token中获取用户ID
	userIdValue := ctx.Value("user_id")
	if userIdValue == nil {
		resp.Base.Code = consts.StatusUnauthorized
		resp.Base.Msg = "用户未登录"
		return resp, nil
	}

	userId, ok := userIdValue.(int64)
	if !ok {
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "用户ID格式错误"
		return resp, nil
	}

	user, err := service.NewUpdateAvatarService(ctx).UpdateAvatar(userId, req.AvatarUrl)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.UpdateAvatar failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "更新头像失败"
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "更新头像成功"
	resp.User = user
	return resp, nil
}

func (s *UserServiceImpl) ChangePassword(ctx context.Context, req *users.ChangePasswordRequest) (resp *users.ChangePasswordResponse, err error) {
	resp = new(users.ChangePasswordResponse)
	resp.Base = &base.Status{}

	// 从JWT token中获取用户ID
	userIdValue := ctx.Value("user_id")
	if userIdValue == nil {
		resp.Base.Code = consts.StatusUnauthorized
		resp.Base.Msg = "用户未登录"
		return resp, nil
	}

	userId, ok := userIdValue.(int64)
	if !ok {
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = "用户ID格式错误"
		return resp, nil
	}

	err = service.NewChangePasswordService(ctx).ChangePassword(userId, req)
	if err != nil {
		hlog.CtxErrorf(ctx, "service.ChangePassword failed,original error:%v", errors.Cause(err))
		hlog.CtxErrorf(ctx, "stack trace: \n%+v\n", err)
		resp.Base.Code = consts.StatusInternalServerError
		resp.Base.Msg = err.Error()
		return resp, err
	}

	resp.Base.Code = consts.StatusOK
	resp.Base.Msg = "密码修改成功"
	return resp, nil
}
