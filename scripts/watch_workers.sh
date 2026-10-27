#!/bin/sh
# watch_workers.sh — 常驻监控三个 herdr 工人的进度，只在状态变化时写日志。
#
# 用法: sh scripts/watch_workers.sh          (前台)
#       nohup sh scripts/watch_workers.sh &   (后台常驻)
#
# 日志: logs/worker-watch.log
# 设计: 每 60s 采样一次「窗口最后一行 + Go 文件数 + 端点总数」，
#       与上一次快照比较，只在变化时写一行 —— 这样日志小且全是真事件。
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG="$ROOT/logs/worker-watch.log"
INTERVAL="${WATCH_INTERVAL:-60}"
export PATH="$HOME/.local/bin:$PATH"

mkdir -p "$ROOT/logs"

# 窗口最后一行的可读摘要（去掉分隔线/空行/提示符）
snap_window() {
    herdr agent read "$1" --lines 3 2>/dev/null \
        | grep -vE '^[─ ]*$|^❯|^ *☤' \
        | tail -1 | cut -c1-70
}

# 工人是否在干活：上下文百分比 > 0 且不是 idle 提示符
busy_hint() {
    herdr agent read "$1" --lines 2 2>/dev/null | tail -1 | grep -oE '· *[0-9]+%' | head -1
}

PREV=""
echo "[$(date +%H:%M:%S)] watcher started (interval ${INTERVAL}s)" >> "$LOG"

while true; do
    TS=$(date +%H:%M:%S)
    SNAP=""
    for p in w1:pN w1:pQ w1:pP; do
        LINE=$(snap_window "$p")
        BUSY=$(busy_hint "$p")
        SNAP="$SNAP | $p${BUSY:+[$BUSY]}: $LINE"
    done

    FILES=$(find "$ROOT/mgmtd/internal" -name '*.go' 2>/dev/null | wc -l)
    EPS=$(grep -cE 'HandleFunc' "$ROOT/mgmtd/internal/server/server.go" 2>/dev/null || echo 0)
    PKGS=$(find "$ROOT/mgmtd/internal" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l)
    VIEWS=$(ls "$ROOT"/web/src/views/*.vue 2>/dev/null | wc -l)

    CUR="go=$FILES pkg=$PKGS ep=$EPS views=$VIEWS$SNAP"

    if [ "$CUR" != "$PREV" ]; then
        echo "[$TS] $CUR" >> "$LOG"
        PREV="$CUR"
    fi

    sleep "$INTERVAL"
done
