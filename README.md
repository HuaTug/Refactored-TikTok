# Refactored-TikTok

一个基于微服务架构的高性能短视频平台，使用 CloudWeGo 生态系统（Kitex + Hertz）构建，实现了完整的 TikTok 功能。

## 🚀 项目特色

- **智能推荐系统**: 多路召回+精排+重排三阶段推荐,支持个性化内容分发 ⭐⭐⭐
- **微服务架构**: 基于 Kitex RPC 框架的分布式微服务设计
- **高性能**: 使用 Hertz HTTP 框架提供高性能 API 网关
- **事件驱动**: 实现事件驱动的数据同步机制，保证最终一致性
- **分库分表**: 评论和关系服务 4库×4表架构,支持海量数据 ⭐⭐⭐
- **多层限流**: 滑动窗口、令牌桶等多种限流算法,集成 Sentinel 熔断降级 ⭐⭐⭐
- **冷热分离**: 基于 MinIO 的多层存储策略,自动热度迁移 ⭐⭐
- **分布式存储**: 支持 MySQL 分库分表、Redis 缓存、MinIO 对象存储
- **消息队列**: 集成 RabbitMQ 实现异步消息处理
- **服务发现**: 基于 etcd 的服务注册与发现
- **链路追踪**: 集成 Jaeger 分布式链路追踪
- **容器化部署**: 完整的 Docker Compose 部署方案

## 🏗️ 系统架构

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Frontend  │    │  Mobile Client  │    │   Admin Panel   │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                    ┌─────────────┴─────────────┐
                    │      API Gateway          │
                    │      (Hertz)              │
                    └─────────────┬─────────────┘
                                  │
          ┌───────────────────────┼───────────────────────┐
          │                       │                       │
    ┌─────┴─────┐         ┌───────┴───────┐       ┌───────┴───────┐
    │   User    │         │    Video      │       │  Interaction  │
    │ Service   │         │   Service     │       │   Service     │
    └─────┬─────┘         └───────┬───────┘       └───────┬───────┘
          │                       │                       │
    ┌─────┴─────┐         ┌───────┴───────┐       ┌───────┴───────┐
    │ Relation  │         │   Message     │       │               │
    │ Service   │         │   Service     │       │               │
    └───────────┘         └───────────────┘       └───────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │    Infrastructure         │
                    │  MySQL | Redis | MinIO    │
                    │  RabbitMQ | etcd | Jaeger │
                    └───────────────────────────┘
```

## 🛠️ 技术栈

### 后端框架
- **[Kitex](https://github.com/cloudwego/kitex)**: 高性能 RPC 框架
- **[Hertz](https://github.com/cloudwego/hertz)**: 高性能 HTTP 框架
- **[Thrift](https://thrift.apache.org/)**: IDL 定义和代码生成

### 数据存储
- **MySQL**: 主数据库，支持分库分表
- **Redis**: 缓存和会话存储
- **MinIO**: 对象存储服务

### 中间件
- **RabbitMQ**: 消息队列
- **etcd**: 服务注册与发现
- **Jaeger**: 分布式链路追踪
- **Elasticsearch**: 日志聚合和搜索


## 📁 项目结构

```
Refactored-TikTok/
├── cmd/                    # 微服务入口
│   ├── api/               # API 网关服务
│   ├── user/              # 用户服务
│   ├── video/             # 视频服务
│   ├── interaction/       # 互动服务（点赞、评论）
│   ├── relation/          # 关系服务（关注、粉丝）
│   ├── message/           # 消息服务
│   └── model/             # 数据模型
├── config/                # 配置文件
├── docs/                  # 项目文档
├── idl/                   # Thrift IDL 定义
├── kitex_gen/             # Kitex 生成代码
├── pkg/                   # 公共包
├── scripts/               # 部署脚本
└── video_recommendation/  # 推荐算法
```

## 🚀 快速开始

### 环境要求

- Go 1.24.4+
- Docker & Docker Compose
- MySQL 8.0+
- Redis 6.0+
- etcd 3.5+

### 1. 克隆项目

```bash
git clone https://github.com/your-username/Refactored-TikTok.git
cd Refactored-TikTok
```

### 2. 配置环境

复制配置文件并修改相关配置：

```bash
cp config/config.yml.example config/config.yml
# 编辑配置文件，设置数据库连接等信息
```

### 3. 启动基础设施

```bash
# 启动 MySQL, Redis, etcd, RabbitMQ 等服务
docker-compose up -d mysql redis etcd rabbitmq minio jaeger
```

### 4. 数据库初始化

```bash
# 创建数据库和表结构
mysql -h localhost -u root -p < data/init.sql
```

### 5. 生成代码

```bash
# 生成 Kitex RPC 代码
chmod +x kitex_gen.sh
./kitex_gen.sh

# 生成 Hertz HTTP 代码
chmod +x hertz_gen.sh
./hertz_gen.sh
```

### 6. 构建和启动服务

```bash
# 构建所有服务
make build

# 启动所有微服务
docker-compose up -d
```

### 7. 验证部署

```bash
# 检查服务状态
curl http://localhost:8080/ping

# 查看服务注册情况
curl http://localhost:2379/v2/keys/kitex
```

## 📖 API 文档

### 用户相关
- `POST /douyin/user/register/` - 用户注册
- `POST /douyin/user/login/` - 用户登录
- `GET /douyin/user/` - 获取用户信息

### 视频相关
- `GET /douyin/feed/` - 视频流
- `POST /douyin/publish/action/` - 视频发布
- `GET /douyin/publish/list/` - 发布列表

### 互动相关
- `POST /douyin/favorite/action/` - 点赞操作
- `GET /douyin/favorite/list/` - 点赞列表
- `POST /douyin/comment/action/` - 评论操作
- `GET /douyin/comment/list/` - 评论列表

### 社交相关
- `POST /douyin/relation/action/` - 关注操作
- `GET /douyin/relation/follow/list/` - 关注列表
- `GET /douyin/relation/follower/list/` - 粉丝列表

详细 API 文档请参考 [API Documentation](docs/api.md)

## 🔧 配置说明

主要配置文件位于 `config/config.yml`：

```yaml
server:
  name: "douyin"
  host: "0.0.0.0"
  port: 8080

mysql:
  host: "localhost"
  port: 3306
  database: "douyin"
  username: "root"
  password: "password"

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0

etcd:
  addr: "localhost:2379"
```

## 🏗️ 微服务详解

### API Gateway (Hertz)
- 统一入口，路由分发
- 身份认证和授权
- 限流和熔断
- 请求日志记录

### User Service
- 用户注册和登录
- 用户信息管理
- JWT Token 生成和验证

### Video Service
- 视频上传和存储
- 视频信息管理
- 视频推荐算法
- 视频转码处理

### Interaction Service
- 点赞功能
- 评论系统
- 事件驱动的数据同步

### Relation Service
- 关注/取消关注
- 粉丝关系管理
- 好友推荐

### Message Service
- 私信功能
- 消息推送
- 聊天记录

## 🔄 事件驱动架构

项目实现了完整的事件驱动数据同步机制：

1. **立即响应**: Redis 缓存提供快速用户反馈
2. **异步处理**: 消息队列处理数据库操作
3. **幂等性保证**: 防止重复处理
4. **事务一致性**: 数据库操作使用事务
5. **监控告警**: 完善的错误处理和重试机制

详细说明请参考 [事件驱动同步文档](docs/EVENT_DRIVEN_SYNC.md)

## 📊 性能优化

- **缓存策略**: 多级缓存，热点数据预加载
- **数据库优化**: 读写分离，分库分表
- **连接池**: 数据库和 Redis 连接池优化
- **异步处理**: 非核心业务异步化

## 🔍 监控和运维

### 日志管理
- 结构化日志输出
- 日志级别控制
- ELK 日志聚合

### 链路追踪
- Jaeger 分布式追踪
- 性能瓶颈分析
- 错误链路定位

### 健康检查
- 服务健康状态监控
- 自动故障转移
- 服务降级策略

## 🎯 技术亮点与未来规划

### ✅ 已实现的核心亮点
1. **智能推荐系统** ⭐⭐⭐
   - 多路召回策略 (协同过滤、热门、内容、社交、探索)
   - Learning to Rank 精排模型
   - MMR 重排序保证多样性
   - 详见: [`pkg/recommendation/README.md`](pkg/recommendation/README.md)

2. **分库分表架构** ⭐⭐⭐
   - 评论服务: 4库×4表
   - 关系服务: 4库×4表
   - 详见: [`scripts/init_relation_shard.sh`](scripts/init_relation_shard.sh)

3. **多层限流与熔断** ⭐⭐⭐
   - 滑动窗口、令牌桶、固定窗口、自适应限流
   - 用户级、IP级、全局限流
   - 集成 Alibaba Sentinel
   - 详见: [`pkg/security/advanced_rate_limiter.go`](pkg/security/advanced_rate_limiter.go)

4. **事件驱动架构** ⭐⭐⭐
   - 完整的事件溯源模型
   - 幂等性和分布式锁
   - 详见: [`cmd/model/event_driven_models.go`](cmd/model/event_driven_models.go)

5. **冷热数据分离** ⭐⭐
   - 基于 MinIO 的多层存储
   - 自动热度迁移
   - 详见: [`pkg/oss/hot_storage_manager.go`](pkg/oss/hot_storage_manager.go)

### 🚀 未来规划
- [ ] 推荐算法优化 (机器学习模型升级, GBDT/DeepFM)
- [ ] 完善监控体系 (Prometheus + Grafana)
- [ ] 视频内容审核 (AI审核)
- [ ] 直播功能
- [ ] 实时消息推送 (WebSocket)
- [ ] A/B测试框架
- [ ] CDN 加速
- [ ] 数据库读写分离

**详细分析请参考**: [`docs/HIGHLIGHTS_AND_IMPROVEMENTS.md`](docs/HIGHLIGHTS_AND_IMPROVEMENTS.md)
