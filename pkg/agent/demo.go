package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"
)

// AgentDemo 智能推荐Agent演示
type AgentDemo struct {
	agent        *RecommendationAgent
	videoService *MockVideoService
	userService  *MockUserService
}

// NewAgentDemo 创建演示实例
func NewAgentDemo() *AgentDemo {
	// 创建配置
	config := DefaultAgentConfig()

	// 创建Agent
	agent := NewRecommendationAgent(config)

	// 创建Mock服务
	videoService := NewMockVideoService()
	userService := NewMockUserService()

	// 设置服务
	agent.actionModule.SetServices(videoService, userService)

	return &AgentDemo{
		agent:        agent,
		videoService: videoService,
		userService:  userService,
	}
}

// RunDemo 运行演示
func (ad *AgentDemo) RunDemo() {
	fmt.Println("=== 智能推荐Agent系统演示 ===")
	fmt.Println()

	// 演示不同用户场景
	ad.demonstrateBoredUser()
	ad.demonstrateEngagedUser()
	ad.demonstrateExploringUser()

	// 显示Agent状态
	ad.showAgentStatus()
}

// demonstrateBoredUser 演示无聊用户场景
func (ad *AgentDemo) demonstrateBoredUser() {
	fmt.Println("📱 场景1: 无聊用户推荐")
	fmt.Println("用户行为: 快速滑动，低完播率，无互动")

	ctx := context.Background()
	userID := int64(1001)

	// 创建无聊用户的行为序列
	behaviorSequence := &BehaviorSequence{
		UserID: userID,
		Behaviors: []Behavior{
			{VideoID: 1, BehaviorType: BehaviorView, Duration: 5 * time.Second, CompletionRate: 0.1, InteractionDepth: 0, Timestamp: time.Now().Add(-5 * time.Minute)},
			{VideoID: 2, BehaviorType: BehaviorSkip, Duration: 2 * time.Second, CompletionRate: 0.05, InteractionDepth: 0, Timestamp: time.Now().Add(-4 * time.Minute)},
			{VideoID: 3, BehaviorType: BehaviorView, Duration: 8 * time.Second, CompletionRate: 0.15, InteractionDepth: 0, Timestamp: time.Now().Add(-3 * time.Minute)},
		},
		StartTime: time.Now().Add(-10 * time.Minute),
		EndTime:   time.Now(),
	}

	// 用户画像
	userProfile := &UserProfile{
		InterestTags:     []string{"entertainment", "music"},
		ConsumptionLevel: ConsumptionMedium,
		ActiveHours:      []int{20, 21, 22},
		PreferredTopics: map[string]float64{
			"entertainment": 0.6,
			"music":         0.5,
		},
	}

	// 用户上下文
	userContext := &UserContext{
		Timestamp:   time.Now(),
		IsWeekend:   false,
		TimeOfDay:   TimeOfDayEvening,
		Location:    "Beijing",
		DeviceType:  "mobile",
		NetworkType: "4G",
	}

	// 处理推荐请求
	result, err := ad.agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
	if err != nil {
		log.Printf("推荐失败: %v", err)
		return
	}

	ad.printRecommendationResult("无聊用户", result)
	fmt.Println()
}

// demonstrateEngagedUser 演示参与用户场景
func (ad *AgentDemo) demonstrateEngagedUser() {
	fmt.Println("🎯 场景2: 深度参与用户推荐")
	fmt.Println("用户行为: 高完播率，频繁点赞，查看作者主页")

	ctx := context.Background()
	userID := int64(1002)

	// 创建参与用户的行为序列
	behaviorSequence := &BehaviorSequence{
		UserID: userID,
		Behaviors: []Behavior{
			{VideoID: 10, BehaviorType: BehaviorView, Duration: 120 * time.Second, CompletionRate: 0.95, InteractionDepth: 3, Timestamp: time.Now().Add(-10 * time.Minute)},
			{VideoID: 10, BehaviorType: BehaviorLike, Duration: 1 * time.Second, CompletionRate: 1.0, InteractionDepth: 4, Timestamp: time.Now().Add(-9 * time.Minute)},
			{VideoID: 11, BehaviorType: BehaviorView, Duration: 180 * time.Second, CompletionRate: 0.90, InteractionDepth: 4, Timestamp: time.Now().Add(-7 * time.Minute)},
			{VideoID: 11, BehaviorType: BehaviorComment, Duration: 30 * time.Second, CompletionRate: 1.0, InteractionDepth: 5, Timestamp: time.Now().Add(-6 * time.Minute)},
			{VideoID: 12, BehaviorType: BehaviorProfileView, Duration: 45 * time.Second, CompletionRate: 1.0, InteractionDepth: 5, Timestamp: time.Now().Add(-3 * time.Minute)},
		},
		StartTime: time.Now().Add(-15 * time.Minute),
		EndTime:   time.Now(),
	}

	// 用户画像
	userProfile := &UserProfile{
		InterestTags:     []string{"technology", "education"},
		ConsumptionLevel: ConsumptionHigh,
		ActiveHours:      []int{19, 20, 21, 22, 23},
		PreferredTopics: map[string]float64{
			"technology": 0.9,
			"education":  0.8,
			"science":    0.7,
		},
	}

	// 用户上下文
	userContext := &UserContext{
		Timestamp:   time.Now(),
		IsWeekend:   true,
		TimeOfDay:   TimeOfDayEvening,
		Location:    "Shanghai",
		DeviceType:  "tablet",
		NetworkType: "WiFi",
	}

	// 处理推荐请求
	result, err := ad.agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
	if err != nil {
		log.Printf("推荐失败: %v", err)
		return
	}

	ad.printRecommendationResult("深度参与用户", result)
	fmt.Println()
}

// demonstrateExploringUser 演示探索用户场景
func (ad *AgentDemo) demonstrateExploringUser() {
	fmt.Println("🔍 场景3: 探索多样化用户推荐")
	fmt.Println("用户行为: 跨类别浏览，中等参与度，寻求新内容")

	ctx := context.Background()
	userID := int64(1003)

	// 创建探索用户的行为序列
	behaviorSequence := &BehaviorSequence{
		UserID: userID,
		Behaviors: []Behavior{
			{VideoID: 20, BehaviorType: BehaviorView, Duration: 60 * time.Second, CompletionRate: 0.5, InteractionDepth: 2, Timestamp: time.Now().Add(-12 * time.Minute)},
			{VideoID: 21, BehaviorType: BehaviorView, Duration: 45 * time.Second, CompletionRate: 0.4, InteractionDepth: 1, Timestamp: time.Now().Add(-10 * time.Minute)},
			{VideoID: 22, BehaviorType: BehaviorLike, Duration: 1 * time.Second, CompletionRate: 1.0, InteractionDepth: 3, Timestamp: time.Now().Add(-8 * time.Minute)},
			{VideoID: 23, BehaviorType: BehaviorView, Duration: 90 * time.Second, CompletionRate: 0.7, InteractionDepth: 2, Timestamp: time.Now().Add(-5 * time.Minute)},
			{VideoID: 24, BehaviorType: BehaviorShare, Duration: 10 * time.Second, CompletionRate: 1.0, InteractionDepth: 4, Timestamp: time.Now().Add(-2 * time.Minute)},
		},
		StartTime: time.Now().Add(-15 * time.Minute),
		EndTime:   time.Now(),
	}

	// 用户画像
	userProfile := &UserProfile{
		InterestTags:     []string{"travel", "food", "music", "sports"},
		ConsumptionLevel: ConsumptionMedium,
		ActiveHours:      []int{12, 13, 19, 20, 21},
		PreferredTopics: map[string]float64{
			"travel": 0.6,
			"food":   0.5,
			"music":  0.5,
			"sports": 0.4,
		},
	}

	// 用户上下文
	userContext := &UserContext{
		Timestamp:   time.Now(),
		IsWeekend:   true,
		TimeOfDay:   TimeOfDayAfternoon,
		Location:    "Guangzhou",
		DeviceType:  "mobile",
		NetworkType: "5G",
	}

	// 处理推荐请求
	result, err := ad.agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
	if err != nil {
		log.Printf("推荐失败: %v", err)
		return
	}

	ad.printRecommendationResult("探索多样化用户", result)
	fmt.Println()
}

// printRecommendationResult 打印推荐结果
func (ad *AgentDemo) printRecommendationResult(userType string, result *ActionResult) {
	fmt.Printf("📊 %s推荐结果:\n", userType)
	fmt.Printf("   推荐模式: %s\n", ad.getModeName(result.Mode))
	fmt.Printf("   推荐数量: %d\n", len(result.RecommendedVideos))
	fmt.Printf("   生成时间: %s\n", result.Timestamp.Format("15:04:05"))

	fmt.Println("   推荐视频列表:")
	for i, video := range result.RecommendedVideos {
		if i >= 5 { // 只显示前5个
			fmt.Printf("   ... 还有 %d 个视频\n", len(result.RecommendedVideos)-5)
			break
		}
		fmt.Printf("   %d. %s (类别:%s, 评分:%.2f)\n",
			i+1, video.Title, video.Category, video.Score)
		fmt.Printf("      推荐理由: %s\n", video.Reason)
	}
}

// getModeName 获取模式名称
func (ad *AgentDemo) getModeName(mode RecommendationMode) string {
	switch mode {
	case ModeRegular:
		return "常规推荐"
	case ModeHotExplore:
		return "热点探索"
	case ModeDeepDive:
		return "深度挖掘"
	case ModeNewContent:
		return "新内容推荐"
	case ModePersonalized:
		return "个性化推荐"
	case ModeDiversified:
		return "多样化推荐"
	default:
		return "未知模式"
	}
}

// showAgentStatus 显示Agent状态
func (ad *AgentDemo) showAgentStatus() {
	fmt.Println("🤖 Agent系统状态:")

	status := ad.agent.GetAgentStatus()

	// 格式化输出
	if statusJSON, err := json.MarshalIndent(status, "", "  "); err == nil {
		fmt.Println(string(statusJSON))
	}
}

// RunContinuousDemo 运行连续演示（模拟真实场景）
func (ad *AgentDemo) RunContinuousDemo(userID int64, sessionDuration time.Duration) {
	fmt.Printf("🔄 开始连续推荐演示 (用户ID: %d, 持续时间: %v)\n", userID, sessionDuration)

	ctx := context.Background()
	startTime := time.Now()

	// 模拟用户Session
	for time.Since(startTime) < sessionDuration {
		// 生成当前行为序列
		behaviorSequence := ad.generateRandomBehaviorSequence(userID)

		// 获取用户画像
		userProfile, _ := ad.userService.GetUserProfile(ctx, userID)

		// 当前上下文
		userContext := &UserContext{
			Timestamp:   time.Now(),
			IsWeekend:   time.Now().Weekday() == time.Saturday || time.Now().Weekday() == time.Sunday,
			TimeOfDay:   ad.getCurrentTimeOfDay(),
			Location:    "Beijing",
			DeviceType:  "mobile",
			NetworkType: "5G",
		}

		// 获取推荐
		result, err := ad.agent.ProcessRecommendationRequest(ctx, userID, behaviorSequence, userProfile, userContext)
		if err != nil {
			log.Printf("推荐失败: %v", err)
			continue
		}

		fmt.Printf("⏰ %s - 推荐了 %d 个视频 (模式: %s)\n",
			time.Now().Format("15:04:05"),
			len(result.RecommendedVideos),
			ad.getModeName(result.Mode))

		// 等待下次推荐
		time.Sleep(10 * time.Second)
	}

	fmt.Println("✅ 连续演示结束")
}

// generateRandomBehaviorSequence 生成随机行为序列
func (ad *AgentDemo) generateRandomBehaviorSequence(userID int64) *BehaviorSequence {
	behaviors := make([]Behavior, 0, 5)

	for i := 0; i < 3+rand.Intn(3); i++ { // 3-5个行为
		behavior := Behavior{
			VideoID:          int64(rand.Intn(100) + 1),
			BehaviorType:     BehaviorType(rand.Intn(8)),
			Duration:         time.Duration(rand.Intn(180)+10) * time.Second,
			CompletionRate:   rand.Float64(),
			InteractionDepth: rand.Intn(6),
			Timestamp:        time.Now().Add(time.Duration(-i*2) * time.Minute),
		}
		behaviors = append(behaviors, behavior)
	}

	return &BehaviorSequence{
		UserID:    userID,
		Behaviors: behaviors,
		StartTime: time.Now().Add(-10 * time.Minute),
		EndTime:   time.Now(),
	}
}

// getCurrentTimeOfDay 获取当前时段
func (ad *AgentDemo) getCurrentTimeOfDay() TimeOfDay {
	hour := time.Now().Hour()
	switch {
	case hour >= 6 && hour < 12:
		return TimeOfDayMorning
	case hour >= 12 && hour < 18:
		return TimeOfDayAfternoon
	case hour >= 18 && hour < 24:
		return TimeOfDayEvening
	default:
		return TimeOfDayNight
	}
}
