package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/pkg/oss"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

type AvatarUploadService struct {
	ctx context.Context
}

func NewAvatarUploadService(ctx context.Context) *AvatarUploadService {
	return &AvatarUploadService{ctx: ctx}
}

func (s *AvatarUploadService) GetAvatarUploadUrl(userId int64, fileExtension string) (uploadUrl, accessUrl string, expiresIn int64, err error) {
	bucketName := "picture"
	objectName := fmt.Sprintf("avatar/%d%s", userId, fileExtension)
	
	// 预签名 URL 有效期 15 分钟
	expires := 15 * time.Minute
	expiresIn = int64(expires.Seconds())

	// 生成预签名上传 URL
	uploadUrl, err = oss.GeneratePresignedPutURL(bucketName, objectName, expires)
	if err != nil {
		hlog.Errorf("生成预签名URL失败: %v", err)
		return "", "", 0, err
	}

	// 生成访问 URL
	accessUrl = fmt.Sprintf("%s/%s/%s", oss.GetMinIOEndpoint(), bucketName, objectName)

	hlog.Infof("为用户 %d 生成头像上传URL: %s", userId, uploadUrl)
	hlog.Infof("头像访问URL: %s", accessUrl)
	
	return uploadUrl, accessUrl, expiresIn, nil
}
