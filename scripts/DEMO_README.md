# 答辩演示环境 · 一键复现指南

本文档说明如何在**新电脑**上还原"用户已注册、视频已上传、互动数据齐全"的展示状态。

---

## 一、需要带走的东西（拷盘 / 网盘）

| 类别 | 路径 | 说明 |
|------|------|------|
| 代码 | `Refactored-TikTok/` | 后端源码（git clone 也可） |
| 代码 | `Tiktok-web/` | 前端源码 |
| 视频 | `bilibili/output/videos_hot/` （约 **2.3 GB**，24 条 mp4） | 一键上传脚本要用 |
| （可选）模型 | `DeepCTR/` 训练好的权重文件 | 如果展示个性化推荐 |

> 视频在新机器上，**建议放到 `Refactored-TikTok/bili_videos/videos_hot/`**，脚本会自动识别该位置；放别处则在执行时指定 `DEMO_VIDEO_DIR=...`。

---

## 二、新电脑前置环境

只要做一次：

```bash
# 1. Docker Desktop（Mac/Win）或 docker + docker compose（Linux）
docker --version
docker compose version

# 2. Go 1.21+（编译后端）
go version

# 3. Node 18+ / pnpm 或 npm（前端）
node -v

# 4. Python 3.9+（执行 seed 脚本）
python3 -V
pip3 install requests pymysql redis
```

---

## 三、一键启动流程（共 5 步）

### Step 1 · 启动基础设施容器

```bash
cd Refactored-TikTok/deploy
docker compose up -d mysql redis minio etcd zookeeper kafka

# 等 30 秒让 mysql 跑完 init.sql
docker compose ps
```

> MySQL 端口 **3307**（不是 3306），密码 `TikTok@MySQL#2025!Secure`，库名 `TikTok`。
>
> MinIO 端口 **9002**（API）/ **9003**（Console），账号 `tiktok_minio_admin` / 密码 `MainMinIO@TikTok#2025!SecurePass`。

### Step 2 · 启动后端微服务

任选一种方式：

```bash
# 方式 A：用 Makefile（如有 run-all 目标）
make -C Refactored-TikTok run-all

# 方式 B：手动逐个启动（推荐用 tmux/iTerm 拆窗口）
cd Refactored-TikTok
go run cmd/user/.        # 用户服务
go run cmd/video/.       # 视频服务（含上传/转码）
go run cmd/interaction/. # 点赞/评论/收藏
go run cmd/relation/.    # 关注关系
go run cmd/api/.         # API 网关，监听 :8888
```

健康检查：

```bash
curl http://localhost:8888/ping
```

### Step 3 · 一键灌注演示数据 ⭐

```bash
cd Refactored-TikTok
bash scripts/setup_demo.sh
```

这一条命令做完了：
1. 注入推荐系统种子数据（分类/标签）
2. 注册 5 个用户（`test_user_01` ~ `test_user_05`，密码 `123456`）
3. 把 24 条 B 站视频按真实分片上传流程入库（`/v2/publish/start → uploading → complete`）
4. 等待异步转码 30 秒
5. 灌入观看 / 点赞 / 收藏 / 曝光样本（让推荐有数据）
6. Redis 缓存预热（热榜、用户画像等）

**预计耗时 5–15 分钟**，主要花在视频上传 + 转码上。中途断了重跑安全（`seed_users_upload_videos.py` 会跳过已上传视频）。

> 视频不在默认位置时：
> ```bash
> DEMO_VIDEO_DIR=/data/myvideos bash scripts/setup_demo.sh
> ```

### Step 4 · 启动前端

```bash
cd Tiktok-web
npm install   # 第一次需要
npm run dev
```

浏览器打开 `http://localhost:5173`（Vite 默认）→ 用 `test_user_01 / 123456` 登录。

### Step 5 · 验收清单

| 功能 | 验证方式 |
|------|---------|
| 登录 | `test_user_01` / `123456` 能进首页 |
| 推荐流 | 首页应有 ≥ 10 条带封面的视频 |
| 视频播放 | 点开任一视频能正常播 |
| 点赞/评论 | 操作后数字立刻变化 |
| 关注 | 关注其他 4 个 test_user 后，关注页有内容 |
| 热门榜 | `/popular` 路由有数据 |

---

## 四、单独重跑某一步

| 需求 | 命令 |
|------|------|
| 只重传视频 | `python3 scripts/seed_users_upload_videos.py` |
| 只灌互动数据 | `python3 scripts/seed_user_actions.py` |
| 只热 Redis | `python3 scripts/warmup_redis.py` |
| 重置数据库 | `docker compose down -v && docker compose up -d`（**会清空所有数据**） |

---

## 五、常见问题

**Q1：上传到 60% 卡住了？**
A：通常是后端转码慢或 MinIO 写入慢。`seed_users_upload_videos.py` 自带断点续传，Ctrl+C 后重跑即可。

**Q2：换电脑后视频播不了？**
A：检查 `MINIO_ENDPOINT` 是否仍是 `localhost:9002`。前端如果跨域请求 MinIO，确认 `MINIO_API_CORS_ALLOW_ORIGIN=*`（docker-compose 已默认开启）。

**Q3：推荐流空白？**
A：90% 是 `seed_user_actions.py` 没跑成功。手动执行检查报错。也可降级用 `/v1/video/popular` 走热门榜。

**Q4：视频 ID 都从 100 开始，能不能改？**
A：`seed_user_actions.py` 中 `NEW_VIDEO_ID_MIN=100` 决定灌互动数据的起始 ID，按需调整即可。

---

## 六、答辩当天 5 分钟应急方案

如果到现场发现某项功能挂了，按这个顺序排查：

```bash
# 1. 服务都活着吗？
docker compose ps
ps aux | grep -E 'cmd/(api|user|video|interaction)'

# 2. 数据库还在吗？
docker exec kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' \
  -e "SELECT COUNT(*) FROM TikTok.videos WHERE open=1;"
# 期望：≥ 24

# 3. MinIO 文件还在吗？
docker exec minio mc ls local/ 2>/dev/null || \
  ls deploy/../config/minio/data/

# 4. Redis 缓存还在吗？
docker exec kitex_redis redis-cli -a 'Redis@TikTok2025_SecurePass' \
  --no-auth-warning DBSIZE
# 期望：≥ 几百
```

任何一步空了，回到对应的"单独重跑"章节即可。
