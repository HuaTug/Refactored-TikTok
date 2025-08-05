create database if not exists TikTok;
use TikTok;

-- Table structure of sys_setting --
drop table if exists `sys_settings`;
create table `sys_settings`(
    `id` bigint not null auto_increment,
    `audit_policy` longtext not null,
    `audit_open` tinyint not null default '0' comment '0:disable 1:enable',
    `hot_limit` varchar(255) not null default '100',
    `allow_ip` varchar(255) not null,
    `auth` tinyint not null default '0' comment '0:disable 1:enable',
    `value` varchar(255) not null,
    `created_at` varchar(255) not null,
    `updated_at` varchar(255) not null,
    primary key (id)
)engine = InnoDB  auto_increment=1 default  charset = utf8mb4;

-- Table structure of role --
drop table if exists `roles`;
create table `roles`(
    `role_id` bigint not null auto_increment,
    `role` varchar(255) not null,
    primary key (role_id)
) engine = InnoDB  auto_increment=1 default  charset = utf8mb4;
INSERT INTO `roles` (`role_id`,`role`) VALUES (1,'admin'),(2,'user'),(3,'guest');-- 完成了对角色的权限划分

-- Table structure of role_permission --
drop table if exists `role_permissions`;
create table `role_permissions`(
    `permission_id` bigint not null auto_increment,
    `role_id` bigint not null,
    primary key (permission_id)
)engine = InnoDB  auto_increment=1 default  charset = utf8mb4;

-- Table structure of user --
drop table if exists `users`;
create table   `users`(
    `user_id` bigint not null auto_increment ,
    `user_name` varchar(255) not null ,
    `password` varchar(255) not null ,
    `email` varchar(30) not null,
    `sex` tinyint(1) not null, -- 0:female 1:male
    `avatar_url` varchar(255) ,
    `created_at` varchar(255) not null,
    `updated_at` varchar(255) not null,
    `deleted_at` varchar(255) ,
    primary key (user_id) ,
    key `username_password_index` (user_name,password) using btree
) engine = InnoDB  auto_increment=1 default  charset = utf8mb4;

-- -- 创建其他分表 users_1, users_2, users_3
-- CREATE TABLE `users_1` LIKE `users_0`;
-- CREATE TABLE `users_2` LIKE `users_0`;
-- CREATE TABLE `users_3` LIKE `users_0`;

-- Table structure of user_role --
drop table if exists `user_roles`;
create table `user_roles`(
    `role_id` bigint not null,
    `user_id` bigint not null,
    `role` varchar(255) not null
);


drop table if exists `user_behaviors`;
create table `user_behaviors`(
    `user_behavior_id` bigint not null auto_increment,
    `user_id` bigint not null,
    `video_id` bigint not null,
    `behavior_type` varchar(50) not null, -- 'view' 'like' 'share' 'comment'
    `behavior_time` varchar(255) not null,
    `created_at` timestamp default current_timestamp,
    -- Remove unique constraint to allow multiple comments per user per video
    -- unique key(user_id,video_id,behavior_type),
    primary key (user_behavior_id),
    key `idx_user_video_behavior` (user_id, video_id, behavior_type),
    key `idx_behavior_time` (behavior_time)
)engine InnoDB auto_increment=1  default  charset=utf8mb4;

-- Table structure of videos --
drop table if exists `videos`;
create table `videos`(
    `video_id` bigint not null auto_increment,
    `user_id` bigint not null ,
    `video_url` varchar(255) not null ,
    `cover_url` varchar(255) not null ,
    `title` varchar(255) not null ,
    `description` varchar(255) not null ,
    `visit_count` varchar(255) default '0' not null,
    `share_count` varchar(255) default '0' not null ,
    `likes_count` varchar(255) default '0' not null,
    `favorites_count` varchar(255) default '0' not null,
    `comment_count` varchar(255) default '0' not null,
    `history_count` varchar(255) default '0' not null,
    `open` tinyint not null default '0' comment '0:private 1:public',
    `audit_status` tinyint not null default '0' comment '0:unreviewed 1:reviewed',
    `label_names` varchar(255) default '' not null,
    `category` varchar(255) default '' not null,
    `created_at` varchar(255) not null ,
    `updated_at` varchar(255) not null ,
    `deleted_at` varchar(255) ,
    primary key (video_id),
    key `time` (created_at) using btree ,
    key `author` (user_id) using btree
)engine InnoDB auto_increment=1  default  charset=utf8mb4;


-- Table structure of user_video_watch_histories --
drop table if exists `user_video_watch_histories`;
create table `user_video_watch_histories`(
    `user_video_watch_history_id` bigint not null auto_increment,
    `user_id` bigint not null,
    `video_id` bigint not null,
    `watch_time` varchar(255) not null,
    `deleted_at` varchar(255),
    primary key (user_video_watch_history_id),
    unique key( user_id,video_id)
)engine InnoDB auto_increment=1  default  charset=utf8mb4;

-- Table structure of video_likes --
drop table if exists `video_likes`;
create table `video_likes`(
    `video_likes_id` bigint not null ,
    `user_id` bigint not null ,
    `video_id` bigint not null ,
    `created_at` varchar(255) not null ,
    `deleted_at` varchar(255)  ,
    primary key (video_likes_id),
    unique key `user_id_video_id_no_duplicate` (user_id,video_id),
    key `user_id_video_id_index`(user_id,video_id) using btree ,
    key `user_id_index` (user_id) using btree ,
    key `video_id_index` (video_id) using btree
)engine = InnoDB auto_increment=1 default charset = utf8mb4;

-- Table structure of video_share --
drop table if exists `video_shares`;
create table `video_shares`(
    `video_share_id` bigint not null auto_increment,
    `user_id` bigint not null, -- 分享者
    `video_id` bigint not null, -- 被分享的视频
    `to_user_id` bigint not null, -- 被分享的用户
    `created_at` varchar(255) not null,
    `deleted_at` varchar(255),
    primary key (video_share_id)
)engine = InnoDB auto_increment=1 default charset = utf8mb4;

-- Table structure of favorites --
drop table if exists `favorites`;
create table `favorites`(
    `favorite_id` bigint not null auto_increment,
    `user_id` bigint not null,
    `name` varchar(255) not null,
    `description` varchar(255) default ''  not null,
    `cover_url` varchar(255) default '' not null,
    `created_at` varchar(255) not null,
    `deleted_at` varchar(255),
    primary key (favorite_id)
)engine = InnoDB auto_increment=1 default charset = utf8mb4;

-- Table structure of favorites_videos --
drop table if exists `favorites_videos`;
create table `favorites_videos`(
    `favorite_video_id` bigint not null auto_increment,
    `favorite_id` bigint not null, -- 收藏夹id
    `video_id` bigint not null, -- 被收藏的视频
    `user_id` bigint not null,
    primary key (favorite_video_id),
    unique key `fav_vid_usr_index` (favorite_id,video_id,user_id) using btree,
    key  `fav_usr_index` (user_id,favorite_id) using btree
)engine = InnoDB auto_increment=1 default charset = utf8mb4;

-- Table structure of user_perferences --
drop table if exists `user_perferences`;
create table `user_perferences`(
    `user_id` bigint not null,
    `label_names` varchar(255) not null   -- 以逗号分隔的用户偏好标签字符串
);


-- Table structure of system_logs --
DROP TABLE IF EXISTS `system_logs`;
CREATE TABLE `system_logs` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `log_type` varchar(50) NOT NULL,
    `message` text NOT NULL,
    `level` varchar(20) NOT NULL DEFAULT 'INFO',
    `created_at` varchar(255) NOT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_log_type` (`log_type`) USING BTREE,
    KEY `idx_level` (`level`) USING BTREE,
    KEY `idx_created_at` (`created_at`) USING BTREE
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入初始化完成日志
INSERT INTO system_logs (log_type, message, level, created_at) 
VALUES ('system_init', 'Multi-database comment system initialization completed - 4 databases with 4 tables each (16 total tables)', 'INFO', DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'));

-- ========================================
-- Relation分库分表初始化脚本
-- 基于Comment分库分表模式设计
-- 创建4个分库，每个分库4张分表，总共16张表
-- ========================================

-- ========================================
-- 创建分库 relation_db_0
-- ========================================
CREATE DATABASE IF NOT EXISTS relation_db_0 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE relation_db_0;

-- 创建关注关系表 follows_0
CREATE TABLE IF NOT EXISTS `follows_0` (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='关注关系表';

-- 创建关注关系表 follows_1
CREATE TABLE IF NOT EXISTS `follows_1` LIKE `follows_0`;

-- 创建关注关系表 follows_2
CREATE TABLE IF NOT EXISTS `follows_2` LIKE `follows_0`;

-- 创建关注关系表 follows_3
CREATE TABLE IF NOT EXISTS `follows_3` LIKE `follows_0`;

-- 创建用户关系统计表（每个分库都有）
CREATE TABLE IF NOT EXISTS `user_relation_stats` (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户关系统计表';

-- ========================================
-- 创建分库 relation_db_1
-- ========================================
CREATE DATABASE IF NOT EXISTS relation_db_1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE relation_db_1;

-- 创建关注关系表 follows_0
CREATE TABLE IF NOT EXISTS `follows_0` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_1` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_2` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_3` LIKE relation_db_0.follows_0;

-- 创建用户关系统计表
CREATE TABLE IF NOT EXISTS `user_relation_stats` LIKE relation_db_0.user_relation_stats;

-- ========================================
-- 创建分库 relation_db_2
-- ========================================
CREATE DATABASE IF NOT EXISTS relation_db_2 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE relation_db_2;

-- 创建关注关系表
CREATE TABLE IF NOT EXISTS `follows_0` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_1` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_2` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_3` LIKE relation_db_0.follows_0;

-- 创建用户关系统计表
CREATE TABLE IF NOT EXISTS `user_relation_stats` LIKE relation_db_0.user_relation_stats;

-- ========================================
-- 创建分库 relation_db_3
-- ========================================
CREATE DATABASE IF NOT EXISTS relation_db_3 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
USE relation_db_3;

-- 创建关注关系表
CREATE TABLE IF NOT EXISTS `follows_0` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_1` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_2` LIKE relation_db_0.follows_0;
CREATE TABLE IF NOT EXISTS `follows_3` LIKE relation_db_0.follows_0;

-- 创建用户关系统计表
CREATE TABLE IF NOT EXISTS `user_relation_stats` LIKE relation_db_0.user_relation_stats;

-- ========================================
-- 回到主库创建全局管理表
-- ========================================
USE TikTok;

-- 创建分片配置表
CREATE TABLE IF NOT EXISTS `relation_shard_config` (
    `id` int NOT NULL AUTO_INCREMENT,
    `shard_key` varchar(50) NOT NULL COMMENT '分片键名称',
    `shard_count` int NOT NULL COMMENT '分片数量',
    `table_count_per_shard` int NOT NULL COMMENT '每个分片的表数量',
    `shard_algorithm` varchar(50) NOT NULL DEFAULT 'hash' COMMENT '分片算法',
    `created_at` timestamp DEFAULT CURRENT_TIMESTAMP,
    `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_shard_key` (`shard_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='关系分片配置表';

-- 插入关系分片配置
INSERT INTO `relation_shard_config` (`shard_key`, `shard_count`, `table_count_per_shard`, `shard_algorithm`) VALUES
('relation', 4, 4, 'hash') ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- 创建分库连接配置表
CREATE TABLE IF NOT EXISTS `relation_db_connections` (
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='关系分库连接配置表';

-- 插入分库连接配置
INSERT INTO `relation_db_connections` (`db_index`, `db_name`, `host`, `port`, `username`, `password`, `max_connections`, `created_at`, `updated_at`) VALUES
(0, 'relation_db_0', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(1, 'relation_db_1', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(2, 'relation_db_2', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s')),
(3, 'relation_db_3', 'localhost', 3306, 'root', '', 100, DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'), DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'));

-- 创建全局关系统计表
CREATE TABLE IF NOT EXISTS `global_relation_stats` (
    `id` int NOT NULL AUTO_INCREMENT,
    `total_follows` bigint NOT NULL DEFAULT 0 COMMENT '总关注数',
    `total_mutual_follows` bigint NOT NULL DEFAULT 0 COMMENT '总互关数',
    `active_users_today` bigint NOT NULL DEFAULT 0 COMMENT '今日活跃用户',
    `stats_date` date NOT NULL COMMENT '统计日期',
    `created_at` timestamp DEFAULT CURRENT_TIMESTAMP,
    `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_stats_date` (`stats_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='全局关系统计表';

-- 创建全局用户关系索引表
CREATE TABLE IF NOT EXISTS `global_user_relation_index` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `user_id` bigint NOT NULL,
    `relation_id` bigint NOT NULL,
    `target_user_id` bigint NOT NULL,
    `relation_type` varchar(20) NOT NULL COMMENT 'follow/friend/mutual',
    `db_index` tinyint NOT NULL COMMENT '分库索引 0-3',
    `table_index` tinyint NOT NULL COMMENT '分表索引 0-3',
    `created_at` varchar(255) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_relation` (`user_id`, `relation_id`, `relation_type`),
    KEY `idx_user_id_created` (`user_id`, `created_at`),
    KEY `idx_target_user_id` (`target_user_id`),
    KEY `idx_relation_type` (`relation_type`),
    KEY `idx_db_table_index` (`db_index`, `table_index`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='全局用户关系索引表';


-- 创建初始化完成日志
INSERT INTO system_logs (log_type, message, level, created_at) 
VALUES ('relation_shard_init', 'Relation sharding system initialization completed - 4 databases with 4 tables each (16 total tables)', 'INFO', DATE_FORMAT(NOW(), '%Y-%m-%d %H:%i:%s'))
ON DUPLICATE KEY UPDATE `created_at` = VALUES(`created_at`);



-- 视频存储映射表
CREATE TABLE IF NOT EXISTS `video_storage_mapping` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    
    -- 存储路径信息
    `source_path` VARCHAR(512) NOT NULL COMMENT '原始文件路径',
    `processed_paths` JSON COMMENT '处理后文件路径映射 {"480": "path1", "720": "path2", "1080": "path3"}',
    `thumbnail_paths` JSON COMMENT '缩略图路径映射 {"small": "path1", "medium": "path2", "large": "path3"}',
    `animated_cover_path` VARCHAR(512) COMMENT '动态封面路径',
    `metadata_path` VARCHAR(512) COMMENT '元数据文件路径',
    
    -- 存储状态
    `storage_status` ENUM('uploading', 'processing', 'completed', 'failed') DEFAULT 'uploading' COMMENT '存储状态',
    `hot_storage` BOOLEAN DEFAULT FALSE COMMENT '是否在热点存储',
    `bucket_name` VARCHAR(128) DEFAULT 'tiktok-user-content' COMMENT '存储桶名称',
    
    -- 访问统计
    `access_count` BIGINT DEFAULT 0 COMMENT '访问次数',
    `last_accessed_at` TIMESTAMP NULL COMMENT '最后访问时间',
    `play_count` BIGINT DEFAULT 0 COMMENT '播放次数',
    `download_count` BIGINT DEFAULT 0 COMMENT '下载次数',
    
    -- 存储元信息
    `file_size` BIGINT COMMENT '文件大小（字节）',
    `duration` INT COMMENT '视频时长（秒）',
    `resolution_width` INT COMMENT '视频宽度',
    `resolution_height` INT COMMENT '视频高度',
    `format` VARCHAR(16) DEFAULT 'mp4' COMMENT '视频格式',
    `codec` VARCHAR(32) COMMENT '视频编码',
    `bitrate` INT COMMENT '比特率',
    
    -- 时间戳
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    
    -- 索引
    INDEX `idx_user_video` (`user_id`, `video_id`),
    INDEX `idx_storage_status` (`storage_status`),
    INDEX `idx_hot_storage` (`hot_storage`),
    INDEX `idx_last_accessed` (`last_accessed_at`),
    INDEX `idx_created_at` (`created_at`),
    UNIQUE INDEX `uk_video_id` (`video_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频存储映射表';

-- 用户存储配额表
CREATE TABLE IF NOT EXISTS `user_storage_quota` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL UNIQUE COMMENT '用户ID',
    
    -- 配额限制
    `max_storage_bytes` BIGINT DEFAULT 10737418240 COMMENT '最大存储空间（字节）10GB',
    `max_video_count` INT DEFAULT 1000 COMMENT '最大视频数量',
    `max_video_duration` INT DEFAULT 600 COMMENT '单个视频最大时长（秒）10分钟',
    `max_video_size` BIGINT DEFAULT 1073741824 COMMENT '单个视频最大大小（字节）1GB',
    
    -- 当前使用情况
    `used_storage_bytes` BIGINT DEFAULT 0 COMMENT '已使用存储空间',
    `video_count` INT DEFAULT 0 COMMENT '当前视频数量',
    `draft_count` INT DEFAULT 0 COMMENT '草稿数量',
    
    -- 配额状态
    `quota_exceeded` BOOLEAN DEFAULT FALSE COMMENT '是否超出配额',
    `warning_sent` BOOLEAN DEFAULT FALSE COMMENT '是否已发送警告',
    `quota_level` ENUM('basic', 'premium', 'vip', 'unlimited') DEFAULT 'basic' COMMENT '配额等级',
    
    -- 统计信息
    `total_upload_bytes` BIGINT DEFAULT 0 COMMENT '总上传流量',
    `total_download_bytes` BIGINT DEFAULT 0 COMMENT '总下载流量',
    `last_upload_at` TIMESTAMP NULL COMMENT '最后上传时间',
    
    -- 时间戳
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    
    -- 索引
    INDEX `idx_quota_exceeded` (`quota_exceeded`),
    INDEX `idx_quota_level` (`quota_level`),
    INDEX `idx_last_upload` (`last_upload_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户存储配额表';

-- 视频访问日志表（用于热度分析）
CREATE TABLE IF NOT EXISTS `video_access_log` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `user_id` BIGINT COMMENT '访问用户ID（可为空，匿名访问）',
    `access_type` ENUM('view', 'download', 'share', 'like', 'comment') NOT NULL COMMENT '访问类型',
    `ip_address` VARCHAR(45) COMMENT 'IP地址',
    `user_agent` VARCHAR(512) COMMENT '用户代理',
    `device_type` ENUM('mobile', 'desktop', 'tablet', 'unknown') DEFAULT 'unknown' COMMENT '设备类型',
    `quality` VARCHAR(16) COMMENT '视频质量',
    `duration_played` INT DEFAULT 0 COMMENT '播放时长（秒）',
    `completion_rate` DECIMAL(5,2) DEFAULT 0.00 COMMENT '完播率（百分比）',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '访问时间',
    
    -- 索引
    INDEX `idx_video_id` (`video_id`),
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_access_type` (`access_type`),
    INDEX `idx_created_at` (`created_at`),
    INDEX `idx_device_type` (`device_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频访问日志表';

-- 热门视频缓存表
CREATE TABLE IF NOT EXISTS `hot_video_cache` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `video_id` BIGINT NOT NULL UNIQUE COMMENT '视频ID',
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `hot_score` DECIMAL(10,2) DEFAULT 0.00 COMMENT '热度分数',
    `cache_bucket` VARCHAR(128) DEFAULT 'tiktok-cache-hot' COMMENT '缓存桶名称',
    `cache_path` VARCHAR(512) COMMENT '缓存路径',
    `cache_status` ENUM('pending', 'cached', 'expired', 'failed') DEFAULT 'pending' COMMENT '缓存状态',
    `expire_at` TIMESTAMP NULL COMMENT '过期时间',
    
    -- 统计数据（用于计算热度）
    `view_count_24h` BIGINT DEFAULT 0 COMMENT '24小时内观看次数',
    `like_count_24h` BIGINT DEFAULT 0 COMMENT '24小时内点赞次数',
    `share_count_24h` BIGINT DEFAULT 0 COMMENT '24小时内分享次数',
    `comment_count_24h` BIGINT DEFAULT 0 COMMENT '24小时内评论次数',
    
    -- 时间戳
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    
    -- 索引
    INDEX `idx_hot_score` (`hot_score` DESC),
    INDEX `idx_cache_status` (`cache_status`),
    INDEX `idx_expire_at` (`expire_at`),
    INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='热门视频缓存表';

-- 存储桶管理表
CREATE TABLE IF NOT EXISTS `storage_bucket_config` (
    `id` BIGINT PRIMARY KEY AUTO_INCREMENT,
    `bucket_name` VARCHAR(128) NOT NULL UNIQUE COMMENT '存储桶名称',
    `bucket_type` ENUM('user_content', 'system_assets', 'cache_hot', 'cache_warm', 'cache_cold', 'analytics') NOT NULL COMMENT '存储桶类型',
    `region` VARCHAR(32) DEFAULT 'us-east-1' COMMENT '存储区域',
    `endpoint` VARCHAR(256) COMMENT '存储端点',
    `access_policy` JSON COMMENT '访问策略配置',
    `lifecycle_config` JSON COMMENT '生命周期配置',
    `hot_retention_days` INT DEFAULT 30 COMMENT '热数据保留天数',
    `warm_retention_days` INT DEFAULT 90 COMMENT '温数据保留天数',
    `cold_retention_days` INT DEFAULT 365 COMMENT '冷数据保留天数',
    `archive_after_days` INT DEFAULT 1095 COMMENT '归档天数',
    `is_active` BOOLEAN DEFAULT TRUE COMMENT '是否激活',
    
    -- 时间戳
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    
    -- 索引
    INDEX `idx_bucket_type` (`bucket_type`),
    INDEX `idx_is_active` (`is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='存储桶配置表';

-- 插入默认存储桶配置
INSERT INTO `storage_bucket_config` (`bucket_name`, `bucket_type`, `lifecycle_config`, `hot_retention_days`, `warm_retention_days`, `cold_retention_days`, `archive_after_days`) VALUES
('tiktok-user-content', 'user_content', '{"hot_days": 30, "warm_days": 90, "cold_days": 365, "archive_days": 1095}', 30, 90, 365, 1095),
('tiktok-system-assets', 'system_assets', '{"hot_days": 365, "warm_days": 0, "cold_days": 0, "archive_days": 0}', 365, 0, 0, 0),
('tiktok-cache-hot', 'cache_hot', '{"hot_days": 7, "warm_days": 0, "cold_days": 0, "archive_days": 0}', 7, 0, 0, 0),
('tiktok-cache-warm', 'cache_warm', '{"hot_days": 0, "warm_days": 30, "cold_days": 0, "archive_days": 0}', 0, 30, 0, 0),
('tiktok-cache-cold', 'cache_cold', '{"hot_days": 0, "warm_days": 0, "cold_days": 90, "archive_days": 0}', 0, 0, 90, 0),
('tiktok-analytics', 'analytics', '{"hot_days": 30, "warm_days": 90, "cold_days": 365, "archive_days": 2190}', 30, 90, 365, 2190)
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

CREATE TABLE IF NOT EXISTS `sync_events` (
    `id` VARCHAR(36) NOT NULL PRIMARY KEY,
    `event_type` VARCHAR(50) NOT NULL,
    `resource_type` VARCHAR(20) NOT NULL,
    `resource_id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `action_type` VARCHAR(20) NOT NULL,
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending',
    `data` TEXT,
    `retry_count` INT DEFAULT 0,
    `max_retries` INT DEFAULT 3,
    `priority` INT DEFAULT 0,
    `idempotency_key` VARCHAR(255),
    `error_message` TEXT,
    `processed_at` TIMESTAMP NULL,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    
    -- 索引
    INDEX `idx_event_type` (`event_type`),
    INDEX `idx_resource_id` (`resource_id`),
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_status` (`status`),
    INDEX `idx_priority` (`priority`),
    UNIQUE INDEX `idx_idempotency_key` (`idempotency_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建其他相关表（如果需要的话）

CREATE TABLE IF NOT EXISTS `sync_metrics` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `metric_type` VARCHAR(50) NOT NULL,
    `metric_name` VARCHAR(100) NOT NULL,
    `metric_value` DOUBLE NOT NULL,
    `tags` TEXT,
    `timestamp` TIMESTAMP NOT NULL,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX `idx_metric_type` (`metric_type`),
    INDEX `idx_timestamp` (`timestamp`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `idempotency_records` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `idempotency_key` VARCHAR(255) NOT NULL,
    `event_id` VARCHAR(36) NOT NULL,
    `status` VARCHAR(20) NOT NULL,
    `result` TEXT,
    `expires_at` TIMESTAMP NOT NULL,
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE INDEX `idx_idempotency_key` (`idempotency_key`),
    INDEX `idx_event_id` (`event_id`),
    INDEX `idx_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


-- ========================================
-- 千万级评论系统真正的分库分表初始化脚本
-- 创建4个分库，每个分库4张分表，总共16张表
-- ========================================

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

-- 创建分片路由配置表
CREATE TABLE IF NOT EXISTS `shard_config` (
    `id` int NOT NULL AUTO_INCREMENT,
    `shard_key` varchar(50) NOT NULL COMMENT '分片键名称',
    `shard_count` int NOT NULL COMMENT '分片数量',
    `table_count_per_shard` int NOT NULL COMMENT '每个分片的表数量',
    `created_at` timestamp DEFAULT CURRENT_TIMESTAMP,
    `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_shard_key` (`shard_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='分片配置表';

-- 插入评论分片配置
INSERT INTO `shard_config` (`shard_key`, `shard_count`, `table_count_per_shard`) VALUES
('comment', 4, 4) ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

-- 创建全局评论统计表
CREATE TABLE IF NOT EXISTS `global_comment_stats` (
    `id` int NOT NULL AUTO_INCREMENT,
    `total_comments` bigint NOT NULL DEFAULT 0 COMMENT '总评论数',
    `total_likes` bigint NOT NULL DEFAULT 0 COMMENT '总点赞数',
    `active_comments_today` bigint NOT NULL DEFAULT 0 COMMENT '今日活跃评论数',
    `stats_date` date NOT NULL COMMENT '统计日期',
    `created_at` timestamp DEFAULT CURRENT_TIMESTAMP,
    `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_stats_date` (`stats_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='全局评论统计表';