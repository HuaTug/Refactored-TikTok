DROP TABLE IF EXISTS video_hot_scores;
CREATE TABLE video_hot_scores (
    id BIGINT NOT NULL AUTO_INCREMENT,
    video_id BIGINT NOT NULL COMMENT '视频ID',
    time_window VARCHAR(10) NOT NULL COMMENT '时间窗口: 1h/6h/24h/7d',
    view_count BIGINT DEFAULT 0 COMMENT '观看次数',
    like_count BIGINT DEFAULT 0 COMMENT '点赞次数',
    comment_count BIGINT DEFAULT 0 COMMENT '评论次数',
    share_count BIGINT DEFAULT 0 COMMENT '分享次数',
    favorite_count BIGINT DEFAULT 0 COMMENT '收藏次数',
    hot_score DECIMAL(12,2) DEFAULT 0.00 COMMENT '综合热度分',
    `rank` INT DEFAULT 0 COMMENT '排名',
    window_start TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '窗口开始时间',
    window_end TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '窗口结束时间',
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_video_window (video_id, time_window),
    KEY idx_time_window (time_window),
    KEY idx_hot_score (hot_score DESC),
    KEY idx_rank (`rank`),
    KEY idx_window_start (window_start),
    KEY idx_updated_at (updated_at)
) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频实时热度表';
