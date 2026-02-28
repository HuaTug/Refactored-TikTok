-- ============================================================
-- 推荐系统测试数据集
-- 模拟 10 个用户、30 个视频、丰富的互动行为
-- 涵盖：冷启动用户、活跃用户、热门视频、冷门视频、多标签多分类
--
-- 使用方法（通过 Docker 中的 MySQL）：
--   docker exec -i kitex_mysql mysql -u root -p'TikTok@MySQL#2025!Secure' TikTok < scripts/seed_recommendation_testdata.sql
-- ============================================================

SET NAMES utf8mb4;

-- ============================================================
-- 1. 测试用户（ID 1000-1009，避免与现有用户冲突）
-- ============================================================
INSERT INTO users (user_id, user_name, password, email, sex, avatar_url, bio, video_count, like_count, following_count, follower_count, status, created_at, updated_at)
VALUES
  (1000, 'test_creator_a',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_a@test.com',   1, '/avatars/default.png', '美食博主，分享各种美食制作教程', 8, 0, 5, 120, 1, NOW() - INTERVAL 60 DAY, NOW()),
  (1001, 'test_creator_b',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_b@test.com',   0, '/avatars/default.png', '旅行vlogger，记录世界各地的风景', 6, 0, 8, 200, 1, NOW() - INTERVAL 45 DAY, NOW()),
  (1002, 'test_creator_c',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_c@test.com',   1, '/avatars/default.png', '科技数码评测达人', 5, 0, 3, 80, 1, NOW() - INTERVAL 30 DAY, NOW()),
  (1003, 'test_creator_d',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_d@test.com',   0, '/avatars/default.png', '健身教练，每天分享健身知识', 4, 0, 2, 50, 1, NOW() - INTERVAL 20 DAY, NOW()),
  (1004, 'test_creator_e',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_e@test.com',   1, '/avatars/default.png', '搞笑段子手', 4, 0, 10, 500, 1, NOW() - INTERVAL 90 DAY, NOW()),
  (1005, 'test_creator_f',   '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'creator_f@test.com',   0, '/avatars/default.png', '音乐人，原创歌曲分享', 3, 0, 1, 30, 1, NOW() - INTERVAL 15 DAY, NOW()),
  (1006, 'test_viewer_x',    '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'viewer_x@test.com',    1, '/avatars/default.png', '只看不发的观众用户', 0, 0, 15, 2, 1, NOW() - INTERVAL 10 DAY, NOW()),
  (1007, 'test_viewer_y',    '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'viewer_y@test.com',    0, '/avatars/default.png', '偶尔互动的普通用户', 0, 0, 6, 5, 1, NOW() - INTERVAL 5 DAY, NOW()),
  (1008, 'test_newuser_cold', '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'cold@test.com',       1, '/avatars/default.png', '刚注册的新用户', 0, 0, 0, 0, 1, NOW() - INTERVAL 1 DAY, NOW()),
  (1009, 'test_power_user',  '$2a$05$6fLJrZRDVVf068ChSyxLbuxIeXW9MgQ28Bs2uj9CrkpqogKzWlvWC', 'power@test.com',      0, '/avatars/default.png', '重度用户，什么都看', 0, 0, 20, 15, 1, NOW() - INTERVAL 40 DAY, NOW())
ON DUPLICATE KEY UPDATE user_name = VALUES(user_name);

-- ============================================================
-- 2. 测试视频（ID 10000-10029，30 个视频，6 个分类）
-- ============================================================
INSERT INTO videos (video_id, user_id, title, description, video_url, cover_url, duration, visit_count, likes_count, comment_count, share_count, favorites_count, label_names, category, open, audit_status, created_at, updated_at)
VALUES
  -- 美食分类（creator_a, 8个视频）
  (10000, 1000, '红烧肉的做法｜入口即化',       '详细教你做出米其林级别的红烧肉，肥而不腻，入口即化，配饭一绝',   '/videos/10000.mp4', '/covers/10000.jpg', 180, 15000, 2300, 450, 120, 300, '美食,红烧肉,家常菜,做饭教程',  '美食', 1, 1, NOW() - INTERVAL 55 DAY, NOW()),
  (10001, 1000, '3分钟学会做蛋炒饭',           '最简单的蛋炒饭教程，新手也能做出粒粒分明的黄金蛋炒饭',         '/videos/10001.mp4', '/covers/10001.jpg', 200, 28000, 5100, 800, 350, 600, '美食,蛋炒饭,快手菜,新手友好',  '美食', 1, 1, NOW() - INTERVAL 50 DAY, NOW()),
  (10002, 1000, '广东煲汤｜花胶鸡汤',           '正宗广东煲汤手法，花胶鸡汤补气养颜，全家都爱喝',               '/videos/10002.mp4', '/covers/10002.jpg', 300, 8000,  1200, 200, 80,  150, '美食,煲汤,广东,养生',          '美食', 1, 1, NOW() - INTERVAL 45 DAY, NOW()),
  (10003, 1000, '日式拉面在家做',              '不用出门也能吃到正宗日式豚骨拉面，汤底浓郁，面条劲道',         '/videos/10003.mp4', '/covers/10003.jpg', 420, 12000, 1800, 300, 90,  200, '美食,拉面,日料,面食',          '美食', 1, 1, NOW() - INTERVAL 40 DAY, NOW()),
  (10004, 1000, '超简单提拉米苏',              '零失败提拉米苏教程，用料简单，口感丝滑',                       '/videos/10004.mp4', '/covers/10004.jpg', 250, 6000,  900,  150, 60,  180, '美食,甜点,提拉米苏,烘焙',      '美食', 1, 1, NOW() - INTERVAL 35 DAY, NOW()),
  (10005, 1000, '韩式炸鸡配啤酒',              '外酥里嫩的韩式炸鸡，酱料调配秘方大公开',                       '/videos/10005.mp4', '/covers/10005.jpg', 280, 20000, 3500, 600, 200, 400, '美食,炸鸡,韩式,下酒菜',        '美食', 1, 1, NOW() - INTERVAL 25 DAY, NOW()),
  (10006, 1000, '减脂餐一周食谱',              '健康减脂不挨饿，一周七天不重样的减脂餐搭配',                   '/videos/10006.mp4', '/covers/10006.jpg', 350, 9500,  1500, 280, 150, 250, '美食,减脂,健康饮食,食谱',      '美食', 1, 1, NOW() - INTERVAL 15 DAY, NOW()),
  (10007, 1000, '自制珍珠奶茶',                '在家做珍珠奶茶，珍珠Q弹，奶茶香浓，比外面买的好喝',           '/videos/10007.mp4', '/covers/10007.jpg', 200, 18000, 3200, 500, 180, 350, '美食,奶茶,饮品,自制',          '美食', 1, 1, NOW() - INTERVAL 8 DAY, NOW()),
  -- 旅行分类（creator_b, 6个视频）
  (10008, 1001, '云南大理三日游攻略',           '最详细的大理旅行攻略，交通住宿美食景点一网打尽',               '/videos/10008.mp4', '/covers/10008.jpg', 600, 25000, 4200, 700, 300, 500, '旅行,云南,大理,攻略',          '旅行', 1, 1, NOW() - INTERVAL 40 DAY, NOW()),
  (10009, 1001, '日本东京五天四夜',             '东京自由行完整攻略，涵盖浅草寺秋叶原新宿涩谷',               '/videos/10009.mp4', '/covers/10009.jpg', 720, 32000, 5800, 900, 450, 700, '旅行,日本,东京,自由行',        '旅行', 1, 1, NOW() - INTERVAL 35 DAY, NOW()),
  (10010, 1001, '新疆自驾环线15天',             '独库公路到赛里木湖，新疆最美自驾路线实拍',                     '/videos/10010.mp4', '/covers/10010.jpg', 900, 18000, 3000, 500, 250, 400, '旅行,新疆,自驾,风景',          '旅行', 1, 1, NOW() - INTERVAL 28 DAY, NOW()),
  (10011, 1001, '曼谷清迈双城记',              '泰国最热门的两座城市，寺庙夜市美食海滩全覆盖',               '/videos/10011.mp4', '/covers/10011.jpg', 500, 14000, 2200, 400, 180, 300, '旅行,泰国,曼谷,清迈',          '旅行', 1, 1, NOW() - INTERVAL 20 DAY, NOW()),
  (10012, 1001, '冰岛极光之旅',                '追逐北极光，冰岛冬季旅行最全指南，附拍摄技巧',               '/videos/10012.mp4', '/covers/10012.jpg', 480, 22000, 4000, 650, 320, 550, '旅行,冰岛,极光,摄影',          '旅行', 1, 1, NOW() - INTERVAL 12 DAY, NOW()),
  (10013, 1001, '成都周末美食之旅',             '成都48小时怎么吃？从火锅到串串，从兔头到冰粉',               '/videos/10013.mp4', '/covers/10013.jpg', 400, 30000, 5500, 850, 400, 650, '旅行,成都,美食,火锅',          '旅行', 1, 1, NOW() - INTERVAL 5 DAY, NOW()),
  -- 科技分类（creator_c, 5个视频）
  (10014, 1002, 'iPhone 16 Pro 深度评测',       '全面评测iPhone 16 Pro，相机提升巨大，但还有这些不足',          '/videos/10014.mp4', '/covers/10014.jpg', 900, 45000, 6000, 1200, 500, 800, '科技,iPhone,苹果,评测',        '科技', 1, 1, NOW() - INTERVAL 25 DAY, NOW()),
  (10015, 1002, 'M4 MacBook Pro 办公体验',      '用了一个月M4 MacBook Pro，聊聊真实办公体验和续航',             '/videos/10015.mp4', '/covers/10015.jpg', 720, 28000, 3800, 600, 300, 500, '科技,MacBook,苹果,办公',       '科技', 1, 1, NOW() - INTERVAL 18 DAY, NOW()),
  (10016, 1002, '2025年最值得买的耳机',         '横评10款千元耳机，音质降噪续航全方位对比',                     '/videos/10016.mp4', '/covers/10016.jpg', 600, 20000, 2800, 450, 200, 350, '科技,耳机,评测,数码',          '科技', 1, 1, NOW() - INTERVAL 10 DAY, NOW()),
  (10017, 1002, 'AI编程工具大比拼',             'Copilot vs Cursor vs CodeBuddy，谁才是最好的AI编程助手',      '/videos/10017.mp4', '/covers/10017.jpg', 800, 35000, 5200, 900, 400, 600, '科技,AI,编程,工具',            '科技', 1, 1, NOW() - INTERVAL 6 DAY, NOW()),
  (10018, 1002, '小米SU7真实车主半年体验',       '提车半年，聊聊小米SU7的优缺点和真实能耗',                     '/videos/10018.mp4', '/covers/10018.jpg', 660, 40000, 5500, 1100, 450, 700, '科技,小米,电动车,评测',        '科技', 1, 1, NOW() - INTERVAL 3 DAY, NOW()),
  -- 健身分类（creator_d, 4个视频）
  (10019, 1003, '新手健身入门完整计划',          '零基础也能练，8周健身入门计划详细讲解',                       '/videos/10019.mp4', '/covers/10019.jpg', 500, 16000, 2500, 400, 150, 300, '健身,新手,入门,训练计划',       '健身', 1, 1, NOW() - INTERVAL 18 DAY, NOW()),
  (10020, 1003, '在家就能做的HIIT燃脂',         '不需要器械，20分钟高效燃脂HIIT训练',                         '/videos/10020.mp4', '/covers/10020.jpg', 1200,22000, 3800, 550, 250, 400, '健身,HIIT,燃脂,居家运动',       '健身', 1, 1, NOW() - INTERVAL 12 DAY, NOW()),
  (10021, 1003, '肩颈放松拉伸教程',             '久坐上班族必看！10分钟缓解肩颈酸痛',                         '/videos/10021.mp4', '/covers/10021.jpg', 600, 30000, 4500, 700, 350, 500, '健身,拉伸,肩颈,上班族',        '健身', 1, 1, NOW() - INTERVAL 7 DAY, NOW()),
  (10022, 1003, '增肌饮食全攻略',               '蛋白质怎么补充？增肌期间饮食搭配终极指南',                     '/videos/10022.mp4', '/covers/10022.jpg', 450, 12000, 1800, 300, 120, 200, '健身,增肌,饮食,蛋白质',        '健身', 1, 1, NOW() - INTERVAL 2 DAY, NOW()),
  -- 搞笑分类（creator_e, 4个视频, 高互动）
  (10023, 1004, '当代打工人的一天',             '打工人打工魂，打工都是人上人',                               '/videos/10023.mp4', '/covers/10023.jpg', 60,  80000, 15000, 3000, 2000, 1500, '搞笑,打工人,日常,段子',       '搞笑', 1, 1, NOW() - INTERVAL 30 DAY, NOW()),
  (10024, 1004, '猫咪的迷惑行为合集',           '我家猫今天又在干什么奇怪的事情了',                           '/videos/10024.mp4', '/covers/10024.jpg', 90,  65000, 12000, 2500, 1800, 1200, '搞笑,猫咪,宠物,可爱',        '搞笑', 1, 1, NOW() - INTERVAL 22 DAY, NOW()),
  (10025, 1004, '外卖小哥的神操作',             '外卖小哥翻墙送餐？这操作我给满分',                           '/videos/10025.mp4', '/covers/10025.jpg', 45,  50000, 9000,  1800, 1200, 900,  '搞笑,外卖,神操作,生活',      '搞笑', 1, 1, NOW() - INTERVAL 14 DAY, NOW()),
  (10026, 1004, '办公室整蛊大赛',               '同事间的整蛊日常，友谊的小船说翻就翻',                       '/videos/10026.mp4', '/covers/10026.jpg', 120, 55000, 10000, 2200, 1500, 1100, '搞笑,整蛊,办公室,日常',      '搞笑', 1, 1, NOW() - INTERVAL 4 DAY, NOW()),
  -- 音乐分类（creator_f, 3个视频）
  (10027, 1005, '吉他弹唱《晴天》',             '周杰伦经典曲目吉他弹唱教学，附和弦谱',                       '/videos/10027.mp4', '/covers/10027.jpg', 300, 10000, 1500, 250, 80,  200, '音乐,吉他,周杰伦,弹唱',       '音乐', 1, 1, NOW() - INTERVAL 12 DAY, NOW()),
  (10028, 1005, '原创歌曲《城市的光》',          '写给每一个在大城市打拼的你，希望你被这个世界温柔以待',         '/videos/10028.mp4', '/covers/10028.jpg', 240, 7000,  1100, 180, 60,  150, '音乐,原创,城市,治愈',          '音乐', 1, 1, NOW() - INTERVAL 8 DAY, NOW()),
  (10029, 1005, '零基础学钢琴第一课',           '从零开始学钢琴，认识键盘和基本指法',                         '/videos/10029.mp4', '/covers/10029.jpg', 600, 5000,  800,  120, 40,  100, '音乐,钢琴,零基础,教程',       '音乐', 1, 1, NOW() - INTERVAL 3 DAY, NOW())
ON DUPLICATE KEY UPDATE title = VALUES(title);


-- ============================================================
-- 3. 视频特征表（video_features）
-- ============================================================
INSERT INTO video_features (video_id, quality_score, popularity_score, freshness_score, ctr, finish_rate, like_rate, comment_rate, share_rate, favorite_rate, interact_score, exposure_count, click_count, author_score, is_high_quality, created_at, updated_at)
VALUES
  (10000, 7.5, 8710,   4.0, 0.620000, 0.5500, 0.1530, 0.0300, 0.0080, 0.0200, 8710,   18000, 15000, 7.0, 1, NOW() - INTERVAL 55 DAY, NOW()),
  (10001, 8.0, 19950,  5.0, 0.750000, 0.6800, 0.1820, 0.0290, 0.0130, 0.0210, 19950,  33000, 28000, 7.0, 1, NOW() - INTERVAL 50 DAY, NOW()),
  (10002, 7.0, 5200,   4.5, 0.500000, 0.4500, 0.1500, 0.0250, 0.0100, 0.0190, 5200,   10000, 8000,  7.0, 0, NOW() - INTERVAL 45 DAY, NOW()),
  (10003, 7.5, 7740,   5.0, 0.580000, 0.5000, 0.1500, 0.0250, 0.0080, 0.0170, 7740,   14000, 12000, 7.0, 1, NOW() - INTERVAL 40 DAY, NOW()),
  (10004, 6.5, 4380,   5.5, 0.480000, 0.4200, 0.1500, 0.0250, 0.0100, 0.0300, 4380,   8000,  6000,  7.0, 0, NOW() - INTERVAL 35 DAY, NOW()),
  (10005, 8.0, 14700,  6.5, 0.700000, 0.6000, 0.1750, 0.0300, 0.0100, 0.0200, 14700,  24000, 20000, 7.0, 1, NOW() - INTERVAL 25 DAY, NOW()),
  (10006, 7.0, 6640,   7.5, 0.550000, 0.4800, 0.1580, 0.0290, 0.0160, 0.0260, 6640,   12000, 9500,  7.0, 0, NOW() - INTERVAL 15 DAY, NOW()),
  (10007, 8.5, 13640,  9.0, 0.720000, 0.6500, 0.1780, 0.0280, 0.0100, 0.0190, 13640,  22000, 18000, 7.0, 1, NOW() - INTERVAL 8 DAY, NOW()),
  (10008, 8.5, 16350,  5.0, 0.680000, 0.5200, 0.1680, 0.0280, 0.0120, 0.0200, 16350,  30000, 25000, 8.0, 1, NOW() - INTERVAL 40 DAY, NOW()),
  (10009, 9.0, 25100,  5.5, 0.780000, 0.5500, 0.1810, 0.0280, 0.0140, 0.0220, 25100,  38000, 32000, 8.0, 1, NOW() - INTERVAL 35 DAY, NOW()),
  (10010, 8.0, 13500,  6.5, 0.620000, 0.4800, 0.1670, 0.0280, 0.0140, 0.0220, 13500,  22000, 18000, 8.0, 1, NOW() - INTERVAL 28 DAY, NOW()),
  (10011, 7.5, 8840,   7.0, 0.580000, 0.5000, 0.1570, 0.0290, 0.0130, 0.0210, 8840,   17000, 14000, 8.0, 0, NOW() - INTERVAL 20 DAY, NOW()),
  (10012, 9.0, 18310,  8.5, 0.720000, 0.5500, 0.1820, 0.0300, 0.0150, 0.0250, 18310,  28000, 22000, 8.0, 1, NOW() - INTERVAL 12 DAY, NOW()),
  (10013, 9.5, 27250,  9.5, 0.800000, 0.6000, 0.1830, 0.0280, 0.0130, 0.0220, 27250,  36000, 30000, 8.0, 1, NOW() - INTERVAL 5 DAY, NOW()),
  (10014, 9.0, 34200,  6.5, 0.820000, 0.5800, 0.1330, 0.0270, 0.0110, 0.0180, 34200,  50000, 45000, 7.5, 1, NOW() - INTERVAL 25 DAY, NOW()),
  (10015, 8.0, 17700,  7.5, 0.720000, 0.5500, 0.1360, 0.0210, 0.0110, 0.0180, 17700,  33000, 28000, 7.5, 1, NOW() - INTERVAL 18 DAY, NOW()),
  (10016, 7.5, 12000,  8.0, 0.650000, 0.5000, 0.1400, 0.0230, 0.0100, 0.0180, 12000,  24000, 20000, 7.5, 1, NOW() - INTERVAL 10 DAY, NOW()),
  (10017, 9.5, 27200,  9.0, 0.800000, 0.5800, 0.1490, 0.0260, 0.0110, 0.0170, 27200,  40000, 35000, 7.5, 1, NOW() - INTERVAL 6 DAY, NOW()),
  (10018, 9.0, 32600,  9.8, 0.850000, 0.6000, 0.1380, 0.0280, 0.0110, 0.0180, 32600,  45000, 40000, 7.5, 1, NOW() - INTERVAL 3 DAY, NOW()),
  (10019, 7.5, 10900,  7.5, 0.600000, 0.5200, 0.1560, 0.0250, 0.0090, 0.0190, 10900,  20000, 16000, 6.5, 1, NOW() - INTERVAL 18 DAY, NOW()),
  (10020, 8.5, 16550,  8.0, 0.700000, 0.6000, 0.1730, 0.0250, 0.0110, 0.0180, 16550,  28000, 22000, 6.5, 1, NOW() - INTERVAL 12 DAY, NOW()),
  (10021, 9.0, 24050,  9.0, 0.780000, 0.6500, 0.1500, 0.0230, 0.0120, 0.0170, 24050,  35000, 30000, 6.5, 1, NOW() - INTERVAL 7 DAY, NOW()),
  (10022, 7.0, 7260,   9.5, 0.580000, 0.5000, 0.1500, 0.0250, 0.0100, 0.0170, 7260,   15000, 12000, 6.5, 0, NOW() - INTERVAL 2 DAY, NOW()),
  (10023, 8.0, 131000, 5.5, 0.900000, 0.8000, 0.1880, 0.0380, 0.0250, 0.0190, 131000, 90000, 80000, 9.0, 1, NOW() - INTERVAL 30 DAY, NOW()),
  (10024, 8.0, 107500, 6.5, 0.880000, 0.7800, 0.1850, 0.0380, 0.0280, 0.0180, 107500, 75000, 65000, 9.0, 1, NOW() - INTERVAL 22 DAY, NOW()),
  (10025, 7.5, 81600,  7.5, 0.850000, 0.7500, 0.1800, 0.0360, 0.0240, 0.0180, 81600,  58000, 50000, 9.0, 1, NOW() - INTERVAL 14 DAY, NOW()),
  (10026, 8.5, 97000,  9.5, 0.880000, 0.7800, 0.1820, 0.0400, 0.0270, 0.0200, 97000,  62000, 55000, 9.0, 1, NOW() - INTERVAL 4 DAY, NOW()),
  (10027, 7.0, 5640,   8.0, 0.520000, 0.5800, 0.1500, 0.0250, 0.0080, 0.0200, 5640,   13000, 10000, 5.5, 0, NOW() - INTERVAL 12 DAY, NOW()),
  (10028, 7.5, 4480,   8.5, 0.480000, 0.6200, 0.1570, 0.0260, 0.0090, 0.0210, 4480,   10000, 7000,  5.5, 0, NOW() - INTERVAL 8 DAY, NOW()),
  (10029, 6.5, 2960,   9.5, 0.420000, 0.5500, 0.1600, 0.0240, 0.0080, 0.0200, 2960,   7000,  5000,  5.5, 0, NOW() - INTERVAL 3 DAY, NOW())
ON DUPLICATE KEY UPDATE quality_score = VALUES(quality_score), popularity_score = VALUES(popularity_score);


-- ============================================================
-- 4. 标签-视频映射（tag_video_mappings）
-- 真实列: id(auto), tag_name, video_id, weight, source, created_at
-- ============================================================
INSERT INTO tag_video_mappings (tag_name, video_id, weight, source) VALUES
  ('美食',10000,1.0000,'seed'),('红烧肉',10000,1.0000,'seed'),('家常菜',10000,0.8000,'seed'),('做饭教程',10000,0.8000,'seed'),
  ('美食',10001,1.0000,'seed'),('蛋炒饭',10001,1.0000,'seed'),('快手菜',10001,0.8000,'seed'),('新手友好',10001,0.8000,'seed'),
  ('美食',10002,1.0000,'seed'),('煲汤',10002,1.0000,'seed'),('广东',10002,0.8000,'seed'),('养生',10002,0.8000,'seed'),
  ('美食',10003,1.0000,'seed'),('拉面',10003,1.0000,'seed'),('日料',10003,0.8000,'seed'),('面食',10003,0.8000,'seed'),
  ('美食',10004,1.0000,'seed'),('甜点',10004,1.0000,'seed'),('提拉米苏',10004,0.8000,'seed'),('烘焙',10004,0.8000,'seed'),
  ('美食',10005,1.0000,'seed'),('炸鸡',10005,1.0000,'seed'),('韩式',10005,0.8000,'seed'),('下酒菜',10005,0.8000,'seed'),
  ('美食',10006,1.0000,'seed'),('减脂',10006,1.0000,'seed'),('健康饮食',10006,0.8000,'seed'),('食谱',10006,0.8000,'seed'),
  ('美食',10007,1.0000,'seed'),('奶茶',10007,1.0000,'seed'),('饮品',10007,0.8000,'seed'),('自制',10007,0.8000,'seed'),
  ('旅行',10008,1.0000,'seed'),('云南',10008,1.0000,'seed'),('大理',10008,0.8000,'seed'),('攻略',10008,0.8000,'seed'),
  ('旅行',10009,1.0000,'seed'),('日本',10009,1.0000,'seed'),('东京',10009,0.8000,'seed'),('自由行',10009,0.8000,'seed'),
  ('旅行',10010,1.0000,'seed'),('新疆',10010,1.0000,'seed'),('自驾',10010,0.8000,'seed'),('风景',10010,0.8000,'seed'),
  ('旅行',10011,1.0000,'seed'),('泰国',10011,1.0000,'seed'),('曼谷',10011,0.8000,'seed'),('清迈',10011,0.8000,'seed'),
  ('旅行',10012,1.0000,'seed'),('冰岛',10012,1.0000,'seed'),('极光',10012,0.8000,'seed'),('摄影',10012,0.8000,'seed'),
  ('旅行',10013,1.0000,'seed'),('成都',10013,1.0000,'seed'),('美食',10013,0.8000,'seed'),('火锅',10013,0.8000,'seed'),
  ('科技',10014,1.0000,'seed'),('iPhone',10014,1.0000,'seed'),('苹果',10014,0.8000,'seed'),('评测',10014,0.8000,'seed'),
  ('科技',10015,1.0000,'seed'),('MacBook',10015,1.0000,'seed'),('苹果',10015,0.8000,'seed'),('办公',10015,0.8000,'seed'),
  ('科技',10016,1.0000,'seed'),('耳机',10016,1.0000,'seed'),('评测',10016,0.8000,'seed'),('数码',10016,0.8000,'seed'),
  ('科技',10017,1.0000,'seed'),('AI',10017,1.0000,'seed'),('编程',10017,0.8000,'seed'),('工具',10017,0.8000,'seed'),
  ('科技',10018,1.0000,'seed'),('小米',10018,1.0000,'seed'),('电动车',10018,0.8000,'seed'),('评测',10018,0.8000,'seed'),
  ('健身',10019,1.0000,'seed'),('新手',10019,1.0000,'seed'),('入门',10019,0.8000,'seed'),('训练计划',10019,0.8000,'seed'),
  ('健身',10020,1.0000,'seed'),('HIIT',10020,1.0000,'seed'),('燃脂',10020,0.8000,'seed'),('居家运动',10020,0.8000,'seed'),
  ('健身',10021,1.0000,'seed'),('拉伸',10021,1.0000,'seed'),('肩颈',10021,0.8000,'seed'),('上班族',10021,0.8000,'seed'),
  ('健身',10022,1.0000,'seed'),('增肌',10022,1.0000,'seed'),('饮食',10022,0.8000,'seed'),('蛋白质',10022,0.8000,'seed'),
  ('搞笑',10023,1.0000,'seed'),('打工人',10023,1.0000,'seed'),('日常',10023,0.8000,'seed'),('段子',10023,0.8000,'seed'),
  ('搞笑',10024,1.0000,'seed'),('猫咪',10024,1.0000,'seed'),('宠物',10024,0.8000,'seed'),('可爱',10024,0.8000,'seed'),
  ('搞笑',10025,1.0000,'seed'),('外卖',10025,1.0000,'seed'),('神操作',10025,0.8000,'seed'),('生活',10025,0.8000,'seed'),
  ('搞笑',10026,1.0000,'seed'),('整蛊',10026,1.0000,'seed'),('办公室',10026,0.8000,'seed'),('日常',10026,0.8000,'seed'),
  ('音乐',10027,1.0000,'seed'),('吉他',10027,1.0000,'seed'),('周杰伦',10027,0.8000,'seed'),('弹唱',10027,0.8000,'seed'),
  ('音乐',10028,1.0000,'seed'),('原创',10028,1.0000,'seed'),('城市',10028,0.8000,'seed'),('治愈',10028,0.8000,'seed'),
  ('音乐',10029,1.0000,'seed'),('钢琴',10029,1.0000,'seed'),('零基础',10029,0.8000,'seed'),('教程',10029,0.8000,'seed')
ON DUPLICATE KEY UPDATE weight = VALUES(weight);


-- ============================================================
-- 5. 分类视频统计（category_video_stats）
-- 真实列: id(auto), category, total_videos, total_views, total_likes, avg_quality, hot_score, daily_new_videos, updated_at
-- ============================================================
INSERT INTO category_video_stats (category, total_videos, total_views, total_likes, avg_quality, hot_score, daily_new_videos) VALUES
  ('美食', 8,  116500, 18500, 7.50, 85000,  1),
  ('旅行', 6,  141000, 25700, 8.50, 110000, 1),
  ('科技', 5,  168000, 23300, 8.50, 130000, 1),
  ('健身', 4,  80000,  12600, 8.00, 60000,  1),
  ('搞笑', 4,  250000, 46000, 8.00, 420000, 1),
  ('音乐', 3,  22000,  3400,  7.00, 13000,  0)
ON DUPLICATE KEY UPDATE total_videos = VALUES(total_videos), total_views = VALUES(total_views);


-- ============================================================
-- 6. 作者评分（author_scores）
-- ============================================================
INSERT INTO author_scores (author_id, quality_score, activity_score, influence_score, growth_score, overall_score, total_videos, avg_video_quality, avg_video_views, avg_engagement_rate, level, is_verified, created_at, updated_at)
VALUES
  (1000, 7.5, 8.0, 6.0, 5.0, 6.6, 8, 7.5, 14563,  0.1800, 5, 0, NOW(), NOW()),
  (1001, 8.5, 7.0, 7.5, 6.0, 7.3, 6, 8.5, 23500,  0.1900, 6, 1, NOW(), NOW()),
  (1002, 8.0, 6.5, 8.0, 7.0, 7.4, 5, 8.0, 33600,  0.1600, 7, 1, NOW(), NOW()),
  (1003, 7.0, 5.5, 5.0, 6.5, 6.0, 4, 8.0, 20000,  0.1700, 4, 0, NOW(), NOW()),
  (1004, 8.0, 6.0, 9.5, 3.0, 6.6, 4, 8.0, 62500,  0.2200, 8, 1, NOW(), NOW()),
  (1005, 7.0, 4.5, 3.0, 7.0, 5.4, 3, 7.0, 7333.33,0.1800, 3, 0, NOW(), NOW())
ON DUPLICATE KEY UPDATE quality_score = VALUES(quality_score), overall_score = VALUES(overall_score);


-- ============================================================
-- 7. 用户画像（user_profiles）
-- 真实列: user_id(PK), interest_tags(JSON), category_preference(JSON), author_preference(JSON),
--         topic_preference(JSON), active_time_slots(JSON), avg_watch_duration, avg_completion_rate,
--         like_rate, comment_rate, share_rate, total_view_count, total_like_count,
--         total_comment_count, total_share_count, user_level, content_quality_pref,
--         video_duration_pref, last_active_at, created_at, updated_at
-- ============================================================
INSERT INTO user_profiles (user_id, interest_tags, category_preference, avg_watch_duration, avg_completion_rate, like_rate, comment_rate, share_rate, total_view_count, total_like_count, total_comment_count, total_share_count, user_level, content_quality_pref, video_duration_pref, last_active_at)
VALUES
  (1000, '["美食","做饭","烘焙","日料"]',             '{"美食":0.6,"旅行":0.2,"搞笑":0.2}',                                    180.0, 0.6500, 0.4000, 0.1500, 0.0500, 200,  80,  30, 10, 5, 4, 3, NOW()),
  (1001, '["旅行","摄影","美食","风景"]',             '{"旅行":0.5,"美食":0.3,"搞笑":0.2}',                                    200.0, 0.7000, 0.3400, 0.1300, 0.0570, 350,  120, 45, 20, 5, 5, 4, NOW()),
  (1002, '["科技","数码","AI","编程"]',               '{"科技":0.7,"搞笑":0.2,"健身":0.1}',                                    300.0, 0.6000, 0.3000, 0.1200, 0.0500, 500,  150, 60, 25, 6, 5, 4, NOW()),
  (1003, '["健身","运动","饮食","减脂"]',             '{"健身":0.6,"美食":0.2,"科技":0.2}',                                    150.0, 0.5500, 0.4000, 0.1300, 0.0530, 150,  60,  20, 8,  4, 3, 2, NOW()),
  (1004, '["搞笑","段子","猫咪","日常"]',             '{"搞笑":0.7,"美食":0.2,"旅行":0.1}',                                    60.0,  0.8500, 0.3750, 0.1250, 0.0625, 800,  300, 100,50, 7, 1, 1, NOW()),
  (1005, '["音乐","吉他","钢琴","原创"]',             '{"音乐":0.6,"搞笑":0.2,"旅行":0.2}',                                    250.0, 0.6500, 0.4000, 0.1500, 0.0500, 100,  40,  15, 5,  3, 4, 3, NOW()),
  (1006, '["科技","iPhone","AI","搞笑","猫咪"]',      '{"科技":0.4,"搞笑":0.4,"健身":0.2}',                                    250.0, 0.7500, 0.3300, 0.0830, 0.0250, 600,  200, 50, 15, 4, 5, 4, NOW()),
  (1007, '["旅行","美食","火锅","日本"]',             '{"旅行":0.5,"美食":0.4,"音乐":0.1}',                                    180.0, 0.8000, 0.3200, 0.0800, 0.0320, 250,  80,  20, 8,  3, 3, 3, NOW()),
  (1008, '[]',                                        '{}',                                                                      0.0,   0.0000, 0.0000, 0.0000, 0.0000, 0,    0,   0,  0,  1, 3, 2, NOW()),
  (1009, '["科技","美食","旅行","健身","搞笑","音乐"]','{"科技":0.25,"美食":0.20,"搞笑":0.20,"旅行":0.15,"健身":0.10,"音乐":0.10}', 200.0, 0.8500, 0.3330, 0.1000, 0.0330, 1200, 400, 120,40, 6, 4, 3, NOW())
ON DUPLICATE KEY UPDATE interest_tags = VALUES(interest_tags), category_preference = VALUES(category_preference);


-- ============================================================
-- 8. 用户行为记录（user_behaviors）
-- ============================================================
INSERT INTO user_behaviors (user_id, video_id, behavior_type, behavior_time, created_at) VALUES
  -- viewer_x (1006) 科技+搞笑
  (1006, 10014, 'view',    NOW() - INTERVAL 20 DAY, NOW() - INTERVAL 20 DAY),
  (1006, 10014, 'like',    NOW() - INTERVAL 20 DAY, NOW() - INTERVAL 20 DAY),
  (1006, 10017, 'view',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1006, 10017, 'like',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1006, 10017, 'comment', NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1006, 10018, 'view',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1006, 10018, 'like',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1006, 10015, 'view',    NOW() - INTERVAL 8 DAY,  NOW() - INTERVAL 8 DAY),
  (1006, 10016, 'view',    NOW() - INTERVAL 5 DAY,  NOW() - INTERVAL 5 DAY),
  (1006, 10023, 'view',    NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1006, 10023, 'like',    NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1006, 10023, 'share',   NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1006, 10024, 'view',    NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1006, 10024, 'like',    NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1006, 10026, 'view',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1006, 10026, 'like',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1006, 10020, 'view',    NOW() - INTERVAL 6 DAY,  NOW() - INTERVAL 6 DAY),
  (1006, 10021, 'view',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  (1006, 10021, 'like',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  -- viewer_y (1007) 旅行+美食
  (1007, 10008, 'view',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1007, 10008, 'like',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1007, 10009, 'view',    NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1007, 10009, 'like',    NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1007, 10009, 'comment', NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1007, 10012, 'view',    NOW() - INTERVAL 8 DAY,  NOW() - INTERVAL 8 DAY),
  (1007, 10012, 'like',    NOW() - INTERVAL 8 DAY,  NOW() - INTERVAL 8 DAY),
  (1007, 10013, 'view',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1007, 10013, 'like',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1007, 10013, 'share',   NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1007, 10001, 'view',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1007, 10001, 'like',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1007, 10005, 'view',    NOW() - INTERVAL 6 DAY,  NOW() - INTERVAL 6 DAY),
  (1007, 10007, 'view',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  (1007, 10007, 'like',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  (1007, 10011, 'view',    NOW() - INTERVAL 5 DAY,  NOW() - INTERVAL 5 DAY),
  (1007, 10027, 'view',    NOW() - INTERVAL 4 DAY,  NOW() - INTERVAL 4 DAY),
  -- power_user (1009) 全品类
  (1009, 10014, 'view',    NOW() - INTERVAL 25 DAY, NOW() - INTERVAL 25 DAY),
  (1009, 10014, 'like',    NOW() - INTERVAL 25 DAY, NOW() - INTERVAL 25 DAY),
  (1009, 10014, 'comment', NOW() - INTERVAL 25 DAY, NOW() - INTERVAL 25 DAY),
  (1009, 10001, 'view',    NOW() - INTERVAL 22 DAY, NOW() - INTERVAL 22 DAY),
  (1009, 10001, 'like',    NOW() - INTERVAL 22 DAY, NOW() - INTERVAL 22 DAY),
  (1009, 10009, 'view',    NOW() - INTERVAL 20 DAY, NOW() - INTERVAL 20 DAY),
  (1009, 10009, 'like',    NOW() - INTERVAL 20 DAY, NOW() - INTERVAL 20 DAY),
  (1009, 10023, 'view',    NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1009, 10023, 'like',    NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1009, 10023, 'share',   NOW() - INTERVAL 18 DAY, NOW() - INTERVAL 18 DAY),
  (1009, 10020, 'view',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1009, 10020, 'like',    NOW() - INTERVAL 15 DAY, NOW() - INTERVAL 15 DAY),
  (1009, 10027, 'view',    NOW() - INTERVAL 12 DAY, NOW() - INTERVAL 12 DAY),
  (1009, 10017, 'view',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1009, 10017, 'like',    NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1009, 10017, 'comment', NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1009, 10017, 'share',   NOW() - INTERVAL 10 DAY, NOW() - INTERVAL 10 DAY),
  (1009, 10013, 'view',    NOW() - INTERVAL 5 DAY,  NOW() - INTERVAL 5 DAY),
  (1009, 10013, 'like',    NOW() - INTERVAL 5 DAY,  NOW() - INTERVAL 5 DAY),
  (1009, 10018, 'view',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1009, 10018, 'like',    NOW() - INTERVAL 3 DAY,  NOW() - INTERVAL 3 DAY),
  (1009, 10026, 'view',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  (1009, 10026, 'like',    NOW() - INTERVAL 2 DAY,  NOW() - INTERVAL 2 DAY),
  (1009, 10021, 'view',    NOW() - INTERVAL 1 DAY,  NOW() - INTERVAL 1 DAY),
  (1009, 10022, 'view',    NOW() - INTERVAL 1 DAY,  NOW() - INTERVAL 1 DAY),
  (1009, 10012, 'view',    NOW() - INTERVAL 8 DAY,  NOW() - INTERVAL 8 DAY),
  (1009, 10005, 'view',    NOW() - INTERVAL 6 DAY,  NOW() - INTERVAL 6 DAY),
  (1009, 10005, 'like',    NOW() - INTERVAL 6 DAY,  NOW() - INTERVAL 6 DAY),
  (1009, 10029, 'view',    NOW() - INTERVAL 4 DAY,  NOW() - INTERVAL 4 DAY);


-- ============================================================
-- 9. 观看历史（user_video_watch_histories）
-- ============================================================
INSERT INTO user_video_watch_histories (user_id, video_id, watch_duration, completion_rate, watch_time) VALUES
  (1006, 10014, 810,  0.90, NOW() - INTERVAL 20 DAY),
  (1006, 10017, 720,  0.90, NOW() - INTERVAL 15 DAY),
  (1006, 10018, 600,  0.91, NOW() - INTERVAL 10 DAY),
  (1006, 10015, 500,  0.69, NOW() - INTERVAL 8 DAY),
  (1006, 10016, 360,  0.60, NOW() - INTERVAL 5 DAY),
  (1006, 10023, 58,   0.97, NOW() - INTERVAL 18 DAY),
  (1006, 10024, 85,   0.94, NOW() - INTERVAL 12 DAY),
  (1006, 10026, 110,  0.92, NOW() - INTERVAL 3 DAY),
  (1006, 10020, 600,  0.50, NOW() - INTERVAL 6 DAY),
  (1006, 10021, 480,  0.80, NOW() - INTERVAL 2 DAY),
  (1007, 10008, 540,  0.90, NOW() - INTERVAL 15 DAY),
  (1007, 10009, 680,  0.94, NOW() - INTERVAL 12 DAY),
  (1007, 10012, 450,  0.94, NOW() - INTERVAL 8 DAY),
  (1007, 10013, 380,  0.95, NOW() - INTERVAL 3 DAY),
  (1007, 10011, 300,  0.60, NOW() - INTERVAL 5 DAY),
  (1007, 10001, 180,  0.90, NOW() - INTERVAL 10 DAY),
  (1007, 10005, 140,  0.50, NOW() - INTERVAL 6 DAY),
  (1007, 10007, 190,  0.95, NOW() - INTERVAL 2 DAY),
  (1009, 10014, 850,  0.94, NOW() - INTERVAL 25 DAY),
  (1009, 10001, 195,  0.98, NOW() - INTERVAL 22 DAY),
  (1009, 10009, 700,  0.97, NOW() - INTERVAL 20 DAY),
  (1009, 10023, 60,   1.00, NOW() - INTERVAL 18 DAY),
  (1009, 10020, 1100, 0.92, NOW() - INTERVAL 15 DAY),
  (1009, 10027, 280,  0.93, NOW() - INTERVAL 12 DAY),
  (1009, 10017, 760,  0.95, NOW() - INTERVAL 10 DAY),
  (1009, 10013, 390,  0.98, NOW() - INTERVAL 5 DAY),
  (1009, 10018, 620,  0.94, NOW() - INTERVAL 3 DAY),
  (1009, 10026, 115,  0.96, NOW() - INTERVAL 2 DAY),
  (1009, 10021, 550,  0.92, NOW() - INTERVAL 1 DAY),
  (1009, 10005, 270,  0.96, NOW() - INTERVAL 6 DAY),
  (1009, 10012, 460,  0.96, NOW() - INTERVAL 8 DAY),
  (1009, 10029, 300,  0.50, NOW() - INTERVAL 4 DAY);


-- ============================================================
-- 10. 视频热度分（video_hot_scores）
-- 真实列: id(auto), video_id, time_window, view_count, like_count, comment_count,
--         share_count, favorite_count, hot_score, rank, window_start, window_end, updated_at
-- ============================================================
INSERT INTO video_hot_scores (video_id, time_window, view_count, like_count, comment_count, share_count, favorite_count, hot_score, `rank`, window_start, window_end, updated_at) VALUES
  -- 全局热度
  (10023, 'global', 80000, 15000, 3000, 2000, 1500, 131000, 1,  NOW() - INTERVAL 30 DAY, NOW(), NOW()),
  (10024, 'global', 65000, 12000, 2500, 1800, 1200, 107500, 2,  NOW() - INTERVAL 22 DAY, NOW(), NOW()),
  (10026, 'global', 55000, 10000, 2200, 1500, 1100, 97000,  3,  NOW() - INTERVAL 4 DAY,  NOW(), NOW()),
  (10025, 'global', 50000, 9000,  1800, 1200, 900,  81600,  4,  NOW() - INTERVAL 14 DAY, NOW(), NOW()),
  (10014, 'global', 45000, 6000,  1200, 500,  800,  34200,  5,  NOW() - INTERVAL 25 DAY, NOW(), NOW()),
  (10018, 'global', 40000, 5500,  1100, 450,  700,  32600,  6,  NOW() - INTERVAL 3 DAY,  NOW(), NOW()),
  (10013, 'global', 30000, 5500,  850,  400,  650,  27250,  7,  NOW() - INTERVAL 5 DAY,  NOW(), NOW()),
  (10017, 'global', 35000, 5200,  900,  400,  600,  27200,  8,  NOW() - INTERVAL 6 DAY,  NOW(), NOW()),
  (10009, 'global', 32000, 5800,  900,  450,  700,  25100,  9,  NOW() - INTERVAL 35 DAY, NOW(), NOW()),
  (10021, 'global', 30000, 4500,  700,  350,  500,  24050,  10, NOW() - INTERVAL 7 DAY,  NOW(), NOW()),
  -- 24小时热度
  (10026, '24h', 8000,  1500, 300, 200, 150, 45000, 1,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10018, '24h', 6000,  1000, 200, 100, 120, 38000, 2,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10013, '24h', 4000,  800,  120, 80,  100, 15000, 3,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10017, '24h', 3500,  700,  100, 60,  80,  12000, 4,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10021, '24h', 3000,  600,  90,  50,  70,  10000, 5,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10007, '24h', 2500,  500,  80,  40,  60,  8000,  6,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10022, '24h', 2000,  400,  60,  30,  40,  7260,  7,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10012, '24h', 1800,  350,  50,  25,  35,  6000,  8,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10025, '24h', 1500,  300,  40,  20,  30,  5000,  9,  NOW() - INTERVAL 1 DAY, NOW(), NOW()),
  (10029, '24h', 1000,  200,  30,  10,  20,  2960,  10, NOW() - INTERVAL 1 DAY, NOW(), NOW())
ON DUPLICATE KEY UPDATE hot_score = VALUES(hot_score), `rank` = VALUES(`rank`);


-- ============================================================
-- 验证
-- ============================================================
SELECT '✅ 推荐系统测试数据集导入完成！' AS result;
SELECT CONCAT('  用户: ', COUNT(*))   AS info FROM users WHERE user_id BETWEEN 1000 AND 1009;
SELECT CONCAT('  视频: ', COUNT(*))   AS info FROM videos WHERE video_id BETWEEN 10000 AND 10029;
SELECT CONCAT('  视频特征: ', COUNT(*)) AS info FROM video_features WHERE video_id BETWEEN 10000 AND 10029;
SELECT CONCAT('  标签映射: ', COUNT(*)) AS info FROM tag_video_mappings WHERE video_id BETWEEN 10000 AND 10029;
SELECT CONCAT('  用户画像: ', COUNT(*)) AS info FROM user_profiles WHERE user_id BETWEEN 1000 AND 1009;
SELECT CONCAT('  用户行为: ', COUNT(*)) AS info FROM user_behaviors WHERE user_id IN (1006, 1007, 1009);
SELECT CONCAT('  观看历史: ', COUNT(*)) AS info FROM user_video_watch_histories WHERE user_id IN (1006, 1007, 1009);
SELECT CONCAT('  热度分: ', COUNT(*))  AS info FROM video_hot_scores WHERE video_id BETWEEN 10000 AND 10029;
