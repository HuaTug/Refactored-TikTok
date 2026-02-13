package model

import "time"

// Video represents the videos table with all fields
type Video struct {
	VideoId        int64      `json:"video_id" gorm:"column:video_id;primaryKey"`
	UserId         int64      `json:"user_id" gorm:"column:user_id"`
	VideoUrl       string     `json:"video_url" gorm:"column:video_url"`
	CoverUrl       string     `json:"cover_url" gorm:"column:cover_url"`
	Title          string     `json:"title" gorm:"column:title"`
	Description    string     `json:"description" gorm:"column:description"`
	Duration       uint       `json:"duration" gorm:"column:duration;default:0"`   // video duration in seconds
	Width          uint       `json:"width" gorm:"column:width;default:0"`         // video width
	Height         uint       `json:"height" gorm:"column:height;default:0"`       // video height
	FileSize       uint64     `json:"file_size" gorm:"column:file_size;default:0"` // file size in bytes
	VisitCount     uint64     `json:"visit_count" gorm:"column:visit_count;default:0"`
	ShareCount     uint64     `json:"share_count" gorm:"column:share_count;default:0"`
	LikesCount     uint64     `json:"likes_count" gorm:"column:likes_count;default:0"`
	FavoritesCount uint64     `json:"favorites_count" gorm:"column:favorites_count;default:0"`
	CommentCount   uint64     `json:"comment_count" gorm:"column:comment_count;default:0"`
	HistoryCount   uint64     `json:"history_count" gorm:"column:history_count;default:0"`
	Open           int8       `json:"open" gorm:"column:open;default:0"`                 // 0:private 1:public 2:friends only
	AuditStatus    int8       `json:"audit_status" gorm:"column:audit_status;default:0"` // 0:unreviewed 1:approved 2:rejected
	SchoolId       *int64     `json:"school_id" gorm:"column:school_id"`                 // school exclusive video
	Location       *string    `json:"location" gorm:"column:location"`                   // publish location
	AllowComment   int8       `json:"allow_comment" gorm:"column:allow_comment;default:1"`
	AllowDuet      int8       `json:"allow_duet" gorm:"column:allow_duet;default:1"`
	AllowDownload  int8       `json:"allow_download" gorm:"column:allow_download;default:1"`
	LabelNames     string     `json:"label_names" gorm:"column:label_names;default:''"`
	Category       string     `json:"category" gorm:"column:category;default:''"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt      *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (Video) TableName() string {
	return "videos"
}

// VideoCounter represents video counters table for high concurrency
type VideoCounter struct {
	VideoId       int64     `json:"video_id" gorm:"column:video_id;primaryKey"`
	VisitCount    uint64    `json:"visit_count" gorm:"column:visit_count;default:0"`
	LikeCount     uint64    `json:"like_count" gorm:"column:like_count;default:0"`
	CommentCount  uint64    `json:"comment_count" gorm:"column:comment_count;default:0"`
	ShareCount    uint64    `json:"share_count" gorm:"column:share_count;default:0"`
	FavoriteCount uint64    `json:"favorite_count" gorm:"column:favorite_count;default:0"`
	DownloadCount uint64    `json:"download_count" gorm:"column:download_count;default:0"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (VideoCounter) TableName() string {
	return "video_counters"
}

// Favorite represents favorites (collection folders) table
type Favorite struct {
	FavoriteId  int64      `json:"favorite_id" gorm:"column:favorite_id;primaryKey"`
	UserId      int64      `json:"user_id" gorm:"column:user_id"`
	Name        string     `json:"name" gorm:"column:name"`
	Description string     `json:"description" gorm:"column:description;default:''"`
	CoverUrl    string     `json:"cover_url" gorm:"column:cover_url;default:''"`
	VideoCount  uint       `json:"video_count" gorm:"column:video_count;default:0"`
	IsPublic    int8       `json:"is_public" gorm:"column:is_public;default:0"` // 0:private 1:public
	CreatedAt   time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt   *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (Favorite) TableName() string {
	return "favorites"
}

// FavoritesVideos represents the videos in favorites table
type FavoritesVideos struct {
	FavoriteVideoId int64     `json:"favorite_video_id" gorm:"column:favorite_video_id;primaryKey"`
	FavoriteId      int64     `json:"favorite_id" gorm:"column:favorite_id"`
	VideoId         int64     `json:"video_id" gorm:"column:video_id"`
	UserId          int64     `json:"user_id" gorm:"column:user_id"`
	CreatedAt       time.Time `json:"created_at" gorm:"column:created_at"`
}

func (FavoritesVideos) TableName() string {
	return "favorites_videos"
}

// VideoShare represents video sharing records
type VideoShare struct {
	VideoShareId int64      `json:"video_share_id" gorm:"column:video_share_id;primaryKey"`
	UserId       int64      `json:"user_id" gorm:"column:user_id"`
	VideoId      int64      `json:"video_id" gorm:"column:video_id"`
	ToUserId     int64      `json:"to_user_id" gorm:"column:to_user_id"`
	ShareType    int8       `json:"share_type" gorm:"column:share_type;default:1"` // 1:private 2:moments 3:external
	Platform     *string    `json:"platform" gorm:"column:platform"`               // wechat/qq/weibo
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at"`
	DeletedAt    *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (VideoShare) TableName() string {
	return "video_shares"
}

// UserVideoWatchHistory represents user video watch history
type UserVideoWatchHistory struct {
	UserVideoWatchHistoryId int64      `json:"user_video_watch_history_id" gorm:"column:user_video_watch_history_id;primaryKey"`
	UserId                  int64      `json:"user_id" gorm:"column:user_id"`
	VideoId                 int64      `json:"video_id" gorm:"column:video_id"`
	WatchDuration           uint       `json:"watch_duration" gorm:"column:watch_duration;default:0"`   // watch duration in seconds
	CompletionRate          float64    `json:"completion_rate" gorm:"column:completion_rate;default:0"` // completion rate percentage
	WatchTime               time.Time  `json:"watch_time" gorm:"column:watch_time"`
	DeletedAt               *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (UserVideoWatchHistory) TableName() string {
	return "user_video_watch_histories"
}
