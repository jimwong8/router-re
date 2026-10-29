#!/usr/bin/env python3
"""min_watch.py — 每分钟一轮的工人进度巡检 + 自动补货（oneshot，由 systemd timer 每 60s 触发）。

只做安全动作，不碰需要 judgment 的事：
- 读 pane_brief.collect()：三窗 state/task/stall/last_line，写一行日志（4 种闲置可区分）。
- 队列有 pending + 有 idle 窗：只记日志（lane 每 3s 自己会领，不代派，避免双重驱动）。
- backlog 有 ready + 在途 <3：跑 seed_tasks.py --apply 补货（flock 串行，rc=99 跳过）。
- dispatched 任务产出已齐但 lane 未结算（IDLE_NO_OUTPUT 形态）：只记 WARN，不自动改判据/不强制 done。
- lane 心跳死（hb_age>15s 两轮）：记 ERR，由 supervisor 自愈，不抢修。

非必要不退出非零：任何异常只记日志，timer 下一轮继续。
"""
from __future__ import annotations

import json
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pane_brief as pb  # noqa: E402
import queue_lib as ql  # noqa: E402  (同目录，lane/seed 同样直引)

ROOT = Path(__file__).resolve().parent.parent
OPS = ROOT / "ops"
LOG = ROOT / "logs" / "min-watch.log"
LOCK = OPS / ".min-watch.lock"
MAX_INFLIGHT = 3


def log(msg: str) -> None:
    try:
        ts = time.strftime("%m-%d %H:%M:%S")
        LOG.parent.mkdir(parents=True, exist_ok=True)
        with open(LOG, "a", encoding="utf-8") as f:
            f.write(f"[{ts}] {msg}\n")
    except OSError:
        print(msg, file=sys.stderr)


def main() -> int:
    try:
        d = pb.collect()
    except Exception as e:  # noqa: BLE001
        log(f"COLLECT_FAIL {e}")
        return 0
    try:
        tasks = ql.read_queue()
    except Exception as e:  # noqa: BLE001
        log(f"QUEUE_READ_FAIL {e}")
        return 0

    pending = [t for t in tasks if t.get("status") == "pending"]
    dispatched = [t for t in tasks if t.get("status") == "dispatched"]
    # 4 种闲置诊断：逐窗一行
    for r in d["panes"]:
        log(f"{r['pane']} label={r['label']} kind={r['kind']} task={r.get('task') or '-'} "
            f"stall={int(r.get('stall_sec') or 0)}s hb={r.get('hb_age_sec')}s "
            f"line={(r.get('last_line') or '')[:60]}")
    log(f"SUMMARY pending={len(pending)} dispatched={len(dispatched)} "
        f"verify_queue={d.get('verify_queue', 0)}")

    # backlog 有 ready 且在途不满：自动补货（flock 串行）
    try:
        bl_path = OPS / "backlog.jsonl"
        ready = 0
        if bl_path.exists():
            with bl_path.open(encoding="utf-8") as _bf:
                for line in _bf:
                    if not line.strip():
                        continue
                    try:
                        item = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    if item.get("status") == "ready":
                        ready += 1
        inflight = len(pending) + len(dispatched)
        if ready > 0 and inflight < MAX_INFLIGHT:
            p = subprocess.run(
                ["flock", "-n", "-E", "99", str(LOCK),
                 "/usr/bin/python3", str(ROOT / "scripts" / "seed_tasks.py"), "--apply"],
                capture_output=True, text=True, timeout=60, cwd=str(ROOT))
            if p.returncode == 99:
                log("SEED_SKIP locked by another run")
            elif p.returncode != 0:
                log(f"SEED_FAIL rc={p.returncode} {p.stderr.strip()[:200]}")
            else:
                out = p.stdout.strip().replace("\n", " | ")
                log(f"SEED_OK {out[:300]}")
        elif ready > 0:
            log(f"SEED_DEFER ready={ready} inflight={inflight} 已满")
    except Exception as e:  # noqa: BLE001
        log(f"SEED_EXC {e}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
