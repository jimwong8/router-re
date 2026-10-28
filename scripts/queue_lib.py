#!/usr/bin/env python3
"""queue_lib.py — 队列的并发安全读写，供多工人 lane 与验证器共用。

## 为什么必须加锁

`dispatch_workers.save_queue()` 用的是 `tmp.replace(QUEUE)`，单次写是原子的，
但**读-改-写整体不是**。一个 lane 一个进程并发起来就是：

  lane A 读 → lane B 读 → A 写(加上自己抢到的任务) → B 写(基于旧快照)
  ⇒ A 抢到的那条 dispatched 状态被 B 的旧快照覆盖，任务回到 pending，
    被第三个 lane 再抢一次 —— 同一个任务派给两个工人。

旧模型是"每 5 分钟一轮、单进程扫全表"，天然不会并发，所以这个坑以前不显。
改成 per-pane 常驻 lane 之后，必须先把这个前提补上。

## 锁的粒度

用一把队列锁（`ops/.tasks.lock`）包住整个「读 → 改 → 写」临界区。临界区里只做
JSON 读写，不做任何网络/子进程调用，持锁时间是毫秒级，四个 lane 不会互相卡。
验证器写 `verified` 字段也用同一把锁，否则它和 lane 的写会互相覆盖。

## 用法

    from queue_lib import read_queue, claim_task, settle_task

    t = claim_task("w1:pN")          # 原子：挑一条 pending 标成 dispatched 返回
    ...
    settle_task(t["id"], status="done")
"""
from __future__ import annotations

import contextlib
import fcntl
import json
import os
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
QUEUE = Path(os.environ.get("QUEUE_PATH", str(ROOT / "ops" / "tasks.jsonl")))
LOCK = Path(os.environ.get("QUEUE_LOCK", str(ROOT / "ops" / ".tasks.lock")))
VERIFY_QUEUE = Path(os.environ.get("VERIFY_QUEUE_PATH", str(ROOT / "ops" / "verify-queue.jsonl")))


@contextlib.contextmanager
def queue_lock(timeout: float = 30.0):
    """排他锁。拿不到就等；等超时直接抛，别静默跳过（跳过等于丢更新）。"""
    LOCK.parent.mkdir(parents=True, exist_ok=True)
    fh = LOCK.open("a+")
    deadline = time.time() + timeout
    while True:
        try:
            fcntl.flock(fh.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            break
        except BlockingIOError:
            if time.time() > deadline:
                fh.close()
                raise TimeoutError(f"队列锁 {LOCK} 等 {timeout}s 超时")
            time.sleep(0.05)
    try:
        yield
    finally:
        fcntl.flock(fh.fileno(), fcntl.LOCK_UN)
        fh.close()


def _read_unlocked() -> list[dict]:
    if not QUEUE.exists():
        return []
    tasks = []
    for line in QUEUE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        try:
            t = json.loads(line)
        except json.JSONDecodeError:
            continue
        t.setdefault("status", "pending")
        tasks.append(t)
    return tasks


def _write_unlocked(tasks: list[dict]) -> None:
    QUEUE.parent.mkdir(parents=True, exist_ok=True)
    tmp = QUEUE.with_suffix(".tmp")
    with tmp.open("w", encoding="utf-8") as f:
        for t in tasks:
            f.write(json.dumps(t, ensure_ascii=False) + "\n")
        f.flush()
        os.fsync(f.fileno())
    tmp.replace(QUEUE)


def read_queue() -> list[dict]:
    with queue_lock():
        return _read_unlocked()


def update_task(task_id: str, **fields) -> dict | None:
    """加锁改单条任务的字段，返回改后的任务（找不到返回 None）。"""
    with queue_lock():
        tasks = _read_unlocked()
        for t in tasks:
            if t.get("id") == task_id:
                t.update(fields)
                _write_unlocked(tasks)
                return t
        return None


def claim_task(pane: str, deps_ok, eligible_ok) -> dict | None:
    """原子抢一条 pending 任务标成 dispatched 给 pane。

    deps_ok / eligible_ok 是调用方传进来的判定函数（依赖是否满足、是否可重试），
    放在锁内求值 —— 判定与占坑必须是同一个临界区，否则两个 lane 会看到同一份
    "都满足" 的快照、抢到同一条。

    **一个 pane 手上只能有一条 dispatched**：抢之前先查这个 pane 有没有在跑的活，
    有就直接返回 None。没有这条保护，调用方一旦在循环里连抢（或两个 lane 误配到
    同一个 pane），单个工人就会被塞进一堆任务，而工人 TUI 里只有一个输入框，
    后派的会把先派的顶掉 —— 结果是任务状态显示 dispatched、实际没人做。
    """
    with queue_lock():
        tasks = _read_unlocked()
        for t in tasks:
            if t.get("status") == "dispatched" and t.get("pane") == pane:
                return None
        for t in tasks:
            if t.get("status") != "pending":
                continue
            if not eligible_ok(t, tasks):
                continue
            if not deps_ok(t, tasks):
                continue
            t["status"] = "dispatched"
            t["pane"] = pane
            t["dispatched_at"] = time.time()
            t["retry_after"] = None
            _write_unlocked(tasks)
            return dict(t)
        return None


def settle_task(task_id: str, status: str, **fields) -> dict | None:
    """结算一条任务（done / failed / pending 重派）。"""
    with queue_lock():
        tasks = _read_unlocked()
        for t in tasks:
            if t.get("id") == task_id:
                t["status"] = status
                t.update(fields)
                _write_unlocked(tasks)
                return t
        return None


# ---------- 验证队列：完成即入队，验证器常驻消费 ----------

def push_verify(task_id: str) -> None:
    """任务一完成就进验证队列。已经在队列里就不重复入队。"""
    with queue_lock():
        VERIFY_QUEUE.parent.mkdir(parents=True, exist_ok=True)
        existing = set()
        if VERIFY_QUEUE.exists():
            for line in VERIFY_QUEUE.read_text(encoding="utf-8").splitlines():
                line = line.strip()
                if not line:
                    continue
                try:
                    existing.add(json.loads(line).get("id"))
                except json.JSONDecodeError:
                    continue
        if task_id in existing:
            return
        with VERIFY_QUEUE.open("a", encoding="utf-8") as f:
            f.write(json.dumps({"id": task_id, "enqueued_at": time.time()},
                               ensure_ascii=False) + "\n")


def pop_verify() -> dict | None:
    """取一条待验证任务并从队列移除（原子）。"""
    with queue_lock():
        if not VERIFY_QUEUE.exists():
            return None
        lines = [ln for ln in VERIFY_QUEUE.read_text(encoding="utf-8").splitlines() if ln.strip()]
        if not lines:
            return None
        head, rest = lines[0], lines[1:]
        try:
            item = json.loads(head)
        except json.JSONDecodeError:
            item = None
        tmp = VERIFY_QUEUE.with_suffix(".tmp")
        with tmp.open("w", encoding="utf-8") as f:
            for ln in rest:
                f.write(ln + "\n")
        tmp.replace(VERIFY_QUEUE)
        return item


def verify_queue_len() -> int:
    with queue_lock():
        if not VERIFY_QUEUE.exists():
            return 0
        return sum(1 for ln in VERIFY_QUEUE.read_text(encoding="utf-8").splitlines() if ln.strip())
