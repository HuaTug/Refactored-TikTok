// Package service provides notification aggregation functionality.
// This implements features like "张三等10人点赞了你的视频" aggregated notifications.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"HuaTug.com/config/cache"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/gomodule/redigo/redis"
	"github.com/google/uuid"
)

// NotificationAggregator 通知聚合器
// 用于将同类型的通知聚合成 "xxx等N人..." 的形式
type NotificationAggregator struct {
	mu sync.RWMutex
}

// AggregationConfig 聚合配置
type AggregationConfig struct {
	// 聚合时间窗口（秒）
	TimeWindowSeconds int64
	// 最小聚合数量（达到此数量才聚合）
	MinAggregateCount int
	// 最大显示用户数
	MaxDisplayUsers int
}

// AggregatedNotification 聚合后的通知
type AggregatedNotification struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"`
	Title           string                 `json:"title"`
	Content         string                 `json:"content"`
	TargetID        int64                  `json:"target_id"`
	TargetType      string                 `json:"target_type"`
	IsRead          bool                   `json:"is_read"`
	CreatedAt       string                 `json:"created_at"`
	AggregatedCount int                    `json:"aggregated_count"`
	Users           []AggregatedUser       `json:"users"`
	Extra           map[string]interface{} `json:"extra"`
}

// AggregatedUser 聚合通知中的用户信息
type AggregatedUser struct {
	UserID   int64  `json:"user_id"`
	UserName string `json:"user_name"`
	Avatar   string `json:"avatar"`
}

// DefaultAggregationConfig 默认聚合配置
var DefaultAggregationConfig = &AggregationConfig{
	TimeWindowSeconds: 3600, // 1小时内的通知可以聚合
	MinAggregateCount: 2,    // 至少2个才聚合
	MaxDisplayUsers:   3,    // 最多显示3个用户名
}

// NewNotificationAggregator 创建通知聚合器
func NewNotificationAggregator() *NotificationAggregator {
	return &NotificationAggregator{}
}

// getAggregationKey 获取聚合键
// 格式: notification:agg:{user_id}:{type}:{target_id}
func (na *NotificationAggregator) getAggregationKey(receiverID int64, notificationType string, targetID int64) string {
	return fmt.Sprintf("notification:agg:%d:%s:%d", receiverID, notificationType, targetID)
}

// getPendingAggregationKey 获取待聚合通知键
// 格式: notification:pending:{user_id}:{type}:{target_id}
func (na *NotificationAggregator) getPendingAggregationKey(receiverID int64, notificationType string, targetID int64) string {
	return fmt.Sprintf("notification:pending:%d:%s:%d", receiverID, notificationType, targetID)
}

// ShouldAggregate 判断是否应该聚合通知
func (na *NotificationAggregator) ShouldAggregate(notificationType string) bool {
	// 以下类型的通知支持聚合
	aggregatableTypes := map[string]bool{
		"video_like":   true,  // 视频点赞
		"comment_like": true,  // 评论点赞
		"follow":       false, // 关注暂不聚合
		"comment":      false, // 评论暂不聚合（内容不同）
		"mention":      false, // @提及暂不聚合
	}
	return aggregatableTypes[notificationType]
}

// AddToAggregation 添加通知到聚合队列
// 返回: (是否需要立即发送聚合通知, 错误)
func (na *NotificationAggregator) AddToAggregation(ctx context.Context, event *mq.NotificationEvent, config *AggregationConfig) (bool, error) {
	if config == nil {
		config = DefaultAggregationConfig
	}

	conn := cache.GetRedis()
	defer conn.Close()

	aggKey := na.getAggregationKey(event.ReceiverID, event.Type, event.TargetID)
	pendingKey := na.getPendingAggregationKey(event.ReceiverID, event.Type, event.TargetID)

	// 构建用户信息
	userInfo := map[string]interface{}{
		"user_id":   event.SenderID,
		"user_name": "", // 可以从用户服务获取
		"timestamp": event.Timestamp,
	}
	userInfoJSON, _ := json.Marshal(userInfo)

	// 添加到待聚合列表（ZSet，按时间戳排序）
	score := float64(event.Timestamp)
	if _, err := conn.Do("ZADD", pendingKey, score, string(userInfoJSON)); err != nil {
		return false, fmt.Errorf("failed to add to pending aggregation: %w", err)
	}

	// 设置过期时间
	if _, err := conn.Do("EXPIRE", pendingKey, config.TimeWindowSeconds+60); err != nil {
		hlog.Warnf("Failed to set expire for pending key: %v", err)
	}

	// 移除时间窗口外的旧数据
	cutoffTime := time.Now().Unix() - config.TimeWindowSeconds
	if _, err := conn.Do("ZREMRANGEBYSCORE", pendingKey, "-inf", cutoffTime); err != nil {
		hlog.Warnf("Failed to remove old entries: %v", err)
	}

	// 获取当前聚合数量
	count, err := redis.Int(conn.Do("ZCARD", pendingKey))
	if err != nil {
		return false, fmt.Errorf("failed to get aggregation count: %w", err)
	}

	// 判断是否达到聚合阈值
	if count >= config.MinAggregateCount {
		// 检查是否已经有聚合通知
		exists, _ := redis.Bool(conn.Do("EXISTS", aggKey))
		if !exists {
			// 首次达到阈值，需要发送聚合通知
			return true, nil
		}
		// 已有聚合通知，只需更新
		return false, na.updateAggregatedNotification(ctx, event, count, config)
	}

	return false, nil
}

// CreateAggregatedNotification 创建聚合通知
func (na *NotificationAggregator) CreateAggregatedNotification(ctx context.Context, event *mq.NotificationEvent, config *AggregationConfig) (*AggregatedNotification, error) {
	if config == nil {
		config = DefaultAggregationConfig
	}

	conn := cache.GetRedis()
	defer conn.Close()

	pendingKey := na.getPendingAggregationKey(event.ReceiverID, event.Type, event.TargetID)

	// 获取所有待聚合的用户
	items, err := redis.Strings(conn.Do("ZRANGE", pendingKey, 0, -1))
	if err != nil {
		return nil, fmt.Errorf("failed to get pending items: %w", err)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no pending items for aggregation")
	}

	// 解析用户信息
	users := make([]AggregatedUser, 0, config.MaxDisplayUsers)
	for i, item := range items {
		if i >= config.MaxDisplayUsers {
			break
		}
		var userInfo map[string]interface{}
		if err := json.Unmarshal([]byte(item), &userInfo); err != nil {
			continue
		}
		userID, _ := userInfo["user_id"].(float64)
		userName, _ := userInfo["user_name"].(string)
		users = append(users, AggregatedUser{
			UserID:   int64(userID),
			UserName: userName,
		})
	}

	// 构建聚合通知内容
	content := na.buildAggregatedContent(event.Type, users, len(items))
	title := na.getAggregatedTitle(event.Type)

	aggNotification := &AggregatedNotification{
		ID:              uuid.New().String(),
		Type:            event.Type + "_aggregated",
		Title:           title,
		Content:         content,
		TargetID:        event.TargetID,
		TargetType:      na.getTargetType(event.Type),
		IsRead:          false,
		CreatedAt:       time.Now().Format("2006-01-02 15:04:05"),
		AggregatedCount: len(items),
		Users:           users,
		Extra: map[string]interface{}{
			"original_type": event.Type,
			"target_id":     event.TargetID,
		},
	}

	// 保存聚合通知到缓存
	if err := na.saveAggregatedNotification(ctx, event.ReceiverID, aggNotification, config); err != nil {
		return nil, err
	}

	return aggNotification, nil
}

// buildAggregatedContent 构建聚合通知内容
func (na *NotificationAggregator) buildAggregatedContent(notificationType string, users []AggregatedUser, totalCount int) string {
	if len(users) == 0 {
		return ""
	}

	// 获取第一个用户名
	firstUser := users[0].UserName
	if firstUser == "" {
		firstUser = fmt.Sprintf("用户%d", users[0].UserID)
	}

	// 根据通知类型构建内容
	actionText := na.getActionText(notificationType)

	if totalCount == 1 {
		return fmt.Sprintf("%s%s", firstUser, actionText)
	}

	otherCount := totalCount - 1
	return fmt.Sprintf("%s等%d人%s", firstUser, otherCount, actionText)
}

// getActionText 获取操作文本
func (na *NotificationAggregator) getActionText(notificationType string) string {
	actions := map[string]string{
		"video_like":   "点赞了你的视频",
		"comment_like": "点赞了你的评论",
		"follow":       "关注了你",
		"comment":      "评论了你的视频",
	}
	if action, ok := actions[notificationType]; ok {
		return action
	}
	return "与你互动"
}

// getAggregatedTitle 获取聚合通知标题
func (na *NotificationAggregator) getAggregatedTitle(notificationType string) string {
	titles := map[string]string{
		"video_like":   "视频获得多个点赞",
		"comment_like": "评论获得多个点赞",
		"follow":       "收到多个关注",
	}
	if title, ok := titles[notificationType]; ok {
		return title
	}
	return "收到多个互动"
}

// getTargetType 获取目标类型
func (na *NotificationAggregator) getTargetType(notificationType string) string {
	switch notificationType {
	case "video_like", "comment":
		return "video"
	case "comment_like":
		return "comment"
	case "follow":
		return "user"
	default:
		return "unknown"
	}
}

// saveAggregatedNotification 保存聚合通知到缓存
func (na *NotificationAggregator) saveAggregatedNotification(ctx context.Context, receiverID int64, notification *AggregatedNotification, config *AggregationConfig) error {
	conn := cache.GetRedis()
	defer conn.Close()

	// 使用原始类型作为聚合键的一部分
	originalType := notification.Extra["original_type"].(string)
	aggKey := na.getAggregationKey(receiverID, originalType, notification.TargetID)

	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal aggregated notification: %w", err)
	}

	// 保存聚合通知
	if _, err := conn.Do("SET", aggKey, string(data), "EX", config.TimeWindowSeconds+60); err != nil {
		return fmt.Errorf("failed to save aggregated notification: %w", err)
	}

	// 同时写入用户的通知列表
	userKey := "notification:user:" + strconv.FormatInt(receiverID, 10)
	score := float64(time.Now().Unix())
	if _, err := conn.Do("ZADD", userKey, score, string(data)); err != nil {
		hlog.Warnf("Failed to add aggregated notification to user list: %v", err)
	}

	hlog.CtxInfof(ctx, "Saved aggregated notification for user %d: %s", receiverID, notification.Content)
	return nil
}

// updateAggregatedNotification 更新已有的聚合通知
func (na *NotificationAggregator) updateAggregatedNotification(ctx context.Context, event *mq.NotificationEvent, newCount int, config *AggregationConfig) error {
	conn := cache.GetRedis()
	defer conn.Close()

	aggKey := na.getAggregationKey(event.ReceiverID, event.Type, event.TargetID)

	// 获取现有的聚合通知
	data, err := redis.String(conn.Do("GET", aggKey))
	if err != nil {
		// 如果不存在，创建新的
		_, err = na.CreateAggregatedNotification(ctx, event, config)
		return err
	}

	var notification AggregatedNotification
	if err := json.Unmarshal([]byte(data), &notification); err != nil {
		return fmt.Errorf("failed to unmarshal existing notification: %w", err)
	}

	// 更新计数和内容
	notification.AggregatedCount = newCount
	notification.Content = na.buildAggregatedContent(event.Type, notification.Users, newCount)
	notification.CreatedAt = time.Now().Format("2006-01-02 15:04:05")

	// 重新保存
	newData, _ := json.Marshal(notification)
	if _, err := conn.Do("SET", aggKey, string(newData), "EX", config.TimeWindowSeconds+60); err != nil {
		return fmt.Errorf("failed to update aggregated notification: %w", err)
	}

	// 更新用户通知列表中的记录
	userKey := "notification:user:" + strconv.FormatInt(event.ReceiverID, 10)
	// 先删除旧的聚合通知
	if _, err := conn.Do("ZREM", userKey, data); err != nil {
		hlog.Warnf("Failed to remove old aggregated notification: %v", err)
	}
	// 添加新的
	score := float64(time.Now().Unix())
	if _, err := conn.Do("ZADD", userKey, score, string(newData)); err != nil {
		hlog.Warnf("Failed to add updated aggregated notification: %v", err)
	}

	hlog.CtxInfof(ctx, "Updated aggregated notification for user %d: count=%d", event.ReceiverID, newCount)
	return nil
}

// GetAggregatedNotifications 获取用户的聚合通知列表
func (na *NotificationAggregator) GetAggregatedNotifications(ctx context.Context, userID int64, page, pageSize int) ([]*AggregatedNotification, int, error) {
	conn := cache.GetRedis()
	defer conn.Close()

	userKey := "notification:user:" + strconv.FormatInt(userID, 10)

	// 获取总数
	total, err := redis.Int(conn.Do("ZCARD", userKey))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}

	// 分页获取（按时间倒序）
	start := (page - 1) * pageSize
	stop := start + pageSize - 1
	items, err := redis.Strings(conn.Do("ZREVRANGE", userKey, start, stop))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get notifications: %w", err)
	}

	notifications := make([]*AggregatedNotification, 0, len(items))
	for _, item := range items {
		var notification AggregatedNotification
		if err := json.Unmarshal([]byte(item), &notification); err != nil {
			// 尝试解析为普通通知
			continue
		}
		notifications = append(notifications, &notification)
	}

	return notifications, total, nil
}

// ClearAggregation 清除指定的聚合数据
func (na *NotificationAggregator) ClearAggregation(ctx context.Context, receiverID int64, notificationType string, targetID int64) error {
	conn := cache.GetRedis()
	defer conn.Close()

	aggKey := na.getAggregationKey(receiverID, notificationType, targetID)
	pendingKey := na.getPendingAggregationKey(receiverID, notificationType, targetID)

	if _, err := conn.Do("DEL", aggKey, pendingKey); err != nil {
		return fmt.Errorf("failed to clear aggregation: %w", err)
	}

	return nil
}
