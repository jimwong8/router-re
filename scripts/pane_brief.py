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
import subprocess
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


def unit_state(pane: str) -> dict:
    """查 systemd 单元状态。

    为什么心跳不够：进程崩溃后 systemd 立刻拉起，新进程第一件事就是写心跳，
    于是「心跳永远新鲜」——一个每 5 秒崩一次的 lane 在心跳上看是健康的。
    实测自己改坏了 worker_lane.py（删掉了 _heartbeat_loop 的函数定义），
    三个 lane 崩溃重启了两分钟，pane_brief 依然报「闲置·可派活」。
    """
    unit = f"router-re-lane@{pane.replace(':', '-')}.service"
    out = {}
    try:
        r = subprocess.run(["systemctl", "--user", "show", unit,
                            "-p", "ActiveState", "-p", "NRestarts", "-p", "SubState"],
                           capture_output=True, text=True, timeout=8)
        for line in r.stdout.splitlines():
            if "=" in line:
                k, v = line.split("=", 1)
                out[k] = v
    except Exception:  # noqa: BLE001
        pass
    return out


def classify(r: dict) -> tuple[str, str]:
    """把一个 pane 判成「闲置 / 干活中 / 卡住 / 死了」。

    为什么不能只看 state：状态栏的 idle 有四种完全不同的含义——
    手上没活（可派）、活干完了等 lane 结算、活没干完就停了、agent 在等一个
    永远不会来的回答。必须结合「有没有任务」和「内容多久没变」才分得开。
    """
    u = r.get("unit") or {}
    if u.get("ActiveState") and u["ActiveState"] != "active":
        # activating/failed 都意味着进程没在正常循环——崩溃重启循环最常见
        return f"进程异常({u['ActiveState']})", "dead"
    if r["lane_dead"]:
        return "lane停", "dead"
    has_task = bool(r["task"])
    state = r["state"] or "?"
    stalled = r["stalled"]
    if has_task and state == "idle":
        return ("有任务·停滞" if stalled else "收尾中"), ("stuck" if stalled else "busy")
    if state == "blocked":
        return "等回应·卡住", "stuck"
    if has_task:
        return ("卡住·输出不动" if stalled else "干活中"), ("stuck" if stalled else "busy")
    if state == "idle":
        return "闲置·可派活", "idle"
    return f"无任务但{state}", "busy"


def collect() -> dict:
    """采集所有 lane 的状态并分类。

    单独暴露出来是给 lane_watch.py 用的——通知器和人工视图必须走**同一套**
    判定，否则会出现「面板说卡住、通知说正常」这种两边打架的情况。
    """
    lanes = [load(p) for p in sorted(OPS.glob("lane-*.json"))]
    lanes = [l for l in lanes if l.get("pane")]
    tasks = []
    if (OPS / "tasks.jsonl").exists():
        tasks = [json.loads(l) for l in (OPS / "tasks.jsonl").read_text(encoding="utf-8").splitlines()
                 if l.strip()]
    rows = []
    for l in lanes:
        pane = l.get("pane") or ""
        age = time.time() - (l.get("updated") or 0)
        hb_path = OPS / f"lane-{pane.replace(':', '-')}.hb"
        try:
            hb_age = time.time() - float(hb_path.read_text().strip())
        except Exception:  # noqa: BLE001
            hb_age = None
        rows.append({
            "pane": pane,
            "state": l.get("state"),
            "task": l.get("task"),
            "elapsed_min": l.get("elapsed_min"),
            "stall_sec": l.get("stall_sec"),
            "stalled": bool(l.get("stalled")),
            "last_line": l.get("last_line") or "",
            "hb_age_sec": None if hb_age is None else round(hb_age, 1),
            "unit": unit_state(pane),
            "lane_dead": hb_age is None or hb_age > 15,
            "busy_long": (hb_age is not None and hb_age <= 15) and age > 45,
        })
    for r in rows:
        r["label"], r["kind"] = classify(r)
    return {
        "panes": rows,
        "pending": sum(1 for t in tasks if t.get("status") == "pending"),
        "dispatched": sum(1 for t in tasks if t.get("status") == "dispatched"),
        "verify_queue": sum(1 for l in (OPS / "verify-queue.jsonl").read_text(encoding="utf-8").splitlines()
                            if l.strip()) if (OPS / "verify-queue.jsonl").exists() else 0,
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true", help="输出 JSON 而不是表格")
    args = ap.parse_args()

    d = collect()
    rows = d["panes"]
    pending_n, dispatched_n, vq = d["pending"], d["dispatched"], d["verify_queue"]
    if args.json:
        print(json.dumps({"panes": rows, "pending": pending_n,
                          "dispatched": dispatched_n, "verify_queue": vq},
                         ensure_ascii=False, indent=2))
        return 0

    if not rows:
        print("没有 lane 状态文件（ops/lane-*.json）——lane 没起，或还没跑过一轮")
    else:
        print(f"{'pane':8} {'判定':14} {'状态':8} {'任务':24} {'停滞':>6}  最后一行")
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
            label, _ = classify(r)
            print(f"{r['pane']:8} {label:14} {state:8} {task:24} {st:>6} {flag} {r['last_line'][:46]}")

    print()
    kinds = [classify(r)[1] for r in rows]
    n_idle = sum(1 for k in kinds if k == "idle")
    n_busy = sum(1 for k in kinds if k == "busy")
    n_stuck = sum(1 for k in kinds if k in ("stuck", "dead"))
    print(f"窗口：{n_idle} 个闲置可派 · {n_busy} 个在忙 · {n_stuck} 个有问题")
    print(f"待派 {pending_n} 条 · 进行中 {dispatched_n} 条 · 待验证 {vq} 条")
    # 只报「有任务却不动」的；没任务的窗口本来就不该有输出变化
    stalled = [r["pane"] for r in rows if r["stalled"] and r["task"]]
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
