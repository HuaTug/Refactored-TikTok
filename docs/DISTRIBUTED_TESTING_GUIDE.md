# 分布式事务测试指南

本文档说明如何在本地部署多个服务实例来测试分布式事务的稳定性。

## 📋 测试架构

```
┌─────────────────────────────────────────────────────────────┐
│                      本地测试环境                             │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  API Gateway (1实例)                                        │
│       ↓                                                     │
│  ┌────────────────────────────────────────────────┐         │
│  │  User Service      (2实例: 8881, 8882)        │         │
│  │  Video Service     (2实例: 8883, 8884)        │         │
│  │  Interaction Svc   (2实例: 8885, 8886)        │         │
│  │  Relation Service  (2实例: 8887, 8888)        │         │
│  └────────────────────────────────────────────────┘         │
│                      ↓                                      │
│  ┌────────────────────────────────────────────────┐         │
│  │  MySQL | Redis | RabbitMQ | etcd | MinIO      │         │
│  └────────────────────────────────────────────────┘         │
│                 (Docker 容器)                               │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 快速开始

### 1. 一键部署所有服务

```bash
# 赋予执行权限
chmod +x scripts/distributed_test.sh
chmod +x scripts/test_distributed_transaction.sh

# 启动所有服务 (基础设施 + 微服务集群)
./scripts/distributed_test.sh start
```

**部署内容**:
- ✅ MySQL (Docker)
- ✅ Redis (Docker)
- ✅ RabbitMQ (Docker)
- ✅ etcd (Docker)
- ✅ MinIO (Docker)
- ✅ User Service × 2
- ✅ Video Service × 2
- ✅ Interaction Service × 2
- ✅ Relation Service × 2
- ✅ API Gateway × 1

### 2. 查看服务状态

```bash
./scripts/distributed_test.sh status
```

输出示例:
```
基础设施:
NAMES              STATUS         PORTS
kitex_mysql        Up 2 minutes   0.0.0.0:3307->3306/tcp
kitex_redis        Up 2 minutes   0.0.0.0:6379->6379/tcp
etcd               Up 2 minutes   0.0.0.0:2379-2380->2379-2380/tcp
kitex_rabbitmq     Up 2 minutes   0.0.0.0:5672->5672/tcp, 0.0.0.0:15672->15672/tcp

微服务实例:
User Service:
  ✓ 实例 1 运行中 (PID: 12345, Port: 8881)
  ✓ 实例 2 运行中 (PID: 12346, Port: 8882)

Video Service:
  ✓ 实例 1 运行中 (PID: 12347, Port: 8883)
  ✓ 实例 2 运行中 (PID: 12348, Port: 8884)
...
```

### 3. 运行分布式事务测试

```bash
./scripts/distributed_test.sh test
```

## 🧪 测试场景

### 测试 1: 并发点赞 - 幂等性测试

**测试目标**: 验证同一个用户对同一个视频多次点赞，最终只生效一次

**测试步骤**:
1. 并发发送 20 个点赞请求
2. 等待事件处理完成
3. 验证 Redis 缓存和 MySQL 数据库中的点赞数

**预期结果**: 点赞数只增加 1

**验证点**:
- ✅ Redis 缓存幂等性
- ✅ 数据库幂等性记录
- ✅ 消息队列去重

---

### 测试 2: 并发评论 - 事务一致性测试

**测试目标**: 验证并发评论场景下的数据一致性

**测试步骤**:
1. 记录初始评论数
2. 并发发送 10 个评论请求
3. 等待异步处理完成
4. 验证最终评论数 = 初始数 + 10

**预期结果**: 评论数完全一致

**验证点**:
- ✅ 分库分表正确路由
- ✅ 事务原子性
- ✅ 数据不丢失

---

### 测试 3: 服务故障恢复 - 消息重试测试

**测试目标**: 验证服务宕机后的消息重试机制

**测试步骤**:
1. 发送点赞请求
2. 模拟 Interaction Service 故障 (kill 进程)
3. 等待 5 秒
4. 重启服务
5. 验证消息是否被重新处理

**预期结果**: 服务恢复后自动处理积压消息

**验证点**:
- ✅ 消息持久化
- ✅ 重试机制
- ✅ 最终一致性

---

### 测试 4: 缓存与数据库一致性

**测试目标**: 验证 Redis 缓存与 MySQL 数据库的最终一致性

**测试步骤**:
1. 执行点赞操作
2. 立即检查 Redis 缓存
3. 等待 10 秒 (异步写入)
4. 检查 MySQL 数据库
5. 对比两者数据

**预期结果**: Redis 和 MySQL 数据一致

**验证点**:
- ✅ 缓存先行策略
- ✅ 异步写入数据库
- ✅ 最终一致性

---

### 测试 5: 消息队列积压测试

**测试目标**: 验证高并发场景下的消息队列处理能力

**测试步骤**:
1. 突发发送 100 个请求
2. 检查 RabbitMQ 队列长度
3. 等待消息处理完成

**预期结果**: 消息队列能够正常处理，积压较少

**验证点**:
- ✅ 队列吞吐量
- ✅ 消费者处理速度
- ✅ 系统稳定性

---

### 测试 6: 分库分表数据分布

**测试目标**: 验证分库分表的数据分布均匀性

**测试步骤**:
1. 查询所有分片的数据量
2. 生成数据分布报告

**预期结果**: 数据相对均匀分布

**验证点**:
- ✅ 哈希分片算法
- ✅ 数据均匀性
- ✅ 无热点问题

## 📊 查看测试结果

测试完成后，结果保存在 `test_results/` 目录:

```bash
# 查看测试报告
cat test_results/test_report.txt

# 查看分片数据分布
cat test_results/sharding_distribution.txt
```

测试报告示例:
```
测试开始时间: 2025-12-28 10:00:00
测试 1: 并发点赞 - PASS
测试 2: 并发评论 - PASS
测试 3: 服务故障恢复 - PASS
测试 4: 缓存与数据库一致性 - PASS
测试 5: 消息队列积压 - WARN (积压: 15)
测试 6: 分库分表数据分布 - PASS

测试结束时间: 2025-12-28 10:15:00
=====================================
测试摘要:
通过: 5
警告: 1
失败: 0
=====================================
```

## 📝 查看日志

### 查看特定服务日志

```bash
# API Gateway
./scripts/distributed_test.sh logs api

# User Service 实例 1
tail -f logs/user/user-1.log

# Interaction Service 实例 2
tail -f logs/interaction/interaction-2.log

# 所有 Interaction Service 日志
tail -f logs/interaction/*.log
```

### 实时监控 RabbitMQ

访问管理界面: http://localhost:15672
- 用户名: guest
- 密码: guest

监控内容:
- 队列长度
- 消息速率
- 消费者状态

### 实时监控 Redis

```bash
# 连接 Redis
docker exec -it kitex_redis redis-cli -a 'Redis@TikTok2025_SecurePass'

# 监控命令
> MONITOR

# 查看特定 key
> GET video:like_count:2001
> SMEMBERS user:watch_history:1001
```

## 🔧 高级测试

### 手动触发故障测试

#### 1. 模拟网络延迟

```bash
# 对 MySQL 添加延迟 (需要 tc 工具)
sudo tc qdisc add dev eth0 root netem delay 100ms

# 恢复
sudo tc qdisc del dev eth0 root
```

#### 2. 模拟服务宕机

```bash
# 停止 Video Service 实例 1
kill $(cat logs/video/video-1.pid)

# 观察负载均衡到实例 2
tail -f logs/video/video-2.log

# 重启实例 1
cd cmd/video
VIDEO_SERVICE_PORT=8883 nohup ./video > ../../logs/video/video-1.log 2>&1 &
```

#### 3. 模拟数据库故障

```bash
# 暂停 MySQL 容器
docker pause kitex_mysql

# 观察服务行为 (应该有错误日志和重试)
tail -f logs/interaction/interaction-1.log

# 恢复 MySQL
docker unpause kitex_mysql
```

#### 4. 模拟 RabbitMQ 故障

```bash
# 停止 RabbitMQ
docker stop kitex_rabbitmq

# 发送请求 (应该返回成功，消息会在内存中)
curl -X POST http://localhost:8080/douyin/favorite/action/ \
  -H "Content-Type: application/json" \
  -d '{"video_id":2001,"action_type":1}'

# 重启 RabbitMQ
docker start kitex_rabbitmq

# 观察消息是否被重新发送
```

### 压力测试

使用 `ab` (Apache Bench) 或 `wrk` 进行压测:

```bash
# 安装 ab
sudo yum install httpd-tools  # CentOS/RHEL
sudo apt install apache2-utils # Ubuntu/Debian

# 压测点赞接口 (1000 请求, 并发 50)
ab -n 1000 -c 50 -p like.json -T application/json \
  http://localhost:8080/douyin/favorite/action/

# like.json 内容:
# {"video_id":2001,"action_type":1}
```

使用 `wrk`:

```bash
# 安装 wrk
git clone https://github.com/wg/wrk.git
cd wrk && make && sudo cp wrk /usr/local/bin/

# 压测 (持续 30 秒, 10 个线程, 100 个连接)
wrk -t10 -c100 -d30s --latency \
  -s post.lua http://localhost:8080/douyin/favorite/action/

# post.lua:
# wrk.method = "POST"
# wrk.body = '{"video_id":2001,"action_type":1}'
# wrk.headers["Content-Type"] = "application/json"
```

## 🛠️ 常用命令

```bash
# 启动所有服务
./scripts/distributed_test.sh start

# 查看状态
./scripts/distributed_test.sh status

# 停止所有服务
./scripts/distributed_test.sh stop

# 重启所有服务
./scripts/distributed_test.sh restart

# 运行测试
./scripts/distributed_test.sh test

# 查看日志
./scripts/distributed_test.sh logs <service>
```

## 📈 性能指标

在本地测试环境中，预期性能指标:

| 指标 | 预期值 | 说明 |
|------|--------|------|
| API 响应时间 | < 100ms | P99 |
| 消息处理延迟 | < 5s | 异步写入数据库 |
| 并发点赞 QPS | > 500 | 单机 |
| 并发评论 QPS | > 200 | 单机 |
| Redis 命中率 | > 90% | 热点数据 |
| 消息丢失率 | 0% | 持久化保证 |

## 🐛 故障排查

### 问题 1: 服务启动失败

```bash
# 检查端口占用
netstat -tuln | grep 8881

# 查看服务日志
tail -f logs/user/user-1.log

# 检查依赖服务
docker ps
```

### 问题 2: 消息未被消费

```bash
# 检查 RabbitMQ 队列
docker exec kitex_rabbitmq rabbitmqctl list_queues

# 检查消费者连接
docker exec kitex_rabbitmq rabbitmqctl list_consumers

# 查看服务日志
tail -f logs/interaction/interaction-1.log
```

### 问题 3: 数据不一致

```bash
# 检查 Redis
docker exec -it kitex_redis redis-cli -a 'Redis@TikTok2025_SecurePass'
> GET video:like_count:2001

# 检查 MySQL
docker exec -it kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' -D TikTok
> SELECT COUNT(*) FROM favorites WHERE video_id=2001;

# 检查事件表
> SELECT * FROM sync_events WHERE status='failed' LIMIT 10;
```

## 🔐 安全注意事项

⚠️ **本测试环境仅用于本地开发和测试，不要在生产环境使用！**

测试环境特点:
- 所有密码都是明文配置
- 没有 TLS/SSL 加密
- 没有防火墙限制
- 日志包含敏感信息

## 📚 相关文档

- [事件驱动架构说明](EVENT_DRIVEN_SYNC.md)
- [分库分表设计](../scripts/init_relation_shard.sh)
- [限流熔断配置](../pkg/security/advanced_rate_limiter.go)
- [项目亮点总结](PROJECT_HIGHLIGHTS_SUMMARY.md)

## 💡 最佳实践

1. **定期测试**: 每次代码变更后运行完整测试
2. **监控日志**: 实时查看服务日志，及时发现问题
3. **数据清理**: 测试后清理测试数据
4. **性能基线**: 记录正常情况下的性能指标
5. **故障演练**: 定期进行故障注入测试

## 🆘 获取帮助

遇到问题？
1. 查看服务日志: `tail -f logs/<service>/<service>-1.log`
2. 检查测试报告: `cat test_results/test_report.txt`
3. 查看本文档的故障排查章节
4. 提交 Issue 到项目仓库

---

**祝测试顺利！** 🎉
