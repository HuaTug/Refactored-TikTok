package model

import "time"

// FollowRelation represents the follow relationship between users
type FollowRelation struct {
	ID         int64      `json:"id" gorm:"column:id;primaryKey"`
	UserID     int64      `json:"user_id" gorm:"column:user_id"`         // followed user ID
	FollowerID int64      `json:"follower_id" gorm:"column:follower_id"` // follower user ID
	Status     int        `json:"status" gorm:"column:status;default:1"` // 1:normal 2:special 3:quiet
	Remark     string     `json:"remark" gorm:"column:remark;default:''"`
	CreatedAt  time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt  *time.Time `json:"deleted_at" gorm:"column:deleted_at;index"`
}

// Follow status constants
const (
	FollowStatusNormal  = 1 // normal follow
	FollowStatusSpecial = 2 // special attention
	FollowStatusQuiet   = 3 // quiet follow
)

// UserRelationStats represents user relation statistics
type UserRelationStats struct {
	UserID            int64     `json:"user_id" gorm:"column:user_id;primaryKey"`
	FollowingCount    int       `json:"following_count" gorm:"column:following_count;default:0"`
	FollowerCount     int       `json:"follower_count" gorm:"column:follower_count;default:0"`
	FriendCount       int       `json:"friend_count" gorm:"column:friend_count;default:0"` // mutual follow
	MutualFollowCount int       `json:"mutual_follow_count" gorm:"column:mutual_follow_count;default:0"`
	CreatedAt         time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (UserRelationStats) TableName() string {
	return "user_relation_stats"
}

// GlobalUserRelationIndex represents the global relation index for sharding
type GlobalUserRelationIndex struct {
	ID           int64     `json:"id" gorm:"column:id;primaryKey"`
	UserID       int64     `json:"user_id" gorm:"column:user_id"`
	RelationID   int64     `json:"relation_id" gorm:"column:relation_id"`
	TargetUserID int64     `json:"target_user_id" gorm:"column:target_user_id"`
	RelationType string    `json:"relation_type" gorm:"column:relation_type"` // follow/friend/mutual
	DbIndex      int8      `json:"db_index" gorm:"column:db_index"`           // shard db index 0-3
	TableIndex   int8      `json:"table_index" gorm:"column:table_index"`     // shard table index 0-3
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
}

func (GlobalUserRelationIndex) TableName() string {
	return "global_user_relation_index"
}

// Relation type constants
const (
	RelationTypeFollow = "follow"
	RelationTypeFriend = "friend"
	RelationTypeMutual = "mutual"
)
