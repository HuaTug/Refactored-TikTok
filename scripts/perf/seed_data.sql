-- ============================================================================
--  Refactored-TikTok · 性能测试骨架数据
--  论文 §6.1 测试环境与方法 / §6 数据基础
-- ----------------------------------------------------------------------------
--  本脚本只造"骨架数据"，让大表查询走真实索引；不依赖真实 MinIO 文件。
--  -- 100,000 用户   (user_id 1,000,000 ~ 1,099,999)
--  -- 1,000,000 视频 (video_id 9,000,000 ~ 9,999,999)，复用同一段虚构 play_url
--  -- 5,000,000 条点赞 (覆盖前 1000 个热门视频)
--
--  执行（容器内）:
--    docker exec -i kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' TikTok \
--      < scripts/perf/seed_data.sql
-- ============================================================================

SET @@session.unique_checks = 0;
SET @@session.foreign_key_checks = 0;
SET @@session.sql_log_bin = 0;
SET @@session.transaction_isolation = 'READ-COMMITTED';
SET @@session.cte_max_recursion_depth = 5000000;

-- ---------- 0. 清场（可选，避免重复 seed） ---------------------------------
DELETE FROM video_likes WHERE user_id BETWEEN 1000000 AND 1099999;
DELETE FROM videos      WHERE video_id BETWEEN 9000000 AND 9999999;
DELETE FROM users       WHERE user_id  BETWEEN 1000000 AND 1099999;

-- ---------- 1. 100,000 用户 -------------------------------------------------
-- 用 RECURSIVE CTE 一次性插入；密码统一是 bcrypt('123456')，长度 60。
INSERT INTO users (user_id, user_name, password, email, sex, status,
                   created_at, updated_at, like_count)
WITH RECURSIVE seq(n) AS (
  SELECT 0
  UNION ALL
  SELECT n + 1 FROM seq WHERE n + 1 < 100000
)
SELECT
  1000000 + n,
  CONCAT('perf_u_', LPAD(n, 6, '0')),
  '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',  -- bcrypt('123456')
  CONCAT('perf_u_', LPAD(n, 6, '0'), '@example.com'),
  IF(n % 2 = 0, 1, 2),
  1,
  NOW() - INTERVAL FLOOR(RAND() * 365) DAY,
  NOW(),
  0
FROM seq;

-- ---------- 2. 1,000,000 视频 ---------------------------------------------
-- 每批 100k，分 10 批，避免 binlog 单事务过大。
DROP PROCEDURE IF EXISTS perf_seed_videos;
DELIMITER $$
CREATE PROCEDURE perf_seed_videos()
BEGIN
  DECLARE i INT DEFAULT 0;
  WHILE i < 10 DO
    INSERT INTO videos (video_id, user_id, video_url, cover_url,
                        title, description,
                        visit_count, likes_count, comment_count,
                        favorites_count, duration, open, audit_status,
                        category, label_names, created_at, updated_at)
    WITH RECURSIVE seq(n) AS (
      SELECT 0
      UNION ALL
      SELECT n + 1 FROM seq WHERE n + 1 < 100000
    )
    SELECT
      9000000 + i * 100000 + n,
      1000000 + (i * 100000 + n) % 100000,
      CONCAT('http://demo.local/perf/v_', i, '_', n, '.mp4'),
      CONCAT('http://demo.local/perf/c_', i, '_', n, '.jpg'),
      CONCAT('perf_video_', i, '_', n),
      'performance test seed',
      FLOOR(RAND() * 100000),
      0,
      0,
      0,
      30 + FLOOR(RAND() * 270),
      1, 1,
      ELT(1 + FLOOR(RAND() * 8),
          '运动','音乐','生活','搞笑','知识','游戏','美食','旅行'),
      'perf,seed',
      NOW() - INTERVAL FLOOR(RAND() * 30) DAY,
      NOW()
    FROM seq;
    SET i = i + 1;
  END WHILE;
END$$
DELIMITER ;
CALL perf_seed_videos();
DROP PROCEDURE perf_seed_videos;

-- ---------- 3. 5,000,000 点赞（集中于前 1000 热门视频） --------------------
-- 让点赞表自然倾斜，便于触发热点 key 路径。
INSERT IGNORE INTO video_likes (user_id, video_id, created_at)
WITH RECURSIVE seq(n) AS (
  SELECT 0 UNION ALL SELECT n + 1 FROM seq WHERE n + 1 < 5000000
)
SELECT
  1000000 + (n % 100000),                       -- 1 用户最多对 1 视频点赞 1 次（unique key 兜底）
  9000000 + (n % 1000),                         -- 集中前 1000 视频
  NOW() - INTERVAL FLOOR(RAND() * 30) DAY
FROM seq;

-- ---------- 4. 同步 likes_count 字段 ---------------------------------------
UPDATE videos v
JOIN (
  SELECT video_id, COUNT(*) cnt
  FROM video_likes
  WHERE video_id BETWEEN 9000000 AND 9000999
  GROUP BY video_id
) t ON v.video_id = t.video_id
SET v.likes_count = t.cnt;

-- ---------- 5. 校验输出 -----------------------------------------------------
SELECT
  (SELECT COUNT(*) FROM users  WHERE user_id  BETWEEN 1000000 AND 1099999)  AS users_seeded,
  (SELECT COUNT(*) FROM videos WHERE video_id BETWEEN 9000000 AND 9999999)  AS videos_seeded,
  (SELECT COUNT(*) FROM video_likes
                   WHERE video_id BETWEEN 9000000 AND 9000999)              AS likes_seeded;

SET @@session.foreign_key_checks = 1;
SET @@session.unique_checks = 1;
