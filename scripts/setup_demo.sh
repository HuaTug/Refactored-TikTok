#!/usr/bin/env bash
# =============================================================================
# 答辩演示一键初始化脚本
# -----------------------------------------------------------------------------
# 适用场景：换一台新电脑，希望复现"用户已注册、视频已上传、互动数据齐全"的状态。
#
# 前置条件（必须先完成）：
#   1. docker compose 已启动：mysql / redis / minio / etcd / kafka 全部 healthy
#   2. 后端微服务已启动：user / video / interaction / relation / api 等
#      （可通过 make run-all 或手动启动各 cmd 下的服务）
#   3. python3 已装好依赖：pip install -r scripts/requirements.txt
#      或者直接：pip install requests pymysql redis
#   4. 视频文件已拷到：
#        Refactored-TikTok/bili_videos/videos_hot/<分组>/<BV号>_<标题>.mp4
#      或通过环境变量 DEMO_VIDEO_DIR 指定其他位置
#
# 用法：
#   bash scripts/setup_demo.sh
#   API_BASE=http://localhost:8888 bash scripts/setup_demo.sh
#   DEMO_VIDEO_DIR=/path/to/videos_hot bash scripts/setup_demo.sh
# =============================================================================

set -euo pipefail

# 切到脚本所在的项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

API_BASE="${API_BASE:-http://localhost:8888}"
PYTHON_BIN="${PYTHON_BIN:-python3}"

color() { printf "\033[1;36m%s\033[0m\n" "$*"; }
warn()  { printf "\033[1;33m%s\033[0m\n" "$*"; }
fail()  { printf "\033[1;31m%s\033[0m\n" "$*" >&2; }

# -------- 0. 健康检查 --------------------------------------------------------
color "==[0/4]== 健康检查 - 确认后端网关已启动"
if ! curl -sf "${API_BASE}/ping" >/dev/null 2>&1 \
   && ! curl -sf "${API_BASE}/" >/dev/null 2>&1 ; then
  warn "  ! 无法访问 ${API_BASE}，但仍尝试继续。如果接下来上传失败，请先启动后端。"
else
  echo "  ✓ 后端网关可达"
fi

# -------- 1. 初始化推荐系统配置数据 -------------------------------------------
color "\n==[1/4]== 注入推荐系统的演示种子数据（分类/标签/默认权重等）"
if [ -f "scripts/seed_demo_data.sql" ]; then
  if command -v mysql >/dev/null 2>&1; then
    # 通过 docker exec 走容器内 mysql，避免本机没装客户端
    if docker ps --format '{{.Names}}' | grep -q '^kitex_mysql$'; then
      docker exec -i kitex_mysql mysql -u root -p'TikTok@MySQL#2025!Secure' TikTok \
        < scripts/seed_demo_data.sql && echo "  ✓ seed_demo_data.sql 注入完成"
    else
      warn "  ! kitex_mysql 容器未运行，跳过 seed_demo_data.sql"
    fi
  else
    docker exec -i kitex_mysql mysql -u root -p'TikTok@MySQL#2025!Secure' TikTok \
      < scripts/seed_demo_data.sql && echo "  ✓ seed_demo_data.sql 注入完成" \
      || warn "  ! seed_demo_data.sql 注入失败，可手动执行"
  fi
else
  warn "  ! 未找到 scripts/seed_demo_data.sql，跳过"
fi

# -------- 2. 注册 5 个测试用户 + 上传 24 条 B 站视频 --------------------------
color "\n==[2/4]== 注册测试用户并通过分片上传 API 入库 B 站视频（约 5-15 分钟）"
echo "  · 用户：test_user_01 ~ test_user_05  密码：123456"
echo "  · 视频：来自 ${DEMO_VIDEO_DIR:-默认路径}（共 24 条，2.3GB 左右）"

API_BASE="${API_BASE}" "${PYTHON_BIN}" scripts/seed_users_upload_videos.py
upload_rc=$?
if [ ${upload_rc} -ne 0 ]; then
  warn "  ! 部分视频上传失败 (rc=${upload_rc})，请查看上方日志。继续往下走，不阻塞。"
fi

# 转码服务是异步的，给它一点时间吃掉 Kafka 消息
echo "  · 等待转码与封面生成（30 秒）..."
sleep 30

# -------- 3. 灌入互动数据（点赞/观看/曝光/画像） ------------------------------
color "\n==[3/4]== 灌入用户互动 + 推荐训练样本"
"${PYTHON_BIN}" scripts/seed_user_actions.py || warn "  ! seed_user_actions.py 执行有警告，请检查日志"

# -------- 4. Redis 预热（推荐缓存、热榜等） ----------------------------------
color "\n==[4/4]== Redis 缓存预热"
if [ -f "scripts/warmup_redis.py" ]; then
  "${PYTHON_BIN}" scripts/warmup_redis.py || warn "  ! warmup_redis 失败，可稍后单独重跑"
else
  warn "  ! 未找到 warmup_redis.py，跳过（不影响基本演示）"
fi

color "\n==[完成]=="
echo "  · 登录账号：test_user_01 ~ test_user_05  密码：123456"
echo "  · 网关地址：${API_BASE}"
echo "  · 接下来：启动 Tiktok-web 前端 (npm run dev) 即可看到推荐流"
