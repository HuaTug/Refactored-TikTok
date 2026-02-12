package service

import (
	"context"
	"regexp"
	"time"

	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/config"
	"HuaTug.com/kitex_gen/users"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/google/uuid"
)

// MentionNotificationService handles @mention notifications in comments
type MentionNotificationService struct {
	mqManager *mq.MQManager
}

// MQ Manager singleton for notification in interaction service
var interactionMQManager *mq.MQManager

// InitInteractionMQManager initializes the MQ manager for interaction service
func InitInteractionMQManager(rabbitmqURL string) error {
	var err error
	interactionMQManager, err = mq.NewMQManager(rabbitmqURL)
	if err != nil {
		hlog.Errorf("Failed to initialize MQ manager for interaction service: %v", err)
		return err
	}
	hlog.Info("MQ manager for interaction service initialized successfully")
	return nil
}

// GetInteractionMQManager returns the MQ manager instance for interaction service
func GetInteractionMQManager() *mq.MQManager {
	return interactionMQManager
}

// NewMentionNotificationService creates a new mention notification service
func NewMentionNotificationService() *MentionNotificationService {
	return &MentionNotificationService{
		mqManager: interactionMQManager,
	}
}

// mentionPattern matches @username patterns in comments
// Supports alphanumeric usernames with underscores and Chinese characters, 2-20 characters
// Note: Go regexp uses \x{XXXX} syntax for Unicode, not \uXXXX
var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_\x{4e00}-\x{9fa5}]{2,20})`)

// ParseMentions extracts all @usernames from a comment content
func (s *MentionNotificationService) ParseMentions(content string) []string {
	matches := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	// Use a map to deduplicate usernames
	usernameMap := make(map[string]bool)
	for _, match := range matches {
		if len(match) >= 2 {
			usernameMap[match[1]] = true
		}
	}

	// Convert map to slice
	usernames := make([]string, 0, len(usernameMap))
	for username := range usernameMap {
		usernames = append(usernames, username)
	}

	return usernames
}

// GetUserIDsByUsernames looks up user IDs by their usernames
func (s *MentionNotificationService) GetUserIDsByUsernames(ctx context.Context, usernames []string) map[string]int64 {
	result := make(map[string]int64)

	for _, username := range usernames {
		// Use QueryUser to find user by exact username match
		resp, err := client.QueryUser(ctx, &users.QueryUserRequest{
			Keyword:  &username,
			Page:     1,
			PageSize: 10, // Search for potential matches
		})

		if err != nil {
			hlog.Warnf("Failed to query user by username %s: %v", username, err)
			continue
		}

		// Find exact match in results
		if resp != nil && resp.Users != nil {
			for _, user := range resp.Users {
				if user.UserName == username {
					result[username] = user.UserId
					break
				}
			}
		}
	}

	return result
}

// SendMentionNotifications sends notifications to all mentioned users
func (s *MentionNotificationService) SendMentionNotifications(ctx context.Context, senderID int64, videoID int64, commentID int64, content string, mentionedUserIDs map[string]int64) {
	if s.mqManager == nil {
		hlog.Warn("MQ manager not initialized, skipping mention notifications")
		return
	}

	for username, userID := range mentionedUserIDs {
		// Don't notify yourself
		if userID == senderID {
			continue
		}

		// Truncate content for notification display
		displayContent := truncateContentForNotification(content, 50)

		notificationEvent := &mq.NotificationEvent{
			Type:       "mention",
			ReceiverID: userID,
			SenderID:   senderID,
			Content:    "在评论中提到了你: " + displayContent,
			Extra: map[string]interface{}{
				"video_id":   videoID,
				"comment_id": commentID,
				"username":   username,
			},
			Timestamp: time.Now().Unix(),
			EventID:   uuid.New().String(),

			// Compatibility fields
			UserID:           userID,
			FromUserID:       senderID,
			NotificationType: "mention",
			TargetID:         commentID,
			VideoID:          videoID,
			CommentID:        commentID,
		}

		if err := s.mqManager.PublishNotificationEvent(context.Background(), notificationEvent); err != nil {
			hlog.Errorf("Failed to send mention notification to user %d: %v", userID, err)
			continue
		}

		hlog.Infof("Sent mention notification: user %d mentioned @%s (user_id=%d) in comment %d",
			senderID, username, userID, commentID)
	}
}

// ProcessMentions is a convenience method that parses mentions and sends notifications
func (s *MentionNotificationService) ProcessMentions(ctx context.Context, senderID int64, videoID int64, commentID int64, content string) {
	// Parse @usernames from content
	usernames := s.ParseMentions(content)
	if len(usernames) == 0 {
		return
	}

	hlog.Infof("Found %d mentions in comment %d: %v", len(usernames), commentID, usernames)

	// Look up user IDs
	mentionedUserIDs := s.GetUserIDsByUsernames(ctx, usernames)
	if len(mentionedUserIDs) == 0 {
		hlog.Infof("No valid users found for mentions in comment %d", commentID)
		return
	}

	// Send notifications
	s.SendMentionNotifications(ctx, senderID, videoID, commentID, content, mentionedUserIDs)
}

// truncateContentForNotification truncates content for notification display
func truncateContentForNotification(content string, maxLen int) string {
	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}

// InitMentionNotificationFromConfig initializes MQ manager from config
func InitMentionNotificationFromConfig() {
	rabbitmqURL := config.GetRabbitMQURL()
	if rabbitmqURL != "" {
		if err := InitInteractionMQManager(rabbitmqURL); err != nil {
			hlog.Warnf("Failed to initialize MQ Manager for interaction: %v (mention notifications will be disabled)", err)
		}
	} else {
		hlog.Warn("RabbitMQ URL not configured, mention notifications will be disabled")
	}
}
