package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/cache"
	"HuaTug.com/pkg/constants"
	"gorm.io/gorm"
)

// MaxCommentLength 评论最大长度
const MaxCommentLength = 500

// ShardedCommentDB 分片评论数据访问对象
type ShardedCommentDB struct {
	shardingManager *ShardingManager
	cacheManager    *cache.CommentCacheManager
}

// NewShardedCommentDB 创建新的ShardedCommentDB实例
func NewShardedCommentDB(shardingManager *ShardingManager, cacheManager *cache.CommentCacheManager) *ShardedCommentDB {
	return &ShardedCommentDB{
		shardingManager: shardingManager,
		cacheManager:    cacheManager,
	}
}

// getShardingManager 获取分片管理器实例，避免重复的空值检查
func (s *ShardedCommentDB) getShardingManager() (*ShardingManager, error) {
	if s.shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}
	return s.shardingManager, nil
}


// CreateCommentWithTransaction 在事务中创建评论
func (s *ShardedCommentDB) CreateCommentWithTransaction(ctx context.Context, comment *model.Comment) error {
	if comment == nil {
		return errors.New("comment cannot be nil")
	}

	// 基本验证
	if comment.VideoId == 0 || comment.UserId == 0 {
		return errors.New("video_id and user_id are required")
	}

	if len(comment.Content) == 0 {
		return errors.New("comment content cannot be empty")
	}

	if len(comment.Content) > MaxCommentLength {
		return fmt.Errorf("comment content too long, max length is %d", MaxCommentLength)
	}

	// 设置时间戳
	now := time.Now().Format("2006-01-02 15:04:05")
	comment.CreatedAt = now
	comment.UpdatedAt = now

	// 使用视频ID作为分片键在事务中创建评论
	return s.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table(tableName).Create(comment).Error; err != nil {
				return fmt.Errorf("failed to create comment in transaction: %w", err)
			}
			return nil
		})
	})
}

// GetCommentByID 根据ID获取评论
func (s *ShardedCommentDB) GetCommentByID(ctx context.Context, commentID int64) (*model.Comment, error) {
	if commentID == 0 {
		return nil, errors.New("comment_id cannot be zero")
	}

	// TODO: 这里可以先检查缓存
	// if s.cacheManager != nil {
	//     if cached := s.cacheManager.GetComment(commentID); cached != nil {
	//         return cached, nil
	//     }
	// }

	// 由于只有commentID，需要遍历所有分片查找
	// 在实际生产环境中，应该有更好的索引策略，比如：
	// 1. 维护一个commentID到videoID的映射表
	// 2. 使用全局索引服务
	// 3. 在评论ID中编码分片信息
	var comment *model.Comment
	var foundErr error

	// 遍历所有数据库分片
	allDatabases := s.shardingManager.GetAllDatabases()
	for dbKey, db := range allDatabases {
		// 遍历该数据库中的所有表分片
		for tableIndex := 0; tableIndex < s.shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var tempComment model.Comment

			err := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).First(&tempComment).Error
			if err == nil {
				comment = &tempComment
				// TODO: 缓存结果
				// if s.cacheManager != nil {
				//     s.cacheManager.SetComment(commentID, comment)
				// }
				return comment, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				foundErr = fmt.Errorf("failed to query comment from %s.%s: %w", dbKey, tableName, err)
			}
		}
	}

	if foundErr != nil {
		return nil, foundErr
	}

	return nil, nil // 未找到评论
}

// GetVideoComments 获取视频的评论列表
func (s *ShardedCommentDB) GetVideoComments(ctx context.Context, videoID int64, limit, offset int) ([]*model.Comment, error) {
	if videoID == 0 {
		return nil, errors.New("video_id cannot be zero")
	}

	if limit <= 0 || limit > constants.MaxLimit {
		limit = constants.DefaultLimit
	}

	var comments []*model.Comment

	// 使用视频ID作为分片键查询对应分片
	err := s.shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Where("video_id = ?", videoID).
			Order("created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&comments).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comments: %w", err)
	}

	return comments, nil
}

// UpdateComment 更新评论
func (s *ShardedCommentDB) UpdateComment(ctx context.Context, comment *model.Comment) error {
	if comment == nil {
		return errors.New("comment cannot be nil")
	}

	if comment.CommentId == 0 {
		return errors.New("comment id cannot be zero")
	}

	comment.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")

	// 使用视频ID作为分片键更新对应分片
	return s.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		result := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", comment.CommentId).Save(comment)
		if result.Error != nil {
			return fmt.Errorf("failed to update comment: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errors.New("comment not found or no changes made")
		}
		return nil
	})
}

// DeleteComment 删除评论
func (s *ShardedCommentDB) DeleteComment(ctx context.Context, commentID int64) error {
	if commentID == 0 {
		return errors.New("comment_id cannot be zero")
	}

	// 先获取评论信息用于分片定位和缓存清理
	comment, err := s.GetCommentByID(ctx, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return errors.New("comment not found")
	}

	// 使用视频ID作为分片键删除对应分片中的评论
	return s.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		result := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).Delete(&model.Comment{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete comment: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errors.New("comment not found in shard")
		}
		return nil
	})
}

// GetCommentCount 获取视频评论数量
func (s *ShardedCommentDB) GetCommentCount(ctx context.Context, videoID int64) (int64, error) {
	if videoID == 0 {
		return 0, errors.New("video_id cannot be zero")
	}

	var count int64

	// 使用视频ID作为分片键查询对应分片
	err := s.shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Model(&model.Comment{}).Where("video_id = ?", videoID).Count(&count).Error
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count comments: %w", err)
	}

	return count, nil
}

// DeleteCommentWithSharding 删除评论（分片版本）
func (s *ShardedCommentDB) DeleteCommentWithSharding(ctx context.Context, commentID int64, videoID ...int64) error {
	// videoID 参数是可选的，用于分片计算，但目前我们简化实现
	return s.DeleteComment(ctx, commentID)
}

// CreateCommentLikeWithSharding 创建评论点赞（分片版本）
func (s *ShardedCommentDB) CreateCommentLikeWithSharding(ctx context.Context, commentID, userID int64) error {
	if userID == 0 || commentID == 0 {
		return errors.New("user_id and comment_id are required")
	}

	// 先获取评论信息以确定分片
	comment, err := s.GetCommentByID(ctx, commentID)
	if err != nil {
		return fmt.Errorf("failed to get comment for sharding: %w", err)
	}
	if comment == nil {
		return errors.New("comment not found")
	}

	like := &model.CommentLike{
		UserId:    userID,
		CommentId: commentID,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	// 使用评论的视频ID作为分片键
	return s.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		// 评论点赞表名应该与评论表对应
		likeTableName := fmt.Sprintf("comment_likes_%s", tableName[len("comments_"):])
		if err := db.WithContext(ctx).Table(likeTableName).Create(like).Error; err != nil {
			return fmt.Errorf("failed to create comment like: %w", err)
		}
		return nil
	})
}

// DeleteCommentLikeWithSharding 删除评论点赞（分片版本）
func (s *ShardedCommentDB) DeleteCommentLikeWithSharding(ctx context.Context, userID, commentID int64) error {
	if userID == 0 || commentID == 0 {
		return errors.New("user_id and comment_id are required")
	}

	// 先获取评论信息以确定分片
	comment, err := s.GetCommentByID(ctx, commentID)
	if err != nil {
		return fmt.Errorf("failed to get comment for sharding: %w", err)
	}
	if comment == nil {
		return errors.New("comment not found")
	}

	// 使用评论的视频ID作为分片键
	return s.shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		// 评论点赞表名应该与评论表对应
		likeTableName := fmt.Sprintf("comment_likes_%s", tableName[len("comments_"):])
		result := db.WithContext(ctx).Table(likeTableName).Where("user_id = ? AND comment_id = ?", userID, commentID).Delete(&model.CommentLike{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete comment like: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errors.New("comment like not found")
		}
		return nil
	})
}

// GetCommentInfoWithSharding 获取评论信息（分片版本）
func (s *ShardedCommentDB) GetCommentInfoWithSharding(ctx context.Context, commentID int64) (*model.Comment, error) {
	return s.GetCommentByID(ctx, commentID)
}

// GetParentCommentId 获取父评论ID
func GetParentCommentId(ctx context.Context, commentID int64) (int64, error) {
	// 使用全局分片管理器实例
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return 0, errors.New("sharding manager is not initialized")
	}

	// 遍历所有分片查找评论
	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var comment model.Comment

			err := db.WithContext(ctx).Table(tableName).Select("parent_id").Where("comment_id = ?", commentID).First(&comment).Error
			if err == nil {
				return comment.ParentId, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("failed to get parent comment id from %s: %w", tableName, err)
			}
		}
	}

	return 0, nil // 未找到评论
}

// GetCommentVideoId 获取评论对应的视频ID
func GetCommentVideoId(ctx context.Context, commentID int64) (int64, error) {
	// 使用全局分片管理器实例
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return 0, errors.New("sharding manager is not initialized")
	}

	// 遍历所有分片查找评论
	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var comment model.Comment

			err := db.WithContext(ctx).Table(tableName).Select("video_id").Where("comment_id = ?", commentID).First(&comment).Error
			if err == nil {
				return comment.VideoId, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("failed to get comment video id from %s: %w", tableName, err)
			}
		}
	}

	return 0, nil // 未找到评论
}

// CreateCommentWithTransaction 创建评论（带事务）
func CreateCommentWithTransaction(ctx context.Context, comment *model.Comment) error {
	if comment == nil {
		return errors.New("comment cannot be nil")
	}

	// 基本验证
	if comment.VideoId == 0 || comment.UserId == 0 {
		return errors.New("video_id and user_id are required")
	}

	if len(comment.Content) == 0 {
		return errors.New("comment content cannot be empty")
	}

	if len(comment.Content) > MaxCommentLength {
		return fmt.Errorf("comment content too long, max length is %d", MaxCommentLength)
	}

	// 使用全局分片管理器实例
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return errors.New("sharding manager is not initialized")
	}

	// 设置时间戳
	now := time.Now().Format("2006-01-02 15:04:05")
	comment.CreatedAt = now
	comment.UpdatedAt = now

	// 使用视频ID作为分片键在事务中创建评论
	return shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table(tableName).Create(comment).Error; err != nil {
				return fmt.Errorf("failed to create comment in transaction: %w", err)
			}
			return nil
		})
	})
}

// AddUserCommentBehavior 添加用户评论行为记录
func AddUserCommentBehavior(ctx context.Context, userBehavior interface{}) error {
	// TODO: 实现用户行为记录逻辑
	// 这里可以记录用户的评论行为，用于推荐系统等
	return nil
}

// GetCommentInfo 获取评论信息
func GetCommentInfo(ctx context.Context, commentID int64) (*model.Comment, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	// 遍历所有分片查找评论
	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var comment model.Comment

			err := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).First(&comment).Error
			if err == nil {
				return &comment, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("failed to get comment info from %s: %w", tableName, err)
			}
		}
	}

	return nil, nil // 未找到评论
}

// GetChildCommentCount 获取子评论数量
func GetChildCommentCount(ctx context.Context, parentCommentID int64) (int64, error) {
	// 使用全局分片管理器实例
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return 0, errors.New("sharding manager is not initialized")
	}

	var totalCount int64

	// 遍历所有分片统计子评论数量
	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var count int64
			if err := db.WithContext(ctx).Table(tableName).Where("parent_id = ?", parentCommentID).Count(&count).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return 0, fmt.Errorf("failed to get child comment count from %s: %w", tableName, err)
				}
			}
			totalCount += count
		}
	}

	return totalCount, nil
}

// GetVideoCommentListForHotSort 获取视频评论列表（热度排序）
func GetVideoCommentListForHotSort(ctx context.Context, videoID int64, pageNum, pageSize int64) (*[]int64, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	var commentIDs []int64
	offset := (pageNum - 1) * pageSize

	err := shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).
			Select("comment_id").
			Where("video_id = ?", videoID).
			Order("like_count DESC, created_at DESC").
			Limit(int(pageSize)).
			Offset(int(offset)).
			Pluck("comment_id", &commentIDs).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comments for hot sort: %w", err)
	}

	return &commentIDs, nil
}

// GetVideoCommentListByPartWithSort 获取视频评论列表（分页+排序）
func GetVideoCommentListByPartWithSort(ctx context.Context, videoID int64, pageNum, pageSize int64, sortType string) (*[]int64, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	var commentIDs []int64
	offset := (pageNum - 1) * pageSize

	err := shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		query := db.WithContext(ctx).Table(tableName).Select("comment_id").Where("video_id = ?", videoID)

		switch sortType {
		case "hot":
			query = query.Order("like_count DESC, created_at DESC")
		case "time":
			query = query.Order("created_at DESC")
		default:
			query = query.Order("created_at DESC")
		}

		return query.Limit(int(pageSize)).Offset(int(offset)).Pluck("comment_id", &commentIDs).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comments by part with sort: %w", err)
	}

	return &commentIDs, nil
}

// GetVideoCommentListByPart 获取视频评论列表（分页）
func GetVideoCommentListByPart(ctx context.Context, videoID int64, pageNum, pageSize int64) (*[]int64, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	var commentIDs []int64
	offset := (pageNum - 1) * pageSize

	err := shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).
			Select("comment_id").
			Where("video_id = ?", videoID).
			Order("created_at DESC").
			Limit(int(pageSize)).
			Offset(int(offset)).
			Pluck("comment_id", &commentIDs).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comments by part: %w", err)
	}

	return &commentIDs, nil
}

// GetCommentChildListByPart 获取子评论列表（分页）
func GetCommentChildListByPart(ctx context.Context, parentCommentID int64, pageNum, pageSize int64) (*[]int64, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	// 由于parentCommentID可能来自任何分片，需要遍历所有分片
	var allCommentIDs []int64
	offset := (pageNum - 1) * pageSize
	limit := int(pageSize)
	currentCount := 0

	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			if currentCount >= limit {
				break
			}

			tableName := fmt.Sprintf("comments_%d", tableIndex)
			var commentIDs []int64

			err := db.WithContext(ctx).Table(tableName).
				Select("comment_id").
				Where("parent_id = ?", parentCommentID).
				Order("created_at ASC").
				Limit(limit-currentCount).
				Offset(int(offset)).
				Pluck("comment_id", &commentIDs).Error

			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("failed to get comment child list from %s: %w", tableName, err)
			}

			allCommentIDs = append(allCommentIDs, commentIDs...)
			currentCount += len(commentIDs)
		}
	}

	return &allCommentIDs, nil
}

// GetVideoCommentList 获取视频评论列表
func GetVideoCommentList(ctx context.Context, videoID int64) (*[]int64, error) {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return nil, errors.New("sharding manager is not initialized")
	}

	var commentIDs []int64

	err := shardingManager.ExecuteInShard(ctx, videoID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).
			Select("comment_id").
			Where("video_id = ?", videoID).
			Order("created_at DESC").
			Pluck("comment_id", &commentIDs).Error
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get video comment list: %w", err)
	}

	return &commentIDs, nil
}

// DeleteComment 删除评论
func DeleteComment(ctx context.Context, commentID int64) error {
	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return errors.New("sharding manager is not initialized")
	}

	// 先获取评论信息以确定分片
	comment, err := GetCommentInfo(ctx, commentID)
	if err != nil {
		return err
	}
	if comment == nil {
		return errors.New("comment not found")
	}

	// 使用视频ID作为分片键删除对应分片中的评论
	return shardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		result := db.WithContext(ctx).Table(tableName).Where("comment_id = ?", commentID).Delete(&model.Comment{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete comment: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errors.New("comment not found in shard")
		}
		return nil
	})
}

// CreateNotification 创建通知
func CreateNotification(ctx context.Context, notification interface{}) error {
	// TODO: 实现通知创建逻辑
	return nil
}

// GetVideoInfo 获取视频信息
func GetVideoInfo(ctx context.Context, videoID int64) (interface{}, error) {
	// TODO: 实现获取视频信息逻辑
	return nil, nil
}


