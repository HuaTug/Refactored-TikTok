package service

import (
	"context"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/cmd/user/infras/redis"
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
	// 这里需要反向查找令牌对应的邮箱
	// 由于Redis的限制，我们需要一种方式来存储令牌到邮箱的映射
	// 暂时通过简单的方式实现，生产环境应该使用更安全的方法

	// TODO: 实际实现中应该在设置令牌时同时设置反向映射
	// 这里暂时返回错误，需要在实际使用时完善
	return "", errors.New("令牌验证功能待完善")
}
