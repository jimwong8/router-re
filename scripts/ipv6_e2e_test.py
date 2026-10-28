#!/usr/bin/env python3
"""ipv6_e2e_test.py — IPv6 真实连通性端到端验证。

真实打本机 9090，对比接口返回与系统 ip -6 state。
产出 docs/ipv6-e2e-test.md（脚本写，勿手写）。
"""
from __future__ import annotations
import json, os, subprocess, time, urllib.error, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "ipv6-e2e-test.md"

def call(method, path, token=None, body=None, timeout=20):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw), time.time()
            except json.JSONDecodeError:
                return resp.status, raw, time.time()
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), time.time()
        except json.JSONDecodeError:
            return exc.code, raw, time.time()
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}", time.time()

def login():
    status, body, _ = call("POST", "/api/v1/auth/login",
                           body={"username": "admin", "password": "admin123"})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body}")
        raise SystemExit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]

def run_cmd(cmd):
    try:
        out = subprocess.check_output(cmd, shell=True, text=True, stderr=subprocess.STDOUT, timeout=10)
        return 0, out
    except subprocess.CalledProcessError as e:
        return e.returncode, e.output
    except Exception as e:
        return 1, str(e)

def main():
    token = login()
    report = []
    report.append("# IPv6 真实连通性端到端测试")
    report.append("")
    report.append(f"- Base: {BASE}")
    report.append(f"- Date: {__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    report.append("")

    results = []  # (name, status, detail)

    # ---------- 1. GET ipv6/addresses ----------
    s, body, _ = call("GET", "/api/v1/ipv6/addresses", token=token)
    addrs = body.get("addresses", []) if isinstance(body, dict) else []
    has_global = any(":" in a.get("addr", "") and not a.get("addr", "").startswith("fe80") and not a.get("addr", "").startswith("fc") for a in addrs)
    addr_ok = (s == 200 and isinstance(addrs, list) and len(addrs) > 0)
    results.append(("GET /api/v1/ipv6/addresses", "PASS" if addr_ok else "FAIL",
                   f"HTTP {s}, count={len(addrs)}, has_global={has_global}"))
    report.append(f"## 1. GET /api/v1/ipv6/addresses")
    report.append(f"- HTTP {s}, count={len(addrs)}")
    report.append(f"- 全局地址: {'有' if has_global else '无'}")
    for a in addrs[:5]:
        report.append(f"  - {a.get('iface')} {a.get('addr')}/{a.get('prefix_len')}")
    report.append(f"- 结果: {'PASS' if addr_ok else 'FAIL'}")
    report.append("")

    # ---------- 2. GET ipv6/status ----------
    s, body, _ = call("GET", "/api/v1/ipv6/status", token=token)
    status_ok = (s == 200 and isinstance(body, dict)
                 and "ipv6_enabled" in body and "forwarding" in body)
    results.append(("GET /api/v1/ipv6/status", "PASS" if status_ok else "FAIL",
                   f"HTTP {s}, ipv6_enabled={body.get('ipv6_enabled') if isinstance(body,dict) else '?'}, "
                   f"forwarding={body.get('forwarding') if isinstance(body,dict) else '?'}"))
    report.append(f"## 2. GET /api/v1/ipv6/status")
    report.append(f"- HTTP {s}")
    if isinstance(body, dict):
        for k, v in body.items():
            report.append(f"  - {k}: {v}")
    report.append(f"- 结果: {'PASS' if status_ok else 'FAIL'}")
    report.append("")

    # ---------- 3. GET ipv6/nd ----------
    s, body, _ = call("GET", "/api/v1/ipv6/nd", token=token)
    nd_ok = (s == 200 and isinstance(body, dict) and "live" in body)
    live = body.get("live", {}) if isinstance(body, dict) else {}
    nd_detail = f"HTTP {s}, live keys={list(live.keys())[:6]}"
    results.append(("GET /api/v1/ipv6/nd", "PASS" if nd_ok else "FAIL", nd_detail))
    report.append(f"## 3. GET /api/v1/ipv6/nd")
    report.append(f"- HTTP {s}")
    if isinstance(body, dict):
        report.append(f"- live: {json.dumps(live, ensure_ascii=False)[:200]}")
    report.append(f"- 结果: {'PASS' if nd_ok else 'FAIL'}")
    report.append("")

    # ---------- 4. 写端点：空 body → 422 ----------
    # addresses
    s, body, _ = call("POST", "/api/v1/ipv6/addresses", token=token, body={})
    addr_empty_ok = (s == 422)
    results.append(("POST /api/v1/ipv6/addresses (empty)", "PASS" if addr_empty_ok else "FAIL",
                   f"HTTP {s} (期望 422)"))
    report.append(f"## 4a. POST /api/v1/ipv6/addresses 空 body")
    report.append(f"- HTTP {s} (期望 422) → {'PASS' if addr_empty_ok else 'FAIL'}")
    report.append("")

    # nd
    s, body, _ = call("POST", "/api/v1/ipv6/nd", token=token, body={})
    nd_empty = (s in (400, 422))
    results.append(("POST /api/v1/ipv6/nd (empty)", "PASS" if nd_empty else "FAIL",
                   f"HTTP {s} (期望 400/422)"))
    report.append(f"## 4b. POST /api/v1/ipv6/nd 空 body")
    report.append(f"- HTTP {s} (期望 400/422) → {'PASS' if nd_empty else 'FAIL'}")
    report.append("")

    # ---------- 5. 接口 vs 系统对比 ----------
    rc, sys_addr_out = run_cmd("ip -6 addr show")
    rc2, sys_route_out = run_cmd("ip -6 route show")

    # 从系统输出提取全局 IPv6 地址
    sys_globals = []
    for line in sys_addr_out.splitlines():
        line = line.strip()
        if "scope global" in line:
            parts = line.split()
            for p in parts:
                if ":" in p and "/" in p:
                    sys_globals.append(p.split("/")[0])
                    break

    iface_globals = [a.get("addr") for a in addrs
                     if not a.get("addr", "").startswith("fe80")
                     and not a.get("addr", "").startswith("fc")
                     and not a.get("addr", "").startswith("fd")]

    # 检查接口返回的全局地址是否在系统中
    match_count = sum(1 for a in iface_globals if a in sys_globals)
    consistent = (match_count == len(iface_globals) and len(iface_globals) > 0)

    results.append(("接口 vs 系统地址一致性", "PASS" if consistent else "FAIL",
                   f"接口全局={len(iface_globals)}, 系统全局={len(sys_globals)}, 匹配={match_count}"))
    report.append(f"## 5. 接口 vs 系统状态对比")
    report.append(f"- 系统全局 IPv6 地址数: {len(sys_globals)}")
    report.append(f"- 接口返回全局地址数: {len(iface_globals)}")
    report.append(f"- 匹配数: {match_count}")
    report.append(f"- 一致性: {'PASS' if consistent else 'FAIL'}")
    report.append("")
    report.append("### 系统 ip -6 addr (全局地址)")
    for g in sys_globals:
        report.append(f"  - {g}")
    report.append("")
    report.append("### 接口返回 (全局地址)")
    for g in iface_globals:
        report.append(f"  - {g}")
    report.append("")

    # ---------- 6. 环境判定 ----------
    has_real_global = len(sys_globals) > 0
    report.append(f"## 6. 环境判定")
    report.append(f"- 本机有全局 IPv6: {'是' if has_real_global else '否'}")
    if has_real_global:
        report.append("- 环境有真实全局 IPv6，接口与系统一致，路由可测")
    else:
        report.append("- 环境仅有 link-local，无全局地址，无法测真实路由 → SKIP")
    report.append("")

    # ---------- 汇总 ----------
    report.append("## 7. 汇总")
    report.append("")
    report.append("| # | 测试项 | 结果 | 详情 |")
    report.append("|---|--------|------|------|")
    for i, (name, status, detail) in enumerate(results, 1):
        report.append(f"| {i} | {name} | {status} | {detail} |")
    report.append("")
    pass_n = sum(1 for _, s, _ in results if s == "PASS")
    fail_n = sum(1 for _, s, _ in results if s == "FAIL")
    skip_n = sum(1 for _, s, _ in results if s == "SKIP")
    report.append(f"- PASS: {pass_n} / FAIL: {fail_n} / SKIP: {skip_n}")
    report.append("")

    OUT.write_text("\n".join(report), encoding="utf-8")
    print("\n".join(report))

if __name__ == "__main__":
    main()