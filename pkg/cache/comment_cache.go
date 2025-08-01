package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"HuaTug.com/cmd/model"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/redis/go-redis/v9"
)

// CommentCacheManager 评论缓存管理器
type CommentCacheManager struct {
	client *redis.Client
	// 缓存过期时间配置
	hotCommentExpire    time.Duration // 热门评论缓存时间
	normalCommentExpire time.Duration // 普通评论缓存时间
	counterExpire       time.Duration // 计数器缓存时间
}

// NewCommentCacheManager 创建评论缓存管理器
func NewCommentCacheManager(client *redis.Client) *CommentCacheManager {
	return &CommentCacheManager{
		client:              client,
		hotCommentExpire:    30 * time.Minute, // 热门评论缓存30分钟
		normalCommentExpire: 10 * time.Minute, // 普通评论缓存10分钟
		counterExpire:       1 * time.Hour,    // 计数器缓存1小时
	}
}

// 缓存键名常量
const (
	// 视频评论列表缓存键
	VideoCommentsKey = "video:comments:%d:sort:%s:page:%d"
	// 评论详情缓存键
	CommentDetailKey = "comment:detail:%d"
	// 评论点赞数缓存键
	CommentLikeCountKey = "comment:like_count:%d"
	// 视频评论总数缓存键
	VideoCommentCountKey = "video:comment_count:%d"
	// 用户评论列表缓存键
	UserCommentsKey = "user:comments:%d:page:%d"
	// 热门评论分数缓存键
	CommentHotScoreKey = "comment:hot_score:%d"
	// 评论子评论列表缓存键
	CommentChildrenKey = "comment:children:%d:page:%d"
)

// CacheCommentList 缓存评论列表
func (ccm *CommentCacheManager) CacheCommentList(ctx context.Context, videoID int64,
	sortType string, page int, comments []int64) error {

	key := fmt.Sprintf(VideoCommentsKey, videoID, sortType, page)

	// 将评论ID列表序列化为JSON
	data, err := json.Marshal(comments)
	if err != nil {
		return fmt.Errorf("failed to marshal comment list: %w", err)
	}

	// 根据是否为热门评论设置不同的过期时间
	expire := ccm.normalCommentExpire
	if sortType == "hot" {
		expire = ccm.hotCommentExpire
	}

	return ccm.client.Set(ctx, key, data, expire).Err()
}

// GetCachedCommentList 获取缓存的评论列表
func (ccm *CommentCacheManager) GetCachedCommentList(ctx context.Context, videoID int64,
	sortType string, page int) ([]int64, error) {

	key := fmt.Sprintf(VideoCommentsKey, videoID, sortType, page)

	data, err := ccm.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 缓存未命中
		}
		return nil, fmt.Errorf("failed to get cached comment list: %w", err)
	}

	var comments []int64
	if err := json.Unmarshal([]byte(data), &comments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal comment list: %w", err)
	}

	return comments, nil
}

// CacheCommentDetail 缓存评论详情
func (ccm *CommentCacheManager) CacheCommentDetail(ctx context.Context, comment *model.Comment) error {
	key := fmt.Sprintf(CommentDetailKey, comment.CommentId)

	data, err := json.Marshal(comment)
	if err != nil {
		return fmt.Errorf("failed to marshal comment detail: %w", err)
	}

	return ccm.client.Set(ctx, key, data, ccm.normalCommentExpire).Err()
}

// GetCachedCommentDetail 获取缓存的评论详情
func (ccm *CommentCacheManager) GetCachedCommentDetail(ctx context.Context, commentID int64) (*model.Comment, error) {
	key := fmt.Sprintf(CommentDetailKey, commentID)

	data, err := ccm.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 缓存未命中
		}
		return nil, fmt.Errorf("failed to get cached comment detail: %w", err)
	}

	var comment model.Comment
	if err := json.Unmarshal([]byte(data), &comment); err != nil {
		return nil, fmt.Errorf("failed to unmarshal comment detail: %w", err)
	}

	return &comment, nil
}

// IncrementCommentLikeCount 增加评论点赞数
func (ccm *CommentCacheManager) IncrementCommentLikeCount(ctx context.Context, commentID int64, delta int64) error {
	key := fmt.Sprintf(CommentLikeCountKey, commentID)

	// 使用Redis的INCRBY命令原子性地增加计数
	_, err := ccm.client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return fmt.Errorf("failed to increment comment like count: %w", err)
	}

	// 设置过期时间
	ccm.client.Expire(ctx, key, ccm.counterExpire)
	return nil
}

// GetCommentLikeCount 获取评论点赞数
func (ccm *CommentCacheManager) GetCommentLikeCount(ctx context.Context, commentID int64) (int64, error) {
	key := fmt.Sprintf(CommentLikeCountKey, commentID)

	count, err := ccm.client.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 缓存未命中，返回0
		}
		return 0, fmt.Errorf("failed to get comment like count: %w", err)
	}

	return count, nil
}

// SetCommentLikeCount 设置评论点赞数
func (ccm *CommentCacheManager) SetCommentLikeCount(ctx context.Context, commentID int64, count int64) error {
	key := fmt.Sprintf(CommentLikeCountKey, commentID)
	return ccm.client.Set(ctx, key, count, ccm.counterExpire).Err()
}

// IncrementVideoCommentCount 增加视频评论总数
func (ccm *CommentCacheManager) IncrementVideoCommentCount(ctx context.Context, videoID int64, delta int64) error {
	key := fmt.Sprintf(VideoCommentCountKey, videoID)

	_, err := ccm.client.IncrBy(ctx, key, delta).Result()
	if err != nil {
		return fmt.Errorf("failed to increment video comment count: %w", err)
	}

	ccm.client.Expire(ctx, key, ccm.counterExpire)
	return nil
}

// GetVideoCommentCount 获取视频评论总数
func (ccm *CommentCacheManager) GetVideoCommentCount(ctx context.Context, videoID int64) (int64, error) {
	key := fmt.Sprintf(VideoCommentCountKey, videoID)

	count, err := ccm.client.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return -1, nil // 缓存未命中
		}
		return 0, fmt.Errorf("failed to get video comment count: %w", err)
	}

	return count, nil
}

// SetVideoCommentCount 设置视频评论总数
func (ccm *CommentCacheManager) SetVideoCommentCount(ctx context.Context, videoID int64, count int64) error {
	key := fmt.Sprintf(VideoCommentCountKey, videoID)
	return ccm.client.Set(ctx, key, count, ccm.counterExpire).Err()
}

// CacheCommentHotScore 缓存评论热度分数
func (ccm *CommentCacheManager) CacheCommentHotScore(ctx context.Context, commentID int64, score float64) error {
	key := fmt.Sprintf(CommentHotScoreKey, commentID)
	return ccm.client.Set(ctx, key, score, ccm.hotCommentExpire).Err()
}

// GetCommentHotScore 获取评论热度分数
func (ccm *CommentCacheManager) GetCommentHotScore(ctx context.Context, commentID int64) (float64, error) {
	key := fmt.Sprintf(CommentHotScoreKey, commentID)

	score, err := ccm.client.Get(ctx, key).Float64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // 缓存未命中
		}
		return 0, fmt.Errorf("failed to get comment hot score: %w", err)
	}

	return score, nil
}

// InvalidateVideoCommentCache 清除视频相关的评论缓存
func (ccm *CommentCacheManager) InvalidateVideoCommentCache(ctx context.Context, videoID int64) error {
	// 构建模式匹配键
	patterns := []string{
		fmt.Sprintf("video:comments:%d:*", videoID),
		fmt.Sprintf("video:comment_count:%d", videoID),
	}

	for _, pattern := range patterns {
		keys, err := ccm.client.Keys(ctx, pattern).Result()
		if err != nil {
			hlog.Warnf("Failed to get keys for pattern %s: %v", pattern, err)
			continue
		}

		if len(keys) > 0 {
			if err := ccm.client.Del(ctx, keys...).Err(); err != nil {
				hlog.Warnf("Failed to delete keys %v: %v", keys, err)
			}
		}
	}

	return nil
}

// InvalidateCommentCache 清除评论相关的缓存
func (ccm *CommentCacheManager) InvalidateCommentCache(ctx context.Context, commentID int64) error {
	keys := []string{
		fmt.Sprintf(CommentDetailKey, commentID),
		fmt.Sprintf(CommentLikeCountKey, commentID),
		fmt.Sprintf(CommentHotScoreKey, commentID),
	}

	// 同时清除子评论列表缓存
	childrenPattern := fmt.Sprintf("comment:children:%d:*", commentID)
	childrenKeys, err := ccm.client.Keys(ctx, childrenPattern).Result()
	if err == nil {
		keys = append(keys, childrenKeys...)
	}

	if len(keys) > 0 {
		return ccm.client.Del(ctx, keys...).Err()
	}

	return nil
}

// BatchCacheCommentDetails 批量缓存评论详情
func (ccm *CommentCacheManager) BatchCacheCommentDetails(ctx context.Context, comments []*model.Comment) error {
	if len(comments) == 0 {
		return nil
	}

	pipe := ccm.client.Pipeline()

	for _, comment := range comments {
		key := fmt.Sprintf(CommentDetailKey, comment.CommentId)
		data, err := json.Marshal(comment)
		if err != nil {
			hlog.Warnf("Failed to marshal comment %d: %v", comment.CommentId, err)
			continue
		}
		pipe.Set(ctx, key, data, ccm.normalCommentExpire)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// BatchGetCommentDetails 批量获取评论详情
func (ccm *CommentCacheManager) BatchGetCommentDetails(ctx context.Context, commentIDs []int64) (map[int64]*model.Comment, error) {
	if len(commentIDs) == 0 {
		return make(map[int64]*model.Comment), nil
	}

	pipe := ccm.client.Pipeline()
	cmds := make(map[int64]*redis.StringCmd)

	for _, commentID := range commentIDs {
		key := fmt.Sprintf(CommentDetailKey, commentID)
		cmds[commentID] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to batch get comment details: %w", err)
	}

	result := make(map[int64]*model.Comment)
	for commentID, cmd := range cmds {
		data, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				continue // 缓存未命中，跳过
			}
			hlog.Warnf("Failed to get comment %d from cache: %v", commentID, err)
			continue
		}

		var comment model.Comment
		if err := json.Unmarshal([]byte(data), &comment); err != nil {
			hlog.Warnf("Failed to unmarshal comment %d: %v", commentID, err)
			continue
		}

		result[commentID] = &comment
	}

	return result, nil
}

// WarmUpCache 预热缓存
func (ccm *CommentCacheManager) WarmUpCache(ctx context.Context, videoID int64, comments []*model.Comment) error {
	// 批量缓存评论详情
	if err := ccm.BatchCacheCommentDetails(ctx, comments); err != nil {
		hlog.Warnf("Failed to batch cache comment details for video %d: %v", videoID, err)
	}

	// 缓存评论ID列表（按最新排序）
	commentIDs := make([]int64, len(comments))
	for i, comment := range comments {
		commentIDs[i] = comment.CommentId
	}

	if err := ccm.CacheCommentList(ctx, videoID, "latest", 1, commentIDs); err != nil {
		hlog.Warnf("Failed to cache comment list for video %d: %v", videoID, err)
	}

	return nil
}
