package recommendation

import (
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// =====================================================
// 热度服务初始化和全局访问
// =====================================================

var (
	hotScoreService   *VideoHotScoreService
	hotScoreScheduler *HotScoreScheduler
	hotScoreOnce      sync.Once
)

// InitHotScoreService 初始化热度计算服务
// 应在应用启动时调用
func InitHotScoreService(db *gorm.DB, config *HotScoreConfig) {
	hotScoreOnce.Do(func() {
		if config == nil {
			config = DefaultHotScoreConfig()
		}

		hotScoreService = NewVideoHotScoreService(config, db)
		hotScoreScheduler = NewHotScoreScheduler(hotScoreService)

		hlog.Info("[HotScore] Hot score service initialized")
	})
}

// StartHotScoreService 启动热度计算服务
func StartHotScoreService() {
	if hotScoreService == nil {
		hlog.Error("[HotScore] Hot score service not initialized")
		return
	}

	// 启动服务
	hotScoreService.Start()

	// 启动调度器
	if hotScoreScheduler != nil {
		hotScoreScheduler.Start()
	}

	hlog.Info("[HotScore] Hot score service started")
}

// StopHotScoreService 停止热度计算服务
func StopHotScoreService() {
	if hotScoreService != nil {
		hotScoreService.Stop()
	}
	if hotScoreScheduler != nil {
		hotScoreScheduler.Stop()
	}
	hlog.Info("[HotScore] Hot score service stopped")
}

// GetHotScoreService 获取热度服务实例
func GetHotScoreService() *VideoHotScoreService {
	return hotScoreService
}

// GetHotScoreScheduler 获取调度器实例
func GetHotScoreScheduler() *HotScoreScheduler {
	return hotScoreScheduler
}

// SetHotScoreRedis sets the Redis client on the hot score service for cache sync.
func SetHotScoreRedis(redisClient *redis.Client) {
	if hotScoreService != nil && redisClient != nil {
		hotScoreService.SetRedisClient(redisClient)
		hlog.Info("[HotScore] Redis client set for hot score cache sync")
	}
}

// =====================================================
// 便捷接口函数
// =====================================================

// QuickHotScoreConfig 快速配置（开发/测试环境）
func QuickHotScoreConfig() *HotScoreConfig {
	config := DefaultHotScoreConfig()
	// 开发环境：更短的更新间隔
	config.UpdateInterval = 1 * time.Minute
	config.CalculateWorker = 2
	return config
}

// ProductionHotScoreConfig 生产环境配置
func ProductionHotScoreConfig() *HotScoreConfig {
	config := DefaultHotScoreConfig()
	// 生产环境：更大的批量处理和更多工作线程
	config.BatchSize = 1000
	config.CalculateWorker = 8
	config.UpdateInterval = 5 * time.Minute
	return config
}

// HighTrafficHotScoreConfig 高流量环境配置
func HighTrafficHotScoreConfig() *HotScoreConfig {
	config := DefaultHotScoreConfig()
	// 高流量：更频繁的更新
	config.BatchSize = 2000
	config.CalculateWorker = 16
	config.UpdateInterval = 2 * time.Minute

	// 调整时间窗口
	config.TimeWindows = []TimeWindow{
		{Name: "10m", Duration: 10 * time.Minute, Weight: 0.5},  // 10分钟窗口
		{Name: "1h", Duration: time.Hour, Weight: 0.3},
		{Name: "6h", Duration: 6 * time.Hour, Weight: 0.15},
		{Name: "24h", Duration: 24 * time.Hour, Weight: 0.05},
	}

	return config
}
