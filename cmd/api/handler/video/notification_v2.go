package handlers

import (
	"context"
	"encoding/json"
	"strconv"

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

	// 更新未读计数
	key := getUnreadCountKey(userId)
	_, err := conn.Do("DECR", key)
	return err
}

func markAllNotificationsRead(userId int64) error {
	conn := cache.GetRedis()
	defer conn.Close()

	// 重置未读计数，设置7天过期
	key := getUnreadCountKey(userId)
	_, err := conn.Do("SETEX", key, 7*24*3600, 0)
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
