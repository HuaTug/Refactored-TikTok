package service

import (
	"context"

	"HuaTug.com/cmd/user/dal/db"
	redis "HuaTug.com/cmd/user/cache"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type ResetPasswordService struct {
	ctx context.Context
}

func NewResetPasswordService(ctx context.Context) *ResetPasswordService {
	return &ResetPasswordService{ctx: ctx}
}

func (s *ResetPasswordService) ResetPassword(req *users.ResetPasswordRequest) error {
	// 验证重置令牌
	email, err := s.validateResetToken(req.ResetToken)
	if err != nil {
		return errors.WithMessage(err, "重置令牌验证失败")
	}

	// 获取用户信息
	user, err := db.GetUserByEmail(s.ctx, email)
	if err != nil {
		return errors.WithMessage(err, "用户不存在")
	}

	// 加密新密码
	hashedPassword, err := utils.Crypt(req.NewPassword_)
	if err != nil {
		return errors.WithMessage(err, "密码加密失败")
	}

	// 更新密码
	err = db.UpdateUserPassword(s.ctx, user.UserId, hashedPassword)
	if err != nil {
		return errors.WithMessage(err, "更新密码失败")
	}

	// 删除重置令牌
	err = redis.DelResetToken(email)
	if err != nil {
		hlog.Warnf("删除重置令牌失败: %v", err)
	}

	hlog.Infof("用户 %s 密码重置成功", email)
	return nil
}

// validateResetToken 验证重置令牌并返回对应的邮箱
func (s *ResetPasswordService) validateResetToken(token string) (string, error) {
	// Lookup email from reverse mapping (token -> email)
	email, err := redis.GetResetTokenReverse(token)
	if err != nil {
		return "", errors.New("重置令牌无效或已过期")
	}

	// Double-check: verify forward mapping (email -> token) matches
	storedToken, err := redis.GetResetToken(email)
	if err != nil || storedToken != token {
		return "", errors.New("重置令牌不匹配或已过期")
	}

	// Clean up reverse mapping after successful validation
	_ = redis.DelResetTokenReverse(token)

	return email, nil
}
