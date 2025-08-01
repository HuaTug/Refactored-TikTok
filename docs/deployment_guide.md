# 千万级评论系统部署指南

## 系统架构概述

本系统采用分库分表 + 缓存 + 消息队列的架构，支持千万级评论数据和高并发访问。

### 核心组件

1. **分库分表层**：8个数据库，每个数据库16张表，支持按视频ID分片
2. **缓存层**：Redis集群，支持热点数据缓存和计数器
3. **消息队列**：RabbitMQ，支持异步处理和削峰填谷
4. **应用层**：Go服务，支持高并发读写

## 部署架构

```
┌─────────────────────────────────────────────────────────────┐
│                        负载均衡层                              │
│                    (Nginx/HAProxy)                          │
└─────────────────────────────────────────────────────────────┘
                                │
┌─────────────────────────────────────────────────────────────┐
│                        应用服务层                              │
│              (Comment Service Cluster)                      │
│                     3-5个实例                                │
└─────────────────────────────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        │                       │                       │
┌───────▼────────┐    ┌────────▼────────┐    ┌────────▼────────┐
│   Redis集群     │    │   RabbitMQ集群   │    │   MySQL分库分表   │
│   (缓存层)      │    │   (消息队列)     │    │   (存储层)       │
│                │    │                 │    │                │
│ Master + 2Slave│    │ 3节点集群        │    │ 8主库 + 16从库   │
└────────────────┘    └─────────────────┘    └─────────────────┘
```

## 环境要求

### 硬件要求

**应用服务器**（3-5台）：
- CPU: 8核心以上
- 内存: 16GB以上
- 磁盘: SSD 100GB以上
- 网络: 千兆网卡

**数据库服务器**（24台：8主+16从）：
- CPU: 16核心以上
- 内存: 32GB以上
- 磁盘: SSD 500GB以上（主库），SSD 300GB以上（从库）
- 网络: 千兆网卡

**Redis服务器**（3台）：
- CPU: 8核心以上
- 内存: 32GB以上
- 磁盘: SSD 200GB以上
- 网络: 千兆网卡

**RabbitMQ服务器**（3台）：
- CPU: 4核心以上
- 内存: 8GB以上
- 磁盘: SSD 100GB以上
- 网络: 千兆网卡

### 软件要求

- **操作系统**: CentOS 7.x / Ubuntu 18.04+
- **MySQL**: 8.0+
- **Redis**: 6.0+
- **RabbitMQ**: 3.8+
- **Go**: 1.19+
- **Docker**: 20.10+ (可选)
- **Kubernetes**: 1.20+ (可选)

## 部署步骤

### 1. 数据库部署

#### 1.1 MySQL主库部署

```bash
# 在8台主库服务器上分别执行
# 服务器1: comment_db_0
# 服务器2: comment_db_1
# ...
# 服务器8: comment_db_7

# 安装MySQL 8.0
sudo apt update
sudo apt install mysql-server-8.0

# 配置MySQL
sudo mysql_secure_installation

# 创建数据库
mysql -u root -p
CREATE DATABASE comment_db_0 CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'comment_user'@'%' IDENTIFIED BY 'SecurePassword123!';
GRANT ALL PRIVILEGES ON comment_db_0.* TO 'comment_user'@'%';
FLUSH PRIVILEGES;

# 导入表结构
mysql -u comment_user -p comment_db_0 < /path/to/comment_system_init.sql
```

#### 1.2 MySQL从库部署

```bash
# 在16台从库服务器上分别执行
# 每个主库对应2个从库

# 配置主从复制
# 在主库上：
CHANGE MASTER TO
  MASTER_HOST='slave-server-ip',
  MASTER_USER='replication_user',
  MASTER_PASSWORD='ReplicationPassword123!',
  MASTER_LOG_FILE='mysql-bin.000001',
  MASTER_LOG_POS=0;

START SLAVE;
SHOW SLAVE STATUS\G;
```

#### 1.3 MySQL配置优化

```ini
# /etc/mysql/mysql.conf.d/mysqld.cnf

[mysqld]
# 基础配置
server-id = 1
log-bin = mysql-bin
binlog-format = ROW
gtid-mode = ON
enforce-gtid-consistency = ON

# 性能优化
innodb_buffer_pool_size = 24G  # 设置为内存的75%
innodb_log_file_size = 1G
innodb_log_buffer_size = 64M
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT

# 连接配置
max_connections = 2000
max_connect_errors = 1000000
connect_timeout = 60
wait_timeout = 28800

# 查询缓存
query_cache_type = 0
query_cache_size = 0

# 慢查询日志
slow_query_log = 1
slow_query_log_file = /var/log/mysql/slow.log
long_query_time = 1

# 分区表支持
partition = ON
```

### 2. Redis集群部署

#### 2.1 Redis主库部署

```bash
# 在Redis主库服务器上执行
sudo apt install redis-server

# 配置Redis
sudo vim /etc/redis/redis.conf
```

```conf
# /etc/redis/redis.conf
bind 0.0.0.0
port 6379
protected-mode no

# 内存配置
maxmemory 24gb
maxmemory-policy allkeys-lru

# 持久化配置
save 900 1
save 300 10
save 60 10000
appendonly yes
appendfsync everysec

# 性能配置
tcp-keepalive 300
timeout 0
tcp-backlog 511

# 安全配置
requirepass Redis@TikTok2025_SecurePass

# 主从复制配置（从库）
# slaveof master-ip 6379
# masterauth Redis@TikTok2025_SecurePass
```

#### 2.2 Redis从库部署

```bash
# 在Redis从库服务器上执行
# 配置从库复制
redis-cli -h slave-server -p 6379 -a Redis@TikTok2025_SecurePass
SLAVEOF master-ip 6379
CONFIG SET masterauth Redis@TikTok2025_SecurePass
CONFIG REWRITE
```

### 3. RabbitMQ集群部署

#### 3.1 RabbitMQ安装

```bash
# 在3台RabbitMQ服务器上分别执行
sudo apt update
sudo apt install rabbitmq-server

# 启用管理插件
sudo rabbitmq-plugins enable rabbitmq_management

# 创建管理用户
sudo rabbitmqctl add_user admin SecurePassword123!
sudo rabbitmqctl set_user_tags admin administrator
sudo rabbitmqctl set_permissions -p / admin ".*" ".*" ".*"
```

#### 3.2 RabbitMQ集群配置

```bash
# 在节点2和节点3上执行
sudo rabbitmqctl stop_app
sudo rabbitmqctl reset
sudo rabbitmqctl join_cluster rabbit@node1
sudo rabbitmqctl start_app

# 设置高可用策略
sudo rabbitmqctl set_policy ha-all ".*" '{"ha-mode":"all","ha-sync-mode":"automatic"}'
```

### 4. 应用服务部署

#### 4.1 编译应用

```bash
# 在开发机器上编译
cd /path/to/Refactored-TikTok
go mod tidy
go build -o comment-service cmd/interaction/main.go

# 构建Docker镜像（可选）
docker build -t comment-service:latest .
```

#### 4.2 配置文件

```yaml
# config/production.yaml
sharding:
  database_count: 8
  table_count: 16
  master_dsns:
    - "comment_user:SecurePassword123!@tcp(master-db-0:3306)/comment_db_0?charset=utf8mb4&parseTime=True&loc=Local"
    - "comment_user:SecurePassword123!@tcp(master-db-1:3306)/comment_db_1?charset=utf8mb4&parseTime=True&loc=Local"
    # ... 其他6个主库配置
  slave_dsns:
    - # 第0个分库的从库
      - "comment_user:SecurePassword123!@tcp(slave-db-0-0:3306)/comment_db_0?charset=utf8mb4&parseTime=True&loc=Local"
      - "comment_user:SecurePassword123!@tcp(slave-db-0-1:3306)/comment_db_0?charset=utf8mb4&parseTime=True&loc=Local"
    # ... 其他从库配置

redis:
  master:
    addr: "redis-master:6379"
    password: "Redis@TikTok2025_SecurePass"
    db: 0
  slaves:
    - addr: "redis-slave-0:6379"
      password: "Redis@TikTok2025_SecurePass"
    - addr: "redis-slave-1:6379"
      password: "Redis@TikTok2025_SecurePass"

rabbitmq:
  url: "amqp://admin:SecurePassword123!@rabbitmq-cluster:5672/"
```

#### 4.3 服务部署

```bash
# 方式1: 直接部署
./comment-service -config config/production.yaml

# 方式2: 使用systemd
sudo vim /etc/systemd/system/comment-service.service
```

```ini
[Unit]
Description=Comment Service
After=network.target

[Service]
Type=simple
User=comment
WorkingDirectory=/opt/comment-service
ExecStart=/opt/comment-service/comment-service -config /opt/comment-service/config/production.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable comment-service
sudo systemctl start comment-service
```

#### 4.4 Docker部署（可选）

```dockerfile
# Dockerfile
FROM golang:1.19-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o comment-service cmd/interaction/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/comment-service .
COPY --from=builder /app/config ./config
CMD ["./comment-service", "-config", "config/production.yaml"]
```

```bash
# 构建和运行
docker build -t comment-service:latest .
docker run -d --name comment-service \
  -p 8080:8080 \
  -v /path/to/config:/root/config \
  comment-service:latest
```

### 5. 负载均衡配置

#### 5.1 Nginx配置

```nginx
# /etc/nginx/sites-available/comment-service
upstream comment_backend {
    server app-server-1:8080 weight=1 max_fails=3 fail_timeout=30s;
    server app-server-2:8080 weight=1 max_fails=3 fail_timeout=30s;
    server app-server-3:8080 weight=1 max_fails=3 fail_timeout=30s;
    server app-server-4:8080 weight=1 max_fails=3 fail_timeout=30s;
    server app-server-5:8080 weight=1 max_fails=3 fail_timeout=30s;
}

server {
    listen 80;
    server_name comment-api.example.com;

    location / {
        proxy_pass http://comment_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时配置
        proxy_connect_timeout 5s;
        proxy_send_timeout 10s;
        proxy_read_timeout 10s;
        
        # 缓冲配置
        proxy_buffering on;
        proxy_buffer_size 4k;
        proxy_buffers 8 4k;
    }
    
    # 健康检查
    location /health {
        proxy_pass http://comment_backend/health;
        access_log off;
    }
}
```

## 监控和运维

### 1. 监控指标

#### 1.1 应用指标
- QPS（每秒查询数）
- 响应时间（P95, P99）
- 错误率
- 内存使用率
- CPU使用率
- 连接池状态

#### 1.2 数据库指标
- 连接数
- 慢查询数量
- 锁等待时间
- 主从延迟
- 磁盘使用率
- IOPS

#### 1.3 缓存指标
- 命中率
- 内存使用率
- 连接数
- 操作延迟

#### 1.4 消息队列指标
- 队列长度
- 消息处理速度
- 错误消息数量
- 连接状态

### 2. 日志管理

```bash
# 应用日志
tail -f /var/log/comment-service/app.log

# 数据库慢查询日志
tail -f /var/log/mysql/slow.log

# Redis日志
tail -f /var/log/redis/redis-server.log

# RabbitMQ日志
tail -f /var/log/rabbitmq/rabbit@hostname.log
```

### 3. 备份策略

#### 3.1 数据库备份

```bash
#!/bin/bash
# 每日备份脚本
DATE=$(date +%Y%m%d)
BACKUP_DIR="/backup/mysql/$DATE"
mkdir -p $BACKUP_DIR

# 备份每个分库
for i in {0..7}; do
    mysqldump -u backup_user -p comment_db_$i > $BACKUP_DIR/comment_db_$i.sql
    gzip $BACKUP_DIR/comment_db_$i.sql
done

# 删除7天前的备份
find /backup/mysql -type d -mtime +7 -exec rm -rf {} \;
```

#### 3.2 Redis备份

```bash
#!/bin/bash
# Redis备份脚本
DATE=$(date +%Y%m%d)
BACKUP_DIR="/backup/redis/$DATE"
mkdir -p $BACKUP_DIR

# 备份RDB文件
cp /var/lib/redis/dump.rdb $BACKUP_DIR/
gzip $BACKUP_DIR/dump.rdb

# 删除7天前的备份
find /backup/redis -type d -mtime +7 -exec rm -rf {} \;
```

### 4. 故障处理

#### 4.1 数据库故障

```bash
# 主库故障切换
# 1. 停止应用写入
# 2. 提升从库为主库
mysql -u root -p
STOP SLAVE;
RESET SLAVE ALL;

# 3. 更新应用配置
# 4. 重启应用服务
```

#### 4.2 缓存故障

```bash
# Redis故障处理
# 1. 检查Redis状态
redis-cli -h redis-server -p 6379 ping

# 2. 如果主库故障，切换到从库
# 3. 清理缓存，重新预热
redis-cli -h redis-server -p 6379 FLUSHALL
```

#### 4.3 消息队列故障

```bash
# RabbitMQ故障处理
# 1. 检查集群状态
sudo rabbitmqctl cluster_status

# 2. 重启故障节点
sudo systemctl restart rabbitmq-server

# 3. 重新加入集群
sudo rabbitmqctl join_cluster rabbit@node1
```

## 性能优化建议

### 1. 数据库优化

- 定期分析和优化表结构
- 监控慢查询并优化索引
- 合理设置连接池大小
- 定期清理过期数据

### 2. 缓存优化

- 合理设置缓存过期时间
- 使用缓存预热策略
- 监控缓存命中率
- 避免缓存雪崩

### 3. 应用优化

- 使用连接池
- 实现熔断机制
- 合理设置超时时间
- 使用批量操作

### 4. 网络优化

- 使用CDN加速
- 启用Gzip压缩
- 优化TCP参数
- 使用HTTP/2

## 扩容方案

### 1. 水平扩容

- 增加应用服务器实例
- 增加数据库从库
- 增加Redis从库
- 增加RabbitMQ节点

### 2. 垂直扩容

- 升级服务器硬件配置
- 增加内存和CPU
- 使用更快的SSD存储
- 升级网络带宽

### 3. 分片扩容

- 增加数据库分片数量
- 重新分布数据
- 更新分片路由规则
- 数据迁移

这个部署指南提供了完整的千万级评论系统部署方案，包括硬件要求、软件配置、监控运维等各个方面。根据实际情况可以进行相应的调整和优化。