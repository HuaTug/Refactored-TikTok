# 事件驱动同步机制说明

本系统采用“缓存优先 + 异步落库”的事件驱动架构，目标是在高并发场景下兼顾用户体验与数据最终一致性。

## 总体流程
1. 前端请求到达 API Gateway（Hertz），进行鉴权、限流。
2. 写路径：
   - 先更新缓存（Redis）或写入用户可见状态（如点赞数自增）。
   - 生成业务事件（如 `LikeCreated`），投递到 MQ（RabbitMQ）。
   - 返回快速响应（避免阻塞在数据库写入）。
3. 消费者（各服务的 Consumer）订阅事件，执行：
   - 幂等校验（基于事件 `event_id` 或业务幂等键）。
   - 事务落库（MySQL，必要时分布式事务/Outbox）。
   - 失败重试与死信队列（DLQ）处理。
4. 读路径：
   - 优先读取缓存；缓存失效或穿透时回源数据库并回填缓存。

## 关键设计
- 幂等性：
  - 对每个事件生成全局唯一 ID（雪花 ID / UUID）。
  - 在消费侧维护处理日志或去重表，确保消费“至多一次/至少一次”策略下的幂等。
- 事务一致性：
  - 数据库更新与事件发布可采用 Outbox/事务消息（本地事务 + 事件轮询发布）。
- 失败处理：
  - 指数退避重试；超过阈值进入 DLQ，异步告警与人工干预。
- 监控追踪：
  - 事件链路全程打点，结合 Jaeger 进行端到端追踪和耗时分析。

## 参考实现位置
- MQ 接入：`pkg/mq/`（如存在）与各服务 `consumer/` 目录。
- Redis 缓存：`config/cache/` 与 `pkg/cache/`。
- 追踪：`config/jaeger/`。
- Sentinel 流控：`config/sentinels/`。

## 典型时序（点赞）
```
Client -> API(Hertz): POST /favorite/action
API -> Redis: INCR like_count
API -> MQ: Publish LikeCreated(event_id, user_id, video_id)
Consumer -> MySQL: INSERT/UPSERT likes
Consumer -> Redis: 校正计数或失效相关 key
```

## 常见问题
- 计数不一致：以 DB 为真源，定期对账（校正 Redis）。
- 热点 Key：对热点视频/用户使用分片计数或 Lua 原子脚本。
- 缓存穿透/击穿/雪崩：布隆过滤器 + 多级缓存 + 随机过期时间。
