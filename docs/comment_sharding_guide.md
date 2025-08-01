# Comment表分片改造说明

## 概述

本次改造将TikTok项目中的comment表从单库单表改造为分库分表架构，以支持千万级评论数据的高并发访问。

## 分片架构

- **分库数量**: 4个数据库 (comment_db_0, comment_db_1, comment_db_2, comment_db_3)
- **分表数量**: 每个数据库4张表 (comments_0, comments_1, comments_2, comments_3)
- **总表数量**: 16张表
- **分片策略**: 基于video_id进行CRC32哈希分片

## 配置说明

### 1. 配置文件修改

在 `config/config.yml` 中添加分片配置：

```yaml
# Comment sharding configuration
comment_sharding:
  database_count: 4
  table_count: 4
  max_open_conns: 100
  max_idle_conns: 10
  conn_max_lifetime: 3600s
  master_dsns:
    - "root:password@tcp(localhost:3307)/comment_db_0?charset=utf8mb4&parseTime=True&loc=Local"
    - "root:password@tcp(localhost:3307)/comment_db_1?charset=utf8mb4&parseTime=True&loc=Local"
    - "root:password@tcp(localhost:3307)/comment_db_2?charset=utf8mb4&parseTime=True&loc=Local"
    - "root:password@tcp(localhost:3307)/comment_db_3?charset=utf8mb4&parseTime=True&loc=Local"
  slave_dsns:
    - []  # No slaves for db_0
    - []  # No slaves for db_1
    - []  # No slaves for db_2
    - []  # No slaves for db_3
```

### 2. 数据库初始化

执行 `config/mysql/multi_db_init.sql` 脚本来创建分库分表结构：

```bash
mysql -u root -p < config/mysql/multi_db_init.sql
```

## 代码改造

### 1. 核心组件

- **ShardingManager**: 分片管理器，负责路由和连接管理
- **ShardedCommentDB**: 分片评论数据库操作类
- **兜底机制**: 当分片管理器未初始化时，自动回退到单库操作

### 2. 主要修改的函数

所有comment表相关的数据库操作函数都已适配分片：

- `CreateComment()` - 创建评论
- `CreateCommentWithTransaction()` - 事务创建评论
- `GetCommentInfo()` - 获取评论信息
- `GetVideoCommentList()` - 获取视频评论列表
- `GetVideoCommentCount()` - 获取视频评论数
- `DeleteComment()` - 删除评论
- `CheckCommentExists()` - 检查评论是否存在
- 等等...

### 3. 分片路由逻辑

```go
// 根据video_id计算分片信息
func (sm *ShardingManager) GetShardInfo(videoID int64) (dbIndex, tableIndex int) {
    hash := crc32.ChecksumIEEE([]byte(fmt.Sprintf("%d", videoID)))
    dbIndex = int(hash) % sm.config.DatabaseCount
    tableIndex = int(hash) % sm.config.TableCount
    return
}
```

## 使用方式

### 1. 启动应用

应用启动时会自动初始化分片管理器：

```go
func Init() {
    db.Init() // mysql init
    
    // 初始化分片管理器
    if err := initShardingManager(); err != nil {
        hlog.Errorf("Failed to initialize sharding manager: %v", err)
        // 不panic，允许系统继续运行使用单库模式
    }
}
```

### 2. 业务代码无需修改

所有现有的业务代码无需修改，分片逻辑对业务层透明：

```go
// 业务代码保持不变
comment := &model.Comment{
    VideoId: 12345,
    UserId:  1001,
    Content: "这是一条评论",
}

// 自动路由到正确的分片
err := db.CreateComment(ctx, comment)
```

## 性能优化

### 1. 连接池配置

每个分片都有独立的连接池，可根据实际负载调整：

- `max_open_conns`: 最大连接数
- `max_idle_conns`: 最大空闲连接数
- `conn_max_lifetime`: 连接最大生存时间

### 2. 读写分离

支持主从读写分离，读操作自动路由到从库：

```go
// 写操作使用主库
err := ShardingManager.ExecuteInShard(ctx, videoId, true, func(db *gorm.DB, tableName string) error {
    return db.Table(tableName).Create(comment).Error
})

// 读操作使用从库
err := ShardingManager.ExecuteInShard(ctx, videoId, false, func(db *gorm.DB, tableName string) error {
    return db.Table(tableName).Where("video_id = ?", videoId).Find(&comments).Error
})
```

## 注意事项

### 1. 跨分片查询

某些操作需要跨分片查询（如根据comment_id查找评论），性能可能较差：

```go
// 需要在所有分片中查找
func GetCommentByIdFromAllShards(ctx context.Context, commentId int64, withLock bool) (*model.Comment, error) {
    // 遍历所有分片查找评论
    for dbIndex := 0; dbIndex < 4; dbIndex++ {
        for tableIndex := 0; tableIndex < 4; tableIndex++ {
            // 查找逻辑...
        }
    }
}
```

建议：
- 使用全局索引表记录comment_id到分片的映射
- 或者在业务层缓存这种映射关系

### 2. 事务处理

分片后无法进行跨分片事务，需要在业务层处理数据一致性。

### 3. 数据迁移

从单库迁移到分库分表时，需要：
1. 停止写入
2. 导出现有数据
3. 按分片规则重新分布数据
4. 验证数据完整性
5. 切换到新架构

## 监控和运维

### 1. 健康检查

分片管理器提供健康检查功能：

```go
err := ShardingManager.HealthCheck(ctx)
if err != nil {
    // 处理健康检查失败
}
```

### 2. 连接监控

监控各分片的连接状态和性能指标。

### 3. 数据均衡

定期检查各分片的数据分布是否均匀。

## 测试

运行分片功能测试：

```bash
cd cmd/interaction/dal/db
go test -v -run TestSharding
```

## 兜底机制

当分片管理器初始化失败时，系统会自动回退到单库模式，确保服务可用性：

```go
if ShardingManager != nil {
    // 使用分片逻辑
    return ShardingManager.ExecuteInShard(ctx, videoId, true, func(db *gorm.DB, tableName string) error {
        return db.Table(tableName).Create(comment).Error
    })
}
// 兜底使用原有的单库操作
return DB.WithContext(ctx).Create(comment).Error
```

这确保了系统的高可用性和向后兼容性。