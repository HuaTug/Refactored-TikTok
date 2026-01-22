package model

import (
	"encoding/json"
	"time"
)

// DirectMessage represents private messages between users
type DirectMessage struct {
	MessageId      int64      `json:"message_id" gorm:"column:message_id;primaryKey"`
	ConversationId int64      `json:"conversation_id" gorm:"column:conversation_id"`
	SenderId       int64      `json:"sender_id" gorm:"column:sender_id"`
	ReceiverId     int64      `json:"receiver_id" gorm:"column:receiver_id"`
	Content        string     `json:"content" gorm:"column:content"`
	MessageType    int8       `json:"message_type" gorm:"column:message_type;default:1"` // 1:text 2:image 3:video 4:share 5:emoji
	RelatedVideoId *int64     `json:"related_video_id" gorm:"column:related_video_id"`
	IsRead         int8       `json:"is_read" gorm:"column:is_read;default:0"`
	ReadAt         *time.Time `json:"read_at" gorm:"column:read_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
	DeletedAt      *time.Time `json:"deleted_at" gorm:"column:deleted_at"`
}

func (DirectMessage) TableName() string {
	return "direct_messages"
}

// Conversation represents a chat conversation between two users
type Conversation struct {
	ConversationId     int64      `json:"conversation_id" gorm:"column:conversation_id;primaryKey"`
	UserId1            int64      `json:"user_id_1" gorm:"column:user_id_1"`
	UserId2            int64      `json:"user_id_2" gorm:"column:user_id_2"`
	LastMessageId      *int64     `json:"last_message_id" gorm:"column:last_message_id"`
	LastMessageContent *string    `json:"last_message_content" gorm:"column:last_message_content"`
	LastMessageTime    *time.Time `json:"last_message_time" gorm:"column:last_message_time"`
	User1UnreadCount   uint       `json:"user_1_unread_count" gorm:"column:user_1_unread_count;default:0"`
	User2UnreadCount   uint       `json:"user_2_unread_count" gorm:"column:user_2_unread_count;default:0"`
	CreatedAt          time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (Conversation) TableName() string {
	return "conversations"
}

// Notification represents user notifications
type Notification struct {
	NotificationId   int64            `json:"notification_id" gorm:"column:notification_id;primaryKey"`
	UserId           int64            `json:"user_id" gorm:"column:user_id"`
	SenderId         *int64           `json:"sender_id" gorm:"column:sender_id"`
	NotificationType int8             `json:"notification_type" gorm:"column:notification_type"` // 1:like 2:comment 3:follow 4:mention 5:system 6:activity
	TargetType       *int8            `json:"target_type" gorm:"column:target_type"`             // 1:video 2:comment 3:user
	TargetId         *int64           `json:"target_id" gorm:"column:target_id"`
	Title            *string          `json:"title" gorm:"column:title"`
	Content          *string          `json:"content" gorm:"column:content"`
	ExtraData        *json.RawMessage `json:"extra_data" gorm:"column:extra_data;type:json"`
	IsRead           int8             `json:"is_read" gorm:"column:is_read;default:0"`
	ReadAt           *time.Time       `json:"read_at" gorm:"column:read_at"`
	CreatedAt        time.Time        `json:"created_at" gorm:"column:created_at"`
}

func (Notification) TableName() string {
	return "notifications"
}

// Notification type constants
const (
	NotificationTypeLike     = 1
	NotificationTypeComment  = 2
	NotificationTypeFollow   = 3
	NotificationTypeMention  = 4
	NotificationTypeSystem   = 5
	NotificationTypeActivity = 6
)

// Notification target type constants
const (
	NotificationTargetVideo   = 1
	NotificationTargetComment = 2
	NotificationTargetUser    = 3
)

// Blacklist represents user blacklist
type Blacklist struct {
	Id            int64     `json:"id" gorm:"column:id;primaryKey"`
	UserId        int64     `json:"user_id" gorm:"column:user_id"`
	BlockedUserId int64     `json:"blocked_user_id" gorm:"column:blocked_user_id"`
	Reason        *string   `json:"reason" gorm:"column:reason"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
}

func (Blacklist) TableName() string {
	return "blacklists"
}

// Report represents user reports
type Report struct {
	ReportId     int64            `json:"report_id" gorm:"column:report_id;primaryKey"`
	ReporterId   int64            `json:"reporter_id" gorm:"column:reporter_id"`
	TargetType   int8             `json:"target_type" gorm:"column:target_type"` // 1:video 2:comment 3:user 4:message
	TargetId     int64            `json:"target_id" gorm:"column:target_id"`
	ReasonType   int8             `json:"reason_type" gorm:"column:reason_type"` // 1:porn 2:violence 3:illegal 4:spam 5:fraud 6:other
	ReasonDetail *string          `json:"reason_detail" gorm:"column:reason_detail"`
	EvidenceUrls *json.RawMessage `json:"evidence_urls" gorm:"column:evidence_urls;type:json"`
	Status       int8             `json:"status" gorm:"column:status;default:0"` // 0:pending 1:processing 2:resolved 3:rejected
	HandlerId    *int64           `json:"handler_id" gorm:"column:handler_id"`
	HandleResult *string          `json:"handle_result" gorm:"column:handle_result"`
	HandleAction *int8            `json:"handle_action" gorm:"column:handle_action"` // 1:warning 2:delete 3:ban user 4:no action
	CreatedAt    time.Time        `json:"created_at" gorm:"column:created_at"`
	HandledAt    *time.Time       `json:"handled_at" gorm:"column:handled_at"`
}

func (Report) TableName() string {
	return "reports"
}

// Report reason type constants
const (
	ReportReasonPorn     = 1
	ReportReasonViolence = 2
	ReportReasonIllegal  = 3
	ReportReasonSpam     = 4
	ReportReasonFraud    = 5
	ReportReasonOther    = 6
)

// Report target type constants
const (
	ReportTargetVideo   = 1
	ReportTargetComment = 2
	ReportTargetUser    = 3
	ReportTargetMessage = 4
)

// Report status constants
const (
	ReportStatusPending    = 0
	ReportStatusProcessing = 1
	ReportStatusResolved   = 2
	ReportStatusRejected   = 3
)

// Report handle action constants
const (
	ReportActionWarning  = 1
	ReportActionDelete   = 2
	ReportActionBanUser  = 3
	ReportActionNoAction = 4
)

// SearchHistory represents user search history
type SearchHistory struct {
	Id          int64     `json:"id" gorm:"column:id;primaryKey"`
	UserId      int64     `json:"user_id" gorm:"column:user_id"`
	Keyword     string    `json:"keyword" gorm:"column:keyword"`
	SearchType  int8      `json:"search_type" gorm:"column:search_type;default:1"` // 1:all 2:user 3:video 4:topic 5:school
	ResultCount uint      `json:"result_count" gorm:"column:result_count;default:0"`
	CreatedAt   time.Time `json:"created_at" gorm:"column:created_at"`
}

func (SearchHistory) TableName() string {
	return "search_histories"
}

// Search type constants
const (
	SearchTypeAll    = 1
	SearchTypeUser   = 2
	SearchTypeVideo  = 3
	SearchTypeTopic  = 4
	SearchTypeSchool = 5
)

// HotSearch represents hot search keywords
type HotSearch struct {
	Id           int64     `json:"id" gorm:"column:id;primaryKey"`
	Keyword      string    `json:"keyword" gorm:"column:keyword;uniqueIndex"`
	SearchCount  uint64    `json:"search_count" gorm:"column:search_count;default:0"`
	HeatScore    float64   `json:"heat_score" gorm:"column:heat_score;default:0"`
	Category     *string   `json:"category" gorm:"column:category"`
	IsPromoted   int8      `json:"is_promoted" gorm:"column:is_promoted;default:0"`
	RankPosition *int      `json:"rank_position" gorm:"column:rank_position"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (HotSearch) TableName() string {
	return "hot_searches"
}
