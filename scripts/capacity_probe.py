#!/usr/bin/env python3
"""capacity_probe.py — 大表端点容量/分页压力探针。

对 /api/v1/system/services、/api/v1/logs、/api/v1/terminals 三个端点做：
  1. 单次 GET 计时 + 响应字节数
  2. 若支持 limit/offset，测 limit=10/100/1000 三档耗时对比；不支持记「无分页」
  3. 连续打 20 次 /api/v1/terminals，记录 max/min/avg，判断抖动（max/avg > 5）

用法: python3 scripts/capacity_probe.py
报告: docs/capacity-probe.md (脚本生成)
"""
from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "capacity-probe.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def call(path, token=None, timeout=30):
    url = BASE + path
    req = urllib.request.Request(url, method="GET")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            return resp.status, raw, time.time() - t0
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        return exc.code, raw, time.time() - t0
    except Exception as exc:  # noqa: BLE001
        return 0, str(exc).encode(), time.time() - t0


def login():
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    req = urllib.request.Request(BASE + "/api/v1/auth/login",
                                data=json.dumps({"username": user, "password": pwd}).encode(),
                                method="POST", headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read())["token"]


def stats(times):
    if not times:
        return {}
    return {
        "n": len(times),
        "min_ms": round(min(times) * 1000, 1),
        "max_ms": round(max(times) * 1000, 1),
        "avg_ms": round(sum(times) / len(times) * 1000, 1),
        "jitter": (max(times) / (sum(times) / len(times))) > 5 if times else False,
    }


token = login()

endpoints = [
    ("/api/v1/system/services", False, "systemctl list-units 全量，无分页参数"),
    ("/api/v1/logs", True, "支持 limit (1..5000)"),
    ("/api/v1/terminals", False, "聚合 ARP+DHCP+conntrack，无分页参数"),
]

report_lines = []
report_lines.append("# 容量/分页压力探针报告 (capacity-probe)")
report_lines.append("")
report_lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
report_lines.append("")

# 1. 单次计时 + 字节数
report_lines.append("## 1. 单次 GET 计时 + 响应大小")
report_lines.append("")
report_lines.append("| 端点 | HTTP | 耗时(ms) | 响应字节 | 条数 | 有分页 |")
report_lines.append("|---|---|---|---|---|---|")
single = {}
for path, has_page, note in endpoints:
    st, raw, dt = call(path, token=token)
    try:
        d = json.loads(raw)
        cnt = d.get("count", len(d.get("entries", d.get("terminals", d.get("units", [])))))
    except Exception:
        cnt = "?"
    single[path] = (st, len(raw), dt, cnt)
    report_lines.append(f"| `{path}` | {st} | {dt*1000:.1f} | {len(raw)} | {cnt} | {'是' if has_page else '否'} |")
    print(f"{path}: HTTP {st} {dt*1000:.1f}ms {len(raw)} bytes count={cnt}")

# 2. 分页三档
report_lines.append("")
report_lines.append("## 2. 分页 limit 三档对比（仅 /api/v1/logs 支持）")
report_lines.append("")
report_lines.append("| 端点 | limit=10 | limit=100 | limit=1000 | 结论 |")
report_lines.append("|---|---|---|---|---|")
for path, has_page, note in endpoints:
    if not has_page:
        report_lines.append(f"| `{path}` | — | — | — | 无分页参数，全量返回 |")
        print(f"{path}: 无分页")
        continue
    times = {}
    for lim in (10, 100, 1000):
        st, raw, dt = call(f"{path}?limit={lim}", token=token)
        times[lim] = dt
        print(f"  {path} limit={lim}: {dt*1000:.1f}ms")
    report_lines.append(
        f"| `{path}` | {times[10]*1000:.1f}ms | {times[100]*1000:.1f}ms | {times[1000]*1000:.1f}ms | "
        f"limit 越大耗时{'递增' if times[1000] > times[10] else '基本持平'} |")

# 3. terminals 20 次抖动
report_lines.append("")
report_lines.append("## 3. /api/v1/terminals 连续 20 次抖动探测")
report_lines.append("")
times = []
for i in range(20):
    st, raw, dt = call("/api/v1/terminals", token=token)
    times.append(dt)
s = stats(times)
report_lines.append(f"- n={s['n']}  min={s['min_ms']}ms  max={s['max_ms']}ms  avg={s['avg_ms']}ms")
report_lines.append(f"- max/avg = {s['max_ms']/s['avg_ms']:.2f}  → {'**抖动** (>5)' if s['jitter'] else '无明显抖动 (<=5)'}")
print(f"terminals 20x: min={s['min_ms']} max={s['max_ms']} avg={s['avg_ms']} jitter={s['jitter']}")

# 4. 优化建议
report_lines.append("")
report_lines.append("## 4. 优化建议（仅建议，未改代码）")
report_lines.append("")
report_lines.append("- `/api/v1/system/services`：无分页，systemctl 全量返回；小内存路由上建议加 `limit`/`unit` 过滤或前端虚拟滚动。")
report_lines.append("- `/api/v1/logs`：已支持 `limit`（上限 5000），建议前端默认 limit=200 并提供分页。")
report_lines.append("- `/api/v1/terminals`：聚合三源无分页；34 条时 harmless，但 ARP/conntrack 大时可能膨胀，建议按 iface/source 过滤或分页。")
report_lines.append("- 若 max/avg > 5 记为抖动，需排查 GC/锁竞争；本报告按实测结论。")

DOC.write_text("\n".join(report_lines) + "\n", encoding="utf-8")
print(f"\n报告: {DOC}")
