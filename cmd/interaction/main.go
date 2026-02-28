package main

import (
	"context"
	"net"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/cmd/interaction/dal"
	client "HuaTug.com/cmd/interaction/client_rpc"
	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/config"
	infraCache "HuaTug.com/pkg/infra/cache"
	"HuaTug.com/pkg/infra/jaeger"
	interaction "HuaTug.com/kitex_gen/interactions/interactionservice"
	"HuaTug.com/pkg/bound"
	"HuaTug.com/pkg/middleware"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func Init() {
	config.Init()
	dal.Init()
	redis.Load()
	client.Init()
	infraCache.Init()

	// 初始化RPC客户端，确保VideoClient被正确初始化
	rpc.InitVideoRpc()

	// Initialize MQ Manager for mention notifications and event publishing
	service.InitMentionNotificationFromConfig()

	// 启动 MQ 消费者 goroutine，消费通知事件并写入 Redis
	startNotificationConsumer()

	hlog.Info("Interaction service initialized successfully")
}

func main() {
	config.Init()
	Init()

	suite, closer := jaeger.NewServerSuite().Init("Interaction")
	defer closer.Close()
	r, err := etcd.NewEtcdRegistry([]string{config.ConfigInfo.Etcd.Addr})
	if err != nil {
		panic(err)
	}
	ip := "localhost"
	addr, err := net.ResolveTCPAddr("tcp", ip+":8893")
	if err != nil {
		panic(err)
	}

	svr := interaction.NewServer(new(InteractionServiceImpl),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "Interaction"}),
		server.WithMiddleware(middleware.CommonMiddleware),
		server.WithMiddleware(middleware.ServerMiddleware),
		server.WithServiceAddr(addr),
		server.WithLimit(&limit.Option{MaxConnections: 1000, MaxQPS: 100}),
		server.WithMuxTransport(),
		server.WithSuite(suite),
		server.WithBoundHandler(bound.NewCpuLimitHandler()),
		server.WithRegistry(r),
	)
	err = svr.Run()
	if err != nil {
		hlog.Info(err)
	}
}

// startNotificationConsumer 在 interaction 主服务中启动通知事件消费者
func startNotificationConsumer() {
	mqManager := service.GetInteractionMQManager()
	if mqManager == nil {
		hlog.Warn("MQ manager not available, notification consumer will not start")
		return
	}

	ctx := context.Background()
	notificationHandler := service.NewNotificationEventHandler()

	if err := mqManager.ConsumeNotificationEvents(ctx, notificationHandler); err != nil {
		hlog.Errorf("Failed to start notification event consumer: %v", err)
		return
	}

	hlog.Info("Notification event consumer started in interaction service")
}
