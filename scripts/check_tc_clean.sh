#!/bin/sh
# check_tc_clean.sh — 检查关键接口是否被遗留的 tc 限速规则污染。
#
# 背景：QoS 测试曾把 10Mbit tbf 打到 `lo` 上且未清理，导致整个系统 loopback
# 被限速（本地 1MB HTTP 响应从 3ms 变成 20s），排查方向被误导到应用代码。
#
# 用法：sh scripts/check_tc_clean.sh    (exit 0 = 干净, exit 1 = 有残留)
set -u

TC=$(command -v tc || echo /usr/sbin/tc)
[ -x "$TC" ] || { echo "SKIP: tc not found"; exit 0; }

DIRTY=0
# lo 必须干净；此外任何接口上的 tbf/htb/netem 都值得人工确认
for dev in lo; do
    q=$("$TC" qdisc show dev "$dev" 2>/dev/null | head -1)
    case "$q" in
        *tbf*|*htb*|*netem*)
            echo "DIRTY: $dev -> $q"
            DIRTY=1
            ;;
        *)
            echo "ok:    $dev -> ${q:-<none>}"
            ;;
    esac
done

# 全局扫描（排除默认 qdisc）
"$TC" qdisc show 2>/dev/null | while read -r line; do
    case "$line" in
        *tbf*|*htb*|*netem*) echo "note:  $line" ;;
    esac
done

if [ "$DIRTY" -eq 1 ]; then
    echo ""
    echo "修复: sudo $TC qdisc del dev lo root"
    exit 1
fi

echo ""
echo "clean: no throttling qdisc on lo"
exit 0
