#!/usr/bin/env bash
# 一拖N 启动器：坚哥报一句「用一拖三方案，pane 是 w1:pA w1:pB w1:pC」，我就调这个。
# 它只做三件事：建任务队列骨架、写本轮环境变量、起一轮调度器试跑。
# 常驻的 timer 照旧跑（PANES 由 service 文件里的 Environment 决定，见下）。
#
# 用法：
#   ./scripts/start拖.sh <项目根> <pane1> [pane2] [pane3] [pane4]
#   ./scripts/start拖.sh /home/jimwong/projects/myapp w1:pN w1:pQ w1:pP
#
# pane 数量即规模：一拖二就传 2 个，一拖三 3 个，一拖四 4 个。
# 强模型 pane（plan/verify 专用）用 STRONG= 环境变量指定，默认无（全 free）。
set -euo pipefail

if [ $# -lt 2 ]; then
  echo "用法: $0 <项目根> <pane1> [pane2] [pane3] [pane4]"
  echo "例:   $0 /home/jimwong/projects/myapp w1:pN w1:pQ w1:pP"
  exit 1
fi

ROOT="$1"; shift
PANES="$*"
N=$#
STRONG="${STRONG:-}"   # 如：STRONG="w1:pM"（plan/verify 只派给它）

mkdir -p "$ROOT/ops" "$ROOT/logs" "$ROOT/scripts"

# 队列骨架（不存在才建，不覆盖已有任务）
if [ ! -f "$ROOT/ops/tasks.jsonl" ]; then
  touch "$ROOT/ops/tasks.jsonl"
  echo "队列已建：$ROOT/ops/tasks.jsonl（往里追加任务，一行一个 JSON）"
else
  echo "队列已存在：$ROOT/ops/tasks.jsonl（不覆盖）"
fi

# 本轮试跑：只做结算 + 派发一轮，验证 pane 解析通不通
echo "pane（$N 个）：$PANES"
echo "强模型 pane：${STRONG:-（无，全 free）}"
PANES="$PANES" STRONG_PANES="$STRONG" \
QUEUE_PATH="$ROOT/ops/tasks.jsonl" \
DISPATCH_LOG="$ROOT/logs/dispatch.log" \
DISPATCH_STATE="$ROOT/.dispatch-state.json" \
WORK_DIR="$ROOT" \
python3 /home/jimwong/projects/router-re/scripts/dispatch_workers.py

echo
echo "试跑完成。常驻调度照旧由 systemd timer 跑，要换 pane 就改 service 里的 PANES："
echo "  systemctl --user edit router-re-dispatch.service"
