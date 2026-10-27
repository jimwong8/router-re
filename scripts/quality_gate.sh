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

echo ""
if [ "$FAIL" -eq 0 ]; then
    echo "QUALITY GATE: PASS"
    exit 0
fi
echo "QUALITY GATE: FAIL"
exit 1
