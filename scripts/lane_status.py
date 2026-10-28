#!/usr/bin/env python3
"""lane_status.py — per-pane lane 状态汇总（只读）。

读 ops/lane-*.json + verify-queue + 队列，打印每个 pane 一行
（pane/state/当前任务/已跑分钟/上下文水位），以及验证队列长度、
最近 5 条验证结论。

用法: python3 scripts/lane_status.py
"""
from __future__ import annotations
import glob, json, os, time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OPS = ROOT / "ops"
LOGS = ROOT / "logs"

def fmt_age(ts):
    if not ts:
        return "—"
    delta = time.time() - ts
    if delta < 60:
        return f"{delta:.0f}s"
    if delta < 3600:
        return f"{delta/60:.1f}m"
    return f"{delta/3600:.1f}h"

def main():
    lanes = sorted(glob.glob(str(OPS / "lane-*.json")))
    print(f"{'pane':<10} {'state':<8} {'task':<32} {'elapsed':>8} {'ctx_kb':>8} {'updated':>8}")
    print("-" * 80)
    for lf in lanes:
        try:
            d = json.loads(Path(lf).read_text())
        except Exception as e:
            print(f"{lf}: parse error {e}")
            continue
        pane = d.get("pane", os.path.basename(lf).replace("lane-", "").replace(".json", ""))
        state = d.get("state", "?")
        task = d.get("task", "—") or "—"
        elapsed = d.get("elapsed_min", 0)
        elapsed_str = f"{elapsed:.1f}m" if isinstance(elapsed, (int, float)) else str(elapsed)
        # 上下文水位：用日志文件大小 KB 作代理
        log_path = LOGS / f"lane-{pane.replace(':', '-')}.log"
        ctx_kb = "—"
        if log_path.exists():
            ctx_kb = f"{log_path.stat().st_size / 1024:.1f}"
        updated = fmt_age(d.get("updated"))
        print(f"{pane:<10} {state:<8} {task:<32} {elapsed_str:>8} {ctx_kb:>8} {updated:>8}")

    print()
    # verify queue
    vq = OPS / "verify-queue.jsonl"
    vlines = []
    if vq.exists():
        vlines = [l for l in vq.read_text().splitlines() if l.strip()]
    print(f"验证队列长度: {len(vlines)}")

    # 最近 5 条验证结论：从 verify-queue 或 tasks.jsonl 取 done 的
    conclusions = []
    tasks_file = OPS / "tasks.jsonl"
    if tasks_file.exists():
        for line in tasks_file.read_text().splitlines():
            if not line.strip():
                continue
            try:
                t = json.loads(line)
            except Exception:
                continue
            if t.get("status") in ("done", "verified", "pass"):
                conclusions.append(t)
    # 按 done_at 排序取最近 5
    conclusions.sort(key=lambda t: t.get("done_at", t.get("created_at", 0)), reverse=True)
    recent = conclusions[:5]
    print()
    print("最近 5 条验证结论:")
    if not recent:
        print("  (无)")
    else:
        for t in recent:
            tid = t.get("id", "?")
            status = t.get("status", "?")
            done_at = t.get("done_at")
            when = time.strftime("%H:%M:%S", time.localtime(done_at)) if done_at else "—"
            print(f"  [{when}] {tid}: {status}")

if __name__ == "__main__":
    main()