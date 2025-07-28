# 智能推荐Agent系统

## 概述

这是一个为TikTok项目设计的智能推荐Agent系统，基于感知-决策-行动（Perception-Decision-Action）三层架构，能够根据用户行为模式、兴趣偏好和上下文信息，提供个性化的视频推荐。

## 🏗️ 系统架构

### 核心模块

```
RecommendationAgent
├── PerceptionModule (感知模块)
│   ├── 用户行为分析
│   ├── 参与度评估
│   └── 探索度分析
├── DecisionModule (决策模块)
│   ├── 规则引擎
│   ├── 机器学习模型
│   └── 强化学习代理
└── ActionModule (行动模块)
    ├── 推荐策略执行
    ├── 内容过滤
    ├── 重排序
    └── 缓存管理
```

### 推荐模式

- **常规推荐 (ModeRegular)**: 基于用户历史兴趣的标准推荐
- **热点探索 (ModeHotExplore)**: 推荐当前热门和趋势内容
- **深度挖掘 (ModeDeepDive)**: 针对用户深度兴趣的专题推荐
- **新内容推荐 (ModeNewContent)**: 推荐最新发布的优质内容
- **个性化推荐 (ModePersonalized)**: 基于用户画像的深度个性化
- **多样化推荐 (ModeDiversified)**: 跨类别的多元化内容推荐

## 🚀 快速开始

### 1. 基础使用

```go
package main

import (
    "context"
    "HuaTug.com/pkg/agent"
)

func main() {
    // 创建Agent配置
    config := agent.DefaultAgentConfig()
    
    // 创建推荐Agent
    agent := agent.NewRecommendationAgent(config)
    
    // 设置外部服务（视频服务、用户服务）
    videoService := agent.NewMockVideoService() // 实际使用时替换为真实服务
    userService := agent.NewMockUserService()
    agent.GetActionModule().SetServices(videoService, userService)
    
    // 处理推荐请求
    ctx := context.Background()
    userID := int64(1001)
    
    // 构建用户行为序列
    behaviorSequence := &agent.BehaviorSequence{
        UserID: userID,
        Behaviors: []agent.Behavior{
            {
                VideoID:          1,
                BehaviorType:     agent.BehaviorView,
                Duration:         60 * time.Second,
                CompletionRate:   0.8,
                InteractionDepth: 3,
                Timestamp:        time.Now(),
            },
        },
        StartTime: time.Now().Add(-10 * time.Minute),
        EndTime:   time.Now(),
    }
    
    // 用户画像
    userProfile := &agent.UserProfile{
        InterestTags:     []string{"entertainment", "music"},
        ConsumptionLevel: agent.ConsumptionMedium,
        PreferredTopics: map[string]float64{
            "entertainment": 0.7,
            "music":        0.6,
        },
    }
    
    // 用户上下文
    userContext := &agent.UserContext{
        Timestamp:   time.Now(),
        TimeOfDay:   agent.TimeOfDayEvening,
        DeviceType:  "mobile",
        NetworkType: "5G",
    }
    
    // 获取推荐结果
    result, err := agent.ProcessRecommendationRequest(
        ctx, userID, behaviorSequence, userProfile, userContext)
    if err != nil {
        log.Fatal(err)
    }
    
    // 处理推荐结果
    fmt.Printf("推荐了 %d 个视频\n", len(result.RecommendedVideos))
    for _, video := range result.RecommendedVideos {
        fmt.Printf("- %s (评分: %.2f)\n", video.Title, video.Score)
    }
}
```

### 2. 运行演示

```bash
# 进入项目目录
cd pkg/agent/examples

# 运行基础演示
go run main.go -demo=basic

# 运行连续演示
go run main.go -demo=continuous -user=1001 -duration=5m
```

### 3. 运行测试

```bash
# 运行单元测试
go test -v ./pkg/agent

# 运行基准测试
go test -bench=. ./pkg/agent

# 查看测试覆盖率
go test -cover ./pkg/agent
```

## 📊 核心功能

### 用户状态感知

系统能够分析用户的行为模式，评估：
- **参与度水平**: 无聊 → 随意 → 参与 → 沉浸
- **探索意愿**: 聚焦 → 混合 → 多样化
- **当前兴趣**: 基于行为序列动态提取

### 智能决策策略

根据用户状态选择最适合的推荐策略：
- **规则引擎**: 基于业务规则的快速决策
- **机器学习**: 基于历史数据的模型预测
- **强化学习**: 基于用户反馈的在线学习

### 多样化推荐执行

- **内容获取**: 支持多种内容源和获取策略
- **质量评估**: 综合考虑内容质量、用户匹配度、新鲜度
- **智能过滤**: 去重、质量筛选、用户级别过滤
- **动态排序**: 基于用户状态的个性化排序

## 🔧 配置说明

### AgentConfig 配置项

```go
type AgentConfig struct {
    // 感知模块配置
    BehaviorWindowSeconds int     // 行为窗口时间（秒），默认300
    BoredThreshold        float64 // 无聊阈值，默认0.15
    DeepInterestThreshold float64 // 深度兴趣阈值，默认0.80
    
    // 决策模块配置
    DecisionMode string // 决策模式: "rule"/"ml"/"rl"
    ModelPath    string // 模型文件路径
    
    // 行动模块配置
    DefaultRecommendCount int // 默认推荐数量，默认10
    HotTopicCount         int // 热点推荐数量，默认5
    DeepDiveCount         int // 深度挖掘数量，默认8
}
```

### 默认配置

```go
config := &AgentConfig{
    BehaviorWindowSeconds: 300,    // 5分钟行为窗口
    BoredThreshold:        0.15,   // 15%完播率以下认为无聊
    DeepInterestThreshold: 0.80,   // 80%完播率以上认为深度兴趣
    DecisionMode:          "rule", // 默认使用规则模式
    DefaultRecommendCount: 10,
    HotTopicCount:         5,
    DeepDiveCount:         8,
}
```

## 📈 性能特性

### 缓存机制

- **推荐缓存**: 相同用户状态下的推荐结果缓存
- **TTL管理**: 自动过期清理
- **LRU淘汰**: 内存使用优化

### 监控指标

- 推荐响应时间
- 缓存命中率
- 各模式推荐分布
- 用户状态分析统计

## 🔌 扩展接口

### VideoService 接口

```go
type VideoService interface {
    GetVideosByCategory(ctx context.Context, category string, limit int) ([]Video, error)
    GetTrendingVideos(ctx context.Context, limit int) ([]Video, error)
    GetPersonalizedVideos(ctx context.Context, userID int64, interests []string, limit int) ([]Video, error)
    GetSimilarVideos(ctx context.Context, videoID int64, limit int) ([]Video, error)
    GetVideosByTags(ctx context.Context, tags []string, limit int) ([]Video, error)
}
```

### UserService 接口

```go
type UserService interface {
    GetUserInteractionHistory(ctx context.Context, userID int64, days int) ([]UserInteraction, error)
    GetUserProfile(ctx context.Context, userID int64) (*UserProfile, error)
    UpdateUserInterests(ctx context.Context, userID int64, interests []string) error
}
```

## 🎯 使用场景

### 1. 无聊用户激活

当用户表现出低参与度行为（低完播率、频繁跳过）时：
- 推荐短视频、娱乐内容
- 增加热门话题推荐
- 降低内容复杂度

### 2. 深度用户满足

当用户表现出高参与度行为（高完播率、频繁互动）时：
- 推荐相关深度内容
- 增加专业性内容比重
- 推荐同作者其他作品

### 3. 探索用户引导

当用户表现出探索意愿时：
- 推荐跨类别内容
- 引入新兴话题
- 平衡熟悉与新奇内容

## 🔍 日志和调试

系统使用hertz的日志组件，可以通过以下方式查看运行日志：

```go
import "github.com/cloudwego/hertz/pkg/common/hlog"

// 设置日志级别
hlog.SetLevel(hlog.LevelDebug)
```

关键日志信息包括：
- 用户状态分析结果
- 决策策略选择过程
- 推荐执行详情
- 性能监控数据

## 🚧 后续优化方向

1. **机器学习增强**: 集成更复杂的ML模型
2. **实时学习**: 实现在线学习和模型更新
3. **多目标优化**: 平衡准确性、多样性、新颖性
4. **A/B测试支持**: 内置实验框架
5. **冷启动优化**: 新用户推荐策略优化

## 📝 更新日志

### v1.0.0 (当前版本)
- 完整的PDA三层架构实现
- 支持6种推荐模式
- Mock服务完整实现
- 单元测试和基准测试
- 演示程序和使用文档

## 📄 许可证

本项目采用MIT许可证。详见 [LICENSE](../../../LICENSE) 文件。
