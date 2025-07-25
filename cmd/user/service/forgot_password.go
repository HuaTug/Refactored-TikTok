package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/cmd/user/infras/redis"
	"HuaTug.com/kitex_gen/users"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type ForgotPasswordService struct {
	ctx context.Context
}

func NewForgotPasswordService(ctx context.Context) *ForgotPasswordService {
	return &ForgotPasswordService{ctx: ctx}
}

func (s *ForgotPasswordService) ForgotPassword(req *users.ForgotPasswordRequest) (string, error) {
	// 检查邮箱是否存在
	exists, err := db.CheckEmailExists(s.ctx, req.Email)
	if err != nil {
		return "", errors.WithMessage(err, "检查邮箱失败")
	}

	if !exists {
		return "", errors.New("邮箱不存在")
	}

	// 生成重置令牌
	resetToken, err := generateResetToken()
	if err != nil {
		return "", errors.WithMessage(err, "生成重置令牌失败")
	}

	// 将重置令牌存储到Redis，设置30分钟过期
	err = redis.SetResetToken(req.Email, resetToken, 30*time.Minute)
	if err != nil {
		return "", errors.WithMessage(err, "存储重置令牌失败")
	}

	// TODO: 这里应该发送包含重置链接的邮件
	// 暂时返回token用于测试
	hlog.Infof("发送重置密码邮件到 %s，重置令牌: %s", req.Email, resetToken)

	return resetToken, nil
}

// generateResetToken 生成重置密码令牌
func generateResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
