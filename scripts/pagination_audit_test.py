#!/usr/bin/env python3
"""pagination_audit_test.py - audit list endpoints pagination (no dup/no miss)."""
from __future__ import annotations

import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "pagination-audit-test.md"
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
        req.add_header("Authorization", "Bearer " + token)
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw)
            except json.JSONDecodeError:
                return resp.status, raw
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw)
        except json.JSONDecodeError:
            return exc.code, raw
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}"


def login():
    st, body = call("POST", "/api/v1/auth/login", body={"username": "admin", "password": "admin123"})
    if st != 200 or not isinstance(body, dict) or "token" not in body:
        print(f"登录失败 HTTP {st}: {body}")
        sys.exit(1)
    return body["token"]


def short(x, n=200):
    s = x if isinstance(x, str) else json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n] + "..."


def find_list(obj):
    if not isinstance(obj, dict):
        return None
    for k, v in obj.items():
        if isinstance(v, list):
            total = obj.get("total")
            if total is None:
                total = obj.get("count")
            if total is None:
                total = len(v)
            return k, v, int(total)
    return None


def safe_list(body):
    info = find_list(body) if isinstance(body, dict) else None
    if info is None:
        return None, [], 0
    key, lst, total = info
    return key, lst, total


def item_key(item):
    try:
        return json.dumps(item, sort_keys=True, ensure_ascii=False)
    except Exception:
        return str(item)


token = login()

endpoints = []
try:
    text = (ROOT / "mgmtd" / "internal" / "server" / "server.go").read_text(encoding="utf-8")
    for m in re.finditer(r'"GET\s+([^"]+)"', text):
        p = m.group(1)
        if "{" in p or "}" in p:
            continue
        endpoints.append(p)
except Exception as e:
    print(f"读 server.go 失败: {e}")
    sys.exit(1)

endpoints = sorted(set(endpoints))

list_endpoints = []
for p in endpoints:
    st, body = call("GET", p + "?limit=3", token=token, timeout=15)
    if st != 200 or not isinstance(body, dict):
        continue
    key, lst, total = safe_list(body)
    if key is None:
        continue
    list_endpoints.append(p)

print(f"检测到 {len(list_endpoints)} 个列表端点")

audit = []
for p in list_endpoints:
    # first page to get total + list key
    st, body = call("GET", p + "?limit=10&offset=0", token=token, timeout=15)
    key, lst0, total = safe_list(body) if isinstance(body, dict) else (None, [], 0)
    N = total if total and total > 0 else None

    # skip empty endpoints quickly
    if N == 0:
        supports = None
        b_results = []
        st, body = call("GET", f"{p}?limit=0&offset=0", token=token, timeout=15)
        b_results.append(f"limit=0 -> HTTP {st} items=0")
        st, body = call("GET", f"{p}?limit=5&offset=100", token=token, timeout=15)
        _, lst_b, _ = safe_list(body)
        b_results.append(f"offset>N -> HTTP {st} items={len(lst_b)}")
        add(p, "PASS", f"N=0 collected=0 dedup=0 dup=0 missing=0 supports={supports}", short({"boundary": b_results}))
        audit.append({"endpoint": p, "N": 0, "collected": 0, "dedup": 0,
                      "dup": 0, "missing": 0, "supports": supports,
                      "boundary": b_results, "status": "PASS"})
        print(f"  -> {p} done (empty)", flush=True)
        continue

    print(f"  -> processing {p} (N={N})", flush=True)
    collected = []
    seen = set()
    dup_count = 0
    page = 0
    while page < 10:
        st, body = call("GET", f"{p}?limit=10&offset={page*10}", token=token, timeout=15)
        if st != 200 or not isinstance(body, dict):
            break
        k, lst, t = safe_list(body)
        if k is None or not lst:
            break
        if N is None:
            N = t if t and t > 0 else len(lst)
        for item in lst:
            kk = item_key(item)
            if kk in seen:
                dup_count += 1
            else:
                seen.add(kk)
            collected.append(item)
        if len(lst) < 10:
            break
        page += 1

    if N is None:
        N = len(collected)
    dedup_count = len(seen)
    missing = N - dedup_count if N >= dedup_count else 0

    # supports pagination?
    st1, body1 = call("GET", f"{p}?limit=5&offset=0", token=token, timeout=15)
    st2, body2 = call("GET", f"{p}?limit=5&offset=5", token=token, timeout=15)
    _, lst1, _ = safe_list(body1)
    _, lst2, _ = safe_list(body2)
    supports = None
    if len(lst1) > 0:
        keys1 = [item_key(x) for x in lst1]
        keys2 = [item_key(x) for x in lst2]
        supports = (keys1 != keys2) or (len(keys2) == 0)

    # boundary tests
    b_results = []
    st, body = call("GET", f"{p}?limit=0&offset=0", token=token, timeout=15)
    _, lst0b, _ = safe_list(body)
    b_results.append(f"limit=0 -> HTTP {st} items={len(lst0b)}")
    st, body = call("GET", f"{p}?limit=5&offset={N+100}", token=token, timeout=15)
    _, lst_b, _ = safe_list(body)
    b_results.append(f"offset>N -> HTTP {st} items={len(lst_b)}")
    st, body = call("GET", f"{p}?limit=100&offset=0", token=token, timeout=30)
    _, lst_h, _ = safe_list(body)
    b_results.append(f"limit=1000 -> HTTP {st} items={len(lst_h)} (N={N})")

    ok = (dedup_count == N and dup_count == 0)
    status = "PASS" if ok else "FAIL"
    if supports is False:
        status = "NOPAGE"
    detail = (f"N={N} collected={len(collected)} dedup={dedup_count} dup={dup_count} "
              f"missing={missing} supports={supports}")
    add(p, status, detail, short({"boundary": b_results}))
    audit.append({"endpoint": p, "N": N, "collected": len(collected), "dedup": dedup_count,
                  "dup": dup_count, "missing": missing, "supports": supports,
                  "boundary": b_results, "status": status})

supported = sum(1 for a in audit if a["supports"] is True)
unsupported = sum(1 for a in audit if a["supports"] is False)
unknown = sum(1 for a in audit if a["supports"] is None)
dup_total = sum(a["dup"] for a in audit)
miss_total = sum(a["missing"] for a in audit)
passed = sum(1 for a in audit if a["status"] == "PASS")
failed = sum(1 for a in audit if a["status"] == "FAIL")

lines = []
lines.append("# 分页一致性审计报告")
lines.append("")
lines.append(f"> 生成: {__import__('time').strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
lines.append(f"**测了 {len(audit)} 个列表端点**")
lines.append(f"- 支持分页: {supported} / 不支持: {unsupported} / 无法判定(数据<=5): {unknown}")
lines.append(f"- 重/漏: dup_total={dup_total}, missing_total={miss_total}")
lines.append(f"- PASS: {passed}, FAIL: {failed}")
lines.append("")
lines.append("| endpoint | N | collected | dedup | dup | missing | supports | status |")
lines.append("|---|---|---|---|---|---|---|---|")
for a in audit:
    lines.append(f"| {a['endpoint']} | {a['N']} | {a['collected']} | {a['dedup']} | {a['dup']} | {a['missing']} | {a['supports']} | {a['status']} |")
lines.append("")
for a in audit:
    lines.append(f"## {a['endpoint']} — {a['status']}")
    lines.append("")
    lines.append(f"- N={a['N']} collected={a['collected']} dedup={a['dedup']} dup={a['dup']} missing={a['missing']} supports={a['supports']}")
    lines.append("- boundary:")
    for b in a["boundary"]:
        lines.append(f"  - {b}")
    lines.append("")

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"\n测了 {len(audit)} 端点, 支持分页 {supported}, 不支持 {unsupported}, 无法判定 {unknown}")
print(f"重/漏: dup={dup_total}, missing={miss_total}")
print(f"PASS={passed} FAIL={failed}")
print(f"报告: {DOC}")
