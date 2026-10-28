#!/usr/bin/env python3
"""test_queue_lib.py — 验证队列的并发抢占不会重复派活。

## 这是整个 lane 改造的正确性前提

旧模型是"单进程每 5 分钟扫全表"，天然没有并发问题。改成 4 个 lane 并发之后，
如果抢任务不是原子操作，就会出现：同一个任务被派给两个工人 → 两人各写一份
产出 → 互相覆盖 → 验证通过但内容是混的。

`queue_lib.claim_task` 用一把 flock 包住「读 → 判定 → 标 dispatched → 写」，
本测试用多进程同时抢，断言每条任务只被一个进程拿到。

跑法: python3 scripts/test_queue_lib.py
"""
from __future__ import annotations

import json
import multiprocessing as mp
import os
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))


def _worker(qpath: str, pane: str, out: str) -> None:
    """子进程：模拟真实 lane —— 抢一条 → 干活(settle done) → 再抢，直到抢不到。"""
    os.environ["QUEUE_PATH"] = qpath
    os.environ["QUEUE_LOCK"] = str(Path(qpath).with_suffix(".lock"))
    # 必须在设置环境变量之后再 import，模块级常量在 import 时读取
    import importlib
    import queue_lib
    importlib.reload(queue_lib)

    got = []
    while True:
        t = queue_lib.claim_task(pane, lambda tk, all_: True, lambda tk, all_: True)
        if not t:
            break
        got.append(t["id"])
        # 模拟"工人做完了"：结算成 done 才能接下一个（一个 pane 同时只持有一条）
        queue_lib.settle_task(t["id"], "done")
    Path(out).write_text(json.dumps(got), encoding="utf-8")


def main() -> int:
    tmpdir = Path(tempfile.mkdtemp(prefix="qlock-"))
    qpath = tmpdir / "tasks.jsonl"
    n_tasks = 40
    with qpath.open("w", encoding="utf-8") as f:
        for i in range(n_tasks):
            f.write(json.dumps({"id": f"t{i:03d}", "status": "pending",
                                "prompt": "x"}, ensure_ascii=False) + "\n")

    n_lanes = 6
    outs = [str(tmpdir / f"out{lane}.json") for lane in range(n_lanes)]
    procs = [mp.Process(target=_worker, args=(str(qpath), f"w1:p{lane}", outs[lane]))
             for lane in range(n_lanes)]
    for p in procs:
        p.start()
    for p in procs:
        p.join(timeout=60)

    claimed: list[str] = []
    for o in outs:
        if Path(o).exists():
            claimed += json.loads(Path(o).read_text(encoding="utf-8"))

    failures = []
    if len(claimed) != len(set(claimed)):
        dupes = {x for x in claimed if claimed.count(x) > 1}
        failures.append(f"重复派发 {sorted(dupes)}")
    if len(set(claimed)) != n_tasks:
        missing = {f"t{i:03d}" for i in range(n_tasks)} - set(claimed)
        failures.append(f"漏派 {sorted(missing)}（抢到 {len(set(claimed))}/{n_tasks}）")

    # 再确认队列里全部被结算成 done（每条都被某个 lane 做过一次）
    final = [json.loads(l) for l in qpath.read_text(encoding="utf-8").splitlines() if l.strip()]
    not_done = [t["id"] for t in final if t.get("status") != "done"]
    if not_done:
        failures.append(f"没被消费 {not_done[:5]}")

    if failures:
        print("FAIL " + "；".join(failures))
        return 1

    per_lane: dict[str, int] = {}
    for t in final:
        per_lane[t["pane"]] = per_lane.get(t["pane"], 0) + 1
    print(f"PASS {n_lanes} 进程并发抢 {n_tasks} 条任务：无重复、无遗漏，"
          f"分布 {dict(sorted(per_lane.items()))}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
