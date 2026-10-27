#!/usr/bin/env python3
"""watch_workers.py — 三工人进度快照 + 空闲/阻塞超时主动提醒。

为什么是 Python：`herdr agent list` 返回 JSON，解析终端文本判断"在不在干活"
不可靠（提示符样式会变）。agent_status 是明确的枚举：
    working | blocked | done | idle | unknown

行为：
  1. 进度快照：Go 文件数 / 包数 / 端点数 / 前端视图数，只在变化时写
     logs/worker-watch.log（保持原设计，避免日志刷屏）。
  2. 空闲提醒：窗口处于 idle/done/unknown 且持续 >= IDLE_MINUTES（默认 5）
     → 桌面通知 + logs/worker-alerts.log，每个空闲周期只提醒一次。
  3. 阻塞提醒：窗口进入 blocked（在等你确认/输入）→ 立刻提醒，没有延迟。
  4. 状态持久化：logs/.worker_state.json，脚本重启不会让计时归零。

环境变量：
  IDLE_MINUTES      空闲多少分钟提醒（默认 5）
  PANES             监控哪些窗口（默认 "w1:pN w1:pQ w1:pP"）
  WATCH_INTERVAL    采样间隔秒（默认 30）
  REMIND_REPEAT_MIN 同一空闲周期内隔多久再提醒一次（默认 0 = 只提醒一次）
  NO_NOTIFY=1       只写日志不弹桌面通知（用于测试）
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LOG = ROOT / "logs" / "worker-watch.log"
ALERT = ROOT / "logs" / "worker-alerts.log"
STATE = ROOT / "logs" / ".worker_state.json"

IDLE_MINUTES = float(os.environ.get("IDLE_MINUTES", "5"))
PANES = os.environ.get("PANES", "w1:pN w1:pQ w1:pP").split()
INTERVAL = float(os.environ.get("WATCH_INTERVAL", "30"))
REPEAT_MIN = float(os.environ.get("REMIND_REPEAT_MIN", "0"))
NO_NOTIFY = os.environ.get("NO_NOTIFY") == "1"

HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")


def now() -> float:
    return time.time()


def stamp(t: float | None = None) -> str:
    return time.strftime("%H:%M:%S", time.localtime(t if t is not None else now()))


def append(path: Path, line: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as fh:
        fh.write(line + "\n")


def fmt_idle(minutes: float) -> str:
    """不足 1 分钟显示秒，避免 (0m) 这种看不懂的输出。"""
    return f"{minutes * 60:.0f}s" if minutes < 1 else f"{minutes:.0f}m"


def notify(title: str, body: str, urgency: str = "normal") -> None:
    """桌面通知；失败只警告，不影响监控主循环。"""
    if NO_NOTIFY:
        return
    env = dict(os.environ)
    env.setdefault("DISPLAY", ":10.0")
    bus = f"/run/user/{os.getuid()}/bus"
    if os.path.exists(bus):
        env.setdefault("DBUS_SESSION_BUS_ADDRESS", f"unix:path={bus}")
    try:
        subprocess.run(
            ["notify-send", "-u", urgency, "-a", "router-re", "-t", "8000", title, body],
            env=env, timeout=5, check=False,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
    except Exception as exc:  # noqa: BLE001 - notification must never kill the watcher
        append(ALERT, f"[{stamp()}] notify-send failed: {exc}")


def agent_states() -> dict[str, str]:
    """返回 {pane_id: agent_status}；herdr 不可用时返回空字典。"""
    try:
        out = subprocess.run(
            [HERDR, "agent", "list"], capture_output=True, text=True, timeout=15
        ).stdout
        data = json.loads(out)
        agents = data.get("result", {}).get("agents", [])
        return {a["pane_id"]: a.get("agent_status", "unknown") for a in agents}
    except Exception as exc:  # noqa: BLE001
        append(ALERT, f"[{stamp()}] herdr agent list failed: {exc}")
        return {}


def project_metrics() -> tuple[int, int, int, int]:
    internal = ROOT / "mgmtd" / "internal"
    go_files = len(list(internal.rglob("*.go"))) if internal.exists() else 0
    pkgs = len([d for d in internal.iterdir() if d.is_dir()]) if internal.exists() else 0
    server = ROOT / "mgmtd" / "internal" / "server" / "server.go"
    try:
        endpoints = server.read_text(encoding="utf-8").count("HandleFunc")
    except OSError:
        endpoints = 0
    views = len(list((ROOT / "web" / "src" / "views").glob("*.vue")))
    return go_files, pkgs, endpoints, views


def load_state() -> dict:
    try:
        return json.loads(STATE.read_text(encoding="utf-8"))
    except Exception:  # noqa: BLE001 - corrupt/missing state is not fatal
        return {}


def save_state(state: dict) -> None:
    tmp = STATE.with_suffix(".tmp")
    tmp.write_text(json.dumps(state, indent=2), encoding="utf-8")
    tmp.replace(STATE)


def main() -> int:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    state = load_state()
    idle_since: dict[str, float] = {k: float(v) for k, v in state.get("idle_since", {}).items()}
    # last_remind 故意不继承：进程重启后宁可重复提醒一次，也不能因为上次
    # 运行留下的记录而永久漏提醒（提醒是目的，重复只是噪音）。
    last_remind: dict[str, float] = {}
    blocked_seen: set[str] = set(state.get("blocked_seen", []))

    # 启动基线：已在空闲的窗口从现在开始计时，避免刚启动就弹通知
    probe = agent_states()
    start = now()
    for pane in PANES:
        if probe.get(pane) in ("idle", "done", "unknown") and pane not in idle_since:
            idle_since[pane] = start

    prev_snap = None
    append(LOG, f"[{stamp()}] watcher started (idle={IDLE_MINUTES:g}m interval={INTERVAL:g}s "
                f"panes={','.join(PANES)})")

    while True:
        states = agent_states()
        t = now()

        # ---- 进度快照（只在变化时记录）----
        go_files, pkgs, endpoints, views = project_metrics()
        snap = (go_files, pkgs, endpoints, views)
        if snap != prev_snap:
            parts = " ".join(f"{p.split(':')[1]}={states.get(p, '?')}" for p in PANES)
            append(LOG, f"[{stamp(t)}] go={go_files} pkg={pkgs} ep={endpoints} "
                        f"views={views} | {parts}")
            prev_snap = snap

        # ---- 逐窗口判定 ----
        expired: list[tuple[str, float]] = []
        newly_blocked: list[str] = []

        for pane in PANES:
            st = states.get(pane, "unknown")

            if st == "working":
                idle_since.pop(pane, None)
                last_remind.pop(pane, None)
                blocked_seen.discard(pane)
                continue

            if st == "blocked":
                idle_since.pop(pane, None)
                last_remind.pop(pane, None)
                if pane not in blocked_seen:
                    blocked_seen.add(pane)
                    newly_blocked.append(pane)
                continue

            # idle / done / unknown
            since = idle_since.setdefault(pane, t)
            idle_min = (t - since) / 60.0
            if idle_min < IDLE_MINUTES:
                continue

            last = last_remind.get(pane)
            if last is None:
                expired.append((pane, idle_min))
                last_remind[pane] = t
            elif REPEAT_MIN > 0 and (t - last) / 60.0 >= REPEAT_MIN:
                expired.append((pane, idle_min))
                last_remind[pane] = t

        # ---- 提醒 ----
        if newly_blocked:
            msg = f"{', '.join(newly_blocked)} 在等待输入/确认"
            append(ALERT, f"[{stamp(t)}] BLOCKED {msg}")
            notify("⚠ router-re 工人阻塞", msg, urgency="critical")

        if expired:
            detail = ", ".join(f"{p}({fmt_idle(m)})" for p, m in expired)
            append(ALERT, f"[{stamp(t)}] IDLE {detail}")
            notify(
                "💤 router-re 工人闲置",
                f"{detail}\n空闲超 {IDLE_MINUTES:g} 分钟，考虑派活或收工",
                urgency="normal",
            )

        save_state({
            "idle_since": {k: v for k, v in idle_since.items()},
            "last_remind": {k: v for k, v in last_remind.items()},
            "blocked_seen": sorted(blocked_seen),
        })

        time.sleep(INTERVAL)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
