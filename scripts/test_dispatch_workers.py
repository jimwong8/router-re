#!/usr/bin/env python3
"""dispatch_workers 修复的回归测试。

覆盖三个必须成立的断言：
  1. pane 卡在重试（blocked）+ 上下文高水位 -> 先重置会话，重置成功后才派活
  2. pane 卡在重试但水位正常 -> 不重置、不派活（让重试自己恢复）
  3. pane 空闲但水位高 -> 先重置再派活
另加一条：正常的 idle + 低水位 -> 直接派活，不被新逻辑误伤。
"""
import io
import json
import sys
import tempfile
from contextlib import redirect_stdout
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "scripts"))
import dispatch_workers as d  # noqa: E402

FAILED = []


def check(name, cond, detail=""):
    print(f"  {'PASS' if cond else 'FAIL'}  {name}" + (f"   {detail}" if detail else ""))
    if not cond:
        FAILED.append(name)


def run_case(pane_state_value, ctx, expect_reset, expect_dispatch, name,
             state_after_reset="idle"):
    """跑一轮 main()，返回日志。

    state_after_reset 模拟重置成功后的会话状态：真实环境里 /new 之后 TUI
    回到空提示（idle）；若上游仍在限流，它可能立刻又变回 blocked —— 这两种
    都要覆盖，后者用于确认"重置了但还没恢复时绝不派活"。
    """
    with tempfile.TemporaryDirectory() as td:
        td = Path(td)
        d.QUEUE = td / "tasks.jsonl"
        d.STATE = td / ".state.json"
        d.LOG = td / "dispatch.log"
        d.QUEUE.write_text(json.dumps({
            "id": "t1", "prompt": "do something",
            "expect": ["out.txt"], "prefer": "w1:pX",
        }, ensure_ascii=False) + "\n", encoding="utf-8")
        # 产出文件不存在，避免被误判 done
        d.PANES_RAW = ["w1:pX"]
        d.resolve_panes = lambda: ["w1:pX"]  # 不真调 herdr
        d.MAX_PER_ROUND = 0  # 自动 = pane 数

        calls = {"reset": 0, "dispatch": 0}
        cur = {"state": pane_state_value}

        d.pane_state = lambda p: cur["state"]
        d.context_pct = lambda p: ctx

        def fake_reset(p):
            calls["reset"] += 1
            cur["state"] = state_after_reset
            return True

        def fake_dispatch(p, prompt):
            calls["dispatch"] += 1
            return True

        d.reset_context = fake_reset
        d.dispatch = fake_dispatch

        buf = io.StringIO()
        with redirect_stdout(buf):
            d.main()

        got_reset = calls["reset"] > 0
        got_dispatch = calls["dispatch"] > 0
        check(f"{name}: 重置={expect_reset}", got_reset == expect_reset, f"实际 {got_reset}")
        check(f"{name}: 派发={expect_dispatch}", got_dispatch == expect_dispatch, f"实际 {got_dispatch}")
        return buf.getvalue()


print("=== 场景 1: blocked + 水位 70% -> 应重置并派活 ===")
log = run_case("blocked", 70.0, True, True, "blocked/70%")
check("日志含死锁判定", "TPM 死锁" in log, log.strip()[-80:] if log.strip() else "空")

print("\n=== 场景 2: blocked + 水位 20% -> 不重置不派活 ===")
log = run_case("blocked", 20.0, False, False, "blocked/20%")
check("日志含跳过原因", "本轮跳过" in log, log.strip()[-80:] if log.strip() else "空")

print("\n=== 场景 3: idle + 水位 70% -> 先重置再派活 ===")
run_case("idle", 70.0, True, True, "idle/70%")

print("\n=== 场景 4: idle + 水位 20% -> 直接派活（不误伤）===")
run_case("idle", 20.0, False, True, "idle/20%")

print("\n=== 场景 5: idle + 水位读不到(None) -> 直接派活（不误伤）===")
run_case("idle", None, False, True, "idle/None")

print("\n=== 场景 6: blocked + 70%，重置后仍 blocked -> 不派活（防二次堆积）===")
run_case("blocked", 70.0, True, False, "blocked/70%/仍blocked", state_after_reset="blocked")

def run_custom(tasks, pane_state_val="idle", ctx=20.0, panes=("w1:pX",)):
    """跑一轮 main()，返回 (是否派发, 最终 tasks, 日志)。"""
    with tempfile.TemporaryDirectory() as td:
        td = Path(td)
        d.QUEUE = td / "tasks.jsonl"
        d.STATE = td / ".state.json"
        d.LOG = td / "dispatch.log"
        d.QUEUE.write_text(
            "\n".join(json.dumps(x, ensure_ascii=False) for x in tasks) + "\n",
            encoding="utf-8")
        d.PANES_RAW = list(panes)
        d.MAX_PER_ROUND = 0  # 自动 = pane 数
        d.resolve_panes = lambda: list(panes)  # 不真调 herdr

        calls = {"dispatch": 0}
        d.pane_state = lambda p: pane_state_val
        d.context_pct = lambda p: ctx
        d.reset_context = lambda p: True

        def fake_dispatch(p, prompt):
            calls["dispatch"] += 1
            return True
        d.dispatch = fake_dispatch

        buf = io.StringIO()
        with redirect_stdout(buf):
            d.main()
        final = [json.loads(l) for l in d.QUEUE.read_text(encoding="utf-8").splitlines() if l.strip()]
        return calls["dispatch"] > 0, final, buf.getvalue()


# ---------- 新增门禁：依赖 / 退避 / 硬预算 ----------

print("\n=== 场景 7: 依赖未完成 -> 不派 ===")
_, final, log7 = run_custom([
    {"id": "api", "status": "pending", "prompt": "do api"},
    {"id": "ui", "status": "pending", "prompt": "do ui", "depends_on": ["api"]},
])
hmm = [t for t in final if t["id"] == "ui"][0]
check("依赖未完成时 ui 仍未派发", hmm["status"] == "pending", f"实际 {hmm['status']}")
check("api 被派发（无依赖）", [t for t in final if t['id']=='api'][0]["status"] == "dispatched")

print("\n=== 场景 8: 依赖已完成 -> 可派 ===")
_, final, _ = run_custom([
    {"id": "api", "status": "done", "prompt": "do api"},
    {"id": "ui", "status": "pending", "prompt": "do ui", "depends_on": ["api"]},
])
ui = [t for t in final if t["id"] == "ui"][0]
# 一轮只派一条，api 已 done 所以 ui 应该被派到
check("依赖完成后 ui 被派发", ui["status"] == "dispatched", f"实际 {ui['status']}")

print("\n=== 场景 9: 退避窗口未到 -> 不派 ===")
import time as _t
_, final, _ = run_custom([
    {"id": "r1", "status": "pending", "prompt": "retry me", "retry_after": _t.time() + 300},
])
check("退避期内不派发", [t for t in final if t['id']=='r1'][0]["status"] == "pending")

print("\n=== 场景 10: 退避窗口已过 -> 可派 ===")
_, final, _ = run_custom([
    {"id": "r2", "status": "pending", "prompt": "retry me", "retry_after": _t.time() - 10},
])
check("退避结束后派发", [t for t in final if t['id']=='r2'][0]["status"] == "dispatched")

print("\n=== 场景 11: 超过硬预算 -> 退回 pending + 设 retry_after ===")
_, final, log11 = run_custom([
    {"id": "slow", "status": "dispatched", "pane": "w1:pX", "prompt": "slow work",
     "dispatched_at": _t.time() - 25 * 60, "expect": ["nope_missing_file_xyz"]},
])
slow = [t for t in final if t["id"] == "slow"][0]
check("超预算后状态回到 pending", slow["status"] == "pending", f"实际 {slow['status']}")
check("设置了 retry_after", "retry_after" in slow, f"{slow.get('retry_after')}")
check("retried 计数为 1", slow.get("retried") == 1, f"{slow.get('retried')}")
check("日志有 RETRY", "RETRY" in log11, log11.strip()[-90:] if log11.strip() else "空")

print("\n=== 场景 12: 重试耗尽 -> failed ===")
_, final, _ = run_custom([
    {"id": "dead", "status": "dispatched", "pane": "w1:pX", "prompt": "dead work", "retried": 3,
     "dispatched_at": _t.time() - 25 * 60, "expect": ["nope_missing_file_xyz"]},
])
dead = [t for t in final if t["id"] == "dead"][0]
check("重试耗尽标记 failed", dead["status"] == "failed", f"实际 {dead['status']}")

print("\n=== 场景 13: 任务级 max_wall_minutes 放宽预算 ===")
# 必须用 busy：pane 空闲 4 分钟会先触发 RELEASE（工人自认做完了），
# 那是另一条判据，会掩盖这里要验证的 wall_limit。
_, final, _ = run_custom([
    {"id": "long", "status": "dispatched", "pane": "w1:pX", "prompt": "long work", "max_wall_minutes": 60,
     "dispatched_at": _t.time() - 25 * 60, "expect": ["nope_missing_file_xyz"]},
], pane_state_val="busy")
longt = [t for t in final if t["id"] == "long"][0]
check("声明 60 分钟预算的任务不被 20 分钟默认值杀掉",
      longt["status"] == "dispatched", f"实际 {longt['status']}")

# ---------- tier 路由：判断类任务只给强模型 pane ----------

print("\n=== 场景 14: tier=verify 只派给 strong pane（pM）===")
with tempfile.TemporaryDirectory() as td:
    td = Path(td)
    d.QUEUE, d.STATE, d.LOG = td/"tasks.jsonl", td/".state.json", td/"dispatch.log"
    d.QUEUE.write_text(json.dumps({"id":"v1","status":"pending","prompt":"审这个","tier":"verify"})+"\n", encoding="utf-8")
    d.PANES_RAW = ["w1:pN", "w1:pM"]          # pN=free, pM=strong
    d.STRONG_PANES = {"w1:pM"}
    d.resolve_panes = lambda: ["w1:pN", "w1:pM"]
    d.MAX_PER_ROUND = 1
    d.pane_state = lambda p: "idle"
    d.context_pct = lambda p: 10.0
    d.reset_context = lambda p: True
    served = []
    d.dispatch = lambda p, prompt: (served.append(p) or True)
    buf = io.StringIO()
    with redirect_stdout(buf):
        d.main()
check("verify 任务被派给 pM（strong）", served == ["w1:pM"], f"实际 {served}")

print("\n=== 场景 15: 只有 free pane 时 tier=verify 不派（宁可不做也不给弱模型）===")
with tempfile.TemporaryDirectory() as td:
    td = Path(td)
    d.QUEUE, d.STATE, d.LOG = td/"tasks.jsonl", td/".state.json", td/"dispatch.log"
    d.QUEUE.write_text(json.dumps({"id":"v2","status":"pending","prompt":"审这个","tier":"verify"})+"\n", encoding="utf-8")
    d.PANES_RAW = ["w1:pN", "w1:pQ"]
    d.STRONG_PANES = {"w1:pM"}  # 本轮没有 strong pane
    d.resolve_panes = lambda: ["w1:pN", "w1:pQ"]
    d.MAX_PER_ROUND = 1
    d.pane_state = lambda p: "idle"
    d.context_pct = lambda p: 10.0
    d.reset_context = lambda p: True
    served = []
    d.dispatch = lambda p, prompt: (served.append(p) or True)
    buf = io.StringIO()
    with redirect_stdout(buf):
        d.main()
    final = [json.loads(l) for l in d.QUEUE.read_text().splitlines() if l.strip()]
check("free pane 不领 verify 任务", served == [], f"实际 {served}")
check("任务保持 pending 等 strong pane", final[0]["status"] == "pending", f"实际 {final[0]['status']}")

print("\n=== 场景 16: tier=exec（默认）任何 pane 都能领 ===")
with tempfile.TemporaryDirectory() as td:
    td = Path(td)
    d.QUEUE, d.STATE, d.LOG = td/"tasks.jsonl", td/".state.json", td/"dispatch.log"
    d.QUEUE.write_text(json.dumps({"id":"e1","status":"pending","prompt":"改个文件"})+"\n", encoding="utf-8")
    d.PANES_RAW = ["w1:pN"]
    d.resolve_panes = lambda: ["w1:pN"]
    d.MAX_PER_ROUND = 1
    d.pane_state = lambda p: "idle"
    d.context_pct = lambda p: 10.0
    d.reset_context = lambda p: True
    served = []
    d.dispatch = lambda p, prompt: (served.append(p) or True)
    buf = io.StringIO()
    with redirect_stdout(buf):
        d.main()
check("exec 任务派给 free pane", served == ["w1:pN"], f"实际 {served}")

print("\n=== 汇总 ===")
if FAILED:
    print(f"  {len(FAILED)} 项失败: {FAILED}")
    sys.exit(1)
print("  全部通过")
