package model

import "time"

// Comment represents the comments table
type Comment struct {
	CommentId        int64      `json:"comment_id" gorm:"column:comment_id;primaryKey"`
	UserId           int64      `json:"user_id" gorm:"column:user_id"`
	VideoId          int64      `json:"video_id" gorm:"column:video_id"`
	ParentId         int64      `json:"parent_id" gorm:"column:parent_id;default:-1"`
	LikeCount        int64      `json:"like_count" gorm:"column:like_count;default:0"`
	ChildCount       int64      `json:"child_count" gorm:"column:child_count;default:0"`
	Content          string     `json:"content" gorm:"column:content"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt        *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
	ReplyToCommentId int64      `json:"reply_to_comment_id" gorm:"column:reply_to_comment_id;default:0"` // target comment id for reply
}

// CommentLike represents comment likes table
type CommentLike struct {
	CommentLikesId int64      `json:"comment_likes_id" gorm:"column:comment_likes_id;primaryKey;autoIncrement"`
	UserId         int64      `json:"user_id" gorm:"column:user_id"`
	CommentId      int64      `json:"comment_id" gorm:"column:comment_id"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	DeletedAt      *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

// VideoLike represents video likes table
type VideoLike struct {
	VideoLikesId int64      `json:"video_likes_id" gorm:"column:video_likes_id;primaryKey;autoIncrement"`
	UserId       int64      `json:"user_id" gorm:"column:user_id"`
	VideoId      int64      `json:"video_id" gorm:"column:video_id"`
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at"`
	DeletedAt    *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (VideoLike) TableName() string {
	return "video_likes"
}
