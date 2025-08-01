package model

type Comment struct {
	CommentId        int64
	UserId           int64
	VideoId          int64
	ParentId         int64
	LikeCount        int64 // 点赞数
	ChildCount       int64 // 子评论数
	Content          string
	CreatedAt        string
	UpdatedAt        string
	DeletedAt        string
	ReplyToCommentId int64 // 实际回复目标评论ID，用于记录互动关系
}

type CommentLike struct {
	CommentLikesId int64
	UserId         int64
	CommentId      int64
	CreatedAt      string
	DeletedAt      string
}

type VideoLike struct {
	VideoLikesId int64
	UserId       int64
	VideoId      int64
	CreatedAt    string
	DeletedAt    string
}
