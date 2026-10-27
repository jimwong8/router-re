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
        d.PANES = ["w1:pX"]
        d.MAX_PER_ROUND = 4

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

print("\n=== 汇总 ===")
if FAILED:
    print(f"  {len(FAILED)} 项失败: {FAILED}")
    sys.exit(1)
print("  全部通过")
