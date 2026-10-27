#!/bin/sh
# watch_workers.sh — 兼容入口，实际逻辑在 watch_workers.py。
#
# v1 只做进度快照，不会主动提醒；v2 改为 Python 实现，因为
# `herdr agent list` 返回 JSON，用 agent_status 判定空闲比解析终端
# 提示符可靠（提示符样式会随版本变，v1 就靠 grep '· 12%' 猜状态）。
#
# 用法:
#   sh scripts/watch_workers.sh                      # 空闲 5 分钟提醒
#   IDLE_MINUTES=10 sh scripts/watch_workers.sh      # 改成 10 分钟
#   PANES="w1:pN w1:pQ" sh scripts/watch_workers.sh  # 只盯两个窗口
#   NO_NOTIFY=1 sh scripts/watch_workers.sh          # 只写日志不弹窗
#
# 日志: logs/worker-watch.log（进度，只在变化时写）
#       logs/worker-alerts.log（空闲/阻塞告警）
set -u
exec python3 "$(cd "$(dirname "$0")" && pwd)/watch_workers.py" "$@"
