package main

import (
	"net"
	"time"

	"HuaTug.com/config/jaeger"
	"HuaTug.com/kitex_gen/videos/videoservice"

	"HuaTug.com/cmd/video/dal"
	"HuaTug.com/cmd/video/infras/client"
	"HuaTug.com/cmd/video/infras/redis"

	"HuaTug.com/config"
	"HuaTug.com/pkg/bound"
	"HuaTug.com/pkg/constants"
	"HuaTug.com/pkg/middleware"
	"HuaTug.com/pkg/oss"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func Init() {
	//tracer2.InitJaeger(constants.UserServiceName)
	config.Init() // 首先加载配置
	dal.Init()    // 然后初始化数据库（需要使用配置）
	redis.Load()
	oss.InitMinio()
	client.Init()
	// common.NewSyncSerivce().Run()
}

func main() {
	Init()
	//pprof.Load()

	// Try to create etcd registry with timeout and retry
	etcdAddr := config.ConfigInfo.Etcd.Addr
	hlog.Infof("Attempting to connect to etcd at: %s", etcdAddr)

	var hasRegistry bool
	var registryOpt server.Option

	etcdRegistry, err := etcd.NewEtcdRegistry([]string{etcdAddr})
	if err != nil {
		hlog.Errorf("Failed to connect to etcd at %s: %v", etcdAddr, err)
		hlog.Warn("Running without service registry (etcd unavailable)")
		hasRegistry = false
	} else {
		hlog.Info("Successfully connected to etcd registry")
		hasRegistry = true
		registryOpt = server.WithRegistry(etcdRegistry)
	}

	suite, closer := jaeger.NewServerSuite().Init("Video")
	defer closer.Close()
	ip, err := constants.GetOutBoundIP()
	if err != nil {
		panic(err)
	}
	addr, err := net.ResolveTCPAddr("tcp", ip+":8891")
	if err != nil {
		panic(err)
	}

	//当出现了UserServiceImpl报错时 说明当前该接口的方法没有被完全实现

	//注意 这里的video等等方法在进行服务注册发现时 video此时是kitex生成下的一个service
	serverOpts := []server.Option{
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "Video"}), // server name
		server.WithMiddleware(middleware.CommonMiddleware),                           // middleware
		server.WithMiddleware(middleware.ServerMiddleware),
		server.WithServiceAddr(addr),                                       // address
		server.WithLimit(&limit.Option{MaxConnections: 1000, MaxQPS: 100}), // limit
		server.WithMuxTransport(),                                          // Multiplex
		server.WithSuite(suite),                                            // tracer
		server.WithBoundHandler(bound.NewCpuLimitHandler()),                // BoundHandler
		server.WithMaxConnIdleTime(30 * time.Second),
	}

	// Only add registry if etcd connection was successful
	if hasRegistry {
		serverOpts = append(serverOpts, registryOpt)
	}

	svr := videoservice.NewServer(new(VideoServiceImpl), serverOpts...)

	hlog.Infof("Starting Video service on %s", addr.String())
	err = svr.Run()
	if err != nil {
		hlog.Errorf("Video service failed: %v", err)
	}
}
