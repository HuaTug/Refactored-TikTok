package service

import (
	"context"
	"strconv"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/cmd/model"
	"HuaTug.com/kitex_gen/interactions"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/pkg/errors"
)

// NotificationService handles notification operations
type NotificationService struct {
	ctx context.Context
}

func NewNotificationService(ctx context.Context) *NotificationService {
	return &NotificationService{ctx: ctx}
}

// GetNotifications retrieves user notifications with pagination
func (s *NotificationService) GetNotifications(req *interactions.GetNotificationsRequest) ([]*interactions.NotificationInfo, int64, int64, error) {
	// 1. Query notifications from database
	var notifications []model.Notification
	query := db.DB.WithContext(s.ctx).Model(&model.Notification{}).
		Where("user_id = ?", req.UserId)

	// Filter by notification type if specified
	if req.NotificationType != "" {
		typeMap := map[string]int8{
			"like":     1,
			"comment":  2,
			"follow":   3,
			"mention":  4,
			"system":   5,
			"activity": 6,
		}
		if typeVal, ok := typeMap[req.NotificationType]; ok {
			query = query.Where("notification_type = ?", typeVal)
		}
	}

	// Get total count
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, errors.WithMessage(err, "failed to count notifications")
	}

	// Get unread count
	var unreadCount int64
	if err := db.DB.WithContext(s.ctx).Model(&model.Notification{}).
		Where("user_id = ? AND is_read = 0", req.UserId).
		Count(&unreadCount).Error; err != nil {
		hlog.Warnf("Failed to count unread notifications: %v", err)
	}

	// Paginate
	page := req.PageNum
	pageSize := req.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	if err := query.Order("created_at DESC").
		Limit(int(pageSize)).
		Offset(int((page - 1) * pageSize)).
		Find(&notifications).Error; err != nil {
		return nil, 0, 0, errors.WithMessage(err, "failed to get notifications")
	}

	// 2. Convert to response format
	result := make([]*interactions.NotificationInfo, 0, len(notifications))
	typeNames := map[int8]string{
		1: "like",
		2: "comment",
		3: "follow",
		4: "mention",
		5: "system",
		6: "activity",
	}

	for _, n := range notifications {
		info := &interactions.NotificationInfo{
			NotificationId:   n.NotificationId,
			NotificationType: typeNames[n.NotificationType],
			IsRead:           n.IsRead == 1,
			CreatedAt:        n.CreatedAt.Format(time.RFC3339),
		}

		if n.SenderId != nil {
			info.FromUserId = *n.SenderId
		}
		if n.Content != nil {
			info.Content = *n.Content
		}
		if n.TargetId != nil {
			info.TargetId = *n.TargetId
		}

		// Try to get sender info (best effort)
		if n.SenderId != nil {
			info.FromUserName = "user_" + strconv.FormatInt(*n.SenderId, 10)
		}

		result = append(result, info)
	}

	return result, totalCount, unreadCount, nil
}

// MarkNotificationRead marks notifications as read
func (s *NotificationService) MarkNotificationRead(req *interactions.MarkNotificationReadRequest) (int64, error) {
	if len(req.NotificationIds) == 0 {
		// Mark all as read
		result := db.DB.WithContext(s.ctx).Model(&model.Notification{}).
			Where("user_id = ? AND is_read = 0", req.UserId).
			Updates(map[string]interface{}{
				"is_read": 1,
				"read_at": time.Now(),
			})
		if result.Error != nil {
			return 0, errors.WithMessage(result.Error, "failed to mark all notifications as read")
		}
		hlog.Infof("Marked all notifications as read for user %d, affected: %d", req.UserId, result.RowsAffected)
		return result.RowsAffected, nil
	}

	// Mark specific notifications as read
	result := db.DB.WithContext(s.ctx).Model(&model.Notification{}).
		Where("notification_id IN ? AND user_id = ?", req.NotificationIds, req.UserId).
		Updates(map[string]interface{}{
			"is_read": 1,
			"read_at": time.Now(),
		})
	if result.Error != nil {
		return 0, errors.WithMessage(result.Error, "failed to mark notifications as read")
	}

	hlog.Infof("Marked %d notifications as read for user %d", result.RowsAffected, req.UserId)
	return result.RowsAffected, nil
}
