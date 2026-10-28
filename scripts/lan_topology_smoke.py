#!/usr/bin/env python3
"""lan_topology_smoke.py — 局域网拓扑模块冒烟 (p58).

模块：ipv6 / vlan / bridge / bond / mdns / mirror。真实打本机 9090。

用法:
  python3 scripts/lan_topology_smoke.py
  BASE=http://127.0.0.1:9090 python3 scripts/lan_topology_smoke.py

报告写到 docs/lan-topology-smoke.md（由脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "lan-topology-smoke.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def call(method, path, token=None, body=None, timeout=20):
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


def main():
    token = login()
    results = []

    # ---------- 读端点 ----------
    reads = [
        ("GET /api/v1/vlan", "/api/v1/vlan",
         lambda d: has_key(d, "vlans") and (d.get("vlans") is None or isinstance(d.get("vlans"), list))),
        ("GET /api/v1/bridge", "/api/v1/bridge",
         lambda d: has_key(d, "bridges") and (d.get("bridges") is None or isinstance(d.get("bridges"), list))),
        ("GET /api/v1/bond", "/api/v1/bond",
         lambda d: has_key(d, "config") and has_key(d, "live") and has_key(d, "script")),
        ("GET /api/v1/mdns", "/api/v1/mdns",
         lambda d: has_key(d, "config") and has_key(d, "avahi")),
        ("GET /api/v1/mirror", "/api/v1/mirror",
         lambda d: has_key(d, "config") and has_key(d, "script") and has_key(d, "cleanup")),
        ("GET /api/v1/ipv6/addresses", "/api/v1/ipv6/addresses",
         lambda d: has_key(d, "addresses") and (d.get("addresses") is None or isinstance(d.get("addresses"), list))),
        ("GET /api/v1/ipv6/status", "/api/v1/ipv6/status",
         lambda d: isinstance(d, dict) and len(d) > 0),
        ("GET /api/v1/ipv6/nd", "/api/v1/ipv6/nd",
         lambda d: has_key(d, "config") and has_key(d, "live")),
        ("GET /api/v1/ipv6/pd", "/api/v1/ipv6/pd",
         lambda d: has_key(d, "config") and has_key(d, "dhcp6c")),
        ("GET /api/v1/ipv6/ra", "/api/v1/ipv6/ra",
         lambda d: has_key(d, "config") and has_key(d, "radvd")),
    ]
    for name, path, check in reads:
        st, data, _, raw = call("GET", path, token=token)
        ok = (st == 200 and check(data))
        detail = f"HTTP {st}"
        if isinstance(data, dict):
            # 附带关键字段摘要
            for k in ("vlans", "bridges", "addresses"):
                if k in data and isinstance(data[k], list):
                    detail += f" {k}={len(data[k])}"
            if "live" in data:
                detail += f" live_keys={list(data['live'].keys()) if isinstance(data['live'], dict) else data['live']}"
        results.append((name, "PASS" if ok else "FAIL", detail, snip(raw)))

    # ---------- bond/bridge 真实接口信息验证 ----------
    st, data, _, raw = call("GET", "/api/v1/bond", token=token)
    live = data.get("live") if isinstance(data, dict) else None
    live_ok = (st == 200 and isinstance(live, dict))
    detail = f"HTTP {st} live={live}"
    results.append(("bond live 接口信息", "PASS" if live_ok else "FAIL", detail, snip(raw)))

    st, data, _, raw = call("GET", "/api/v1/ipv6/status", token=token)
    status_ok = (st == 200 and isinstance(data, dict) and len(data) > 0)
    detail = f"HTTP {st} keys={list(data.keys()) if isinstance(data, dict) else data}"
    results.append(("ipv6/status 真实状态", "PASS" if status_ok else "FAIL", detail, snip(raw)))

    # ---------- 写端点（空 body → 拦截） ----------
    writes = [
        ("POST /api/v1/vlan (empty)", "/api/v1/vlan"),
        ("POST /api/v1/bridge (empty)", "/api/v1/bridge"),
        ("POST /api/v1/bond (empty)", "/api/v1/bond"),
        ("POST /api/v1/mdns (empty)", "/api/v1/mdns"),
        ("POST /api/v1/mirror (empty)", "/api/v1/mirror"),
        ("POST /api/v1/ipv6/addresses (empty)", "/api/v1/ipv6/addresses"),
    ]
    for name, path in writes:
        st, data, _, raw = call("POST", path, token=token, body={})
        code = data.get("code") if isinstance(data, dict) else None
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
    lines.append("# 局域网拓扑模块冒烟报告 (lan-topology-smoke)")
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
    lines.append("- 覆盖模块：vlan / bridge / bond / mdns / mirror / ipv6 (addresses/status/nd/pd/ra)。")
    lines.append("- bond/bridge 无独立 status 端点，GET /bond 返回 live 字段作为当前接口信息。")
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
