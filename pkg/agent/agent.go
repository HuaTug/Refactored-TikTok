package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// RecommendationAgent 智能推荐代理
type RecommendationAgent struct {
	perceptionModule *PerceptionModule
	decisionModule   *DecisionModule
	actionModule     *ActionModule
	config          *AgentConfig
}

// AgentConfig Agent配置
type AgentConfig struct {
	// 感知配置
	BehaviorWindowSeconds    int     `json:"behavior_window_seconds"`    // 行为窗口时间（秒）
	BoredThreshold          float64 `json:"bored_threshold"`            // 无聊阈值
	DeepInterestThreshold   float64 `json:"deep_interest_threshold"`    // 深度兴趣阈值
	
	// 决策配置
	DecisionMode            string  `json:"decision_mode"`              // 决策模式: rule/ml/rl
	ModelPath               string  `json:"model_path"`                 // 模型路径
	
	// 行动配置
	DefaultRecommendCount   int     `json:"default_recommend_count"`    // 默认推荐数量
	HotTopicCount          int     `json:"hot_topic_count"`            // 热点话题数量
	DeepDiveCount          int     `json:"deep_dive_count"`            // 深度挖掘数量
}

// UserState 用户状态
type UserState struct {
	UserID           int64                  `json:"user_id"`
	EngagementLevel  EngagementLevel       `json:"engagement_level"`  // 参与度
	ExplorationLevel ExplorationLevel      `json:"exploration_level"` // 探索度
	CurrentInterests []string              `json:"current_interests"` // 当前兴趣
	LongTermProfile  *UserProfile          `json:"long_term_profile"` // 长期画像
	Context          *UserContext          `json:"context"`           // 上下文
}

// EngagementLevel 参与度枚举
type EngagementLevel int

const (
	EngagementBored EngagementLevel = iota  // 无聊
	EngagementCasual                        // 随意
	EngagementEngaged                       // 参与
	EngagementImmersed                      // 沉浸
)

// ExplorationLevel 探索度枚举
type ExplorationLevel int

const (
	ExplorationFocused ExplorationLevel = iota  // 聚焦
	ExplorationMixed                            // 混合
	ExplorationDiverse                          // 多样化
)

// UserProfile 用户长期画像
type UserProfile struct {
	InterestTags     []string           `json:"interest_tags"`     // 兴趣标签
	ConsumptionLevel ConsumptionLevel   `json:"consumption_level"` // 消费能力
	ActiveHours      []int              `json:"active_hours"`      // 活跃时段
	PreferredTopics  map[string]float64 `json:"preferred_topics"`  // 偏好主题权重
}

// ConsumptionLevel 消费能力枚举
type ConsumptionLevel int

const (
	ConsumptionLow ConsumptionLevel = iota
	ConsumptionMedium
	ConsumptionHigh
	ConsumptionPremium
)

// UserContext 用户上下文
type UserContext struct {
	Timestamp    time.Time    `json:"timestamp"`     // 当前时间
	IsWeekend    bool         `json:"is_weekend"`    // 是否周末
	TimeOfDay    TimeOfDay    `json:"time_of_day"`   // 时段
	Location     string       `json:"location"`      // 地理位置
	DeviceType   string       `json:"device_type"`   // 设备类型
	NetworkType  string       `json:"network_type"`  // 网络类型
}

// TimeOfDay 时段枚举
type TimeOfDay int

const (
	TimeOfDayMorning TimeOfDay = iota  // 早晨
	TimeOfDayAfternoon                 // 下午
	TimeOfDayEvening                   // 晚上
	TimeOfDayNight                     // 深夜
)

// BehaviorSequence 行为序列
type BehaviorSequence struct {
	UserID     int64      `json:"user_id"`
	Behaviors  []Behavior `json:"behaviors"`
	StartTime  time.Time  `json:"start_time"`
	EndTime    time.Time  `json:"end_time"`
}

// Behavior 单个行为
type Behavior struct {
	VideoID          int64         `json:"video_id"`
	BehaviorType     BehaviorType  `json:"behavior_type"`
	Duration         time.Duration `json:"duration"`         // 行为持续时间
	CompletionRate   float64       `json:"completion_rate"`  // 完播率
	InteractionDepth int           `json:"interaction_depth"` // 交互深度(0-5)
	Timestamp        time.Time     `json:"timestamp"`
}

// BehaviorType 行为类型枚举
type BehaviorType int

const (
	BehaviorView BehaviorType = iota
	BehaviorLike
	BehaviorShare
	BehaviorComment
	BehaviorFollow
	BehaviorSkip
	BehaviorRewatch
	BehaviorProfileView
)

// DecisionResult 决策结果
type DecisionResult struct {
	RecommendationMode RecommendationMode `json:"recommendation_mode"`
	Parameters         map[string]interface{} `json:"parameters"`
	Confidence         float64            `json:"confidence"`
	Reasoning          string             `json:"reasoning"`
}

// RecommendationMode 推荐模式枚举
type RecommendationMode int

const (
	ModeRegular RecommendationMode = iota  // 常规推荐
	ModeHotExplore                         // 热点探索
	ModeDeepDive                           // 主题深挖
	ModeNewContent                         // 新品推荐
	ModePersonalized                       // 个性化推荐
	ModeDiversified                        // 多样化推荐
)

// ActionResult 行动结果
type ActionResult struct {
	RecommendedVideos []RecommendedVideo `json:"recommended_videos"`
	Mode              RecommendationMode `json:"mode"`
	Parameters        map[string]interface{} `json:"parameters"`
	Timestamp         time.Time          `json:"timestamp"`
}

// RecommendedVideo 推荐视频
type RecommendedVideo struct {
	VideoID     int64             `json:"video_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Tags        []string          `json:"tags"`
	Score       float64           `json:"score"`
	Reason      string            `json:"reason"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewRecommendationAgent 创建新的推荐Agent
func NewRecommendationAgent(config *AgentConfig) *RecommendationAgent {
	return &RecommendationAgent{
		perceptionModule: NewPerceptionModule(config),
		decisionModule:   NewDecisionModule(config),
		actionModule:     NewActionModule(config),
		config:          config,
	}
}

// ProcessRecommendationRequest 处理推荐请求
func (ra *RecommendationAgent) ProcessRecommendationRequest(
	ctx context.Context,
	userID int64,
	behaviorSequence *BehaviorSequence,
	userProfile *UserProfile,
	context *UserContext,
) (*ActionResult, error) {
	
	hlog.Infof("Processing recommendation request for user %d", userID)
	
	// 1. 感知阶段 - 分析用户状态
	userState, err := ra.perceptionModule.AnalyzeUserState(
		ctx, userID, behaviorSequence, userProfile, context,
	)
	if err != nil {
		return nil, fmt.Errorf("perception failed: %w", err)
	}
	
	hlog.Infof("User state analyzed: engagement=%v, exploration=%v", 
		userState.EngagementLevel, userState.ExplorationLevel)
	
	// 2. 决策阶段 - 选择推荐策略
	decision, err := ra.decisionModule.MakeDecision(ctx, userState)
	if err != nil {
		return nil, fmt.Errorf("decision making failed: %w", err)
	}
	
	hlog.Infof("Decision made: mode=%v, confidence=%.2f", 
		decision.RecommendationMode, decision.Confidence)
	
	// 3. 行动阶段 - 执行推荐
	result, err := ra.actionModule.ExecuteRecommendation(ctx, userState, decision)
	if err != nil {
		return nil, fmt.Errorf("action execution failed: %w", err)
	}
	
	hlog.Infof("Recommendation executed: %d videos recommended", 
		len(result.RecommendedVideos))
	
	return result, nil
}

// GetAgentStatus 获取Agent状态
func (ra *RecommendationAgent) GetAgentStatus() map[string]interface{} {
	return map[string]interface{}{
		"config": ra.config,
		"perception_stats": ra.perceptionModule.GetStats(),
		"decision_stats":   ra.decisionModule.GetStats(),
		"action_stats":     ra.actionModule.GetStats(),
	}
}

// UpdateConfig 更新Agent配置
func (ra *RecommendationAgent) UpdateConfig(config *AgentConfig) error {
	ra.config = config
	
	// 更新各模块配置
	if err := ra.perceptionModule.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update perception module: %w", err)
	}
	
	if err := ra.decisionModule.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update decision module: %w", err)
	}
	
	if err := ra.actionModule.UpdateConfig(config); err != nil {
		return fmt.Errorf("failed to update action module: %w", err)
	}
	
	return nil
}

// DefaultAgentConfig 默认Agent配置
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		BehaviorWindowSeconds:   300,  // 5分钟行为窗口
		BoredThreshold:         0.15, // 15%完播率以下认为无聊
		DeepInterestThreshold:  0.80, // 80%完播率以上认为深度兴趣
		DecisionMode:           "rule", // 默认使用规则模式
		DefaultRecommendCount:  10,
		HotTopicCount:         5,
		DeepDiveCount:         8,
	}
}