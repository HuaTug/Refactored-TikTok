package recommendation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/kitex_gen/videos"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// =====================================================
// Recommendation Agent: Intelligent Decision Layer
// Sits on top of the existing recommendation engine
// and dynamically selects the best pipeline based on
// real-time user state.
// =====================================================

// --- Strategy Types ---

// RecommendStrategy defines the type of recommendation pipeline to execute.
type RecommendStrategy string

const (
	// StrategyStandard uses the existing IntegratedRecommendationEngine full pipeline.
	StrategyStandard RecommendStrategy = "STANDARD"
	// StrategyHotExplore triggers hot/trending content exploration for disengaged users.
	StrategyHotExplore RecommendStrategy = "HOT_EXPLORE"
	// StrategyTopicDeepDive performs deep retrieval for a focused interest topic.
	StrategyTopicDeepDive RecommendStrategy = "TOPIC_DEEP_DIVE"
	// StrategyColdStart handles new users with minimal behavioral data.
	StrategyColdStart RecommendStrategy = "COLD_START"
)

// --- User Realtime State ---

// UserRealtimeState captures a user's short-term behavioral indicators,
// computed from the most recent action sequence stored in Redis.
type UserRealtimeState struct {
	// EngagementLevel is the average completion rate of recent videos (0.0 - 1.0).
	EngagementLevel float64 `json:"engagement_level"`
	// SwipeSpeed is the average dwell time (seconds) on recent videos.
	SwipeSpeed float64 `json:"swipe_speed"`
	// ExplorationEntropy measures how diverse the user's recent viewing is (higher = more diverse).
	ExplorationEntropy float64 `json:"exploration_entropy"`
	// FocusedTopic is the detected topic when the user shows sustained deep interest.
	// Empty string means no focused topic detected.
	FocusedTopic string `json:"focused_topic"`
	// ConsecutiveSkips counts how many videos in a row the user skipped (dwell < 3s).
	ConsecutiveSkips int `json:"consecutive_skips"`
	// RecentActionCount is the total number of recent actions available.
	RecentActionCount int `json:"recent_action_count"`
}

// --- Decision Strategy Interface ---

// DecisionStrategy defines a pluggable strategy for the Agent to decide
// which recommendation pipeline to use based on the user's realtime state.
type DecisionStrategy interface {
	// Decide analyzes the user state and returns the chosen strategy
	// along with optional parameters (e.g., topic keyword for TOPIC_DEEP_DIVE).
	Decide(state *UserRealtimeState) (RecommendStrategy, map[string]string)
}

// --- Agent Configuration ---

// AgentConfig holds all tunable parameters for the RecommendationAgent.
type AgentConfig struct {
	// Enabled controls whether the Agent decision layer is active.
	// When false, all requests go directly to IntegratedRecommendationEngine.
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`

	// ConsecutiveSkipThreshold is the number of consecutive skips that triggers HOT_EXPLORE.
	ConsecutiveSkipThreshold int `json:"consecutive_skip_threshold" yaml:"consecutive_skip_threshold" mapstructure:"consecutive_skip_threshold"`
	// DeepInteractionThreshold is the minimum consecutive deep interactions to trigger TOPIC_DEEP_DIVE.
	DeepInteractionThreshold int `json:"deep_interaction_threshold" yaml:"deep_interaction_threshold" mapstructure:"deep_interaction_threshold"`
	// EngagementThreshold is the engagement level below which HOT_EXPLORE may trigger.
	EngagementThreshold float64 `json:"engagement_threshold" yaml:"engagement_threshold" mapstructure:"engagement_threshold"`
	// ColdStartActionThreshold is the minimum action count below which COLD_START triggers.
	ColdStartActionThreshold int `json:"cold_start_action_threshold" yaml:"cold_start_action_threshold" mapstructure:"cold_start_action_threshold"`

	// MaxNonStandardRatio limits the proportion of non-STANDARD recall sources in the final list.
	MaxNonStandardRatio float64 `json:"max_non_standard_ratio" yaml:"max_non_standard_ratio" mapstructure:"max_non_standard_ratio"`
	// HotExploreTimeoutMs is the maximum time (in ms) allowed for the HOT_EXPLORE pipeline.
	HotExploreTimeoutMs int `json:"hot_explore_timeout_ms" yaml:"hot_explore_timeout_ms" mapstructure:"hot_explore_timeout_ms"`
	// TopicDeepDiveMinCandidates is the minimum number of candidates required for TOPIC_DEEP_DIVE.
	TopicDeepDiveMinCandidates int `json:"topic_deep_dive_min_candidates" yaml:"topic_deep_dive_min_candidates" mapstructure:"topic_deep_dive_min_candidates"`
}

// DefaultAgentConfig returns sensible defaults for the Agent configuration.
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Enabled:                    true,
		ConsecutiveSkipThreshold:   5,
		DeepInteractionThreshold:   3,
		EngagementThreshold:        0.15,
		ColdStartActionThreshold:   10,
		MaxNonStandardRatio:        0.3,
		HotExploreTimeoutMs:        300,
		TopicDeepDiveMinCandidates: 5,
	}
}

// --- Agent Stats (Observability) ---

// AgentStats holds atomic counters for Agent observability.
type AgentStats struct {
	TotalRequests       int64
	StandardCount       int64
	HotExploreCount     int64
	TopicDeepDiveCount  int64
	ColdStartCount      int64
	FallbackCount       int64
	TotalDecisionTimeNs int64
	CTRSuccessCount     int64
	CTRFailureCount     int64
	TotalLatencyNs      int64
}

// Snapshot returns a copy of the current stats for reporting.
func (s *AgentStats) Snapshot() map[string]interface{} {
	totalReqs := atomic.LoadInt64(&s.TotalRequests)
	avgDecisionUs := int64(0)
	avgLatencyMs := int64(0)
	if totalReqs > 0 {
		avgDecisionUs = atomic.LoadInt64(&s.TotalDecisionTimeNs) / totalReqs / 1000
		avgLatencyMs = atomic.LoadInt64(&s.TotalLatencyNs) / totalReqs / 1e6
	}
	ctrSuccess := atomic.LoadInt64(&s.CTRSuccessCount)
	ctrFail := atomic.LoadInt64(&s.CTRFailureCount)
	ctrTotal := ctrSuccess + ctrFail
	ctrSuccessRate := 0.0
	if ctrTotal > 0 {
		ctrSuccessRate = float64(ctrSuccess) / float64(ctrTotal)
	}

	return map[string]interface{}{
		"total_requests":           totalReqs,
		"strategy_standard":        atomic.LoadInt64(&s.StandardCount),
		"strategy_hot_explore":     atomic.LoadInt64(&s.HotExploreCount),
		"strategy_topic_deep_dive": atomic.LoadInt64(&s.TopicDeepDiveCount),
		"strategy_cold_start":      atomic.LoadInt64(&s.ColdStartCount),
		"strategy_fallback":        atomic.LoadInt64(&s.FallbackCount),
		"avg_decision_time_us":     avgDecisionUs,
		"avg_e2e_latency_ms":       avgLatencyMs,
		"ctr_success_rate":         ctrSuccessRate,
		"ctr_success_count":        ctrSuccess,
		"ctr_failure_count":        ctrFail,
	}
}

// --- Recommendation Agent ---

// RecommendationAgent is the intelligent decision layer that sits on top of
// the existing recommendation engine. It observes the user's real-time state
// and dynamically selects the optimal recommendation pipeline.
type RecommendationAgent struct {
	// Core dependencies (all existing components, injected via constructor)
	integratedEngine *IntegratedRecommendationEngine
	recallEngine     *RecommendationEngine
	ctrClient        *CTRServiceClient
	hotScoreService  *VideoHotScoreService
	userProfileSvc   *UserProfileService
	realtimeStateSvc *RealtimeStateService

	// Pluggable decision strategy
	decisionStrategy DecisionStrategy

	// Configuration
	config *AgentConfig

	// Observability
	stats *AgentStats
}

// NewRecommendationAgent creates a new Agent with all dependencies injected.
func NewRecommendationAgent(
	integratedEngine *IntegratedRecommendationEngine,
	recallEngine *RecommendationEngine,
	ctrClient *CTRServiceClient,
	hotScoreService *VideoHotScoreService,
	userProfileSvc *UserProfileService,
	realtimeStateSvc *RealtimeStateService,
	config *AgentConfig,
) *RecommendationAgent {
	if config == nil {
		config = DefaultAgentConfig()
	}

	agent := &RecommendationAgent{
		integratedEngine: integratedEngine,
		recallEngine:     recallEngine,
		ctrClient:        ctrClient,
		hotScoreService:  hotScoreService,
		userProfileSvc:   userProfileSvc,
		realtimeStateSvc: realtimeStateSvc,
		decisionStrategy: NewRuleBasedDecisionStrategy(config),
		config:           config,
		stats:            &AgentStats{},
	}

	return agent
}

// SetDecisionStrategy replaces the current decision strategy (e.g., swap to LLM-based).
func (a *RecommendationAgent) SetDecisionStrategy(strategy DecisionStrategy) {
	a.decisionStrategy = strategy
}

// GetStats returns the Agent's runtime statistics.
func (a *RecommendationAgent) GetStats() *AgentStats {
	return a.stats
}

// =====================================================
// Main Recommend Flow (Task 4)
// =====================================================

// Recommend is the Agent's primary entry point. It replaces direct calls to
// IntegratedRecommendationEngine.Recommend() and adds the intelligent decision layer.
func (a *RecommendationAgent) Recommend(ctx context.Context, req *RecommendRequest) (resp *RecommendResponse, err error) {
	startTime := time.Now()
	atomic.AddInt64(&a.stats.TotalRequests, 1)

	// Defer: record total latency
	defer func() {
		atomic.AddInt64(&a.stats.TotalLatencyNs, time.Since(startTime).Nanoseconds())
	}()

	// === Zero-cost bypass: if Agent is disabled, go straight to the existing engine ===
	if !a.config.Enabled {
		return a.integratedEngine.Recommend(ctx, req)
	}

	// === Panic/error safety net: any failure falls back to STANDARD ===
	defer func() {
		if r := recover(); r != nil {
			hlog.Errorf("[Agent] Panic recovered in Recommend, falling back to STANDARD: %v", r)
			atomic.AddInt64(&a.stats.FallbackCount, 1)
			resp, err = a.executeStandard(ctx, req)
		}
	}()

	// === A/B Test: check user's experiment assignment ===
	experimentID, groupID, isControl := a.resolveABTest(ctx, req.UserID)
	if isControl {
		// Control group: bypass Agent, use standard engine directly
		hlog.Infof("[Agent/AB] user_id=%d experiment=%d group=%d -> control (STANDARD)", req.UserID, experimentID, groupID)
		resp, err = a.executeStandard(ctx, req)
		if resp != nil {
			resp.ExperimentID = experimentID
			resp.GroupID = groupID
		}
		return
	}

	// --- Phase 1: Perceive (get user realtime state) ---
	perceiveStart := time.Now()
	var userState *UserRealtimeState
	if a.realtimeStateSvc != nil {
		userState, err = a.realtimeStateSvc.GetUserRealtimeState(ctx, req.UserID)
		if err != nil {
			hlog.Warnf("[Agent] Failed to get realtime state for user %d, using defaults: %v", req.UserID, err)
			userState = &UserRealtimeState{
				EngagementLevel:    0.5,
				SwipeSpeed:         5.0,
				ExplorationEntropy: 1.0,
				RecentActionCount:  50, // assume non-cold-start
			}
		}
	} else {
		userState = &UserRealtimeState{
			EngagementLevel:    0.5,
			SwipeSpeed:         5.0,
			ExplorationEntropy: 1.0,
			RecentActionCount:  50,
		}
	}
	perceiveLatency := time.Since(perceiveStart)

	// --- Phase 2: Decide (choose strategy) ---
	decideStart := time.Now()
	strategy, params := a.decisionStrategy.Decide(userState)
	decideLatency := time.Since(decideStart)
	atomic.AddInt64(&a.stats.TotalDecisionTimeNs, decideLatency.Nanoseconds())

	// Record strategy selection
	switch strategy {
	case StrategyStandard:
		atomic.AddInt64(&a.stats.StandardCount, 1)
	case StrategyHotExplore:
		atomic.AddInt64(&a.stats.HotExploreCount, 1)
	case StrategyColdStart:
		atomic.AddInt64(&a.stats.ColdStartCount, 1)
	case StrategyTopicDeepDive:
		atomic.AddInt64(&a.stats.TopicDeepDiveCount, 1)
	}

	hlog.Infof("[Agent] request_id=%s user_id=%d decision=%s "+
		"engagement=%.2f skips=%d focused_topic=%q action_count=%d "+
		"perceive_ms=%d decide_us=%d",
		req.RequestID, req.UserID, strategy,
		userState.EngagementLevel, userState.ConsecutiveSkips, userState.FocusedTopic, userState.RecentActionCount,
		perceiveLatency.Milliseconds(), decideLatency.Microseconds())

	// --- Phase 3: Execute (dispatch to chosen pipeline) ---
	executeStart := time.Now()
	switch strategy {
	case StrategyHotExplore:
		resp, err = a.executeHotExplore(ctx, req, params)
	case StrategyTopicDeepDive:
		resp, err = a.executeTopicDeepDive(ctx, req, params)
	case StrategyColdStart:
		resp, err = a.executeColdStart(ctx, req, params)
	default:
		resp, err = a.executeStandard(ctx, req)
	}

	// Fallback on error
	if err != nil {
		hlog.Warnf("[Agent] Pipeline %s failed for user %d: %v, falling back to STANDARD", strategy, req.UserID, err)
		atomic.AddInt64(&a.stats.FallbackCount, 1)
		resp, err = a.executeStandard(ctx, req)
	}

	executeLatency := time.Since(executeStart)

	// --- Phase 4: Log completion and tag experiment info ---
	if resp != nil {
		// Tag A/B test info
		resp.ExperimentID = experimentID
		resp.GroupID = groupID

		candidateCounts := "n/a"
		if resp.RecallStats != nil {
			candidateCounts = fmt.Sprintf("%v", resp.RecallStats)
		}
		hlog.Infof("[Agent] request_id=%s user_id=%d strategy=%s experiment=%d group=%d "+
			"execute_ms=%d candidates=%d final=%d recall_stats=%s",
			req.RequestID, req.UserID, strategy, experimentID, groupID,
			executeLatency.Milliseconds(), resp.CandidateSize, len(resp.Videos), candidateCounts)
	}

	return resp, err
}

// =====================================================
// Strategy Execution Methods
// =====================================================

// executeStandard delegates directly to the existing IntegratedRecommendationEngine.
func (a *RecommendationAgent) executeStandard(ctx context.Context, req *RecommendRequest) (*RecommendResponse, error) {
	return a.integratedEngine.Recommend(ctx, req)
}

// executeHotExplore handles the HOT_EXPLORE strategy:
// 1. Fetch hot/trending videos from VideoHotScoreService
// 2. Supplement with quick recall (hot + new video)
// 3. Unified fine-ranking and enhanced reranking
func (a *RecommendationAgent) executeHotExplore(ctx context.Context, req *RecommendRequest, params map[string]string) (*RecommendResponse, error) {
	timeoutMs := a.config.HotExploreTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	candidateSet := make(map[int64]string) // videoID -> recallSource
	var mu sync.Mutex
	var wg sync.WaitGroup

	// --- Branch 1: Hot score service (primary) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.hotScoreService == nil {
			return
		}
		hotIDs, err := a.hotScoreService.GetTopHotVideos(ctx, "1h", 30)
		if err != nil {
			hlog.Warnf("[Agent/HotExplore] GetTopHotVideos failed: %v", err)
			return
		}
		mu.Lock()
		for _, vid := range hotIDs {
			candidateSet[vid] = "hot_explore"
		}
		mu.Unlock()
	}()

	// --- Branch 2: Trending videos ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.hotScoreService == nil {
			return
		}
		trends, err := a.hotScoreService.GetTrendingVideos(ctx, 20)
		if err != nil {
			hlog.Warnf("[Agent/HotExplore] GetTrendingVideos failed: %v", err)
			return
		}
		mu.Lock()
		for _, t := range trends {
			if _, exists := candidateSet[t.VideoID]; !exists {
				candidateSet[t.VideoID] = "hot_explore"
			}
		}
		mu.Unlock()
	}()

	// --- Branch 3: Quick recall supplement (hot + new video strategies only) ---
	if a.recallEngine != nil {
		for _, strategy := range a.recallEngine.recallStrategies {
			name := strategy.Name()
			if name == "hot_video" || name == "new_video" {
				wg.Add(1)
				go func(s RecallStrategy) {
					defer wg.Done()
					ids, err := s.Recall(ctx, req.UserID, 20)
					if err != nil {
						return
					}
					mu.Lock()
					for _, vid := range ids {
						if _, exists := candidateSet[vid]; !exists {
							candidateSet[vid] = "hot_explore"
						}
					}
					mu.Unlock()
				}(strategy)
			}
		}
	}

	wg.Wait()

	// Check timeout
	if ctx.Err() != nil {
		hlog.Warnf("[Agent/HotExplore] Pipeline timed out, falling back to STANDARD")
		return a.executeStandard(context.Background(), req)
	}

	if len(candidateSet) == 0 {
		hlog.Warnf("[Agent/HotExplore] No candidates found, falling back to STANDARD")
		return a.executeStandard(ctx, req)
	}

	// Convert to slice
	candidateIDs := make([]int64, 0, len(candidateSet))
	recallSources := make(map[int64]string)
	for vid, src := range candidateSet {
		candidateIDs = append(candidateIDs, vid)
		recallSources[vid] = src
	}

	// Unified fine-ranking and reranking
	finalVideos, err := a.unifiedRankAndRerank(ctx, req.UserID, candidateIDs, recallSources, req.Limit)
	if err != nil {
		return a.executeStandard(ctx, req)
	}

	return &RecommendResponse{
		Videos:        finalVideos,
		RequestID:     req.RequestID,
		RecallStats:   map[string]int{"hot_explore": len(candidateSet)},
		CandidateSize: len(candidateSet),
		LatencyMs:     time.Since(time.Now()).Milliseconds(),
	}, nil
}

// executeTopicDeepDive handles the TOPIC_DEEP_DIVE strategy:
// 1. Extract focused topic from params
// 2. Multi-route recall: tag, category, search, similar video
// 3. Unified fine-ranking and enhanced reranking
func (a *RecommendationAgent) executeTopicDeepDive(ctx context.Context, req *RecommendRequest, params map[string]string) (*RecommendResponse, error) {
	topic := params["topic"]
	if topic == "" {
		hlog.Warnf("[Agent/TopicDeepDive] No topic provided, falling back to STANDARD")
		return a.executeStandard(ctx, req)
	}

	candidateSet := make(map[int64]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// --- Route 1: Tag-based recall via Redis ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.recallEngine == nil || a.recallEngine.redis == nil {
			return
		}
		tagKey := fmt.Sprintf("tag:videos:%s", topic)
		results, err := a.recallEngine.redis.ZRevRange(ctx, tagKey, 0, 29).Result()
		if err != nil {
			return
		}
		mu.Lock()
		for _, vidStr := range results {
			var vid int64
			fmt.Sscanf(vidStr, "%d", &vid)
			if vid > 0 {
				candidateSet[vid] = "topic_deep_dive"
			}
		}
		mu.Unlock()
	}()

	// --- Route 2: Category-based recall via Redis ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.recallEngine == nil || a.recallEngine.redis == nil {
			return
		}
		catKey := fmt.Sprintf("category:videos:%s", topic)
		results, err := a.recallEngine.redis.ZRevRange(ctx, catKey, 0, 29).Result()
		if err != nil {
			return
		}
		mu.Lock()
		for _, vidStr := range results {
			var vid int64
			fmt.Sscanf(vidStr, "%d", &vid)
			if vid > 0 {
				if _, exists := candidateSet[vid]; !exists {
					candidateSet[vid] = "topic_deep_dive"
				}
			}
		}
		mu.Unlock()
	}()

	// --- Route 3: Search-based recall via RPC ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := rpc.VideoSearch(ctx, &videos.VideoSearchRequestV2{
			Keyword:  topic,
			PageNum:  1,
			PageSize: 30,
		})
		if err != nil || resp == nil {
			return
		}
		mu.Lock()
		for _, v := range resp.VideoSearch {
			if v.VideoId > 0 {
				if _, exists := candidateSet[v.VideoId]; !exists {
					candidateSet[v.VideoId] = "topic_deep_dive"
				}
			}
		}
		mu.Unlock()
	}()

	// --- Route 4: Similar video recall from recent deep-interaction videos ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.recallEngine == nil || a.recallEngine.redis == nil {
			return
		}
		// Get recent deeply-interacted video IDs from user's watch history
		watchKey := fmt.Sprintf("user:watch_history:%d", req.UserID)
		recentVids, err := a.recallEngine.redis.ZRevRange(ctx, watchKey, 0, 2).Result()
		if err != nil || len(recentVids) == 0 {
			return
		}
		for _, vidStr := range recentVids {
			var refVid int64
			fmt.Sscanf(vidStr, "%d", &refVid)
			if refVid <= 0 {
				continue
			}
			similarKey := fmt.Sprintf("video:similar:%d", refVid)
			simResults, err := a.recallEngine.redis.ZRevRange(ctx, similarKey, 0, 9).Result()
			if err != nil {
				continue
			}
			mu.Lock()
			for _, simStr := range simResults {
				var simVid int64
				fmt.Sscanf(simStr, "%d", &simVid)
				if simVid > 0 {
					if _, exists := candidateSet[simVid]; !exists {
						candidateSet[simVid] = "topic_deep_dive"
					}
				}
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	// Check if we have enough candidates
	if len(candidateSet) < a.config.TopicDeepDiveMinCandidates {
		// Fallback: try broader search
		hlog.Infof("[Agent/TopicDeepDive] Only %d candidates for topic %q, trying broader search", len(candidateSet), topic)
		resp, err := rpc.VideoSearch(ctx, &videos.VideoSearchRequestV2{
			Keyword:  topic,
			PageNum:  1,
			PageSize: 50,
		})
		if err == nil && resp != nil {
			for _, v := range resp.VideoSearch {
				if v.VideoId > 0 {
					if _, exists := candidateSet[v.VideoId]; !exists {
						candidateSet[v.VideoId] = "topic_deep_dive"
					}
				}
			}
		}
	}

	// Still not enough => fall back to STANDARD
	if len(candidateSet) < a.config.TopicDeepDiveMinCandidates {
		hlog.Warnf("[Agent/TopicDeepDive] Insufficient candidates (%d) for topic %q, falling back to STANDARD",
			len(candidateSet), topic)
		return a.executeStandard(ctx, req)
	}

	candidateIDs := make([]int64, 0, len(candidateSet))
	recallSources := make(map[int64]string)
	for vid, src := range candidateSet {
		candidateIDs = append(candidateIDs, vid)
		recallSources[vid] = src
	}

	finalVideos, err := a.unifiedRankAndRerank(ctx, req.UserID, candidateIDs, recallSources, req.Limit)
	if err != nil {
		return a.executeStandard(ctx, req)
	}

	return &RecommendResponse{
		Videos:        finalVideos,
		RequestID:     req.RequestID,
		RecallStats:   map[string]int{"topic_deep_dive": len(candidateSet)},
		CandidateSize: len(candidateSet),
	}, nil
}

// executeColdStart handles the COLD_START strategy:
// 1. Ensure user profile is initialized
// 2. Only use hot + new video recall (lightweight)
// 3. Coarse ranking only (skip CTR since user features are insufficient)
func (a *RecommendationAgent) executeColdStart(ctx context.Context, req *RecommendRequest, params map[string]string) (*RecommendResponse, error) {
	// Ensure user profile exists
	if a.userProfileSvc != nil {
		if err := a.userProfileSvc.InitNewUserProfile(ctx, req.UserID, nil); err != nil {
			hlog.Warnf("[Agent/ColdStart] Failed to init user profile for %d: %v", req.UserID, err)
		}
	}

	// Only execute hot + new video recall
	candidateSet := make(map[int64]bool)
	if a.recallEngine != nil {
		for _, strategy := range a.recallEngine.recallStrategies {
			name := strategy.Name()
			if name == "hot_video" || name == "new_video" {
				ids, err := strategy.Recall(ctx, req.UserID, 50)
				if err != nil {
					hlog.Warnf("[Agent/ColdStart] %s recall failed: %v", name, err)
					continue
				}
				for _, vid := range ids {
					candidateSet[vid] = true
				}
			}
		}
	}

	// Redis 召回为空时，fallback 到 MySQL 热度查询
	if len(candidateSet) == 0 && a.hotScoreService != nil {
		hlog.Infof("[Agent/ColdStart] Redis recall empty, falling back to MySQL hot videos")
		for _, tw := range []string{"24h", "global"} {
			hotIDs, err := a.hotScoreService.GetTopHotVideos(ctx, tw, 50)
			if err != nil {
				hlog.Warnf("[Agent/ColdStart] MySQL hot recall (%s) failed: %v", tw, err)
				continue
			}
			for _, vid := range hotIDs {
				candidateSet[vid] = true
			}
			if len(candidateSet) > 0 {
				hlog.Infof("[Agent/ColdStart] MySQL hot recall (%s) got %d videos", tw, len(candidateSet))
				break
			}
		}
	}

	if len(candidateSet) == 0 {
		return a.executeStandard(ctx, req)
	}

	candidateIDs := make([]int64, 0, len(candidateSet))
	for vid := range candidateSet {
		candidateIDs = append(candidateIDs, vid)
	}

	// Coarse ranking only (skip CTR because cold-start users lack features)
	var scoredVideos []ScoredVideo
	if a.integratedEngine != nil && a.integratedEngine.rankingModel != nil {
		ranked, err := a.integratedEngine.rankingModel.Rank(ctx, req.UserID, candidateIDs)
		if err != nil {
			hlog.Warnf("[Agent/ColdStart] Coarse ranking failed: %v", err)
			scoredVideos = a.integratedEngine.convertToScoredVideos(candidateIDs)
		} else {
			scoredVideos = ranked
		}
	} else {
		// Fallback: position-based scoring
		scoredVideos = make([]ScoredVideo, len(candidateIDs))
		for i, vid := range candidateIDs {
			scoredVideos[i] = ScoredVideo{
				VideoID:      vid,
				Score:        float64(len(candidateIDs)-i) / float64(len(candidateIDs)),
				RecallSource: "cold_start",
			}
		}
	}

	// Trim to limit
	if len(scoredVideos) > req.Limit {
		scoredVideos = scoredVideos[:req.Limit]
	}

	// Tag recall source
	for i := range scoredVideos {
		scoredVideos[i].RecallSource = "cold_start"
	}

	return &RecommendResponse{
		Videos:        scoredVideos,
		RequestID:     req.RequestID,
		RecallStats:   map[string]int{"cold_start": len(candidateSet)},
		CandidateSize: len(candidateSet),
	}, nil
}

// =====================================================
// Unified Fine-Ranking and Enhanced Reranking (Task 8)
// =====================================================

// unifiedRankAndRerank performs CTR-based fine ranking on candidate videos
// from non-standard pipelines, then applies enhanced reranking.
func (a *RecommendationAgent) unifiedRankAndRerank(
	ctx context.Context,
	userID int64,
	candidateIDs []int64,
	recallSources map[int64]string,
	limit int,
) ([]ScoredVideo, error) {

	var scoredVideos []ScoredVideo

	// --- CTR Fine Ranking ---
	if a.ctrClient != nil && a.ctrClient.IsHealthy() {
		ctxInfo := map[string]string{}
		predictions, err := a.ctrClient.Predict(ctx, userID, candidateIDs, ctxInfo)
		if err != nil {
			hlog.Warnf("[Agent/Rank] CTR prediction failed: %v, using position scores", err)
			atomic.AddInt64(&a.stats.CTRFailureCount, 1)
			scoredVideos = a.integratedEngine.convertToScoredVideos(candidateIDs)
		} else {
			atomic.AddInt64(&a.stats.CTRSuccessCount, 1)
			scoredVideos = make([]ScoredVideo, len(predictions))
			for i, pred := range predictions {
				scoredVideos[i] = PredictionToScoredVideo(pred)
			}
		}
	} else {
		atomic.AddInt64(&a.stats.CTRFailureCount, 1)
		scoredVideos = a.integratedEngine.convertToScoredVideos(candidateIDs)
	}

	// Apply freshness boost for videos created in the last 24 hours
	a.applyFreshnessBoost(ctx, scoredVideos)

	// Tag recall sources
	for i := range scoredVideos {
		if src, ok := recallSources[scoredVideos[i].VideoID]; ok {
			scoredVideos[i].RecallSource = src
		}
	}

	// Sort by score descending
	sort.Slice(scoredVideos, func(i, j int) bool {
		return scoredVideos[i].Score > scoredVideos[j].Score
	})

	// --- Enhanced Reranking ---
	scoredVideos = a.enhancedRerank(scoredVideos, limit)

	// Trim to limit
	if len(scoredVideos) > limit {
		scoredVideos = scoredVideos[:limit]
	}

	// Async: record exposures
	if a.recallEngine != nil {
		go a.recallEngine.recordExposures(context.Background(), userID, scoredVideos, "")
	}

	return scoredVideos, nil
}

// applyFreshnessBoost boosts scores for recently-created videos.
func (a *RecommendationAgent) applyFreshnessBoost(ctx context.Context, videos []ScoredVideo) {
	if a.recallEngine == nil || a.recallEngine.redis == nil {
		return
	}
	for i := range videos {
		key := fmt.Sprintf("video:feature:%d", videos[i].VideoID)
		data, err := a.recallEngine.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var feature VideoFeatureData
		if err := json.Unmarshal([]byte(data), &feature); err != nil {
			continue
		}
		if feature.CreatedAt.IsZero() {
			continue
		}
		hoursOld := time.Since(feature.CreatedAt).Hours()
		if hoursOld < 24 {
			// Freshness: exponential decay with half-life of 24h
			freshness := math.Pow(0.5, hoursOld/24.0)
			videos[i].Score *= (1.0 + 0.3*freshness)
		}
	}
}

// enhancedRerank applies multi-source diversification and ratio limits
// on top of the base scored video list.
func (a *RecommendationAgent) enhancedRerank(videos []ScoredVideo, limit int) []ScoredVideo {
	if len(videos) == 0 {
		return videos
	}

	// Step 1: Apply MMR-based diversity reranking if integrated engine is available
	if a.integratedEngine != nil && len(videos) > limit {
		videos = a.integratedEngine.rerankMMR(videos, limit, a.integratedEngine.config.DiversityLambda)
	}

	// Step 2: Multi-source scatter — no more than 3 consecutive same-source videos
	videos = a.scatterBySources(videos)

	// Step 3: Non-standard ratio limit
	videos = a.enforceNonStandardRatio(videos)

	return videos
}

// scatterBySources ensures no more than 3 consecutive videos from the same recall source.
func (a *RecommendationAgent) scatterBySources(videos []ScoredVideo) []ScoredVideo {
	if len(videos) <= 3 {
		return videos
	}

	maxConsecutive := 3
	for i := maxConsecutive; i < len(videos); i++ {
		// Check if the last maxConsecutive videos have the same source
		allSame := true
		refSource := videos[i].RecallSource
		if refSource == "" {
			continue
		}
		for j := 1; j <= maxConsecutive; j++ {
			if videos[i-j].RecallSource != refSource {
				allSame = false
				break
			}
		}
		if allSame {
			// Find next video with a different source to swap
			for j := i + 1; j < len(videos); j++ {
				if videos[j].RecallSource != refSource {
					videos[i], videos[j] = videos[j], videos[i]
					break
				}
			}
		}
	}
	return videos
}

// enforceNonStandardRatio limits hot_explore + topic_deep_dive sources to MaxNonStandardRatio.
func (a *RecommendationAgent) enforceNonStandardRatio(videos []ScoredVideo) []ScoredVideo {
	if len(videos) == 0 || a.config.MaxNonStandardRatio >= 1.0 {
		return videos
	}

	maxNonStandard := int(float64(len(videos)) * a.config.MaxNonStandardRatio)
	if maxNonStandard <= 0 {
		maxNonStandard = 1
	}

	nonStandardCount := 0
	result := make([]ScoredVideo, 0, len(videos))

	for _, v := range videos {
		if v.RecallSource == "hot_explore" || v.RecallSource == "topic_deep_dive" {
			if nonStandardCount >= maxNonStandard {
				continue // Skip excess non-standard videos
			}
			nonStandardCount++
		}
		result = append(result, v)
	}

	return result
}

// =====================================================
// Global Singleton
// =====================================================

var (
	globalRecommendationAgent *RecommendationAgent
	recommendationAgentOnce   sync.Once
)

// InitRecommendationAgent initializes the global RecommendationAgent singleton.
func InitRecommendationAgent(
	integratedEngine *IntegratedRecommendationEngine,
	recallEngine *RecommendationEngine,
	ctrClient *CTRServiceClient,
	hotScoreService *VideoHotScoreService,
	userProfileSvc *UserProfileService,
	realtimeStateSvc *RealtimeStateService,
	config *AgentConfig,
) {
	recommendationAgentOnce.Do(func() {
		globalRecommendationAgent = NewRecommendationAgent(
			integratedEngine, recallEngine, ctrClient,
			hotScoreService, userProfileSvc, realtimeStateSvc, config,
		)
		hlog.Info("[Agent] RecommendationAgent initialized")
	})
}

// GetRecommendationAgent returns the global RecommendationAgent instance.
func GetRecommendationAgent() *RecommendationAgent {
	return globalRecommendationAgent
}

// =====================================================
// A/B Test Resolution (Task 10)
// =====================================================

// resolveABTest checks if the user belongs to a running experiment and whether
// they are in the control group. Returns (experimentID, groupID, isControl).
// If no running experiments are found, returns (0, 0, false) so the Agent
// proceeds normally.
func (a *RecommendationAgent) resolveABTest(ctx context.Context, userID int64) (int64, int64, bool) {
	if a.recallEngine == nil || a.recallEngine.db == nil {
		return 0, 0, false
	}

	dbConn := a.recallEngine.db

	// Find running experiments for recommendation_agent
	var experiment struct {
		ID             int64  `gorm:"column:id"`
		ExperimentName string `gorm:"column:experiment_name"`
		Status         int8   `gorm:"column:status"`
	}
	err := dbConn.WithContext(ctx).
		Table("ab_test_experiments").
		Where("status = ? AND experiment_name LIKE ?", 1, "%recommendation_agent%").
		Order("id DESC").
		Limit(1).
		First(&experiment).Error
	if err != nil {
		// No running experiment => proceed with Agent
		return 0, 0, false
	}

	// Check user assignment
	var assignment struct {
		GroupID int64 `gorm:"column:group_id"`
	}
	err = dbConn.WithContext(ctx).
		Table("user_ab_test_assignments").
		Where("user_id = ? AND experiment_id = ?", userID, experiment.ID).
		Select("group_id").
		First(&assignment).Error
	if err != nil {
		// Not assigned yet => check if user falls into control based on hash
		// Simple deterministic assignment: userID % 100 < trafficRatio * 100
		// For now, if not assigned, proceed as treatment (Agent active)
		return experiment.ID, 0, false
	}

	// Check if the assigned group is "control"
	var group struct {
		GroupName string `gorm:"column:group_name"`
	}
	err = dbConn.WithContext(ctx).
		Table("ab_test_groups").
		Where("id = ? AND experiment_id = ?", assignment.GroupID, experiment.ID).
		Select("group_name").
		First(&group).Error
	if err != nil {
		return experiment.ID, assignment.GroupID, false
	}

	isControl := group.GroupName == "control"
	return experiment.ID, assignment.GroupID, isControl
}

// =====================================================
// Observability: JSON Stats Endpoint (Task 11)
// =====================================================

// StatsJSON returns a JSON-serializable map of all Agent runtime metrics.
// This can be wired to an HTTP handler at /debug/recommendation/stats.
func (a *RecommendationAgent) StatsJSON() map[string]interface{} {
	if a == nil || a.stats == nil {
		return map[string]interface{}{"status": "agent_not_initialized"}
	}
	stats := a.stats.Snapshot()
	stats["agent_enabled"] = a.config.Enabled
	stats["config"] = map[string]interface{}{
		"consecutive_skip_threshold":     a.config.ConsecutiveSkipThreshold,
		"deep_interaction_threshold":     a.config.DeepInteractionThreshold,
		"engagement_threshold":           a.config.EngagementThreshold,
		"cold_start_action_threshold":    a.config.ColdStartActionThreshold,
		"max_non_standard_ratio":         a.config.MaxNonStandardRatio,
		"hot_explore_timeout_ms":         a.config.HotExploreTimeoutMs,
		"topic_deep_dive_min_candidates": a.config.TopicDeepDiveMinCandidates,
	}
	return stats
}
