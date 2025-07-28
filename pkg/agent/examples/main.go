package main

import (
	"flag"
	"fmt"
	"time"

	"HuaTug.com/pkg/agent"
	"github.com/cloudwego/hertz/pkg/common/hlog"
)

func main() {
	// 命令行参数
	var (
		demoType = flag.String("demo", "basic", "演示类型: basic, continuous")
		userID   = flag.Int64("user", 1001, "用户ID")
		duration = flag.Duration("duration", 2*time.Minute, "连续演示持续时间")
	)
	flag.Parse()

	// 设置日志级别
	hlog.SetLevel(hlog.LevelInfo)

	fmt.Println("🚀 启动智能推荐Agent系统")
	fmt.Println("=====================================")

	// 创建演示实例
	demo := agent.NewAgentDemo()

	switch *demoType {
	case "basic":
		// 基础演示
		demo.RunDemo()
	case "continuous":
		// 连续演示
		demo.RunContinuousDemo(*userID, *duration)
	default:
		fmt.Printf("未知的演示类型: %s\n", *demoType)
		fmt.Println("支持的类型: basic, continuous")
	}

	fmt.Println("=====================================")
	fmt.Println("🎉 演示完成")
}
