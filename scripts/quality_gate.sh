#!/bin/sh
# quality_gate.sh — 提交前质量门禁：build / vet / gofmt / test 四项全过才允许提交。
#
# 用法: sh scripts/quality_gate.sh
# 退出码 0 = 全绿, 1 = 有问题（明细打印到 stdout）
#
# 为什么需要它：本项目的 vet 曾在 radius 客户端里抓出
#   `fmt.Sprintf("%s:%d", server, port)` 在 IPv6 下拼出 "::1:1812" 无法拨号，
# 而 go test 全过、build 也过 —— 只有 vet 会报。
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/mgmtd" || exit 1

FAIL=0
export CGO_ENABLED=1

echo "== build =="
if ! go build ./... ; then
    echo "FAIL: build"
    FAIL=1
else
    echo "ok"
fi

echo "== vet =="
if ! go vet ./... ; then
    echo "FAIL: vet"
    FAIL=1
else
    echo "ok"
fi

echo "== gofmt =="
UNFMT=$(gofmt -l . 2>/dev/null | grep -v vendor)
if [ -n "$UNFMT" ]; then
    echo "FAIL: unformatted files:"
    echo "$UNFMT"
    echo "  fix: gofmt -w \$(gofmt -l . | grep -v vendor)"
    FAIL=1
else
    echo "ok"
fi

echo "== test =="
if ! go test ./... >/tmp/quality_gate_test.log 2>&1 ; then
    echo "FAIL: tests"
    grep -E '^(FAIL|---.? FAIL)' /tmp/quality_gate_test.log | head -20
    FAIL=1
else
    echo "ok ($(grep -c '^ok' /tmp/quality_gate_test.log) packages)"
fi

# nftables 危险模式静态扫描。同一个错误已经重复出现过多次，靠 review 记不住：
# flush ruleset（清空整机所有表）×3、policy drop（切断 SSH）×1、
# 空集合 elements = {}（语法错）、inet 表裸 dnat to（被 nft 拒绝）。
echo "== nft lint =="
if ! python3 "$ROOT/scripts/nft_lint.py" >/tmp/quality_gate_nft.log 2>&1 ; then
    echo "FAIL: nft 危险模式"
    head -20 /tmp/quality_gate_nft.log
    FAIL=1
else
    cat /tmp/quality_gate_nft.log
fi

# 前端调用的路径必须真在后端注册过。之前 /vpn/tunnels 就是前端调用了未注册路径，
# 页面永远空白但所有后端测试都是绿的 —— 这类问题只有跨层对比才能发现。
echo "== api contract =="
if ! python3 "$ROOT/scripts/api_contract_check.py" >/tmp/quality_gate_api.log 2>&1 ; then
    echo "FAIL: 前端调用了未注册的后端路径"
    grep -A2 '❌' /tmp/quality_gate_api.log | head -20
    FAIL=1
else
    grep '✅' /tmp/quality_gate_api.log | head -1
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "QUALITY GATE: PASS"
    exit 0
fi
echo "QUALITY GATE: FAIL"
exit 1
