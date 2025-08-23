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

// CommentEventProcessor 评论事件处理器
type CommentEventProcessor struct {
	shardedDB    *db.ShardedCommentDB
	cacheManager *cache.CommentCacheManager
	mqManager    *mq.MQManager
}

// NewCommentEventProcessor 创建评论事件处理器
func NewCommentEventProcessor(shardedDB *db.ShardedCommentDB, cacheManager *cache.CommentCacheManager, mqManager *mq.MQManager) *CommentEventProcessor {
	return &CommentEventProcessor{
		shardedDB:    shardedDB,
		cacheManager: cacheManager,
		mqManager:    mqManager,
	}
}

// HandleCommentEvent 处理评论事件
func (cep *CommentEventProcessor) HandleCommentEvent(ctx context.Context, event *mq.CommentEvent) error {
	switch event.Type {
	case "create":
		return cep.handleCommentCreate(ctx, event)
	case "update":
		return cep.handleCommentUpdate(ctx, event)
	case "delete":
		return cep.handleCommentDelete(ctx, event)
	case "like":
		return cep.handleCommentLike(ctx, event)
	case "unlike":
		return cep.handleCommentUnlike(ctx, event)
	default:
		hlog.Warnf("Unknown comment event type: %s", event.Type)
		return nil
	}
}

// handleCommentCreate 处理评论创建事件
func (cep *CommentEventProcessor) handleCommentCreate(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in create event")
	}

	// 批量处理：将评论写入数据库
	if err := cep.shardedDB.CreateCommentWithTransaction(ctx, event.Comment); err != nil {
		hlog.Errorf("Failed to create comment %d: %v", event.Comment.CommentId, err)
		return err
	}

	// 异步处理缓存更新
	go func() {
		// 清除相关缓存
		if cep.cacheManager != nil {
			cep.cacheManager.InvalidateVideoCommentCache(context.Background(), event.VideoID)
			cep.cacheManager.IncrementVideoCommentCount(context.Background(), event.VideoID, 1)
		}

		// 发送通知事件（如果是回复）
		if event.Comment.ParentId != -1 {
			cep.sendReplyNotification(context.Background(), event)
		}
	}()

	hlog.Infof("Successfully processed comment create event: comment_id=%d, video_id=%d",
		event.Comment.CommentId, event.VideoID)
	return nil
}

// handleCommentUpdate 处理评论更新事件
func (cep *CommentEventProcessor) handleCommentUpdate(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in update event")
	}

	// 更新数据库中的评论
	// 这里需要实现更新逻辑

	// 清除缓存
	if cep.cacheManager != nil {
		go func() {
			cep.cacheManager.InvalidateCommentCache(context.Background(), event.Comment.CommentId)
			cep.cacheManager.InvalidateVideoCommentCache(context.Background(), event.VideoID)
		}()
	}

	return nil
}

// handleCommentDelete 处理评论删除事件
func (cep *CommentEventProcessor) handleCommentDelete(ctx context.Context, event *mq.CommentEvent) error {
	if event.Comment == nil {
		return fmt.Errorf("comment data is nil in delete event")
	}

	// 软删除评论
	if err := cep.shardedDB.DeleteCommentWithSharding(ctx, event.Comment.CommentId, event.VideoID); err != nil {
		hlog.Errorf("Failed to delete comment %d: %v", event.Comment.CommentId, err)
		return err
	}

	// 异步清除缓存
	go func() {
		if cep.cacheManager != nil {
			cep.cacheManager.InvalidateCommentCache(context.Background(), event.Comment.CommentId)
			cep.cacheManager.InvalidateVideoCommentCache(context.Background(), event.VideoID)
			cep.cacheManager.IncrementVideoCommentCount(context.Background(), event.VideoID, -1)
		}
	}()

	hlog.Infof("Successfully processed comment delete event: comment_id=%d", event.Comment.CommentId)
	return nil
}

// handleCommentLike 处理评论点赞事件
func (cep *CommentEventProcessor) handleCommentLike(ctx context.Context, event *mq.CommentEvent) error {
	commentID, ok := event.Extra["comment_id"].(int64)
	if !ok {
		return fmt.Errorf("comment_id not found in like event")
	}

	// 创建点赞记录
	if err := cep.shardedDB.CreateCommentLikeWithSharding(ctx, commentID, event.UserID); err != nil {
		hlog.Errorf("Failed to create comment like: comment_id=%d, user_id=%d, error=%v",
			commentID, event.UserID, err)
		return err
	}

	// 异步更新缓存
	go func() {
		if cep.cacheManager != nil {
			cep.cacheManager.IncrementCommentLikeCount(context.Background(), commentID, 1)
			cep.cacheManager.InvalidateCommentCache(context.Background(), commentID)
		}

		// 发送点赞通知
		cep.sendLikeNotification(context.Background(), commentID, event.UserID)
	}()

	return nil
}

// handleCommentUnlike 处理评论取消点赞事件
func (cep *CommentEventProcessor) handleCommentUnlike(ctx context.Context, event *mq.CommentEvent) error {
	commentID, ok := event.Extra["comment_id"].(int64)
	if !ok {
		return fmt.Errorf("comment_id not found in unlike event")
	}

	// 删除点赞记录
	if err := cep.shardedDB.DeleteCommentLikeWithSharding(ctx, commentID, event.UserID); err != nil {
		hlog.Errorf("Failed to delete comment like: comment_id=%d, user_id=%d, error=%v",
			commentID, event.UserID, err)
		return err
	}

	// 异步更新缓存
	go func() {
		if cep.cacheManager != nil {
			cep.cacheManager.IncrementCommentLikeCount(context.Background(), commentID, -1)
			cep.cacheManager.InvalidateCommentCache(context.Background(), commentID)
		}
	}()

	return nil
}

// sendReplyNotification 发送回复通知
func (cep *CommentEventProcessor) sendReplyNotification(ctx context.Context, event *mq.CommentEvent) {
	if cep.mqManager == nil {
		return
	}

	// 获取父评论信息以确定通知接收者
	parentComment, err := cep.shardedDB.GetCommentInfoWithSharding(ctx, event.Comment.ParentId)
	if err != nil {
		hlog.Warnf("Failed to get parent comment %d for notification: %v", event.Comment.ParentId, err)
		return
	}

	// 不给自己发通知
	if parentComment.UserId == event.UserID {
		return
	}

	notificationEvent := &mq.NotificationEvent{
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

	if err := cep.mqManager.PublishNotificationEvent(ctx, notificationEvent); err != nil {
		hlog.Warnf("Failed to send reply notification: %v", err)
	}
}

// sendLikeNotification 发送点赞通知
func (cep *CommentEventProcessor) sendLikeNotification(ctx context.Context, commentID, userID int64) {
	if cep.mqManager == nil {
		return
	}

	// 获取评论信息以确定通知接收者
	comment, err := cep.shardedDB.GetCommentInfoWithSharding(ctx, commentID)
	if err != nil {
		hlog.Warnf("Failed to get comment %d for like notification: %v", commentID, err)
		return
	}

	// 不给自己发通知
	if comment.UserId == userID {
		return
	}

	notificationEvent := &mq.NotificationEvent{
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

	if err := cep.mqManager.PublishNotificationEvent(ctx, notificationEvent); err != nil {
		hlog.Warnf("Failed to send like notification: %v", err)
	}
}

// BatchProcessCommentEvents 批量处理评论事件
func (cep *CommentEventProcessor) BatchProcessCommentEvents(ctx context.Context, events []*mq.CommentEvent) error {
	if len(events) == 0 {
		return nil
	}

	// 按事件类型分组
	createEvents := make([]*mq.CommentEvent, 0)
	otherEvents := make([]*mq.CommentEvent, 0)

	for _, event := range events {
		if event.Type == "create" {
			createEvents = append(createEvents, event)
		} else {
			otherEvents = append(otherEvents, event)
		}
	}

	// 批量处理创建事件
	if len(createEvents) > 0 {
		if err := cep.batchProcessCreateEvents(ctx, createEvents); err != nil {
			hlog.Errorf("Failed to batch process create events: %v", err)
			return err
		}
	}

	// 逐个处理其他事件
	for _, event := range otherEvents {
		if err := cep.HandleCommentEvent(ctx, event); err != nil {
			hlog.Errorf("Failed to process event %s: %v", event.Type, err)
			// 继续处理其他事件，不中断整个批次
		}
	}

	hlog.Infof("Successfully batch processed %d events (%d creates, %d others)",
		len(events), len(createEvents), len(otherEvents))
	return nil
}

// batchProcessCreateEvents 批量处理创建事件
func (cep *CommentEventProcessor) batchProcessCreateEvents(ctx context.Context, events []*mq.CommentEvent) error {
	// 按视频ID分组以优化数据库操作
	videoGroups := make(map[int64][]*mq.CommentEvent)
	for _, event := range events {
		videoGroups[event.VideoID] = append(videoGroups[event.VideoID], event)
	}

	// 按视频分组批量处理
	for videoID, videoEvents := range videoGroups {
		for _, event := range videoEvents {
			if err := cep.handleCommentCreate(ctx, event); err != nil {
				hlog.Errorf("Failed to process create event for video %d: %v", videoID, err)
				return err
			}
		}

		// 批量清除该视频的缓存
		if cep.cacheManager != nil {
			go func(vID int64, count int) {
				cep.cacheManager.InvalidateVideoCommentCache(context.Background(), vID)
				cep.cacheManager.IncrementVideoCommentCount(context.Background(), vID, int64(count))
			}(videoID, len(videoEvents))
		}
	}

	return nil
}

// GetProcessingStats 获取处理统计信息
func (cep *CommentEventProcessor) GetProcessingStats() map[string]interface{} {
	// 这里可以添加统计信息，如处理的事件数量、错误率等
	return map[string]interface{}{
		"processor_status": "running",
		"last_processed":   time.Now().Unix(),
	}
}
