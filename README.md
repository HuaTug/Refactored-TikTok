# Refactored-TikTok

一个基于微服务架构的短视频平台，使用 CloudWeGo 生态（Kitex + Hertz）构建。

## 项目特色

- **智能推荐系统**：多路召回 + 精排 + 重排三阶段流程，支持个性化内容分发
- **微服务架构**：基于 Kitex RPC 的分布式微服务
- **事件驱动**：事件驱动的数据同步机制，保证最终一致性
- **分库分表**：评论和关系服务 4 库 × 4 表架构
- **多层限流**：滑动窗口 / 令牌桶等多种限流算法
- **冷热分离**：基于 MinIO 的多层存储策略
- **分布式存储**：MySQL 分库分表、Redis 缓存、MinIO 对象存储
- **消息队列**：RabbitMQ / Kafka 异步消息处理
- **服务发现**：基于 etcd 的服务注册与发现
- **链路追踪**：Jaeger 分布式链路追踪
- **容器化部署**：Docker Compose 一键部署

## 系统架构

```
┌─────────────────┐    ┌─────────────────┐
│   Web Frontend  │    │  Mobile Client  │
└─────────┬───────┘    └─────────┬───────┘
          └──────────────┬───────┘
                         │
               ┌─────────┴─────────┐
               │   API Gateway     │
               │     (Hertz)       │
               └─────────┬─────────┘
                         │
    ┌────────────┬───────┼────────────┬────────────┐
    │            │       │            │            │
 ┌──┴──┐     ┌───┴──┐  ┌─┴──┐    ┌────┴────┐  ┌────┴────┐
 │User │     │Video │  │Rel │    │Interact │  │  ...    │
 │ RPC │     │ RPC  │  │ RPC│    │   RPC   │  │         │
 └──┬──┘     └───┬──┘  └─┬──┘    └────┬────┘  └─────────┘
    │            │       │            │
    └────────────┼───────┼────────────┘
                 │       │
          ┌──────┴───────┴──────┐
          │  MySQL | Redis      │
          │  MinIO | Milvus     │
          │  Kafka | RabbitMQ   │
          │  etcd  | Jaeger     │
          │  Elasticsearch      │
          └─────────────────────┘
```

## 技术栈

- **RPC 框架**：[Kitex](https://github.com/cloudwego/kitex)
- **HTTP 框架**：[Hertz](https://github.com/cloudwego/hertz)
- **IDL**：Thrift
- **数据存储**：MySQL、Redis、MinIO、Milvus（向量）
- **中间件**：Kafka、RabbitMQ、etcd、Jaeger、Elasticsearch

## 项目结构

```
Refactored-TikTok/
├── cmd/                        # 微服务入口
│   ├── api/                   # HTTP 网关（:8888）
│   ├── user/                  # 用户 RPC（:8889）
│   ├── video/                 # 视频 RPC（:8891）
│   ├── relation/              # 关系 RPC（:8892）
│   └── interaction/           # 互动 RPC（:8893）
├── config/                     # 配置文件（含数据库初始化 SQL）
├── deploy/docker-compose.yml   # 基础设施编排
├── docs/                       # 项目文档（RUNBOOK）
├── idl/                        # Thrift IDL 定义
├── internal/                   # 内部通用模型
├── kitex_gen/                  # Kitex 生成代码
├── pkg/                        # 公共包（推荐、日志、安全、缓存等）
├── scripts/                    # 数据初始化与运维脚本
└── logs/                       # 运行时日志目录
```

## 快速开始

### 环境要求

- Go 1.24+
- Docker & Docker Compose

### 1. 启动基础设施

```bash
cd Refactored-TikTok
docker-compose -f deploy/docker-compose.yml up -d
```

容器映射端口：

| 容器 | 端口 |
|---|---|
| kitex_mysql | 3307 |
| kitex_redis | 6379 |
| etcd | 2379 |
| kafka | 9092 |
| minio | 9002 |
| milvus | 19530 |
| rabbitmq | 5672 |
| elasticsearch | 9200 |

### 2. 数据库初始化

```bash
make init-all-db        # 初始化主库 + 推荐库
```

### 3. 构建并启动微服务

```bash
make build              # 构建 5 个二进制

make users &            # 启动 4 个 RPC 服务
make videos &
make relations &
make interactions &
sleep 3
make api                # 最后启动 HTTP 网关
```

### 4. 验证

```bash
curl http://localhost:8888/ping
```

详细步骤（含 DeepCTR 推理服务、前端、推荐链路调试、故障排查）请参考
[`docs/RUNBOOK.md`](docs/RUNBOOK.md)。

## API 列表

### 用户
- `POST /douyin/user/register/` — 注册
- `POST /douyin/user/login/` — 登录
- `GET /douyin/user/` — 用户信息

### 视频
- `GET /douyin/feed/` — 视频流
- `POST /douyin/publish/action/` — 发布
- `GET /douyin/publish/list/` — 发布列表

### 互动
- `POST /douyin/favorite/action/` — 点赞 / 取消点赞
- `GET /douyin/favorite/list/` — 点赞列表
- `POST /douyin/comment/action/` — 评论 / 删除
- `GET /douyin/comment/list/` — 评论列表

### 关系
- `POST /douyin/relation/action/` — 关注 / 取关
- `GET /douyin/relation/follow/list/` — 关注列表
- `GET /douyin/relation/follower/list/` — 粉丝列表

### 推荐
- `GET /v1/recommend/video` — 个性化推荐流

## 微服务说明

| 服务 | 职责 |
|---|---|
| API Gateway | 统一入口、鉴权、路由、限流 |
| User | 注册登录、用户资料、JWT 签发校验 |
| Video | 视频上传、转码、Feed、推荐链路 |
| Interaction | 点赞、评论；事件驱动数据同步 |
| Relation | 关注 / 粉丝（分库分表） |

## 事件驱动架构

项目实现了事件驱动的数据同步机制：

1. **立即响应**：Redis 缓存提供快速用户反馈
2. **异步处理**：消息队列处理数据库写入
3. **幂等性保证**：防止重复消费
4. **事务一致性**：关键写入使用事务
5. **监控与重试**：完善的错误处理机制

## 技术亮点

1. **智能推荐系统**
   - 多路召回（协同过滤 / 热门 / 内容 / 社交 / 探索）
   - DeepCTR 精排（DeepFM）
   - MMR 重排序保证多样性
   - 详见：[`pkg/recommendation/README.md`](pkg/recommendation/README.md)

2. **分库分表架构**
   - 评论服务：4 库 × 4 表
   - 关系服务：4 库 × 4 表
   - 初始化脚本：[`scripts/init_relation_shard.sh`](scripts/init_relation_shard.sh)

3. **冷热数据分离**
   - 基于 MinIO 多层存储
   - 实现：[`pkg/oss/hot_storage_manager.go`](pkg/oss/hot_storage_manager.go)

## 未来规划

- [ ] 推荐算法升级（DIN / MMoE）
- [ ] Prometheus + Grafana 监控体系
- [ ] 视频 AI 审核
- [ ] WebSocket 实时消息
- [ ] A/B 测试框架
- [ ] 数据库读写分离
