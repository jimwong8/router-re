#!/usr/bin/env python3
"""worker_supervisor.py — 多窗口工人的常驻自愈守护。

一拖几模式里，三条 lane（worker_lane.py）已经各自盯自己窗格、完成即结算即派下一条；
watch_workers.py 只做进度快照 + 桌面通知。但还差一个「自愈」环节——当窗格因为
上游 429 / 窗格 UI 态坏掉 / 残留旧编译报错而卡死时，lane 只能判 idle/busy/unknown，
不会主动把窗格救回来。结果就是「四个窗口全停」：工人其实没偷懒，是窗格卡住没人修。

本进程补齐这个环节。它常驻、每 INTERVAL 秒采样一次，对每个窗格做四类自愈：

  1. 429 限流        → 等过 reset 时间后发 `/retry`（不是 `/new`，保住已读任务上下文）
  2. 窗格卡死(unknown)→ `reset_context`（esc + /new，把会话重置回干净 idle）
  3. 残留旧编译报错   → 先 `go build ./...` 确认真实编译态；RC=0 则只重置窗格，不动代码
  4. lane 进程消失     → systemd 会 Restart=always，这里只告警写日志

原则（全部来自 multi-window-worker-orchestration skill 的实测教训）：
  - 429 不要连发 /retry：三窗同频打 llm-gateway 会互相踩配额，要错峰（每个窗格有独立退避）。
  - 不能靠 `agent_status`（恒 idle），只能读终端文本判定。
  - 绝不 pkill lane（systemd 秒拉回，且 -f 会匹配自身 shell 自杀）；要重置的是窗格不是 lane。
  - 「变化才写日志」，避免日志噪声。

环境变量：
  PANES              监控哪些窗口（默认 "w1:pN w1:pQ w1:pP w1:pM"）
  SUPERVISE_INTERVAL 采样间隔秒（默认 20）
  RETRY_COOLDOWN     同一窗格两次 /retry 的最小间隔秒（默认 120，错峰防踩配额）
  RESET_COOLDOWN     同一窗格两次 /new 重置的最小间隔秒（默认 300）
  NO_ACT=1           只诊断写日志，不实际发 retry/reset（干跑）
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LOG = ROOT / "logs" / "worker-supervisor.log"
STATE = ROOT / "logs" / ".worker_supervisor_state.json"

PANES = os.environ.get("PANES", "w1:pN w1:pQ w1:pP w1:pM").split()
INTERVAL = float(os.environ.get("SUPERVISE_INTERVAL", "20"))
RETRY_COOLDOWN = float(os.environ.get("RETRY_COOLDOWN", "120"))
RESET_COOLDOWN = float(os.environ.get("RESET_COOLDOWN", "300"))
NO_ACT = os.environ.get("NO_ACT") == "1"

HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")

# 429 / 限流 特征词（从真实捕获文本摘，见 skill 生产教训 11/27）
RATE_LIMIT_MARKERS = [
    "429", "rate limit", "rate limited", "rate-limited",
    "no available channel", "channel rate gate", "provider circuit open",
    "insufficient_quota", "inference exceeds tpm", "tpm/rpm limit",
    "resets in", "resets at",
]
# 空闲占位符（大小写不敏感判定在函数里做）
IDLE_MARKERS = [
    "type / to browse commands", "ctrl+p for the palette", "ctrl+p commands",
    "ask anything",
]


def now() -> float:
    return time.time()


def stamp(t: float | None = None) -> str:
    return time.strftime("%H:%M:%S", time.localtime(t or now()))


def log(line: str) -> None:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    with open(LOG, "a") as f:
        f.write(f"[{stamp()}] {line}\n")


def load_state() -> dict:
    if STATE.exists():
        try:
            return json.loads(STATE.read_text())
        except Exception:
            pass
    return {}


def save_state(state: dict) -> None:
    STATE.parent.mkdir(parents=True, exist_ok=True)
    STATE.write_text(json.dumps(state))


def read_tail(pane: str, lines: int = 20) -> str:
    try:
        return subprocess.run(
            [HERDR, "agent", "read", pane, "--lines", str(lines)],
            capture_output=True, text=True, timeout=15,
        ).stdout
    except Exception as exc:
        return ""


def classify(text: str) -> str:
    """把窗格尾部文本分类成：busy / idle / rate_limit / unknown。

    注意 429 有两种屏幕形态：
      (a) 重试中：尾部含 `⏳ rate limited` + `type to redirect` —— 明显在跑，重试在自动进行
      (b) 已放弃：含 `Final error: HTTP 429` + `Explain this error` 或
          `Limit resets at` + `Type / to browse commands` —— 模型已停手，等人 /retry
    只有 (b) 才需要自愈发 /retry；(a) 的重试不要打断。
    并且限流形态里同时带 idle 占位符，限流必须判在 idle 之前。
    """
    low = text.lower()
    if not text.strip():
        return "unknown"
    # 会话历史损坏：/retry 报 rewind failed，尾巴虽然带 idle 占位符但会话已经废了，
    # 只能 /new 重置。必须先于 idle 判定，否则 supervisor 会把它当正常空闲放着不动。
    if "retry rewind failed" in low or "history was not changed" in low \
       or "rewind failed" in low:
        return "broken"
    # 429 已放弃形态：Final error / 已停手（Explain this error 或建议切模型），必须先判
    if ("final error" in low and ("429" in text or "rate limit" in low)) \
       or ("limit resets at" in low and "explain this error" in low):
        return "rate_limit"
    # 429 重试中形态：模型还在自动重试，不要打断，按忙处理
    if "⏳" in text and "rate limit" in low:
        return "busy"
    if any(m in low for m in RATE_LIMIT_MARKERS):
        # limit resets at + idle 占位符 = 停手形态 → rate_limit
        return "rate_limit"
    if any(m in low for m in IDLE_MARKERS):
        return "idle"
    # 有工作状态词算忙
    busy_words = ["working", "pondering", "synthesizing", "reasoning",
                  "processing", "reflecting", "computing", "writing",
                  "analyzing", "designing", "deliberating", "preparing terminal",
                  "type to redirect this turn", "run ", "ran ", "read "]
    if any(m in low for m in busy_words):
        return "busy"
    return "unknown"


def send_retry(pane: str) -> bool:
    r = subprocess.run([HERDR, "agent", "prompt", pane, "/retry"],
                       capture_output=True, text=True, timeout=30)
    return r.returncode == 0


def send_reset(pane: str) -> bool:
    """esc + /new，重置会话。返回是否成功。"""
    try:
        subprocess.run([HERDR, "agent", "send-keys", pane, "esc"],
                       capture_output=True, text=True, timeout=15)
    except Exception:
        pass
    time.sleep(2)
    r = subprocess.run([HERDR, "agent", "prompt", pane, "/new"],
                       capture_output=True, text=True, timeout=30)
    return r.returncode == 0


def go_build_ok() -> bool:
    """确认真实编译态：RC=0 说明窗格里的是旧报错残影，不用动代码。"""
    try:
        r = subprocess.run(
            ["go", "build", "./..."],
            cwd=str(ROOT / "mgmtd"),
            capture_output=True, text=True, timeout=180,
        )
        return r.returncode == 0
    except Exception:
        return False


def main() -> int:
    state = load_state()
    last_retry = {k: float(v) for k, v in state.get("last_retry", {}).items()}
    last_reset = {k: float(v) for k, v in state.get("last_reset", {}).items()}

    log(f"supervisor started panes={','.join(PANES)} interval={INTERVAL:.0f}s "
        f"retry_cooldown={RETRY_COOLDOWN:.0f}s reset_cooldown={RESET_COOLDOWN:.0f}s "
        f"no_act={NO_ACT}")

    while True:
        t = now()
        for pane in PANES:
            text = read_tail(pane)
            st = classify(text)
            act = ""

            if st == "rate_limit":
                # 等过 retry 冷却再发 /retry，错峰防三窗同频踩配额
                if t - last_retry.get(pane, 0) >= RETRY_COOLDOWN:
                    ok = False if NO_ACT else send_retry(pane)
                    last_retry[pane] = t
                    act = "RETRY"
                    log(f"{pane} rate_limit -> {act} (ok={ok})")
            elif st == "broken":
                # 会话历史损坏（retry rewind failed）：/retry 救不回来，只能 /new 重置
                if t - last_reset.get(pane, 0) >= RESET_COOLDOWN:
                    ok = False if NO_ACT else send_reset(pane)
                    last_reset[pane] = t
                    act = "RESET(broken)"
                    log(f"{pane} broken(rewind failed) -> {act} (ok={ok})")
                else:
                    log(f"{pane} broken 但 reset 冷却中({RESET_COOLDOWN - (t - last_reset.get(pane, 0)):.0f}s剩)")
            elif st == "unknown":
                # 窗格卡死/坏态/残留旧报错：先确认真实编译态再决定
                build = go_build_ok()
                if t - last_reset.get(pane, 0) >= RESET_COOLDOWN:
                    ok = False if NO_ACT else send_reset(pane)
                    last_reset[pane] = t
                    act = f"RESET(build={'ok' if build else 'FAIL'})"
                    log(f"{pane} unknown -> {act} (ok={ok})")
                else:
                    log(f"{pane} unknown 但 reset 冷却中({RESET_COOLDOWN - (t - last_reset.get(pane, 0)):.0f}s剩)")
            # idle / busy 不做动作：idle 交给 lane 派活，busy 是正常的

        save_state({"last_retry": last_retry, "last_reset": last_reset})
        time.sleep(INTERVAL)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log("supervisor 收到中断，退出")
