package db

import (
	"context"
	"errors"
	"time"

	"HuaTug.com/cmd/model"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/database"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 全局分片管理器实例
var ShardingManager *database.ShardingManager

// InitShardingManager 初始化分片管理器
func InitShardingManager(config *database.ShardingConfig) error {
	var err error
	ShardingManager, err = database.NewShardingManager(config)
	return err
}

// GetCommentByIdFromAllShards 从所有分片中查找评论
func GetCommentByIdFromAllShards(ctx context.Context, commentId int64, withLock bool) (*model.Comment, error) {
	if ShardingManager == nil {
		return nil, errors.New("sharding manager not initialized")
	}

	// 遍历所有分片查找评论
	for dbIndex := 0; dbIndex < 4; dbIndex++ {
		for tableIndex := 0; tableIndex < 4; tableIndex++ {
			// 构造一个虚拟的videoId来获取对应的分片
			virtualVideoId := int64(dbIndex*4 + tableIndex)

			var comment model.Comment
			var err error

			err = ShardingManager.ExecuteInShard(ctx, virtualVideoId, false, func(db *gorm.DB, tableName string) error {
				query := db.Table(tableName).Where("comment_id = ?", commentId)
				if withLock {
					query = query.Set("gorm:query_option", "FOR UPDATE")
				}
				return query.First(&comment).Error
			})

			if err == nil {
				return &comment, nil
			}

			if err != gorm.ErrRecordNotFound {
				hlog.Errorf("Error searching comment %d in shard %d-%d: %v", commentId, dbIndex, tableIndex, err)
			}
		}
	}

	return nil, gorm.ErrRecordNotFound
}
func CreateComment(ctx context.Context, comment *model.Comment) error {
	// 生成唯一的评论ID
	if comment.CommentId == 0 {
		uuid := uuid.New().ID()
		comment.CommentId = int64(uuid)
	}

	// 总是使用分片逻辑，因为comments表已经不存在
	return ShardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Create(comment).Error
	})
}

// CreateCommentWithTransaction creates a comment within a database transaction
// This ensures data consistency and allows for rollback if any operation fails
func CreateCommentWithTransaction(ctx context.Context, comment *model.Comment) error {
	// 生成唯一的评论ID
	if comment.CommentId == 0 {
		uuid := uuid.New().ID()
		comment.CommentId = int64(uuid)
	}

	if ShardingManager != nil {
		return ShardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
			return db.Transaction(func(tx *gorm.DB) error {
				// Create the comment
				if err := tx.Table(tableName).Create(comment).Error; err != nil {
					return err
				}

				// Update parent comment's child count if this is a reply
				if comment.ParentId != -1 {
					if err := tx.Table(tableName).
						Where("comment_id = ?", comment.ParentId).
						UpdateColumn("child_count", gorm.Expr("child_count + ?", 1)).Error; err != nil {
						return err
					}
				}

				// Update video's comment count in main database
				// Note: This assumes there's a video_comment_count field in videos table
				// If not available, this can be removed or implemented differently
				if err := DB.WithContext(ctx).Exec("UPDATE videos SET comment_count = comment_count + 1 WHERE video_id = ?", comment.VideoId).Error; err != nil {
					// Log the error but don't fail the transaction if videos table doesn't have comment_count
					hlog.Warnf("Failed to update video comment count: %v", err)
				}

				return nil
			})
		})
	}

	// 如果分片管理器未初始化，返回错误
	return errors.New("sharding manager not initialized")
}

// ValidateCommentHierarchy validates that the comment hierarchy is correct
func ValidateCommentHierarchy(ctx context.Context, parentId, videoId int64) error {
	if parentId == -1 {
		// Root comment, no validation needed
		return nil
	}

	var parentComment model.Comment
	var err error

	// 总是使用分片逻辑，因为comments表已经不存在
	// 需要根据parentId找到对应的分片，这里我们需要一个全局索引表或者遍历所有分片
	// 为了简化，我们先尝试在videoId对应的分片中查找
	err = ShardingManager.ExecuteInShard(ctx, videoId, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("comment_id = ?", parentId).First(&parentComment).Error
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.New("parent comment not found")
		}
		return err
	}

	// Ensure parent comment belongs to the same video
	if parentComment.VideoId != videoId {
		return errors.New("parent comment belongs to different video")
	}

	// Prevent deep nesting (max 2 levels: root -> reply)
	if parentComment.ParentId != -1 {
		return errors.New("maximum comment depth exceeded")
	}

	return nil
}

// GetCommentWithLock retrieves a comment with row-level locking for concurrent safety
// Note: For sharded tables, we need to know the videoId to determine the correct shard
func GetCommentWithLock(ctx context.Context, commentId int64) (*model.Comment, error) {
	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，我们需要知道videoId来确定正确的分片
	// 这里需要先从全局索引表或者遍历所有分片来查找
	// 为了简化，我们可以添加一个辅助函数来处理这种情况
	return GetCommentByIdFromAllShards(ctx, commentId, true)
}

// CheckCommentExists efficiently checks if a comment exists
func CheckCommentExists(ctx context.Context, commentId int64) (bool, error) {
	// 总是使用分片逻辑，因为comments表已经不存在
	comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return comment.DeletedAt == "", nil
}

// GetUserRecentComments gets user's recent comments for spam detection
func GetUserRecentComments(ctx context.Context, userId int64, minutes int) ([]model.Comment, error) {
	var comments []model.Comment
	timeThreshold := time.Now().Add(-time.Duration(minutes) * time.Minute).Format(constants.DataFormate)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在所有分片中查找用户的评论
	// 这是一个跨分片查询，性能可能不佳，建议使用全局用户评论索引表
	for dbIndex := 0; dbIndex < 4; dbIndex++ {
		for tableIndex := 0; tableIndex < 4; tableIndex++ {
			// 构造一个虚拟的videoId来获取对应的分片
			virtualVideoId := int64(dbIndex*4 + tableIndex)

			var shardComments []model.Comment
			err := ShardingManager.ExecuteInShard(ctx, virtualVideoId, false, func(db *gorm.DB, tableName string) error {
				return db.Table(tableName).Where("user_id = ? AND created_at > ?", userId, timeThreshold).
					Find(&shardComments).Error
			})

			if err != nil {
				hlog.Errorf("Error searching user comments in shard %d-%d: %v", dbIndex, tableIndex, err)
				continue
			}

			comments = append(comments, shardComments...)
		}
	}
	return comments, nil
}

func DeleteComment(ctx context.Context, commentId int64) error {
	// 总是使用分片逻辑，因为comments表已经不存在
	// 首先找到评论所在的分片
	comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
	if err != nil {
		return err
	}

	// 在对应的分片中删除评论
	return ShardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("comment_id = ?", commentId).Delete(&model.Comment{}).Error
	})
}

// 获取子评论的数目
func GetChildCommentCount(ctx context.Context, commentId int64) (int64, error) {
	var count int64

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要先找到父评论所在的分片
	parentComment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
	if err != nil {
		return 0, err
	}

	// 在对应的分片中查询子评论数量
	err = ShardingManager.ExecuteInShard(ctx, parentComment.VideoId, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("parent_id = ?", commentId).Count(&count).Error
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func GetVideoCommentCount(ctx context.Context, videoId int64) (count int64, err error) {
	if ShardingManager != nil {
		// 对于分片表，需要在对应的分片中查询
		err = ShardingManager.ExecuteInShard(ctx, videoId, false, func(db *gorm.DB, tableName string) error {
			return db.Table(tableName).Where("video_id = ?", videoId).Count(&count).Error
		})
		return count, err
	}

	// 兜底使用原有的单库操作
	if err := DB.WithContext(ctx).Model(&model.Comment{}).Where("video_id = ?", videoId).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 获取某一条评论的全部信息
func GetCommentInfo(ctx context.Context, commentId int64) (comment *model.Comment, err error) {
	if ShardingManager != nil {
		// 对于分片表，需要在所有分片中查找
		return GetCommentByIdFromAllShards(ctx, commentId, false)
	}

	// 兜底使用原有的单库操作
	comment = &model.Comment{}
	if err := DB.WithContext(ctx).Model(&model.Comment{}).Where("comment_id = ?", commentId).Find(comment).Error; err != nil {
		return nil, err
	}
	return comment, nil
}

func GetParentCommentId(ctx context.Context, commentId int64) (parentId int64, err error) {
	// 总是使用分片逻辑，因为comments表已经不存在
	comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
	if err != nil {
		return 0, err
	}
	return comment.ParentId, nil
}

// 获得点赞某一条评论的所有用户
func GetCommentLikeList(ctx context.Context, commentId int64) (*[]int64, error) {
	list := make([]int64, 0)

	if ShardingManager != nil {
		// 对于分片表，评论点赞表也需要分片，这里需要先找到评论所在的分片
		comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
		if err != nil {
			return nil, err
		}

		// 在对应的分片中查询评论点赞列表
		err = ShardingManager.ExecuteInShard(ctx, comment.VideoId, false, func(db *gorm.DB, tableName string) error {
			return db.Table("comment_likes").Where("comment_id = ?", commentId).Select("user_id").Scan(&list).Error
		})
		if err != nil {
			return nil, err
		}
		return &list, nil
	}

	// 如果分片管理器未初始化，返回错误
	return nil, errors.New("sharding manager not initialized")
}

// 这段代码表示获得评论的点赞数
func GetCommentLikeCount(ctx context.Context, commentId int64) (count int64, err error) {
	if ShardingManager != nil {
		// 对于分片表，评论点赞表也需要分片，这里需要先找到评论所在的分片
		comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
		if err != nil {
			return 0, err
		}

		// 在对应的分片中查询评论点赞数
		err = ShardingManager.ExecuteInShard(ctx, comment.VideoId, false, func(db *gorm.DB, tableName string) error {
			return db.Table("comment_likes").Where("comment_id = ?", commentId).Count(&count).Error
		})
		return count, err
	}

	// 兜底使用原有的单库操作
	if err := DB.WithContext(ctx).Model(&model.CommentLike{}).Where("comment_id = ?", commentId).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 获得被评论的视频Id
func GetCommentVideoId(ctx context.Context, commentId int64) (videoId int64, err error) {
	if ShardingManager != nil {
		// 对于分片表，需要在所有分片中查找
		comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
		if err != nil {
			return 0, err
		}
		return comment.VideoId, nil
	}

	// 兜底使用原有的单库操作
	if err := DB.WithContext(ctx).Model(&model.Comment{}).Where("comment_id = ?", commentId).Select("video_id").Find(&videoId).Error; err != nil {
		return 0, err
	}
	return videoId, nil
}

// 获取子评论列表
func GetCommentChildList(ctx context.Context, comment_id int64) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要先找到父评论所在的分片
	parentComment, err := GetCommentByIdFromAllShards(ctx, comment_id, false)
	if err != nil {
		return nil, err
	}

	// 在对应的分片中查询子评论列表
	err = ShardingManager.ExecuteInShard(ctx, parentComment.VideoId, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("parent_id = ?", comment_id).Select("comment_id").Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

func GetCommentChildListByPart(ctx context.Context, comment_id, pagenum, pagesize int64) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要先找到父评论所在的分片
	parentComment, err := GetCommentByIdFromAllShards(ctx, comment_id, false)
	if err != nil {
		return nil, err
	}

	// 在对应的分片中查询子评论列表
	err = ShardingManager.ExecuteInShard(ctx, parentComment.VideoId, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("parent_id = ?", comment_id).Select("comment_id").
			Limit(int(pagesize)).Offset(int(pagenum-1) * int(pagesize)).Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

// 获取视频的评论列表
func GetVideoCommentList(ctx context.Context, video_id int64) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在对应的分片中查询
	err := ShardingManager.ExecuteInShard(ctx, video_id, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("video_id = ?", video_id).Select("comment_id").Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

func GetVideoCommentListByPart(ctx context.Context, video_id, pagenum, pagesize int64) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在对应的分片中查询
	err := ShardingManager.ExecuteInShard(ctx, video_id, false, func(db *gorm.DB, tableName string) error {
		return db.Table(tableName).Where("video_id = ?", video_id).Select("comment_id").
			Limit(int(pagesize)).Offset(int(pagenum-1) * int(pagesize)).Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

// GetVideoCommentListByPartWithSort gets video comments with specified sorting
func GetVideoCommentListByPartWithSort(ctx context.Context, video_id, pagenum, pagesize int64, sortType string) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在对应的分片中查询
	err := ShardingManager.ExecuteInShard(ctx, video_id, false, func(db *gorm.DB, tableName string) error {
		query := db.Table(tableName).Where("video_id = ? AND deleted_at = ''", video_id)

		switch sortType {
		case "latest":
			// Sort by creation time descending (newest first)
			query = query.Order("created_at DESC")
		case "hot":
			// For hot comments, we'll get more results and sort them in service layer
			// This allows us to incorporate Redis like counts into the sorting
			query = query.Order("created_at DESC")
		default:
			// Default to latest
			query = query.Order("created_at DESC")
		}

		return query.Select("comment_id").Limit(int(pagesize)).Offset(int(pagenum-1) * int(pagesize)).Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

// GetVideoCommentListForHotSort gets a larger set of comments for hot sorting
// This allows the service layer to sort by like count + time
func GetVideoCommentListForHotSort(ctx context.Context, video_id, pagenum, pagesize int64) (*[]int64, error) {
	list := make([]int64, 0)
	// Get more comments than requested to allow for proper hot sorting
	// We'll get 3x the requested amount to ensure we have enough data for sorting
	extendedSize := pagesize * 3
	if extendedSize > 100 {
		extendedSize = 100 // Cap at 100 to avoid performance issues
	}

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在对应的分片中查询
	err := ShardingManager.ExecuteInShard(ctx, video_id, false, func(db *gorm.DB, tableName string) error {
		query := db.Table(tableName).
			Where("video_id = ? AND deleted_at = ''", video_id).
			Order("created_at DESC")

		return query.Select("comment_id").Limit(int(extendedSize)).Scan(&list).Error
	})
	if err != nil {
		return nil, err
	}
	return &list, nil
}

func GetCommentIdList(ctx context.Context) (*[]int64, error) {
	list := make([]int64, 0)

	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在所有分片中查找
	// 这是一个全表扫描操作，性能可能不佳，建议谨慎使用
	for dbIndex := 0; dbIndex < 4; dbIndex++ {
		for tableIndex := 0; tableIndex < 4; tableIndex++ {
			// 构造一个虚拟的videoId来获取对应的分片
			virtualVideoId := int64(dbIndex*4 + tableIndex)

			var shardList []int64
			err := ShardingManager.ExecuteInShard(ctx, virtualVideoId, false, func(db *gorm.DB, tableName string) error {
				return db.Table(tableName).Select("comment_id").Scan(&shardList).Error
			})

			if err != nil {
				hlog.Errorf("Error getting comment IDs from shard %d-%d: %v", dbIndex, tableIndex, err)
				continue
			}

			list = append(list, shardList...)
		}
	}
	return &list, nil
}

// 用来检查给定的comment_id是否在这个数据表中
func IsCommentIdList(ctx context.Context, comment_id int64) (bool, error) {
	// 总是使用分片逻辑，因为comments表已经不存在
	// 对于分片表，需要在所有分片中查找
	_, err := GetCommentByIdFromAllShards(ctx, comment_id, false)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func CreateCommentLike(ctx context.Context, comemntId, userId int64) error {
	uuid := uuid.New().ID()
	commentLike := &model.CommentLike{
		CommentLikesId: int64(uuid),
		CommentId:      comemntId,
		UserId:         userId,
		CreatedAt:      time.Now().Format(constants.DataFormate),
		DeletedAt:      "",
	}

	if ShardingManager != nil {
		// 对于分片表，需要先找到评论所在的分片
		comment, err := GetCommentByIdFromAllShards(ctx, comemntId, false)
		if err != nil {
			return err
		}

		// 在对应的分片中创建评论点赞
		return ShardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
			return db.Table("comment_likes").Create(commentLike).Error
		})
	}

	// 兜底使用原有的单库操作
	if err := DB.WithContext(ctx).Create(commentLike).Error; err != nil {
		return err
	}
	return nil
}

func DeleteCommentLike(ctx context.Context, commentId, UserId int64) error {
	if ShardingManager != nil {
		// 对于分片表，需要先找到评论所在的分片
		comment, err := GetCommentByIdFromAllShards(ctx, commentId, false)
		if err != nil {
			return err
		}

		// 在对应的分片中删除评论点赞
		return ShardingManager.ExecuteInShard(ctx, comment.VideoId, true, func(db *gorm.DB, tableName string) error {
			return db.Table("comment_likes").Where("comment_id = ? And user_id = ?", commentId, UserId).Delete(&model.CommentLike{}).Error
		})
	}

	// 兜底使用原有的单库操作
	if err := DB.WithContext(ctx).Model(&model.CommentLike{}).Where("comment_id = ? And user_id = ?", commentId, UserId).Delete(&model.CommentLike{}).Error; err != nil {
		hlog.Info(err)
		return err
	}
	return nil
}

func CreateVideoLike(ctx context.Context, videoLike *model.VideoLike) error {
	if err := DB.Create(videoLike).Error; err != nil {
		return err
	}
	return nil
}

func DeleteVideoLike(ctx context.Context, videoId, userId int64) error {
	if err := DB.WithContext(ctx).Model(&model.VideoLike{}).Where("video_id = ? And user_id = ?", videoId, userId).Delete(&model.VideoLike{}).Error; err != nil {
		return err
	}
	return nil
}

// 与下面的函数刚好相对应，即这个函数获得是这个视频被多少人喜欢，下面的函数则是一个用户喜欢的所有视频
func GetVideoLikeList(ctx context.Context, videoId int64) (*[]string, error) {
	list := make([]string, 0)
	if err := DB.WithContext(ctx).Model(&model.VideoLike{}).Where("video_id = ?", videoId).Select("user_id").Scan(&list).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

// 获取用户喜欢的视频列表
func GetVideoLikeListByUserId(ctx context.Context, userId, pageNum, pageSize int64) (*[]string, error) {
	list := make([]string, 0)
	if err := DB.WithContext(ctx).Model(&model.VideoLike{}).Where("user_id = ?", userId).Select("video_id").Offset(int(pageNum-1) * int(pageSize)).Limit(int(pageSize)).Select("video_id").Scan(&list).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

// 记录用户的点赞行为
func AddUserLikeBehavior(ctx context.Context, behavior *model.UserBehavior) error {
	if err := DB.WithContext(ctx).Create(behavior).Error; err != nil {
		return err
	}
	return nil
}

func DeleteUserLikeBehavior(ctx context.Context, userId, videoId int64, behavior string) error {
	if err := DB.WithContext(ctx).Model(&model.UserBehavior{}).Where("user_id = ? and video_id = ? and behavior_type = ?", userId, videoId, behavior).Delete(&model.UserBehavior{}).Error; err != nil {
		return err
	}
	return nil
}

// 记录用户的评论行为
func AddUserCommentBehavior(ctx context.Context, behavior *model.UserBehavior) error {
	// For comment behavior, we want to record each comment action
	// Use raw SQL with ON DUPLICATE KEY UPDATE to handle potential conflicts gracefully
	if behavior.BehaviorType == "comment" {
		// For comments, always insert a new record (after removing unique constraint)
		if err := DB.WithContext(ctx).Create(behavior).Error; err != nil {
			hlog.Warnf("Failed to record comment behavior for user %d on video %d: %v",
				behavior.UserId, behavior.VideoId, err)
			// Don't return error to avoid blocking comment creation
			return nil
		}
	}
	return nil
}

// 获取视频信息
func GetVideoInfo(ctx context.Context, videoID int64) (*model.Video, error) {
	var video model.Video
	if err := DB.WithContext(ctx).Model(&model.Video{}).Where("video_id = ?", videoID).First(&video).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

// 通知相关的数据库操作
func CreateNotification(ctx context.Context, notification interface{}) error {
	if err := DB.WithContext(ctx).Create(notification).Error; err != nil {
		return err
	}
	return nil
}
