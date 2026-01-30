// Package dto provides Data Transfer Objects for API request/response handling.
// This package centralizes all parameter structures used across different handlers.
package dto

// =============== User DTOs ===============

// UserParam defines user registration parameters.
type UserParam struct {
	UserName string `form:"user_name" json:"username" binding:"required"`
	PassWord string `form:"password" json:"password" binding:"required"`
	Email    string `form:"email" json:"email"`
	Sex      int64  `form:"sex" json:"sex"`
}

// LoginParam defines user login parameters.
type LoginParam struct {
	UserName string `form:"user_name" json:"username"`
	PassWord string `form:"password" json:"password"`
	Email    string `form:"email" json:"email"`
}

// QueryParam defines user query parameters.
type QueryParam struct {
	PageNum  int64  `form:"page_num" json:"page_num"`
	PageSize int64  `form:"page_size" json:"page_size"`
	Keyword  string `form:"keyword" json:"keyword"`
}

// UpdateUserParam defines user update parameters.
type UpdateUserParam struct {
	UserName string `form:"user_name" json:"user_name"`
	PassWord string `form:"password" json:"password"`
	Sex      int64  `form:"sex" json:"sex"`
	Bio      string `form:"bio" json:"bio"`
}

// SendCodeParam defines email code sending parameters.
type SendCodeParam struct {
	Email string `form:"email" json:"email" binding:"required,email"`
}

// VerifyCodeParam defines email code verification parameters.
type VerifyCodeParam struct {
	Email string `form:"email" json:"email" binding:"required,email"`
	Code  string `form:"code" json:"code" binding:"required"`
}

// =============== Video DTOs ===============

// UploadVideoParam defines video upload parameters.
type UploadVideoParam struct {
	ContentType string `json:"content_type" form:"content_type"`
	ObjectName  string `json:"object_name" form:"object_name"`
	BucketName  string `json:"bucket_name" form:"bucket_name"`
	Title       string `json:"title" form:"title" binding:"required"`
	CoverUrl    string `json:"cover_url" form:"cover_url"`
}

// FeedListParam defines video feed parameters.
type FeedListParam struct {
	LastTime string `json:"last_time" form:"last_time"`
}

// VideoFeedListParam defines video feed list parameters.
type VideoFeedListParam struct {
	AuthorId int64 `form:"author_id" json:"author_id"`
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
}

// VideoSearchParam defines video search parameters.
type VideoSearchParam struct {
	Keyword  string `form:"keyword" json:"keyword" binding:"required"`
	PageNum  int64  `form:"page_num" json:"page_num"`
	PageSize int64  `form:"page_size" json:"page_size"`
	FromDate string `form:"from_date" json:"from_date"`
	ToDate   string `form:"to_date" json:"to_date"`
}

// VideoPublishStartParam defines video publish start parameters.
type VideoPublishStartParam struct {
	Title            string `form:"title" json:"title" binding:"required"`
	Description      string `form:"description" json:"description"`
	LabName          string `form:"lab_name" json:"lab_name"`
	Category         string `form:"category" json:"category"`
	Open             int64  `form:"open" json:"open"`
	ChunkTotalNumber int64  `form:"chunk_total_number" json:"chunk_total_number"`
}

// VideoPublishUploadingParam defines video chunk upload parameters.
type VideoPublishUploadingParam struct {
	Uuid        string `form:"uuid" json:"uuid" binding:"required"`
	IsM3U8      bool   `form:"is_m3u8" json:"is_m3u8"`
	FileName    string `form:"filename" json:"filename"`
	ChunkNumber int64  `form:"chunk_number" json:"chunk_number"`
}

// VideoPublishCompleteParam defines video publish complete parameters.
type VideoPublishCompleteParam struct {
	Uuid string `form:"uuid" json:"uuid" binding:"required"`
}

// VideoPublishCancelParam defines video publish cancel parameters.
type VideoPublishCancelParam struct {
	Uuid string `form:"uuid" json:"uuid" binding:"required"`
}

// VideoDeleteParam defines video delete parameters.
type VideoDeleteParam struct {
	VideoId int64 `form:"video_id" json:"video_id" binding:"required"`
}

// =============== Favorite DTOs ===============

// CreateFavoriteParam defines favorite creation parameters.
type CreateFavoriteParam struct {
	UserId      int64  `form:"user_id" json:"user_id"`
	Name        string `form:"name" json:"name" binding:"required"`
	Description string `form:"description" json:"description"`
	CoverUrl    string `form:"cover_url" json:"cover_url"`
}

// GetFavoriteListParam defines favorite list query parameters.
type GetFavoriteListParam struct {
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
}

// AddFavoriteVideoParam defines add video to favorite parameters.
type AddFavoriteVideoParam struct {
	FavoriteId int64 `form:"favorite_id" json:"favorite_id" binding:"required"`
	VideoId    int64 `form:"video_id" json:"video_id" binding:"required"`
}

// DeleteFavoriteParam defines favorite deletion parameters.
type DeleteFavoriteParam struct {
	FavoriteId int64 `form:"favorite_id" json:"favorite_id" binding:"required"`
}

// DeleteVideoFromFavoriteParam defines video removal from favorite parameters.
type DeleteVideoFromFavoriteParam struct {
	FavoriteId int64 `form:"favorite_id" json:"favorite_id" binding:"required"`
	VideoId    int64 `form:"video_id" json:"video_id" binding:"required"`
}

// =============== Comment DTOs ===============

// CreateCommentParam defines comment creation parameters.
type CreateCommentParam struct {
	VideoId   int64  `form:"video_id" json:"video_id" binding:"required"`
	CommentId int64  `form:"comment_id" json:"comment_id"`
	Mode      int64  `form:"mode" json:"mode"`
	Content   string `form:"content" json:"content" binding:"required"`
}

// ListCommentParam defines comment list query parameters.
type ListCommentParam struct {
	VideoId   int64  `form:"video_id" json:"video_id" binding:"required"`
	CommentId int64  `form:"comment_id" json:"comment_id"`
	PageNum   int64  `form:"page_num" json:"page_num"`
	PageSize  int64  `form:"page_size" json:"page_size"`
	SortType  string `form:"sort_type" json:"sort_type"` // "hot" for popular, "latest" for newest
}

// DeleteCommentParam defines comment deletion parameters.
type DeleteCommentParam struct {
	VideoId    int64 `form:"video_id" json:"video_id" binding:"required"`
	CommentId  int64 `form:"comment_id" json:"comment_id" binding:"required"`
	FromUserId int64 `form:"from_user_id" json:"from_user_id"`
}

// =============== Like DTOs ===============

// LikeParam defines like action parameters.
type LikeParam struct {
	VideoId    int64  `form:"video_id" json:"video_id"`
	CommentId  int64  `form:"comment_id" json:"comment_id"`
	ActionType string `form:"action_type" json:"action_type" binding:"required"`
}

// LikeListParam defines like list query parameters.
type LikeListParam struct {
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
}

// =============== Relation DTOs ===============

// RelationParam defines relation action parameters.
type RelationParam struct {
	ActionType int64 `form:"action_type" json:"action_type" binding:"required"`
	ToUserId   int64 `form:"to_user_id" json:"to_user_id" binding:"required"`
	UserId     int64 `form:"user_id" json:"user_id"`
}

// RelationPageParam defines relation list pagination parameters.
type RelationPageParam struct {
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
	UserId   int64 `form:"user_id" json:"user_id"`
}

// =============== Share DTOs ===============

// SharedVideoParam defines video sharing parameters.
type SharedVideoParam struct {
	VideoId  int64 `form:"video_id" json:"video_id" binding:"required"`
	ToUserId int64 `form:"to_user_id" json:"to_user_id" binding:"required"`
}

// =============== Common Pagination DTOs ===============

// PageParam defines common pagination parameters.
type PageParam struct {
	PageNum  int64 `form:"page_num" json:"page_num"`
	PageSize int64 `form:"page_size" json:"page_size"`
}

// IDParam defines common ID parameter.
type IDParam struct {
	ID int64 `form:"id" json:"id" binding:"required"`
}
