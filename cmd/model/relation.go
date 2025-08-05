package model

import "time"

// FollowRelation 关注关系实体
type FollowRelation struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`     // 被关注者ID
	FollowerID int64      `json:"follower_id"` // 关注者ID
	Status     int        `json:"status"`      // 1:正常关注 2:特别关注 3:悄悄关注
	Remark     string     `json:"remark"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at" gorm:"index"`
}
