package handlers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"HuaTug.com/pkg/infra/cache"
	jwt "HuaTug.com/pkg/auth"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/gomodule/redigo/redis"
)

// NotificationItem 通知项
type NotificationItem struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`         // like, comment, follow, system
	Title      string                 `json:"title"`        // 通知标题
	Content    string                 `json:"content"`      // 通知内容
	FromUserId int64                  `json:"from_user_id"` // 触发通知的用户
	FromUser   string                 `json:"from_user"`    // 触发通知的用户名
	TargetId   int64                  `json:"target_id"`    // 目标ID（视频/评论等）
	TargetType string                 `json:"target_type"`  // video, comment
	IsRead     bool                   `json:"is_read"`      // 是否已读
	CreatedAt  string                 `json:"created_at"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

// NotificationListResponse 通知列表响应
type NotificationListResponse struct {
	List        []NotificationItem `json:"list"`
	Total       int64              `json:"total"`
	UnreadCount int64              `json:"unread_count"`
}

// GetNotificationListV2 获取通知列表
func GetNotificationListV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64

	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	UserId = utils.Transfer(v)

	// 获取分页参数
	pageNum, _ := strconv.ParseInt(string(c.Query("page_num")), 10, 64)
	pageSize, _ := strconv.ParseInt(string(c.Query("page_size")), 10, 64)
	notifyType := string(c.Query("type")) // all, like, comment, follow, system

	if pageNum <= 0 {
		pageNum = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	// 从Redis获取通知列表
	notifications, total, err := getNotificationsFromCache(UserId, notifyType, pageNum, pageSize)
	if err != nil {
		hlog.Errorf("Failed to get notifications for user %d: %v", UserId, err)
		// 返回空列表而不是错误
		notifications = []NotificationItem{}
		total = 0
	}

	// 获取未读数量
	unreadCount := getUnreadNotificationCount(UserId)

	resp := NotificationListResponse{
		List:        notifications,
		Total:       total,
		UnreadCount: unreadCount,
	}

	SendResponse(c, errno.Success, resp)
}

// MarkNotificationReadV2 标记通知为已读
func MarkNotificationReadV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64

	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	UserId = utils.Transfer(v)

	// 获取通知ID
	notificationId := string(c.Query("notification_id"))
	markAll := string(c.Query("mark_all")) == "true"

	if markAll {
		// 标记所有通知为已读
		if err := markAllNotificationsRead(UserId); err != nil {
			hlog.Errorf("Failed to mark all notifications read for user %d: %v", UserId, err)
			SendResponse(c, errno.ServiceErr, nil)
			return
		}
	} else if notificationId != "" {
		// 标记单个通知为已读
		if err := markNotificationRead(UserId, notificationId); err != nil {
			hlog.Errorf("Failed to mark notification %s read: %v", notificationId, err)
			SendResponse(c, errno.ServiceErr, nil)
			return
		}
	}

	SendResponse(c, errno.Success, nil)
}

// GetUnreadCountV2 获取未读通知数量
func GetUnreadCountV2(ctx context.Context, c *app.RequestContext) {
	var err error
	var v interface{}
	var UserId int64

	if v, err = jwt.ConvertJWTPayloadToString(ctx, c); err != nil {
		SendResponse(c, errno.ConvertErr(err), nil)
		return
	}
	UserId = utils.Transfer(v)

	count := getUnreadNotificationCount(UserId)

	SendResponse(c, errno.Success, map[string]int64{
		"unread_count": count,
	})
}

// ========== 辅助函数（使用 redigo） ==========

func getNotificationsFromCache(userId int64, notifyType string, pageNum, pageSize int64) ([]NotificationItem, int64, error) {
	conn := cache.GetRedis()
	defer conn.Close()

	key := getNotificationKey(userId, notifyType)
	offset := (pageNum - 1) * pageSize

	// 获取总数
	total, err := redis.Int64(conn.Do("ZCARD", key))
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据（按时间倒序）
	results, err := redis.Strings(conn.Do("ZREVRANGE", key, offset, offset+pageSize-1))
	if err != nil {
		return nil, 0, err
	}

	notifications := make([]NotificationItem, 0, len(results))
	for _, r := range results {
		var item NotificationItem
		if err := json.Unmarshal([]byte(r), &item); err != nil {
			continue
		}
		notifications = append(notifications, item)
	}

	return notifications, total, nil
}

func getUnreadNotificationCount(userId int64) int64 {
	conn := cache.GetRedis()
	defer conn.Close()

	key := getUnreadCountKey(userId)
	count, err := redis.Int64(conn.Do("GET", key))
	if err != nil {
		return 0
	}
	return count
}

func markNotificationRead(userId int64, notificationId string) error {
	conn := cache.GetRedis()
	defer conn.Close()

	mainKey := getNotificationKey(userId, "all")

	// 扫描主 key 中所有 members，找到匹配 notificationId 的条目
	members, err := redis.Strings(conn.Do("ZRANGE", mainKey, 0, -1))
	if err != nil {
		return err
	}

	var matchedMember string
	var matchedType string
	for _, m := range members {
		if strings.Contains(m, `"id":"`+notificationId+`"`) {
			matchedMember = m
			// 解析 type 字段以构造类型 key
			var item NotificationItem
			if err := json.Unmarshal([]byte(m), &item); err == nil {
				matchedType = item.Type
			}
			break
		}
	}

	if matchedMember == "" {
		return nil // 未找到该通知，可能已被删除
	}

	// 从主 key 中删除
	if _, err := conn.Do("ZREM", mainKey, matchedMember); err != nil {
		hlog.Errorf("Failed to ZREM from main key: %v", err)
	}

	// 从类型 key 中删除
	if matchedType != "" {
		typeKey := getNotificationKey(userId, matchedType)
		if _, err := conn.Do("ZREM", typeKey, matchedMember); err != nil {
			hlog.Errorf("Failed to ZREM from type key %s: %v", typeKey, err)
		}
	}

	// 减少未读计数，防止小于 0
	unreadKey := getUnreadCountKey(userId)
	newCount, err := redis.Int64(conn.Do("DECR", unreadKey))
	if err == nil && newCount < 0 {
		conn.Do("SET", unreadKey, 0)
	}

	return nil
}

func markAllNotificationsRead(userId int64) error {
	conn := cache.GetRedis()
	defer conn.Close()

	// 删除主 key 和所有类型 key
	mainKey := getNotificationKey(userId, "all")
	typeKeys := []string{
		getNotificationKey(userId, "video_like"),
		getNotificationKey(userId, "comment"),
		getNotificationKey(userId, "comment_like"),
		getNotificationKey(userId, "comment_reply"),
		getNotificationKey(userId, "follow"),
		getNotificationKey(userId, "system"),
	}

	// 删除主 key
	if _, err := conn.Do("DEL", mainKey); err != nil {
		hlog.Errorf("Failed to DEL main notification key: %v", err)
	}

	// 删除所有类型 key
	for _, tk := range typeKeys {
		if _, err := conn.Do("DEL", tk); err != nil {
			hlog.Errorf("Failed to DEL type key %s: %v", tk, err)
		}
	}

	// 重置未读计数，设置7天过期
	unreadKey := getUnreadCountKey(userId)
	_, err := conn.Do("SETEX", unreadKey, 7*24*3600, 0)
	return err
}

func getNotificationKey(userId int64, notifyType string) string {
	if notifyType == "" || notifyType == "all" {
		return "notification:user:" + strconv.FormatInt(userId, 10)
	}
	return "notification:user:" + strconv.FormatInt(userId, 10) + ":" + notifyType
}

func getUnreadCountKey(userId int64) string {
	return "notification:unread:" + strconv.FormatInt(userId, 10)
}
