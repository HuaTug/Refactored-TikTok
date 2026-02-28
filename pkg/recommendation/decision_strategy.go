package recommendation

// =====================================================
// Decision Strategy: Rule-Based Implementation
// Implements the DecisionStrategy interface using
// configurable rule chains.
// =====================================================

// RuleBasedDecisionStrategy implements DecisionStrategy using a priority-chain of rules.
// This is the Phase-1 implementation; the interface supports swapping in ML/LLM-based
// strategies without changing the Agent's main flow.
type RuleBasedDecisionStrategy struct {
	config *AgentConfig
}

// NewRuleBasedDecisionStrategy creates a new rule-based decision strategy.
func NewRuleBasedDecisionStrategy(config *AgentConfig) *RuleBasedDecisionStrategy {
	if config == nil {
		config = DefaultAgentConfig()
	}
	return &RuleBasedDecisionStrategy{config: config}
}

// Decide evaluates the user's realtime state and returns the most appropriate strategy.
// Priority chain (first match wins):
//  1. COLD_START  — if the user has very few recent actions
//  2. HOT_EXPLORE — if the user is consecutively skipping and disengaged
//  3. TOPIC_DEEP_DIVE — if the user shows sustained deep interest in a topic
//  4. STANDARD — default fallback
func (r *RuleBasedDecisionStrategy) Decide(state *UserRealtimeState) (RecommendStrategy, map[string]string) {
	if state == nil {
		return StrategyStandard, nil
	}

	// Rule 1: Cold Start — insufficient behavioral data
	if state.RecentActionCount < r.config.ColdStartActionThreshold {
		return StrategyColdStart, nil
	}

	// Rule 2: Hot Explore — user is bored (many consecutive skips + low engagement)
	if state.ConsecutiveSkips >= r.config.ConsecutiveSkipThreshold &&
		state.EngagementLevel < r.config.EngagementThreshold {
		return StrategyHotExplore, nil
	}

	// Rule 3: Topic Deep Dive — user is deeply engaged with a specific topic
	if state.FocusedTopic != "" {
		return StrategyTopicDeepDive, map[string]string{
			"topic": state.FocusedTopic,
		}
	}

	// Rule 4: Default — standard recommendation pipeline
	return StrategyStandard, nil
}
