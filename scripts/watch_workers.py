#!/usr/bin/env python3
"""watch_workers.py — 三工人进度快照 + 空闲/阻塞超时主动提醒。

判定忙闲只能读终端文本，不能信 `herdr agent list` 的 agent_status。
实测：派 `sleep 45` 给 w1:pN，上下文 0%→13%、模型切换，明显在干活，
但 agent_status 全程报 idle。用它会得出「全员空闲」的假结论。

行为：
  1. 进度快照：Go 文件数 / 包数 / 端点数 / 前端视图数，只在变化时写
     logs/worker-watch.log（避免日志刷屏）。
  2. 空闲提醒：pane 尾部是空闲占位符（❯ Ask anything 等）且持续
     >= IDLE_MINUTES（默认 5）→ 桌面通知 + logs/worker-alerts.log，
     每个空闲周期只提醒一次。
  3. unknown（读不到输出 / 既无忙标记也无空闲占位符）不计入空闲 —
     宁可漏提醒，也不要在工人真在干活时弹「闲置」。

环境变量：
  IDLE_MINUTES      空闲多少分钟提醒（默认 5）
  PANES             监控哪些窗口（默认 "w1:pN w1:pQ w1:pP w1:pM"）
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
PANES = os.environ.get("PANES", "w1:pN w1:pQ w1:pP w1:pM").split()
INTERVAL = float(os.environ.get("WATCH_INTERVAL", "30"))
REPEAT_MIN = float(os.environ.get("REMIND_REPEAT_MIN", "0"))
NO_NOTIFY = os.environ.get("NO_NOTIFY") == "1"
# 检测到空闲就立刻触发一轮调度，把"等下一个 5 分钟 timer"变成"一空闲就补活"。
# 快工人完成瞬间最迟 30 秒（一个采样间隔）内就领到下一条任务。
# 必须 flock：否则 watch 触发和 timer 触发会并发跑两个 dispatch 实例，
# 同一批 pane 各派一次 = 重复派发。
AUTO_DISPATCH = os.environ.get("AUTO_DISPATCH", "1") == "1"
DISPATCH_SCRIPT = os.environ.get(
    "DISPATCH_SCRIPT", str(ROOT / "scripts" / "dispatch_workers.py"))
DISPATCH_LOCK = os.environ.get("DISPATCH_LOCK", str(ROOT / "logs" / ".dispatch.lock"))
MIN_IDLE_DISPATCH = float(os.environ.get("MIN_IDLE_DISPATCH", "30"))  # 秒

HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")


def run_dispatch() -> None:
    """检测到空闲后补派一轮。flock 防与 timer 并发；失败只记日志不弹窗。"""
    env = dict(os.environ)
    env.setdefault("PATH", "/home/jimwong/.local/bin:/usr/local/bin:/usr/bin:/bin")
    try:
        r = subprocess.run(
            ["flock", "-n", "-E", "99", DISPATCH_LOCK,
             "/usr/bin/python3", str(DISPATCH_SCRIPT)],
            env=env, capture_output=True, text=True, timeout=180,
        )
        if r.returncode == 99:
            append(ALERT, f"[{stamp()}] dispatch 被占用（timer 在跑），本轮跳过补派")
        elif r.returncode != 0:
            append(ALERT, f"[{stamp()}] dispatch rc={r.returncode}: "
                          f"{(r.stderr or r.stdout or '')[:120]}")
    except Exception as exc:  # noqa: BLE001 - 补派失败绝不影响 watch 主循环
        append(ALERT, f"[{stamp()}] dispatch trigger failed: {exc}")


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
    """已废弃：herdr agent_status 恒为 idle，不能判忙闲。

    实测（2026-10-27）：给 w1:pN 派 `sleep 45` 任务，上下文从 0% 走到 13%、
    模型从 deepseek-v4-flash 切到 AUTO，说明确实在干活，但 `herdr agent list`
    的 agent_status 全程报 idle。保留此函数仅为兼容，主循环不再调用它。
    """
    return {}


# 忙碌标记：Hermes TUI 的 spinner 标签 + 执行指示。
# 缺任何一个都会把真在干活的 worker 误判成空闲（历史踩坑：漏掉 pondering/
# synthesizing/mulling 等词，用户连续三次纠正「两个工人空闲了你居然不知道」）。
BUSY_MARKERS = (
    "Working", "pondering", "synthesizing", "reasoning", "contemplating",
    "processing", "reflecting", "musing", "computing", "formulating",
    "ruminating", "analyzing", "designing", "writing", "mulling",
    "deliberating", "cogitating", "waiting on",
    "💻", "✍️", "📝", "⏳",
    "type to redirect this turn",
    # OpenCode TUI（Build · ... / OpenCode）：干活时状态栏出现 "esc interrupt"。
    # 注意它旁边永远有 "ctrl+p commands" 提示，所以 busy 必须先于 idle 判定。
    "esc interrupt",  # 忙碌时才出现的提示语
)

# 空闲标记：只有这些输入框占位符才代表真的在等输入。
IDLE_MARKERS = (
    "❯ Ask anything",
    "❯ Explain this error",
    "❯ Draft a reply",
    "❯ Type / to browse commands",
    # OpenCode TUI 空闲态状态栏（忙碌时也存在，靠上面 busy 优先命中兜住）
    "ctrl+p commands",
)


def pane_state(pane: str) -> str:
    """读取 pane 尾部文本判定 busy / idle / unknown。

    unknown 故意不当成空闲：误报「闲置」会打扰人，漏报只是少提醒一次。
    """
    try:
        out = subprocess.run(
            [HERDR, "agent", "read", pane, "--lines", "8"],
            capture_output=True, text=True, timeout=15,
        ).stdout
    except Exception as exc:  # noqa: BLE001
        append(ALERT, f"[{stamp()}] herdr agent read {pane} failed: {exc}")
        return "unknown"

    if not out.strip():
        return "unknown"
    if any(m in out for m in BUSY_MARKERS):
        return "busy"
    if any(m in out for m in IDLE_MARKERS):
        return "idle"
    return "unknown"


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

    # 启动基线：已在空闲的窗口从现在开始计时，避免刚启动就弹通知
    probe = {p: pane_state(p) for p in PANES}
    start = now()
    for pane in PANES:
        if probe.get(pane) in ("idle", "done", "unknown") and pane not in idle_since:
            idle_since[pane] = start

    prev_snap = None
    append(LOG, f"[{stamp()}] watcher started (idle={IDLE_MINUTES:g}m interval={INTERVAL:g}s "
                f"panes={','.join(PANES)})")

    while True:
        states = {p: pane_state(p) for p in PANES}
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
        newly_idle: list[str] = []
        prev_state: dict[str, str] = state.get("prev_state", {})

        for pane in PANES:
            st = states.get(pane, "unknown")

            # busy -> idle 转变：上一次采样是 busy，这次是 idle ⇒ 刚干完活
            if st == "idle" and prev_state.get(pane) == "busy":
                newly_idle.append(pane)

            if st == "busy":
                idle_since.pop(pane, None)
                last_remind.pop(pane, None)
                prev_state[pane] = "busy"
                continue

            if st != "idle":
                # unknown / 空输出：不参与空闲计时。宁可漏提醒，
                # 也不要在工人其实在干活时弹「闲置」打扰人。
                prev_state[pane] = st
                continue

            since = idle_since.setdefault(pane, t)
            idle_min = (t - since) / 60.0
            if idle_min < IDLE_MINUTES:
                prev_state[pane] = "idle"
                continue

            last = last_remind.get(pane)
            if last is None:
                expired.append((pane, idle_min))
                last_remind[pane] = t
            elif REPEAT_MIN > 0 and (t - last) / 60.0 >= REPEAT_MIN:
                expired.append((pane, idle_min))
                last_remind[pane] = t
            prev_state[pane] = "idle"

        # ---- 新空闲触发补派（一空闲就领下一条活，不等下一个 timer）----
        if AUTO_DISPATCH and newly_idle:
            for p in newly_idle:
                append(LOG, f"[{stamp(t)}] IDLE->DISPATCH {p} (刚从忙碌转空闲)")
            run_dispatch()
            prev_state = dict(state.get("prev_state", {}))
            for p in newly_idle:
                prev_state[p] = "idle"

        # ---- 提醒 ----
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
            "prev_state": {k: v for k, v in prev_state.items()},
        })

        time.sleep(INTERVAL)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(0)
