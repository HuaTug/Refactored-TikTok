#!/bin/bash

# Relation分库分表初始化脚本
# 基于Comment分库分表模式设计

set -e

echo "开始初始化Relation分库分表系统..."

# 数据库连接配置
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-3306}
DB_USER=${DB_USER:-root}
DB_PASS=${DB_PASS:-""}

# 创建分库
echo "创建4个分库..."
for i in {0..3}; do
    echo "创建分库 relation_db_$i..."
    mysql -h$DB_HOST -P$DB_PORT -u$DB_USER -p$DB_PASS -e "
        CREATE DATABASE IF NOT EXISTS relation_db_$i DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
    "
done

# 执行分库分表初始化SQL
echo "执行分库分表初始化SQL..."
mysql -h$DB_HOST -P$DB_PORT -u$DB_USER -p$DB_PASS < config/mysql/relation_shard_init.sql

# 验证初始化结果
echo "验证初始化结果..."
for i in {0..3}; do
    echo "验证分库 relation_db_$i..."
    mysql -h$DB_HOST -P$DB_PORT -u$DB_USER -p$DB_PASS -e "
        USE relation_db_$i;
        SHOW TABLES;
        SELECT 'follows_0' as table_name, COUNT(*) as row_count FROM follows_0
        UNION ALL
        SELECT 'follows_1' as table_name, COUNT(*) as row_count FROM follows_1
        UNION ALL
        SELECT 'follows_2' as table_name, COUNT(*) as row_count FROM follows_2
        UNION ALL
        SELECT 'follows_3' as table_name, COUNT(*) as row_count FROM follows_3;
    "
done

echo "Relation分库分表初始化完成！"
echo "总计创建了4个分库，16张分表"
echo "分片策略：按follower_id哈希分片"
echo "分片算法：follower_id % 4 确定分库，(follower_id / 4) % 4 确定分表"