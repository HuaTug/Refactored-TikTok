package db

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// CommentIndex 评论索引表结构
type CommentIndex struct {
	CommentId  int64     `gorm:"primaryKey;column:comment_id" json:"comment_id"`
	VideoId    int64     `gorm:"column:video_id;not null;index:idx_video_id" json:"video_id"`
	RootId     int64     `gorm:"column:root_id;not null;index:idx_root_id" json:"root_id"`
	UserId     int64     `gorm:"column:user_id;not null;index:idx_user_id" json:"user_id"`
	DbShard    int8      `gorm:"column:db_shard;not null" json:"db_shard"`
	TableShard int8      `gorm:"column:table_shard;not null" json:"table_shard"`
	CreatedAt  time.Time `gorm:"column:created_at;not null" json:"created_at"`
}

func (CommentIndex) TableName() string {
	return "comment_id_index"
}

// CommentIndexManager 评论索引管理器
type CommentIndexManager struct {
	db *gorm.DB
}

func NewCommentIndexManager(db *gorm.DB) *CommentIndexManager {
	return &CommentIndexManager{db: db}
}

// CreateCommentIndex 创建评论索引
func (cim *CommentIndexManager) CreateCommentIndex(ctx context.Context, index *CommentIndex) error {
	return cim.db.WithContext(ctx).Create(index).Error
}

// GetCommentIndex 根据评论ID获取索引信息
func (cim *CommentIndexManager) GetCommentIndex(ctx context.Context, commentID int64) (*CommentIndex, error) {
	var index CommentIndex
	err := cim.db.WithContext(ctx).Where("comment_id = ?", commentID).First(&index).Error
	if err != nil {
		return nil, err
	}
	return &index, nil
}

// GetCommentsByRootId 根据根评论ID获取整个评论树的索引
func (cim *CommentIndexManager) GetCommentsByRootId(ctx context.Context, rootId int64, pageNum, pageSize int64) ([]CommentIndex, error) {
	var indexes []CommentIndex
	offset := (pageNum - 1) * pageSize

	err := cim.db.WithContext(ctx).
		Where("root_id = ?", rootId).
		Order("created_at ASC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&indexes).Error

	return indexes, err
}

// GetVideoCommentIndexes 根据视频ID获取评论索引（用于视频评论列表）
func (cim *CommentIndexManager) GetVideoCommentIndexes(ctx context.Context, videoId int64, pageNum, pageSize int64) ([]CommentIndex, error) {
	var indexes []CommentIndex
	offset := (pageNum - 1) * pageSize

	err := cim.db.WithContext(ctx).
		Where("video_id = ?", videoId).
		Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int(offset)).
		Find(&indexes).Error

	return indexes, err
}

// DeleteCommentIndex 删除评论索引
func (cim *CommentIndexManager) DeleteCommentIndex(ctx context.Context, commentID int64) error {
	return cim.db.WithContext(ctx).Where("comment_id = ?", commentID).Delete(&CommentIndex{}).Error
}

// BatchCreateCommentIndexes 批量创建评论索引
func (cim *CommentIndexManager) BatchCreateCommentIndexes(ctx context.Context, indexes []CommentIndex) error {
	if len(indexes) == 0 {
		return nil
	}
	return cim.db.WithContext(ctx).CreateInBatches(indexes, 100).Error
}

// UpdateCommentIndex 更新评论索引
func (cim *CommentIndexManager) UpdateCommentIndex(ctx context.Context, commentID int64, updates map[string]interface{}) error {
	return cim.db.WithContext(ctx).Model(&CommentIndex{}).Where("comment_id = ?", commentID).Updates(updates).Error
}

// GetShardInfo 根据评论ID快速获取分片信息
func (cim *CommentIndexManager) GetShardInfo(ctx context.Context, commentID int64) (dbShard, tableShard int8, err error) {
	var index CommentIndex
	err = cim.db.WithContext(ctx).
		Select("db_shard, table_shard").
		Where("comment_id = ?", commentID).
		First(&index).Error

	if err != nil {
		return 0, 0, err
	}

	return index.DbShard, index.TableShard, nil
}
