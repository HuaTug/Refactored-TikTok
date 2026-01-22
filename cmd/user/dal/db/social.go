package db

import (
	"context"
	"time"

	"HuaTug.com/cmd/model"
	"github.com/pkg/errors"
)

// ========================================
// Direct Message Operations
// ========================================

// SendMessage sends a direct message
func SendMessage(ctx context.Context, message *model.DirectMessage) error {
	message.CreatedAt = time.Now()
	message.IsRead = 0

	if err := DB.WithContext(ctx).Create(message).Error; err != nil {
		return errors.Wrapf(err, "SendMessage failed, err: %v", err)
	}

	// Update conversation
	UpdateConversationLastMessage(ctx, message.ConversationId, message.MessageId, message.Content, message.ReceiverId)

	return nil
}

// GetMessagesByConversation gets messages in a conversation
func GetMessagesByConversation(ctx context.Context, conversationId int64, page, pageSize int64) ([]*model.DirectMessage, int64, error) {
	db := DB.WithContext(ctx).Model(&model.DirectMessage{}).Where("conversation_id = ? AND deleted_at IS NULL", conversationId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetMessagesByConversation count failed, err: %v", err)
	}

	var messages []*model.DirectMessage
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&messages).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetMessagesByConversation query failed, err: %v", err)
	}

	return messages, total, nil
}

// MarkMessagesAsRead marks messages as read
func MarkMessagesAsRead(ctx context.Context, conversationId, userId int64) error {
	now := time.Now()
	if err := DB.WithContext(ctx).Model(&model.DirectMessage{}).
		Where("conversation_id = ? AND receiver_id = ? AND is_read = 0", conversationId, userId).
		Updates(map[string]interface{}{
			"is_read": 1,
			"read_at": now,
		}).Error; err != nil {
		return errors.Wrapf(err, "MarkMessagesAsRead failed, err: %v", err)
	}

	// Update conversation unread count
	ClearConversationUnread(ctx, conversationId, userId)

	return nil
}

// DeleteMessage soft deletes a message
func DeleteMessage(ctx context.Context, messageId, userId int64) error {
	now := time.Now()
	if err := DB.WithContext(ctx).Model(&model.DirectMessage{}).
		Where("message_id = ? AND sender_id = ?", messageId, userId).
		Update("deleted_at", now).Error; err != nil {
		return errors.Wrapf(err, "DeleteMessage failed, err: %v", err)
	}
	return nil
}

// ========================================
// Conversation Operations
// ========================================

// GetOrCreateConversation gets or creates a conversation between two users
func GetOrCreateConversation(ctx context.Context, userId1, userId2 int64) (*model.Conversation, error) {
	// Ensure userId1 < userId2 for consistency
	if userId1 > userId2 {
		userId1, userId2 = userId2, userId1
	}

	var conversation model.Conversation
	err := DB.WithContext(ctx).Where("user_id_1 = ? AND user_id_2 = ?", userId1, userId2).First(&conversation).Error

	if err != nil {
		// Create new conversation
		now := time.Now()
		conversation = model.Conversation{
			UserId1:   userId1,
			UserId2:   userId2,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := DB.WithContext(ctx).Create(&conversation).Error; err != nil {
			return nil, errors.Wrapf(err, "GetOrCreateConversation create failed, err: %v", err)
		}
	}

	return &conversation, nil
}

// GetUserConversations gets all conversations for a user
func GetUserConversations(ctx context.Context, userId int64, page, pageSize int64) ([]*model.Conversation, int64, error) {
	db := DB.WithContext(ctx).Model(&model.Conversation{}).Where("user_id_1 = ? OR user_id_2 = ?", userId, userId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUserConversations count failed, err: %v", err)
	}

	var conversations []*model.Conversation
	if err := db.Order("last_message_time DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&conversations).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUserConversations query failed, err: %v", err)
	}

	return conversations, total, nil
}

// UpdateConversationLastMessage updates the last message info
func UpdateConversationLastMessage(ctx context.Context, conversationId, messageId int64, content string, receiverId int64) error {
	now := time.Now()
	contentPreview := content
	if len(content) > 100 {
		contentPreview = content[:100] + "..."
	}

	// Get conversation to determine which unread count to increment
	var conversation model.Conversation
	if err := DB.WithContext(ctx).Where("conversation_id = ?", conversationId).First(&conversation).Error; err != nil {
		return errors.Wrapf(err, "UpdateConversationLastMessage get conversation failed, err: %v", err)
	}

	updates := map[string]interface{}{
		"last_message_id":      messageId,
		"last_message_content": contentPreview,
		"last_message_time":    now,
		"updated_at":           now,
	}

	// Increment unread count for receiver
	if receiverId == conversation.UserId1 {
		updates["user_1_unread_count"] = DB.Raw("user_1_unread_count + 1")
	} else {
		updates["user_2_unread_count"] = DB.Raw("user_2_unread_count + 1")
	}

	if err := DB.WithContext(ctx).Model(&model.Conversation{}).Where("conversation_id = ?", conversationId).Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "UpdateConversationLastMessage failed, err: %v", err)
	}

	return nil
}

// ClearConversationUnread clears unread count for a user
func ClearConversationUnread(ctx context.Context, conversationId, userId int64) error {
	var conversation model.Conversation
	if err := DB.WithContext(ctx).Where("conversation_id = ?", conversationId).First(&conversation).Error; err != nil {
		return errors.Wrapf(err, "ClearConversationUnread get conversation failed, err: %v", err)
	}

	var field string
	if userId == conversation.UserId1 {
		field = "user_1_unread_count"
	} else {
		field = "user_2_unread_count"
	}

	if err := DB.WithContext(ctx).Model(&model.Conversation{}).Where("conversation_id = ?", conversationId).Update(field, 0).Error; err != nil {
		return errors.Wrapf(err, "ClearConversationUnread failed, err: %v", err)
	}

	return nil
}

// GetUnreadMessageCount gets total unread message count for a user
func GetUnreadMessageCount(ctx context.Context, userId int64) (int64, error) {
	var count1, count2 int64

	if err := DB.WithContext(ctx).Model(&model.Conversation{}).
		Where("user_id_1 = ?", userId).
		Select("COALESCE(SUM(user_1_unread_count), 0)").Scan(&count1).Error; err != nil {
		return 0, errors.Wrapf(err, "GetUnreadMessageCount failed, err: %v", err)
	}

	if err := DB.WithContext(ctx).Model(&model.Conversation{}).
		Where("user_id_2 = ?", userId).
		Select("COALESCE(SUM(user_2_unread_count), 0)").Scan(&count2).Error; err != nil {
		return 0, errors.Wrapf(err, "GetUnreadMessageCount failed, err: %v", err)
	}

	return count1 + count2, nil
}

// ========================================
// Notification Operations
// ========================================

// CreateNotification creates a new notification
func CreateNotification(ctx context.Context, notification *model.Notification) error {
	notification.CreatedAt = time.Now()
	notification.IsRead = 0

	if err := DB.WithContext(ctx).Create(notification).Error; err != nil {
		return errors.Wrapf(err, "CreateNotification failed, err: %v", err)
	}
	return nil
}

// GetUserNotifications gets notifications for a user
func GetUserNotifications(ctx context.Context, userId int64, notificationType *int8, page, pageSize int64) ([]*model.Notification, int64, error) {
	db := DB.WithContext(ctx).Model(&model.Notification{}).Where("user_id = ?", userId)

	if notificationType != nil && *notificationType > 0 {
		db = db.Where("notification_type = ?", *notificationType)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUserNotifications count failed, err: %v", err)
	}

	var notifications []*model.Notification
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&notifications).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetUserNotifications query failed, err: %v", err)
	}

	return notifications, total, nil
}

// MarkNotificationsAsRead marks notifications as read
func MarkNotificationsAsRead(ctx context.Context, userId int64, notificationIds []int64) error {
	now := time.Now()
	query := DB.WithContext(ctx).Model(&model.Notification{}).Where("user_id = ?", userId)

	if len(notificationIds) > 0 {
		query = query.Where("notification_id IN ?", notificationIds)
	}

	if err := query.Updates(map[string]interface{}{
		"is_read": 1,
		"read_at": now,
	}).Error; err != nil {
		return errors.Wrapf(err, "MarkNotificationsAsRead failed, err: %v", err)
	}
	return nil
}

// GetUnreadNotificationCount gets unread notification count
func GetUnreadNotificationCount(ctx context.Context, userId int64) (int64, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&model.Notification{}).
		Where("user_id = ? AND is_read = 0", userId).
		Count(&count).Error; err != nil {
		return 0, errors.Wrapf(err, "GetUnreadNotificationCount failed, err: %v", err)
	}
	return count, nil
}

// ========================================
// Blacklist Operations
// ========================================

// BlockUser adds a user to blacklist
func BlockUser(ctx context.Context, userId, blockedUserId int64, reason *string) error {
	blacklist := &model.Blacklist{
		UserId:        userId,
		BlockedUserId: blockedUserId,
		Reason:        reason,
		CreatedAt:     time.Now(),
	}

	if err := DB.WithContext(ctx).Create(blacklist).Error; err != nil {
		return errors.Wrapf(err, "BlockUser failed, err: %v", err)
	}
	return nil
}

// UnblockUser removes a user from blacklist
func UnblockUser(ctx context.Context, userId, blockedUserId int64) error {
	if err := DB.WithContext(ctx).Where("user_id = ? AND blocked_user_id = ?", userId, blockedUserId).Delete(&model.Blacklist{}).Error; err != nil {
		return errors.Wrapf(err, "UnblockUser failed, err: %v", err)
	}
	return nil
}

// IsBlocked checks if a user is blocked
func IsBlocked(ctx context.Context, userId, blockedUserId int64) (bool, error) {
	var count int64
	if err := DB.WithContext(ctx).Model(&model.Blacklist{}).
		Where("user_id = ? AND blocked_user_id = ?", userId, blockedUserId).
		Count(&count).Error; err != nil {
		return false, errors.Wrapf(err, "IsBlocked failed, err: %v", err)
	}
	return count > 0, nil
}

// GetBlockedUsers gets all blocked users for a user
func GetBlockedUsers(ctx context.Context, userId int64, page, pageSize int64) ([]*model.Blacklist, int64, error) {
	db := DB.WithContext(ctx).Model(&model.Blacklist{}).Where("user_id = ?", userId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetBlockedUsers count failed, err: %v", err)
	}

	var blacklists []*model.Blacklist
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&blacklists).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetBlockedUsers query failed, err: %v", err)
	}

	return blacklists, total, nil
}

// ========================================
// Report Operations
// ========================================

// CreateReport creates a new report
func CreateReport(ctx context.Context, report *model.Report) error {
	report.CreatedAt = time.Now()
	report.Status = 0 // pending

	if err := DB.WithContext(ctx).Create(report).Error; err != nil {
		return errors.Wrapf(err, "CreateReport failed, err: %v", err)
	}
	return nil
}

// GetReportById gets report by id
func GetReportById(ctx context.Context, reportId int64) (*model.Report, error) {
	var report model.Report
	if err := DB.WithContext(ctx).Where("report_id = ?", reportId).First(&report).Error; err != nil {
		return nil, errors.Wrapf(err, "GetReportById failed, err: %v", err)
	}
	return &report, nil
}

// ListReports lists reports with filters (for admin)
func ListReports(ctx context.Context, status *int8, targetType *int8, page, pageSize int64) ([]*model.Report, int64, error) {
	db := DB.WithContext(ctx).Model(&model.Report{})

	if status != nil {
		db = db.Where("status = ?", *status)
	}
	if targetType != nil && *targetType > 0 {
		db = db.Where("target_type = ?", *targetType)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListReports count failed, err: %v", err)
	}

	var reports []*model.Report
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&reports).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListReports query failed, err: %v", err)
	}

	return reports, total, nil
}

// HandleReport handles a report (for admin)
func HandleReport(ctx context.Context, reportId, handlerId int64, status int8, handleResult *string, handleAction *int8) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":     status,
		"handler_id": handlerId,
		"handled_at": now,
	}

	if handleResult != nil {
		updates["handle_result"] = handleResult
	}
	if handleAction != nil {
		updates["handle_action"] = handleAction
	}

	if err := DB.WithContext(ctx).Model(&model.Report{}).Where("report_id = ?", reportId).Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "HandleReport failed, err: %v", err)
	}
	return nil
}

// ========================================
// Search History Operations
// ========================================

// SaveSearchHistory saves a search record
func SaveSearchHistory(ctx context.Context, userId int64, keyword string, searchType int8, resultCount uint) error {
	history := &model.SearchHistory{
		UserId:      userId,
		Keyword:     keyword,
		SearchType:  searchType,
		ResultCount: resultCount,
		CreatedAt:   time.Now(),
	}

	if err := DB.WithContext(ctx).Create(history).Error; err != nil {
		return errors.Wrapf(err, "SaveSearchHistory failed, err: %v", err)
	}

	// Update hot search
	UpdateHotSearch(ctx, keyword)

	return nil
}

// GetUserSearchHistory gets user's search history
func GetUserSearchHistory(ctx context.Context, userId int64, limit int64) ([]*model.SearchHistory, error) {
	var histories []*model.SearchHistory
	if err := DB.WithContext(ctx).
		Where("user_id = ?", userId).
		Order("created_at DESC").
		Limit(int(limit)).
		Find(&histories).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserSearchHistory failed, err: %v", err)
	}
	return histories, nil
}

// ClearUserSearchHistory clears user's search history
func ClearUserSearchHistory(ctx context.Context, userId int64) error {
	if err := DB.WithContext(ctx).Where("user_id = ?", userId).Delete(&model.SearchHistory{}).Error; err != nil {
		return errors.Wrapf(err, "ClearUserSearchHistory failed, err: %v", err)
	}
	return nil
}

// ========================================
// Hot Search Operations
// ========================================

// UpdateHotSearch updates or creates hot search entry
func UpdateHotSearch(ctx context.Context, keyword string) error {
	var hotSearch model.HotSearch
	err := DB.WithContext(ctx).Where("keyword = ?", keyword).First(&hotSearch).Error

	if err != nil {
		// Create new hot search
		hotSearch = model.HotSearch{
			Keyword:     keyword,
			SearchCount: 1,
			HeatScore:   1.0,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		return DB.WithContext(ctx).Create(&hotSearch).Error
	}

	// Update existing
	return DB.WithContext(ctx).Model(&model.HotSearch{}).
		Where("keyword = ?", keyword).
		Updates(map[string]interface{}{
			"search_count": DB.Raw("search_count + 1"),
			"heat_score":   DB.Raw("heat_score + 1"),
			"updated_at":   time.Now(),
		}).Error
}

// GetHotSearches gets hot search keywords
func GetHotSearches(ctx context.Context, limit int64) ([]*model.HotSearch, error) {
	var hotSearches []*model.HotSearch
	if err := DB.WithContext(ctx).
		Order("heat_score DESC, search_count DESC").
		Limit(int(limit)).
		Find(&hotSearches).Error; err != nil {
		return nil, errors.Wrapf(err, "GetHotSearches failed, err: %v", err)
	}
	return hotSearches, nil
}
