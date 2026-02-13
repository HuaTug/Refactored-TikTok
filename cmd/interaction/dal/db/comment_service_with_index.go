package db

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/internal/model"

	"gorm.io/gorm"
)

// CommentServiceWithIndex 带索引的评论服务
type CommentServiceWithIndex struct {
	shardingManager *ShardingManager
	indexManager    *CommentIndexManager
}

func NewCommentServiceWithIndex(shardingManager *ShardingManager, indexManager *CommentIndexManager) *CommentServiceWithIndex {
	return &CommentServiceWithIndex{
		shardingManager: shardingManager,
		indexManager:    indexManager,
	}
}

// CreateCommentWithIndex 创建评论并维护索引
func (cs *CommentServiceWithIndex) CreateCommentWithIndex(ctx context.Context, comment *model.Comment) error {
	// 1. 计算分片信息
	dbIndex := int(comment.VideoId % 4)
	tableIndex := int((comment.VideoId / 4) % 4)

	// 2. 确定根评论ID
	rootId := comment.CommentId // 默认为根评论
	if comment.ParentId != 0 {
		// 如果是子评论，获取父评论的根ID
		parentIndex, err := cs.indexManager.GetCommentIndex(ctx, comment.ParentId)
		if err != nil {
			return fmt.Errorf("failed to get parent comment index: %w", err)
		}
		rootId = parentIndex.RootId
	}

	// 3. 在对应分片创建评论
	err := cs.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		now := time.Now()
		comment.CreatedAt = now
		comment.UpdatedAt = now
		return db.WithContext(ctx).Table(tableName).Create(comment).Error
	})

	if err != nil {
		return fmt.Errorf("failed to create comment in shard: %w", err)
	}

	// 4. 创建索引记录
	commentIndex := &CommentIndex{
		CommentId:  comment.CommentId,
		VideoId:    comment.VideoId,
		RootId:     rootId,
		UserId:     comment.UserId,
		DbShard:    int8(dbIndex),
		TableShard: int8(tableIndex),
		CreatedAt:  time.Now(),
	}

	err = cs.indexManager.CreateCommentIndex(ctx, commentIndex)
	if err != nil {
		// 索引创建失败，但评论已创建，记录错误日志进行后续处理
		return fmt.Errorf("comment created but index creation failed: %w", err)
	}

	return nil
}

// GetCommentChildrenWithIndex 使用索引查询子评论
func (cs *CommentServiceWithIndex) GetCommentChildrenWithIndex(ctx context.Context, parentCommentID int64, pageNum, pageSize int64) ([]int64, error) {
	// 1. 获取父评论的根ID
	parentIndex, err := cs.indexManager.GetCommentIndex(ctx, parentCommentID)
	if err != nil {
		return nil, fmt.Errorf("parent comment not found: %w", err)
	}

	// 2. 查找同一评论树下的所有评论
	indexes, err := cs.indexManager.GetCommentsByRootId(ctx, parentIndex.RootId, pageNum, pageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get comment tree indexes: %w", err)
	}

	// 3. 从具体分片验证父子关系并获取评论ID
	var commentIDs []int64
	for _, index := range indexes {
		if index.CommentId == parentCommentID {
			continue // 跳过父评论本身
		}

		// 直接定位到具体分片验证
		dbKey := fmt.Sprintf("db_%d", index.DbShard)
		tableName := fmt.Sprintf("comments_%d", index.TableShard)

		dbs := cs.shardingManager.GetAllDatabases()
		db := dbs[dbKey]
		if db == nil {
			continue
		}
		var count int64
		err := db.WithContext(ctx).Table(tableName).
			Where("comment_id = ? AND parent_id = ?", index.CommentId, parentCommentID).
			Count(&count).Error

		if err == nil && count > 0 {
			commentIDs = append(commentIDs, index.CommentId)
		}
	}

	return commentIDs, nil
}

// GetCommentByIDWithIndex 使用索引快速查询评论
func (cs *CommentServiceWithIndex) GetCommentByIDWithIndex(ctx context.Context, commentID int64) (*model.Comment, error) {
	// 1. 从索引表获取分片信息
	dbShard, tableShard, err := cs.indexManager.GetShardInfo(ctx, commentID)
	if err != nil {
		return nil, fmt.Errorf("comment index not found: %w", err)
	}

	// 2. 直接从对应分片查询
	dbKey := fmt.Sprintf("db_%d", dbShard)
	tableName := fmt.Sprintf("comments_%d", tableShard)

	dbs := cs.shardingManager.GetAllDatabases()
	db := dbs[dbKey]
	if db == nil {
		return nil, fmt.Errorf("database %s not found", dbKey)
	}
	var comment model.Comment
	err = db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).First(&comment).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get comment from shard: %w", err)
	}

	return &comment, nil
}

// DeleteCommentWithIndex 删除评论并清理索引
func (cs *CommentServiceWithIndex) DeleteCommentWithIndex(ctx context.Context, commentID int64) error {
	// 1. 先获取分片信息
	dbShard, tableShard, err := cs.indexManager.GetShardInfo(ctx, commentID)
	if err != nil {
		return fmt.Errorf("comment index not found: %w", err)
	}

	// 2. 从分片删除评论
	dbKey := fmt.Sprintf("db_%d", dbShard)
	tableName := fmt.Sprintf("comments_%d", tableShard)

	dbs := cs.shardingManager.GetAllDatabases()
	db := dbs[dbKey]
	if db == nil {
		return fmt.Errorf("database %s not found", dbKey)
	}
	err = db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).Delete(&model.Comment{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete comment from shard: %w", err)
	}

	// 3. 删除索引记录
	err = cs.indexManager.DeleteCommentIndex(ctx, commentID)
	if err != nil {
		// 记录错误日志，但不影响主流程
		return fmt.Errorf("comment deleted but index cleanup failed: %w", err)
	}

	return nil
}

// GetVideoCommentsWithIndex 查询视频评论（可选择使用索引优化）
func (cs *CommentServiceWithIndex) GetVideoCommentsWithIndex(ctx context.Context, videoID int64, pageNum, pageSize int64) ([]int64, error) {
	// 方案1: 直接使用原有分片查询（推荐，性能最优）
	var commentIDs []int64
	err := cs.shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		offset := (pageNum - 1) * pageSize
		return db.WithContext(ctx).Table(tableName).
			Select("comment_id").
			Where("video_id = ?", videoID).
			Order("created_at DESC").
			Limit(int(pageSize)).
			Offset(int(offset)).
			Pluck("comment_id", &commentIDs).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comments: %w", err)
	}

	return commentIDs, nil

	// 方案2: 使用索引表查询（备选方案，可用于验证数据一致性）
	/*
		indexes, err := cs.indexManager.GetVideoCommentIndexes(ctx, videoID, pageNum, pageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to get video comment indexes: %w", err)
		}

		commentIDs := make([]int64, 0, len(indexes))
		for _, index := range indexes {
			commentIDs = append(commentIDs, index.CommentId)
		}

		return commentIDs, nil
	*/
}
