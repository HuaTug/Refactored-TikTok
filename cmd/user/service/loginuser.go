package service

import (
	"context"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/users"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type LoginuserService struct {
	ctx context.Context
}

func NewLoginUserService(ctx context.Context) *LoginuserService {
	return &LoginuserService{ctx: ctx}
}

func (v *LoginuserService) LoginUser(req *users.LoginUserResquest) (*base.User, error, bool) {
	var user base.User
	var err error
	var flag bool

	// 用户名密码登录
	if req.UserName == "" || req.Password == "" {
		return nil, errors.WithMessage(nil, "用户名和密码不能为空"), false
	}

	hlog.Info("username:", req.UserName, ", password:", req.Password)
	if user, err, flag = db.CheckUser(v.ctx, req.UserName, req.Password); err != nil || !flag {
		return nil, errors.WithMessage(err, "用户名或密码错误"), false
	}

	return &user, nil, true
}
