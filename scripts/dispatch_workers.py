#!/usr/bin/env python3
"""工人调度器：队列驱动，工人一空就自动派下一个。

## 为什么需要它

原先 `watch_workers.py` 只做一件事：工人空闲超过 N 分钟就发通知。
问题在于通知的接收者是我（助手/人），我一旦在忙别的，工人就空转 ——
实测出现过 pQ 在 23:01 就交了 voucher，直到 23:10 才被派下一单，
中间 9 分钟纯浪费。瓶颈不在工人，在"谁来决定下一个做什么"。

这个脚本把那个决定搬进队列：

    ops/tasks.jsonl  （我往这里塞任务）
            │
            ▼
      dispatch_workers.py  ──每 60s──►  空闲的 pane 立刻领走一条
            │
            ├─ 记录 dispatched_at，防止重复派发
            └─ 下次运行检查 expect_files：文件出现了就标 done，并自动填下一条

于是我的工作从"盯着谁空了"变成"把活排进队列 + 审查产出"。

## 队列格式（每行一个 JSON）

    {"id": "p18-linkcheck", "prompt": "完整的任务描述...",
     "expect": ["mgmtd/internal/linkcheck/probe.go"],
     "prefer": "w1:pN"}        # prefer 可省，省略则任意空闲 pane

字段：
    id       唯一标识，重复 id 会被忽略
    prompt   派给 herdr agent prompt 的全文
    expect   产出文件（相对项目根），全部存在才算完成；可省
    prefer   指定 pane；被占用时可以让别的 pane 接

## 状态机

    pending ──派发──► dispatched ──产出出现──► done
                            │
                            └──空闲且超时仍无产出──► failed（可重新入队）

## 安全阀

- 一个 pane 同时只能有一个 dispatched 任务，绝不叠加
- pane_state 不是 idle 绝不派发（unknown 当作忙，避免打断）
- 派发命令失败 → 立刻退回 pending，不吞掉任务
- 每轮最多派 N 条（默认 4），避免一次性把队列灌空
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
QUEUE = ROOT / "ops" / "tasks.jsonl"
LOG = ROOT / "logs" / "dispatch.log"
STATE = ROOT / ".dispatch-state.json"

PANES = os.environ.get("PANES", "w1:pN w1:pQ w1:pP w1:pM").split()
MAX_PER_ROUND = int(os.environ.get("MAX_PER_ROUND", "4"))
STALE_MINUTES = float(os.environ.get("STALE_MINUTES", "20"))  # 派发后多久没产出算失败
# 工人已经 idle（它认为做完了）但产出不达标，多久之后释放它。
# 没有这个，会出现"任务卡在 WAIT 占着 pane，pane 又领不到新活"的死局：
# 实测 pQ 写了 97 行（要求 120）后认为自己完成，之后一直空转。
IDLE_STUCK_MINUTES = float(os.environ.get("IDLE_STUCK_MINUTES", "4"))
HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")

# 终端里的忙/闲标记。与 watch_workers.py 保持一致。
# 判忙闲的标记。这些字符串来自实测的终端尾部文本，不是猜的：
#
#   busy（真的在干活）
#     "esc interrupt"                 带 ☤ 前缀，等待可打断
#     "⏳ waiting on <model>"           上游慢，还在等第一个 chunk
#     "Working / Preparing / Thinking"
#     "☤ ❯ type to redirect this turn" 这一行在输出过程中出现
#
#   idle（可以派活）
#     "Type / to browse commands"      无历史时的空提示
#     "Ctrl+P for the palette"
#     "❯ Explain this error and how..." 有历史时的空提示（实测 pP 停在这里）
#
# 顺序很关键：先判 busy 再判 idle，因为忙的时候也可能出现 ❯ 提示行。
BUSY_MARKERS = [
    "esc interrupt",
    "⏳ waiting on",
    "waiting for the first stream chunk",
    "working", "preparing", "thinking",
    "type to redirect this turn",
]
IDLE_MARKERS = [
    "type / to browse commands",
    "ctrl+p commands",          # OpenCode 状态栏的空闲写法（实测 pM）
    "ctrl+p for the palette",
    "❯ explain this error",
    "❯ ask anything",
    "❯ type a message",
]


def now() -> float:
    return time.time()


def stamp(t: float | None = None) -> str:
    return time.strftime("%H:%M:%S", time.localtime(t if t is not None else now()))


def log(msg: str) -> None:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    with LOG.open("a", encoding="utf-8") as f:
        f.write(f"[{stamp()}] {msg}\n")
    print(msg)


# ---------- 队列 ----------

def load_queue() -> list[dict]:
    """读队列。坏行跳过并记录，不让一行脏数据卡死整个调度。"""
    if not QUEUE.exists():
        return []
    tasks = []
    for i, line in enumerate(QUEUE.read_text(encoding="utf-8").splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        try:
            tasks.append(json.loads(line))
        except json.JSONDecodeError as exc:
            log(f"WARN 队列第 {i} 行不是合法 JSON，已跳过: {exc}")
    return tasks


def save_queue(tasks: list[dict]) -> None:
    QUEUE.parent.mkdir(parents=True, exist_ok=True)
    tmp = QUEUE.with_suffix(".tmp")
    with tmp.open("w", encoding="utf-8") as f:
        for t in tasks:
            f.write(json.dumps(t, ensure_ascii=False) + "\n")
    tmp.replace(QUEUE)  # 原子替换，避免写到一半被读


def load_state() -> dict:
    if not STATE.exists():
        return {}
    try:
        return json.loads(STATE.read_text(encoding="utf-8"))
    except Exception:
        return {}


def save_state(st: dict) -> None:
    STATE.write_text(json.dumps(st, ensure_ascii=False, indent=2), encoding="utf-8")


# ---------- 工人状态 ----------

def pane_state(pane: str) -> str:
    """读 pane 尾部判忙闲。与 watch_workers.py 同一套标记。"""
    try:
        out = subprocess.run(
            [HERDR, "agent", "read", pane, "--lines", "8"],
            capture_output=True, text=True, timeout=15,
        ).stdout
    except Exception as exc:  # noqa: BLE001
        log(f"WARN herdr read {pane} 失败: {exc}")
        return "unknown"
    if not out.strip():
        return "unknown"
    # 大小写不敏感：同一个 TUI 在不同场景下会写 "Ctrl+P" / "ctrl+p"，
    # 区分大小写会让工人被误判成 unknown 从而永远领不到活。
    low = out.lower()
    if any(m.lower() in low for m in BUSY_MARKERS):
        return "busy"
    if any(m.lower() in low for m in IDLE_MARKERS):
        return "idle"
    return "unknown"


def dispatch(pane: str, prompt: str) -> bool:
    """派活。返回是否成功。命令本身失败必须如实返回，不能假装派了。"""
    try:
        r = subprocess.run(
            [HERDR, "agent", "prompt", pane, prompt],
            capture_output=True, text=True, timeout=60,
        )
    except Exception as exc:  # noqa: BLE001
        log(f"ERROR 派发到 {pane} 抛异常: {exc}")
        return False
    if r.returncode != 0:
        log(f"ERROR 派发到 {pane} rc={r.returncode}: {(r.stderr or r.stdout)[:200]}")
        return False
    return True


def produced(task: dict) -> bool:
    """产出验收。三道关，缺一不可。

    为什么要三道：最早只查"文件存在"，结果一个任务 1 分钟就被标成 DONE。
    文件确实存在，但那是工人写的空壳，或者写了一半。文件存在只能说明
    "动手了"，说明不了"做对了"。

    1. expect      —— 文件存在
    2. min_lines   —— {"path": 行数}，挡住空壳（一个 200 行的 handler 不可能 3 行写完）
    3. must_contain —— {path: [必须出现的字符串]}，比行数精确
    4. must_build  —— 跑一次 go build，挡住写不出来的代码
    """
    expect = task.get("expect") or []
    if not expect:
        return False
    if not all((ROOT / p).exists() for p in expect):
        return False

    for path, need in (task.get("min_lines") or {}).items():
        f = ROOT / path
        if not f.exists():
            return False
        try:
            got = sum(1 for _ in f.open(encoding="utf-8", errors="replace"))
        except OSError:
            return False
        if got < int(need):
            log(f"WAIT  {task['id']}: {path} 只有 {got} 行（要求 >= {need}）")
            return False

    # must_contain：文件里必须出现的字符串。比 min_lines 精确 ——
    # 行数只是"写了多少"的粗代理，实测出现过 97 行的完整实现被 120 行门槛
    # 挡住（p18b.go 三个 handler 全在、CSP 头也有）。真正该断言的是
    # "关键东西在不在"，不是"够不够长"。
    for path, needles in (task.get("must_contain") or {}).items():
        f = ROOT / path
        if not f.exists():
            return False
        try:
            text = f.read_text(encoding="utf-8", errors="replace")
        except OSError:
            return False
        for needle in needles:
            if needle not in text:
                log(f"WAIT  {task['id']}: {path} 缺少关键内容 {needle!r}")
                return False

    if task.get("must_build"):
        for workdir, cmd in (task.get("build_cmds") or [("mgmtd", ["go", "build", "./..."])]):
            try:
                r = subprocess.run(cmd, cwd=ROOT / workdir, capture_output=True, text=True, timeout=240)
            except subprocess.TimeoutExpired:
                log(f"WAIT  {task['id']}: 构建超时")
                return False
            if r.returncode != 0:
                log(f"WAIT  {task['id']}: 构建失败 {(r.stderr or '')[:160]}")
                return False

    return True


# ---------- 主流程 ----------

def main() -> int:
    tasks = load_queue()
    if not tasks:
        return 0

    state = load_state()
    t = now()

    # 1) 结算：已派发的任务，产出出现就标 done
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        if produced(task):
            task["status"] = "done"
            task["done_at"] = t
            log(f"DONE  {task['id']}  (pane={task.get('pane')}, 耗时 {int((t - task.get('dispatched_at', t)) / 60)} 分钟)")
            state.pop(task.get("pane", ""), None)

    # 2) 超时无产出 -> failed，交回队列让人看
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        age_min = (t - task.get("dispatched_at", t)) / 60
        if age_min > STALE_MINUTES and not produced(task):
            task["status"] = "failed"
            task["failed_at"] = t
            task["fail_reason"] = f"派发 {int(age_min)} 分钟仍无产出文件"
            log(f"FAIL  {task['id']}  {task['fail_reason']} (pane={task.get('pane')})")
            state.pop(task.get("pane", ""), None)

    # 2.5) 工人已经空闲但产出不达标 -> 释放 pane，任务标 failed
    #
    # 死局场景：工人写了 97 行（要求 120）后自己认为完成并回到 idle，
    # 调度器因为 min_lines 不通过而卡在 WAIT。此时任务仍占着 pane，
    # 该 pane 领不到新活 —— 工人和任务互相等待，纯浪费。
    # 判据：pane 已 idle + 距离派发已超过 IDLE_STUCK_MINUTES。
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        pane = task.get("pane", "")
        age_min = (t - task.get("dispatched_at", t)) / 60
        if age_min < IDLE_STUCK_MINUTES:
            continue
        if pane_state(pane) != "idle":
            continue
        task["status"] = "failed"
        task["failed_at"] = t
        task["fail_reason"] = (
            f"工人已空闲但产出不达标（{int(age_min)} 分钟）: "
            f"没通过 {'/'.join(task.get('min_lines') or {}) or '文件存在性'} 检查"
        )
        log(f"RELEASE {task['id']}  工人 {pane} 已空闲但产出不达标，释放 pane")
        state.pop(pane, None)

    # 3) 哪些 pane 手上还有活（dispatched 未结算）
    busy_panes = {task.get("pane") for task in tasks if task.get("status") == "dispatched"}

    # 4) 派发：空闲 pane 领 pending 任务
    dispatched_now = 0
    for pane in PANES:
        if dispatched_now >= MAX_PER_ROUND:
            break
        if pane in busy_panes:
            continue
        if pane_state(pane) != "idle":
            continue

        # 优先取指定了本 pane 的，其次取没指定 prefer 的
        cand = None
        for task in tasks:
            if task.get("status") != "pending":
                continue
            prefer = task.get("prefer")
            if prefer == pane:
                cand = task
                break
        if cand is None:
            for task in tasks:
                if task.get("status") == "pending" and not task.get("prefer"):
                    cand = task
                    break
        if cand is None:
            break  # 队列里没有能派的活了

        if dispatch(pane, cand["prompt"]):
            cand["status"] = "dispatched"
            cand["pane"] = pane
            cand["dispatched_at"] = t
            state[pane] = cand["id"]
            dispatched_now += 1
            log(f"DISPATCH {cand['id']} -> {pane}")
        else:
            # 派发失败：保持 pending，下一轮重试（不吞任务）
            log(f"RETRY {cand['id']} 派发失败，保持 pending")

    save_queue(tasks)
    save_state(state)

    # 汇总
    counts: dict[str, int] = {}
    for task in tasks:
        counts[task.get("status", "pending")] = counts.get(task.get("status", "pending"), 0) + 1
    summary = " ".join(f"{k}={v}" for k, v in sorted(counts.items()))
    if dispatched_now or counts.get("pending"):
        log(f"SUMMARY {summary} | 本轮派发 {dispatched_now}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
