#!/usr/bin/env python3
"""提交前拦截：不允许把「正在被工人改的文件」提交进去。

## 为什么

`git add -A` 在并行开发时会顺手卷进工人写到一半的文件。实际发生过两次：
一次是工人在改 `server.go`（提交后包编译不过），一次是把 p82 只写了 10 行的
`Users.vue` 卷进了一个讲「收下工人产出+回归报告」的 chore 提交——历史里就多出
一个语义对不上的半成品改动，事后没人能从那句提交信息猜到它是什么。

规则：`ops/tasks.jsonl` 里 status=dispatched 的任务，其 expect 列表中的文件
被 staged 时，拒绝提交并说清是哪个任务在改它。

确实要在工人干活期间提交同一个文件（比如修它引入的编译错误）时：
    ALLOW_INFLIGHT=1 git commit ...
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent  # .git/hooks/x → repo
QUEUE = ROOT / "ops" / "tasks.jsonl"


def staged_files() -> list[str]:
    out = subprocess.run(
        ["git", "diff", "--cached", "--name-only"],
        cwd=ROOT, capture_output=True, text=True, check=False,
    ).stdout
    return [line.strip() for line in out.splitlines() if line.strip()]


def inflight_claims() -> dict[str, str]:
    """返回 {文件: 任务id}，只算仍在 dispatched 的任务。"""
    claims: dict[str, str] = {}
    if not QUEUE.exists():
        return claims
    for line in QUEUE.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        try:
            task = json.loads(line)
        except json.JSONDecodeError:
            continue
        if task.get("status") != "dispatched":
            continue
        for path in task.get("expect") or []:
            claims[path] = task.get("id", "?")
    return claims


def main() -> int:
    if os.environ.get("ALLOW_INFLIGHT") == "1":
        return 0
    claims = inflight_claims()
    if not claims:
        return 0
    hits = [(f, claims[f]) for f in staged_files() if f in claims]
    if not hits:
        return 0
    print("✗ 拒绝提交：以下文件正被进行中的工人任务修改", file=sys.stderr)
    for f, tid in hits:
        print(f"    {f}  ←  {tid}", file=sys.stderr)
    print("\n  工人可能只写了一半。等它结算完再提交，或确认要提交时：", file=sys.stderr)
    print("    ALLOW_INFLIGHT=1 git commit ...", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
