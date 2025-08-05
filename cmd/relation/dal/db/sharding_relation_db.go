package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"HuaTug.com/cmd/model"
	"gorm.io/gorm"
)

// ShardedFollowDB 分片关注关系DB
type ShardedFollowDB struct {
	shardingManager *ShardingManager
}

func NewShardedFollowDB(shardingManager *ShardingManager) *ShardedFollowDB {
	return &ShardedFollowDB{
		shardingManager: shardingManager,
	}
}

func (s *ShardedFollowDB) getShardingManager() (*ShardingManager, error) {
	if s.shardingManager == nil {
		return nil, errors.New("sharding manager is nil")
	}
	return s.shardingManager, nil
}

// InsertFollow 插入关注关系
func (s *ShardedFollowDB) InsertFollowWithTransaction(ctx context.Context, relation *model.FollowRelation) error {
	if relation == nil {
		return errors.New("relation cannot be nil")
	}

	if relation.UserID == 0 || relation.FollowerID == 0 {
		return errors.New("user_id and follower_id cannot be zero")
	}

	now := time.Now()
	relation.CreatedAt = now
	relation.UpdatedAt = now
	relation.DeletedAt = nil // 确保新记录的删除时间为 nil

	return s.shardingManager.ExecuteInShard(ctx, relation.FollowerID, true, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table(tableName).Create(relation).Error; err != nil {
				return fmt.Errorf("failed to create follow relation in transaction: %w", err)
			}
			return nil
		})
	})
}

// DeleteFollow 删除关注关系（软删除）
func (s *ShardedFollowDB) DeleteFollow(ctx context.Context, userID, followerID int64) error {
	if userID == 0 || followerID == 0 {
		return errors.New("user_id and follower_id cannot be zero")
	}

	return s.shardingManager.ExecuteInShard(ctx, followerID, true, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			now := time.Now()
			if err := tx.Table(tableName).Where("user_id = ? AND follower_id = ? AND deleted_at IS NULL", userID, followerID).
				Update("deleted_at", now).Error; err != nil {
				return fmt.Errorf("failed to soft delete follow relation: %w", err)
			}
			return nil
		})
	})
}

// GetFollowRelation 获取关注关系
// func (s *ShardedFollowDB) GetFollowRelation(ctx context.Context, followerID int64, limit, offset int) ([]*model.FollowRelation, error) {

// }

// GetFollowingList 获取关注列表
func (s *ShardedFollowDB) GetFollowingList(ctx context.Context, followerID int64, offset, limit int) ([]*model.FollowRelation, error) {
	if followerID == 0 {
		return nil, errors.New("follower_id cannot be zero")
	}

	var users []*model.FollowRelation

	err := s.shardingManager.ExecuteInShard(ctx, followerID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Where("follower_id = ? AND deleted_at IS NULL", followerID).Limit(limit).Offset(offset).Find(&users).Error
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get follow relation: %w", err)
	}
	return users, nil
}

// GetFollowerList 获取粉丝列表
func (s *ShardedFollowDB) GetFollowerList(ctx context.Context, userID int64, offset, limit int) ([]*model.FollowRelation, error) {
	if userID == 0 {
		return nil, errors.New("user_id cannot be zero")
	}

	var users []*model.FollowRelation

	err := s.shardingManager.ExecuteInShard(ctx, userID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Where("user_id = ? AND deleted_at IS NULL", userID).Limit(limit).Offset(offset).Find(&users).Error
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get follower list: %w", err)
	}
	return users, nil
}

// GetFollowingCount 获取关注数量
func (s *ShardedFollowDB) GetFollowingCount(ctx context.Context, followerID int64) (int64, error) {
	if followerID == 0 {
		return 0, errors.New("follower_id cannot be zero")
	}

	var count int64
	err := s.shardingManager.ExecuteInShard(ctx, followerID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Where("follower_id = ? AND deleted_at IS NULL", followerID).Count(&count).Error
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get following count: %w", err)
	}
	return count, nil
}

// IsFollowing 检查是否已关注
func (s *ShardedFollowDB) IsFollowing(ctx context.Context, userID, followerId int64) (bool, error) {
	if userID == 0 || followerId == 0 {
		return false, errors.New("user_id and follower_id cannot be zero")
	}

	var count int64
	err := s.shardingManager.ExecuteInShard(ctx, userID, false, func(db *gorm.DB, tableName string) error {
		return db.WithContext(ctx).Table(tableName).Where("user_id = ? AND follower_id = ? AND deleted_at IS NULL", userID, followerId).Count(&count).Error
	})
	if err != nil {
		return false, fmt.Errorf("failed to check following status: %w", err)
	}
	return count > 0, nil
}

// GetMutualFollowList 获取互关列表
func (s *ShardedFollowDB) GetMutualFollowList(ctx context.Context, userID int64, offset, limit int) ([]*model.FollowRelation, error) {
	if userID == 0 {
		return nil, errors.New("user_id cannot be zero")
	}

	// 首先获取我关注的所有人（我是关注者）
	followingList, err := s.GetFollowingList(ctx, userID, 0, -1) // 获取所有我关注的人
	if err != nil {
		return nil, fmt.Errorf("failed to get following list: %w", err)
	}

	if len(followingList) == 0 {
		return []*model.FollowRelation{}, nil
	}

	// 收集我关注的所有用户的ID（这些用户的user_id）
	followingUserIDs := make([]int64, 0, len(followingList))
	for _, follow := range followingList {
		followingUserIDs = append(followingUserIDs, follow.UserID)
	}

	// 在所有分片中查找这些用户是否也关注了我
	// 即：在这些用户中，查找follower_id等于这些用户ID，且user_id等于我的ID的记录
	var mutualFollows []*model.FollowRelation

	allDatabases := s.shardingManager.GetAllDatabases()

	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < s.shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("follows_%d", tableIndex)

			var follows []*model.FollowRelation
			err := db.WithContext(ctx).Table(tableName).
				Where("user_id = ? AND follower_id IN ? AND deleted_at IS NULL", userID, followingUserIDs).
				Find(&follows).Error

			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("failed to query mutual follows in table %s: %w", tableName, err)
			}

			if len(follows) > 0 {
				mutualFollows = append(mutualFollows, follows...)
			}
		}
	}

	// 应用分页
	start := offset
	if start >= len(mutualFollows) {
		return []*model.FollowRelation{}, nil
	}

	end := start + limit
	if end > len(mutualFollows) {
		end = len(mutualFollows)
	}

	if limit <= 0 { // 如果limit为0或负数，返回所有结果
		return mutualFollows, nil
	}

	return mutualFollows[start:end], nil
}

// GetFollowerCount 获取粉丝数量
func GetFollowerCount(ctx context.Context, userID int64) (int64, error) {
	if userID == 0 {
		return 0, errors.New("user_id cannot be zero")
	}

	shardingManager := GetShardingManager()
	if shardingManager == nil {
		return 0, errors.New("sharding manager is not initialized")
	}

	var totalCount int64

	allDatabases := shardingManager.GetAllDatabases()
	for _, db := range allDatabases {
		for tableIndex := 0; tableIndex < shardingManager.config.TableCount; tableIndex++ {
			tableName := fmt.Sprintf("follows_%d", tableIndex)
			var count int64
			if err := db.WithContext(ctx).Table(tableName).Where("user_id = ? AND deleted_at IS NULL", userID).Count(&count).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return 0, fmt.Errorf("failed to count followers in table %s: %w", tableName, err)
				}
			}
			totalCount += count
		}
	}
	return totalCount, nil
}
