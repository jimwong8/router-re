#!/usr/bin/env python3
"""pane_brief.py — 一条命令看清所有工人窗口「现在在干什么」。

主模型（我）不是一个常驻进程，没法实时订阅每个 pane 的对话。worker_lane 每个
pane 常驻一个进程，会把状态写进 ops/lane-<pane>.json（含停滞秒数和尾部最后一行）。
这个脚本把它们汇总成一张表，让我每次只需要一条命令就能回答：

  - 哪个窗口在干活、干的是哪条任务、干了多久
  - 状态栏说 busy 但输出多久没变化（stalled）
  - 最后一句话是什么（是不是在问一个没人回答的问题）
  - 队列里还有多少活、验证队列积压多少

用法：
    python3 scripts/pane_brief.py
    python3 scripts/pane_brief.py --json     # 给程序消费
"""
from __future__ import annotations

import argparse
import json
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OPS = ROOT / "ops"


def load(path: Path) -> dict:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:  # noqa: BLE001
        return {}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true", help="输出 JSON 而不是表格")
    args = ap.parse_args()

    lanes = [load(p) for p in sorted(OPS.glob("lane-*.json"))]
    lanes = [l for l in lanes if l.get("pane")]

    tasks = [json.loads(l) for l in (OPS / "tasks.jsonl").read_text(encoding="utf-8").splitlines()
             if l.strip()] if (OPS / "tasks.jsonl").exists() else []
    pending = [t for t in tasks if t.get("status") == "pending"]
    dispatched = [t for t in tasks if t.get("status") == "dispatched"]

    vq = []
    if (OPS / "verify-queue.jsonl").exists():
        vq = [json.loads(l) for l in (OPS / "verify-queue.jsonl").read_text(encoding="utf-8").splitlines() if l.strip()]

    rows = []
    for l in lanes:
        age = time.time() - (l.get("updated") or 0)
        pane = l.get("pane") or ""
        hb_path = OPS / f"lane-{pane.replace(':', '-')}.hb"
        try:
            hb_age = time.time() - float(hb_path.read_text().strip())
        except Exception:  # noqa: BLE001
            hb_age = None
        rows.append({
            "pane": l.get("pane"),
            "state": l.get("state"),
            "task": l.get("task"),
            "elapsed_min": l.get("elapsed_min"),
            "stall_sec": l.get("stall_sec"),
            "stalled": bool(l.get("stalled")),
            "last_line": l.get("last_line") or "",
            "heartbeat_age_sec": round(age, 1),
            "hb_age_sec": None if hb_age is None else round(hb_age, 1),
            # 存活看心跳（每 3 秒一次，15 秒没有就是真死了）。
            # 不能用循环进度（age）判死：一轮里 go build 最长 240 秒不写状态。
            "lane_dead": hb_age is None or hb_age > 15,
            # 心跳在、循环很久没更新 = 卡在长操作（构建/测试），不是死了
            "busy_long": (hb_age is not None and hb_age <= 15) and age > 45,
        })

    if args.json:
        print(json.dumps({"panes": rows, "pending": len(pending),
                          "dispatched": len(dispatched), "verify_queue": len(vq)},
                         ensure_ascii=False, indent=2))
        return 0

    if not rows:
        print("没有 lane 状态文件（ops/lane-*.json）——lane 没起，或还没跑过一轮")
    else:
        print(f"{'pane':8} {'状态':10} {'任务':26} {'用时':>7} {'停滞':>7}  最后一行")
        print("-" * 110)
        for r in rows:
            task = (r["task"] or "-")[:24]
            el = f"{r['elapsed_min']:.1f}m" if isinstance(r["elapsed_min"], (int, float)) else "-"
            st = f"{r['stall_sec']:.0f}s" if isinstance(r["stall_sec"], (int, float)) else "-"
            flag = "⚠" if r["stalled"] or r["lane_dead"] else " "
            state = r["state"] or "-"
            if r["lane_dead"]:
                state += "(lane停)"
            elif r["busy_long"]:
                state += "(长操作)"
            print(f"{r['pane']:8} {state:10} {task:26} {el:>7} {st:>7} {flag} {r['last_line'][:52]}")

    print()
    print(f"待派 {len(pending)} 条 · 进行中 {len(dispatched)} 条 · 待验证 {len(vq)} 条")
    stalled = [r["pane"] for r in rows if r["stalled"]]
    dead = [r["pane"] for r in rows if r["lane_dead"]]
    if stalled:
        print(f"⚠ 停滞（尾部长时间无变化）：{', '.join(stalled)}")
    if dead:
        print(f"⚠ lane 心跳过期（进程可能已死）：{', '.join(dead)}")
    longops = [r["pane"] for r in rows if r["busy_long"]]
    if longops:
        print(f"… 循环超过 45s 未更新（多半在构建/测试，心跳正常）：{', '.join(longops)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
