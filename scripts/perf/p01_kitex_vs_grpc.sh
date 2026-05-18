#!/usr/bin/env bash
# P01 · RPC 通信框架性能对比（复现论文表 6-1）
#
# 前置：
#   - cmd/interaction 已运行（监听 :8889 或读取项目配置中的 Interaction 地址）
#   - 已经 seed_data.sql，确保 video_id 9000001~9001000 + user_id 1000000~1099999 存在
#
# 运行：
#   bash scripts/perf/p01_kitex_vs_grpc.sh                   # 默认 100 并发 / 5 分钟
#   KITEX_ADDR=127.0.0.1:8889 CONCURRENCY=200 DURATION=3m \
#     bash scripts/perf/p01_kitex_vs_grpc.sh
#
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${PROJECT_ROOT}"

KITEX_ADDR="${KITEX_ADDR:-127.0.0.1:8893}"
GRPC_ADDR="${GRPC_ADDR:-127.0.0.1:50051}"
CONCURRENCY="${CONCURRENCY:-100}"
DURATION="${DURATION:-5m}"
TS="$(date +%Y%m%d-%H%M)"
REPORT="scripts/perf/reports/report-${TS}-P01.md"

mkdir -p scripts/perf/reports
exec > >(tee "${REPORT}") 2>&1

echo "# P01 · Kitex vs gRPC  (${TS})"
echo
echo "- 被测：interaction.LikeAction"
echo "- 并发：${CONCURRENCY}"
echo "- 时长：${DURATION}"
echo "- Kitex 地址：${KITEX_ADDR}"
echo "- gRPC  地址：${GRPC_ADDR} (可选)"
echo

# ---------- A. Kitex ---------------------------------------------------------
echo "## A · Kitex"
go run -tags perf ./scripts/perf/kitex_dolike_bench \
    -target "${KITEX_ADDR}" \
    -c "${CONCURRENCY}" \
    -duration "${DURATION}" || true

# ---------- B. gRPC（可选）--------------------------------------------------
echo
echo "## B · gRPC"
if command -v ghz >/dev/null 2>&1 && [ -f "idl/interaction.proto" ]; then
  ghz --insecure \
      --proto idl/interaction.proto \
      --call interaction.InteractionService.LikeAction \
      -d '{"user_id":1000001,"video_id":9000001,"action_type":"1"}' \
      -c "${CONCURRENCY}" -z "${DURATION}" \
      "${GRPC_ADDR}" || true
else
  echo "（跳过）ghz 未安装或缺少 idl/interaction.proto。如需对比请："
  echo "  brew install ghz   # macOS"
  echo "  并把 .thrift 转成 .proto 后再运行。"
fi

echo
echo "## 结论模板"
echo "| 框架 | QPS | TP99 | TP999 | 错误率 |"
echo "|---|---|---|---|---|"
echo "| Kitex | …  | …    | …     | …      |"
echo "| gRPC  | …  | …    | …     | …      |"
echo
echo "通过标准：Kitex QPS ≥ 1.5 × gRPC，且 TP99 ≤ gRPC × 0.6（与论文表 6-1 趋势一致）。"
