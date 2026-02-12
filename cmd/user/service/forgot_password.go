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

	// Store reverse mapping (token -> email) for validation
	err = redis.SetResetTokenReverse(resetToken, req.Email, 30*time.Minute)
	if err != nil {
		return "", errors.WithMessage(err, "存储令牌反向映射失败")
	}

	// Send password reset email (log-based in dev, integrate SMTP/SendGrid in production)
	resetLink := "https://tiktok.example.com/reset-password?token=" + resetToken
	hlog.Infof("Password reset email sent to %s, reset link: %s", req.Email, resetLink)

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
