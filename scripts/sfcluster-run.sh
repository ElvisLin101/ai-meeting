#!/usr/bin/env bash
# ============================================================
# sfcluster-run.sh: SingleFlight 集群(多实例)压测一键脚本
#
# 启动 1 个 coordinator + N 个 worker 进程(每个 worker 是独立实例,
# 拥有独立 DistributedGroup/独立 nodeID, 仅共享 Redis), 输出汇总报告。
#
# 用法:
#   bash scripts/sfcluster-run.sh                      # 默认 3 实例 × 50 并发, 去重场景
#   bash scripts/sfcluster-run.sh --instances 5        # 5 实例
#   bash scripts/sfcluster-run.sh --concurrency 100    # 每实例 100 并发
#   bash scripts/sfcluster-run.sh --failover-worker 0  # 换主场景: 首个 leader 卡死触发换主
#   bash scripts/sfcluster-run.sh --addr <redis> --password <pwd>
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

INSTANCES=3
CONCURRENCY=50
ADDR=localhost:6379
PASSWORD=""
FAILOVER_WORKER=-1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --instances) INSTANCES="$2"; shift 2 ;;
    --concurrency) CONCURRENCY="$2"; shift 2 ;;
    --addr) ADDR="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --failover-worker) FAILOVER_WORKER="$2"; shift 2 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done

# 压测批次 ID: 秒级时间戳, coordinator 与全部 worker 共用
RUN_ID="$(date +%s)"

# 校验 Redis 连通
if command -v redis-cli >/dev/null 2>&1; then
  if ! redis-cli -h "${ADDR%%:*}" -p "${ADDR##*:}" ping >/dev/null 2>&1; then
    echo "无法连接 Redis $ADDR, 请先启动: docker compose up -d redis"
    exit 1
  fi
fi

COMMON_ARGS=(--run "$RUN_ID" --addr "$ADDR")
[[ -n "$PASSWORD" ]] && COMMON_ARGS+=(--password "$PASSWORD")

echo "===== SingleFlight 集群压测启动 ====="
echo "实例数:      $INSTANCES"
echo "每实例并发:  $CONCURRENCY"
if [[ "$FAILOVER_WORKER" -ge 0 ]]; then
  echo "故障注入:    首个 leader 卡死触发换主"
else
  echo "故障注入:    关闭(纯去重场景)"
fi
echo "runID:       $RUN_ID"
echo ""

PIDS=()
cleanup() {
  for pid in "${PIDS[@]}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

# 启动 coordinator(后台), 它会等待全部 worker 就绪后广播 start
go run "$ROOT/cmd/sfcluster" --role coordinator --instances "$INSTANCES" --concurrency "$CONCURRENCY" "${COMMON_ARGS[@]}" &
PIDS+=("$!")

# 启动 N 个 worker(故障注入模式: 全部 worker 带 --failover-worker,
# 由 fn 内 SETNX 全局标记保证只有首个 leader 触发卡死, 换主后不再卡死)
for ((i = 0; i < INSTANCES; i++)); do
  worker_args=(--role worker --id "$i" --concurrency "$CONCURRENCY" "${COMMON_ARGS[@]}")
  if [[ "$FAILOVER_WORKER" -ge 0 ]]; then
    worker_args+=(--failover-worker "$i")
  fi
  go run "$ROOT/cmd/sfcluster" "${worker_args[@]}" &
  PIDS+=("$!")
done

# 等待全部进程结束(coordinator 结束后脚本退出)
FAIL=0
for pid in "${PIDS[@]}"; do
  if ! wait "$pid"; then
    FAIL=1
  fi
done
exit $FAIL
