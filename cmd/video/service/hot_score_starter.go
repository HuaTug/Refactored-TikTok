package service

import (
	"HuaTug.com/cmd/video/dal/db"
	"HuaTug.com/pkg/recommendation"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

// StartHotScoreService 启动热度计算服务
// 在应用初始化时调用
func StartHotScoreService() {
	// 获取数据库连接
	database := db.DB
	if database == nil {
		hlog.Error("[HotScore] Database not initialized, cannot start hot score service")
		return
	}

	// 使用默认配置初始化热度服务
	// 生产环境可以使用 recommendation.ProductionHotScoreConfig()
	config := recommendation.DefaultHotScoreConfig()

	// 初始化服务
	recommendation.InitHotScoreService(database, config)

	// 启动服务
	recommendation.StartHotScoreService()

	hlog.Info("[HotScore] Hot score calculation service started successfully")
}

// StopHotScoreService 停止热度计算服务
// 在应用关闭时调用
func StopHotScoreService() {
	recommendation.StopHotScoreService()
	hlog.Info("[HotScore] Hot score calculation service stopped")
}
