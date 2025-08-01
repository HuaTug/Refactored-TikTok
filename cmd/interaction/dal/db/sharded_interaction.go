package db

import (
	"context"
	"fmt"
	"sort"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/cache"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/database"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ShardedCommentDB 分库分表评论数据库操作
type ShardedCommentDB struct {
	shardingManager *database.ShardingManager
	cacheManager    *cache.CommentCacheManager
}

// NewShardedCommentDB 创建分库分表评论数据库操作实例
func NewShardedCommentDB(shardingManager *database.ShardingManager, cacheManager *cache.CommentCacheManager) *ShardedCommentDB {
	return &ShardedCommentDB{
		shardingManager: shardingManager,
		cacheManager:    cacheManager,
	}
}

// CreateCommentWithSharding 在分片中创建评论
func (sdb *ShardedCommentDB) CreateCommentWithSharding(ctx context.Context, comment *model.Comment) error {
	// 生成唯一的评论ID
	if comment.CommentId == 0 {
		uuid := uuid.New().ID()
		comment.CommentId = int64(uuid)
	}

	return sdb.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Create(comment).Error
	})
}

// CreateCommentWithTransaction 在分片中使用事务创建评论
func (sdb *ShardedCommentDB) CreateCommentWithTransaction(ctx context.Context, comment *model.Comment) error {
	// 生成唯一的评论ID
	if comment.CommentId == 0 {
		uuid := uuid.New().ID()
		comment.CommentId = int64(uuid)
	}

	return sdb.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// 创建评论
			if err := tx.Table(tableName).Create(comment).Error; err != nil {
				return err
			}

			// 更新父评论的子评论数
			if comment.ParentId != -1 {
				if err := tx.Table(tableName).
					Where("comment_id = ?", comment.ParentId).
					UpdateColumn("child_count", gorm.Expr("child_count + ?", 1)).Error; err != nil {
					return err
				}
			}

			// 异步更新缓存
			go func() {
				if sdb.cacheManager != nil {
					// 清除相关缓存
					sdb.cacheManager.InvalidateVideoCommentCache(context.Background(), comment.VideoId)
					// 增加视频评论计数
					sdb.cacheManager.IncrementVideoCommentCount(context.Background(), comment.VideoId, 1)
				}
			}()

			return nil
		})
	})
}

// GetVideoCommentListByPartWithSharding 分片获取视频评论列表
func (sdb *ShardedCommentDB) GetVideoCommentListByPartWithSharding(ctx context.Context, videoId, pagenum, pagesize int64, sortType string) (*[]int64, error) {
	// 先尝试从缓存获取
	if sdb.cacheManager != nil {
		cachedList, err := sdb.cacheManager.GetCachedCommentList(ctx, videoId, sortType, int(pagenum))
		if err == nil && cachedList != nil {
			return &cachedList, nil
		}
	}

	var list []int64
	err := sdb.shardingManager.ExecuteInShard(ctx, videoId, false, func(db *gorm.DB, tableName string) error {
		query := db.Table(tableName).Where("video_id = ? AND deleted_at = ''", videoId)

		switch sortType {
		case "latest":
			query = query.Order("created_at DESC")
		case "hot":
			// 对于热门排序，我们需要获取更多数据然后在应用层排序
			extendedSize := pagesize * 3
			if extendedSize > 100 {
				extendedSize = 100
			}

			var comments []model.Comment
			if err := query.Limit(int(extendedSize)).Find(&comments).Error; err != nil {
				return err
			}

			// 计算热度分数并排序
			commentScores := make([]CommentScore, len(comments))
			for i, comment := range comments {
				score := sdb.calculateHotScore(ctx, &comment)
				commentScores[i] = CommentScore{
					CommentID: comment.CommentId,
					Score:     score,
				}
			}

			// 按分数排序
			sort.Slice(commentScores, func(i, j int) bool {
				return commentScores[i].Score > commentScores[j].Score
			})

			// 分页
			start := int(pagenum-1) * int(pagesize)
			end := start + int(pagesize)
			if start >= len(commentScores) {
				list = []int64{}
				return nil
			}
			if end > len(commentScores) {
				end = len(commentScores)
			}

			for i := start; i < end; i++ {
				list = append(list, commentScores[i].CommentID)
			}
			return nil
		default:
			query = query.Order("created_at DESC")
		}

		return query.Select("comment_id").Limit(int(pagesize)).Offset(int(pagenum-1) * int(pagesize)).Scan(&list).Error
	})

	if err != nil {
		return nil, err
	}

	// 缓存结果
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.CacheCommentList(context.Background(), videoId, sortType, int(pagenum), list)
		}()
	}

	return &list, nil
}

// CommentScore 评论分数结构
type CommentScore struct {
	CommentID int64
	Score     float64
}

// calculateHotScore 计算评论热度分数
func (sdb *ShardedCommentDB) calculateHotScore(ctx context.Context, comment *model.Comment) float64 {
	// 先尝试从缓存获取
	if sdb.cacheManager != nil {
		if score, err := sdb.cacheManager.GetCommentHotScore(ctx, comment.CommentId); err == nil && score > 0 {
			return score
		}
	}

	// 计算热度分数：点赞数权重70%，时间权重30%
	likeCount := float64(comment.LikeCount)

	// 时间衰减因子（越新的评论分数越高）
	createdTime, _ := time.Parse(constants.DataFormate, comment.CreatedAt)
	hoursSinceCreated := time.Since(createdTime).Hours()
	timeScore := 1.0 / (1.0 + hoursSinceCreated/24.0) // 24小时衰减

	// 综合分数
	score := likeCount*0.7 + timeScore*100*0.3

	// 缓存分数
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.CacheCommentHotScore(context.Background(), comment.CommentId, score)
		}()
	}

	return score
}

// GetCommentInfoWithSharding 分片获取评论信息
func (sdb *ShardedCommentDB) GetCommentInfoWithSharding(ctx context.Context, commentId int64) (*model.Comment, error) {
	// 先尝试从缓存获取
	if sdb.cacheManager != nil {
		if cachedComment, err := sdb.cacheManager.GetCachedCommentDetail(ctx, commentId); err == nil && cachedComment != nil {
			return cachedComment, nil
		}
	}

	// 需要先获取评论的视频ID来确定分片
	// 这里我们需要一个全局的评论ID到视频ID的映射表，或者在所有分片中查找
	// 为了简化，我们假设有一个全局的映射表或者使用一致性哈希

	// 临时方案：在所有分片中查找（生产环境中应该有更好的方案）
	var comment *model.Comment
	var foundErr error

	for i := 0; i < sdb.shardingManager.GetDatabaseCount(); i++ {
		for j := 0; j < sdb.shardingManager.GetTableCount(); j++ {
			db := sdb.shardingManager.GetMasterDBByIndex(i)
			tableName := fmt.Sprintf("comments_%d", j)

			var tempComment model.Comment
			err := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentId).First(&tempComment).Error
			if err == nil {
				comment = &tempComment
				foundErr = nil
				break
			} else if err != gorm.ErrRecordNotFound {
				foundErr = err
			}
		}
		if comment != nil {
			break
		}
	}

	if comment == nil {
		if foundErr != nil {
			return nil, foundErr
		}
		return nil, gorm.ErrRecordNotFound
	}

	// 缓存结果
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.CacheCommentDetail(context.Background(), comment)
		}()
	}

	return comment, nil
}

// BatchGetCommentInfoWithSharding 批量获取评论信息
func (sdb *ShardedCommentDB) BatchGetCommentInfoWithSharding(ctx context.Context, commentIds []int64) (map[int64]*model.Comment, error) {
	if len(commentIds) == 0 {
		return make(map[int64]*model.Comment), nil
	}

	result := make(map[int64]*model.Comment)

	// 先尝试从缓存批量获取
	if sdb.cacheManager != nil {
		cachedComments, err := sdb.cacheManager.BatchGetCommentDetails(ctx, commentIds)
		if err == nil {
			for commentId, comment := range cachedComments {
				result[commentId] = comment
			}
		}
	}

	// 找出缓存中没有的评论ID
	missingIds := make([]int64, 0)
	for _, commentId := range commentIds {
		if _, exists := result[commentId]; !exists {
			missingIds = append(missingIds, commentId)
		}
	}

	if len(missingIds) == 0 {
		return result, nil
	}

	// 从数据库获取缺失的评论
	// 这里需要在所有分片中查找，实际生产环境中应该有更好的索引策略
	for i := 0; i < sdb.shardingManager.GetDatabaseCount(); i++ {
		for j := 0; j < sdb.shardingManager.GetTableCount(); j++ {
			db := sdb.shardingManager.GetSlaveDBByIndex(i)
			tableName := fmt.Sprintf("comments_%d", j)

			var comments []model.Comment
			err := db.WithContext(ctx).Table(tableName).Where("comment_id IN ?", missingIds).Find(&comments).Error
			if err != nil && err != gorm.ErrRecordNotFound {
				hlog.Warnf("Failed to query comments from %s: %v", tableName, err)
				continue
			}

			for _, comment := range comments {
				result[comment.CommentId] = &comment
			}
		}
	}

	// 缓存新获取的评论
	if sdb.cacheManager != nil && len(result) > len(commentIds)-len(missingIds) {
		newComments := make([]*model.Comment, 0)
		for _, commentId := range missingIds {
			if comment, exists := result[commentId]; exists {
				newComments = append(newComments, comment)
			}
		}
		if len(newComments) > 0 {
			go func() {
				sdb.cacheManager.BatchCacheCommentDetails(context.Background(), newComments)
			}()
		}
	}

	return result, nil
}

// GetVideoCommentCountWithSharding 分片获取视频评论总数
func (sdb *ShardedCommentDB) GetVideoCommentCountWithSharding(ctx context.Context, videoId int64) (int64, error) {
	// 先尝试从缓存获取
	if sdb.cacheManager != nil {
		if count, err := sdb.cacheManager.GetVideoCommentCount(ctx, videoId); err == nil && count >= 0 {
			return count, nil
		}
	}

	var count int64
	err := sdb.shardingManager.ExecuteInShard(ctx, videoId, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("video_id = ? AND deleted_at = ''", videoId).Count(&count).Error
	})

	if err != nil {
		return 0, err
	}

	// 缓存结果
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.SetVideoCommentCount(context.Background(), videoId, count)
		}()
	}

	return count, nil
}

// DeleteCommentWithSharding 分片删除评论
func (sdb *ShardedCommentDB) DeleteCommentWithSharding(ctx context.Context, commentId int64, videoId int64) error {
	err := sdb.shardingManager.ExecuteInShard(ctx, videoId, true, func(db *gorm.DB, tableName string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// 软删除评论
			now := time.Now().Format(constants.DataFormate)
			if err := tx.Table(tableName).Where("comment_id = ?", commentId).Update("deleted_at", now).Error; err != nil {
				return err
			}

			// 获取评论信息以更新父评论计数
			var comment model.Comment
			if err := tx.Table(tableName).Where("comment_id = ?", commentId).First(&comment).Error; err != nil {
				return err
			}

			// 更新父评论的子评论数
			if comment.ParentId != -1 {
				if err := tx.Table(tableName).
					Where("comment_id = ?", comment.ParentId).
					UpdateColumn("child_count", gorm.Expr("child_count - ?", 1)).Error; err != nil {
					return err
				}
			}

			return nil
		})
	})

	if err != nil {
		return err
	}

	// 异步清除缓存
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.InvalidateCommentCache(context.Background(), commentId)
			sdb.cacheManager.InvalidateVideoCommentCache(context.Background(), videoId)
			sdb.cacheManager.IncrementVideoCommentCount(context.Background(), videoId, -1)
		}()
	}

	return nil
}

// CreateCommentLikeWithSharding 分片创建评论点赞
func (sdb *ShardedCommentDB) CreateCommentLikeWithSharding(ctx context.Context, commentId, userId int64) error {
	// 首先需要获取评论的视频ID
	comment, err := sdb.GetCommentInfoWithSharding(ctx, commentId)
	if err != nil {
		return err
	}

	uuid := uuid.New().ID()
	commentLike := &model.CommentLike{
		CommentLikesId: int64(uuid),
		CommentId:      commentId,
		UserId:         userId,
		CreatedAt:      time.Now().Format(constants.DataFormate),
		DeletedAt:      "",
	}

	// 在评论所在的分片中创建点赞记录
	err = sdb.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// 创建点赞记录（使用comment_likes表）
			if err := tx.Create(commentLike).Error; err != nil {
				return err
			}

			// 更新评论的点赞数
			if err := tx.Table(tableName).
				Where("comment_id = ?", commentId).
				UpdateColumn("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}

			return nil
		})
	})

	if err != nil {
		return err
	}

	// 异步更新缓存
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.IncrementCommentLikeCount(context.Background(), commentId, 1)
			sdb.cacheManager.InvalidateCommentCache(context.Background(), commentId)
		}()
	}

	return nil
}

// DeleteCommentLikeWithSharding 分片删除评论点赞
func (sdb *ShardedCommentDB) DeleteCommentLikeWithSharding(ctx context.Context, commentId, userId int64) error {
	// 首先需要获取评论的视频ID
	comment, err := sdb.GetCommentInfoWithSharding(ctx, commentId)
	if err != nil {
		return err
	}

	err = sdb.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// 删除点赞记录
			if err := tx.Model(&model.CommentLike{}).
				Where("comment_id = ? AND user_id = ?", commentId, userId).
				Delete(&model.CommentLike{}).Error; err != nil {
				return err
			}

			// 更新评论的点赞数
			if err := tx.Table(tableName).
				Where("comment_id = ?", commentId).
				UpdateColumn("like_count", gorm.Expr("like_count - ?", 1)).Error; err != nil {
				return err
			}

			return nil
		})
	})

	if err != nil {
		return err
	}

	// 异步更新缓存
	if sdb.cacheManager != nil {
		go func() {
			sdb.cacheManager.IncrementCommentLikeCount(context.Background(), commentId, -1)
			sdb.cacheManager.InvalidateCommentCache(context.Background(), commentId)
		}()
	}

	return nil
}

// GetUserCommentsWithSharding 分片获取用户评论列表
func (sdb *ShardedCommentDB) GetUserCommentsWithSharding(ctx context.Context, userId, pagenum, pagesize int64) (*[]int64, error) {
	// 用户评论需要在所有分片中查找
	// 这里需要一个更好的索引策略，比如按用户ID分片的辅助表

	var allComments []int64

	// 在所有分片中查找用户的评论
	for i := 0; i < sdb.shardingManager.GetDatabaseCount(); i++ {
		for j := 0; j < sdb.shardingManager.GetTableCount(); j++ {
			db := sdb.shardingManager.GetSlaveDBByIndex(i)
			tableName := fmt.Sprintf("comments_%d", j)

			var comments []int64
			err := db.WithContext(ctx).Table(tableName).
				Where("user_id = ? AND deleted_at = ''", userId).
				Order("created_at DESC").
				Select("comment_id").
				Scan(&comments).Error

			if err != nil && err != gorm.ErrRecordNotFound {
				hlog.Warnf("Failed to query user comments from %s: %v", tableName, err)
				continue
			}

			allComments = append(allComments, comments...)
		}
	}

	// 排序（按评论ID倒序，假设评论ID是递增的）
	sort.Slice(allComments, func(i, j int) bool {
		return allComments[i] > allComments[j]
	})

	// 分页
	start := int(pagenum-1) * int(pagesize)
	end := start + int(pagesize)
	if start >= len(allComments) {
		return &[]int64{}, nil
	}
	if end > len(allComments) {
		end = len(allComments)
	}

	result := allComments[start:end]
	return &result, nil
}

// Note: ShardingManager methods are defined in pkg/database/sharding.go
// These methods should be called on database.ShardingManager instances
