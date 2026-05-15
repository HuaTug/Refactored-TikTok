-- =============================================================================
-- seed_demo_data.sql
-- 演示数据：更新 5 个测试用户头像 + 让 user_id=8 关注其他 4 人 + 更新计数
-- 用法：docker exec -i kitex_mysql mysql -u root -p'TikTok@MySQL#2025!Secure' --default-character-set=utf8mb4 TikTok < scripts/seed_demo_data.sql
-- =============================================================================

SET NAMES utf8mb4;
USE TikTok;

-- ===================== 1. 更新 5 个测试用户的头像和个人信息 =====================
-- 头像文件位于：
--   前端: Tiktok-web/public/avatars/user_{8..12}.jpg (dev server 直接访问 /avatars/xxx.jpg)
--   后端: Refactored-TikTok/static/avatars/user_{8..12}.jpg (git 仓库内备份)

UPDATE users SET
    avatar_url = '/avatars/user_8.jpg',
    bio = '热爱美食和旅行的博主 | 西北大学',
    location = '北京'
WHERE user_id = 8;

UPDATE users SET
    avatar_url = '/avatars/user_9.jpg',
    bio = '编程教学 | 分享技术干货 | 西北大学',
    location = '上海'
WHERE user_id = 9;

UPDATE users SET
    avatar_url = '/avatars/user_10.jpg',
    bio = '生活记录 | 日常vlog | 西北大学',
    location = '广州'
WHERE user_id = 10;

UPDATE users SET
    avatar_url = '/avatars/user_11.jpg',
    bio = '音乐推荐 | 搞笑视频 | 西北大学',
    location = '深圳'
WHERE user_id = 11;

UPDATE users SET
    avatar_url = '/avatars/user_12.jpg',
    bio = '健身打卡 | 科技数码 | 西北大学',
    location = '杭州'
WHERE user_id = 12;

-- 插入西北大学到 schools 表（幂等）
INSERT INTO schools (school_name, school_code, province, city, school_type, is_active)
VALUES ('西北大学', 'NWU', '陕西', '西安', 1, 1)
ON DUPLICATE KEY UPDATE school_name = '西北大学';

-- 关联 5 个用户到西北大学
SET @nwu_id = (SELECT school_id FROM schools WHERE school_code = 'NWU' LIMIT 1);
UPDATE users SET school_id = @nwu_id WHERE user_id IN (8, 9, 10, 11, 12);

-- ===================== 2. 插入关注关系（分库分表） =====================
-- 分片策略：follower_id % 4 = 分库, (follower_id / 4) % 4 = 分表
--
-- user_id=8 关注 user_id=9,10,11,12
-- follower_id=8: 8%4=0 → relation_db_0, (8/4)%4=2 → follows_2

-- 先清理可能存在的旧数据（幂等）
DELETE FROM relation_db_0.follows_2 WHERE follower_id = 8 AND user_id IN (9, 10, 11, 12);

INSERT INTO relation_db_0.follows_2 (user_id, follower_id, status, remark, created_at) VALUES
(9,  8, 1, '', NOW() - INTERVAL 4 DAY),
(10, 8, 1, '', NOW() - INTERVAL 3 DAY),
(11, 8, 1, '', NOW() - INTERVAL 2 DAY),
(12, 8, 1, '', NOW() - INTERVAL 1 DAY);

-- user_id=9 也关注 user_id=8（形成互关/好友）
-- follower_id=9: 9%4=1 → relation_db_1, (9/4)%4=2 → follows_2
DELETE FROM relation_db_1.follows_2 WHERE follower_id = 9 AND user_id = 8;
INSERT INTO relation_db_1.follows_2 (user_id, follower_id, status, remark, created_at) VALUES
(8, 9, 1, '', NOW() - INTERVAL 3 DAY);

-- user_id=10 也关注 user_id=8（互关）
-- follower_id=10: 10%4=2 → relation_db_2, (10/4)%4=2 → follows_2
DELETE FROM relation_db_2.follows_2 WHERE follower_id = 10 AND user_id = 8;
INSERT INTO relation_db_2.follows_2 (user_id, follower_id, status, remark, created_at) VALUES
(8, 10, 1, '', NOW() - INTERVAL 2 DAY);

-- ===================== 3. 更新用户的关注/粉丝计数 =====================
-- user_id=8: 关注了4人, 被2人关注 (9,10)
UPDATE users SET following_count = 4, follower_count = 2 WHERE user_id = 8;
-- user_id=9: 关注了1人(8), 被1人关注(8)
UPDATE users SET following_count = 1, follower_count = 1 WHERE user_id = 9;
-- user_id=10: 关注了1人(8), 被1人关注(8)
UPDATE users SET following_count = 1, follower_count = 1 WHERE user_id = 10;
-- user_id=11: 关注了0人, 被1人关注(8)
UPDATE users SET following_count = 0, follower_count = 1 WHERE user_id = 11;
-- user_id=12: 关注了0人, 被1人关注(8)
UPDATE users SET following_count = 0, follower_count = 1 WHERE user_id = 12;

-- ===================== 4. 更新分库的统计表 =====================
-- relation_db_0: user_id=8 的统计
INSERT INTO relation_db_0.user_relation_stats (user_id, following_count, follower_count, friend_count, mutual_follow_count)
VALUES (8, 4, 2, 2, 2)
ON DUPLICATE KEY UPDATE
    following_count = 4, follower_count = 2, friend_count = 2, mutual_follow_count = 2, updated_at = NOW();

-- relation_db_1: user_id=9 的统计
INSERT INTO relation_db_1.user_relation_stats (user_id, following_count, follower_count, friend_count, mutual_follow_count)
VALUES (9, 1, 1, 1, 1)
ON DUPLICATE KEY UPDATE
    following_count = 1, follower_count = 1, friend_count = 1, mutual_follow_count = 1, updated_at = NOW();

-- relation_db_2: user_id=10 的统计
INSERT INTO relation_db_2.user_relation_stats (user_id, following_count, follower_count, friend_count, mutual_follow_count)
VALUES (10, 1, 1, 1, 1)
ON DUPLICATE KEY UPDATE
    following_count = 1, follower_count = 1, friend_count = 1, mutual_follow_count = 1, updated_at = NOW();

SELECT '✅ 演示数据初始化完成！' AS status;
SELECT '   - 5个用户头像已更新 (user_id 8~12)' AS detail
UNION ALL SELECT '   - user_id=8 已关注 user_id=9,10,11,12'
UNION ALL SELECT '   - user_id=9,10 与 user_id=8 互关'
UNION ALL SELECT '   - 所有计数已同步';
