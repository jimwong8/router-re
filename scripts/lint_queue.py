#!/usr/bin/env python3
"""lint_queue.py — 派发前检查任务队列里"注定无法收敛"的条目。

## 为什么需要它

调度器判定"干完了"的唯一依据是 `produced()`：expect 里的文件存在，或者
verify_cmd 跑通。两者都没有的任务，工人干得再对也永远不会被标 done，
只能等 max_wall 超时 → RELEASE 回 pending → 再派一次 → 再超时……无限循环，
白烧工人时间还看不出异常（日志里只有重复的 DISPATCH/RELEASE）。

实测中招：p45-acct-readback-web、p46-security-arp-map-fix 都是 expect=[]，
排队时看不出来，跑起来才发现收敛不了。p44 用了 `.p44-done` 标记文件绕过去，
说明作者其实知道有这个问题、只是没有统一约定。

## 检查项

1. `expect` 和 `verify_cmd` 至少有一个（否则永远无法结算）
2. expect 里的路径不出项目根（`..` / 绝对路径都算错）
3. `min_lines` / `must_contain` 的 key 必须是 expect 里的路径
   （key 写错 → 门槛静默失效，等于没有门槛）
4. prompt 里提到的"只改 X / 只写 X"类路径应当出现在 expect 里
5. verify_cmd 里不该出现 `|| true`（会把失败吞成成功）

用法:
  python3 scripts/lint_queue.py            # 检查未完成任务，有问题 rc=1
  python3 scripts/lint_queue.py --all      # 连已完成的一起查
"""
from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
QUEUE = ROOT / "ops" / "tasks.jsonl"

# prompt 里"只改 xxx" / "只写 xxx" 的路径：这种被点名的文件必然该在 expect 里
NAMED_PATH_RE = re.compile(
    r"(?:只改|只写|只新增|新增|产出|写到)\s*([A-Za-z0-9_./-]+\.(?:go|py|vue|ts|js|md|json|sh))"
)


def load() -> list[dict]:
    if not QUEUE.exists():
        return []
    out = []
    for line in QUEUE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def lint_task(t: dict) -> list[str]:
    problems: list[str] = []
    tid = t.get("id", "?")
    expect = t.get("expect") or []
    vcmd = t.get("verify_cmd") or ""

    # 1) 可结算性
    if not expect and not vcmd:
        problems.append(f"{tid}: expect 和 verify_cmd 都没有 —— 永远不会被标 done，"
                        f"会超时→重派无限循环。补 expect 或 verify_cmd")

    # 2) 路径合法性
    for p in expect:
        if p.startswith("/") or ".." in Path(p).parts:
            problems.append(f"{tid}: expect 路径 `{p}` 必须是项目根下的相对路径")

    # 3) 门槛 key 必须命中 expect
    for field in ("min_lines", "must_contain"):
        d = t.get(field) or {}
        if isinstance(d, dict):
            for k in d:
                if expect and k not in expect:
                    problems.append(f"{tid}: {field} 的 key `{k}` 不在 expect 里 —— 门槛对产出不生效")

    # 4) prompt 点名的文件应当出现在 expect
    prompt = t.get("prompt") or ""
    for m in NAMED_PATH_RE.finditer(prompt):
        path = m.group(1).rstrip(".,;，。；")
        # 只对"本项目内的相对路径"提要求，忽略 /api/v1 这类接口路径
        if path.startswith("api/") or "/api/" in path or path.startswith("v1/"):
            continue
        if path.startswith(("web/", "mgmtd/", "scripts/", "docs/")):
            if expect and path not in expect:
                problems.append(f"{tid}: prompt 点名要改 `{path}`，但 expect 里没有 —— "
                                f"工人改了也不算数")

    # 5) verify_cmd 吞错
    if vcmd and re.search(r"\|\|\s*(?:true|:)\s*$", vcmd.strip()):
        problems.append(f"{tid}: verify_cmd 里的 `|| true` 会把失败吞成成功")

    # 6) 门槛已经成立 = 门槛无效
    #
    # must_contain 的用途是挡空壳：断言"改动之后才该出现的东西"。如果这个字符串
    # 在文件里**本来就有**，那么工人什么都没做也会被判 produced=True。实测栽过：
    # p64 要求 LogCenter.vue 含 "has_more"，而该文件本来就有这个词，watchdog 扫
    # 一轮就把正在写的任务判成了 done。
    #
    # 只对 pending（还没派发）的任务做这个判断：一旦派发出去，工人就开始改那个
    # 文件，任何门槛都会立刻"本来就有"，再报就是噪声。
    if t.get("status", "pending") == "pending":
        for path, needles in (t.get("must_contain") or {}).items():
            f = ROOT / path
            if not f.exists():
                continue          # 文件还不存在：说明是新产出，门槛天然有效
            try:
                text = f.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            for needle in needles:
                if needle in text:
                    problems.append(
                        f"{tid}: must_contain 的 {needle!r} 在 {path} 里**本来就有** —— "
                        f"挡不住空壳（工人不做也算达标），换一个改动后才出现的独特字符串")

    return problems


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--all", action="store_true", help="连已完成的任务一起检查")
    args = ap.parse_args()

    tasks = load()
    if not tasks:
        print("队列为空")
        return 0

    targets = tasks if args.all else [t for t in tasks if t.get("status") != "done"]
    problems: list[str] = []
    for t in targets:
        problems += lint_task(t)

    if problems:
        print(f"发现 {len(problems)} 个问题（检查了 {len(targets)} 个任务）：")
        for p in problems:
            print("  ✗", p)
        print("\n修法：直接改 ops/tasks.jsonl 里对应任务的 expect / verify_cmd。")
        return 1

    print(f"队列干净：{len(targets)} 个任务全部可结算")
    return 0


if __name__ == "__main__":
    sys.exit(main())
