package redis

import (
	"context"
	"crypto/md5"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
)

// Redis key templates for comment system
const (
	// 评论频率限制 Key：comment_rate_limit:{user_id}
	CommentRateLimitKeyTemplate = "comment_rate_limit:%d"

	// 评论内容哈希 Key：comment_hash:{user_id}:{hash}
	CommentHashKeyTemplate = "comment_hash:%d:%s"

	// 视频热门列表 Key：video:popular:list
	VideoPopularListKey = "video:popular:list"

	// 评论点赞数 Key：comment:like:{comment_id}
	CommentLikeKeyTemplate = "comment:like:%d"

	// 视频点赞数 Key：video:like:{video_id}
	VideoLikeKeyTemplate = "video:like:%d"

	// 评论相关数据 Key：comment:data:{comment_id}
	CommentDataKeyTemplate = "comment:data:%d"
)

// GetCommentRateLimit 获取用户评论频率限制计数
func GetCommentRateLimit(key string) (int64, error) {
	countStr, err := RedisDBInteraction.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 键不存在，返回 0
		}
		return 0, fmt.Errorf("failed to get comment rate limit: %w", err)
	}

	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse comment rate limit: %w", err)
	}

	return count, nil
}

// IncrementCommentRateLimit 增加用户评论频率限制计数
func IncrementCommentRateLimit(key string, expireSeconds int64) error {
	pipe := RedisDBInteraction.TxPipeline()

	// 增加计数
	pipe.Incr(key)
	// 设置过期时间
	pipe.Expire(key, time.Duration(expireSeconds)*time.Second)

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to increment comment rate limit: %w", err)
	}

	return nil
}

// CheckDuplicateComment 检查重复评论
func CheckDuplicateComment(userId int64, content string, timeWindow int) (bool, error) {
	// 生成内容哈希
	contentHash := generateContentHash(content)
	key := fmt.Sprintf(CommentHashKeyTemplate, userId, contentHash)

	// 检查键是否存在
	exists, err := RedisDBInteraction.Exists(key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check duplicate comment: %w", err)
	}

	return exists > 0, nil
}

// StoreCommentHash 存储评论内容哈希
func StoreCommentHash(userId int64, content string, timeWindow int) error {
	// 生成内容哈希
	contentHash := generateContentHash(content)
	key := fmt.Sprintf(CommentHashKeyTemplate, userId, contentHash)

	// 设置键和过期时间
	err := RedisDBInteraction.Set(key, "1", time.Duration(timeWindow)*time.Second).Err()
	if err != nil {
		return fmt.Errorf("failed to store comment hash: %w", err)
	}

	return nil
}

// generateContentHash 生成内容哈希
func generateContentHash(content string) string {
	hash := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", hash)
}

// GetCommentLikeCount 获取评论点赞数
func GetCommentLikeCount(commentId int64) (int64, error) {
	// 使用现有的LikeCacheManager实现
	manager := NewLikeCacheManager(RedisDBInteraction)
	return manager.GetCommentLikeCount(context.Background(), commentId)
}

// GetVideoLikeCount 获取视频点赞数
func GetVideoLikeCount(videoId int64) (int64, error) {
	// 使用现有的LikeCacheManager实现
	manager := NewLikeCacheManager(RedisDBInteraction)
	return manager.GetVideoLikeCount(context.Background(), videoId)
}

// DeleteCommentAndAllAbout 删除评论及相关所有数据
func DeleteCommentAndAllAbout(commentId int64) error {
	manager := NewLikeCacheManager(RedisDBInteraction)

	pipe := RedisDBInteraction.TxPipeline()

	// 删除评论点赞相关缓存
	err := manager.DeleteContentLikeCache(context.Background(), BusinessTypeComment, commentId)
	if err != nil {
		return fmt.Errorf("failed to delete comment like cache: %w", err)
	}

	// 删除评论数据缓存
	commentDataKey := fmt.Sprintf(CommentDataKeyTemplate, commentId)
	pipe.Del(commentDataKey)

	_, err = pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to delete comment data: %w", err)
	}

	return nil
}

// GetVideoPopularList 获取视频热门列表
func GetVideoPopularList(pageNum, pageSize int64) (*[]string, error) {
	start := (pageNum - 1) * pageSize
	end := start + pageSize - 1

	// 使用ZREVRANGE获取热门视频列表（按分数倒序）
	result, err := RedisDBInteraction.ZRevRange(VideoPopularListKey, start, end).Result()
	if err != nil {
		if err == redis.Nil {
			// 返回空列表
			emptyList := make([]string, 0)
			return &emptyList, nil
		}
		return nil, fmt.Errorf("failed to get video popular list: %w", err)
	}

	return &result, nil
}

// DeleteAllComment 批量删除评论缓存
func DeleteAllComment(commentIds []int64) error {
	if len(commentIds) == 0 {
		return nil
	}

	manager := NewLikeCacheManager(RedisDBInteraction)
	pipe := RedisDBInteraction.TxPipeline()

	for _, commentId := range commentIds {
		// 删除评论点赞相关缓存
		err := manager.DeleteContentLikeCache(context.Background(), BusinessTypeComment, commentId)
		if err != nil {
			return fmt.Errorf("failed to delete comment like cache for comment %d: %w", commentId, err)
		}

		// 删除评论数据缓存
		commentDataKey := fmt.Sprintf(CommentDataKeyTemplate, commentId)
		pipe.Del(commentDataKey)
	}

	_, err := pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to batch delete comments: %w", err)
	}

	return nil
}

// DeleteVideoAndAllAbout 删除视频及相关所有数据
func DeleteVideoAndAllAbout(videoId int64) error {
	manager := NewLikeCacheManager(RedisDBInteraction)

	pipe := RedisDBInteraction.TxPipeline()

	// 删除视频点赞相关缓存
	err := manager.DeleteContentLikeCache(context.Background(), BusinessTypeVideo, videoId)
	if err != nil {
		return fmt.Errorf("failed to delete video like cache: %w", err)
	}

	// 从热门列表中移除该视频
	pipe.ZRem(VideoPopularListKey, strconv.FormatInt(videoId, 10))

	// 删除视频相关的其他缓存键
	videoDataPattern := fmt.Sprintf("video:*:%d", videoId)
	keys, err := RedisDBInteraction.Keys(videoDataPattern).Result()
	if err == nil && len(keys) > 0 {
		pipe.Del(keys...)
	}

	_, err = pipe.Exec()
	if err != nil {
		return fmt.Errorf("failed to delete video data: %w", err)
	}

	return nil
}

// AddVideoToPopularList 添加视频到热门列表
func AddVideoToPopularList(videoId int64, score float64) error {
	err := RedisDBInteraction.ZAdd(VideoPopularListKey, redis.Z{
		Score:  score,
		Member: strconv.FormatInt(videoId, 10),
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to add video to popular list: %w", err)
	}

	return nil
}

// UpdateVideoPopularScore 更新视频热门分数
func UpdateVideoPopularScore(videoId int64, score float64) error {
	err := RedisDBInteraction.ZAdd(VideoPopularListKey, redis.Z{
		Score:  score,
		Member: strconv.FormatInt(videoId, 10),
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to update video popular score: %w", err)
	}

	return nil
}

// RemoveVideoFromPopularList 从热门列表移除视频
func RemoveVideoFromPopularList(videoId int64) error {
	err := RedisDBInteraction.ZRem(VideoPopularListKey, strconv.FormatInt(videoId, 10)).Err()
	if err != nil {
		return fmt.Errorf("failed to remove video from popular list: %w", err)
	}

	return nil
}

// GetVideoPopularScore 获取视频热门分数
func GetVideoPopularScore(videoId int64) (float64, error) {
	score, err := RedisDBInteraction.ZScore(VideoPopularListKey, strconv.FormatInt(videoId, 10)).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 视频不在热门列表中
		}
		return 0, fmt.Errorf("failed to get video popular score: %w", err)
	}

	return score, nil
}

// GetPopularListSize 获取热门列表大小
func GetPopularListSize() (int64, error) {
	size, err := RedisDBInteraction.ZCard(VideoPopularListKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get popular list size: %w", err)
	}

	return size, nil
}
