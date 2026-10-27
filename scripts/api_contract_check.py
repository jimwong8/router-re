#!/usr/bin/env python3
"""前端调用路径 vs 后端注册路径的契约检查。

为什么需要这个：前端 200 不代表功能可用。之前 `/vpn/tunnels` 就是这么漏掉的
——前端调用了一个后端根本没注册的路径，页面永远空白，但所有测试都是绿的
（因为测试只覆盖后端已注册的端点）。

检查两件事：
1. 前端 `api.get/post/put/delete('...')` 里的路径，后端有没有对应注册；
2. 后端注册了但前端从没调用过的路径（只提示，不算失败 —— 有些是给外部用的）。

用法：python3 scripts/api_contract_check.py [--verbose]
退出码：0 无缺失；1 有前端调用但后端缺失。
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
VIEWS = ROOT / "web" / "src"
SERVER = ROOT / "mgmtd" / "internal" / "server" / "server.go"

# 前端：api.get('/api/v1/xxx') / api.post(`/api/v1/xxx/${id}`)
CALL_RE = re.compile(r"api\.(get|post|put|delete|patch)\(\s*[`'\"]([^`'\"]+)[`'\"]", re.I)
# 后端：mux.HandleFunc("/api/v1/xxx", ...) / r.HandleFunc(...)
REG_RE = re.compile(r'HandleFunc\(\s*"([^"]+)"')


def normalize(path: str) -> str:
    """把动态段归一化，让 /users/abc 与 /users/{username} 能对上。

    前端模板串 `/api/v1/users/${encodeURIComponent(x)}` 归一成 `/api/v1/users/*`；
    后端 `/api/v1/users/{username}` 也归一成 `/api/v1/users/*`。
    """
    path = path.split("?")[0].rstrip("/")
    path = re.sub(r"\$\{[^}]*\}", "*", path)   # 前端模板变量
    path = re.sub(r"\{[^}]*\}", "*", path)     # Go 路由变量
    return path or "/"


def collect_calls() -> dict[str, list[str]]:
    """返回 {归一化路径: [出现位置, ...]}"""
    calls: dict[str, list[str]] = {}
    for f in VIEWS.rglob("*"):
        if f.suffix not in {".vue", ".ts", ".js"}:
            continue
        try:
            text = f.read_text(encoding="utf-8")
        except OSError:
            continue
        for m in CALL_RE.finditer(text):
            raw = m.group(2)
            if not raw.startswith("/api/"):
                continue
            calls.setdefault(normalize(raw), []).append(f"{f.relative_to(ROOT)}")
    return calls


# 后端的写法是 HandleFunc("GET /api/v1/xxx", ...) —— 方法和路径在同一个字符串里，
# 必须先剥掉方法前缀，否则归一化结果带 "GET " 前缀，永远对不上前端的纯路径。
METHOD_RE = re.compile(r"^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s+", re.I)


def collect_registered() -> set[str]:
    if not SERVER.exists():
        sys.exit(f"找不到 {SERVER}")
    text = SERVER.read_text(encoding="utf-8")
    out = set()
    for m in REG_RE.finditer(text):
        raw = METHOD_RE.sub("", m.group(1).strip())
        out.add(normalize(raw))
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--verbose", action="store_true", help="同时列出后端有但前端没调用的路径")
    args = ap.parse_args()

    calls = collect_calls()
    registered = collect_registered()

    # 前端带动态段、后端是不带段的具体路径（或反之）时，做一次宽松匹配
    missing = []
    for path, where in sorted(calls.items()):
        if path in registered:
            continue
        # /api/v1/users/* 可能对应后端的 /api/v1/users（列表）或 /api/v1/users/{x}
        loose = path.rstrip("/*")
        if any(r == loose or r.startswith(loose + "/") or r.rstrip("/*") == loose for r in registered):
            continue
        missing.append((path, sorted(set(where))))

    print(f"前端调用路径 {len(calls)} 个 / 后端注册 {len(registered)} 个")
    print()

    if missing:
        print(f"❌ 前端调用但后端未注册：{len(missing)} 个")
        for path, where in missing:
            print(f"   {path}")
            print(f"      调用处: {', '.join(where[:3])}")
        print()
    else:
        print("✅ 所有前端调用都有后端对应路径")

    if args.verbose:
        unused = sorted(registered - set(calls.keys()))
        if unused:
            print()
            print(f"ℹ️  后端注册但前端未调用（{len(unused)} 个，可能是给外部/脚本用的）：")
            for p in unused[:40]:
                print(f"   {p}")

    return 1 if missing else 0


if __name__ == "__main__":
    sys.exit(main())
