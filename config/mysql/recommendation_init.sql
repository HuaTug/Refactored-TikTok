-- ========================================
-- 视频推荐系统数据库表初始化脚本
-- 包含用户画像、视频特征、向量嵌入、曝光记录等核心推荐表
-- ========================================

USE TikTok;

-- ========================================
-- 1. 用户画像持久化表 (user_profiles)
-- 存储用户的兴趣偏好、行为特征等，用于个性化推荐
-- ========================================
DROP TABLE IF EXISTS `user_profiles`;
CREATE TABLE `user_profiles` (
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `interest_tags` JSON DEFAULT NULL COMMENT '兴趣标签权重 {"搞笑":0.8, "美食":0.6}',
    `category_preference` JSON DEFAULT NULL COMMENT '分类偏好 {"娱乐":0.9, "科技":0.5}',
    `author_preference` JSON DEFAULT NULL COMMENT '喜好的作者ID及权重',
    `topic_preference` JSON DEFAULT NULL COMMENT '话题偏好',
    `active_time_slots` JSON DEFAULT NULL COMMENT '活跃时段 [8,9,12,18,19,20,21,22]',
    `avg_watch_duration` DECIMAL(10,2) DEFAULT 0.00 COMMENT '平均观看时长(秒)',
    `avg_completion_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '平均完播率',
    `like_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '点赞率',
    `comment_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '评论率',
    `share_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '分享率',
    `total_view_count` BIGINT DEFAULT 0 COMMENT '总观看数',
    `total_like_count` BIGINT DEFAULT 0 COMMENT '总点赞数',
    `total_comment_count` BIGINT DEFAULT 0 COMMENT '总评论数',
    `total_share_count` BIGINT DEFAULT 0 COMMENT '总分享数',
    `user_level` TINYINT DEFAULT 1 COMMENT '用户活跃等级 1-5',
    `content_quality_pref` TINYINT DEFAULT 3 COMMENT '内容质量偏好 1-5',
    `video_duration_pref` TINYINT DEFAULT 2 COMMENT '视频时长偏好 1:短 2:中 3:长',
    `last_active_at` TIMESTAMP NULL COMMENT '最后活跃时间',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`),
    KEY `idx_user_level` (`user_level`),
    KEY `idx_last_active` (`last_active_at`),
    KEY `idx_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户画像表';

-- ========================================
-- 2. 视频特征/质量表 (video_features)
-- 存储视频的各项指标，用于推荐排序
-- ========================================
DROP TABLE IF EXISTS `video_features`;
CREATE TABLE `video_features` (
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `quality_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '内容质量分 0-10',
    `popularity_score` DECIMAL(10,2) DEFAULT 0.00 COMMENT '热度分',
    `freshness_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '新鲜度分 (时间衰减)',
    `ctr` DECIMAL(7,6) DEFAULT 0.000000 COMMENT '点击通过率',
    `finish_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '完播率',
    `like_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '点赞率',
    `comment_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '评论率',
    `share_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '分享率',
    `favorite_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '收藏率',
    `interact_score` DECIMAL(10,2) DEFAULT 0.00 COMMENT '综合互动分',
    `avg_watch_duration` DECIMAL(10,2) DEFAULT 0.00 COMMENT '平均观看时长',
    `exposure_count` BIGINT DEFAULT 0 COMMENT '曝光次数',
    `click_count` BIGINT DEFAULT 0 COMMENT '点击次数',
    `author_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '作者权重分',
    `is_high_quality` TINYINT DEFAULT 0 COMMENT '是否优质内容 0:否 1:是',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`video_id`),
    KEY `idx_quality_score` (`quality_score`),
    KEY `idx_popularity_score` (`popularity_score`),
    KEY `idx_freshness_score` (`freshness_score`),
    KEY `idx_ctr` (`ctr`),
    KEY `idx_finish_rate` (`finish_rate`),
    KEY `idx_is_high_quality` (`is_high_quality`),
    KEY `idx_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频特征表';

-- ========================================
-- 3. 视频向量嵌入表 (video_embeddings)
-- 存储视频的特征向量，用于相似推荐和内容理解
-- ========================================
DROP TABLE IF EXISTS `video_embeddings`;
CREATE TABLE `video_embeddings` (
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `embedding_type` VARCHAR(20) NOT NULL DEFAULT 'content' COMMENT '向量类型: content/visual/audio/text',
    `embedding_vector` JSON NOT NULL COMMENT '向量数据 [0.1, 0.2, ...]',
    `dimension` INT NOT NULL DEFAULT 128 COMMENT '向量维度',
    `model_version` VARCHAR(20) NOT NULL DEFAULT 'v1' COMMENT '模型版本',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`video_id`, `embedding_type`),
    KEY `idx_embedding_type` (`embedding_type`),
    KEY `idx_model_version` (`model_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频向量嵌入表';

-- ========================================
-- 4. 用户兴趣向量表 (user_embeddings)
-- 存储用户的兴趣向量，用于协同过滤推荐
-- ========================================
DROP TABLE IF EXISTS `user_embeddings`;
CREATE TABLE `user_embeddings` (
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `embedding_type` VARCHAR(20) NOT NULL DEFAULT 'interest' COMMENT '向量类型: interest/behavior/social',
    `embedding_vector` JSON NOT NULL COMMENT '向量数据',
    `dimension` INT NOT NULL DEFAULT 128 COMMENT '向量维度',
    `model_version` VARCHAR(20) NOT NULL DEFAULT 'v1' COMMENT '模型版本',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`, `embedding_type`),
    KEY `idx_embedding_type` (`embedding_type`),
    KEY `idx_model_version` (`model_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户兴趣向量表';

-- ========================================
-- 5. 视频相似度表 (video_similarities)
-- 预计算视频间的相似度，加速推荐
-- ========================================
DROP TABLE IF EXISTS `video_similarities`;
CREATE TABLE `video_similarities` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `similar_video_id` BIGINT NOT NULL COMMENT '相似视频ID',
    `similarity_score` DECIMAL(5,4) NOT NULL DEFAULT 0.0000 COMMENT '相似度分数 0-1',
    `similarity_type` VARCHAR(20) NOT NULL DEFAULT 'content' COMMENT '相似类型: content/collaborative/tag',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_video_similar` (`video_id`, `similar_video_id`, `similarity_type`),
    KEY `idx_video_id` (`video_id`),
    KEY `idx_similar_video_id` (`similar_video_id`),
    KEY `idx_similarity_score` (`similarity_score`),
    KEY `idx_similarity_type` (`similarity_type`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频相似度表';

-- ========================================
-- 6. 推荐曝光记录表 (recommendation_exposures)
-- 记录推荐给用户的视频，用于去重和效果追踪
-- ========================================
DROP TABLE IF EXISTS `recommendation_exposures`;
CREATE TABLE `recommendation_exposures` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `recall_source` VARCHAR(30) NOT NULL COMMENT '召回来源: hot/cf/content/social/new/trending',
    `position` INT NOT NULL DEFAULT 0 COMMENT '曝光位置',
    `score` DECIMAL(10,6) DEFAULT 0.000000 COMMENT '推荐分数',
    `is_clicked` TINYINT DEFAULT 0 COMMENT '是否点击',
    `is_liked` TINYINT DEFAULT 0 COMMENT '是否点赞',
    `is_commented` TINYINT DEFAULT 0 COMMENT '是否评论',
    `is_shared` TINYINT DEFAULT 0 COMMENT '是否分享',
    `is_favorited` TINYINT DEFAULT 0 COMMENT '是否收藏',
    `watch_duration` INT DEFAULT 0 COMMENT '观看时长(秒)',
    `completion_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '完播率',
    `exposure_time` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '曝光时间',
    `request_id` VARCHAR(64) DEFAULT NULL COMMENT '请求ID',
    PRIMARY KEY (`id`, `exposure_time`),
    KEY `idx_user_exposure` (`user_id`, `exposure_time`),
    KEY `idx_user_video` (`user_id`, `video_id`),
    KEY `idx_video_id` (`video_id`),
    KEY `idx_recall_source` (`recall_source`),
    KEY `idx_exposure_time` (`exposure_time`),
    KEY `idx_request_id` (`request_id`),
    KEY `idx_is_clicked` (`is_clicked`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='推荐曝光记录表'
PARTITION BY RANGE (UNIX_TIMESTAMP(exposure_time)) (
    PARTITION p202601 VALUES LESS THAN (UNIX_TIMESTAMP('2026-02-01')),
    PARTITION p202602 VALUES LESS THAN (UNIX_TIMESTAMP('2026-03-01')),
    PARTITION p202603 VALUES LESS THAN (UNIX_TIMESTAMP('2026-04-01')),
    PARTITION p202604 VALUES LESS THAN (UNIX_TIMESTAMP('2026-05-01')),
    PARTITION p202605 VALUES LESS THAN (UNIX_TIMESTAMP('2026-06-01')),
    PARTITION p202606 VALUES LESS THAN (UNIX_TIMESTAMP('2026-07-01')),
    PARTITION p_future VALUES LESS THAN MAXVALUE
);

-- ========================================
-- 7. 用户负反馈表 (negative_feedbacks)
-- 记录用户"不感兴趣"等负反馈，用于过滤推荐
-- ========================================
DROP TABLE IF EXISTS `negative_feedbacks`;
CREATE TABLE `negative_feedbacks` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `target_type` TINYINT NOT NULL COMMENT '1:video 2:author 3:category 4:tag',
    `target_id` BIGINT DEFAULT NULL COMMENT '目标ID (视频ID/作者ID)',
    `target_value` VARCHAR(100) DEFAULT NULL COMMENT '目标值 (分类名/标签名)',
    `feedback_type` TINYINT NOT NULL COMMENT '1:不感兴趣 2:看过了 3:内容重复 4:内容低质',
    `reason` VARCHAR(255) DEFAULT NULL COMMENT '具体原因',
    `expire_at` TIMESTAMP NULL COMMENT '过期时间 (部分负反馈可过期)',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_user_target` (`user_id`, `target_type`, `target_id`),
    KEY `idx_user_feedback` (`user_id`, `feedback_type`),
    KEY `idx_target_type` (`target_type`),
    KEY `idx_expire_at` (`expire_at`),
    KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户负反馈表';

-- ========================================
-- 8. 视频实时热度表 (video_hot_scores)
-- 按时间窗口统计视频热度，用于热门推荐
-- ========================================
DROP TABLE IF EXISTS `video_hot_scores`;
CREATE TABLE `video_hot_scores` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `time_window` VARCHAR(10) NOT NULL COMMENT '时间窗口: 1h/6h/24h/7d',
    `view_count` BIGINT DEFAULT 0 COMMENT '观看次数',
    `like_count` BIGINT DEFAULT 0 COMMENT '点赞次数',
    `comment_count` BIGINT DEFAULT 0 COMMENT '评论次数',
    `share_count` BIGINT DEFAULT 0 COMMENT '分享次数',
    `favorite_count` BIGINT DEFAULT 0 COMMENT '收藏次数',
    `hot_score` DECIMAL(12,2) DEFAULT 0.00 COMMENT '综合热度分',
    `rank` INT DEFAULT 0 COMMENT '排名',
    `window_start` TIMESTAMP NOT NULL COMMENT '窗口开始时间',
    `window_end` TIMESTAMP NOT NULL COMMENT '窗口结束时间',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_video_window` (`video_id`, `time_window`),
    KEY `idx_time_window` (`time_window`),
    KEY `idx_hot_score` (`hot_score` DESC),
    KEY `idx_rank` (`rank`),
    KEY `idx_window_start` (`window_start`),
    KEY `idx_updated_at` (`updated_at`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频实时热度表';

-- ========================================
-- 9. 作者评分表 (author_scores)
-- 存储创作者的质量评分，用于推荐加权
-- ========================================
DROP TABLE IF EXISTS `author_scores`;
CREATE TABLE `author_scores` (
    `author_id` BIGINT NOT NULL COMMENT '作者ID',
    `quality_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '内容质量分',
    `activity_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '活跃度分',
    `influence_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '影响力分',
    `growth_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '成长潜力分',
    `overall_score` DECIMAL(5,2) DEFAULT 0.00 COMMENT '综合评分',
    `total_videos` INT DEFAULT 0 COMMENT '总视频数',
    `avg_video_quality` DECIMAL(5,2) DEFAULT 0.00 COMMENT '平均视频质量',
    `avg_video_views` DECIMAL(12,2) DEFAULT 0.00 COMMENT '平均视频播放量',
    `avg_engagement_rate` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '平均互动率',
    `last_publish_at` TIMESTAMP NULL COMMENT '最后发布时间',
    `level` TINYINT DEFAULT 1 COMMENT '作者等级 1-10',
    `is_verified` TINYINT DEFAULT 0 COMMENT '是否认证',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`author_id`),
    KEY `idx_overall_score` (`overall_score`),
    KEY `idx_quality_score` (`quality_score`),
    KEY `idx_influence_score` (`influence_score`),
    KEY `idx_level` (`level`),
    KEY `idx_is_verified` (`is_verified`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='作者评分表';

-- ========================================
-- 10. 推荐布隆过滤器状态表 (recommendation_bloom_filters)
-- 存储用户的布隆过滤器状态，用于高效去重
-- ========================================
DROP TABLE IF EXISTS `recommendation_bloom_filters`;
CREATE TABLE `recommendation_bloom_filters` (
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `filter_data` MEDIUMBLOB COMMENT '布隆过滤器序列化数据',
    `filter_size` BIGINT DEFAULT 0 COMMENT '过滤器大小(bit)',
    `item_count` BIGINT DEFAULT 0 COMMENT '已添加元素数量',
    `last_reset_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '上次重置时间',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`user_id`),
    KEY `idx_last_reset` (`last_reset_at`),
    KEY `idx_item_count` (`item_count`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='推荐布隆过滤器状态表';

-- ========================================
-- 11. 标签-视频映射表 (tag_video_mappings)
-- 用于基于标签的内容推荐
-- ========================================
DROP TABLE IF EXISTS `tag_video_mappings`;
CREATE TABLE `tag_video_mappings` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `tag_name` VARCHAR(50) NOT NULL COMMENT '标签名',
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `weight` DECIMAL(5,4) DEFAULT 1.0000 COMMENT '标签权重',
    `source` VARCHAR(20) DEFAULT 'manual' COMMENT '来源: manual/ai/user',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_tag_video` (`tag_name`, `video_id`),
    KEY `idx_tag_name` (`tag_name`),
    KEY `idx_video_id` (`video_id`),
    KEY `idx_weight` (`weight`),
    KEY `idx_source` (`source`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='标签-视频映射表';

-- ========================================
-- 12. 分类视频统计表 (category_video_stats)
-- 用于按分类推荐
-- ========================================
DROP TABLE IF EXISTS `category_video_stats`;
CREATE TABLE `category_video_stats` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `category` VARCHAR(50) NOT NULL COMMENT '分类名',
    `total_videos` BIGINT DEFAULT 0 COMMENT '总视频数',
    `total_views` BIGINT DEFAULT 0 COMMENT '总播放量',
    `total_likes` BIGINT DEFAULT 0 COMMENT '总点赞数',
    `avg_quality` DECIMAL(5,2) DEFAULT 0.00 COMMENT '平均质量分',
    `hot_score` DECIMAL(12,2) DEFAULT 0.00 COMMENT '热度分',
    `daily_new_videos` INT DEFAULT 0 COMMENT '日新增视频数',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_category` (`category`),
    KEY `idx_hot_score` (`hot_score`),
    KEY `idx_total_videos` (`total_videos`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='分类视频统计表';

-- ========================================
-- 13. 用户视频详细交互记录表 (user_video_interactions)
-- 补充现有的 user_behaviors，记录更详细的交互数据
-- ========================================
DROP TABLE IF EXISTS `user_video_interactions`;
CREATE TABLE `user_video_interactions` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `video_id` BIGINT NOT NULL COMMENT '视频ID',
    `impression_count` INT DEFAULT 0 COMMENT '曝光次数',
    `click_count` INT DEFAULT 0 COMMENT '点击次数',
    `total_watch_time` INT DEFAULT 0 COMMENT '总观看时长(秒)',
    `max_watch_progress` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '最大观看进度',
    `last_watch_position` INT DEFAULT 0 COMMENT '上次观看位置(秒)',
    `replay_count` INT DEFAULT 0 COMMENT '重播次数',
    `is_liked` TINYINT DEFAULT 0 COMMENT '是否点赞',
    `is_favorited` TINYINT DEFAULT 0 COMMENT '是否收藏',
    `is_shared` TINYINT DEFAULT 0 COMMENT '是否分享',
    `comment_count` INT DEFAULT 0 COMMENT '评论次数',
    `engagement_score` DECIMAL(10,4) DEFAULT 0.0000 COMMENT '综合互动分',
    `first_interact_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '首次交互时间',
    `last_interact_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '最后交互时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_video` (`user_id`, `video_id`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_video_id` (`video_id`),
    KEY `idx_engagement_score` (`engagement_score`),
    KEY `idx_last_interact` (`last_interact_at`),
    KEY `idx_is_liked` (`is_liked`),
    KEY `idx_is_favorited` (`is_favorited`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户视频交互详情表';

-- ========================================
-- 14. A/B测试实验表 (ab_test_experiments)
-- 用于推荐算法的 A/B 测试
-- ========================================
DROP TABLE IF EXISTS `ab_test_experiments`;
CREATE TABLE `ab_test_experiments` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `experiment_name` VARCHAR(100) NOT NULL COMMENT '实验名称',
    `description` TEXT DEFAULT NULL COMMENT '实验描述',
    `traffic_ratio` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '实验流量比例',
    `status` TINYINT DEFAULT 0 COMMENT '状态 0:draft 1:running 2:paused 3:finished',
    `config` JSON DEFAULT NULL COMMENT '实验配置',
    `metrics` JSON DEFAULT NULL COMMENT '实验指标结果',
    `start_time` TIMESTAMP NULL COMMENT '开始时间',
    `end_time` TIMESTAMP NULL COMMENT '结束时间',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_experiment_name` (`experiment_name`),
    KEY `idx_status` (`status`),
    KEY `idx_start_time` (`start_time`),
    KEY `idx_end_time` (`end_time`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B测试实验表';

-- ========================================
-- 15. A/B测试分组表 (ab_test_groups)
-- ========================================
DROP TABLE IF EXISTS `ab_test_groups`;
CREATE TABLE `ab_test_groups` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `experiment_id` BIGINT NOT NULL COMMENT '实验ID',
    `group_name` VARCHAR(50) NOT NULL COMMENT '分组名称: control/treatment_a/treatment_b',
    `traffic_ratio` DECIMAL(5,4) DEFAULT 0.0000 COMMENT '分组流量比例',
    `config` JSON DEFAULT NULL COMMENT '分组特定配置',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_experiment_id` (`experiment_id`),
    KEY `idx_group_name` (`group_name`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B测试分组表';

-- ========================================
-- 16. 用户A/B测试分配表 (user_ab_test_assignments)
-- ========================================
DROP TABLE IF EXISTS `user_ab_test_assignments`;
CREATE TABLE `user_ab_test_assignments` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL COMMENT '用户ID',
    `experiment_id` BIGINT NOT NULL COMMENT '实验ID',
    `group_id` BIGINT NOT NULL COMMENT '分组ID',
    `assigned_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '分配时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_experiment` (`user_id`, `experiment_id`),
    KEY `idx_experiment_id` (`experiment_id`),
    KEY `idx_group_id` (`group_id`),
    KEY `idx_assigned_at` (`assigned_at`)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户A/B测试分配表';

-- ========================================
-- 插入初始化完成日志
-- ========================================
INSERT INTO system_logs (log_type, message, level)
VALUES ('recommendation_init', 'Recommendation system tables initialization completed - 18 tables created', 'INFO');

-- ========================================
-- 插入默认分类统计
-- ========================================
INSERT INTO `category_video_stats` (`category`, `total_videos`, `total_views`, `total_likes`, `avg_quality`, `hot_score`, `daily_new_videos`) VALUES
('娱乐', 0, 0, 0, 0.00, 0.00, 0),
('搞笑', 0, 0, 0, 0.00, 0.00, 0),
('美食', 0, 0, 0, 0.00, 0.00, 0),
('音乐', 0, 0, 0, 0.00, 0.00, 0),
('舞蹈', 0, 0, 0, 0.00, 0.00, 0),
('游戏', 0, 0, 0, 0.00, 0.00, 0),
('知识', 0, 0, 0, 0.00, 0.00, 0),
('科技', 0, 0, 0, 0.00, 0.00, 0),
('生活', 0, 0, 0, 0.00, 0.00, 0),
('时尚', 0, 0, 0, 0.00, 0.00, 0),
('运动', 0, 0, 0, 0.00, 0.00, 0),
('旅行', 0, 0, 0, 0.00, 0.00, 0),
('萌宠', 0, 0, 0, 0.00, 0.00, 0),
('二次元', 0, 0, 0, 0.00, 0.00, 0),
('校园', 0, 0, 0, 0.00, 0.00, 0)
ON DUPLICATE KEY UPDATE `updated_at` = CURRENT_TIMESTAMP;

SELECT '推荐系统数据库表初始化完成' AS message;
