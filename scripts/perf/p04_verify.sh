#!/usr/bin/env bash
# P04 · 一致性校验
# 用法：bash scripts/perf/p04_verify.sh 9000777
set -euo pipefail
VID="${1:-9000777}"

# ----- MySQL 计数 -----
mysql_cnt=$(docker exec -i kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' \
  -DTikTok -N -B -e "SELECT COUNT(*) FROM video_likes WHERE video_id=${VID} AND deleted_at IS NULL;" 2>/dev/null \
  | tail -1)

# ----- Redis 计数 -----
# 业务里 Redis DB=1，互动服务用 like:obj:1:<vid> (zset, 成员=user_id) 表示点赞集合，
# like:count:<vid> (string) 表示计数。
redis_set=$(docker exec -i kitex_redis redis-cli --no-auth-warning \
  -a 'Redis@TikTok2025_SecurePass' -n 1 ZCARD "like:obj:1:${VID}" 2>/dev/null || echo "?")
redis_cnt=$(docker exec -i kitex_redis redis-cli --no-auth-warning \
  -a 'Redis@TikTok2025_SecurePass' -n 1 HGET "like:count:1" "${VID}" 2>/dev/null || echo "?")

# ----- 视频汇总字段 -----
video_lc=$(docker exec -i kitex_mysql mysql -uroot -p'TikTok@MySQL#2025!Secure' \
  -DTikTok -N -B -e "SELECT likes_count FROM videos WHERE video_id=${VID};" 2>/dev/null | tail -1)

cat <<EOF
============= P04 · 一致性校验  video_id=${VID} =============
  MySQL  COUNT(video_likes)  = ${mysql_cnt}
  Redis  ZCARD like:obj:1:.. = ${redis_set}
  Redis  GET   like:count:.. = ${redis_cnt}
  MySQL  videos.likes_count  = ${video_lc}
=============================================================
通过标准：四个数字应当完全相等（预期 = 实际成功的点赞数）。
若 Redis 比 MySQL 大，说明异步落库尚未完成，等待 5 分钟后再跑一次本脚本。
EOF
