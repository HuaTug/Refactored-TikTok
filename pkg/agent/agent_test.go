package agent

import (
	"context"
	"testing"
	"time"
)

func TestRecommendationAgent(t *testing.T) {
	// 创建Agent配置
	config := DefaultAgentConfig()

	// 创建Agent
	agent := NewRecommendationAgent(config)

	// 创建Mock服务
	videoService := NewMockVideoService()
	userService := NewMockUserService()

	// 设置服务
	agent.actionModule.SetServices(videoService, userService)

	// 测试无聊用户推荐
	t.Run("BoredUserRecommendation", func(t *testing.T) {
		ctx := context.Background()
		userID := int64(1001)

		behaviorSequence := &BehaviorSequence{
			UserID: userID,
			Behaviors: []Behavior{
				{VideoID: 1, BehaviorType: BehaviorView, Duration: 5 * time.Second, CompletionRate: 0.1, InteractionDepth: 0, Timestamp: time.Now().Add(-5 * time.Minute)},
				{VideoID: 2, BehaviorType: BehaviorSkip, Duration: 2 * time.Second, CompletionRate: 0.05, InteractionDepth: 0, Timestamp: time.Now().Add(-4 * time.Minute)},
			},
			StartTime: time.Now().Add(-10 * time.Minute),
			EndTime:   time.Now(),
		}

		userProfile := &UserProfile{
			InterestTags:     []string{"entertainment", "music"},
			ConsumptionLevel: ConsumptionMedium,
			ActiveHours:      []int{20, 21, 22},
			PreferredTopics: map[string]float64{
				"entertainment": 0.6,
				"music":         0.5,
			},
		}

		userContext := &UserContext{
			Timestamp:   time.Now(),
			IsWeekend:   false,
			TimeOfDay:   TimeOfDayEvening,
			Location:    "Beijing",
			DeviceType:  "mobile",
			NetworkType: "4G",
		}

		result, err := agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
		if err != nil {
			t.Fatalf("推荐失败: %v", err)
		}

		// 验证推荐结果
		if len(result.RecommendedVideos) == 0 {
			t.Error("推荐视频数量为0")
		}

		if result.Mode == RecommendationMode(0) {
			t.Error("推荐模式未设置")
		}

		t.Logf("无聊用户推荐成功: %d个视频, 模式: %v", len(result.RecommendedVideos), result.Mode)
	})

	// 测试参与用户推荐
	t.Run("EngagedUserRecommendation", func(t *testing.T) {
		ctx := context.Background()
		userID := int64(1002)

		behaviorSequence := &BehaviorSequence{
			UserID: userID,
			Behaviors: []Behavior{
				{VideoID: 10, BehaviorType: BehaviorView, Duration: 120 * time.Second, CompletionRate: 0.95, InteractionDepth: 3, Timestamp: time.Now().Add(-10 * time.Minute)},
				{VideoID: 10, BehaviorType: BehaviorLike, Duration: 1 * time.Second, CompletionRate: 1.0, InteractionDepth: 4, Timestamp: time.Now().Add(-9 * time.Minute)},
				{VideoID: 11, BehaviorType: BehaviorComment, Duration: 30 * time.Second, CompletionRate: 1.0, InteractionDepth: 5, Timestamp: time.Now().Add(-6 * time.Minute)},
			},
			StartTime: time.Now().Add(-15 * time.Minute),
			EndTime:   time.Now(),
		}

		userProfile := &UserProfile{
			InterestTags:     []string{"technology", "education"},
			ConsumptionLevel: ConsumptionHigh,
			ActiveHours:      []int{19, 20, 21, 22, 23},
			PreferredTopics: map[string]float64{
				"technology": 0.9,
				"education":  0.8,
			},
		}

		// 确保用户状态包含长期画像
		userContextObj := &UserContext{
			Timestamp:   time.Now(),
			IsWeekend:   true,
			TimeOfDay:   TimeOfDayEvening,
			Location:    "Shanghai",
			DeviceType:  "tablet",
			NetworkType: "WiFi",
		}

		result, err := agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContextObj)
		if err != nil {
			t.Fatalf("推荐失败: %v", err)
		}

		// 验证推荐结果
		if len(result.RecommendedVideos) == 0 {
			t.Error("推荐视频数量为0")
		}

		t.Logf("参与用户推荐成功: %d个视频, 模式: %v", len(result.RecommendedVideos), result.Mode)
	})

	// 测试Agent状态
	t.Run("AgentStatus", func(t *testing.T) {
		status := agent.GetAgentStatus()

		if status == nil {
			t.Error("Agent状态为nil")
		}

		if _, exists := status["config"]; !exists {
			t.Error("Agent状态中缺少config")
		}

		t.Logf("Agent状态获取成功")
	})

	// 测试配置更新
	t.Run("ConfigUpdate", func(t *testing.T) {
		newConfig := &AgentConfig{
			BehaviorWindowSeconds: 600,  // 10分钟
			BoredThreshold:        0.20, // 20%
			DeepInterestThreshold: 0.85, // 85%
			DecisionMode:          "ml", // 机器学习模式
			DefaultRecommendCount: 15,
			HotTopicCount:         8,
			DeepDiveCount:         12,
		}

		err := agent.UpdateConfig(newConfig)
		if err != nil {
			t.Fatalf("配置更新失败: %v", err)
		}

		// 验证配置是否更新
		if agent.config.DefaultRecommendCount != 15 {
			t.Error("配置更新失败，推荐数量未更新")
		}

		t.Logf("配置更新成功")
	})
}

func TestMockServices(t *testing.T) {
	// 测试Mock视频服务
	t.Run("MockVideoService", func(t *testing.T) {
		videoService := NewMockVideoService()
		ctx := context.Background()

		// 测试按类别获取视频
		videos, err := videoService.GetVideosByCategory(ctx, "entertainment", 5)
		if err != nil {
			t.Fatalf("获取娱乐类视频失败: %v", err)
		}

		if len(videos) == 0 {
			t.Error("娱乐类视频数量为0")
		}

		// 测试获取热门视频
		trending, err := videoService.GetTrendingVideos(ctx, 10)
		if err != nil {
			t.Fatalf("获取热门视频失败: %v", err)
		}

		if len(trending) == 0 {
			t.Error("热门视频数量为0")
		}

		// 测试个性化推荐
		personalized, err := videoService.GetPersonalizedVideos(ctx, 1001, []string{"music", "sports"}, 8)
		if err != nil {
			t.Fatalf("获取个性化视频失败: %v", err)
		}

		t.Logf("Mock视频服务测试成功: 类别视频=%d, 热门视频=%d, 个性化视频=%d",
			len(videos), len(trending), len(personalized))
	})

	// 测试Mock用户服务
	t.Run("MockUserService", func(t *testing.T) {
		userService := NewMockUserService()
		ctx := context.Background()
		userID := int64(1001)

		// 测试获取用户画像
		profile, err := userService.GetUserProfile(ctx, userID)
		if err != nil {
			t.Fatalf("获取用户画像失败: %v", err)
		}

		if profile == nil {
			t.Error("用户画像为nil")
		}

		// 测试更新用户兴趣
		newInterests := []string{"technology", "education", "science"}
		err = userService.UpdateUserInterests(ctx, userID, newInterests)
		if err != nil {
			t.Fatalf("更新用户兴趣失败: %v", err)
		}

		// 验证更新后的画像
		updatedProfile, err := userService.GetUserProfile(ctx, userID)
		if err != nil {
			t.Fatalf("获取更新后用户画像失败: %v", err)
		}

		if len(updatedProfile.InterestTags) != len(newInterests) {
			t.Error("用户兴趣更新失败")
		}

		// 测试获取交互历史
		interactions, err := userService.GetUserInteractionHistory(ctx, userID, 7)
		if err != nil {
			t.Fatalf("获取用户交互历史失败: %v", err)
		}

		t.Logf("Mock用户服务测试成功: 用户画像获取✓, 兴趣更新✓, 交互历史=%d条",
			len(interactions))
	})
}

func BenchmarkRecommendationAgent(b *testing.B) {
	// 创建Agent
	config := DefaultAgentConfig()
	agent := NewRecommendationAgent(config)

	// 创建Mock服务
	videoService := NewMockVideoService()
	userService := NewMockUserService()
	agent.actionModule.SetServices(videoService, userService)

	// 准备测试数据
	ctx := context.Background()
	userID := int64(1001)

	behaviorSequence := &BehaviorSequence{
		UserID: userID,
		Behaviors: []Behavior{
			{VideoID: 1, BehaviorType: BehaviorView, Duration: 60 * time.Second, CompletionRate: 0.8, InteractionDepth: 3, Timestamp: time.Now().Add(-5 * time.Minute)},
		},
		StartTime: time.Now().Add(-10 * time.Minute),
		EndTime:   time.Now(),
	}

	userProfile := &UserProfile{
		InterestTags:     []string{"entertainment"},
		ConsumptionLevel: ConsumptionMedium,
		ActiveHours:      []int{20, 21, 22},
		PreferredTopics: map[string]float64{
			"entertainment": 0.6,
		},
	}

	userContext := &UserContext{
		Timestamp:   time.Now(),
		IsWeekend:   false,
		TimeOfDay:   TimeOfDayEvening,
		Location:    "Beijing",
		DeviceType:  "mobile",
		NetworkType: "4G",
	}

	// 基准测试
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
		if err != nil {
			b.Fatalf("推荐失败: %v", err)
		}
	}
}
