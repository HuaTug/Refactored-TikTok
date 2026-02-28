package service

import (
	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/config"
	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	goredisv8 "github.com/go-redis/redis/v8"
)

// StartRecommendationAgent initializes the full RecommendationAgent chain:
//
//	RealtimeStateService → UserProfileService → RecommendationEngine
//	→ IntegratedRecommendationEngine → RecommendationAgent
//
// This must be called AFTER dal.Init() and StartHotScoreService().
func StartRecommendationAgent() {
	database := db.DB
	if database == nil {
		hlog.Error("[RecAgent] Database not initialized, skipping recommendation agent startup")
		return
	}

	// Build a go-redis/v8 client from the shared config.
	// The recommendation package uses go-redis/redis/v8.
	redisClient := goredisv8.NewClient(&goredisv8.Options{
		Addr:     config.ConfigInfo.Redis.Addr,
		Password: config.ConfigInfo.Redis.Password,
		DB:       0,
	})

	// --- 1. RealtimeStateService (perception layer for Agent) ---
	recommendation.InitRealtimeStateService(redisClient, database, nil)
	realtimeStateSvc := recommendation.GetRealtimeStateServiceInstance()
	hlog.Info("[RecAgent] RealtimeStateService initialized")

	// --- 2. UserProfileService (user interest tracking) ---
	userProfileSvc := recommendation.NewUserProfileService(redisClient, database, nil)
	// Wire the realtime state service so every RecordAction also feeds the Agent
	if realtimeStateSvc != nil {
		userProfileSvc.SetRealtimeStateService(realtimeStateSvc)
	}
	hlog.Info("[RecAgent] UserProfileService initialized")

	// --- 3. RecommendationEngine (multi-strategy recall) ---
	recallEngine := recommendation.NewRecommendationEngine(redisClient, database)
	hlog.Info("[RecAgent] RecommendationEngine (recall) initialized with 6 strategies")

	// --- 4. IntegratedRecommendationEngine (recall → coarse rank → CTR fine rank → rerank) ---
	integratedEngine := recommendation.NewIntegratedRecommendationEngine(nil, recallEngine, redisClient, database)
	hlog.Info("[RecAgent] IntegratedRecommendationEngine initialized")

	// --- 5. Existing singletons ---
	ctrClient := recommendation.GetCTRClient()
	hotScoreSvc := recommendation.GetHotScoreService()

	// Wire Redis to HotScoreService for cache sync (hot:video:* and videos:timeline)
	recommendation.SetHotScoreRedis(redisClient)

	// --- 6. Build AgentConfig from config.yml ---
	cfg := config.ConfigInfo.RecAgent
	agentConfig := &recommendation.AgentConfig{
		Enabled:                    cfg.Enabled,
		ConsecutiveSkipThreshold:   cfg.ConsecutiveSkipThreshold,
		DeepInteractionThreshold:   cfg.DeepInteractionThreshold,
		EngagementThreshold:        cfg.EngagementThreshold,
		ColdStartActionThreshold:   cfg.ColdStartActionThreshold,
		MaxNonStandardRatio:        cfg.MaxNonStandardRatio,
		HotExploreTimeoutMs:        cfg.HotExploreTimeoutMs,
		TopicDeepDiveMinCandidates: cfg.TopicDeepDiveMinCandidates,
	}

	// --- 7. Initialize global RecommendationAgent singleton ---
	recommendation.InitRecommendationAgent(
		integratedEngine,
		recallEngine,
		ctrClient,
		hotScoreSvc,
		userProfileSvc,
		realtimeStateSvc,
		agentConfig,
	)

	hlog.Infof("[RecAgent] RecommendationAgent initialized (enabled=%v)", agentConfig.Enabled)
}
