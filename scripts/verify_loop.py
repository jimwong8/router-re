#!/usr/bin/env python3
"""verify_loop.py — 任务完成即验证（常驻，不再每 15 分钟批量扫）。

## 为什么换

旧模型是 timer 每 15 分钟扫一遍「所有 done 但 verified 为空」的任务。问题：
- 一条任务做完要等最多 15 分钟才开始审，审完失败再回贴给工人（工人那时早闲了）
- 批量审意味着一次要塞给网关 N 个审查请求，正好撞上游 TPM 限流，集体 SKIP

现在 lane 结算任务时 `push_verify(task_id)` 写进 `ops/verify-queue.jsonl`，
本进程常驻消费：取一条 → 立刻审 → 出队。并发默认 2（再多会撞上游限流，
实测审查 prompt 1.5K token 以上就容易被 429）。

`verified` 的写回仍走 queue_lib 的同一把锁，避免和 lane 的写互相覆盖。

用法:
  python3 scripts/verify_loop.py                 # 常驻
  python3 scripts/verify_loop.py --once          # 量化：处理完当前队列就退出（测试/CI 用）
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import queue_lib as ql  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
CONCURRENCY = int(os.environ.get("VERIFY_CONCURRENCY", "2"))
IDLE_SLEEP = float(os.environ.get("VERIFY_IDLE_SLEEP", "3"))
PANES = os.environ.get("PANES", "w1:pN w1:pQ w1:pP w1:pM")


def log(msg: str) -> None:
    logs = ROOT / "logs"
    logs.mkdir(exist_ok=True)
    with (logs / "verify-loop.log").open("a", encoding="utf-8") as f:
        f.write(f"[{time.strftime('%H:%M:%S')}] {msg}\n")


def verify_one(task_id: str) -> tuple[str, int, str]:
    """单独起 verify_outputs.py 审一条。用子进程而不是 import：它已经是
    可独立运行的 CLI，且内部的网关重试/候选退让逻辑都在那里，别复制。"""
    env = dict(os.environ)
    env["PANES"] = PANES
    try:
        r = subprocess.run(
            [sys.executable, str(ROOT / "scripts" / "verify_outputs.py"), "--id", task_id],
            cwd=str(ROOT), capture_output=True, text=True, timeout=600, env=env,
        )
        tail = (r.stdout or r.stderr or "").strip().splitlines()[-3:]
        return task_id, r.returncode, " | ".join(tail)[:300]
    except subprocess.TimeoutExpired:
        return task_id, 124, "验证超时 600s"


def reconcile_missing(age_seconds: float = 60.0) -> int:
    """把「done 但没验证、也不在验证队列里」的任务捡回来。

    为什么需要：旧路径（dispatch_workers watchdog）标 done 时不会 push_verify，
    实测 p67 在 21:39 被 watchdog 误跑那一轮标成 done，之后就一直躺在
    status=done / verified=None，没人审也没人发现。lane 是主路径之后这种任务
    只可能来自兜底路径或历史遗留，所以定时捞一次最省事。
    """
    queued = set()
    if ql.VERIFY_QUEUE.exists():
        for line in ql.VERIFY_QUEUE.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                queued.add(json.loads(line).get("id"))
            except json.JSONDecodeError:
                continue

    now = time.time()
    picked = 0
    for t in ql.read_queue():
        if t.get("status") != "done" or t.get("verified"):
            continue
        if t["id"] in queued:
            continue
        if now - (t.get("done_at") or 0) < age_seconds:
            continue        # 刚完成，lane 可能正在 push，别抢
        ql.push_verify(t["id"])
        log(f"RECONCILE {t['id']} done 但未验证，补进验证队列")
        picked += 1
    return picked


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--once", action="store_true", help="处理完当前队列就退出")
    args = ap.parse_args()

    log(f"verify-loop 启动 concurrency={CONCURRENCY} panes={PANES}")
    inflight: set = set()
    done_count = 0

    with ThreadPoolExecutor(max_workers=CONCURRENCY) as pool:
        while True:
            # 先补满并发槽
            while len(inflight) < CONCURRENCY:
                item = ql.pop_verify()
                if not item:
                    break
                tid = item.get("id")
                if not tid:
                    continue
                log(f"VERIFY {tid}")
                inflight.add(pool.submit(verify_one, tid))

            if not inflight:
                if args.once:
                    log("队列已空，--once 退出")
                    return 0
                # 空闲时顺手把孤儿任务（done 但没验证）捡回队列
                if reconcile_missing():
                    continue
                time.sleep(IDLE_SLEEP)
                continue

            finished = [f for f in inflight if f.done()]
            if not finished:
                time.sleep(0.5)
                continue
            for f in finished:
                inflight.discard(f)
                tid, rc, tail = f.result()
                done_count += 1
                log(f"DONE {tid} rc={rc} {tail}")

            if args.once and ql.verify_queue_len() == 0 and not inflight:
                log(f"--once 完成，处理 {done_count} 条")
                return 0


if __name__ == "__main__":
    sys.exit(main())
