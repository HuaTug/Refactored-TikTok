package model

import "time"

// User represents the users table with all fields
type User struct {
	UserId         int64      `json:"user_id" gorm:"column:user_id;primaryKey"`
	UserName       string     `json:"user_name" gorm:"column:user_name"`
	Password       string     `json:"-" gorm:"column:password"`
	Phone          *string    `json:"phone" gorm:"column:phone"`
	Email          string     `json:"email" gorm:"column:email"`
	Sex            int64      `json:"sex" gorm:"column:sex"`
	AvatarUrl      string     `json:"avatar_url" gorm:"column:avatar_url"`
	BackgroundUrl  *string    `json:"background_url" gorm:"column:background_url"`
	Bio            string     `json:"bio" gorm:"column:bio;default:''"`
	Birthday       *time.Time `json:"birthday" gorm:"column:birthday"`
	Location       *string    `json:"location" gorm:"column:location"`
	SchoolId       *int64     `json:"school_id" gorm:"column:school_id"`
	FollowingCount uint       `json:"following_count" gorm:"column:following_count;default:0"`
	FollowerCount  uint       `json:"follower_count" gorm:"column:follower_count;default:0"`
	LikeCount      uint64     `json:"like_count" gorm:"column:like_count;default:0"`
	VideoCount     uint       `json:"video_count" gorm:"column:video_count;default:0"`
	Status         int8       `json:"status" gorm:"column:status;default:1"` // 1:normal 2:muted 3:banned
	LastLoginAt    *time.Time `json:"last_login_at" gorm:"column:last_login_at"`
	LastLoginIp    *string    `json:"last_login_ip" gorm:"column:last_login_ip"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt      *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (User) TableName() string {
	return "users"
}

// UserBehavior represents user behavior tracking
type UserBehavior struct {
	UserBehaviorId int64     `json:"user_behavior_id" gorm:"column:user_behavior_id;primaryKey"`
	UserId         int64     `json:"user_id" gorm:"column:user_id"`
	VideoId        int64     `json:"video_id" gorm:"column:video_id"`
	BehaviorType   string    `json:"behavior_type" gorm:"column:behavior_type"` // 'view' 'like' 'share' 'comment'
	BehaviorTime   time.Time `json:"behavior_time" gorm:"column:behavior_time"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at"`
}

func (UserBehavior) TableName() string {
	return "user_behaviors"
}

// UserPreference represents user preferences
type UserPreference struct {
	Id                  int64     `json:"id" gorm:"column:id;primaryKey"`
	UserId              int64     `json:"user_id" gorm:"column:user_id;uniqueIndex"`
	LabelNames          string    `json:"label_names" gorm:"column:label_names"`
	PreferredCategories string    `json:"preferred_categories" gorm:"column:preferred_categories;default:''"`
	Language            string    `json:"language" gorm:"column:language;default:'zh-CN'"`
	PushEnabled         int8      `json:"push_enabled" gorm:"column:push_enabled;default:1"`
	PrivateAccount      int8      `json:"private_account" gorm:"column:private_account;default:0"`
	CreatedAt           time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (UserPreference) TableName() string {
	return "user_perferences"
}

// UserCounter represents user counters table for high concurrency
type UserCounter struct {
	UserId         int64     `json:"user_id" gorm:"column:user_id;primaryKey"`
	FollowingCount uint      `json:"following_count" gorm:"column:following_count;default:0"`
	FollowerCount  uint      `json:"follower_count" gorm:"column:follower_count;default:0"`
	LikeCount      uint64    `json:"like_count" gorm:"column:like_count;default:0"`
	VideoCount     uint      `json:"video_count" gorm:"column:video_count;default:0"`
	FavoriteCount  uint      `json:"favorite_count" gorm:"column:favorite_count;default:0"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (UserCounter) TableName() string {
	return "user_counters"
}
