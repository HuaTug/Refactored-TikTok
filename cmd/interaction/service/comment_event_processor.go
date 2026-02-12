package service

import (
	"context"
	"fmt"
	"time"

	"HuaTug.com/cmd/interaction/dal/db"
	"HuaTug.com/pkg/cache"
	"HuaTug.com/pkg/mq"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// maxNotificationContentLen is the max display length for notification content.
const maxNotificationContentLen = 50

// CommentEventProcessor handles comment events from the message queue.
type CommentEventProcessor struct {
	shardedDB    *db.ShardedCommentDB
	cacheManager *cache.CommentCacheManager
	mqManager    *mq.MQManager
}

// NewCommentEventProcessor creates a comment event processor.
func NewCommentEventProcessor(shardedDB *db.ShardedCommentDB, cacheManager *cache.CommentCacheManager, mqManager *mq.MQManager) *CommentEventProcessor {
	return &CommentEventProcessor{
		shardedDB:    shardedDB,
		cacheManager: cacheManager,
		mqManager:    mqManager,
	}
}

// HandleCommentEvent dispatches a comment event by type.
func (p *CommentEventProcessor) HandleCommentEvent(ctx context.Context, event *mq.CommentEvent) error {
	switch event.Type {
	case "create":
		return p.handleCreate(ctx, event)
	case "update":
		return p.handleUpdate(ctx, event)
	case "delete":
		return p.handleDelete(ctx, event)
	case "like":
		return p.handleLike(ctx, event)
	case "unlike":
		return p.handleUnlike(ctx, event)
	default:
		hlog.Warnf("Unknown comment event type: %s", event.Type)
		return nil
	}
}

// handleCreate processes a comment creation event.
func (p *CommentEventProcessor) handleCreate(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in create event")
	}

	if err := p.shardedDB.CreateCommentWithTransaction(ctx, event.Comment); err != nil {
		hlog.Errorf("Failed to create comment %d: %v", event.Comment.CommentId, err)
		return err
	}

	go p.onCommentCreated(event)

	hlog.Infof("Processed comment create: comment_id=%d, video_id=%d", event.Comment.CommentId, event.VideoID)
	return nil
}

// onCommentCreated performs async cache invalidation and notification after creation.
func (p *CommentEventProcessor) onCommentCreated(event *mq.CommentEvent) {
	ctx := context.Background()

	if p.cacheManager != nil {
		p.cacheManager.InvalidateVideoCommentCache(ctx, event.VideoID)
		p.cacheManager.IncrementVideoCommentCount(ctx, event.VideoID, 1)
	}

	if event.Comment.ParentId == -1 {
		p.sendVideoCommentNotification(ctx, event)
	} else {
		p.sendReplyNotification(ctx, event)
	}
}

// handleUpdate processes a comment update event.
func (p *CommentEventProcessor) handleUpdate(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in update event")
	}

	// TODO: implement DB update logic here.

	if p.cacheManager != nil {
		go func() {
			bgCtx := context.Background()
			p.cacheManager.InvalidateCommentCache(bgCtx, event.Comment.CommentId)
			p.cacheManager.InvalidateVideoCommentCache(bgCtx, event.VideoID)
		}()
	}

	return nil
}

// handleDelete processes a comment deletion event.
func (p *CommentEventProcessor) handleDelete(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in delete event")
	}

	if err := p.shardedDB.DeleteCommentWithSharding(ctx, event.Comment.CommentId, event.VideoID); err != nil {
		hlog.Errorf("Failed to delete comment %d: %v", event.Comment.CommentId, err)
		return err
	}

	if p.cacheManager != nil {
		go func() {
			bgCtx := context.Background()
			p.cacheManager.InvalidateCommentCache(bgCtx, event.Comment.CommentId)
			p.cacheManager.InvalidateVideoCommentCache(bgCtx, event.VideoID)
			p.cacheManager.IncrementVideoCommentCount(bgCtx, event.VideoID, -1)
		}()
	}

	hlog.Infof("Processed comment delete: comment_id=%d", event.Comment.CommentId)
	return nil
}

// handleLike processes a comment like event.
func (p *CommentEventProcessor) handleLike(ctx context.Context, event *mq.CommentEvent) error {
	commentID, ok := event.Extra["comment_id"].(int64)
	if !ok {
		return fmt.Errorf("comment_id not found in like event")
	}

	if err := p.shardedDB.CreateCommentLikeWithSharding(ctx, commentID, event.UserID); err != nil {
		hlog.Errorf("Failed to create comment like: comment_id=%d, user_id=%d, err=%v", commentID, event.UserID, err)
		return err
	}

	go func() {
		bgCtx := context.Background()
		if p.cacheManager != nil {
			p.cacheManager.IncrementCommentLikeCount(bgCtx, commentID, 1)
			p.cacheManager.InvalidateCommentCache(bgCtx, commentID)
		}
		p.sendLikeNotification(bgCtx, commentID, event.UserID)
	}()

	return nil
}

// handleUnlike processes a comment unlike event.
func (p *CommentEventProcessor) handleUnlike(ctx context.Context, event *mq.CommentEvent) error {
	commentID, ok := event.Extra["comment_id"].(int64)
	if !ok {
		return fmt.Errorf("comment_id not found in unlike event")
	}

	if err := p.shardedDB.DeleteCommentLikeWithSharding(ctx, commentID, event.UserID); err != nil {
		hlog.Errorf("Failed to delete comment like: comment_id=%d, user_id=%d, err=%v", commentID, event.UserID, err)
		return err
	}

	if p.cacheManager != nil {
		go func() {
			bgCtx := context.Background()
			p.cacheManager.IncrementCommentLikeCount(bgCtx, commentID, -1)
			p.cacheManager.InvalidateCommentCache(bgCtx, commentID)
		}()
	}

	return nil
}

// --- Notifications ---

// sendVideoCommentNotification notifies the video author when someone comments on their video.
func (p *CommentEventProcessor) sendVideoCommentNotification(ctx context.Context, event *mq.CommentEvent) {
	if p.mqManager == nil || event.Comment.ParentId != -1 {
		return
	}

	authorID, ok := p.resolveVideoAuthorID(event)
	if !ok || authorID == event.UserID {
		return
	}

	notification := &mq.NotificationEvent{
		Type:       "comment",
		ReceiverID: authorID,
		SenderID:   event.UserID,
		Content:    fmt.Sprintf("评论了你的视频: %s", truncateString(event.Comment.Content, maxNotificationContentLen)),
		Extra: map[string]interface{}{
			"comment_id": event.Comment.CommentId,
			"video_id":   event.VideoID,
		},
		Timestamp:        time.Now().Unix(),
		UserID:           authorID,
		FromUserID:       event.UserID,
		NotificationType: "comment",
		TargetID:         event.VideoID,
		VideoID:          event.VideoID,
		CommentID:        event.Comment.CommentId,
	}

	if err := p.mqManager.PublishNotificationEvent(ctx, notification); err != nil {
		hlog.Warnf("Failed to send video comment notification: %v", err)
	} else {
		hlog.Infof("Sent video comment notification: user %d commented on video %d by user %d",
			event.UserID, event.VideoID, authorID)
	}
}

// resolveVideoAuthorID extracts the video author ID from event extra data.
func (p *CommentEventProcessor) resolveVideoAuthorID(event *mq.CommentEvent) (int64, bool) {
	raw := event.Extra["video_author_id"]
	if raw == nil {
		hlog.Warnf("Video author ID not found in comment event extra data")
		return 0, false
	}

	switch v := raw.(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		hlog.Warnf("Invalid video author ID type in comment event")
		return 0, false
	}
}

// sendReplyNotification notifies the parent comment author when someone replies.
func (p *CommentEventProcessor) sendReplyNotification(ctx context.Context, event *mq.CommentEvent) {
	if p.mqManager == nil {
		return
	}

	parentComment, err := p.shardedDB.GetCommentInfoWithSharding(ctx, event.Comment.ParentId)
	if err != nil {
		hlog.Warnf("Failed to get parent comment %d for notification: %v", event.Comment.ParentId, err)
		return
	}

	if parentComment.UserId == event.UserID {
		return // don't notify self
	}

	notification := &mq.NotificationEvent{
		Type:       "comment_reply",
		ReceiverID: parentComment.UserId,
		SenderID:   event.UserID,
		Content:    fmt.Sprintf("回复了你的评论: %s", event.Comment.Content),
		Extra: map[string]interface{}{
			"comment_id":        event.Comment.CommentId,
			"parent_comment_id": event.Comment.ParentId,
			"video_id":          event.VideoID,
		},
		Timestamp: time.Now().Unix(),
	}

	if err := p.mqManager.PublishNotificationEvent(ctx, notification); err != nil {
		hlog.Warnf("Failed to send reply notification: %v", err)
	}
}

// sendLikeNotification notifies the comment author when someone likes their comment.
func (p *CommentEventProcessor) sendLikeNotification(ctx context.Context, commentID, userID int64) {
	if p.mqManager == nil {
		return
	}

	comment, err := p.shardedDB.GetCommentInfoWithSharding(ctx, commentID)
	if err != nil {
		hlog.Warnf("Failed to get comment %d for like notification: %v", commentID, err)
		return
	}

	if comment.UserId == userID {
		return
	}

	notification := &mq.NotificationEvent{
		Type:       "comment_like",
		ReceiverID: comment.UserId,
		SenderID:   userID,
		Content:    "点赞了你的评论",
		Extra: map[string]interface{}{
			"comment_id": commentID,
			"video_id":   comment.VideoId,
		},
		Timestamp: time.Now().Unix(),
	}

	if err := p.mqManager.PublishNotificationEvent(ctx, notification); err != nil {
		hlog.Warnf("Failed to send like notification: %v", err)
	}
}

// --- Batch Processing ---

// BatchProcessCommentEvents processes a batch of comment events.
func (p *CommentEventProcessor) BatchProcessCommentEvents(ctx context.Context, events []*mq.CommentEvent) error {
	if len(events) == 0 {
		return nil
	}

	createEvents := make([]*mq.CommentEvent, 0, len(events))
	otherEvents := make([]*mq.CommentEvent, 0)

	for _, event := range events {
		if event.Type == "create" {
			createEvents = append(createEvents, event)
		} else {
			otherEvents = append(otherEvents, event)
		}
	}

	if len(createEvents) > 0 {
		if err := p.batchProcessCreates(ctx, createEvents); err != nil {
			hlog.Errorf("Failed to batch process create events: %v", err)
			return err
		}
	}

	for _, event := range otherEvents {
		if err := p.HandleCommentEvent(ctx, event); err != nil {
			hlog.Errorf("Failed to process event %s: %v", event.Type, err)
		}
	}

	hlog.Infof("Batch processed %d events (%d creates, %d others)",
		len(events), len(createEvents), len(otherEvents))
	return nil
}

// batchProcessCreates processes creation events grouped by video ID.
func (p *CommentEventProcessor) batchProcessCreates(ctx context.Context, events []*mq.CommentEvent) error {
	videoGroups := make(map[int64][]*mq.CommentEvent, len(events))
	for _, event := range events {
		videoGroups[event.VideoID] = append(videoGroups[event.VideoID], event)
	}

	for videoID, videoEvents := range videoGroups {
		for _, event := range videoEvents {
			if err := p.handleCreate(ctx, event); err != nil {
				hlog.Errorf("Failed to process create event for video %d: %v", videoID, err)
				return err
			}
		}

		// Batch invalidate cache per video.
		if p.cacheManager != nil {
			go func(vID int64, count int) {
				bgCtx := context.Background()
				p.cacheManager.InvalidateVideoCommentCache(bgCtx, vID)
				p.cacheManager.IncrementVideoCommentCount(bgCtx, vID, int64(count))
			}(videoID, len(videoEvents))
		}
	}

	return nil
}

// GetProcessingStats returns basic processor stats.
func (p *CommentEventProcessor) GetProcessingStats() map[string]interface{} {
	return map[string]interface{}{
		"processor_status": "running",
		"last_processed":   time.Now().Unix(),
	}
}

// truncateString truncates a string to maxLen runes, appending "..." if needed.
func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
