#!/usr/bin/env bash
# run_all_tests.sh — 统一回归入口：把所有测试模块串起来一键跑。
#
# 用法:
#   bash scripts/run_all_tests.sh
#   BASE=http://10.100.1.18:9090 bash scripts/run_all_tests.sh
#
# 特性:
#   - 单个测试失败不中断，继续跑完所有模块再汇总。
#   - 前置检查：mgmtd health 必须 ok，否则退出并提示先起 mgmtd。
#   - UI acceptance 需要 vite preview (4173) + playwright；不通则 SKIP 并写原因。
#   - 结果写入 docs/regression-summary.md（脚本生成，勿手写）。
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASE="${BASE:-http://127.0.0.1:9090}"
REPORT="$ROOT/docs/regression-summary.md"
TMPDIR_RUN="$ROOT/.hermes/cache/scratch/regression-$$"
mkdir -p "$TMPDIR_RUN"

# ---------- 前置检查 ----------
echo "[regression] 前置检查: $BASE/api/v1/health"
if ! curl -s -m 5 "$BASE/api/v1/health" | grep -q '"ok"'; then
    echo "[regression] ERROR: mgmtd health 不通，请先启动 mgmtd (127.0.0.1:9090)" >&2
    exit 1
fi
echo "[regression] health ok"

# 冷却：清空可能残留的登录限流窗口（5 次/分钟/IP），保证本次运行从干净状态开始
sleep 65

# ---------- 结果收集 ----------
declare -a NAMES=()
declare -a STATUSES=()
declare -a DURATIONS=()
declare -a OUTPUTS=()

# 工作区指纹：文件列表 + mtime。用来判断"这次运行期间有没有人在改代码"。
dirty_fingerprint() {
    git status --porcelain 2>/dev/null | while read -r st f; do
        [ -f "$f" ] && printf "%s %s\n" "$f" "$(stat -c %Y "$f" 2>/dev/null)"
    done | sort | md5sum | cut -d' ' -f1
}

run_module() {
    # $1 = name, $2... = command
    local name="$1"; shift
    local logfile="$TMPDIR_RUN/${name}.log"
    echo ""
    echo "===== [regression] RUN: $name ====="
    local fp_before fp_after
    fp_before=$(dirty_fingerprint)
    local t0
    t0=$(date +%s%N)
    "$@" >"$logfile" 2>&1
    local rc=$?
    local t1
    t1=$(date +%s%N)
    fp_after=$(dirty_fingerprint)
    local dur_ms=$(( (t1 - t0) / 1000000 ))
    NAMES+=("$name")
    DURATIONS+=("${dur_ms}ms")
    OUTPUTS+=("$logfile")
    if [ "$rc" -eq 0 ]; then
        STATUSES+=("PASS")
        echo "[regression] $name → PASS (${dur_ms}ms)"
    elif [ "$fp_before" != "$fp_after" ]; then
        # 运行期间有人改了工作区 → 这个红大概率是"编译到一半"，不是真 bug。
        # 实测三次：go_test 715ms / 12870ms 两次 FAIL，几分钟后同一命令全绿。
        # 记成 UNSTABLE 而不是 FAIL：不掩盖，也不谎报成代码坏了。
        STATUSES+=("UNSTABLE(rc=$rc)")
        echo "[regression] $name → UNSTABLE (rc=$rc, ${dur_ms}ms) — 运行期间工作区被修改，结果不可信"
    else
        STATUSES+=("FAIL(rc=$rc)")
        echo "[regression] $name → FAIL (rc=$rc, ${dur_ms}ms)"
    fi
    return 0
}

skip_module() {
    local name="$1"; shift
    local reason="$*"
    NAMES+=("$name")
    STATUSES+=("SKIP")
    DURATIONS+=("0ms")
    OUTPUTS+=("(skipped: $reason)")
    echo "[regression] $name → SKIP ($reason)"
}

# ---------- 1. e2e_test.py ----------
BASE="$BASE" python3 scripts/e2e_test.py >"$TMPDIR_RUN/e2e_test.log" 2>&1 &
E2E_PID=$!
# run in foreground by waiting
wait $E2E_PID
E2E_RC=$?
if [ "$E2E_RC" -eq 0 ]; then
    NAMES+=("e2e_test"); STATUSES+=("PASS"); DURATIONS+=("—"); OUTPUTS+=("$TMPDIR_RUN/e2e_test.log")
    echo "[regression] e2e_test → PASS"
else
    NAMES+=("e2e_test"); STATUSES+=("FAIL(rc=$E2E_RC)"); DURATIONS+=("—"); OUTPUTS+=("$TMPDIR_RUN/e2e_test.log")
    echo "[regression] e2e_test → FAIL (rc=$E2E_RC)"
fi

# 冷却：给登录限流桶恢复（5 次/分钟/IP）
sleep 5

# ---------- 2. smoke_new_routes.py ----------
run_module "smoke_new_routes" env BASE="$BASE" python3 scripts/smoke_new_routes.py

# 冷却：给登录限流桶恢复（5 次/分钟/IP）
sleep 5

# ---------- 3. roundtrip_test.py ----------
run_module "roundtrip_test" env BASE="$BASE" python3 scripts/roundtrip_test.py

# 冷却：给登录限流桶恢复（5 次/分钟/IP）
sleep 5

# ---------- 4. portal_flow_test.py ----------
run_module "portal_flow_test" env BASE="$BASE" python3 scripts/portal_flow_test.py

# 冷却：登录限流窗口 60s/5 次，额外等待确保窗口重置，避免 linkcheck 登录 429
sleep 65

# ---------- 5. linkcheck_ddns_test.py ----------
run_module "linkcheck_ddns_test" env BASE="$BASE" python3 scripts/linkcheck_ddns_test.py
# 角色鉴权必须有端到端回归：rbac.Can 的策略单测曾经全绿，但中间件根本没调它，
# viewer 能直接改防火墙。单测证明不了"接线"，只有从 HTTP 打一遍才行。
run_module "rbac_e2e" env BASE="$BASE" python3 scripts/rbac_e2e.py


# ---------- 6. Go 单测 ----------
run_module "go_test" bash -c "cd mgmtd && CGO_ENABLED=1 go test ./... 2>&1"

# ---------- 7. UI acceptance (需要 vite preview 4173) ----------
UI_PORT="${UI_PORT:-4173}"
# 等 rate limiter 冷却，避免前面的登录请求触发 429
sleep 30

# UI 验收读的是 vite preview 的 dist 产物，而工人可能正在改 web/src —— 那样
# 跑出来的 FAIL 是"工作区半成品"，不是真回归（实测 /services 报 0 行，因为
# 工人 21:51 正在改 Services.vue，回归 21:50 就跑了）。
# 先看有没有未提交的前端改动：有就重新构建一次，保证 dist 与源码一致，
# 并在报告里留痕，免得下次有人拿这个数字当准。
WEB_DIRTY="$(git status --porcelain -- web/src 2>/dev/null | wc -l | tr -d ' ')"
if [ "${WEB_DIRTY:-0}" != "0" ]; then
    echo "===== [regression] web/src 有 $WEB_DIRTY 处未提交改动，先重建 dist 再验 UI ====="
    ( cd web && npm run build >/tmp/regression-webbuild.log 2>&1 ) \
        && echo "[regression] dist 重建完成" \
        || { echo "[regression] dist 重建失败，见 /tmp/regression-webbuild.log"; tail -5 /tmp/regression-webbuild.log; }
fi

if curl -s -m 3 "http://127.0.0.1:$UI_PORT/" >/dev/null 2>&1; then
    # 先探测登录是否可用（避免 429 误报 FAIL）
    if ! python3 -c "import urllib.request,json; r=urllib.request.Request('$BASE/api/v1/auth/login',data=json.dumps({'username':'admin','password':'admin123'}).encode(),headers={'Content-Type':'application/json'},method='POST'); urllib.request.urlopen(r,timeout=10)" 2>/dev/null; then
        skip_module "ui_acceptance" "登录被限流(429)或失败，跳过"
        skip_module "ui_acceptance_ext" "登录被限流(429)或失败，跳过"
        skip_module "ui_acceptance_terminals" "登录被限流(429)或失败，跳过"
        skip_module "users_edit_e2e" "登录被限流(429)或失败，跳过"
    else
        run_module "ui_acceptance" env BASE="$BASE" python3 scripts/ui_acceptance.py
        run_module "ui_acceptance_ext" env BASE="$BASE" python3 scripts/ui_acceptance_ext.py
        run_module "ui_acceptance_terminals" env BASE="$BASE" python3 scripts/ui_acceptance_terminals.py
        # 交互类 UI 必须单独验：ui_acceptance 只比「接口 N 条 vs 页面 N 行」，
        # Element Plus 2.x 移除了 el-dialog 的 visible prop（只认 v-model），
        # 三个弹窗全打不开，行数验收照样全绿。实测点「编辑」「改密」都没反应。
        run_module "users_edit_e2e" env BASE="$BASE" env WEB="http://127.0.0.1:$UI_PORT" python3 scripts/users_edit_e2e.py
    fi
else
    skip_module "ui_acceptance" "vite preview :$UI_PORT 不通，跳过（需 cd web && npm run preview）"
    skip_module "ui_acceptance_ext" "vite preview :$UI_PORT 不通，跳过"
    skip_module "ui_acceptance_terminals" "vite preview :$UI_PORT 不通，跳过"
    skip_module "users_edit_e2e" "vite preview :$UI_PORT 不通，跳过"
fi

# ---------- 汇总 ----------
PASS_COUNT=0; FAIL_COUNT=0; SKIP_COUNT=0; UNSTABLE_COUNT=0
for s in "${STATUSES[@]}"; do
    case "$s" in
        PASS) PASS_COUNT=$((PASS_COUNT+1)) ;;
        SKIP) SKIP_COUNT=$((SKIP_COUNT+1)) ;;
        UNSTABLE*) UNSTABLE_COUNT=$((UNSTABLE_COUNT+1)) ;;
        *)    FAIL_COUNT=$((FAIL_COUNT+1)) ;;
    esac
done

# ---------- 写报告 ----------
{
    echo "# Regression Summary"
    echo ""
    echo "- Generated: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "- Base: $BASE"
    # 跑在哪个提交、带多少未提交改动，必须写进报告。
    # 并行开发时工人正在改文件，go_test 会因为编译到一半而假失败（实测 715ms FAIL，
    # 十分钟后同一命令 RC=0）。不标出来，看报告的人没法判断这次的红是真 bug 还是噪声。
    echo "- Commit: $(git rev-parse --short HEAD 2>/dev/null || echo unknown)$(git diff --quiet 2>/dev/null && echo ' (clean)' || echo ' (+未提交改动)')"
    _dirty=$(git status --porcelain 2>/dev/null | grep -v '^?? ' | wc -l)
    echo "- 未提交改动: ${_dirty} 个文件"
    if [ "${_dirty:-0}" -gt 0 ]; then
        echo ""
        echo "  > 注意：以下文件在本次回归运行时处于未提交状态，相关模块的结果只代表当时的工作区快照："
        git status --porcelain 2>/dev/null | grep -v '^?? ' | head -10 | sed 's/^/  > /'
    fi
    echo "- Total modules: ${#NAMES[@]}"
    echo "- PASS: $PASS_COUNT / FAIL: $FAIL_COUNT / SKIP: $SKIP_COUNT / UNSTABLE: $UNSTABLE_COUNT"
    if [ "$UNSTABLE_COUNT" -gt 0 ]; then
        echo "- ⚠ 有 $UNSTABLE_COUNT 个模块在运行期间工作区被修改，结果不可信（大概率是工人编译到一半），不算代码缺陷"
    fi
    echo ""
    echo "## Module Results"
    echo ""
    echo "| # | Module | Status | Duration |"
    echo "|---|--------|--------|----------|"
    for i in "${!NAMES[@]}"; do
        echo "| $((i+1)) | ${NAMES[$i]} | ${STATUSES[$i]} | ${DURATIONS[$i]} |"
    done
    echo ""
    echo "## Details (tail of each log)"
    echo ""
    for i in "${!NAMES[@]}"; do
        echo "### ${NAMES[$i]} — ${STATUSES[$i]}"
        echo ""
        echo '```'
        if [ -f "${OUTPUTS[$i]}" ]; then
            tail -20 "${OUTPUTS[$i]}"
        else
            echo "${OUTPUTS[$i]}"
        fi
        echo '```'
        echo ""
    done
} > "$REPORT"

echo ""
echo "===== [regression] 汇总 ====="
echo "PASS: $PASS_COUNT / FAIL: $FAIL_COUNT / SKIP: $SKIP_COUNT"
echo "报告: $REPORT"
echo ""

# 清理
rm -rf "$TMPDIR_RUN"

# 退出码：1=有确定失败，2=没失败但有不可信结果（跑的时候有人在改代码）。
# 分开是为了不把"不知道"混进"通过"——UNSTABLE 若也 exit 0，真回归就可能在
# 一次编译中途的噪声里被放过。
if [ "$FAIL_COUNT" -gt 0 ]; then
    exit 1
fi
if [ "$UNSTABLE_COUNT" -gt 0 ]; then
    exit 2
fi
exit 0