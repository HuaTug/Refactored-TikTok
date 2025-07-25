package main

import (
	"net"
	"os"

	"HuaTug.com/cmd/interaction/dal"
	"HuaTug.com/cmd/interaction/infras/client"
	"HuaTug.com/cmd/interaction/infras/redis"
	"HuaTug.com/config/jaeger"

	"HuaTug.com/config"
	interaction "HuaTug.com/kitex_gen/interactions/interactionservice"
	"HuaTug.com/pkg/bound"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/middleware"
	"HuaTug.com/pkg/mq"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
	//trace "github.com/kitex-contrib/tracer-opentracing"
)

func Init() {
	//tracer2.InitJaeger(constants.UserServiceName)
	dal.Init()
	redis.Load()
	client.Init()

	// 初始化消息队列生产者
	initMessageQueue()

	// go common.NewCommentSync().Run()
	// go common.NewVideoSyncman().Run()
}

func initMessageQueue() {
	// 从环境变量或配置文件获取RabbitMQ连接URL
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}

	// 创建消息队列生产者
	producer, err := mq.NewProducer(rabbitmqURL)
	if err != nil {
		hlog.Fatalf("Failed to initialize message queue producer: %v", err)
		panic(err)
	}

	// 设置全局生产者实例
	SetGlobalProducer(producer)

	hlog.Info("Message queue producer initialized successfully")
}

func main() {
	//pprof.Load()
	config.Init()
	Init()
	//cache.Init()

	suite, closer := jaeger.NewServerSuite().Init("Interaction")
	defer closer.Close()
	r, err := etcd.NewEtcdRegistry([]string{config.ConfigInfo.Etcd.Addr})
	if err != nil {
		panic(err)
	}
	ip, err := constants.GetOutBoundIP()
	if err != nil {
		panic(err)
	}
	addr, err := net.ResolveTCPAddr("tcp", ip+":8893")
	if err != nil {
		panic(err)
	}

	//当出现了UserServiceImpl报错时 说明当前该接口的方法没有被完全实现

	svr := interaction.NewServer(new(InteractionServiceImpl),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "Interaction"}), // server name
		server.WithMiddleware(middleware.CommonMiddleware),                                 // middleware
		server.WithMiddleware(middleware.ServerMiddleware),
		server.WithServiceAddr(addr),                                       // address
		server.WithLimit(&limit.Option{MaxConnections: 1000, MaxQPS: 100}), // limit
		server.WithMuxTransport(),                                          // Multiplex
		//server.WithSuite(trace.NewDefaultServerSuite()),
		server.WithSuite(suite),                             // tracer
		server.WithBoundHandler(bound.NewCpuLimitHandler()), // BoundHandler
		server.WithRegistry(r),                              // registry
	)
	err = svr.Run()
	if err != nil {
		hlog.Info(err)
	}
}
