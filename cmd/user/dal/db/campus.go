package db

import (
	"context"
	"time"

	"HuaTug.com/cmd/model"
	"github.com/pkg/errors"
)

// ========================================
// School Operations
// ========================================

// CreateSchool creates a new school record
func CreateSchool(ctx context.Context, school *model.School) error {
	now := time.Now()
	school.CreatedAt = now
	school.UpdatedAt = now
	if err := DB.WithContext(ctx).Create(school).Error; err != nil {
		return errors.Wrapf(err, "CreateSchool failed, err: %v", err)
	}
	return nil
}

// GetSchoolById gets school by id
func GetSchoolById(ctx context.Context, schoolId int64) (*model.School, error) {
	var school model.School
	if err := DB.WithContext(ctx).Where("school_id = ? AND is_active = 1", schoolId).First(&school).Error; err != nil {
		return nil, errors.Wrapf(err, "GetSchoolById failed, err: %v", err)
	}
	return &school, nil
}

// GetSchoolByCode gets school by school code
func GetSchoolByCode(ctx context.Context, schoolCode string) (*model.School, error) {
	var school model.School
	if err := DB.WithContext(ctx).Where("school_code = ? AND is_active = 1", schoolCode).First(&school).Error; err != nil {
		return nil, errors.Wrapf(err, "GetSchoolByCode failed, err: %v", err)
	}
	return &school, nil
}

// ListSchools lists schools with pagination
func ListSchools(ctx context.Context, province, city *string, schoolType *int8, page, pageSize int64) ([]*model.School, int64, error) {
	db := DB.WithContext(ctx).Model(&model.School{}).Where("is_active = 1")

	if province != nil && *province != "" {
		db = db.Where("province = ?", *province)
	}
	if city != nil && *city != "" {
		db = db.Where("city = ?", *city)
	}
	if schoolType != nil && *schoolType > 0 {
		db = db.Where("school_type = ?", *schoolType)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListSchools count failed, err: %v", err)
	}

	var schools []*model.School
	if err := db.Order("student_count DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&schools).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListSchools query failed, err: %v", err)
	}

	return schools, total, nil
}

// UpdateSchoolCounts updates school student/video counts
func UpdateSchoolCounts(ctx context.Context, schoolId int64, field string, delta int) error {
	var updateExpr string
	switch field {
	case "student_count":
		updateExpr = "student_count + ?"
	case "video_count":
		updateExpr = "video_count + ?"
	default:
		return errors.Errorf("invalid field: %s", field)
	}

	if err := DB.WithContext(ctx).Model(&model.School{}).Where("school_id = ?", schoolId).
		UpdateColumn(field, DB.Raw(updateExpr, delta)).Error; err != nil {
		return errors.Wrapf(err, "UpdateSchoolCounts failed, err: %v", err)
	}
	return nil
}

// ========================================
// User Verification Operations
// ========================================

// CreateUserVerification creates a verification request
func CreateUserVerification(ctx context.Context, verification *model.UserVerification) error {
	now := time.Now()
	verification.CreatedAt = now
	verification.UpdatedAt = now
	verification.VerificationStatus = 1 // pending

	if err := DB.WithContext(ctx).Create(verification).Error; err != nil {
		return errors.Wrapf(err, "CreateUserVerification failed, err: %v", err)
	}
	return nil
}

// GetUserVerification gets user verification by user_id
func GetUserVerification(ctx context.Context, userId int64) (*model.UserVerification, error) {
	var verification model.UserVerification
	if err := DB.WithContext(ctx).Where("user_id = ?", userId).First(&verification).Error; err != nil {
		return nil, errors.Wrapf(err, "GetUserVerification failed, err: %v", err)
	}
	return &verification, nil
}

// UpdateVerificationStatus updates verification status
func UpdateVerificationStatus(ctx context.Context, userId int64, status int8, rejectionReason *string) error {
	updates := map[string]interface{}{
		"verification_status": status,
		"updated_at":          time.Now(),
	}

	if status == 2 { // verified
		now := time.Now()
		updates["verified_at"] = &now
		// Set expiry date to graduation year + 1 year
		var verification model.UserVerification
		DB.WithContext(ctx).Where("user_id = ?", userId).First(&verification)
		if verification.GraduationYear != nil {
			expireAt := time.Date(*verification.GraduationYear+1, 7, 1, 0, 0, 0, 0, time.Local)
			updates["expire_at"] = &expireAt
		}
	} else if status == 3 { // failed
		updates["rejection_reason"] = rejectionReason
	}

	if err := DB.WithContext(ctx).Model(&model.UserVerification{}).Where("user_id = ?", userId).Updates(updates).Error; err != nil {
		return errors.Wrapf(err, "UpdateVerificationStatus failed, err: %v", err)
	}
	return nil
}

// CheckVerificationExpired checks if verification is expired
func CheckVerificationExpired(ctx context.Context, userId int64) (bool, error) {
	var verification model.UserVerification
	if err := DB.WithContext(ctx).Where("user_id = ?", userId).First(&verification).Error; err != nil {
		return false, errors.Wrapf(err, "CheckVerificationExpired failed, err: %v", err)
	}

	if verification.VerificationStatus != 2 {
		return true, nil // not verified
	}

	if verification.ExpireAt != nil && verification.ExpireAt.Before(time.Now()) {
		// Update status to expired
		DB.WithContext(ctx).Model(&model.UserVerification{}).Where("user_id = ?", userId).Update("verification_status", 4)
		return true, nil
	}

	return false, nil
}

// ========================================
// Topic Operations
// ========================================

// CreateTopic creates a new topic/challenge
func CreateTopic(ctx context.Context, topic *model.Topic) error {
	now := time.Now()
	topic.CreatedAt = now
	topic.UpdatedAt = now

	if err := DB.WithContext(ctx).Create(topic).Error; err != nil {
		return errors.Wrapf(err, "CreateTopic failed, err: %v", err)
	}
	return nil
}

// GetTopicById gets topic by id
func GetTopicById(ctx context.Context, topicId int64) (*model.Topic, error) {
	var topic model.Topic
	if err := DB.WithContext(ctx).Where("topic_id = ?", topicId).First(&topic).Error; err != nil {
		return nil, errors.Wrapf(err, "GetTopicById failed, err: %v", err)
	}
	return &topic, nil
}

// ListTopics lists topics with filters
func ListTopics(ctx context.Context, topicType *int8, schoolId *int64, status *int8, page, pageSize int64) ([]*model.Topic, int64, error) {
	db := DB.WithContext(ctx).Model(&model.Topic{})

	if topicType != nil && *topicType > 0 {
		db = db.Where("topic_type = ?", *topicType)
	}
	if schoolId != nil {
		db = db.Where("school_id = ? OR school_id IS NULL", *schoolId)
	} else {
		db = db.Where("school_id IS NULL") // only public topics
	}
	if status != nil && *status > 0 {
		db = db.Where("status = ?", *status)
	} else {
		db = db.Where("status IN (1, 2)") // normal or hot
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListTopics count failed, err: %v", err)
	}

	var topics []*model.Topic
	if err := db.Order("participate_count DESC, created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&topics).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "ListTopics query failed, err: %v", err)
	}

	return topics, total, nil
}

// GetHotTopics gets hot topics
func GetHotTopics(ctx context.Context, schoolId *int64, limit int64) ([]*model.Topic, error) {
	db := DB.WithContext(ctx).Model(&model.Topic{}).Where("status IN (1, 2)")

	if schoolId != nil {
		db = db.Where("school_id = ? OR school_id IS NULL", *schoolId)
	} else {
		db = db.Where("school_id IS NULL")
	}

	var topics []*model.Topic
	if err := db.Order("participate_count DESC").Limit(int(limit)).Find(&topics).Error; err != nil {
		return nil, errors.Wrapf(err, "GetHotTopics failed, err: %v", err)
	}

	return topics, nil
}

// UpdateTopicStatus updates topic status
func UpdateTopicStatus(ctx context.Context, topicId int64, status int8) error {
	if err := DB.WithContext(ctx).Model(&model.Topic{}).Where("topic_id = ?", topicId).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return errors.Wrapf(err, "UpdateTopicStatus failed, err: %v", err)
	}
	return nil
}

// IncrementTopicCount increments topic participate/view count
func IncrementTopicCount(ctx context.Context, topicId int64, field string, delta int64) error {
	var updateExpr string
	switch field {
	case "participate_count":
		updateExpr = "participate_count + ?"
	case "view_count":
		updateExpr = "view_count + ?"
	default:
		return errors.Errorf("invalid field: %s", field)
	}

	if err := DB.WithContext(ctx).Model(&model.Topic{}).Where("topic_id = ?", topicId).
		UpdateColumn(field, DB.Raw(updateExpr, delta)).Error; err != nil {
		return errors.Wrapf(err, "IncrementTopicCount failed, err: %v", err)
	}
	return nil
}

// ========================================
// Video Topic Association Operations
// ========================================

// AddVideoToTopic associates a video with a topic
func AddVideoToTopic(ctx context.Context, videoId, topicId int64) error {
	videoTopic := &model.VideoTopic{
		VideoId:   videoId,
		TopicId:   topicId,
		CreatedAt: time.Now(),
	}

	if err := DB.WithContext(ctx).Create(videoTopic).Error; err != nil {
		return errors.Wrapf(err, "AddVideoToTopic failed, err: %v", err)
	}

	// Increment topic participate count
	IncrementTopicCount(ctx, topicId, "participate_count", 1)

	return nil
}

// GetVideosByTopic gets videos by topic
func GetVideosByTopic(ctx context.Context, topicId int64, page, pageSize int64) ([]int64, int64, error) {
	db := DB.WithContext(ctx).Model(&model.VideoTopic{}).Where("topic_id = ?", topicId)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetVideosByTopic count failed, err: %v", err)
	}

	var videoTopics []*model.VideoTopic
	if err := db.Order("created_at DESC").Limit(int(pageSize)).Offset(int(pageSize * (page - 1))).Find(&videoTopics).Error; err != nil {
		return nil, 0, errors.Wrapf(err, "GetVideosByTopic query failed, err: %v", err)
	}

	var videoIds []int64
	for _, vt := range videoTopics {
		videoIds = append(videoIds, vt.VideoId)
	}

	return videoIds, total, nil
}

// GetTopicsByVideo gets topics associated with a video
func GetTopicsByVideo(ctx context.Context, videoId int64) ([]*model.Topic, error) {
	var videoTopics []*model.VideoTopic
	if err := DB.WithContext(ctx).Where("video_id = ?", videoId).Find(&videoTopics).Error; err != nil {
		return nil, errors.Wrapf(err, "GetTopicsByVideo failed, err: %v", err)
	}

	var topicIds []int64
	for _, vt := range videoTopics {
		topicIds = append(topicIds, vt.TopicId)
	}

	if len(topicIds) == 0 {
		return []*model.Topic{}, nil
	}

	var topics []*model.Topic
	if err := DB.WithContext(ctx).Where("topic_id IN ?", topicIds).Find(&topics).Error; err != nil {
		return nil, errors.Wrapf(err, "GetTopicsByVideo topics query failed, err: %v", err)
	}

	return topics, nil
}
