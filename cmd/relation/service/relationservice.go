package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/internal/model"
	"HuaTug.com/cmd/relation/dal/db"
	"HuaTug.com/kitex_gen/relations"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

type RelationService struct {
	ctx       context.Context
	shardeDB  *db.ShardedFollowDB
	mqManager *mq.MQManager
}

// MQ Manager singleton for notification
var mqManagerInstance *mq.MQManager

// InitMQManager initializes the MQ manager for relation service
func InitMQManager(rabbitmqURL string) error {
	var err error
	mqManagerInstance, err = mq.NewMQManager(rabbitmqURL)
	if err != nil {
		hlog.Errorf("Failed to initialize MQ manager for relation service: %v", err)
		return err
	}
	hlog.Info("MQ manager for relation service initialized successfully")
	return nil
}

// GetMQManager returns the MQ manager instance
func GetMQManager() *mq.MQManager {
	return mqManagerInstance
}

func NewRelationService(ctx context.Context, shardeDB *db.ShardedFollowDB) *RelationService {
	return &RelationService{
		ctx:       ctx,
		shardeDB:  shardeDB,
		mqManager: mqManagerInstance,
	}
}

func (s *RelationService) RelationService(ctx context.Context, req *relations.RelationServiceRequest) error {
	if req.FromUserId == req.ToUserId {
		return errno.RequestErr
	}

	switch req.ActionType {
	case 1: // 关注
		return s.CreateFollow(ctx, req)
	case 2: // 取消关注
		return s.CancelFollow(ctx, req)
	default:
		return errno.ParamErr
	}
}

func (s *RelationService) CreateFollow(ctx context.Context, req *relations.RelationServiceRequest) error {
	// 检查是否已关注
	exists, err := s.shardeDB.IsFollowing(ctx, req.FromUserId, req.ToUserId)
	if err != nil {
		return fmt.Errorf("failed to check following status: %w", errno.ServiceErr)
	}
	if exists {
		return errno.RequestErr // 已关注
	}

	if err := s.shardeDB.InsertFollowWithTransaction(ctx, &model.FollowRelation{
		UserID:     req.ToUserId,
		FollowerID: req.FromUserId,
	}); err != nil {
		return fmt.Errorf("failed to create follow: %w", errno.ServiceErr)
	}

	// Send follow notification asynchronously
	go s.sendFollowNotification(ctx, req.FromUserId, req.ToUserId)

	return nil
}

func (s *RelationService) CancelFollow(ctx context.Context, req *relations.RelationServiceRequest) error {
	// 检查是否已关注
	exists, err := s.shardeDB.IsFollowing(ctx, req.FromUserId, req.ToUserId)
	if err != nil {
		return fmt.Errorf("failed to check following status: %w", errno.ServiceErr)
	}
	if !exists {
		return errno.RequestErr // 未关注
	}

	if err := s.shardeDB.DeleteFollow(ctx, req.FromUserId, req.ToUserId); err != nil {
		return fmt.Errorf("failed to delete follow: %w", errno.ServiceErr)
	}
	return nil
}

// sendFollowNotification sends a notification to the user being followed
func (s *RelationService) sendFollowNotification(ctx context.Context, followerID, followedID int64) {
	if s.mqManager == nil {
		hlog.Warnf("MQ manager not initialized, skipping follow notification")
		return
	}

	// Create notification event
	notificationEvent := &mq.NotificationEvent{
		Type:       "follow",
		ReceiverID: followedID, // The user being followed receives the notification
		SenderID:   followerID, // The user who followed
		Content:    "关注了你",
		Extra: map[string]interface{}{
			"follower_id": followerID,
		},
		Timestamp: time.Now().Unix(),
		EventID:   uuid.New().String(),

		// Compatibility fields
		UserID:           followedID,
		FromUserID:       followerID,
		NotificationType: "follow",
		TargetID:         followerID, // The target is the follower (for profile link)
	}

	// Publish notification event to MQ
	if err := s.mqManager.PublishNotificationEvent(context.Background(), notificationEvent); err != nil {
		hlog.Errorf("Failed to send follow notification: follower_id=%d, followed_id=%d, error=%v",
			followerID, followedID, err)
		return
	}

	hlog.Infof("Sent follow notification: user %d followed user %d", followerID, followedID)
}
