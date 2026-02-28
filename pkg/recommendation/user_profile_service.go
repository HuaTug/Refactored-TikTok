package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ========================================
// 用户画像更新服务
// ========================================

// UserProfileService 用户画像服务
type UserProfileService struct {
	redis            *redis.Client
	db               *gorm.DB
	config           *ProfileServiceConfig
	updateQueue      chan *ProfileUpdateEvent
	wg               sync.WaitGroup
	stopCh           chan struct{}
	realtimeStateSvc *RealtimeStateService // Optional: feeds the Agent's realtime perception layer
}

// ProfileServiceConfig 画像服务配置
type ProfileServiceConfig struct {
	// 实时更新配置
	QueueSize     int           `json:"queue_size"`
	BatchSize     int           `json:"batch_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	WorkerCount   int           `json:"worker_count"`

	// 衰减配置
	InterestDecayDays int     `json:"interest_decay_days"` // 兴趣衰减天数
	DecayFactor       float64 `json:"decay_factor"`        // 衰减因子

	// 画像配置
	MaxInterestTags   int `json:"max_interest_tags"`   // 最大兴趣标签数
	MaxCategoryPrefer int `json:"max_category_prefer"` // 最大分类偏好数
	MaxAuthorPrefer   int `json:"max_author_prefer"`   // 最大作者偏好数

	// Redis 过期配置
	RedisExpireDays int `json:"redis_expire_days"`
}

// DefaultProfileServiceConfig 默认配置
func DefaultProfileServiceConfig() *ProfileServiceConfig {
	return &ProfileServiceConfig{
		QueueSize:         10000,
		BatchSize:         100,
		FlushInterval:     5 * time.Second,
		WorkerCount:       4,
		InterestDecayDays: 30,
		DecayFactor:       0.95,
		MaxInterestTags:   50,
		MaxCategoryPrefer: 20,
		MaxAuthorPrefer:   100,
		RedisExpireDays:   30,
	}
}

// ProfileUpdateEvent 画像更新事件
type ProfileUpdateEvent struct {
	UserID     int64             `json:"user_id"`
	VideoID    int64             `json:"video_id"`
	ActionType string            `json:"action_type"` // view/like/comment/share/finish/favorite/dislike
	ActionTime time.Time         `json:"action_time"`
	Duration   int               `json:"duration"` // 观看时长（秒）
	Progress   float64           `json:"progress"` // 观看进度 0-1
	ExtraData  map[string]string `json:"extra_data"`
}

// NewUserProfileService 创建用户画像服务
func NewUserProfileService(redisClient *redis.Client, db *gorm.DB, config *ProfileServiceConfig) *UserProfileService {
	if config == nil {
		config = DefaultProfileServiceConfig()
	}

	service := &UserProfileService{
		redis:       redisClient,
		db:          db,
		config:      config,
		updateQueue: make(chan *ProfileUpdateEvent, config.QueueSize),
		stopCh:      make(chan struct{}),
	}

	// 启动后台 worker
	service.startWorkers()

	return service
}

// startWorkers 启动后台 worker
func (ups *UserProfileService) startWorkers() {
	for i := 0; i < ups.config.WorkerCount; i++ {
		ups.wg.Add(1)
		go ups.worker(i)
	}
}

// worker 后台处理 worker
func (ups *UserProfileService) worker(id int) {
	defer ups.wg.Done()

	batch := make([]*ProfileUpdateEvent, 0, ups.config.BatchSize)
	ticker := time.NewTicker(ups.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ups.stopCh:
			// 处理剩余事件
			if len(batch) > 0 {
				ups.processBatch(batch)
			}
			return

		case event := <-ups.updateQueue:
			batch = append(batch, event)
			if len(batch) >= ups.config.BatchSize {
				ups.processBatch(batch)
				batch = make([]*ProfileUpdateEvent, 0, ups.config.BatchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				ups.processBatch(batch)
				batch = make([]*ProfileUpdateEvent, 0, ups.config.BatchSize)
			}
		}
	}
}

// Stop 停止服务
func (ups *UserProfileService) Stop() {
	close(ups.stopCh)
	ups.wg.Wait()
}

// RecordAction 记录用户行为（异步）
func (ups *UserProfileService) RecordAction(event *ProfileUpdateEvent) {
	select {
	case ups.updateQueue <- event:
		// 成功入队
	default:
		// 队列已满，丢弃或降级处理
		hlog.Warnf("Profile update queue is full, dropping event for user %d", event.UserID)
	}

	// Non-blocking: also record to the realtime action stream for Agent perception.
	// This never blocks the main RecordAction flow.
	if ups.realtimeStateSvc != nil {
		go ups.realtimeStateSvc.RecordRealtimeAction(context.Background(), event.UserID, UserAction{
			VideoID:    event.VideoID,
			ActionType: event.ActionType,
			Timestamp:  event.ActionTime.UnixMilli(),
			Duration:   event.Duration,
			Progress:   event.Progress,
		})
	}
}

// SetRealtimeStateService attaches the realtime state service for Agent integration.
func (ups *UserProfileService) SetRealtimeStateService(svc *RealtimeStateService) {
	ups.realtimeStateSvc = svc
}

// RecordActionSync 记录用户行为（同步）
func (ups *UserProfileService) RecordActionSync(ctx context.Context, event *ProfileUpdateEvent) error {
	return ups.processEvent(ctx, event)
}

// processBatch 批量处理事件
func (ups *UserProfileService) processBatch(events []*ProfileUpdateEvent) {
	ctx := context.Background()

	// 按用户分组
	userEvents := make(map[int64][]*ProfileUpdateEvent)
	for _, event := range events {
		userEvents[event.UserID] = append(userEvents[event.UserID], event)
	}

	// 并行处理每个用户
	var wg sync.WaitGroup
	for userID, userEventList := range userEvents {
		wg.Add(1)
		go func(uid int64, eventList []*ProfileUpdateEvent) {
			defer wg.Done()
			for _, event := range eventList {
				if err := ups.processEvent(ctx, event); err != nil {
					hlog.Warnf("Failed to process event for user %d: %v", uid, err)
				}
			}
		}(userID, userEventList)
	}
	wg.Wait()
}

// processEvent 处理单个事件
func (ups *UserProfileService) processEvent(ctx context.Context, event *ProfileUpdateEvent) error {
	// 1. 获取视频信息
	videoInfo := ups.getVideoInfo(ctx, event.VideoID)
	if videoInfo == nil {
		return nil
	}

	// 2. 计算行为权重
	weight := ups.calculateActionWeight(event)

	// 3. 更新兴趣标签
	if err := ups.updateInterestTags(ctx, event.UserID, videoInfo.Tags, weight); err != nil {
		hlog.Warnf("Failed to update interest tags: %v", err)
	}

	// 4. 更新分类偏好
	if err := ups.updateCategoryPreference(ctx, event.UserID, videoInfo.Category, weight); err != nil {
		hlog.Warnf("Failed to update category preference: %v", err)
	}

	// 5. 更新作者偏好
	if err := ups.updateAuthorPreference(ctx, event.UserID, videoInfo.AuthorID, weight); err != nil {
		hlog.Warnf("Failed to update author preference: %v", err)
	}

	// 6. 更新行为统计
	if err := ups.updateBehaviorStats(ctx, event); err != nil {
		hlog.Warnf("Failed to update behavior stats: %v", err)
	}

	// 7. 更新活跃时段
	if err := ups.updateActiveTimeSlots(ctx, event.UserID, event.ActionTime); err != nil {
		hlog.Warnf("Failed to update active time slots: %v", err)
	}

	return nil
}

// VideoInfo 视频信息
type VideoInfo struct {
	VideoID  int64
	AuthorID int64
	Category string
	Tags     []string
	Duration int
}

// getVideoInfo 获取视频信息
func (ups *UserProfileService) getVideoInfo(ctx context.Context, videoID int64) *VideoInfo {
	// 先从 Redis 缓存获取
	key := fmt.Sprintf("video:info:%d", videoID)
	data, err := ups.redis.Get(ctx, key).Result()
	if err == nil && data != "" {
		var info VideoInfo
		if json.Unmarshal([]byte(data), &info) == nil {
			return &info
		}
	}

	// 缓存未命中，从数据库获取
	var video struct {
		VideoID    int64  `gorm:"column:video_id"`
		UserID     int64  `gorm:"column:user_id"`
		Category   string `gorm:"column:category"`
		LabelNames string `gorm:"column:label_names"`
		Duration   int    `gorm:"column:duration"`
	}

	if err := ups.db.WithContext(ctx).Table("videos").
		Select("video_id, user_id, category, label_names, duration").
		Where("video_id = ?", videoID).
		First(&video).Error; err != nil {
		return nil
	}

	info := &VideoInfo{
		VideoID:  video.VideoID,
		AuthorID: video.UserID,
		Category: video.Category,
		Duration: video.Duration,
	}

	// 解析标签
	if video.LabelNames != "" {
		json.Unmarshal([]byte(video.LabelNames), &info.Tags)
		if info.Tags == nil {
			// 尝试逗号分隔
			info.Tags = splitTags(video.LabelNames)
		}
	}

	// 缓存到 Redis
	if infoData, err := json.Marshal(info); err == nil {
		ups.redis.Set(ctx, key, infoData, 24*time.Hour)
	}

	return info
}

// splitTags 分割标签字符串
func splitTags(tagStr string) []string {
	if tagStr == "" {
		return nil
	}
	// 简单按逗号分割
	tags := make([]string, 0)
	start := 0
	for i := 0; i < len(tagStr); i++ {
		if tagStr[i] == ',' {
			if i > start {
				tags = append(tags, tagStr[start:i])
			}
			start = i + 1
		}
	}
	if start < len(tagStr) {
		tags = append(tags, tagStr[start:])
	}
	return tags
}

// calculateActionWeight 计算行为权重
func (ups *UserProfileService) calculateActionWeight(event *ProfileUpdateEvent) float64 {
	baseWeights := map[string]float64{
		"view":     0.1,
		"finish":   0.4,
		"like":     0.6,
		"comment":  0.7,
		"share":    0.9,
		"favorite": 0.8,
		"dislike":  -0.5,
	}

	weight := baseWeights[event.ActionType]
	if weight == 0 {
		weight = 0.1
	}

	// 根据观看进度调整权重
	if event.ActionType == "view" && event.Progress > 0 {
		weight *= event.Progress
		// 完播给额外加成
		if event.Progress >= 0.9 {
			weight *= 1.5
		}
	}

	// 时间衰减（越近的行为权重越高）
	hoursSince := time.Since(event.ActionTime).Hours()
	if hoursSince > 0 {
		decayFactor := math.Pow(ups.config.DecayFactor, hoursSince/24)
		weight *= decayFactor
	}

	return weight
}

// updateInterestTags 更新兴趣标签
func (ups *UserProfileService) updateInterestTags(ctx context.Context, userID int64, tags []string, weight float64) error {
	if len(tags) == 0 {
		return nil
	}

	key := fmt.Sprintf("user:interests:%d", userID)
	pipe := ups.redis.Pipeline()

	for _, tag := range tags {
		if tag != "" {
			pipe.ZIncrBy(ctx, key, weight, tag)
		}
	}

	// 设置过期时间
	pipe.Expire(ctx, key, time.Duration(ups.config.RedisExpireDays)*24*time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// 修剪，只保留 top N
	return ups.redis.ZRemRangeByRank(ctx, key, 0, int64(-ups.config.MaxInterestTags-1)).Err()
}

// updateCategoryPreference 更新分类偏好
func (ups *UserProfileService) updateCategoryPreference(ctx context.Context, userID int64, category string, weight float64) error {
	if category == "" {
		return nil
	}

	key := fmt.Sprintf("user:category_prefer:%d", userID)
	pipe := ups.redis.Pipeline()

	pipe.ZIncrBy(ctx, key, weight, category)
	pipe.Expire(ctx, key, time.Duration(ups.config.RedisExpireDays)*24*time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// 修剪
	return ups.redis.ZRemRangeByRank(ctx, key, 0, int64(-ups.config.MaxCategoryPrefer-1)).Err()
}

// updateAuthorPreference 更新作者偏好
func (ups *UserProfileService) updateAuthorPreference(ctx context.Context, userID, authorID int64, weight float64) error {
	if authorID == 0 {
		return nil
	}

	key := fmt.Sprintf("user:author_prefer:%d", userID)
	pipe := ups.redis.Pipeline()

	pipe.ZIncrBy(ctx, key, weight, fmt.Sprintf("%d", authorID))
	pipe.Expire(ctx, key, time.Duration(ups.config.RedisExpireDays)*24*time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	// 修剪
	return ups.redis.ZRemRangeByRank(ctx, key, 0, int64(-ups.config.MaxAuthorPrefer-1)).Err()
}

// updateBehaviorStats 更新行为统计
func (ups *UserProfileService) updateBehaviorStats(ctx context.Context, event *ProfileUpdateEvent) error {
	key := fmt.Sprintf("user:stats:%d", event.UserID)

	pipe := ups.redis.Pipeline()

	// 更新总计数
	pipe.HIncrBy(ctx, key, "total_actions", 1)
	pipe.HIncrBy(ctx, key, fmt.Sprintf("%s_count", event.ActionType), 1)

	// 更新观看统计
	if event.ActionType == "view" || event.ActionType == "finish" {
		pipe.HIncrBy(ctx, key, "total_watch_time", int64(event.Duration))
		pipe.HIncrBy(ctx, key, "total_views", 1)
	}

	// 更新互动统计
	if event.ActionType == "like" || event.ActionType == "comment" || event.ActionType == "share" {
		pipe.HIncrBy(ctx, key, "total_interactions", 1)
	}

	pipe.Expire(ctx, key, time.Duration(ups.config.RedisExpireDays)*24*time.Hour)

	_, err := pipe.Exec(ctx)
	return err
}

// updateActiveTimeSlots 更新活跃时段
func (ups *UserProfileService) updateActiveTimeSlots(ctx context.Context, userID int64, actionTime time.Time) error {
	hour := actionTime.Hour()
	key := fmt.Sprintf("user:active_hours:%d", userID)

	pipe := ups.redis.Pipeline()
	pipe.ZIncrBy(ctx, key, 1, fmt.Sprintf("%d", hour))
	pipe.Expire(ctx, key, time.Duration(ups.config.RedisExpireDays)*24*time.Hour)

	_, err := pipe.Exec(ctx)
	return err
}

// ========================================
// 批量画像更新（定时任务用）
// ========================================

// BatchUpdateUserProfiles 批量更新用户画像（从数据库同步到缓存）
func (ups *UserProfileService) BatchUpdateUserProfiles(ctx context.Context, userIDs []int64) error {
	for _, userID := range userIDs {
		if err := ups.syncUserProfileToCache(ctx, userID); err != nil {
			hlog.Warnf("Failed to sync profile for user %d: %v", userID, err)
		}
	}
	return nil
}

// syncUserProfileToCache 同步单个用户画像到缓存
func (ups *UserProfileService) syncUserProfileToCache(ctx context.Context, userID int64) error {
	// 从 user_profiles 表获取持久化画像
	var profile struct {
		InterestTags       string  `gorm:"column:interest_tags"`
		CategoryPreference string  `gorm:"column:category_preference"`
		AuthorPreference   string  `gorm:"column:author_preference"`
		AvgWatchDuration   float64 `gorm:"column:avg_watch_duration"`
		LikeRate           float64 `gorm:"column:like_rate"`
		CommentRate        float64 `gorm:"column:comment_rate"`
		ShareRate          float64 `gorm:"column:share_rate"`
		UserLevel          int     `gorm:"column:user_level"`
	}

	if err := ups.db.WithContext(ctx).Table("user_profiles").
		Where("user_id = ?", userID).
		First(&profile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil // 用户画像不存在
		}
		return err
	}

	pipe := ups.redis.Pipeline()
	expiration := time.Duration(ups.config.RedisExpireDays) * 24 * time.Hour

	// 同步兴趣标签
	if profile.InterestTags != "" {
		var tags map[string]float64
		if json.Unmarshal([]byte(profile.InterestTags), &tags) == nil {
			key := fmt.Sprintf("user:interests:%d", userID)
			pipe.Del(ctx, key)
			for tag, weight := range tags {
				pipe.ZAdd(ctx, key, &redis.Z{Score: weight, Member: tag})
			}
			pipe.Expire(ctx, key, expiration)
		}
	}

	// 同步分类偏好
	if profile.CategoryPreference != "" {
		var categories map[string]float64
		if json.Unmarshal([]byte(profile.CategoryPreference), &categories) == nil {
			key := fmt.Sprintf("user:category_prefer:%d", userID)
			pipe.Del(ctx, key)
			for cat, weight := range categories {
				pipe.ZAdd(ctx, key, &redis.Z{Score: weight, Member: cat})
			}
			pipe.Expire(ctx, key, expiration)
		}
	}

	// 同步作者偏好
	if profile.AuthorPreference != "" {
		var authors map[string]float64
		if json.Unmarshal([]byte(profile.AuthorPreference), &authors) == nil {
			key := fmt.Sprintf("user:author_prefer:%d", userID)
			pipe.Del(ctx, key)
			for author, weight := range authors {
				pipe.ZAdd(ctx, key, &redis.Z{Score: weight, Member: author})
			}
			pipe.Expire(ctx, key, expiration)
		}
	}

	// 同步统计信息
	statsKey := fmt.Sprintf("user:stats:%d", userID)
	pipe.HSet(ctx, statsKey, map[string]interface{}{
		"avg_watch_time": profile.AvgWatchDuration,
		"like_rate":      profile.LikeRate,
		"comment_rate":   profile.CommentRate,
		"share_rate":     profile.ShareRate,
		"active_level":   profile.UserLevel,
	})
	pipe.Expire(ctx, statsKey, expiration)

	_, err := pipe.Exec(ctx)
	return err
}

// PersistUserProfileToDB 将缓存中的用户画像持久化到数据库
func (ups *UserProfileService) PersistUserProfileToDB(ctx context.Context, userID int64) error {
	// 从 Redis 获取画像数据
	pipe := ups.redis.Pipeline()

	interestsCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:interests:%d", userID), 0, int64(ups.config.MaxInterestTags-1))
	categoryCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:category_prefer:%d", userID), 0, int64(ups.config.MaxCategoryPrefer-1))
	authorCmd := pipe.ZRevRangeWithScores(ctx, fmt.Sprintf("user:author_prefer:%d", userID), 0, int64(ups.config.MaxAuthorPrefer-1))
	statsCmd := pipe.HGetAll(ctx, fmt.Sprintf("user:stats:%d", userID))

	pipe.Exec(ctx)

	// 构建更新数据
	updates := make(map[string]interface{})
	updates["updated_at"] = time.Now()

	// 兴趣标签
	if interests, err := interestsCmd.Result(); err == nil && len(interests) > 0 {
		tags := make(map[string]float64)
		for _, z := range interests {
			tags[z.Member.(string)] = z.Score
		}
		if data, err := json.Marshal(tags); err == nil {
			updates["interest_tags"] = string(data)
		}
	}

	// 分类偏好
	if categories, err := categoryCmd.Result(); err == nil && len(categories) > 0 {
		cats := make(map[string]float64)
		for _, z := range categories {
			cats[z.Member.(string)] = z.Score
		}
		if data, err := json.Marshal(cats); err == nil {
			updates["category_preference"] = string(data)
		}
	}

	// 作者偏好
	if authors, err := authorCmd.Result(); err == nil && len(authors) > 0 {
		auths := make(map[string]float64)
		for _, z := range authors {
			auths[z.Member.(string)] = z.Score
		}
		if data, err := json.Marshal(auths); err == nil {
			updates["author_preference"] = string(data)
		}
	}

	// 统计数据
	if stats, err := statsCmd.Result(); err == nil {
		if v, ok := stats["avg_watch_time"]; ok {
			updates["avg_watch_duration"] = v
		}
		if v, ok := stats["like_rate"]; ok {
			updates["like_rate"] = v
		}
		if v, ok := stats["interact_rate"]; ok {
			updates["interact_rate"] = v
		}
	}

	// 更新或创建数据库记录
	result := ups.db.WithContext(ctx).Table("user_profiles").
		Where("user_id = ?", userID).
		Updates(updates)

	if result.RowsAffected == 0 {
		// 记录不存在，创建新记录
		updates["user_id"] = userID
		updates["created_at"] = time.Now()
		return ups.db.WithContext(ctx).Table("user_profiles").Create(updates).Error
	}

	return result.Error
}

// DecayAllUserProfiles 衰减所有用户画像（定时任务）
func (ups *UserProfileService) DecayAllUserProfiles(ctx context.Context) error {
	// 获取所有有画像的用户
	// 这里简化处理，实际应该分批处理
	var userIDs []int64
	err := ups.db.WithContext(ctx).Table("user_profiles").
		Select("user_id").
		Pluck("user_id", &userIDs).Error
	if err != nil {
		return err
	}

	for _, userID := range userIDs {
		if err := ups.decayUserProfile(ctx, userID); err != nil {
			hlog.Warnf("Failed to decay profile for user %d: %v", userID, err)
		}
	}

	return nil
}

// decayUserProfile 衰减单个用户画像
func (ups *UserProfileService) decayUserProfile(ctx context.Context, userID int64) error {
	keys := []string{
		fmt.Sprintf("user:interests:%d", userID),
		fmt.Sprintf("user:category_prefer:%d", userID),
		fmt.Sprintf("user:author_prefer:%d", userID),
	}

	for _, key := range keys {
		// 获取所有成员
		members, err := ups.redis.ZRangeWithScores(ctx, key, 0, -1).Result()
		if err != nil {
			continue
		}

		// 衰减每个成员的分数
		pipe := ups.redis.Pipeline()
		for _, z := range members {
			newScore := z.Score * ups.config.DecayFactor
			if newScore < 0.01 {
				pipe.ZRem(ctx, key, z.Member)
			} else {
				pipe.ZAdd(ctx, key, &redis.Z{Score: newScore, Member: z.Member})
			}
		}
		pipe.Exec(ctx)
	}

	return nil
}

// ========================================
// 冷启动处理
// ========================================

// InitNewUserProfile 初始化新用户画像（冷启动）
func (ups *UserProfileService) InitNewUserProfile(ctx context.Context, userID int64, initialPreferences map[string]interface{}) error {
	pipe := ups.redis.Pipeline()
	expiration := time.Duration(ups.config.RedisExpireDays) * 24 * time.Hour

	// 设置默认兴趣标签（基于热门标签）
	if tags, ok := initialPreferences["tags"].([]string); ok {
		key := fmt.Sprintf("user:interests:%d", userID)
		for _, tag := range tags {
			pipe.ZAdd(ctx, key, &redis.Z{Score: 1.0, Member: tag})
		}
		pipe.Expire(ctx, key, expiration)
	}

	// 设置默认分类偏好
	if categories, ok := initialPreferences["categories"].([]string); ok {
		key := fmt.Sprintf("user:category_prefer:%d", userID)
		for _, cat := range categories {
			pipe.ZAdd(ctx, key, &redis.Z{Score: 1.0, Member: cat})
		}
		pipe.Expire(ctx, key, expiration)
	}

	// 初始化统计
	statsKey := fmt.Sprintf("user:stats:%d", userID)
	pipe.HSet(ctx, statsKey, map[string]interface{}{
		"active_level":   1,
		"total_views":    0,
		"total_likes":    0,
		"total_comments": 0,
	})
	pipe.Expire(ctx, statsKey, expiration)

	_, err := pipe.Exec(ctx)
	return err
}

// GetPopularTagsForColdStart 获取热门标签用于冷启动
func (ups *UserProfileService) GetPopularTagsForColdStart(ctx context.Context, limit int) ([]string, error) {
	key := "global:popular_tags"
	tags, err := ups.redis.ZRevRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil || len(tags) == 0 {
		// 从数据库统计
		return ups.computePopularTags(ctx, limit)
	}
	return tags, nil
}

// computePopularTags 计算热门标签
func (ups *UserProfileService) computePopularTags(ctx context.Context, limit int) ([]string, error) {
	var results []struct {
		Tag   string `gorm:"column:tag_name"`
		Count int64  `gorm:"column:cnt"`
	}

	err := ups.db.WithContext(ctx).Table("tag_video_mappings").
		Select("tag_name, COUNT(*) as cnt").
		Group("tag_name").
		Order("cnt DESC").
		Limit(limit).
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(results))
	for _, r := range results {
		tags = append(tags, r.Tag)
	}

	return tags, nil
}
