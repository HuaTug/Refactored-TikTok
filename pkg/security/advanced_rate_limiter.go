package security

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

// AdvancedRateLimiter 高级限流器
type AdvancedRateLimiter struct {
	redis  *redis.Client
	config *RateLimitConfig
	mu     sync.RWMutex
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// 滑动窗口配置
	WindowSize  time.Duration `json:"window_size"`
	MaxRequests int64         `json:"max_requests"`

	// 令牌桶配置
	BucketSize     int64         `json:"bucket_size"`
	RefillRate     int64         `json:"refill_rate"`
	RefillInterval time.Duration `json:"refill_interval"`

	// 分层限流
	UserLimits  map[string]int64 `json:"user_limits"`  // 用户级别限流
	IPLimits    map[string]int64 `json:"ip_limits"`    // IP级别限流
	GlobalLimit int64            `json:"global_limit"` // 全局限流

	// 动态调整
	EnableAdaptive bool    `json:"enable_adaptive"`
	LoadThreshold  float64 `json:"load_threshold"`
	AdjustFactor   float64 `json:"adjust_factor"`
}

// RateLimitResult 限流结果
type RateLimitResult struct {
	Allowed    bool          `json:"allowed"`
	Remaining  int64         `json:"remaining"`
	ResetTime  time.Time     `json:"reset_time"`
	RetryAfter time.Duration `json:"retry_after"`
	LimitType  string        `json:"limit_type"`
}

// NewAdvancedRateLimiter 创建高级限流器
func NewAdvancedRateLimiter(redisClient *redis.Client, config *RateLimitConfig) *AdvancedRateLimiter {
	return &AdvancedRateLimiter{
		redis:  redisClient,
		config: config,
	}
}

// CheckLimit 检查限流
func (arl *AdvancedRateLimiter) CheckLimit(ctx context.Context, key string, limitType string) (*RateLimitResult, error) {
	arl.mu.RLock()
	defer arl.mu.RUnlock()

	// 根据限流类型选择不同的算法
	switch limitType {
	case "sliding_window":
		return arl.slidingWindowLimit(ctx, key)
	case "token_bucket":
		return arl.tokenBucketLimit(ctx, key)
	case "fixed_window":
		return arl.fixedWindowLimit(ctx, key)
	case "adaptive":
		return arl.adaptiveLimit(ctx, key)
	default:
		return arl.slidingWindowLimit(ctx, key)
	}
}

// slidingWindowLimit 滑动窗口限流
func (arl *AdvancedRateLimiter) slidingWindowLimit(ctx context.Context, key string) (*RateLimitResult, error) {
	now := time.Now()
	windowStart := now.Add(-arl.config.WindowSize)

	pipe := arl.redis.TxPipeline()

	// 清理过期记录
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// 添加当前请求
	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(now.UnixNano()),
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// 计算当前窗口内的请求数
	countCmd := pipe.ZCard(ctx, key)

	// 设置过期时间
	pipe.Expire(ctx, key, arl.config.WindowSize+time.Minute)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	count := countCmd.Val()
	remaining := arl.config.MaxRequests - count

	result := &RateLimitResult{
		Allowed:   count <= arl.config.MaxRequests,
		Remaining: remaining,
		ResetTime: now.Add(arl.config.WindowSize),
		LimitType: "sliding_window",
	}

	if !result.Allowed {
		result.RetryAfter = arl.config.WindowSize
	}

	return result, nil
}

// tokenBucketLimit 令牌桶限流
func (arl *AdvancedRateLimiter) tokenBucketLimit(ctx context.Context, key string) (*RateLimitResult, error) {
	now := time.Now()
	bucketKey := fmt.Sprintf("bucket:%s", key)

	// Lua脚本实现原子性令牌桶操作
	luaScript := `
		local bucket_key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local refill_interval = tonumber(ARGV[3])
		local now = tonumber(ARGV[4])
		local requested = tonumber(ARGV[5])
		
		local bucket = redis.call('HMGET', bucket_key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1]) or capacity
		local last_refill = tonumber(bucket[2]) or now
		
		-- 计算需要添加的令牌数
		local time_passed = now - last_refill
		local tokens_to_add = math.floor(time_passed / refill_interval * refill_rate)
		tokens = math.min(capacity, tokens + tokens_to_add)
		
		local allowed = 0
		local remaining = tokens
		
		if tokens >= requested then
			tokens = tokens - requested
			allowed = 1
			remaining = tokens
		end
		
		-- 更新桶状态
		redis.call('HMSET', bucket_key, 'tokens', tokens, 'last_refill', now)
		redis.call('EXPIRE', bucket_key, 3600)
		
		return {allowed, remaining}
	`

	result, err := arl.redis.Eval(ctx, luaScript, []string{bucketKey},
		arl.config.BucketSize,
		arl.config.RefillRate,
		arl.config.RefillInterval.Nanoseconds(),
		now.UnixNano(),
		1, // 请求1个令牌
	).Result()

	if err != nil {
		return nil, err
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	remaining := values[1].(int64)

	rateLimitResult := &RateLimitResult{
		Allowed:   allowed,
		Remaining: remaining,
		ResetTime: now.Add(arl.config.RefillInterval),
		LimitType: "token_bucket",
	}

	if !allowed {
		rateLimitResult.RetryAfter = arl.config.RefillInterval
	}

	return rateLimitResult, nil
}

// fixedWindowLimit 固定窗口限流
func (arl *AdvancedRateLimiter) fixedWindowLimit(ctx context.Context, key string) (*RateLimitResult, error) {
	now := time.Now()
	windowKey := fmt.Sprintf("%s:%d", key, now.Unix()/int64(arl.config.WindowSize.Seconds()))

	pipe := arl.redis.TxPipeline()
	incrCmd := pipe.Incr(ctx, windowKey)
	pipe.Expire(ctx, windowKey, arl.config.WindowSize)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, err
	}

	count := incrCmd.Val()
	remaining := arl.config.MaxRequests - count

	result := &RateLimitResult{
		Allowed:   count <= arl.config.MaxRequests,
		Remaining: remaining,
		ResetTime: time.Unix((now.Unix()/int64(arl.config.WindowSize.Seconds())+1)*int64(arl.config.WindowSize.Seconds()), 0),
		LimitType: "fixed_window",
	}

	if !result.Allowed {
		result.RetryAfter = time.Until(result.ResetTime)
	}

	return result, nil
}

// adaptiveLimit 自适应限流
func (arl *AdvancedRateLimiter) adaptiveLimit(ctx context.Context, key string) (*RateLimitResult, error) {
	// 获取系统负载
	load, err := arl.getSystemLoad(ctx)
	if err != nil {
		// 如果无法获取负载，使用默认限流
		return arl.slidingWindowLimit(ctx, key)
	}

	// 根据负载动态调整限流阈值
	adjustedLimit := arl.config.MaxRequests
	if load > arl.config.LoadThreshold {
		adjustedLimit = int64(float64(arl.config.MaxRequests) * arl.config.AdjustFactor)
	}

	// 临时修改配置
	originalLimit := arl.config.MaxRequests
	arl.config.MaxRequests = adjustedLimit

	result, err := arl.slidingWindowLimit(ctx, key)

	// 恢复原始配置
	arl.config.MaxRequests = originalLimit

	if err != nil {
		return nil, err
	}

	result.LimitType = "adaptive"
	return result, nil
}

// getSystemLoad 获取系统负载
func (arl *AdvancedRateLimiter) getSystemLoad(ctx context.Context) (float64, error) {
	// 从Redis获取系统负载指标
	loadKey := "system:load"
	loadStr, err := arl.redis.Get(ctx, loadKey).Result()
	if err != nil {
		return 0.5, nil // 默认负载
	}

	var load float64
	fmt.Sscanf(loadStr, "%f", &load)
	return load, nil
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	name          string
	maxFailures   int64
	resetTimeout  time.Duration
	state         CircuitState
	failures      int64
	lastFailTime  time.Time
	mu            sync.RWMutex
	onStateChange func(name string, from CircuitState, to CircuitState)
}

// CircuitState 熔断器状态
type CircuitState int

const (
	StateClosed CircuitState = iota
	StateHalfOpen
	StateOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(name string, maxFailures int64, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// Execute 执行操作
func (cb *CircuitBreaker) Execute(operation func() error) error {
	if !cb.allowRequest() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := operation()
	cb.recordResult(err == nil)

	return err
}

// allowRequest 是否允许请求
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		return time.Since(cb.lastFailTime) >= cb.resetTimeout
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// recordResult 记录结果
func (cb *CircuitBreaker) recordResult(success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if success {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

// onSuccess 成功处理
func (cb *CircuitBreaker) onSuccess() {
	cb.failures = 0

	if cb.state == StateHalfOpen {
		cb.setState(StateClosed)
	}
}

// onFailure 失败处理
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailTime = time.Now()

	if cb.state == StateClosed && cb.failures >= cb.maxFailures {
		cb.setState(StateOpen)
	} else if cb.state == StateHalfOpen {
		cb.setState(StateOpen)
	}
}

// setState 设置状态
func (cb *CircuitBreaker) setState(state CircuitState) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}
}

// GetState 获取状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures 获取失败次数
func (cb *CircuitBreaker) GetFailures() int64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}
