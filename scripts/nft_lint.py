#!/usr/bin/env python3
"""nftables 代码的静态危险模式扫描。

这个项目里同一个错误已经重复出现过多次，每次都是生产事故级别的：
- `flush ruleset` 出现过 3 次（worker 各自独立写的）—— 它会清空**整机所有** nft 表，
  包括 docker、tailscale、xrdp 的表，等于把机器网络打瘫。
- `policy drop` 出现过 1 次（attackguard）—— 只放行 SYN/ICMP，SSH 和管理界面全断。
- `elements = {}` 是 nft 语法错误（空集合非法），下发时整条规则集被拒。
- inet 表里裸写 `dnat to` 会被 nft 拒绝，必须 `dnat ip to` / `dnat ip6 to`。

靠 code review 记不住这些，所以做成门禁扫描。

用法：python3 scripts/nft_lint.py [--verbose]
退出码：0 干净；1 发现危险模式。
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GO_DIR = ROOT / "mgmtd"

# 每条：(正则, 严重级别, 说明)
RULES: list[tuple[re.Pattern[str], str, str]] = [
    (
        re.compile(r"flush\s+ruleset", re.I),
        "FATAL",
        "flush ruleset 会清空整机所有 nft 表（含 docker/tailscale/xrdp），等于网络瘫痪",
    ),
    (
        re.compile(r"policy\s+drop", re.I),
        "FATAL",
        "自建表的 chain 用 policy drop 会切断 SSH/管理界面/DNS；默认拒绝属于 firewall 模块",
    ),
    (
        re.compile(r"elements\s*=\s*\{\s*\}", re.I),
        "ERROR",
        "空集合字面量 elements = {} 是 nft 语法错误，整条规则集下发失败",
    ),
]


def scan() -> list[tuple[Path, int, str, str, str]]:
    findings: list[tuple[Path, int, str, str, str]] = []
    for f in GO_DIR.rglob("*.go"):
        if "_test.go" in f.name:
            # 测试文件里出现这些词是正常的（断言"不得包含 flush ruleset"）
            continue
        try:
            lines = f.read_text(encoding="utf-8", errors="ignore").splitlines()
        except OSError:
            continue
        for i, line in enumerate(lines, 1):
            stripped = line.lstrip()
            if stripped.startswith("//"):
                continue  # 注释里提到这些词是说明，不是代码
            for pat, level, why in RULES:
                if pat.search(line):
                    findings.append((f.relative_to(ROOT), i, level, pat.pattern, why))
    return findings


def check_dnat(text: str) -> list[tuple[Path, int, str, str, str]]:
    """inet 表里的裸 dnat to 会被 nft 拒绝，必须带族别。

    这条只在同一个文件里同时出现 `table inet` 和裸 `dnat to` 时才判定 ——
    ip/ip6 表里裸写是合法的。
    """
    out: list[tuple[Path, int, str, str, str]] = []
    for f in GO_DIR.rglob("*.go"):
        if "_test.go" in f.name:
            continue
        try:
            content = f.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        if "table inet" not in content and "inet router" not in content:
            continue
        for i, line in enumerate(content.splitlines(), 1):
            if line.lstrip().startswith("//"):
                continue
            if re.search(r"dnat\s+to\s", line) and not re.search(r"dnat\s+ip6?\s+to\s", line):
                out.append(
                    (
                        f.relative_to(ROOT),
                        i,
                        "ERROR",
                        "dnat to（缺族别）",
                        "inet 表必须写 dnat ip to / dnat ip6 to，否则 nft 拒绝："
                        "specify 'dnat ip' or 'dnat ip6' to disambiguate",
                    )
                )
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--verbose", action="store_true")
    args = ap.parse_args()

    findings = scan() + check_dnat("")
    if not findings:
        files = len(list(GO_DIR.rglob("*.go")))
        print(f"✅ nft 危险模式扫描干净（{files} 个 .go 文件）")
        return 0

    print(f"❌ 发现 {len(findings)} 处 nft 危险模式：")
    for path, line, level, pat, why in findings:
        print(f"   [{level}] {path}:{line}")
        print(f"          匹配: {pat}")
        print(f"          原因: {why}")
    return 1


if __name__ == "__main__":
    sys.exit(main())
