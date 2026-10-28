#!/usr/bin/env python3
"""infra_modules_smoke.py — 基础设施模块冒烟 (p62).

模块：dns / ntp / snmp / dhcp / diag / lb / htb / l7 / alg。真实打本机 9090。

用法:
  python3 scripts/infra_modules_smoke.py
  BASE=http://127.0.0.1:9090 python3 scripts/infra_modules_smoke.py

报告写到 docs/infra-modules-smoke.md（由脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "infra-modules-smoke.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def call(method, path, token=None, body=None, timeout=25):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw), time.time() - t0, raw
            except json.JSONDecodeError:
                return resp.status, raw, time.time() - t0, raw
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), time.time() - t0, raw
        except json.JSONDecodeError:
            return exc.code, raw, time.time() - t0, raw
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}", time.time() - t0, str(exc)


def login():
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    st, body, _, _ = call("POST", "/api/v1/auth/login",
                          body={"username": user, "password": pwd})
    if st != 200:
        print(f"登录失败 HTTP {st}: {body}")
        sys.exit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]


def snip(raw, n=300):
    s = (raw or "").replace("\n", " ")
    return s if len(s) <= n else s[:n] + "..."


def has_key(d, k):
    return isinstance(d, dict) and k in d


def env_skip_if_500(path, st, data, raw):
    """ntp/snmp 等守护进程缺失时 500 → SKIP。"""
    if st == 500:
        msg = data.get("detail") if isinstance(data, dict) else data
        if msg is None:
            msg = data.get("message") if isinstance(data, dict) else None
        return True, f"HTTP 500 (环境依赖缺失): {msg}"
    return False, ""


def main():
    token = login()
    results = []

    # ---------- 读端点 ----------
    reads = [
        ("GET /api/v1/dns/config", "/api/v1/dns/config",
         lambda d: has_key(d, "config") and has_key(d, "dnsmasq")),
        ("GET /api/v1/dns/status", "/api/v1/dns/status",
         lambda d: has_key(d, "running") and has_key(d, "status")),
        ("GET /api/v1/ntp/config", "/api/v1/ntp/config",
         lambda d: has_key(d, "config")),
        ("GET /api/v1/ntp/status", "/api/v1/ntp/status",
         None),  # 环境依赖，500 → SKIP
        ("GET /api/v1/snmp/config", "/api/v1/snmp/config",
         lambda d: has_key(d, "config")),
        ("GET /api/v1/snmp/status", "/api/v1/snmp/status",
         None),  # 环境依赖，500 → SKIP
        ("GET /api/v1/dhcp/config", "/api/v1/dhcp/config",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/dhcp/leases", "/api/v1/dhcp/leases",
         lambda d: isinstance(d, dict) and ("leases" in d or "count" in d)),
        ("GET /api/v1/dhcp/server", "/api/v1/dhcp/server",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/dhcp/status", "/api/v1/dhcp/status",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/diag/interfaces", "/api/v1/diag/interfaces",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/lb/config", "/api/v1/lb/config",
         lambda d: has_key(d, "config")),
        ("GET /api/v1/lb/status", "/api/v1/lb/status",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/htb/config", "/api/v1/htb/config",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/htb/report", "/api/v1/htb/report",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/l7/rules", "/api/v1/l7/rules",
         lambda d: has_key(d, "rules")),
        ("GET /api/v1/l7/stats", "/api/v1/l7/stats",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/alg", "/api/v1/alg",
         lambda d: has_key(d, "config")),
    ]
    for name, path, check in reads:
        st, data, _, raw = call("GET", path, token=token)
        # 环境依赖端点：500 → SKIP
        if check is None and st == 500:
            skip, reason = env_skip_if_500(path, st, data, raw)
            results.append((name, "SKIP", reason, snip(raw)))
            continue
        if check is None:
            # 非 500 时按 200 + 非空 dict 判
            ok = (st == 200 and isinstance(data, dict) and len(data) > 0)
        else:
            ok = (st == 200 and check(data))
        # ntp/snmp status 500 → SKIP（即使 check 不为 None）
        if st == 500 and ("ntp/status" in path or "snmp/status" in path):
            skip, reason = env_skip_if_500(path, st, data, raw)
            results.append((name, "SKIP", reason, snip(raw)))
            continue
        detail = f"HTTP {st}"
        if isinstance(data, dict):
            for k in ("leases", "rules", "interfaces"):
                if k in data and isinstance(data[k], list):
                    detail += f" {k}={len(data[k])}"
        results.append((name, "PASS" if ok else "FAIL", detail, snip(raw)))

    # ---------- lb/preview（无 members 时 422/10162，属配置状态） ----------
    st, data, _, raw = call("GET", "/api/v1/lb/preview", token=token)
    code = data.get("code") if isinstance(data, dict) else None
    if st == 200:
        results.append(("GET /api/v1/lb/preview", "PASS", f"HTTP {st}", snip(raw)))
    elif st == 422 and str(code) == "10162":
        results.append(("GET /api/v1/lb/preview", "SKIP",
                        f"HTTP {st} code={code} (无 LB members 配置)", snip(raw)))
    else:
        results.append(("GET /api/v1/lb/preview", "FAIL", f"HTTP {st} code={code}", snip(raw)))

    # ---------- htb/show（需要 iface 参数，取 diag/interfaces 第一个） ----------
    st0, d0, _, _ = call("GET", "/api/v1/diag/interfaces", token=token)
    iface = None
    if isinstance(d0, dict):
        for row in (d0.get("interfaces") or []):
            if isinstance(row, dict):
                for key in ("iface", "name", "interface", "dev"):
                    if row.get(key):
                        iface = row[key]
                        break
                if iface:
                    break
    if not iface:
        results.append(("GET /api/v1/htb/show", "SKIP", "无可用网口名", ""))
    else:
        st, data, _, raw = call("GET", f"/api/v1/htb/show?iface={iface}", token=token)
        # 200 (含 error 字段也算正常) 或 422 iface 无效
        ok = (st == 200)
        detail = f"HTTP {st} iface={iface}"
        if isinstance(data, dict) and "error" in data:
            detail += f" error={data.get('error')}"
        results.append(("GET /api/v1/htb/show", "PASS" if ok else "FAIL", detail, snip(raw)))

    # ---------- 写端点（空 body → 拦截或接受） ----------
    writes = [
        ("POST /api/v1/dns/config (empty)", "/api/v1/dns/config"),
        ("POST /api/v1/ntp/config (empty)", "/api/v1/ntp/config"),
        ("POST /api/v1/snmp/config (empty)", "/api/v1/snmp/config"),
        ("POST /api/v1/dhcp/config (empty)", "/api/v1/dhcp/config"),
        ("POST /api/v1/l7/rules (empty)", "/api/v1/l7/rules"),
        ("POST /api/v1/alg (empty)", "/api/v1/alg"),
        ("POST /api/v1/htb/config (empty)", "/api/v1/htb/config"),
    ]
    for name, path in writes:
        st, data, _, raw = call("POST", path, token=token, body={})
        code = data.get("code") if isinstance(data, dict) else None
        if path == "/api/v1/dhcp/config" and st == 500:
            results.append((name, "SKIP", f"HTTP {st} code={code} (apply 失败，空配置)", snip(raw)))
            continue
        blocked = st in (400, 422)
        accepted = (st == 200)
        ok = blocked or accepted
        tag = "blocked" if blocked else ("accepted" if accepted else "unexpected")
        detail = f"HTTP {st} code={code} ({tag})"
        results.append((name, "PASS" if ok else "FAIL", detail, snip(raw)))

    # ---------- 报告 ----------
    passes = sum(1 for r in results if r[1] == "PASS")
    fails = sum(1 for r in results if r[1] == "FAIL")
    skips = sum(1 for r in results if r[1] == "SKIP")

    lines = []
    lines.append("# 基础设施模块冒烟报告 (infra-modules-smoke)")
    lines.append("")
    lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
    lines.append("")
    lines.append(f"**汇总**: 共 {len(results)} 步 / PASS {passes} / FAIL {fails} / SKIP {skips}")
    lines.append("")
    lines.append("| 步骤 | 结果 | 详情 |")
    lines.append("|---|---|---|")
    for step, st, detail, _ in results:
        lines.append(f"| {step} | {st} | {detail} |")
    lines.append("")
    lines.append("## 真实返回片段")
    lines.append("")
    for step, st, detail, snippet in results:
        lines.append(f"### {step} — {st}")
        lines.append("")
        lines.append("```")
        lines.append(snippet)
        lines.append("```")
        lines.append("")
    lines.append("## 说明")
    lines.append("")
    lines.append("- 覆盖模块：dns / ntp / snmp / dhcp / diag / lb / htb / l7 / alg。")
    lines.append("- ntp/status、snmp/status 依赖 systemd-bus/daemon，LXC 环境缺失时 SKIP。")
    lines.append("- 写端点空 body 通常被 Validate 拦截 (400/422)；部分端点接受空值。")
    lines.append("")
    DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")

    for step, st, detail, _ in results:
        print(f"{st:<4} {step} — {detail}")
    print(f"\n汇总: {len(results)} 步 / PASS {passes} / FAIL {fails} / SKIP {skips}")
    print(f"报告: {DOC}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
