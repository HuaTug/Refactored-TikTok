package main

import (
	"context"
	"fmt"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/cmd/video/infras/redis"
	jwt "HuaTug.com/pkg"
	"HuaTug.com/pkg/errno"
	"HuaTug.com/pkg/oss"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/cors"
)

func Init() {
	rpc.InitRPC()
	redis.Load()

	// 初始化MinIO客户端
	if err := oss.InitMinio(); err != nil {
		hlog.Fatalf("Failed to initialize MinIO: %v", err)
	}
	hlog.Info("MinIO initialized successfully")

	// 初始化TikTok存储架构
	tikTokStorage := oss.NewTikTokStorage()
	if err := tikTokStorage.InitializeBuckets(context.Background()); err != nil {
		hlog.Fatalf("Failed to initialize TikTok storage buckets: %v", err)
	}
	hlog.Info("TikTok storage architecture initialized successfully")

	// 启动热度存储管理器（可选，用于生产环境）
	// go func() {
	// 	hotStorageManager := oss.NewHotStorageManager()
	// 	ctx := context.Background()
	//
	// 	// 启动热度管理
	// 	go hotStorageManager.StartHotStorageManager(ctx, nil)
	//
	// 	// 启动清理工作者
	// 	go hotStorageManager.StartCleanupWorker(ctx)
	//
	// 	hlog.Info("Hot storage manager started")
	// }()
}

func main() {
	Init()
	//pprof.Load()
	r := server.New(
		server.WithHostPorts("0.0.0.0:8888"),
		server.WithHandleMethodNotAllowed(true),
		server.WithMaxRequestBodySize(16*1024*1024*1024),
	)

	// 配置 CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:3000", "http://localhost:8888"}, // 允许的来源
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},        // 允许的请求方法
		// AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},      // 允许的请求头
		// ExposeHeaders:    []string{"Content-Length"},                              // 可暴露的响应头
		AllowCredentials: true,      // 是否允许发送凭证
		MaxAge:           12 * 3600, // 预检请求的缓存时间
	}))

	// 初始化 JWT
	jwt.AccessTokenJwtInit()
	jwt.RefreshTokenJwtInit()

	// 错误处理
	r.Use(recovery.Recovery(recovery.WithRecoveryHandler(
		func(ctx context.Context, c *app.RequestContext, err interface{}, stack []byte) {
			hlog.SystemLogger().CtxErrorf(ctx, "[Recovery] err=%v\nstack=%s", err, stack)
			c.JSON(consts.StatusInternalServerError, map[string]interface{}{
				"code":    errno.ServiceErrCode,
				"message": fmt.Sprintf("[Recovery] err=%v\nstack=%s", err, stack),
			})
		})))

	// 注册路由
	register(r)
	r.Spin()
}
