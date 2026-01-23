package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/oss"

	"github.com/pkg/errors"
)

type UpdateUserService struct {
	ctx context.Context
}

func NewUpdateUserService(ctx context.Context) *UpdateUserService {
	return &UpdateUserService{ctx: ctx}
}

func (v *UpdateUserService) UpdateUser(req *users.UpdateUserRequest) (err error) {
	// 上传头像到OSS
	var avatarUrl string
	if req.Data != nil && req.Filesize > 0 {
		if avatarUrl, err = v.uploadAvatarToOss(fmt.Sprint(req.UserId), req.Data, req.Filesize); err != nil {
			return errors.WithMessage(err, "uploadAvatarToOss failed")
		}
	}

	// 构建用户更新对象，只包含非空字段
	user := &base.User{
		UserId:    req.UserId,
		UpdatedAt: time.Now().Format(constants.DataFormate),
	}

	// 只有当字段非空时才更新
	if req.UserName != "" {
		user.UserName = req.UserName
	}
	if avatarUrl != "" {
		user.AvatarUrl = avatarUrl
	}
	// 更新性别 (0:女, 1:男, 2:保密)
	if req.Sex >= 0 {
		user.Sex = req.Sex
	}
	// 更新个人简介
	if req.Bio != "" {
		user.Bio = req.Bio
	}

	// 调用数据库更新
	if err := db.UpdateUser(v.ctx, user); err != nil {
		return errors.WithMessage(err, "dao.UpdateUser failed")
	}
	return nil
}

func (v *UpdateUserService) uploadAvatarToOss(uid string, fileContent []byte, fileSize int64) (string, error) {
	fileType := http.DetectContentType(fileContent)
	switch fileType {
	case "image/png", "image/jpg", "image/jpeg":
		{
			var avatarUrl string
			var err error
			if avatarUrl, err = oss.UploadAvatar(&fileContent, fileSize, uid, fileType); err != nil {
				return "", errno.ServiceErr
			}
			return avatarUrl, nil
		}
	default:
		return "", errno.DataProcessErr
	}
}
