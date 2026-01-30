# TikTok 日志上报系统使用文档

## 目录

1. [系统概述](#系统概述)
2. [架构设计](#架构设计)
3. [快速开始](#快速开始)
4. [API 参考](#api-参考)
5. [使用示例](#使用示例)
6. [配置说明](#配置说明)
7. [ES 查询指南](#es-查询指南)
8. [最佳实践](#最佳实践)

---

## 系统概述

TikTok 日志上报系统是一个基于 **Kafka + Elasticsearch** 的分布式日志收集与存储方案，主要用于：

- 记录服务调用失败的详细信息
- 追踪系统错误和异常
- 审计用户敏感操作
- 监控告警事件

### 日志类型

| 日志类型 | Kafka Topic | ES 索引前缀 | 用途 |
|---------|-------------|-------------|------|
| 服务调用日志 | `service_log` | `tiktok-service-log-` | 记录所有 API/RPC 调用的成功/失败 |
| 错误日志 | `error_log` | `tiktok-error-log-` | 记录详细的错误信息和堆栈 |
| 审计日志 | `audit_log` | `tiktok-audit-log-` | 记录用户敏感操作 |
| 告警日志 | `alert_log` | `tiktok-alert-log-` | 记录系统告警事件 |

---

## 架构设计

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              日志系统架构                                            │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                     │
│   ┌──────────────┐    ┌──────────────┐    ┌──────────────┐                          │
│   │  API 网关    │    │ Video 服务   │    │  其他服务    │                          │
│   │  (HTTP)      │    │  (RPC)       │    │  (RPC)       │                          │
│   └──────┬───────┘    └──────┬───────┘    └──────┬───────┘                          │
│          │                   │                   │                                  │
│          └───────────────────┼───────────────────┘                                  │
│                              │                                                      │
│                              ▼                                                      │
│   ┌─────────────────────────────────────────────────────────────────────────────┐   │
│   │                        logsystem 包                                          │   │
│   │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │   │
│   │  │ LogServiceError │  │ LogError        │  │ LogAudit        │              │   │
│   │  │ LogRPCError     │  │ LogBusinessError│  │ LogAlert        │              │   │
│   │  └─────────────────┘  └─────────────────┘  └─────────────────┘              │   │
│   └─────────────────────────────────────────────────────────────────────────────┘   │
│                              │                                                      │
│                              ▼ (异步发送)                                           │
│   ┌─────────────────────────────────────────────────────────────────────────────┐   │
│   │                         Kafka Cluster                                        │   │
│   │   service_log │ error_log │ audit_log │ alert_log                           │   │
│   └─────────────────────────────────────────────────────────────────────────────┘   │
│                              │                                                      │
│                              ▼ (批量消费)                                           │
│   ┌─────────────────────────────────────────────────────────────────────────────┐   │
│   │                      LogConsumer 独立服务                                    │   │
│   │   • 批量写入 (batchSize=100)                                                │   │
│   │   • 定时刷新 (flushInterval=5s)                                             │   │
│   └─────────────────────────────────────────────────────────────────────────────┘   │
│                              │                                                      │
│                              ▼                                                      │
│   ┌─────────────────────────────────────────────────────────────────────────────┐   │
│   │                       Elasticsearch                                          │   │
│   │   tiktok-service-log-2026.01.30                                              │   │
│   │   tiktok-error-log-2026.01.30                                                │   │
│   │   tiktok-audit-log-2026.01.30                                                │   │
│   └─────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 快速开始

### 1. 初始化日志系统

在服务的 `main.go` 中初始化：

```go
package main

import (
    "HuaTug.com/pkg/logsystem"
)

func main() {
    // 初始化日志系统
    if err := logsystem.Init(&logsystem.LogSystemConfig{
        ServiceName:      "your-service-name",  // 服务名称
        Environment:      "production",         // 环境: dev/test/prod
        Version:          "v1.0.0",             // 服务版本
        EnableESConsumer: false,                // API/RPC 服务设为 false
    }); err != nil {
        log.Printf("Failed to init log system: %v", err)
    }
    
    // 确保退出时关闭
    defer logsystem.Close()
    
    // ... 其他初始化代码 ...
}
```

### 2. 启动独立日志消费服务

```bash
cd cmd/logservice
go run main.go
```

---

## API 参考

### 包导入

```go
import "HuaTug.com/pkg/logsystem"
```

### 核心函数

#### 1. LogServiceError - 记录服务调用失败

用于在 **HTTP Handler** 中记录 API 调用失败。

```go
func LogServiceError(
    ctx context.Context, 
    c *app.RequestContext, 
    methodName string,      // 方法名称
    errorCode string,       // 错误码
    errorMessage string,    // 错误信息
    duration int64,         // 耗时(毫秒)
)
```

**示例：**
```go
timer := logsystem.NewTimer()
// ... 业务逻辑 ...
if err != nil {
    logsystem.LogServiceError(ctx, c, "VideoPublishStartV2", "RPC_CALL_FAILED", err.Error(), timer.ElapsedMs())
}
```

---

#### 2. LogError - 记录错误详情

用于记录详细的错误信息。

```go
func LogError(
    ctx context.Context, 
    c *app.RequestContext, 
    methodName string,      // 方法名称
    errorCode string,       // 错误码
    errorType string,       // 错误类型: panic/business/system/network
    errorMessage string,    // 错误信息
)
```

**示例：**
```go
logsystem.LogError(ctx, c, "UserLogin", "AUTH_FAILED", "business", "用户名或密码错误")
```

---

#### 3. LogErrorWithContext - 记录带上下文的错误

用于记录包含额外上下文信息的错误。

```go
func LogErrorWithContext(
    ctx context.Context, 
    c *app.RequestContext, 
    methodName string,           // 方法名称
    errorCode string,            // 错误码
    errorType string,            // 错误类型
    errorMessage string,         // 错误信息
    extraContext map[string]string,  // 额外上下文
)
```

**示例：**
```go
logsystem.LogErrorWithContext(ctx, c, "VideoUpload", "STORAGE_ERROR", "system", err.Error(), map[string]string{
    "user_id":   "12345",
    "video_id":  "67890",
    "file_size": "1024000",
})
```

---

#### 4. LogRPCError - 记录 RPC 调用失败

用于在 **RPC Service Handler** 中记录调用失败。

```go
func LogRPCError(
    ctx context.Context, 
    serviceName string,     // 服务名称
    methodName string,      // 方法名称
    errorCode string,       // 错误码
    errorMessage string,    // 错误信息
    userID int64,           // 用户ID
    duration int64,         // 耗时(毫秒)
)
```

**示例：**
```go
startTime := time.Now()
// ... 业务逻辑 ...
if err != nil {
    logsystem.LogRPCError(ctx, "video-service", "VideoFeedListV2", "DB_ERROR", err.Error(), req.UserId, time.Since(startTime).Milliseconds())
}
```

---

#### 5. LogRPCErrorWithContext - 记录带上下文的 RPC 错误

```go
func LogRPCErrorWithContext(
    ctx context.Context, 
    serviceName string,          // 服务名称
    methodName string,           // 方法名称
    errorCode string,            // 错误码
    errorType string,            // 错误类型
    errorMessage string,         // 错误信息
    userID int64,                // 用户ID
    extraContext map[string]string,  // 额外上下文
)
```

**示例：**
```go
logsystem.LogRPCErrorWithContext(ctx, "video-service", "VideoPublishStartV2", "UPLOAD_FAILED", "system", err.Error(), req.UserId, map[string]string{
    "title":        req.Title,
    "chunk_number": "10",
})
```

---

#### 6. LogBusinessError - 记录业务错误 (简化版)

```go
func LogBusinessError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string)
```

**示例：**
```go
logsystem.LogBusinessError(ctx, c, "USER_NOT_FOUND", "用户不存在")
```

---

#### 7. LogSystemError - 记录系统错误 (简化版)

```go
func LogSystemError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string)
```

**示例：**
```go
logsystem.LogSystemError(ctx, c, "DB_CONNECTION_FAILED", "数据库连接失败")
```

---

#### 8. LogNetworkError - 记录网络错误 (简化版)

```go
func LogNetworkError(ctx context.Context, c *app.RequestContext, errorCode, errorMessage string)
```

**示例：**
```go
logsystem.LogNetworkError(ctx, c, "RPC_TIMEOUT", "RPC调用超时")
```

---

#### 9. LogAudit - 记录审计日志

用于记录用户敏感操作（如修改密码、删除数据等）。

```go
func LogAudit(
    ctx context.Context, 
    c *app.RequestContext, 
    targetID int64,         // 目标ID
    targetType string,      // 目标类型: user/video/comment
    action string,          // 操作: create/update/delete
    resource string,        // 资源路径
    oldValue string,        // 旧值 (可选)
    newValue string,        // 新值 (可选)
    success bool,           // 是否成功
)
```

**示例：**
```go
logsystem.LogAudit(ctx, c, videoID, "video", "delete", "/api/v1/videos/123", "", "", true)
```

---

#### 10. LogAlert - 记录告警日志

用于记录系统告警事件。

```go
func LogAlert(
    ctx context.Context, 
    alertName string,       // 告警名称
    alertType string,       // 告警类型
    severity string,        // 严重程度: info/warning/critical
    metricName string,      // 指标名称
    metricValue float64,    // 指标值
    threshold float64,      // 阈值
    message string,         // 告警信息
)
```

**示例：**
```go
logsystem.LogAlert(ctx, "HighCPUUsage", "resource", "critical", "cpu_usage", 95.5, 80.0, "CPU使用率超过阈值")
```

---

### 辅助函数

#### NewTimer - 创建计时器

```go
timer := logsystem.NewTimer()
// ... 业务逻辑 ...
elapsed := timer.ElapsedMs()  // 返回毫秒数
```

#### SetUserID - 设置用户ID到上下文

```go
logsystem.SetUserID(c, userID)  // 供日志中间件自动获取
```

#### SetErrorInfo - 设置错误信息到上下文

```go
logsystem.SetErrorInfo(c, "ERROR_CODE", "错误信息")  // 供日志中间件自动记录
```

---

## 使用示例

### 示例 1: HTTP Handler 中记录错误

```go
package handlers

import (
    "context"
    "HuaTug.com/pkg/logsystem"
    "HuaTug.com/pkg/errno"
    "github.com/cloudwego/hertz/pkg/app"
)

func VideoPublishStartV2(ctx context.Context, c *app.RequestContext) {
    // 1. 开始计时
    timer := logsystem.NewTimer()
    
    // 2. 参数绑定
    var req VideoPublishStartParam
    if err := c.BindAndValidate(&req); err != nil {
        // 记录参数错误
        logsystem.LogBusinessError(ctx, c, "INVALID_PARAMS", err.Error())
        logsystem.SetErrorInfo(c, "INVALID_PARAMS", err.Error())
        SendResponse(c, errno.ConvertErr(err), nil)
        return
    }
    
    // 3. JWT 认证
    userID, err := getJWTUserID(ctx, c)
    if err != nil {
        // 记录认证失败
        logsystem.LogBusinessError(ctx, c, "AUTH_FAILED", err.Error())
        logsystem.SetErrorInfo(c, "AUTH_FAILED", err.Error())
        SendResponse(c, errno.ConvertErr(err), nil)
        return
    }
    
    // 设置用户ID供后续日志使用
    logsystem.SetUserID(c, userID)
    
    // 4. 调用 RPC 服务
    resp, err := rpc.VideoPublishStartV2(ctx, &videos.VideoPublishStartRequestV2{
        UserId: userID,
        Title:  req.Title,
    })
    
    if err != nil {
        // 记录 RPC 调用失败 -> 写入 Kafka -> ES
        logsystem.LogServiceError(ctx, c, "VideoPublishStartV2", "RPC_CALL_FAILED", err.Error(), timer.ElapsedMs())
        
        // 记录详细错误信息（带上下文）
        logsystem.LogErrorWithContext(ctx, c, "VideoPublishStartV2", "RPC_CALL_FAILED", "network", err.Error(), map[string]string{
            "user_id": fmt.Sprintf("%d", userID),
            "title":   req.Title,
        })
        
        logsystem.SetErrorInfo(c, "RPC_CALL_FAILED", err.Error())
        SendResponse(c, errno.ConvertErr(err), nil)
        return
    }
    
    SendResponse(c, errno.Success, resp)
}
```

### 示例 2: RPC Service Handler 中记录错误

```go
package main

import (
    "context"
    "time"
    "HuaTug.com/pkg/logsystem"
)

func (s *VideoServiceImpl) VideoFeedListV2(ctx context.Context, req *videos.VideoFeedListRequestV2) (*videos.VideoFeedListResponseV2, error) {
    startTime := time.Now()
    resp := &videos.VideoFeedListResponseV2{
        Base: &base.Status{},
    }
    
    // 调用业务逻辑
    videos, err := service.NewVideoListService(ctx).VideoList(req)
    if err != nil {
        // 记录错误 -> Kafka -> ES
        logsystem.LogRPCError(ctx, "video-service", "VideoFeedListV2", "VIDEO_LIST_FAILED", err.Error(), req.UserId, time.Since(startTime).Milliseconds())
        
        resp.Base.Code = 400
        resp.Base.Msg = "获取视频列表失败"
        return resp, err
    }
    
    resp.Base.Code = 200
    resp.Base.Msg = "成功"
    resp.VideoList = videos
    return resp, nil
}
```

### 示例 3: 记录审计日志

```go
func DeleteVideo(ctx context.Context, c *app.RequestContext) {
    videoID := c.Param("id")
    
    // 执行删除操作
    err := videoService.Delete(videoID)
    
    // 无论成功失败都记录审计日志
    logsystem.LogAudit(ctx, c, 
        parseInt64(videoID),  // targetID
        "video",              // targetType
        "delete",             // action
        "/api/v1/videos/"+videoID,  // resource
        "",                   // oldValue
        "",                   // newValue
        err == nil,           // success
    )
    
    if err != nil {
        SendResponse(c, errno.ConvertErr(err), nil)
        return
    }
    
    SendResponse(c, errno.Success, nil)
}
```

---

## 配置说明

### config.yaml 配置项

```yaml
# Kafka 配置
kafka:
  brokers:
    - "localhost:9092"
  version: "2.8.0"
  producer_retries: 3

# Elasticsearch 配置
elasticsearch:
  addresses:
    - "http://localhost:9200"
  username: ""
  password: ""
  index_prefix: "tiktok"
  max_retries: 3
  enable_sniff: false
```

### 日志消费服务配置

```go
consumerConfig := &logger.LogConsumerConfig{
    BatchSize:     100,           // 批量写入大小
    FlushInterval: 5 * time.Second,  // 刷新间隔
}
```

---

## ES 查询指南

### 1. 查询最近失败的请求

```json
GET tiktok-service-log-*/_search
{
  "query": {
    "bool": {
      "must": [
        { "term": { "success": false } },
        { "range": { "timestamp": { "gte": "now-1h" } } }
      ]
    }
  },
  "sort": [{ "timestamp": "desc" }],
  "size": 100
}
```

### 2. 查询特定错误码

```json
GET tiktok-error-log-*/_search
{
  "query": {
    "term": { "error_code": "RPC_CALL_FAILED" }
  }
}
```

### 3. 按服务分组统计错误数量

```json
GET tiktok-service-log-*/_search
{
  "size": 0,
  "query": {
    "bool": {
      "must": [
        { "term": { "success": false } },
        { "range": { "timestamp": { "gte": "now-24h" } } }
      ]
    }
  },
  "aggs": {
    "by_service": {
      "terms": { "field": "service_name.keyword" }
    }
  }
}
```

### 4. 查询特定用户的操作审计

```json
GET tiktok-audit-log-*/_search
{
  "query": {
    "term": { "user_id": 12345 }
  },
  "sort": [{ "timestamp": "desc" }]
}
```

### 5. 查询响应时间超过阈值的请求

```json
GET tiktok-service-log-*/_search
{
  "query": {
    "range": {
      "duration": { "gte": 1000 }
    }
  }
}
```

---

## 最佳实践

### 1. 错误码规范

建议使用统一的错误码命名规范：

| 前缀 | 含义 | 示例 |
|------|------|------|
| `AUTH_` | 认证相关 | `AUTH_FAILED`, `AUTH_EXPIRED` |
| `PARAM_` | 参数相关 | `PARAM_INVALID`, `PARAM_MISSING` |
| `DB_` | 数据库相关 | `DB_ERROR`, `DB_NOT_FOUND` |
| `RPC_` | RPC调用相关 | `RPC_TIMEOUT`, `RPC_CALL_FAILED` |
| `STORAGE_` | 存储相关 | `STORAGE_ERROR`, `STORAGE_FULL` |
| `BIZ_` | 业务相关 | `BIZ_USER_NOT_FOUND`, `BIZ_VIDEO_NOT_FOUND` |

### 2. 错误类型规范

| 类型 | 含义 | 使用场景 |
|------|------|----------|
| `business` | 业务错误 | 用户输入错误、业务规则校验失败 |
| `system` | 系统错误 | 数据库异常、内部逻辑错误 |
| `network` | 网络错误 | RPC超时、连接失败 |
| `panic` | 程序崩溃 | 未捕获的异常 |

### 3. 上下文信息建议

记录错误时，建议包含以下上下文信息：

```go
logsystem.LogErrorWithContext(ctx, c, "MethodName", "ERROR_CODE", "system", err.Error(), map[string]string{
    "user_id":    "用户ID",
    "request_id": "请求ID",
    "video_id":   "相关视频ID",
    "action":     "执行的操作",
    "input":      "关键输入参数(脱敏后)",
})
```

### 4. 敏感信息处理

**禁止记录以下敏感信息：**
- 用户密码
- 完整的身份证号
- 完整的手机号
- Token/密钥

**建议脱敏处理：**
```go
// 手机号脱敏
phone := "138****1234"

// 身份证脱敏
idCard := "110***********1234"
```

### 5. 性能注意事项

- 日志发送是**异步**的，不会阻塞主业务流程
- 避免在循环中频繁记录日志
- 大批量操作建议使用汇总日志而非逐条记录

---

## 文件结构

```
pkg/
├── logsystem/
│   ├── init.go          # 日志系统初始化
│   └── helpers.go       # 便捷日志记录函数
├── logger/
│   ├── collector.go     # 日志收集器
│   ├── consumer.go      # 日志消费者
│   └── middleware.go    # HTTP 日志中间件
├── kafka/
│   ├── manager.go       # Kafka 管理器
│   ├── events.go        # 日志事件定义
│   └── topics.go        # Topic 定义
└── elasticsearch/
    └── client.go        # ES 客户端

cmd/
└── logservice/
    └── main.go          # 独立日志消费服务
```

---

## 常见问题

### Q1: 日志没有写入 ES？

检查：
1. Kafka 服务是否正常运行
2. ES 服务是否正常运行
3. `logservice` 消费服务是否启动
4. 配置文件中的地址是否正确

### Q2: 如何在本地测试？

```bash
# 1. 启动 Kafka
docker-compose up kafka -d

# 2. 启动 ES
docker-compose up elasticsearch -d

# 3. 启动日志消费服务
cd cmd/logservice && go run main.go

# 4. 启动 API 服务
cd cmd/api && go run main.go
```

### Q3: 如何扩展新的日志类型？

1. 在 `pkg/kafka/events.go` 中定义新的事件结构
2. 在 `pkg/kafka/topics.go` 中添加新的 Topic
3. 在 `pkg/logger/collector.go` 中添加新的收集方法
4. 在 `pkg/logger/consumer.go` 中添加新的消费处理
5. 在 `pkg/logsystem/helpers.go` 中添加便捷函数
