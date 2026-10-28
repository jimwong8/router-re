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
        req.add_header("Authorization", f"Bearer {token}")
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
    except Exception as exc:  # noqa: BLE001
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
    """Return (list_field_name, list_value, total) for a response dict."""
    if not isinstance(obj, dict):
        return None
    # find first list value
    list_key = None
    list_val = None
    for k, v in obj.items():
        if isinstance(v, list):
            list_key = k
            list_val = v
            break
    if list_val is None:
        return None
    total = obj.get("total")
    if total is None:
        total = obj.get("count")
    if total is None:
        total = len(list_val)
    return list_key, list_val, int(total)


def item_key(item):
    try:
        return json.dumps(item, sort_keys=True, ensure_ascii=False)
    except Exception:
        return str(item)


token = login()

# 枚举 GET 端点
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

# 只测返回列表的端点
list_endpoints = []
for p in endpoints:
    st, body = call("GET", p + "?limit=5", token=token, timeout=15)
    if st != 200 or not isinstance(body, dict):
        continue
    info = find_list(body)
    if info is None:
        continue
    _, lst, total = info
    if len(lst) == 0 and total == 0:
        # could be empty list endpoint; still consider if key present
        pass
    list_endpoints.append(p)

print(f"检测到 {len(list_endpoints)} 个列表端点")

# 审计每个
audit = []
for p in list_endpoints:
    # full fetch
    st, body = call("GET", p + "?limit=10000", token=token, timeout=30)
    info = find_list(body) if isinstance(body, dict) else None
    if info is None:
        audit.append({"endpoint": p, "supported": False, "N": 0, "dedup": 0,
                     "dup": 0, "missing": 0, "boundary": "no-list", "status": "SKIP"})
        add(p, "SKIP", "非列表响应")
        continue
    key, full_list, total = info
    N = total if total and total > 0 else len(full_list)
    if N == 0:
        N = len(full_list)

    # paginate by 5
    collected = []
    page = 0
    seen = set()
    dup_count = 0
    while True:
        st, body = call("GET", f"{p}?limit=5&offset={page*5}", token=token, timeout=15)
        if st != 200 or not isinstance(body, dict):
            break
        info2 = find_list(body)
        if info2 is None:
            break
        _, lst, _ = info2
        if not lst:
            break
        for item in lst:
            k = item_key(item)
            if k in seen:
                dup_count += 1
            else:
                seen.add(k)
            collected.append(item)
        if len(lst) < 5:
            break
        page += 1
        if page > 1000:
            break

    dedup_count = len(seen)
    missing = N - dedup_count if N >= dedup_count else 0
    # supports pagination? check if second page request returned fewer/different
    st1, body1 = call("GET", f"{p}?limit=5&offset=0", token=token, timeout=15)
    st2, body2 = call("GET", f"{p}?limit=5&offset=5", token=token, timeout=15)
    info1 = find_list(body1) if isinstance(body1, dict) else None
    info2 = find_list(body2) if isinstance(body2, dict) else None
    len1 = len(info1[1]) if info1 else 0
    len2 = len(info2[1]) if info2 else 0
    # supports pagination if offset=5 returns different items or empty (not identical full list)
    supports = True
    if info1 and info2:
        keys1 = [item_key(x) for x in info1[1]]
        keys2 = [item_key(x) for x in info2[1]]
        if keys1 == keys2 and len(keys1) > 0:
            supports = False
    if N <= 5:
        supports = None  # can't tell

    # boundary tests
    b_results = []
    # offset=0 limit=0
    st, body = call("GET", f"{p}?limit=0&offset=0", token=token, timeout=15)
    b_results.append(f"limit=0 -> HTTP {st} items={len(find_list(body)[1]) if (isinstance(body,dict) and find_list(body)) else 0}")
    # offset beyond total
    st, body = call("GET", f"{p}?limit=5&offset={N+100}", token=token, timeout=15)
    lst_b = find_list(body)[1] if (isinstance(body, dict) and find_list(body)) else []
    b_results.append(f"offset>N -> HTTP {st} items={len(lst_b)}")
    # limit huge
    st, body = call("GET", f"{p}?limit=100000&offset=0", token=token, timeout=30)
    lst_h = find_list(body)[1] if (isinstance(body, dict) and find_list(body)) else []
    b_results.append(f"limit=100000 -> HTTP {st} items={len(lst_h)} (N={N})")

    ok = (dedup_count == N and dup_count == 0)
    status = "PASS" if ok else "FAIL"
    if supports is False:
        status = "NOPAGE"
    detail = (f"N={N} collected={len(collected)} dedup={dedup_count} dup={dup_count} "
              f"missing={missing} supports_pagination={supports}")
    add(p, status, detail, short({"boundary": b_results, "collected_sample": collected[:2]}))
    audit.append({"endpoint": p, "N": N, "collected": len(collected), "dedup": dedup_count,
                  "dup": dup_count, "missing": missing, "supports": supports,
                  "boundary": b_results, "status": status})

# summary
supported = sum(1 for a in audit if a["supports"] is True)
unsupported = sum(1 for a in audit if a["supports"] is False)
unknown = sum(1 for a in audit if a["supports"] is None)
dup_total = sum(a["dup"] for a in audit)
miss_total = sum(a["missing"] for a in audit)
passed = sum(1 for a in audit if a["status"] == "PASS")
failed = sum(1 for a in audit if a["status"] == "FAIL")

# write report
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
