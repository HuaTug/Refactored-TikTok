#!/usr/bin/env bash
# P03 · 切换 interaction 服务的"同步直写 DB" vs "Kafka 异步"模式
# -----------------------------------------------------------------------------
# 用法：
#   bash scripts/perf/p03_toggle_sync.sh A   # 切到 A 组（同步直写，预期会崩）
#   bash scripts/perf/p03_toggle_sync.sh B   # 切到 B 组（Kafka 异步，正常路径）
#   bash scripts/perf/p03_toggle_sync.sh status
#
# 实现方式：在仓库根写入 .env.perf，由 cmd/interaction 启动时读取 LIKE_SYNC 环境变量。
# 切换之后必须**重启 cmd/interaction**（脚本会提示）。
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ENV_FILE="${PROJECT_ROOT}/.env.perf"

mode="${1:-status}"
case "${mode}" in
  A|a|sync)
    echo "LIKE_SYNC=1" > "${ENV_FILE}"
    echo "[P03] 已切到 A 组（同步直写 DB）。请重启 cmd/interaction："
    echo "       set -a && source ${ENV_FILE} && set +a && go run ./cmd/interaction"
    ;;
  B|b|async|kafka)
    echo "LIKE_SYNC=0" > "${ENV_FILE}"
    echo "[P03] 已切到 B 组（Kafka 异步落库）。请重启 cmd/interaction："
    echo "       set -a && source ${ENV_FILE} && set +a && go run ./cmd/interaction"
    ;;
  status)
    if [ -f "${ENV_FILE}" ]; then cat "${ENV_FILE}"; else echo "未设置（默认 B 组：异步）"; fi
    ;;
  *)
    echo "用法: $0 A|B|status" >&2
    exit 1
    ;;
esac

echo
echo "提示：A 组需要在 cmd/interaction 启动逻辑里识别 LIKE_SYNC=1 走 like_service_legacy.go。"
echo "      项目里 like_service_legacy.go 已经实现了同步直写路径（Cache-Aside），"
echo "      只需要在 main.go 启动时根据 LIKE_SYNC 选择 NewLikeActionService 即可。"
