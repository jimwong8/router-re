#!/usr/bin/env python3
"""worker_lane.py — 一个工人一条常驻通道（per-pane lane）。

## 为什么要换掉"每 5 分钟扫一次"

旧模型是单进程、每 300s 扫全表：所有工人共用同一个节奏。后果是显式的——
A 工人 2 分钟就交活，却要等到本轮扫描才发现；B 工人干 20 分钟，A 就在那干等
18 分钟。谁快谁慢完全不体现在调度上，"并行"只是名义上的。

lane 模型把节奏降到每个工人自己身上：

    派一条 → 只盯这一个 pane → 它一 idle 且产出达标 → 立刻结算 → 立刻派下一条

不做批量、不扫别人、不等整轮。单个 lane 的响应延迟 = 轮询间隔（默认 3s），
与其它工人是否完成无关。

## 单 lane 的状态机

    ┌─────────────┐  抢到任务   ┌────────────┐  produced()  ┌────────┐
    │ 手上没任务  │ ──────────> │ dispatched │ ───────────> │  done  │
    └─────────────┘             └────────────┘              └────────┘
        ↑                             │ 超时 / 重试超限
        │                             ↓
        └───────────────────────── RELEASE（回 pending，释放 pane）

## 与 dispatch_workers.py 的关系

判定逻辑（pane_state / produced / deps_satisfied / 上下文水位 / 派发措辞）全部
复用 dispatch_workers，不重写。dispatch_workers.py 退化成 watchdog：只在 lane
进程挂了时兜底收拾孤儿任务，不再承担主路径。

用法:
  python3 scripts/worker_lane.py --pane w1:pN
  python3 scripts/worker_lane.py --lane w1-pN      # systemd 实例名转义形式
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
import traceback
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import dispatch_workers as dw        # noqa: E402  复用判定逻辑
import queue_lib as ql               # noqa: E402  并发安全队列

ROOT = dw.ROOT
POLL_SECONDS = float(os.environ.get("LANE_POLL_SECONDS", "3"))
# 输出多久不变化就算「停滞」。lane 原来只看状态栏（busy/idle/blocked），
# 工人在 busy 状态里反复自问自答、反复重试同一条命令、或者停下来等一个
# 永远等不到的回答时，状态栏一直显示 busy，只有 wall limit 超时才被发现——
# 而 wall limit 是 20 分钟起步。这里改成看**对话尾部有没有变化**。
STALL_WARN_SECONDS = float(os.environ.get("LANE_STALL_SECONDS", "240"))
# 刚判定 idle 时不一定真的写完了（TUI 落盘有一两秒延迟），给一个稳定窗口：
# 连续 N 次看到 idle 且 produced 才结算，避免把"写完但还没刷盘"判成没做完。
IDLE_STABLE_HITS = int(os.environ.get("LANE_IDLE_STABLE_HITS", "2"))
# 上下文水位保护：超过就重置会话再派，否则会撞上游 TPM 死锁（见 skill）
CONTEXT_LIMIT = float(os.environ.get("LANE_CONTEXT_LIMIT", "60"))
MAX_DEAD_PANE_WAIT = 120.0


def lane_name(pane: str) -> str:
    return pane.replace(":", "-")


# TUI 装饰行：状态栏（☤ AUTO · 10% · ◎ 36%）、提示行（❯ type to redirect…）、
# 纯分隔线（───）、只有 spinner 的行。这些每轮都在变或者完全没有信息量，
# 直接取"最后一行"会拿到状态栏，看起来像在监测、其实什么也没说。
_DECOR_MARKERS = ("☤", "type to redirect this turn", "Tab queues it")
_SPIN = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"


def _content_line(text: str, state: str = "", limit: int = 110) -> str:
    """从 pane 尾部取一行**能说明它在干什么**的话。

    TUI 底部是输入建议面板（"─── Help with question ───" / "❯ Explain this error…"），
    整块切掉；剩下的内容再按状态挑：

      - 非 busy 且有 `☤ Hermes` 汇报块 → 取汇报第一行（"p104 完成。新增：…"）
      - busy → 取最后一条 `💻 $ ...`（正在跑什么）
      - 都不满足 → 取最后一行像样的内容（跳过折行碎片和时间戳）
    """
    lines = (text or "").splitlines()
    cut = len(lines)
    for i, l in enumerate(lines):
        if "Help with question" in l:
            cut = i
            break
    body = lines[:cut]

    if state != "busy":
        for i, l in enumerate(body):
            if "☤ Hermes" in l:
                for nxt in body[i + 1:]:
                    t = nxt.strip()
                    if t and not set(t) <= set("─━═-_ ") and len(t) >= 6:
                        return t[:limit]
                break

    for raw in reversed(body[-14:]):
        if "💻 $" in raw:
            return raw.strip().strip("─━═-_· ").strip()[:limit]

    for raw in reversed(body):
        if "· edited " in raw or "· ran " in raw:
            continue
        line = raw.strip().strip("─━═-_· ").strip()
        if not line or len(line) < 6:            # 折行碎片没有信息量
            continue
        if any(m in line for m in _DECOR_MARKERS):
            continue
        if any(ch in line for ch in _SPIN):
            continue
        if line.startswith("❯"):
            continue
        if re.fullmatch(r"\d{1,2}:\d{2}(\s*[AP]M)?", line):   # 时间戳
            continue
        return line[:limit]
    return ""

def _content_signature(text: str) -> str:
    """对「去装饰后的内容」取指纹，用于判断输出是否真的在推进。

    不能对原始尾部取指纹：状态栏/进度 spinner 每帧都在变（☤ 10% → ☤ 11%，
    ⠦ → ⠧），指纹永远在变，停滞检测就永远不会触发——第一版就是这么写的，
    降到 20 秒阈值等了 75 秒一次都没报。
    """
    lines = (text or "").splitlines()
    cut = len(lines)
    for i, l in enumerate(lines):
        if "Help with question" in l:
            cut = i
            break
    keep = []
    for raw in lines[:cut]:
        line = raw.strip().strip("─━═-_· ").strip()
        if not line:
            continue
        if any(m in line for m in _DECOR_MARKERS):
            continue
        if any(ch in line for ch in _SPIN):
            continue
        keep.append(line)
    return "\n".join(keep[-12:])   # 只看最后 12 行内容


def lane_log(pane: str, msg: str) -> None:
    logs = ROOT / "logs"
    logs.mkdir(exist_ok=True)
    line = f"[{time.strftime('%H:%M:%S')}] {msg}\n"
    with (logs / f"lane-{lane_name(pane)}.log").open("a", encoding="utf-8") as f:
        f.write(line)


def write_lane_status(pane: str, **fields) -> None:
    """每 lane 一份状态文件 —— 单独调试用，不用去翻全局日志。"""
    ops = ROOT / "ops"
    ops.mkdir(exist_ok=True)
    path = ops / f"lane-{lane_name(pane)}.json"
    data = {"pane": pane, "updated": time.time(), "pid": os.getpid()}
    try:
        if path.exists():
            data.update(json.loads(path.read_text(encoding="utf-8")))
    except Exception:  # noqa: BLE001
        pass
    data.update(fields)
    data["updated"] = time.time()
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def deps_ok(task: dict, tasks: list[dict]) -> bool:
    return dw.deps_satisfied(task, tasks)


def eligible(task: dict, tasks: list[dict]) -> bool:
    """这个 lane 能不能接：只接 pending、满足重试退避、没超过重试上限。"""
    if task.get("status") != "pending":
        return False
    if not dw.retry_ready(task, dw.now()):
        return False
    if task.get("retries", 0) >= task.get("max_retries", 3):
        return False
    return True


def my_task(pane: str) -> dict | None:
    for t in ql.read_queue():
        if t.get("status") == "dispatched" and t.get("pane") == pane:
            return t
    return None


def settle_done(task: dict, pane: str) -> None:
    ql.settle_task(task["id"], "done", done_at=time.time(), verified=None)
    ql.push_verify(task["id"])       # 完成即入验证队列，不等定时器
    lane_log(pane, f"DONE  {task['id']}（已转入验证队列，立即派下一条）")


def release(task: dict, pane: str, reason: str) -> None:
    ql.settle_task(task["id"], "pending", pane=None, retry_after=time.time() + 30,
                   retries=(task.get("retries") or 0) + 1, fail_reason=reason)
    lane_log(pane, f"RELEASE {task['id']} {reason}")


def dispatch_next(pane: str) -> bool:
    """抢一条任务派给本 lane 的工人。成功返回 True。"""
    if dw.pane_state(pane) != "idle":
        return False

    pct = dw.context_pct(pane)
    if pct is not None and pct >= CONTEXT_LIMIT:
        lane_log(pane, f"RESET 上下文 {pct:.0f}% >= {CONTEXT_LIMIT:.0f}%，重置会话后再派")
        dw.reset_context(pane)
        return False

    task = ql.claim_task(pane, deps_ok, eligible)
    if not task:
        return False

    prompt = task.get("prompt") or ""
    ok = dw.dispatch(pane, prompt)
    if not ok:
        release(task, pane, "派发失败（herdr 未接受）")
        return False
    lane_log(pane, f"DISPATCH {task['id']}")
    write_lane_status(pane, task=task["id"], state="dispatched")
    return True


def run(pane: str) -> int:
    lane_log(pane, f"lane 启动 poll={POLL_SECONDS}s idle_stable={IDLE_STABLE_HITS}")
    write_lane_status(pane, state="starting")
    idle_hits = 0
    dead_since: float | None = None
    tail_sig: str | None = None      # 上一次看到的尾部指纹
    tail_since = time.time()         # 尾部最后一次变化的时间
    stall_warned = False
    last_task_id: str | None = None

    while True:
        try:
            live = dw.live_pane_ids()
            if live and pane not in live:
                if dead_since is None:
                    dead_since = time.time()
                    lane_log(pane, "WARN pane 不在 herdr 里，等它回来（不抢回任务）")
                elif time.time() - dead_since > MAX_DEAD_PANE_WAIT:
                    t = my_task(pane)
                    if t:
                        release(t, pane, f"pane {pane} 消失超过 {MAX_DEAD_PANE_WAIT:.0f}s")
                    dead_since = None
                time.sleep(POLL_SECONDS)
                continue
            dead_since = None

            task = my_task(pane)
            if task is not None and task["id"] != last_task_id:
                # 换了新任务：尾部基线重新算，否则会把上一条任务的静默时间
                # 算到新任务头上，一上来就报 STALL。
                last_task_id = task["id"]
                tail_sig = None
                tail_since = time.time()
                stall_warned = False
            if task is None:
                idle_hits = 0
                write_lane_status(pane, state="idle", task=None)
                dispatch_next(pane)
                time.sleep(POLL_SECONDS)
                continue

            # 读一次尾部，同时用于状态判定和停滞检测（少一次 herdr 调用）
            tail = dw.read_tail(pane, 20)
            state = dw.pane_state_from(tail)
            sig = _content_signature(tail)
            if sig != tail_sig:
                tail_sig = sig
                tail_since = time.time()
                stall_warned = False
            stall_sec = time.time() - tail_since
            elapsed_min = (time.time() - task.get("dispatched_at", time.time())) / 60

            if state == "idle":
                if dw.produced(task):
                    idle_hits += 1
                    # 连续 N 次都满足才结算：避免"文件刚出现、内容还没写完"
                    if idle_hits >= IDLE_STABLE_HITS:
                        settle_done(task, pane)
                        idle_hits = 0
                        write_lane_status(pane, state="idle", task=None)
                        continue          # 立刻回去抢下一条，不等任何定时器
                else:
                    idle_hits = 0
                    limit = dw.wall_limit_minutes(task)
                    if elapsed_min > limit:
                        release(task, pane, f"超时 {elapsed_min:.1f}min 无产出（上限 {limit:.0f}min）")
            elif state == "blocked":
                idle_hits = 0
                if elapsed_min > dw.wall_limit_minutes(task):
                    release(task, pane, "卡在上游限流/重试，超时释放")
            else:
                idle_hits = 0
                if elapsed_min > dw.wall_limit_minutes(task) * 1.5:
                    release(task, pane, f"运行 {elapsed_min:.1f}min 远超上限，强制释放")

            # 状态栏说 busy、但尾部几分钟没变 = 大概率卡住（在等一个等不到的答案、
            # 或在一个循环里反复重试）。只记日志不自动释放：误判的代价比等 wall
            # limit 更高，先让它可见，由人来判。
            # 两种要报的情况：
            #   1) 非 idle 且内容长时间不变 → 卡住（在等一个等不到的答案 / 循环重试）
            #   2) idle 但产出不达标        → 工人说完了，判据却认为没产出
            # 第 2 种最坑：lane 会一直等到 wall limit（20 分钟）才释放并返工，
            # 而真正的原因往往是 expect 路径写错了。实测 p103：prompt 允许实现到
            # server/，expect 只写了 system/，工人做对了照样判"没产出"。
            stalled = stall_sec >= STALL_WARN_SECONDS
            if stalled and not stall_warned:
                if state == "idle":
                    missing = [f for f in (task.get("expect") or [])
                               if not (ROOT / f).exists()]
                    lane_log(pane, f"IDLE_NO_OUTPUT {task['id']} 空闲 {stall_sec/60:.1f} 分钟"
                                   f"但产出判据不成立；缺失路径={missing or '（文件都在，内容门槛没过）'}")
                else:
                    lane_log(pane, f"STALL {task['id']} 尾部 {stall_sec/60:.1f} 分钟无变化："
                                   f"{_content_line(tail, state)}")
                stall_warned = True
            write_lane_status(pane, state=state, task=task["id"],
                              elapsed_min=round(elapsed_min, 1),
                              stall_sec=round(stall_sec, 1), stalled=stalled,
                              last_line=_content_line(tail, state))
            time.sleep(POLL_SECONDS)

        except KeyboardInterrupt:
            lane_log(pane, "lane 收到中断，退出")
            return 0
        except Exception:  # noqa: BLE001
            lane_log(pane, "ERR " + traceback.format_exc().replace("\n", " | ")[:400])
            time.sleep(POLL_SECONDS * 2)


def main() -> int:
    ap = argparse.ArgumentParser()
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--pane", help="完整 pane 名，如 w1:pN")
    g.add_argument("--lane", help="systemd 实例名转义形式，如 w1-pN")
    args = ap.parse_args()
    pane = args.pane
    if not pane and args.lane:
        # systemd 实例名不能带冒号，约定 lane 名把第一个 '-' 还原成 ':'
        lane = args.lane
        head, _, tail = lane.partition("-")
        pane = f"{head}:{tail}" if tail else lane
    return run(pane)


if __name__ == "__main__":
    sys.exit(main())
