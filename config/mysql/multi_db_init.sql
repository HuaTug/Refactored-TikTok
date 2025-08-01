-- 千万级评论系统真正的分库分表初始化脚本
-- 创建4个分库，每个分库4张分表，总共16张表

-- ========================================
-- 创建分库 comment_db_0
-- ========================================
CREATE DATABASE IF NOT EXISTS comment_db_0 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE comment_db_0;

-- 创建分表 comments_0 到 comments_3
CREATE TABLE IF NOT EXISTS `comments_0` (
    `comment_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `video_id` bigint NOT NULL,
    `parent_id` bigint NOT NULL DEFAULT -1,
    `like_count` bigint NOT NULL DEFAULT 0,
    `child_count` bigint NOT NULL DEFAULT 0,
    `content` text NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    `reply_to_comment_id` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`comment_id`),
    KEY `idx_video_id` (`video_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_parent_id` (`parent_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE,
    KEY `idx_video_created` (`video_id`, `created_at`) USING BTREE,
    KEY `idx_video_like_count` (`video_id`, `like_count`) USING BTREE,
    KEY `idx_deleted_at` (`deleted_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY HASH(comment_id) PARTITIONS 4;

CREATE TABLE IF NOT EXISTS `comments_1` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_2` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_3` LIKE `comments_0`;

-- 创建评论点赞表（每个分库都有）
CREATE TABLE IF NOT EXISTS `comment_likes` (
    `comment_likes_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `comment_id` bigint NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    PRIMARY KEY (`comment_likes_id`),
    UNIQUE KEY `uk_user_comment` (`user_id`, `comment_id`),
    KEY `idx_comment_id` (`comment_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ========================================
-- 创建分库 comment_db_1
-- ========================================
CREATE DATABASE IF NOT EXISTS comment_db_1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE comment_db_1;

-- 创建分表 comments_0 到 comments_3
CREATE TABLE IF NOT EXISTS `comments_0` (
    `comment_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `video_id` bigint NOT NULL,
    `parent_id` bigint NOT NULL DEFAULT -1,
    `like_count` bigint NOT NULL DEFAULT 0,
    `child_count` bigint NOT NULL DEFAULT 0,
    `content` text NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    `reply_to_comment_id` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`comment_id`),
    KEY `idx_video_id` (`video_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_parent_id` (`parent_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE,
    KEY `idx_video_created` (`video_id`, `created_at`) USING BTREE,
    KEY `idx_video_like_count` (`video_id`, `like_count`) USING BTREE,
    KEY `idx_deleted_at` (`deleted_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY HASH(comment_id) PARTITIONS 4;

CREATE TABLE IF NOT EXISTS `comments_1` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_2` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_3` LIKE `comments_0`;

-- 创建评论点赞表
CREATE TABLE IF NOT EXISTS `comment_likes` (
    `comment_likes_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `comment_id` bigint NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    PRIMARY KEY (`comment_likes_id`),
    UNIQUE KEY `uk_user_comment` (`user_id`, `comment_id`),
    KEY `idx_comment_id` (`comment_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ========================================
-- 创建分库 comment_db_2
-- ========================================
CREATE DATABASE IF NOT EXISTS comment_db_2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE comment_db_2;

-- 创建分表 comments_0 到 comments_3
CREATE TABLE IF NOT EXISTS `comments_0` (
    `comment_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `video_id` bigint NOT NULL,
    `parent_id` bigint NOT NULL DEFAULT -1,
    `like_count` bigint NOT NULL DEFAULT 0,
    `child_count` bigint NOT NULL DEFAULT 0,
    `content` text NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    `reply_to_comment_id` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`comment_id`),
    KEY `idx_video_id` (`video_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_parent_id` (`parent_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE,
    KEY `idx_video_created` (`video_id`, `created_at`) USING BTREE,
    KEY `idx_video_like_count` (`video_id`, `like_count`) USING BTREE,
    KEY `idx_deleted_at` (`deleted_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY HASH(comment_id) PARTITIONS 4;

CREATE TABLE IF NOT EXISTS `comments_1` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_2` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_3` LIKE `comments_0`;

-- 创建评论点赞表
CREATE TABLE IF NOT EXISTS `comment_likes` (
    `comment_likes_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `comment_id` bigint NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    PRIMARY KEY (`comment_likes_id`),
    UNIQUE KEY `uk_user_comment` (`user_id`, `comment_id`),
    KEY `idx_comment_id` (`comment_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ========================================
-- 创建分库 comment_db_3
-- ========================================
CREATE DATABASE IF NOT EXISTS comment_db_3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE comment_db_3;

-- 创建分表 comments_0 到 comments_3
CREATE TABLE IF NOT EXISTS `comments_0` (
    `comment_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `video_id` bigint NOT NULL,
    `parent_id` bigint NOT NULL DEFAULT -1,
    `like_count` bigint NOT NULL DEFAULT 0,
    `child_count` bigint NOT NULL DEFAULT 0,
    `content` text NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    `reply_to_comment_id` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`comment_id`),
    KEY `idx_video_id` (`video_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_parent_id` (`parent_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE,
    KEY `idx_video_created` (`video_id`, `created_at`) USING BTREE,
    KEY `idx_video_like_count` (`video_id`, `like_count`) USING BTREE,
    KEY `idx_deleted_at` (`deleted_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
PARTITION BY HASH(comment_id) PARTITIONS 4;

CREATE TABLE IF NOT EXISTS `comments_1` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_2` LIKE `comments_0`;
CREATE TABLE IF NOT EXISTS `comments_3` LIKE `comments_0`;

-- 创建评论点赞表
CREATE TABLE IF NOT EXISTS `comment_likes` (
    `comment_likes_id` bigint NOT NULL,
    `user_id` bigint NOT NULL,
    `comment_id` bigint NOT NULL,
    `created_at` varchar(255) NOT NULL,
    `deleted_at` varchar(255) DEFAULT '',
    PRIMARY KEY (`comment_likes_id`),
    UNIQUE KEY `uk_user_comment` (`user_id`, `comment_id`),
    KEY `idx_comment_id` (`comment_id`) USING BTREE,
    KEY `idx_user_id` (`user_id`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ========================================
-- 回到主库创建全局管理表
-- ========================================
USE TikTok;

-- 创建分库分表路由配置表
CREATE TABLE IF NOT EXISTS `comment_shard_config` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `db_count` int NOT NULL DEFAULT 4 COMMENT '分库数量',
    `table_count_per_db` int NOT NULL DEFAULT 4 COMMENT '每个分库的分表数量',
    `shard_algorithm` varchar(50) NOT NULL DEFAULT 'hash' COMMENT '分片算法',
    `shard_key` varchar(50) NOT NULL DEFAULT 'comment_id' COMMENT '分片键',
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入默认配置
INSERT INTO `comment_shard_config` (`db_count`, `table_count_per_db`, `shard_algorithm`, `shard_key`, `created_at`, `updated_at`) 
VALUES (4, 4, 'hash', 'comment_id', DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'));

-- 创建数据库连接配置表
CREATE TABLE IF NOT EXISTS `comment_db_connections` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `db_index` int NOT NULL COMMENT '分库索引',
    `db_name` varchar(100) NOT NULL COMMENT '数据库名称',
    `host` varchar(255) NOT NULL DEFAULT 'localhost' COMMENT '数据库主机',
    `port` int NOT NULL DEFAULT 3306 COMMENT '数据库端口',
    `username` varchar(100) NOT NULL DEFAULT 'root' COMMENT '用户名',
    `password` varchar(255) NOT NULL DEFAULT '' COMMENT '密码',
    `max_connections` int NOT NULL DEFAULT 100 COMMENT '最大连接数',
    `is_active` tinyint NOT NULL DEFAULT 1 COMMENT '是否激活',
    `created_at` varchar(255) NOT NULL,
    `updated_at` varchar(255) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_db_index` (`db_index`),
    KEY `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入分库连接配置
INSERT INTO `comment_db_connections` (`db_index`, `db_name`, `host`, `port`, `username`, `password`, `max_connections`, `created_at`, `updated_at`) VALUES
(0, 'comment_db_0', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(1, 'comment_db_1', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(2, 'comment_db_2', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(3, 'comment_db_3', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'));

-- 创建分片路由函数
DELIMITER $$

CREATE FUNCTION GetCommentDbIndex(comment_id BIGINT) RETURNS INT
READS SQL DATA
DETERMINISTIC
BEGIN
    DECLARE db_index INT;
    SET db_index = comment_id % 4;
    RETURN db_index;
END$$

CREATE FUNCTION GetCommentTableIndex(comment_id BIGINT) RETURNS INT
READS SQL DATA
DETERMINISTIC
BEGIN
    DECLARE table_index INT;
    SET table_index = (comment_id DIV 4) % 4;
    RETURN table_index;
END$$

DELIMITER ;

-- 创建分片路由存储过程
DELIMITER $$

CREATE PROCEDURE GetCommentShardInfo(IN comment_id BIGINT, OUT db_name VARCHAR(100), OUT table_name VARCHAR(100))
BEGIN
    DECLARE db_index INT;
    DECLARE table_index INT;
    
    SET db_index = GetCommentDbIndex(comment_id);
    SET table_index = GetCommentTableIndex(comment_id);
    
    SET db_name = CONCAT('comment_db_', db_index);
    SET table_name = CONCAT('comments_', table_index);
END$$

DELIMITER ;

-- 创建全局评论统计表（汇总所有分库分表的数据）
CREATE TABLE IF NOT EXISTS `global_comment_stats` (
    `video_id` bigint NOT NULL,
    `total_comment_count` bigint NOT NULL DEFAULT 0,
    `total_like_count` bigint NOT NULL DEFAULT 0,
    `last_comment_time` varchar(255) DEFAULT '',
    `hot_score` decimal(10,2) DEFAULT 0.00,
    `db_0_count` bigint DEFAULT 0 COMMENT '分库0的评论数',
    `db_1_count` bigint DEFAULT 0 COMMENT '分库1的评论数',
    `db_2_count` bigint DEFAULT 0 COMMENT '分库2的评论数',
    `db_3_count` bigint DEFAULT 0 COMMENT '分库3的评论数',
    `updated_at` varchar(255) NOT NULL,
    PRIMARY KEY (`video_id`),
    KEY `idx_total_comment_count` (`total_comment_count`) USING BTREE,
    KEY `idx_hot_score` (`hot_score`) USING BTREE,
    KEY `idx_last_comment_time` (`last_comment_time`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建全局用户评论索引表（记录用户评论分布在哪个分库分表）
CREATE TABLE IF NOT EXISTS `global_user_comment_index` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `user_id` bigint NOT NULL,
    `comment_id` bigint NOT NULL,
    `video_id` bigint NOT NULL,
    `db_index` tinyint NOT NULL COMMENT '分库索引 0-3',
    `table_index` tinyint NOT NULL COMMENT '分表索引 0-3',
    `created_at` varchar(255) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_comment` (`user_id`, `comment_id`),
    KEY `idx_user_id_created` (`user_id`, `created_at`) USING BTREE,
    KEY `idx_comment_id` (`comment_id`) USING BTREE,
    KEY `idx_db_table_index` (`db_index`, `table_index`) USING BTREE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入初始化完成日志
INSERT INTO system_logs (log_type, message, level, created_at) 
VALUES ('system_init', 'Multi-database comment system initialization completed - 4 databases with 4 tables each (16 total tables)', 'INFO', DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'));

-- 显示分库分表结构信息
SELECT 
    'Database Structure' as info_type,
    'comment_db_0: comments_0, comments_1, comments_2, comments_3' as db_0,
    'comment_db_1: comments_0, comments_1, comments_2, comments_3' as db_1,
    'comment_db_2: comments_0, comments_1, comments_2, comments_3' as db_2,
    'comment_db_3: comments_0, comments_1, comments_2, comments_3' as db_3,
    'Total: 4 databases × 4 tables = 16 tables' as summary;