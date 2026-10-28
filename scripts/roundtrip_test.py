#!/usr/bin/env python3
"""roundtrip_test.py — 写→读回 round-trip 测试，验证数据真的落盘。

用法: python3 scripts/roundtrip_test.py
报告: docs/roundtrip-test.md (脚本生成)
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
DOC = ROOT / "docs" / "roundtrip-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def call(method, path, token=None, body=None, timeout=20):
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
            try:
                return resp.status, json.loads(raw), time.time() - t0
            except json.JSONDecodeError:
                return resp.status, raw, time.time() - t0
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), time.time() - t0
        except json.JSONDecodeError:
            return exc.code, raw, time.time() - t0
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}", time.time() - t0


def login():
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    status, body, _ = call("POST", "/api/v1/auth/login",
                           body={"username": user, "password": pwd})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body}")
        sys.exit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]


def short(x):
    s = json.dumps(x, ensure_ascii=False)
    return s if len(s) < 120 else s[:117] + "..."


results = []


def rec(name, write_body, read_path, check_fn, write_path):
    global token
    # write
    st, resp, _ = call("POST", write_path, token=token, body=write_body)
    write_ok = (st == 200)
    read_status, read_data, _ = call("GET", read_path, token=token)
    read_ok = (read_status == 200)
    try:
        verified = check_fn(read_data, write_body, resp) if (write_ok and read_ok) else False
    except Exception as e:  # noqa: BLE001
        verified = False
        err_note = str(e)
    else:
        err_note = ""
    overall = write_ok and read_ok and verified
    results.append({
        "name": name,
        "write_path": write_path,
        "read_path": read_path,
        "write_status": st,
        "read_status": read_status,
        "write_value": write_body,
        "write_response": resp if write_ok else None,
        "read_value": read_data,
        "verified": verified,
        "overall": overall,
        "err_note": err_note,
    })
    print(f"{'PASS' if overall else 'FAIL'} {name}: write={st} read={read_status} verified={verified}")


token = login()

# 1. voucher/generate -> voucher/list
rec(
    "voucher/generate→list",
    {"count": 1, "code_len": 10, "minutes": 30, "traffic_mb": 0, "device_limit": 1, "expire_days": 0, "note": "roundtrip-test"},
    "/api/v1/voucher/list",
    lambda rd, wb, wr: any(v.get("code") in [x.get("code") for x in (wr.get("vouchers") or [])] for v in (rd.get("vouchers") or [])),
    "/api/v1/voucher/generate",
)

# 2. portal/template -> portal/templates
builtin = None
st, rd0, _ = call("GET", "/api/v1/portal/templates", token=token)
tmpl = (rd0.get("builtin") or [{}])[0] if isinstance(rd0, dict) else {}
tmpl = dict(tmpl)
tmpl["title"] = "roundtrip-portal-title"
rec(
    "portal/template→templates",
    {"template": tmpl},
    "/api/v1/portal/templates",
    lambda rd, wb, wr: (rd.get("current") or {}).get("title") == wb["template"]["title"],
    "/api/v1/portal/template",
)

# 3. linkcheck/targets -> linkcheck/targets
tgt = {
    "id": "rt-probe-1", "name": "roundtrip", "iface": "eth0", "kind": "icmp",
    "host": "127.0.0.1", "interval": 30, "timeout": 5,
    "fail_threshold": 3, "recover_threshold": 2, "enabled": True,
}
rec(
    "linkcheck/targets→targets",
    {"targets": [tgt]},
    "/api/v1/linkcheck/targets",
    lambda rd, wb, wr: any(t.get("id") == "rt-probe-1" for t in (rd.get("targets") or [])),
    "/api/v1/linkcheck/targets",
)

# 4. ddns/records -> ddns/records
rec_ddns = {
    "id": "rt-ddns-1", "provider": "cloudflare", "domain": "example.com",
    "subdomain": "rt", "token": "roundtriptoken1234", "enabled": True,
}
rec(
    "ddns/records→records",
    {"records": [rec_ddns], "confirm": True},
    "/api/v1/ddns/records",
    lambda rd, wb, wr: any(r.get("domain") == "example.com" for r in (rd.get("records") if isinstance(rd, dict) else rd or [])),
    "/api/v1/ddns/records",
)

# 5. policy/timeschedule -> policy/timeschedule (confirm=true 才落盘; nft 不可用可能不保存)
rule = {
    "id": "rt-rule-1", "name": "roundtrip", "days": [1, 2, 3, 4, 5],
    "start_min": 60, "end_min": 120, "targets": ["10.0.0.0/24"],
    "action": "ratelimit", "rate_kbps": 1000, "enabled": True,
}
rec(
    "policy/timeschedule→timeschedule",
    {"rules": [rule], "confirm": True},
    "/api/v1/policy/timeschedule",
    lambda rd, wb, wr: any(r.get("id") == "rt-rule-1" for r in (rd.get("rules") or [])),
    "/api/v1/policy/timeschedule",
)

# 写报告
lines = []
lines.append("# 写端点 Round-trip 测试报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
lines.append(f"**汇总**: 测了 {len(results)} 个写端点 / PASS {sum(1 for r in results if r['overall'])} / FAIL {sum(1 for r in results if not r['overall'])}")
lines.append("")
lines.append("| # | 写端点 | 读端点 | 写码 | 读码 | 结果 |")
lines.append("|---|---|---|---|---|---|")
for i, r in enumerate(results, 1):
    lines.append(f"| {i} | `{r['write_path']}` | `{r['read_path']}` | {r['write_status']} | {r['read_status']} | {'PASS' if r['overall'] else 'FAIL'} |")
lines.append("")
for i, r in enumerate(results, 1):
    lines.append(f"## {i}. {r['name']}")
    lines.append("")
    lines.append(f"- 写入值（完整，不截断——截断会让审查者无法核对读回命中）:")
    lines.append("")
    lines.append("```json")
    lines.append(json.dumps(r["write_value"], ensure_ascii=False, indent=2))
    lines.append("```")
    lines.append("")
    if r["write_response"] is not None:
        lines.append(f"- 写接口返回（用于确认写成功）:")
        lines.append("")
        lines.append("```json")
        lines.append(json.dumps(r["write_response"], ensure_ascii=False, indent=2)[:3000])
        lines.append("```")
        lines.append("")
    lines.append(f"- **落盘确认 verified = {r['verified']}**"
                 f"（True 表示写进去的标识能在对应读接口里找到）")
    lines.append("")
    lines.append(f"- 读回值（完整）:")
    lines.append("")
    lines.append("```json")
    lines.append(json.dumps(r["read_value"], ensure_ascii=False, indent=2))
    lines.append("```")
    lines.append("")
    if r["err_note"]:
        lines.append(f"- 异常: {r['err_note']}")
    lines.append("")

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")

for r in results:
    print(f"{'PASS' if r['overall'] else 'FAIL'} {r['name']}: write={r['write_status']} read={r['read_status']} verified={r['verified']}")
print(f"\n汇总: {len(results)} 个写端点 / PASS {sum(1 for r in results if r['overall'])} / FAIL {sum(1 for r in results if not r['overall'])}")
print(f"报告: {DOC}")
sys.exit(0 if all(r['overall'] for r in results) else 1)
