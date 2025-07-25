package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type AvatarUploadService struct {
	ctx context.Context
}

func NewAvatarUploadService(ctx context.Context) *AvatarUploadService {
	return &AvatarUploadService{ctx: ctx}
}

func (s *AvatarUploadService) GetAvatarUploadUrl(userId int64, fileExtension string) (uploadUrl, accessUrl string, expiresIn int64, err error) {
	// 暂时使用传统上传方式，返回提示信息
	// TODO: 实现真正的预签名上传URL功能

	expiresIn = 15 * 60 // 15分钟

	// 生成唯一的上传标识
	uploadToken := fmt.Sprintf("upload_%d_%d", userId, time.Now().Unix())

	// 返回模拟的上传URL和访问URL
	uploadUrl = fmt.Sprintf("http://localhost:8080/upload/avatar?token=%s&ext=%s", uploadToken, fileExtension)
	accessUrl = fmt.Sprintf("http://localhost:9091/browser/picture/avatar/%d%s", userId, fileExtension)

	hlog.Infof("为用户 %d 生成头像上传URL: %s", userId, uploadUrl)
	return uploadUrl, accessUrl, expiresIn, nil
}
