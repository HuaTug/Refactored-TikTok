# Relation分库分表使用指南

## 概述

本指南介绍了如何将Relation模块从单表设计迁移到分库分表架构，基于Comment系统的分库分表模式设计。

## 架构设计

### 分片策略
- **分库数量**: 4个数据库
- **分表数量**: 每个库4张表，共16张表
- **分片键**: `follower_id` (关注者ID)
- **分片算法**: 哈希分片
  - 分库索引: `follower_id % 4`
  - 分表索引: `(follower_id / 4) % 4`

### 数据库结构
```
relation_db_0/
├── follows_0
├── follows_1
├── follows_2
├── follows_3
└── user_relation_stats

relation_db_1/
├── follows_0
├── follows_1
├── follows_2
├── follows_3
└── user_relation_stats

relation_db_2/
├── follows_0
├── follows_1
├── follows_2
├── follows_3
└── user_relation_stats

relation_db_3/
├── follows_0
├── follows_1
├── follows_2
├── follows_3
└── user_relation_stats
```

## 表结构优化

### 关注关系表 (follows_x)
```sql
CREATE TABLE `follows_x` (
    `id` bigint NOT NULL AUTO_INCREMENT COMMENT '自增主键',
    `user_id` bigint NOT NULL COMMENT '被关注者ID',
    `follower_id` bigint NOT NULL COMMENT '关注者ID',
    `status` tinyint DEFAULT 1 COMMENT '1:正常关注 2:特别关注 3:悄悄关注',
    `remark` varchar(100) DEFAULT '' COMMENT '备注信息',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_follower` (`user_id`, `follower_id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_follower_id` (`follower_id`),
    KEY `idx_status` (`status`),
    KEY `idx_created_at` (`created_at`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 用户关系统计表 (user_relation_stats)
```sql
CREATE TABLE `user_relation_stats` (
    `user_id` bigint NOT NULL COMMENT '用户ID',
    `following_count` int NOT NULL DEFAULT 0 COMMENT '关注数量',
    `follower_count` int NOT NULL DEFAULT 0 COMMENT '粉丝数量',
    `friend_count` int NOT NULL DEFAULT 0 COMMENT '好友数量（互相关注）',
    `mutual_follow_count` int NOT NULL DEFAULT 0 COMMENT '互关数量',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`),
    KEY `idx_following_count` (`following_count`),
    KEY `idx_follower_count` (`follower_count`),
    KEY `idx_friend_count` (`friend_count`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

## 使用方法

### 1. 初始化分库分表

```bash
# 执行初始化脚本
chmod +x scripts/init_relation_shard.sh
./scripts/init_relation_shard.sh

# 或者手动执行SQL
mysql -u root -p < config/mysql/relation_shard_init.sql
```

### 2. 代码集成

#### 基本使用
```go
import "cmd/relation/dal"

// 创建DAO实例
followDAO := dal.NewShardFollowDAO()

// 插入关注关系
relation := &dal.FollowRelation{
    UserID:     123,      // 被关注者
    FollowerID: 456,      // 关注者
    Status:     1,        // 正常关注
    Remark:     "好友",
    CreatedAt:  time.Now(),
    UpdatedAt:  time.Now(),
}
err := followDAO.InsertFollow(relation)

// 获取关注列表
followingList, err := followDAO.GetFollowingList(456, 0, 20)

// 获取粉丝列表
followerList, err := followDAO.GetFollowerList(123, 0, 20)

// 获取互关列表
mutualList, err := followDAO.GetMutualFollowList(456, 0, 20)
```

#### 分片路由
```go
shardManager := dal.NewRelationShardManager()
dbIndex, tableIndex, tableName := shardManager.GetShardInfo(userID)
// dbIndex: 0-3 (分库索引)
// tableIndex: 0-3 (分表索引)
// tableName: "follows_0" - "follows_3"
```

### 3. 数据迁移

#### 从旧表迁移数据
```sql
-- 使用存储过程迁移数据
CALL MigrateRelationData();
```

#### 验证迁移结果
```sql
-- 检查每个分库的数据分布
SELECT 'relation_db_0' as db_name, COUNT(*) as total_records FROM relation_db_0.follows_0
UNION ALL
SELECT 'relation_db_1' as db_name, COUNT(*) as total_records FROM relation_db_1.follows_0
UNION ALL
SELECT 'relation_db_2' as db_name, COUNT(*) as total_records FROM relation_db_2.follows_0
UNION ALL
SELECT 'relation_db_3' as db_name, COUNT(*) as total_records FROM relation_db_3.follows_0;
```

## 性能优化

### 1. 查询优化
- **关注列表**: 直接定位到单个分表查询
- **粉丝列表**: 需要跨所有分表查询
- **互关列表**: 先查关注列表，再验证互关

### 2. 批量操作
```go
// 批量插入
relations := []*dal.FollowRelation{relation1, relation2, relation3}
err := followDAO.BatchInsertFollows(relations)
```

### 3. 缓存策略
- Redis缓存用户关注/粉丝数量
- 缓存热点用户的关注列表
- 使用布隆过滤器减少无效查询

## 监控和维护

### 1. 数据分布监控
```sql
-- 查看数据分布
SELECT 
    db_name,
    table_name,
    COUNT(*) as record_count
FROM (
    SELECT 'relation_db_0' as db_name, 'follows_0' as table_name, COUNT(*) as cnt FROM relation_db_0.follows_0
    UNION ALL SELECT 'relation_db_0', 'follows_1', COUNT(*) FROM relation_db_0.follows_1
    UNION ALL SELECT 'relation_db_0', 'follows_2', COUNT(*) FROM relation_db_0.follows_2
    UNION ALL SELECT 'relation_db_0', 'follows_3', COUNT(*) FROM relation_db_0.follows_3
    UNION ALL SELECT 'relation_db_1', 'follows_0', COUNT(*) FROM relation_db_1.follows_0
    -- ... 其他分库分表
) AS distribution
ORDER BY db_name, table_name;
```

### 2. 性能监控
- 监控每个分库的QPS
- 监控慢查询
- 监控数据倾斜

## 故障处理

### 1. 数据倾斜处理
如果发现某个分片数据过多：
```sql
-- 调整分片策略
UPDATE relation_shard_config 
SET shard_algorithm = 'consistent_hash' 
WHERE shard_key = 'relation';
```

### 2. 扩容方案
当需要增加分库时：
1. 创建新的分库
2. 修改分片配置
3. 重新分布数据
4. 更新路由代码

## 注意事项

1. **事务处理**: 跨分库操作需要分布式事务
2. **分页查询**: 粉丝列表需要聚合所有分表结果
3. **数据一致性**: 使用最终一致性模型
4. **备份策略**: 每个分库独立备份
5. **监控告警**: 设置数据倾斜和性能告警

## 与Comment系统的对比

| 特性 | Comment系统 | Relation系统 |
|------|-------------|--------------|
| 分片键 | comment_id | follower_id |
| 查询模式 | 按视频聚合 | 按用户聚合 |
| 跨表查询 | 较少 | 粉丝列表需要 |
| 数据倾斜 | 均匀分布 | 大V用户可能倾斜 |
| 缓存策略 | 视频维度 | 用户维度 |

通过以上设计，Relation系统具备了与Comment系统相同的分库分表能力，同时针对社交关系的特点进行了优化。