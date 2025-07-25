package service

import (
	"context"

	"HuaTug.com/cmd/user/dal/db"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/utils"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

type ChangePasswordService struct {
	ctx context.Context
}

func NewChangePasswordService(ctx context.Context) *ChangePasswordService {
	return &ChangePasswordService{ctx: ctx}
}

func (s *ChangePasswordService) ChangePassword(userId int64, req *users.ChangePasswordRequest) error {
	// 1. 参数验证
	if req.NewPassword_ != req.ConfirmPassword {
		return errors.New("新密码与确认密码不一致")
	}

	if req.OldPassword == req.NewPassword_ {
		return errors.New("新密码不能与旧密码相同")
	}

	// 2. 验证密码强度
	if err := s.validatePasswordStrength(req.NewPassword_); err != nil {
		return err
	}

	// 3. 获取用户当前信息，验证旧密码
	userWithPassword, err := s.getUserWithPassword(userId)
	if err != nil {
		return errors.WithMessage(err, "获取用户信息失败")
	}

	// 4. 验证旧密码
	if err, valid := utils.VerifyPassword(req.OldPassword, userWithPassword.Password); !valid {
		return errors.WithMessage(err, "旧密码验证失败")
	}

	// 5. 加密新密码
	hashedNewPassword, err := utils.Crypt(req.NewPassword_)
	if err != nil {
		return errors.WithMessage(err, "新密码加密失败")
	}

	// 6. 更新密码
	err = db.UpdateUserPassword(s.ctx, userId, hashedNewPassword)
	if err != nil {
		return errors.WithMessage(err, "更新密码失败")
	}

	hlog.Infof("用户 %d 密码修改成功", userId)
	return nil
}

// validatePasswordStrength 验证密码强度
func (s *ChangePasswordService) validatePasswordStrength(password string) error {
	if len(password) < 6 {
		return errors.New("密码长度不能少于6位")
	}

	if len(password) > 20 {
		return errors.New("密码长度不能超过20位")
	}

	// 检查是否包含数字
	hasDigit := false
	// 检查是否包含字母
	hasLetter := false

	for _, char := range password {
		if char >= '0' && char <= '9' {
			hasDigit = true
		}
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') {
			hasLetter = true
		}
	}

	if !hasDigit {
		return errors.New("密码必须包含至少一个数字")
	}

	if !hasLetter {
		return errors.New("密码必须包含至少一个字母")
	}

	return nil
}

// getUserWithPassword 获取包含密码的用户信息
func (s *ChangePasswordService) getUserWithPassword(userId int64) (*db.UserWithPassword, error) {
	var userWithPassword db.UserWithPassword
	err := db.DB.WithContext(s.ctx).Model(&db.UserWithPassword{}).Where("user_id=?", userId).First(&userWithPassword).Error
	if err != nil {
		return nil, errors.Wrapf(err, "查询用户失败, userId: %d", userId)
	}
	return &userWithPassword, nil
}
