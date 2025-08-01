package redis

import (
	"context"
	"crypto/md5"
	"fmt"
	"strconv"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"github.com/go-redis/redis"
	"github.com/pkg/errors"
)

func PutCommentLikeInfo(commentId int64, userIdList *[]int64) error {
	//创建管道
	pipe := redisDBCommentInfo.TxPipeline()
	pipe.Del("l_comment:" + fmt.Sprint(commentId))
	pipe.Del("nl_comment:" + fmt.Sprint(commentId))
	for _, item := range *userIdList {
		pipe.SAdd("l_comment:"+fmt.Sprint(commentId), item)
	}
	if _, err := pipe.Exec(); err != nil {
		return errors.WithMessage(err, "Pipe Exec failed")
	}
	return nil
}

func GetCommentLikeCount(commentId int64) (int64, error) {
	//	SCard用于获取集合中的元素数量
	countOld, err := redisDBCommentInfo.SCard("l_comment:" + fmt.Sprint(commentId)).Result()
	if err != nil {
		return -1, err
	}
	//
	countNew, err := redisDBCommentInfo.ZCount("nl_comment:"+fmt.Sprint(commentId), "1", "1").Result()
	if err != nil {
		return -1, err
	}
	return countOld + countNew, nil
}

func GetCommentLikeList(commentId int64) (*[]string, error) {
	list, err := redisDBCommentInfo.SMembers("l_comment:" + fmt.Sprint(commentId)).Result()
	if err != nil {
		return nil, err
	}
	nl_commentist, err := GetNewUpdateCommentLikeList(commentId)
	if err != nil {
		return nil, err
	}
	list = append(list, *nl_commentist...)
	return &list, nil
}

func GetNewUpdateCommentLikeList(commentId int64) (*[]string, error) {
	list, err := redisDBCommentInfo.ZRangeByScore("nl_comment:"+fmt.Sprint(commentId), redis.ZRangeBy{Min: "1", Max: "1"}).Result()
	if err != nil {
		return nil, err
	}
	return &list, err
}

func GetNewDeleteCommentLikeList(commentId int64) (*[]string, error) {
	//这个过程类似于点赞过程发生一次 然后取消点赞过程发生一次 所有是两次
	list, err := redisDBCommentInfo.ZRangeByScore("nl_comment:"+fmt.Sprint(commentId), redis.ZRangeBy{Min: "2", Max: "2"}).Result()
	if err != nil {
		return nil, err
	}
	return &list, err
}

func AppendCommentLikeInfo(commentId, userId int64) error {
	exists, err := redisDBCommentInfo.ZCount("nl_comment:"+fmt.Sprint(commentId), "1", "1").Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if exists != 0 {
		return fmt.Errorf("user:%d has already liked video:%d", userId, commentId)
	}
	if _, err := redisDBCommentInfo.ZAdd("nl_comment:"+fmt.Sprint(commentId), redis.Z{Score: 1, Member: userId}).Result(); err != nil {
		return err
	}
	_, err = redisDBCommentInfo.ZAdd("nl_comment:"+fmt.Sprint(commentId), redis.Z{Score: 1, Member: userId}).Result()
	if err != nil {
		return err
	}
	if _, err := redisDBCommentInfo.SRem("l_comment:"+fmt.Sprint(commentId), userId).Result(); err != nil {
		return err
	}
	return nil
}

func AppendCommentLikeInfoToStaticSpace(commentId, userId int64) error {
	if _, err := redisDBCommentInfo.SAdd("l_comment:"+fmt.Sprint(commentId), userId).Result(); err != nil {
		return err
	}
	return nil
}

// ToDo
func DeleteCommentLikeInfoFromDynamicSpace(commentId, userId int64) error {
	if _, err := redisDBCommentInfo.ZRem("nl_comment:"+fmt.Sprint(commentId), userId).Result(); err != nil {
		return err
	}
	return nil
}

func RemoveCommentLikeInfo(commentId, userId int64) error {
	exists, err := redisDBCommentInfo.ZCount("nl_comment:"+fmt.Sprint(commentId), "2", "2").Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if exists != 0 {
		return fmt.Errorf("user:%d has already liked video:%d", userId, commentId)
	}
	if _, err := redisDBCommentInfo.ZAdd("nl_comment:"+fmt.Sprint(commentId), redis.Z{Score: 1, Member: userId}).Result(); err != nil {
		return err
	}
	_, err = redisDBCommentInfo.ZAdd("nl_comment:"+fmt.Sprint(commentId), redis.Z{Score: 2, Member: userId}).Result()
	if err != nil {
		return err
	}
	if _, err := redisDBCommentInfo.SRem("l_comment:"+fmt.Sprint(commentId), userId).Result(); err != nil {
		return err
	}
	return nil
}

// IncrCommentLikeCount 增加评论点赞数
func IncrCommentLikeCount(commentId int64) error {
	_, err := redisDBCommentInfo.Incr(fmt.Sprintf("comment_like_count:%d", commentId)).Result()
	return err
}

// DecrCommentLikeCount 减少评论点赞数
func DecrCommentLikeCount(commentId int64) error {
	_, err := redisDBCommentInfo.Decr(fmt.Sprintf("comment_like_count:%d", commentId)).Result()
	return err
}

// 删除所有评论
func DeleteAllComment(commentIdList []int64) error {
	var (
		childList   *[]int64
		err         error
		commentPipe = redisDBCommentInfo.TxPipeline()
	)

	for _, commentId := range commentIdList {
		commentPipe.Unlink("l_comment:" + fmt.Sprint(commentId))
		commentPipe.Unlink("nl_comment:" + fmt.Sprint(commentId))

		if childList, err = db.GetCommentChildList(context.Background(), commentId); err != nil {
			return err
		}

		for _, item := range *childList {
			commentPipe.Unlink("l_comment:" + fmt.Sprint(item))
			commentPipe.Unlink("nl_comment:" + fmt.Sprint(item))
		}
	}

	if _, err := commentPipe.Exec(); err != nil {
		return err
	}

	return nil
}

// 删除一条评论及其子评论
func DeleteCommentAndAllAbout(commentId int64) error {
	return DeleteAllComment([]int64{commentId})
}

// GetCommentRateLimit gets the current comment count for rate limiting
func GetCommentRateLimit(key string) (int64, error) {
	count, err := redisDBCommentInfo.Get(key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	result, err := strconv.ParseInt(count, 10, 64)
	if err != nil {
		return 0, err
	}
	return result, nil
}

// IncrementCommentRateLimit increments the comment rate limit counter
func IncrementCommentRateLimit(key string, expireSeconds int) error {
	pipe := redisDBCommentInfo.TxPipeline()
	pipe.Incr(key)
	pipe.Expire(key, time.Duration(expireSeconds)*time.Second)
	_, err := pipe.Exec()
	return err
}

// CheckDuplicateComment checks if a comment is duplicate within time window
func CheckDuplicateComment(userId int64, content string, timeWindowSeconds int) (bool, error) {
	// Create hash of the content for efficient storage and comparison
	contentHash := fmt.Sprintf("%x", md5.Sum([]byte(content)))
	key := fmt.Sprintf("comment_hash:%d:%s", userId, contentHash)

	// Check if the hash exists
	exists, err := redisDBCommentInfo.Exists(key).Result()
	if err != nil {
		return false, err
	}

	return exists > 0, nil
}

// StoreCommentHash stores comment hash for duplicate detection
func StoreCommentHash(userId int64, content string, expireSeconds int) error {
	contentHash := fmt.Sprintf("%x", md5.Sum([]byte(content)))
	key := fmt.Sprintf("comment_hash:%d:%s", userId, contentHash)

	return redisDBCommentInfo.Set(key, "1", time.Duration(expireSeconds)*time.Second).Err()
}

// GetUserCommentFrequency gets user's comment frequency in recent time
func GetUserCommentFrequency(userId int64, timeWindowSeconds int) (int64, error) {
	key := fmt.Sprintf("user_comment_freq:%d", userId)
	now := time.Now().Unix()
	minScore := now - int64(timeWindowSeconds)

	count, err := redisDBCommentInfo.ZCount(key, fmt.Sprintf("%d", minScore), fmt.Sprintf("%d", now)).Result()
	if err != nil {
		return 0, err
	}

	return count, nil
}

// RecordUserCommentTime records when user made a comment for frequency analysis
func RecordUserCommentTime(userId int64, expireSeconds int) error {
	key := fmt.Sprintf("user_comment_freq:%d", userId)
	now := time.Now().Unix()

	pipe := redisDBCommentInfo.TxPipeline()
	pipe.ZAdd(key, redis.Z{Score: float64(now), Member: now})
	pipe.Expire(key, time.Duration(expireSeconds)*time.Second)

	// Clean old entries to prevent memory bloat
	minScore := now - int64(expireSeconds)
	pipe.ZRemRangeByScore(key, "-inf", fmt.Sprintf("%d", minScore))

	_, err := pipe.Exec()
	return err
}

// GetCommentSpamScore calculates a spam score for content analysis
func GetCommentSpamScore(userId int64, content string) (float64, error) {
	// This is a basic implementation - in production you'd use more sophisticated ML models
	score := 0.0

	// Check user's recent comment frequency
	freq, err := GetUserCommentFrequency(userId, 3600) // Last hour
	if err != nil {
		return 0, err
	}

	// Higher frequency increases spam score
	if freq > 50 {
		score += 0.8
	} else if freq > 20 {
		score += 0.5
	} else if freq > 10 {
		score += 0.3
	}

	// Content-based scoring (basic implementation)
	contentLength := len(content)
	if contentLength < 5 {
		score += 0.4 // Very short comments are often spam
	}

	// Check for excessive repetition
	if hasRepeatedPatterns(content) {
		score += 0.6
	}

	return score, nil
}

// hasRepeatedPatterns is a helper function to detect repeated patterns in text
func hasRepeatedPatterns(content string) bool {
	if len(content) < 10 {
		return false
	}

	// Simple pattern detection - count repeated substrings
	for i := 2; i <= len(content)/2; i++ {
		pattern := content[:i]
		count := 0
		for j := 0; j <= len(content)-i; j += i {
			if j+i <= len(content) && content[j:j+i] == pattern {
				count++
			}
		}
		if count >= 3 { // Pattern repeats 3+ times
			return true
		}
	}

	return false
}
