# 分布式测试快速参考卡

## 🚀 一键命令

```bash
# 1. 启动完整测试环境 (基础设施 + 10个微服务实例)
./scripts/distributed_test.sh start

# 2. 查看所有服务状态
./scripts/distributed_test.sh status

# 3. 运行分布式事务测试
./scripts/distributed_test.sh test

# 4. 查看测试结果
cat test_results/test_report.txt

# 5. 停止所有服务
./scripts/distributed_test.sh stop
```

## 📊 服务实例分布

| 服务 | 实例数 | 端口 |
|------|--------|------|
| User Service | 2 | 8881, 8882 |
| Video Service | 2 | 8883, 8884 |
| Interaction Service | 2 | 8885, 8886 |
| Relation Service | 2 | 8887, 8888 |
| API Gateway | 1 | 8080 |

## 🧪 测试场景一览

1. ✅ **并发点赞** - 幂等性测试
2. ✅ **并发评论** - 事务一致性测试
3. ✅ **服务故障恢复** - 消息重试测试
4. ✅ **缓存DB一致性** - 最终一致性测试
5. ✅ **消息队列积压** - 高并发测试
6. ✅ **分库分表分布** - 数据均匀性测试

## 📝 常用监控命令

```bash
# 查看 RabbitMQ 队列
docker exec kitex_rabbitmq rabbitmqctl list_queues

# 查看 Redis 缓存
docker exec -it kitex_redis redis-cli -a 'Redis@TikTok2025_SecurePass'

# 查看 MySQL 数据
docker exec -it kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' -D TikTok

# 实时查看日志
tail -f logs/interaction/interaction-1.log
```

## 🔧 故障注入测试

```bash
# 1. 杀掉一个服务实例
kill $(cat logs/interaction/interaction-1.pid)

# 2. 观察负载均衡
tail -f logs/interaction/interaction-2.log

# 3. 重启服务
cd cmd/interaction
INTERACTION_SERVICE_PORT=8885 nohup ./interaction > ../../logs/interaction/interaction-1.log 2>&1 &
```

## 📈 测试后检查清单

- [ ] 所有测试是否通过？
- [ ] Redis 与 MySQL 数据是否一致？
- [ ] 消息队列是否有积压？
- [ ] 日志中是否有 ERROR？
- [ ] 服务恢复后数据是否完整？

## 🆘 快速故障排查

**问题**: 服务启动失败
```bash
# 检查端口占用
netstat -tuln | grep 8881
# 查看日志
tail -100 logs/user/user-1.log
```

**问题**: 测试失败
```bash
# 查看测试详情
cat test_results/test_report.txt
# 检查数据一致性
docker exec kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' -D TikTok -e "SELECT COUNT(*) FROM favorites;"
```

**问题**: 消息未消费
```bash
# 检查队列状态
docker exec kitex_rabbitmq rabbitmqctl list_queues
# 重启消费者服务
./scripts/distributed_test.sh restart
```

---

**详细文档**: [docs/DISTRIBUTED_TESTING_GUIDE.md](DISTRIBUTED_TESTING_GUIDE.md)
