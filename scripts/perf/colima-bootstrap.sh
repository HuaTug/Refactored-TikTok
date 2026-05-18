#!/usr/bin/env bash
# 在 macOS 上以 4C/8G 启动 colima，并把基础组件拉起来。
# Linux 直接用 docker，本脚本会跳过 colima 步骤。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

color() { printf "\033[1;36m%s\033[0m\n" "$*"; }
warn()  { printf "\033[1;33m%s\033[0m\n" "$*"; }

# ---------- 1. colima (macOS only) ----------
if [[ "$(uname)" == "Darwin" ]]; then
  if ! command -v colima >/dev/null 2>&1; then
    warn "colima 未安装，请先 brew install colima docker"
    exit 1
  fi
  if colima status >/dev/null 2>&1; then
    color "[1/3] colima 已运行；跳过启动"
    colima status
  else
    color "[1/3] colima 启动 (4 cpu / 8 GB / 60 GB)"
    colima start --cpu 4 --memory 8 --disk 60
  fi
else
  color "[1/3] 非 macOS，跳过 colima；请确保 docker daemon 已就绪"
fi

# ---------- 2. docker compose up 基础组件 ----------
color "\n[2/3] 启动中间件 (mysql / redis / kafka / etcd / minio / es / zookeeper)"
cd "${PROJECT_ROOT}/deploy"
docker compose up -d mysql redis kafka zookeeper etcd minio elasticsearch

# ---------- 3. 等待 healthy ----------
color "\n[3/3] 等待容器健康（最长 90s）"
for i in $(seq 1 18); do
  pending=$(docker compose ps --format '{{.Service}} {{.Health}}' \
    | awk '$2!="healthy" && $2!=""' | wc -l | tr -d ' ')
  total=$(docker compose ps --format '{{.Service}}' | wc -l | tr -d ' ')
  printf "  · %d/18 round  pending=%s total=%s\r" "$i" "$pending" "$total"
  [ "${pending}" -eq 0 ] && break
  sleep 5
done
echo ""
docker compose ps

color "\n基础组件就绪。下一步：启动 6 个微服务，再跑 setup_demo.sh / seed_data.sql。"
