package service

import (
	"context"
	"sync"
	"time"

	"HuaTug.com/config"
	"HuaTug.com/pkg/kafka"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// KafkaTracker 用于追踪用户行为和视频统计的 Kafka 服务
type KafkaTracker struct {
	manager *kafka.Manager
	mu      sync.RWMutex
	enabled bool
}

var (
	kafkaTracker *KafkaTracker
	once         sync.Once
)

// GetKafkaTracker 获取全局 KafkaTracker 实例
func GetKafkaTracker() *KafkaTracker {
	once.Do(func() {
		kafkaTracker = &KafkaTracker{
			enabled: false,
		}
	})
	return kafkaTracker
}

// Init 初始化 Kafka 连接
func (t *KafkaTracker) Init() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.enabled {
		return nil
	}

	// 检查配置
	if len(config.ConfigInfo.Kafka.Brokers) == 0 {
		hlog.Warn("[KafkaTracker] Kafka brokers not configured, tracking disabled")
		return nil
	}

	kafkaConfig := &kafka.KafkaConfig{
		Brokers:         config.ConfigInfo.Kafka.Brokers,
		Version:         config.ConfigInfo.Kafka.Version,
		ProducerRetries: config.ConfigInfo.Kafka.ProducerRetries,
	}

	manager, err := kafka.NewManager(kafkaConfig)
	if err != nil {
		hlog.Errorf("[KafkaTracker] Failed to create Kafka manager: %v", err)
		return err
	}

	// 初始化 Topics
	if err := manager.InitTopics(); err != nil {
		hlog.Warnf("[KafkaTracker] Failed to init topics: %v", err)
		// 不返回错误，Topics 可能已存在
	}

	t.manager = manager
	t.enabled = true
	hlog.Info("[KafkaTracker] Initialized successfully")
	return nil
}

// Close 关闭 Kafka 连接
func (t *KafkaTracker) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled || t.manager == nil {
		return nil
	}

	err := t.manager.Close()
	t.enabled = false
	t.manager = nil
	return err
}

// IsEnabled 检查是否启用
func (t *KafkaTracker) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled
}

// ============ 用户行为追踪 ============

// TrackPlay 记录播放行为
func (t *KafkaTracker) TrackPlay(ctx context.Context, userID, videoID int64, deviceType, platform string) {
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorPlay, 0, 0, deviceType, platform, nil)
}

// TrackComplete 记录完播行为
func (t *KafkaTracker) TrackComplete(ctx context.Context, userID, videoID int64, watchTime int64, deviceType, platform string) {
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorComplete, watchTime, 0, deviceType, platform, nil)
}

// TrackLike 记录点赞行为
func (t *KafkaTracker) TrackLike(ctx context.Context, userID, videoID int64) {
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorLike, 0, 0, "", "", nil)
}

// TrackUnlike 记录取消点赞行为
func (t *KafkaTracker) TrackUnlike(ctx context.Context, userID, videoID int64) {
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorUnlike, 0, 0, "", "", nil)
}

// TrackComment 记录评论行为
func (t *KafkaTracker) TrackComment(ctx context.Context, userID, videoID int64) {
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorComment, 0, 0, "", "", nil)
}

// TrackShare 记录分享行为
func (t *KafkaTracker) TrackShare(ctx context.Context, userID, videoID int64, sharePlatform string) {
	extra := map[string]string{"share_platform": sharePlatform}
	t.trackBehavior(ctx, userID, videoID, kafka.BehaviorShare, 0, 0, "", "", extra)
}

// TrackFollow 记录关注行为
func (t *KafkaTracker) TrackFollow(ctx context.Context, userID, targetUserID int64) {
	extra := map[string]string{"target_user_id": string(rune(targetUserID))}
	t.trackBehavior(ctx, userID, 0, kafka.BehaviorFollow, 0, 0, "", "", extra)
}

// TrackSearch 记录搜索行为
func (t *KafkaTracker) TrackSearch(ctx context.Context, userID int64, query string, resultCount int, clickedIDs []int64) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.SearchLogEvent{
		UserID:      userID,
		Query:       query,
		Timestamp:   time.Now(),
		ResultCount: resultCount,
		ClickedIDs:  clickedIDs,
	}

	go func() {
		if err := t.manager.PublishSearchLog(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish search log: %v", err)
		}
	}()
}

// trackBehavior 通用行为追踪
func (t *KafkaTracker) trackBehavior(ctx context.Context, userID, videoID int64, behavior kafka.BehaviorType, duration, position int64, deviceType, platform string, extra map[string]string) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.UserBehaviorEvent{
		UserID:     userID,
		VideoID:    videoID,
		Behavior:   behavior,
		Timestamp:  time.Now(),
		Duration:   duration,
		Position:   position,
		DeviceType: deviceType,
		Platform:   platform,
		Extra:      extra,
	}

	// 异步发送，不阻塞主流程
	go func() {
		if err := t.manager.PublishUserBehavior(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish user behavior: %v", err)
		}
	}()
}

// ============ 视频播放统计 ============

// TrackVideoView 记录视频播放详情
func (t *KafkaTracker) TrackVideoView(ctx context.Context, videoID, userID, authorID int64, watchTime, videoDuration int64, source string) {
	if !t.IsEnabled() {
		return
	}

	watchPercent := float64(0)
	if videoDuration > 0 {
		watchPercent = float64(watchTime) / float64(videoDuration) * 100
	}

	event := &kafka.VideoViewEvent{
		VideoID:       videoID,
		UserID:        userID,
		AuthorID:      authorID,
		Timestamp:     time.Now(),
		WatchTime:     watchTime,
		VideoDuration: videoDuration,
		WatchPercent:  watchPercent,
		IsComplete:    watchPercent >= 90, // 90%以上算完播
		Source:        source,
	}

	go func() {
		if err := t.manager.PublishVideoView(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish video view: %v", err)
		}
	}()
}

// TrackVideoExposure 记录视频曝光
func (t *KafkaTracker) TrackVideoExposure(ctx context.Context, userID int64, videoIDs []int64, source, recallType string) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.VideoExposureEvent{
		UserID:     userID,
		VideoIDs:   videoIDs,
		Timestamp:  time.Now(),
		Source:     source,
		RecallType: recallType,
	}

	go func() {
		if err := t.manager.PublishVideoExposure(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish video exposure: %v", err)
		}
	}()
}

// IncrementVideoStats 增加视频统计计数
func (t *KafkaTracker) IncrementVideoStats(ctx context.Context, videoID int64, statsType string, delta int64) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.VideoStatsEvent{
		VideoID:   videoID,
		StatsType: statsType,
		Delta:     delta,
		Timestamp: time.Now(),
	}

	go func() {
		if err := t.manager.PublishVideoStats(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish video stats: %v", err)
		}
	}()
}

// ============ 推荐系统特征更新 ============

// UpdateUserProfile 更新用户画像
func (t *KafkaTracker) UpdateUserProfile(ctx context.Context, userID int64, tags []string, categories []string, scores map[string]float64) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.UserProfileUpdateEvent{
		UserID:     userID,
		Timestamp:  time.Now(),
		UpdateType: "behavior_update",
		Tags:       tags,
		Categories: categories,
		Scores:     scores,
	}

	go func() {
		if err := t.manager.PublishUserProfileUpdate(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish user profile update: %v", err)
		}
	}()
}

// UpdateVideoFeature 更新视频特征
func (t *KafkaTracker) UpdateVideoFeature(ctx context.Context, videoID int64, playCount, likeCount, commentCount, shareCount int64, hotScore float64) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.VideoFeatureUpdateEvent{
		VideoID:      videoID,
		Timestamp:    time.Now(),
		UpdateType:   "stats_update",
		PlayCount:    playCount,
		LikeCount:    likeCount,
		CommentCount: commentCount,
		ShareCount:   shareCount,
		HotScore:     hotScore,
	}

	go func() {
		if err := t.manager.PublishVideoFeatureUpdate(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish video feature update: %v", err)
		}
	}()
}

// LogRecommendation 记录推荐结果
func (t *KafkaTracker) LogRecommendation(ctx context.Context, userID int64, videoIDs []int64, scores []float64, recallType, requestID, abTestGroup, modelVersion string) {
	if !t.IsEnabled() {
		return
	}

	event := &kafka.RecommendationEvent{
		UserID:       userID,
		Timestamp:    time.Now(),
		RecallType:   recallType,
		VideoIDs:     videoIDs,
		Scores:       scores,
		RequestID:    requestID,
		ABTestGroup:  abTestGroup,
		ModelVersion: modelVersion,
	}

	go func() {
		if err := t.manager.PublishRecommendation(ctx, event); err != nil {
			hlog.Errorf("[KafkaTracker] Failed to publish recommendation: %v", err)
		}
	}()
}
