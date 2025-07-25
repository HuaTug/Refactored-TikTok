package service

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/kitex_gen/base"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type UpdateAvatarService struct {
	ctx context.Context
}

func NewUpdateAvatarService(ctx context.Context) *UpdateAvatarService {
	return &UpdateAvatarService{ctx: ctx}
}

func (s *UpdateAvatarService) UpdateAvatar(userId int64, avatarUrl string) (*base.User, error) {
	// 更新数据库中的头像URL
	err := db.UploadAvatarUrl(s.ctx, fmt.Sprintf("%d", userId), avatarUrl)
	if err != nil {
		return nil, errors.WithMessage(err, "更新头像URL失败")
	}

	// 获取更新后的用户信息
	user, err := db.GetUser(s.ctx, fmt.Sprintf("%d", userId))
	if err != nil {
		return nil, errors.WithMessage(err, "获取用户信息失败")
	}

	hlog.Infof("用户 %d 头像更新成功: %s", userId, avatarUrl)
	return user, nil
}
