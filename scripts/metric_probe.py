#!/usr/bin/env python3
"""metric_probe.py - probe read-only GET endpoints for latency >5s."""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
import concurrent.futures
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "metric-probe.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
SERVER_GO = ROOT / "mgmtd" / "internal" / "server" / "server.go"

results = []


def add(name, status, detail="", snippet=""):
    results.append({"name": name, "status": status, "detail": detail, "snippet": snippet})
    print(f"{status} {name}: {detail}")


def call(method, path, token=None, body=None, timeout=15):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            dt = (time.time() - t0) * 1000
            try:
                return resp.status, json.loads(raw), dt
            except json.JSONDecodeError:
                return resp.status, raw, dt
    except urllib.error.HTTPError as exc:
        dt = (time.time() - t0) * 1000
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), dt
        except json.JSONDecodeError:
            return exc.code, raw, dt
    except Exception as exc:  # noqa: BLE001
        dt = (time.time() - t0) * 1000
        return 0, f"{type(exc).__name__}: {exc}", dt


def login():
    st, body, _ = call("POST", "/api/v1/auth/login", body={"username": "admin", "password": "admin123"})
    if st != 200 or not isinstance(body, dict) or "token" not in body:
        print(f"登录失败 HTTP {st}: {body}")
        sys.exit(1)
    return body["token"]


def short(x, n=200):
    s = x if isinstance(x, str) else json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n] + "..."


token = login()

# 1. 枚举 GET 端点（无路径参数）
endpoints = []
try:
    text = SERVER_GO.read_text(encoding="utf-8")
    for m in re.finditer(r'"GET\s+([^"]+)"', text):
        p = m.group(1)
        if "{" in p or "}" in p:
            continue
        endpoints.append(p)
except Exception as e:
    print(f"读 server.go 失败: {e}")
    sys.exit(1)

endpoints = sorted(set(endpoints))
print(f"枚举到 {len(endpoints)} 个无参 GET 端点")

# 2. 单发计时
slow = []
very_slow = []
all_times = []
for p in endpoints:
    st, body, dt = call("GET", p, token=token, timeout=15)
    all_times.append((p, st, dt))
    tag = ""
    if dt > 10000:
        very_slow.append((p, st, dt))
        tag = "VERY_SLOW"
    elif dt > 5000:
        slow.append((p, st, dt))
        tag = "SLOW"
    print(f"{dt:8.1f}ms  HTTP {st:3d}  {p} {tag}")

# 3. SLOW 端点：加大参数再打一次
probe_results = []
for p, st, dt in slow + very_slow:
    # 尝试加大 limit/page size
    extra = ""
    if "?" in p:
        extra = "&limit=1000"
    else:
        extra = "?limit=1000"
    st2, body2, dt2 = call("GET", p + extra, token=token, timeout=15)
    probe_results.append((p, dt, dt2, st2))
    print(f"  probe {p}: {dt:.1f}ms -> {dt2:.1f}ms (HTTP {st2})")

# 4. 特测 diag/ping (POST, 外部依赖)
ping_times = []
for host in ["8.8.8.8", "127.0.0.1"]:
    st, body, dt = call("POST", "/api/v1/diag/ping", token=token,
                        body={"host": host, "count": 3}, timeout=20)
    ping_times.append((host, st, dt, body))
    print(f"  ping {host}: {dt:.1f}ms HTTP {st}")

# 5. 写报告
lines = []
lines.append("# 接口超时/慢查询探测报告")
lines.append("")
lines.append(f"> 生成: {__import__('time').strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
lines.append(f"**测了 {len(endpoints)} 个无参 GET 端点**")
lines.append(f"- SLOW (>5s): {len(slow)}")
lines.append(f"- VERY SLOW (>10s): {len(very_slow)}")
all_slow = slow + very_slow
if all_slow:
    slowest = max(all_slow, key=lambda x: x[2])
    lines.append(f"- 最慢: {slowest[0]} = {slowest[2]:.0f} ms")
else:
    slowest = max(all_times, key=lambda x: x[2])
    lines.append(f"- 最慢: {slowest[0]} = {slowest[2]:.0f} ms (无 >5s)")
lines.append("")
lines.append("## 1. 所有端点耗时 (ms, 降序)")
lines.append("")
lines.append("| endpoint | HTTP | ms |")
lines.append("|---|---|---|")
for p, st, dt in sorted(all_times, key=lambda x: -x[2]):
    tag = " 🔴" if dt > 10000 else (" 🟡" if dt > 5000 else "")
    lines.append(f"| {p} | {st} | {dt:.0f}{tag} |")
lines.append("")
lines.append("## 2. SLOW 端点探测 (加大 page size)")
lines.append("")
if probe_results:
    lines.append("| endpoint | 原 ms | 加大后 ms | HTTP |")
    lines.append("|---|---|---|---|")
    for p, dt, dt2, st2 in probe_results:
        lines.append(f"| {p} | {dt:.0f} | {dt2:.0f} | {st2} |")
else:
    lines.append("(无 SLOW 端点)")
lines.append("")
lines.append("## 3. diag/ping 外部依赖耗时")
lines.append("")
lines.append("| host | HTTP | ms |")
lines.append("|---|---|---|")
for host, st, dt, body in ping_times:
    lines.append(f"| {host} | {st} | {dt:.0f} |")
lines.append("")

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"\n测了 {len(endpoints)} 端点, SLOW={len(slow)}, VERY_SLOW={len(very_slow)}")
if all_slow:
    s = max(all_slow, key=lambda x: x[2])
    print(f"最慢: {s[0]} = {s[2]:.0f} ms")
else:
    s = max(all_times, key=lambda x: x[2])
    print(f"最慢: {s[0]} = {s[2]:.0f} ms (无 >5s)")
print(f"报告: {DOC}")
