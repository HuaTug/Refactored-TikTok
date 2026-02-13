package main

import (
	"net"

	"HuaTug.com/cmd/api/rpc"
	"HuaTug.com/cmd/interaction/dal"
	client "HuaTug.com/cmd/interaction/client_rpc"
	redis "HuaTug.com/cmd/interaction/cache"
	"HuaTug.com/cmd/interaction/service"
	"HuaTug.com/config"
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

	// 初始化RPC客户端，确保VideoClient被正确初始化
	rpc.InitVideoRpc()

	// Initialize MQ Manager for mention notifications
	service.InitMentionNotificationFromConfig()

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
