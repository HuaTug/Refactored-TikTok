package db

import (
	"context"
	"time"

	"HuaTug.com/kitex_gen/base"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const TableName = "users"

// UserWithPassword represents the internal database model with all user fields
type UserWithPassword struct {
	UserId         int64      `gorm:"column:user_id;primaryKey" json:"user_id"`
	UserName       string     `gorm:"column:user_name" json:"user_name"`
	Password       string     `gorm:"column:password" json:"-"`
	Phone          *string    `gorm:"column:phone" json:"phone,omitempty"`
	Email          string     `gorm:"column:email" json:"email"`
	Sex            int64      `gorm:"column:sex" json:"sex"`
	AvatarUrl      string     `gorm:"column:avatar_url" json:"avatar_url"`
	BackgroundUrl  *string    `gorm:"column:background_url" json:"background_url,omitempty"`
	Bio            string     `gorm:"column:bio;default:''" json:"bio"`
	Birthday       *time.Time `gorm:"column:birthday" json:"birthday,omitempty"`
	Location       *string    `gorm:"column:location" json:"location,omitempty"`
	SchoolId       *int64     `gorm:"column:school_id" json:"school_id,omitempty"`
	FollowingCount uint       `gorm:"column:following_count;default:0" json:"following_count"`
	FollowerCount  uint       `gorm:"column:follower_count;default:0" json:"follower_count"`
	LikeCount      uint64     `gorm:"column:like_count;default:0" json:"like_count"`
	VideoCount     uint       `gorm:"column:video_count;default:0" json:"video_count"`
	Status         int8       `gorm:"column:status;default:1" json:"status"` // 1:normal 2:muted 3:banned
	LastLoginAt    *time.Time `gorm:"column:last_login_at" json:"last_login_at,omitempty"`
	LastLoginIp    *string    `gorm:"column:last_login_ip" json:"last_login_ip,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
}

func (u *UserWithPassword) TableName() string {
	return TableName
}

// convertToBaseUser converts internal model to base.User
// Note: base.User is generated from thrift, has limited fields
func (u *UserWithPassword) convertToBaseUser() *base.User {
	user := &base.User{
		UserId:    u.UserId,
		UserName:  u.UserName,
		Email:     u.Email,
		Sex:       u.Sex,
		AvatarUrl: u.AvatarUrl,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
		UpdatedAt: u.UpdatedAt.Format(time.RFC3339),
	}

	if u.DeletedAt != nil {
		user.DeletedAt = u.DeletedAt.Format(time.RFC3339)
	}

	return user
}

func CreateUser(ctx context.Context, user *base.User) error {
	now := time.Now()
	userWithPassword := &UserWithPassword{
		UserName:  user.UserName,
		Password:  user.Password,
		Email:     user.Email,
		Sex:       user.Sex,
		AvatarUrl: user.AvatarUrl,
		Status:    1, // default normal status
		CreatedAt: now,
		UpdatedAt: now,
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
		// Check if user is banned
		if userWithPassword.Status == 3 {
			return base.User{}, errors.Errorf("用户已被封禁"), false
		}
		if err, flag := utils.VerifyPassword(password, userWithPassword.Password); !flag {
			return base.User{}, errors.Wrapf(err, "Password Wrong,err:%v", err), false
		}
	}

	// Update last login time
	now := time.Now()
	DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userWithPassword.UserId).Updates(map[string]interface{}{
		"last_login_at": now,
	})

	return *userWithPassword.convertToBaseUser(), nil, true
}

func CheckUserExistById(ctx context.Context, userId int64) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=? AND deleted_at IS NULL", userId).Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "User not exist,err:%v", err)
	}
	return count > 0, nil
}

func DeleteUser(ctx context.Context, userId int64) error {
	// Soft delete by setting deleted_at
	now := time.Now()
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id = ?", userId).Update("deleted_at", now).Error; err != nil {
		return errors.Wrapf(err, "Delete user failed, userId: %d", userId)
	}
	return nil
}

func UpdateUser(ctx context.Context, user *base.User) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	// 只更新非空字段
	if user.UserName != "" {
		updates["user_name"] = user.UserName
	}
	if user.Email != "" {
		updates["email"] = user.Email
	}
	if user.Sex != 0 {
		updates["sex"] = user.Sex
	}
	if user.AvatarUrl != "" {
		updates["avatar_url"] = user.AvatarUrl
	}

	if len(updates) == 1 { // 只有updated_at，没有其他字段需要更新
		return nil
	}

	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", user.UserId).Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "Update user failed,err: %v", err)
	}
	return nil
}

// UpdateUserExtended updates user with extended fields (not in base.User)
func UpdateUserExtended(ctx context.Context, userId int64, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "Update user extended failed,err: %v", err)
	}
	return nil
}

// UpdateUserPassword updates user password
func UpdateUserPassword(ctx context.Context, userId int64, newPassword string) error {
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).Updates(map[string]interface{}{
		"password":   newPassword,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return errors.Wrapf(err, "Update user password failed,err: %v", err)
	}
	return nil
}

// UpdateUserStatus updates user status (for muting/banning)
func UpdateUserStatus(ctx context.Context, userId int64, status int8) error {
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return errors.Wrapf(err, "Update user status failed,err: %v", err)
	}
	return nil
}

// UpdateUserCounts updates user follower/following/video counts
func UpdateUserCounts(ctx context.Context, userId int64, field string, delta int) error {
	var updateExpr string
	switch field {
	case "following_count":
		updateExpr = "following_count + ?"
	case "follower_count":
		updateExpr = "follower_count + ?"
	case "video_count":
		updateExpr = "video_count + ?"
	case "like_count":
		updateExpr = "like_count + ?"
	default:
		return errors.Errorf("invalid field: %s", field)
	}

	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=?", userId).
		UpdateColumn(field, DB.Raw(updateExpr, delta)).Error; err != nil {
		return errors.Wrapf(err, "Update user %s failed,err: %v", field, err)
	}
	return nil
}

func QueryUser(ctx context.Context, keyword *string, page, pageSize int64) ([]*base.User, int64, error) {
	db := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("deleted_at IS NULL AND status = 1")
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

	var res []*base.User
	for _, u := range userWithPasswords {
		res = append(res, u.convertToBaseUser())
	}

	return res, total, nil
}

func GetUser(ctx context.Context, userId string) (*base.User, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=? AND deleted_at IS NULL", userId).First(&userWithPassword).Error; err != nil {
		logrus.Info(err)
		return nil, errors.Wrapf(err, "GetUser failed,err:%v", err)
	}
	return userWithPassword.convertToBaseUser(), nil
}

// GetUserWithExtended gets user with all extended fields
func GetUserWithExtended(ctx context.Context, userId int64) (*UserWithPassword, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=? AND deleted_at IS NULL", userId).First(&userWithPassword).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserWithExtended failed,err:%v", err)
	}
	return &userWithPassword, nil
}

func UserExist(ctx context.Context, uid string) ([]*base.User, error) {
	var users []*UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id=? AND deleted_at IS NULL", uid).Find(&users).Error; err != nil {
		return nil, errors.Wrapf(err, "User not exist,err:%v", err)
	}

	var result []*base.User
	for _, u := range users {
		result = append(result, u.convertToBaseUser())
	}
	return result, nil
}

func RemoveDuplicate(ctx context.Context, username string) (err error, flag bool) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_name=? AND deleted_at IS NULL", username).Count(&count).Error; err != nil {
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
	if err = DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id = ?", uid).Updates(map[string]interface{}{
		"avatar_url": avatarUrl,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return err
	}
	return nil
}

// UploadBackgroundUrl uploads user profile background image
func UploadBackgroundUrl(ctx context.Context, uid, backgroundUrl string) error {
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("user_id = ?", uid).Updates(map[string]interface{}{
		"background_url": backgroundUrl,
		"updated_at":     time.Now(),
	}).Error; err != nil {
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

// CheckEmailExists checks if email already exists
func CheckEmailExists(ctx context.Context, email string) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("email=? AND deleted_at IS NULL", email).Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "CheckEmailExists failed,err:%v", err)
	}
	return count > 0, nil
}

// CheckPhoneExists checks if phone number already exists
func CheckPhoneExists(ctx context.Context, phone string) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("phone=? AND deleted_at IS NULL", phone).Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "CheckPhoneExists failed,err:%v", err)
	}
	return count > 0, nil
}

// GetUserByEmail gets user by email
func GetUserByEmail(ctx context.Context, email string) (*base.User, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("email=? AND deleted_at IS NULL", email).First(&userWithPassword).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserByEmail failed,err:%v", err)
	}
	return userWithPassword.convertToBaseUser(), nil
}

// GetUserByPhone gets user by phone number
func GetUserByPhone(ctx context.Context, phone string) (*base.User, error) {
	var userWithPassword UserWithPassword
	if err := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("phone=? AND deleted_at IS NULL", phone).First(&userWithPassword).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserByPhone failed,err:%v", err)
	}
	return userWithPassword.convertToBaseUser(), nil
}

// GetUsersBySchool gets users by school_id
func GetUsersBySchool(ctx context.Context, schoolId int64, page, pageSize int64) ([]*base.User, int64, error) {
	db := DB.WithContext(ctx).Model(&UserWithPassword{}).Where("school_id=? AND deleted_at IS NULL AND status = 1", schoolId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUsersBySchool count failed,err:%v", err)
	}

	var users []*UserWithPassword
	if err := db.Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&users).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUsersBySchool query failed,err:%v", err)
	}

	var result []*base.User
	for _, u := range users {
		result = append(result, u.convertToBaseUser())
	}
	return result, total, nil
}
