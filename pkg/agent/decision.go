package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// DecisionModule 决策模块
type DecisionModule struct {
	config         *AgentConfig
	ruleEngine     *RuleEngine
	mlModel        *MLModel
	rlAgent        *RLAgent
	stats          *DecisionStats
}

// DecisionStats 决策模块统计信息
type DecisionStats struct {
	TotalDecisions      int64                          `json:"total_decisions"`
	DecisionsByMode     map[RecommendationMode]int64   `json:"decisions_by_mode"`
	AverageConfidence   float64                        `json:"average_confidence"`
	LastDecisionTime    time.Time                      `json:"last_decision_time"`
}

// RuleEngine 规则引擎
type RuleEngine struct {
	rules []Rule
}

// Rule 规则定义
type Rule struct {
	Name        string                                    `json:"name"`
	Condition   func(*UserState) bool                     `json:"-"`
	Action      func(*UserState) *DecisionResult          `json:"-"`
	Priority    int                                       `json:"priority"`
	Description string                                    `json:"description"`
}

// MLModel 机器学习模型（简化实现）
type MLModel struct {
	modelPath   string
	isLoaded    bool
	features    []string
	weights     map[string]float64
}

// RLAgent 强化学习代理（简化实现）
type RLAgent struct {
	qTable      map[string]map[RecommendationMode]float64
	epsilon     float64  // 探索率
	alpha       float64  // 学习率
	gamma       float64  // 折扣因子
	lastState   string
	lastAction  RecommendationMode
}

// NewDecisionModule 创建决策模块
func NewDecisionModule(config *AgentConfig) *DecisionModule {
	dm := &DecisionModule{
		config:      config,
		ruleEngine:  NewRuleEngine(),
		mlModel:     NewMLModel(config.ModelPath),
		rlAgent:     NewRLAgent(),
		stats: &DecisionStats{
			DecisionsByMode: make(map[RecommendationMode]int64),
		},
	}
	
	return dm
}

// MakeDecision 做出决策
func (dm *DecisionModule) MakeDecision(ctx context.Context, userState *UserState) (*DecisionResult, error) {
	dm.stats.TotalDecisions++
	dm.stats.LastDecisionTime = time.Now()
	
	var decision *DecisionResult
	var err error
	
	switch dm.config.DecisionMode {
	case "rule":
		decision, err = dm.makeRuleBasedDecision(userState)
	case "ml":
		decision, err = dm.makeMLBasedDecision(userState)
	case "rl":
		decision, err = dm.makeRLBasedDecision(userState)
	default:
		decision, err = dm.makeRuleBasedDecision(userState)
	}
	
	if err != nil {
		return nil, err
	}
	
	// 更新统计信息
	dm.stats.DecisionsByMode[decision.RecommendationMode]++
	
	// 更新平均置信度
	totalConfidence := dm.stats.AverageConfidence * float64(dm.stats.TotalDecisions-1)
	dm.stats.AverageConfidence = (totalConfidence + decision.Confidence) / float64(dm.stats.TotalDecisions)
	
	hlog.Infof("Decision made: mode=%v, confidence=%.2f, reasoning=%s",
		decision.RecommendationMode, decision.Confidence, decision.Reasoning)
	
	return decision, nil
}

// makeRuleBasedDecision 基于规则的决策
func (dm *DecisionModule) makeRuleBasedDecision(userState *UserState) (*DecisionResult, error) {
	return dm.ruleEngine.Evaluate(userState), nil
}

// makeMLBasedDecision 基于机器学习的决策
func (dm *DecisionModule) makeMLBasedDecision(userState *UserState) (*DecisionResult, error) {
	if !dm.mlModel.isLoaded {
		// 如果模型未加载，回退到规则决策
		hlog.Warn("ML model not loaded, falling back to rule-based decision")
		return dm.makeRuleBasedDecision(userState)
	}
	
	return dm.mlModel.Predict(userState), nil
}

// makeRLBasedDecision 基于强化学习的决策
func (dm *DecisionModule) makeRLBasedDecision(userState *UserState) (*DecisionResult, error) {
	return dm.rlAgent.SelectAction(userState), nil
}

// NewRuleEngine 创建规则引擎
func NewRuleEngine() *RuleEngine {
	re := &RuleEngine{}
	re.initializeRules()
	return re
}

// initializeRules 初始化规则
func (re *RuleEngine) initializeRules() {
	re.rules = []Rule{
		{
			Name:     "BoredUserRule",
			Priority: 1,
			Description: "用户无聊时触发热点探索",
			Condition: func(state *UserState) bool {
				return state.EngagementLevel == EngagementBored
			},
			Action: func(state *UserState) *DecisionResult {
				return &DecisionResult{
					RecommendationMode: ModeHotExplore,
					Parameters: map[string]interface{}{
						"hot_topic_count": 5,
						"diversity_boost": true,
					},
					Confidence: 0.85,
					Reasoning:  "用户连续快速划过多个视频，判定为无聊状态，触发热点探索模式",
				}
			},
		},
		{
			Name:     "DeepInterestRule",
			Priority: 2,
			Description: "用户深度兴趣时触发主题深挖",
			Condition: func(state *UserState) bool {
				return state.EngagementLevel == EngagementImmersed && 
					   state.ExplorationLevel == ExplorationFocused &&
					   len(state.CurrentInterests) > 0
			},
			Action: func(state *UserState) *DecisionResult {
				return &DecisionResult{
					RecommendationMode: ModeDeepDive,
					Parameters: map[string]interface{}{
						"target_topics": state.CurrentInterests,
						"deep_dive_count": 8,
						"similarity_threshold": 0.7,
					},
					Confidence: 0.90,
					Reasoning:  "用户对特定主题表现出深度兴趣，触发主题深挖模式",
				}
			},
		},
		{
			Name:     "DiverseExplorationRule",
			Priority: 3,
			Description: "用户多样化探索时提供个性化推荐",
			Condition: func(state *UserState) bool {
				return state.ExplorationLevel == ExplorationDiverse &&
					   state.EngagementLevel >= EngagementCasual
			},
			Action: func(state *UserState) *DecisionResult {
				return &DecisionResult{
					RecommendationMode: ModePersonalized,
					Parameters: map[string]interface{}{
						"personalization_weight": 0.7,
						"diversity_weight": 0.3,
						"explore_new_topics": true,
					},
					Confidence: 0.75,
					Reasoning:  "用户表现出多样化探索行为，提供个性化推荐",
				}
			},
		},
		{
			Name:     "NewUserRule",
			Priority: 4,
			Description: "新用户或兴趣不明确时提供多样化内容",
			Condition: func(state *UserState) bool {
				return len(state.CurrentInterests) == 0 ||
					   (state.LongTermProfile != nil && len(state.LongTermProfile.InterestTags) == 0)
			},
			Action: func(state *UserState) *DecisionResult {
				return &DecisionResult{
					RecommendationMode: ModeDiversified,
					Parameters: map[string]interface{}{
						"category_distribution": map[string]float64{
							"entertainment": 0.3,
							"education":     0.2,
							"lifestyle":     0.2,
							"technology":    0.15,
							"sports":        0.15,
						},
					},
					Confidence: 0.60,
					Reasoning:  "用户兴趣不明确，提供多样化内容进行兴趣探索",
				}
			},
		},
		{
			Name:     "DefaultRule",
			Priority: 999,
			Description: "默认规则，提供常规推荐",
			Condition: func(state *UserState) bool {
				return true // 总是匹配
			},
			Action: func(state *UserState) *DecisionResult {
				return &DecisionResult{
					RecommendationMode: ModeRegular,
					Parameters: map[string]interface{}{
						"recommend_count": 10,
					},
					Confidence: 0.70,
					Reasoning:  "使用常规推荐策略",
				}
			},
		},
	}
}

// Evaluate 评估规则
func (re *RuleEngine) Evaluate(userState *UserState) *DecisionResult {
	// 按优先级排序规则
	for _, rule := range re.rules {
		if rule.Condition(userState) {
			hlog.Infof("Rule matched: %s", rule.Name)
			return rule.Action(userState)
		}
	}
	
	// 不应该到达这里，因为有默认规则
	return &DecisionResult{
		RecommendationMode: ModeRegular,
		Parameters:         map[string]interface{}{},
		Confidence:         0.5,
		Reasoning:          "No rule matched, using fallback",
	}
}

// NewMLModel 创建机器学习模型
func NewMLModel(modelPath string) *MLModel {
	ml := &MLModel{
		modelPath: modelPath,
		isLoaded:  false,
		features: []string{
			"engagement_level",
			"exploration_level",
			"interest_count",
			"time_of_day",
			"is_weekend",
			"device_type",
		},
		weights: make(map[string]float64),
	}
	
	// 简化实现：使用随机权重
	ml.initializeWeights()
	ml.isLoaded = true
	
	return ml
}

// initializeWeights 初始化权重
func (ml *MLModel) initializeWeights() {
	rand.Seed(time.Now().UnixNano())
	for _, feature := range ml.features {
		ml.weights[feature] = rand.Float64()*2 - 1 // -1 到 1 之间的随机数
	}
}

// Predict 预测推荐模式
func (ml *MLModel) Predict(userState *UserState) *DecisionResult {
	// 特征提取
	features := ml.extractFeatures(userState)
	
	// 计算各模式的分数
	scores := make(map[RecommendationMode]float64)
	
	for mode := ModeRegular; mode <= ModeDiversified; mode++ {
		scores[mode] = ml.calculateScore(features, mode)
	}
	
	// 选择最高分数的模式
	bestMode := ModeRegular
	bestScore := scores[ModeRegular]
	
	for mode, score := range scores {
		if score > bestScore {
			bestMode = mode
			bestScore = score
		}
	}
	
	// 归一化置信度
	confidence := math.Tanh(bestScore) * 0.5 + 0.5
	
	return &DecisionResult{
		RecommendationMode: bestMode,
		Parameters:         ml.generateParameters(bestMode, userState),
		Confidence:         confidence,
		Reasoning:          fmt.Sprintf("ML model prediction with score %.2f", bestScore),
	}
}

// extractFeatures 提取特征
func (ml *MLModel) extractFeatures(userState *UserState) map[string]float64 {
	features := make(map[string]float64)
	
	features["engagement_level"] = float64(userState.EngagementLevel)
	features["exploration_level"] = float64(userState.ExplorationLevel)
	features["interest_count"] = float64(len(userState.CurrentInterests))
	
	if userState.Context != nil {
		features["time_of_day"] = float64(userState.Context.TimeOfDay)
		if userState.Context.IsWeekend {
			features["is_weekend"] = 1.0
		} else {
			features["is_weekend"] = 0.0
		}
		
		// 简化设备类型编码
		switch userState.Context.DeviceType {
		case "mobile":
			features["device_type"] = 1.0
		case "tablet":
			features["device_type"] = 2.0
		case "desktop":
			features["device_type"] = 3.0
		default:
			features["device_type"] = 0.0
		}
	}
	
	return features
}

// calculateScore 计算分数
func (ml *MLModel) calculateScore(features map[string]float64, mode RecommendationMode) float64 {
	score := 0.0
	
	// 简化的线性模型
	for feature, value := range features {
		if weight, exists := ml.weights[feature]; exists {
			// 为不同模式添加偏置
			bias := float64(mode) * 0.1
			score += (weight + bias) * value
		}
	}
	
	return score
}

// generateParameters 生成参数
func (ml *MLModel) generateParameters(mode RecommendationMode, userState *UserState) map[string]interface{} {
	params := make(map[string]interface{})
	
	switch mode {
	case ModeHotExplore:
		params["hot_topic_count"] = 5
		params["diversity_boost"] = true
	case ModeDeepDive:
		params["target_topics"] = userState.CurrentInterests
		params["deep_dive_count"] = 8
	case ModePersonalized:
		params["personalization_weight"] = 0.8
	case ModeDiversified:
		params["diversity_weight"] = 0.9
	default:
		params["recommend_count"] = 10
	}
	
	return params
}

// NewRLAgent 创建强化学习代理
func NewRLAgent() *RLAgent {
	return &RLAgent{
		qTable:  make(map[string]map[RecommendationMode]float64),
		epsilon: 0.1,  // 10% 探索率
		alpha:   0.1,  // 学习率
		gamma:   0.9,  // 折扣因子
	}
}

// SelectAction 选择动作
func (rl *RLAgent) SelectAction(userState *UserState) *DecisionResult {
	stateKey := rl.encodeState(userState)
	
	// 初始化Q值
	if _, exists := rl.qTable[stateKey]; !exists {
		rl.qTable[stateKey] = make(map[RecommendationMode]float64)
		for mode := ModeRegular; mode <= ModeDiversified; mode++ {
			rl.qTable[stateKey][mode] = 0.0
		}
	}
	
	var selectedMode RecommendationMode
	
	// ε-贪心策略
	if rand.Float64() < rl.epsilon {
		// 探索：随机选择
		selectedMode = RecommendationMode(rand.Intn(int(ModeDiversified) + 1))
	} else {
		// 利用：选择Q值最高的动作
		bestMode := ModeRegular
		bestValue := rl.qTable[stateKey][ModeRegular]
		
		for mode, value := range rl.qTable[stateKey] {
			if value > bestValue {
				bestMode = mode
				bestValue = value
			}
		}
		selectedMode = bestMode
	}
	
	// 记录状态和动作
	rl.lastState = stateKey
	rl.lastAction = selectedMode
	
	confidence := 0.5 + math.Abs(rl.qTable[stateKey][selectedMode])*0.1
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return &DecisionResult{
		RecommendationMode: selectedMode,
		Parameters:         rl.generateParameters(selectedMode, userState),
		Confidence:         confidence,
		Reasoning:          fmt.Sprintf("RL agent selection with Q-value %.2f", rl.qTable[stateKey][selectedMode]),
	}
}

// encodeState 编码状态
func (rl *RLAgent) encodeState(userState *UserState) string {
	return fmt.Sprintf("eng_%d_exp_%d_int_%d",
		int(userState.EngagementLevel),
		int(userState.ExplorationLevel),
		len(userState.CurrentInterests))
}

// generateParameters 生成参数
func (rl *RLAgent) generateParameters(mode RecommendationMode, userState *UserState) map[string]interface{} {
	params := make(map[string]interface{})
	
	switch mode {
	case ModeHotExplore:
		params["hot_topic_count"] = 5
	case ModeDeepDive:
		params["target_topics"] = userState.CurrentInterests
		params["deep_dive_count"] = 8
	case ModePersonalized:
		params["personalization_weight"] = 0.7
	case ModeDiversified:
		params["diversity_weight"] = 0.8
	default:
		params["recommend_count"] = 10
	}
	
	return params
}

// UpdateQValue 更新Q值（用于强化学习训练）
func (rl *RLAgent) UpdateQValue(reward float64, nextState *UserState) {
	if rl.lastState == "" {
		return
	}
	
	nextStateKey := rl.encodeState(nextState)
	
	// 初始化下一状态的Q值
	if _, exists := rl.qTable[nextStateKey]; !exists {
		rl.qTable[nextStateKey] = make(map[RecommendationMode]float64)
		for mode := ModeRegular; mode <= ModeDiversified; mode++ {
			rl.qTable[nextStateKey][mode] = 0.0
		}
	}
	
	// 找到下一状态的最大Q值
	maxNextQ := rl.qTable[nextStateKey][ModeRegular]
	for _, value := range rl.qTable[nextStateKey] {
		if value > maxNextQ {
			maxNextQ = value
		}
	}
	
	// 更新Q值
	currentQ := rl.qTable[rl.lastState][rl.lastAction]
	newQ := currentQ + rl.alpha*(reward+rl.gamma*maxNextQ-currentQ)
	rl.qTable[rl.lastState][rl.lastAction] = newQ
	
	hlog.Infof("RL Q-value updated: state=%s, action=%v, reward=%.2f, newQ=%.2f",
		rl.lastState, rl.lastAction, reward, newQ)
}

// GetStats 获取决策模块统计信息
func (dm *DecisionModule) GetStats() *DecisionStats {
	return dm.stats
}

// UpdateConfig 更新配置
func (dm *DecisionModule) UpdateConfig(config *AgentConfig) error {
	dm.config = config
	
	// 如果模型路径改变，重新加载模型
	if dm.mlModel.modelPath != config.ModelPath {
		dm.mlModel = NewMLModel(config.ModelPath)
	}
	
	return nil
}