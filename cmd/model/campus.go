package model

import "time"

// School represents the schools table for campus features
type School struct {
	SchoolId     int64     `json:"school_id" gorm:"column:school_id;primaryKey"`
	SchoolName   string    `json:"school_name" gorm:"column:school_name"`
	SchoolCode   string    `json:"school_code" gorm:"column:school_code;uniqueIndex"`
	Province     string    `json:"province" gorm:"column:province"`
	City         string    `json:"city" gorm:"column:city"`
	Address      *string   `json:"address" gorm:"column:address"`
	SchoolType   int8      `json:"school_type" gorm:"column:school_type;default:1"` // 1:university 2:college 3:high school 4:other
	LogoUrl      *string   `json:"logo_url" gorm:"column:logo_url"`
	CoverUrl     *string   `json:"cover_url" gorm:"column:cover_url"`
	StudentCount uint      `json:"student_count" gorm:"column:student_count;default:0"`
	VideoCount   uint      `json:"video_count" gorm:"column:video_count;default:0"`
	IsActive     int8      `json:"is_active" gorm:"column:is_active;default:1"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (School) TableName() string {
	return "schools"
}

// UserVerification represents campus verification for users
type UserVerification struct {
	Id                 int64      `json:"id" gorm:"column:id;primaryKey"`
	UserId             int64      `json:"user_id" gorm:"column:user_id;uniqueIndex"`
	SchoolId           int64      `json:"school_id" gorm:"column:school_id"`
	StudentId          string     `json:"student_id" gorm:"column:student_id"`
	RealName           string     `json:"real_name" gorm:"column:real_name"`
	IdCardHash         *string    `json:"id_card_hash" gorm:"column:id_card_hash"`
	Department         *string    `json:"department" gorm:"column:department"`
	Major              *string    `json:"major" gorm:"column:major"`
	EnrollmentYear     *int       `json:"enrollment_year" gorm:"column:enrollment_year"`
	GraduationYear     *int       `json:"graduation_year" gorm:"column:graduation_year"`
	VerificationStatus int8       `json:"verification_status" gorm:"column:verification_status;default:0"` // 0:unverified 1:pending 2:verified 3:failed 4:expired
	RejectionReason    *string    `json:"rejection_reason" gorm:"column:rejection_reason"`
	VerifiedAt         *time.Time `json:"verified_at" gorm:"column:verified_at"`
	ExpireAt           *time.Time `json:"expire_at" gorm:"column:expire_at"`
	CreatedAt          time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (UserVerification) TableName() string {
	return "user_verifications"
}

// Topic represents topics/challenges table
type Topic struct {
	TopicId          int64      `json:"topic_id" gorm:"column:topic_id;primaryKey"`
	Title            string     `json:"title" gorm:"column:title"`
	Description      *string    `json:"description" gorm:"column:description"`
	CoverUrl         *string    `json:"cover_url" gorm:"column:cover_url"`
	CreatorId        int64      `json:"creator_id" gorm:"column:creator_id"`
	TopicType        int8       `json:"topic_type" gorm:"column:topic_type;default:1"` // 1:normal 2:challenge 3:campus activity 4:official
	SchoolId         *int64     `json:"school_id" gorm:"column:school_id"`             // school exclusive topic (null for public)
	ParticipateCount uint64     `json:"participate_count" gorm:"column:participate_count;default:0"`
	ViewCount        uint64     `json:"view_count" gorm:"column:view_count;default:0"`
	Status           int8       `json:"status" gorm:"column:status;default:1"` // 1:normal 2:hot 3:banned 4:ended
	StartTime        *time.Time `json:"start_time" gorm:"column:start_time"`
	EndTime          *time.Time `json:"end_time" gorm:"column:end_time"`
	PrizeInfo        *string    `json:"prize_info" gorm:"column:prize_info"`
	Rules            *string    `json:"rules" gorm:"column:rules"`
	CreatedAt        time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt        time.Time  `json:"updated_at" gorm:"column:updated_at"`
}

func (Topic) TableName() string {
	return "topics"
}

// TopicCounter represents topic counters table for high concurrency
type TopicCounter struct {
	TopicId          int64     `json:"topic_id" gorm:"column:topic_id;primaryKey"`
	ParticipateCount uint64    `json:"participate_count" gorm:"column:participate_count;default:0"`
	ViewCount        uint64    `json:"view_count" gorm:"column:view_count;default:0"`
	VideoCount       uint64    `json:"video_count" gorm:"column:video_count;default:0"`
	UpdatedAt        time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (TopicCounter) TableName() string {
	return "topic_counters"
}

// VideoTopic represents video-topic association
type VideoTopic struct {
	Id        int64     `json:"id" gorm:"column:id;primaryKey"`
	VideoId   int64     `json:"video_id" gorm:"column:video_id"`
	TopicId   int64     `json:"topic_id" gorm:"column:topic_id"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
}

func (VideoTopic) TableName() string {
	return "video_topics"
}
