#!/usr/bin/env python3
"""seed_tasks.py — 从 backlog 补货到工人队列。

## 为什么需要它

worker_lane 只消费 `ops/tasks.jsonl`：队列空了，三个窗口就只能闲置。
之前那 101 条任务全是主模型（我）在对话里手写的——我一按需运行，流水线
就断粮。这个脚本把「写任务」变成一个可调度的动作：backlog 里有活就补，
补到在途数量够为止。

## 它不生成任务内容

prompt / expect / must_contain 全部来自 `ops/backlog.jsonl`（人工维护）。
自动"想"任务是负价值的：任务描述一错，工人做对也被判没产出（p103/p104
白等 11~21 分钟就是这么来的）。

## 三道守门

1. **不重复**：同 id 的任务已存在（任何状态）就跳过
2. **不超量**：在途（pending + dispatched）达到上限就停，避免一次倒 50 条
3. **不空转**：must_contain 的关键词若已出现在目标文件里，说明功能已经做了，
   直接标记 backlog 为 done，不派任务

用法：
    python3 scripts/seed_tasks.py            # 干跑，只打印决定
    python3 scripts/seed_tasks.py --apply    # 真的写入队列
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import queue_lib as ql  # noqa: E402

ROOT = ql.ROOT
BACKLOG = ROOT / "ops/backlog.jsonl"
MAX_INFLIGHT = int(os.environ.get("SEED_MAX_INFLIGHT", "3"))

HEADER = """【全新任务，放下旧的一切】项目根 /home/jimwong/projects/router-re，mgmtd Go 后端在 mgmtd/（internal/*），mgmtd 正跑在本机 127.0.0.1:9090（root 起），admin/admin123 登录拿 Bearer token。只做【真实实现】不是测试：在 mgmtd/internal/ 里实现新功能 + 在 mgmtd/internal/server/server.go 注册路由 + 补单测。约束：不 git commit；CGO_ENABLED=1 go build ./... 必须 rc=0；错误码从 10480 起顺序用（查已有最大号后接续）；命令参数一律 slice 不拼 shell 字符串。任务小而独立，25 分钟内完成。"""


def load_backlog() -> list[dict]:
    if not BACKLOG.exists():
        return []
    return [json.loads(l) for l in BACKLOG.read_text(encoding="utf-8").splitlines() if l.strip()]


def save_backlog(items: list[dict]) -> None:
    BACKLOG.write_text("\n".join(json.dumps(i, ensure_ascii=False) for i in items) + "\n",
                       encoding="utf-8")


def already_done(item: dict) -> str | None:
    """功能是否已经在代码里了？返回证据字符串，否则 None。

    两类任务永不跳过（存在≠完成）：
    - rerunnable: 回归/重跑类（报告存在恰恰是要重跑的信号）。
    - needs_commit: 交付物需入库（文件存在但 git status 还能看到它=没入库）。"""
    if item.get("rerunnable") or item.get("needs_commit"):
        return None
    for path, needles in (item.get("must_contain") or {}).items():
        f = ROOT / path
        if not f.exists():
            continue
        text = f.read_text(encoding="utf-8", errors="replace")
        present = [n for n in needles if n in text]
        if present and len(present) == len(needles):
            return f"{path} 已含 {present}"
    return None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--apply", action="store_true", help="真的写入队列")
    args = ap.parse_args()

    items = load_backlog()
    if not items:
        print("backlog 为空")
        return 0

    tasks = ql.read_queue()

    # 清扫「重试用尽却仍是 pending」的僵死任务。
    # claim_task 会把 retries>=max 的任务滤掉，于是一条耗尽重试的任务既不会被派、
    # 也不会被标 giveup——它就静静躺在 pending 里，还占着「在途」名额，让补货
    # 误判没空位。实测 p108-frp 就是这样卡住的。
    exhausted = [t for t in tasks
                 if t.get("status") == "pending"
                 and (t.get("retries") or 0) >= t.get("max_retries", 3)]
    for t in exhausted:
        # 只按重试次数判死会误杀：工人最后一次可能真的把活干完了，只是验证
        # 环节因为别的原因失败（编译中间态、验证者抽风）。先看产出成立不成立。
        import dispatch_workers as _dw
        try:
            done = _dw.produced(t)
        except Exception as e:  # noqa: BLE001
            done = False
            print(f"  {t['id']} 产出检查异常：{e}")
        if done:
            print(f"判定已完成 {t['id']}：产出成立，转入验证（而不是 giveup）")
            if args.apply:
                ql.settle_task(t["id"], "done", pane=None, verified=None)
                ql.push_verify(t["id"])
        else:
            print(f"标记 giveup {t['id']}：重试 {(t.get('retries') or 0)} 次且产出不成立")
            if args.apply:
                ql.settle_task(t["id"], "giveup",
                               fail_reason=t.get("fail_reason") or "retries exhausted")
    if exhausted:
        tasks = ql.read_queue()

    existing = {t["id"] for t in tasks}
    inflight = sum(1 for t in tasks if t.get("status") in ("pending", "dispatched"))
    room = MAX_INFLIGHT - inflight

    print(f"在途 {inflight}/{MAX_INFLIGHT}，可补 {max(room, 0)} 条")
    changed = False
    for item in items:
        if room <= 0:
            break
        if item.get("status") != "ready":
            continue
        if item["id"] in existing:
            continue

        done = already_done(item)
        if done:
            item["status"] = "done"
            item["done_reason"] = done
            changed = True
            print(f"跳过 {item['id']}：功能已存在（{done}）")
            continue

        task = {
            "id": item["id"],
            "status": "pending",
            "pane": None,
            "prompt": f"{HEADER}任务 {item['id']}：{item['prompt_head']}\n\n"
                      f"背景：{item.get('why', '')}\n"
                      f"路由注册必须落在 mgmtd/internal/server/server.go（验收要能看到）。",
            "expect": item.get("expect") or [],
            "must_contain": item.get("must_contain") or {},
            "created_at": time.time(),
            "source": "backlog",
        }
        print(f"补货 {item['id']}：{item.get('title')}")
        if args.apply:
            with ql.queue_lock():
                cur = ql._read_unlocked()
                if not any(t["id"] == task["id"] for t in cur):
                    cur.append(task)
                    ql._write_unlocked(cur)
        item["status"] = "seeded"
        item["task_id"] = item["id"]
        existing.add(item["id"])
        changed = True
        room -= 1

    if changed and args.apply:
        save_backlog(items)
    if not args.apply:
        print("（干跑，未写入；加 --apply 生效）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
