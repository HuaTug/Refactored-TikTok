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

// =====================================================
// Realtime State Service
// Captures and computes user short-term behavioral indicators
// from a Redis-backed action sequence.
// =====================================================

// UserAction represents a single user behavior event.
type UserAction struct {
	VideoID    int64   `json:"video_id"`
	ActionType string  `json:"action_type"` // view/like/comment/share/finish/favorite/dislike
	Timestamp  int64   `json:"timestamp"`   // Unix timestamp in milliseconds
	Duration   int     `json:"duration"`    // Dwell time in seconds
	Progress   float64 `json:"progress"`    // Watch progress 0.0 - 1.0
	Category   string  `json:"category"`    // Video category (denormalized for fast access)
	Tags       string  `json:"tags"`        // Comma-separated video tags
}

// RealtimeStateService computes user realtime state from Redis-backed action streams.
type RealtimeStateService struct {
	redis  *redis.Client
	db     *gorm.DB
	config *RealtimeStateConfig
}

// RealtimeStateConfig holds tunable parameters for the realtime state service.
type RealtimeStateConfig struct {
	// MaxActions is the maximum number of recent actions to keep per user.
	MaxActions int `json:"max_actions"`
	// TTL is the expiration time for the action stream key.
	TTL time.Duration `json:"ttl"`
	// SkipDwellThreshold is the dwell time (seconds) below which a view counts as a "skip".
	SkipDwellThreshold int `json:"skip_dwell_threshold"`
	// DeepInteractionMinProgress is the minimum progress to consider a view as "deep interaction".
	DeepInteractionMinProgress float64 `json:"deep_interaction_min_progress"`
	// DeepInteractionMinCount is the minimum consecutive deep interactions to detect a focused topic.
	DeepInteractionMinCount int `json:"deep_interaction_min_count"`
}

// DefaultRealtimeStateConfig returns sensible defaults.
func DefaultRealtimeStateConfig() *RealtimeStateConfig {
	return &RealtimeStateConfig{
		MaxActions:                 50,
		TTL:                        30 * time.Minute,
		SkipDwellThreshold:         3,
		DeepInteractionMinProgress: 0.8,
		DeepInteractionMinCount:    3,
	}
}

// NewRealtimeStateService creates a new realtime state service.
func NewRealtimeStateService(redisClient *redis.Client, db *gorm.DB, config *RealtimeStateConfig) *RealtimeStateService {
	if config == nil {
		config = DefaultRealtimeStateConfig()
	}
	return &RealtimeStateService{
		redis:  redisClient,
		db:     db,
		config: config,
	}
}

// realtimeActionKey returns the Redis key for a user's recent action stream.
func realtimeActionKey(userID int64) string {
	return fmt.Sprintf("user:recent_actions:%d", userID)
}

// RecordRealtimeAction writes a user action into the Redis Sorted Set.
// This method is designed to be called non-blocking from UserProfileService.RecordAction().
// Redis write failures are logged but never block the caller.
func (s *RealtimeStateService) RecordRealtimeAction(ctx context.Context, userID int64, action UserAction) {
	if s.redis == nil {
		return
	}

	key := realtimeActionKey(userID)

	// Serialize action to JSON
	data, err := json.Marshal(action)
	if err != nil {
		hlog.Warnf("[RealtimeState] Failed to marshal action for user %d: %v", userID, err)
		return
	}

	pipe := s.redis.Pipeline()

	// Add to sorted set with timestamp as score
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(action.Timestamp),
		Member: string(data),
	})

	// Trim to keep only the most recent MaxActions entries
	// ZREMRANGEBYRANK key 0 -(MaxActions+1) removes the oldest entries
	if s.config.MaxActions > 0 {
		pipe.ZRemRangeByRank(ctx, key, 0, int64(-s.config.MaxActions-1))
	}

	// Set TTL
	pipe.Expire(ctx, key, s.config.TTL)

	if _, err := pipe.Exec(ctx); err != nil {
		hlog.Warnf("[RealtimeState] Failed to record action for user %d: %v", userID, err)
	}
}

// GetUserRealtimeState computes the user's realtime state from the Redis action stream.
// If Redis data is unavailable or insufficient, it degrades to long-term UserProfile data.
func (s *RealtimeStateService) GetUserRealtimeState(ctx context.Context, userID int64) (*UserRealtimeState, error) {
	state := &UserRealtimeState{}

	// Try to read from Redis
	actions, err := s.readActions(ctx, userID)
	if err != nil || len(actions) == 0 {
		// Degrade to long-term profile data
		return s.buildDegradedState(ctx, userID)
	}

	state.RecentActionCount = len(actions)

	// Compute EngagementLevel: average completion rate of recent videos with Progress field
	state.EngagementLevel = s.computeEngagementLevel(actions)

	// Compute SwipeSpeed: average dwell time of recent videos
	state.SwipeSpeed = s.computeSwipeSpeed(actions)

	// Compute ExplorationEntropy: category distribution entropy
	state.ExplorationEntropy = s.computeExplorationEntropy(actions)

	// Compute FocusedTopic: detect sustained deep interaction with a single topic
	state.FocusedTopic = s.detectFocusedTopic(actions)

	// Compute ConsecutiveSkips: count of consecutive quick skips from the most recent action
	state.ConsecutiveSkips = s.computeConsecutiveSkips(actions)

	return state, nil
}

// readActions reads and parses the recent action sequence from Redis.
// Actions are returned in chronological order (oldest first).
func (s *RealtimeStateService) readActions(ctx context.Context, userID int64) ([]UserAction, error) {
	if s.redis == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	key := realtimeActionKey(userID)
	results, err := s.redis.ZRangeWithScores(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read actions from Redis: %w", err)
	}

	actions := make([]UserAction, 0, len(results))
	for _, z := range results {
		var action UserAction
		memberStr, ok := z.Member.(string)
		if !ok {
			continue
		}
		if err := json.Unmarshal([]byte(memberStr), &action); err != nil {
			continue
		}
		actions = append(actions, action)
	}

	return actions, nil
}

// computeEngagementLevel calculates average completion rate from the last 5-10 actions with Progress.
func (s *RealtimeStateService) computeEngagementLevel(actions []UserAction) float64 {
	var totalProgress float64
	var count int

	// Iterate from newest to oldest
	for i := len(actions) - 1; i >= 0 && count < 10; i-- {
		if actions[i].Progress > 0 {
			totalProgress += actions[i].Progress
			count++
		}
	}

	if count == 0 {
		return 0.5 // default medium engagement
	}
	return totalProgress / float64(count)
}

// computeSwipeSpeed calculates average dwell time from the last 10 actions.
func (s *RealtimeStateService) computeSwipeSpeed(actions []UserAction) float64 {
	var totalDwell float64
	var count int

	for i := len(actions) - 1; i >= 0 && count < 10; i-- {
		if actions[i].Duration > 0 {
			totalDwell += float64(actions[i].Duration)
			count++
		} else if i > 0 {
			// Estimate dwell from timestamp difference (ms to seconds)
			dwell := float64(actions[i].Timestamp-actions[i-1].Timestamp) / 1000.0
			if dwell > 0 && dwell < 300 { // sanity check: max 5 minutes
				totalDwell += dwell
				count++
			}
		}
	}

	if count == 0 {
		return 5.0 // default 5 seconds
	}
	return totalDwell / float64(count)
}

// computeExplorationEntropy calculates the Shannon entropy of category distribution
// from the most recent 20 actions.
func (s *RealtimeStateService) computeExplorationEntropy(actions []UserAction) float64 {
	categoryCount := make(map[string]int)
	total := 0

	start := len(actions) - 20
	if start < 0 {
		start = 0
	}

	for i := start; i < len(actions); i++ {
		cat := actions[i].Category
		if cat == "" {
			cat = "unknown"
		}
		categoryCount[cat]++
		total++
	}

	if total == 0 {
		return 0
	}

	entropy := 0.0
	for _, count := range categoryCount {
		p := float64(count) / float64(total)
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// detectFocusedTopic detects if the user has consecutive deep interactions
// with the same category/tag. Returns the topic or empty string.
func (s *RealtimeStateService) detectFocusedTopic(actions []UserAction) string {
	if len(actions) < s.config.DeepInteractionMinCount {
		return ""
	}

	// Walk backwards through actions looking for consecutive deep interactions
	type topicStreak struct {
		topic string
		count int
	}

	var currentStreak topicStreak

	for i := len(actions) - 1; i >= 0; i-- {
		a := actions[i]
		isDeep := a.Progress >= s.config.DeepInteractionMinProgress &&
			(a.ActionType == "like" || a.ActionType == "finish" || a.ActionType == "favorite" ||
				a.ActionType == "view" && a.Progress >= s.config.DeepInteractionMinProgress)

		if !isDeep {
			if currentStreak.count >= s.config.DeepInteractionMinCount {
				return currentStreak.topic
			}
			currentStreak = topicStreak{}
			continue
		}

		topic := a.Category
		if topic == "" {
			// Fallback to first tag
			if a.Tags != "" {
				for j := 0; j < len(a.Tags); j++ {
					if a.Tags[j] == ',' {
						topic = a.Tags[:j]
						break
					}
				}
				if topic == "" {
					topic = a.Tags
				}
			}
		}

		if topic == "" {
			continue
		}

		if currentStreak.topic == topic {
			currentStreak.count++
		} else {
			if currentStreak.count >= s.config.DeepInteractionMinCount {
				return currentStreak.topic
			}
			currentStreak = topicStreak{topic: topic, count: 1}
		}
	}

	if currentStreak.count >= s.config.DeepInteractionMinCount {
		return currentStreak.topic
	}

	return ""
}

// computeConsecutiveSkips counts consecutive quick-skip actions from the most recent.
func (s *RealtimeStateService) computeConsecutiveSkips(actions []UserAction) int {
	count := 0
	for i := len(actions) - 1; i >= 0; i-- {
		if actions[i].Duration > 0 && actions[i].Duration < s.config.SkipDwellThreshold {
			count++
		} else if actions[i].Duration == 0 && actions[i].Progress > 0 && actions[i].Progress < 0.1 {
			// If no explicit duration, check progress: < 10% is likely a skip
			count++
		} else {
			break // Streak broken
		}
	}
	return count
}

// buildDegradedState constructs a UserRealtimeState from long-term UserProfile data
// when Redis data is unavailable or insufficient.
func (s *RealtimeStateService) buildDegradedState(ctx context.Context, userID int64) (*UserRealtimeState, error) {
	state := &UserRealtimeState{
		EngagementLevel:    0.5,
		SwipeSpeed:         5.0,
		ExplorationEntropy: 1.0,
		RecentActionCount:  0,
	}

	if s.db == nil {
		return state, nil
	}

	// Read from user_profiles table
	var profile struct {
		AvgCompletionRate float64 `gorm:"column:avg_completion_rate"`
		LikeRate          float64 `gorm:"column:like_rate"`
		AvgWatchDuration  float64 `gorm:"column:avg_watch_duration"`
		TotalViewCount    int64   `gorm:"column:total_view_count"`
	}

	err := s.db.WithContext(ctx).Table("user_profiles").
		Where("user_id = ?", userID).
		Select("avg_completion_rate, like_rate, avg_watch_duration, total_view_count").
		First(&profile).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// New user, keep defaults
			return state, nil
		}
		return state, nil // Degrade gracefully
	}

	state.EngagementLevel = profile.AvgCompletionRate
	if state.EngagementLevel == 0 {
		state.EngagementLevel = 0.5
	}

	state.SwipeSpeed = profile.AvgWatchDuration
	if state.SwipeSpeed == 0 {
		state.SwipeSpeed = 5.0
	}

	// Estimate action count from total views (for cold-start detection)
	if profile.TotalViewCount > 0 {
		state.RecentActionCount = 50 // assume non-cold-start
	}

	return state, nil
}

// =====================================================
// Global Singleton for RealtimeStateService
// =====================================================

var (
	globalRealtimeStateSvc *RealtimeStateService
	realtimeStateSvcOnce   sync.Once
)

// InitRealtimeStateService initializes the global RealtimeStateService singleton.
func InitRealtimeStateService(redisClient *redis.Client, db *gorm.DB, config *RealtimeStateConfig) {
	realtimeStateSvcOnce.Do(func() {
		globalRealtimeStateSvc = NewRealtimeStateService(redisClient, db, config)
		hlog.Info("[RealtimeState] RealtimeStateService initialized")
	})
}

// GetRealtimeStateServiceInstance returns the global RealtimeStateService instance.
func GetRealtimeStateServiceInstance() *RealtimeStateService {
	return globalRealtimeStateSvc
}
