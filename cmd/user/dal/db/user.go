package db

import (
	"context"

	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/pkg/utils"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const TableName = "users"

// UserWithPassword 内部数据库模型，包含密码字段
type UserWithPassword struct {
	UserId    int64  `gorm:"column:user_id" json:"user_id"`
	UserName  string `gorm:"column:user_name" json:"user_name"`
	Password  string `gorm:"column:password" json:"-"` // 密码字段，不在JSON中序列化
	Email     string `gorm:"column:email" json:"email"`
	Sex       int64  `gorm:"column:sex" json:"sex"`
	AvatarUrl string `gorm:"column:avatar_url" json:"avatar_url"`
	CreatedAt string `gorm:"column:created_at" json:"created_at"`
	UpdatedAt string `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt string `gorm:"column:deleted_at" json:"deleted_at"`
}

func (u *UserWithPassword) TableName() string {
	return TableName
}
/*
这里需要注意的是这个TableName()方法其实是GORM框架的一个接口约定，在GORM中有一个接口定义
type Tabler interface {
	TableName() string
}

GORM在内部会自动检查和调用这个方法，例如DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_name=?", username).Count(&count)
GORM内部的工作流程就是其会检查&UserWithPassword{}是否实现了Tabler接口，如果实现了则会调用TableName()方法获取表名

*/

// convertToBaseUser 将内部模型转换为base.User
func (u *UserWithPassword) convertToBaseUser() *base.User {
	return &base.User{
		UserId:    u.UserId,
		UserName:  u.UserName,
		Email:     u.Email,
		Sex:       u.Sex,
		AvatarUrl: u.AvatarUrl,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		DeletedAt: u.DeletedAt,
	}
}


func CreateUser(ctx context.Context, user *base.User) error {
	userWithPassword := &UserWithPassword{
		UserName:  user.UserName,
		Password:  user.Password,
		Email:     user.Email,
		Sex:       user.Sex,
		AvatarUrl: user.AvatarUrl,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	if err := DB.WithContext(ctx).Create(userWithPassword).Error; err != nil {
		return errors.Wrapf(err, "CreateUser failed,err: %v", err)
	}
	return nil
}

func CheckUser(ctx context.Context, username, password string) (base.User, error, bool) {
	var userWithPassword UserWithPassword
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("Binary user_name=?", username).Count(&count).Find(&userWithPassword).Error; err != nil {
		hlog.Info(err)
		return base.User{}, errors.Wrap(err, "查询用户存在性失败"), false
	}
	if count == 0 {
		logrus.Info("没有这个用户，请重新登录")
		return base.User{}, errors.Errorf("数据库中不存在这个用户"), false
	} else if count == 1 {
		if err, flag := utils.VerifyPassword(password, userWithPassword.Password); !flag {
			return base.User{}, errors.Wrapf(err, "Password Wrong,err:%v", err), false
		}
	}
	return *userWithPassword.convertToBaseUser(), nil, true
}

func CheckUserExistById(ctx context.Context, userId int64) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "User not exist,err:%v", err)
	}
	return count > 0, nil
}

func DeleteUser(ctx context.Context, userId int64) error {
	if err := DB.WithContext(ctx).Where("user_id = ?", userId).Delete(&UserWithPassword{}).Error; err != nil {
		return errors.Wrapf(err, "Delete user failed, userId: %d", userId)
	}
	return nil
}

func UpdateUser(ctx context.Context, user *base.User) error {
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", user.UserId).Updates(map[string]interface{}{
		"user_name":  user.UserName,
		"email":      user.Email,
		"sex":        user.Sex,
		"avatar_url": user.AvatarUrl,
		"updated_at": user.UpdatedAt,
	}).Error; err != nil {
		return errors.Wrapf(err, "Update user failed,err: %v", err)
	}
	return nil
}

// UpdateUserPassword 专门用于更新用户密码
func UpdateUserPassword(ctx context.Context, userId int64, newPassword string) error {
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).Update("password", newPassword).Error; err != nil {
		return errors.Wrapf(err, "Update user password failed,err: %v", err)
	}
	return nil
}

func QueryUser(ctx context.Context, keyword *string, page, pageSize int64) ([]*base.User, int64, error) {
	db := DB.WithContext(ctx).Model(&UserWithPassword{})
	if keyword != nil && len(*keyword) != 0 {
		db = db.Where("Binary user_name like ?", "%"+*keyword+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, total, errors.Wrapf(err, "QueryUser count failed,err:%v", err)
	}

	var userWithPasswords []*UserWithPassword
	if err := db.Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&userWithPasswords).Error; err != nil {
		return nil, total, errors.Wrapf(err, "Limit failed,err:%v", err)
	}

	// 转换为 base.User 类型，不包含密码
	var res []*base.User
	for _, u := range userWithPasswords {
		res = append(res, u.convertToBaseUser())
	}

	return res, total, nil
}

func GetUser(ctx context.Context, userId string) (*base.User, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).First(&userWithPassword).Error; err != nil {
		logrus.Info(err)
		return nil, errors.Wrapf(err, "GetUser failed,err:%v", err)
	}
	return userWithPassword.convertToBaseUser(), nil
}

func UserExist(ctx context.Context, uid string) ([]*base.User, error) {
	var users []*UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", uid).Find(&users).Error; err != nil {
		return nil, errors.Wrapf(err, "User not exist,err:%v", err)
	}

	// 转换为 base.User 类型
	var result []*base.User
	for _, u := range users {
		result = append(result, u.convertToBaseUser())
	}
	return result, nil
}

func RemoveDuplicate(ctx context.Context, username string) (err error, flag bool) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_name=?", username).Count(&count).Error; err != nil {
		return errors.Wrap(err, "Query user failed!"), false
	}
	if count > 0 {
		return errors.Errorf("User is duplicate,please register again!"), false
	}
	return nil, true
}

func UploadAvatarUrl(ctx context.Context, uid, avatarUrl string) error {
	user, err := UserExist(ctx, uid)
	if err != nil || len(user) == 0 {
		return err
	}
	if err = DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id = ?", uid).Update("avatar_url", avatarUrl).Error; err != nil {
		return err
	}
	return nil
}

func GetMaxId(ctx context.Context) (int64, error) {
	var maxId int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Select("max(user_id)").Scan(&maxId).Error; err != nil {
		return 0, err
	}
	return maxId, nil
}

// CheckEmailExists 检查邮箱是否存在
func CheckEmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("email=?", email).Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "CheckEmailExists failed,err:%v", err)
	}
	return count > 0, nil
}

// GetUserByEmail 根据邮箱获取用户信息
func GetUserByEmail(ctx context.Context, email string) (*base.User, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("email=?", email).First(&userWithPassword).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserByEmail failed,err:%v", err)
	}
	return userWithPassword.convertToBaseUser(), nil
}
