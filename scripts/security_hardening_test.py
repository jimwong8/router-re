#!/usr/bin/env python3
"""security_hardening_test.py — 安全加固测试（认证/授权/注入/CSRF/越权）。

真实打本机 9090，串行走完检查项，产出 docs/security-hardening-test.md（脚本生成）。
"""
from __future__ import annotations

import http.client
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "security-hardening-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
HOST = "127.0.0.1"
PORT = 9090

results = []  # list of dicts: name, status (PASS/FAIL/SKIP), detail, snippet


def add(name, status, detail="", snippet=""):
    results.append({"name": name, "status": status, "detail": detail, "snippet": snippet})
    print(f"{status} {name}: {detail}")


def http_raw(method, path, headers=None, body=None, timeout=20):
    """Return (status, body_str) using http.client for raw path control."""
    conn = http.client.HTTPConnection(HOST, PORT, timeout=timeout)
    try:
        h = dict(headers or {})
        data = None
        if body is not None:
            data = body if isinstance(body, (bytes, str)) else json.dumps(body)
            if isinstance(data, str):
                data = data.encode()
            h.setdefault("Content-Type", "application/json")
        conn.request(method, path, body=data, headers=h)
        resp = conn.getresponse()
        raw = resp.read().decode("utf-8", "replace")
        return resp.status, raw
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}"
    finally:
        conn.close()


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


def login(user="admin", pwd="admin123"):
    st, body = call("POST", "/api/v1/auth/login", body={"username": user, "password": pwd})
    if st != 200 or not isinstance(body, dict) or "token" not in body:
        return None
    return body["token"]


def short(x, n=200):
    s = x if isinstance(x, str) else json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n] + "..."


# ---------- 0. 登录 ----------
token = login()
if not token:
    print("管理员登录失败，终止")
    sys.exit(1)

# ---------- 1. 无 token 访问敏感端点 ----------
sensitive = [
    ("GET", "/api/v1/users"),
    ("GET", "/api/v1/config/export"),
    ("POST", "/api/v1/firmware/apply"),
    ("GET", "/api/v1/ddns/config"),
    ("GET", "/api/v1/dns/config"),
    ("POST", "/api/v1/diag/ping"),
    ("GET", "/api/v1/system/services"),
    ("GET", "/api/v1/logs"),
]
no_token_ok = True
details = []
for method, path in sensitive:
    st, body = http_raw(method, path)
    ok = (st == 401)
    no_token_ok = no_token_ok and ok
    details.append(f"{method} {path} -> {st}")
add("1. 无 token 访问敏感端点",
    "PASS" if no_token_ok else "FAIL",
    "; ".join(details),
    short(details))

# ---------- 2. 篡改 token ----------
bad_token = token[:-1] + ("0" if token[-1] != "0" else "1")
tampered_ok = True
tdetails = []
for method, path in [("GET", "/api/v1/users"), ("GET", "/api/v1/logs")]:
    st, body = http_raw(method, path, headers={"Authorization": f"Bearer {bad_token}"})
    ok = (st == 401)
    tampered_ok = tampered_ok and ok
    tdetails.append(f"{method} {path} -> {st}")
add("2. 篡改 token", "PASS" if tampered_ok else "FAIL",
    "; ".join(tdetails), short(tdetails))

# ---------- 3. 路径穿越 ----------
traversal_paths = ["/api/v1/../etc/passwd", "/api/v1/..%2f..%2fetc%2fpasswd"]
trav_ok = True
td = []
for p in traversal_paths:
    st, body = http_raw("GET", p)
    leak = (st == 200) and ("root:" in body or "passwd" in body.lower())
    trav_ok = trav_ok and (not leak) and (st != 200 or "root:" not in body)
    td.append(f"GET {p} -> {st} leak={leak}")
add("3. 路径穿越", "PASS" if trav_ok else "FAIL",
    "; ".join(td), short(td))

# ---------- 4. 命令注入 ----------
inj_payloads = ["127.0.0.1;id", "$(id)", "127.0.0.1 | id", "127.0.0.1; cat /etc/passwd"]
inj_ok = True
idetails = []
for payload in inj_payloads:
    # diag/ping
    st, body = call("POST", "/api/v1/diag/ping", token=token, body={"host": payload, "count": 1})
    leaked = isinstance(body, str) and "uid=" in body
    if isinstance(body, dict):
        leaked = "uid=" in json.dumps(body)
    inj_ok = inj_ok and not leaked
    idetails.append(f"ping host={payload!r} -> {st} uid_leak={leaked}")
    # diag/dns
    st2, body2 = call("POST", "/api/v1/diag/dns", token=token, body={"name": payload})
    leaked2 = (isinstance(body2, str) and "uid=" in body2) or (isinstance(body2, dict) and "uid=" in json.dumps(body2))
    inj_ok = inj_ok and not leaked2
    idetails.append(f"dns name={payload!r} -> {st2} uid_leak={leaked2}")
add("4. 命令注入", "PASS" if inj_ok else "FAIL",
    "; ".join(idetails), short(idetails))

# ---------- 5. 越权 ----------
# 检查现有用户角色
st, ubody = call("GET", "/api/v1/users", token=token)
roles = set()
if isinstance(ubody, dict):
    for u in ubody.get("users", []):
        roles.add(u.get("role"))
has_multi = len(roles) > 1 or True  # rbac 支持多角色
# 创建一个 viewer 用户测试越权
test_user = "sectest_viewer"
test_pwd = "SecTest#2025!"
# 先尝试删除残留
call("DELETE", f"/api/v1/users/{test_user}", token=token)
stc, cbody = call("POST", "/api/v1/users", token=token, body={
    "username": test_user, "password": test_pwd, "role": "viewer"})
created = (stc in (200, 201))
viewer_token = login(test_user, test_pwd) if created else None
priv_ok = True
pdetails = []
if viewer_token:
    # viewer 访问 admin-only 端点应被拒
    st, _ = call("GET", "/api/v1/users", token=viewer_token)
    rejected = st in (401, 403)
    priv_ok = priv_ok and rejected
    pdetails.append(f"viewer GET /users -> {st} (expect 401/403) rejected={rejected}")
    # viewer 调用 create user 应被拒
    st2, _ = call("POST", "/api/v1/users", token=viewer_token, body={
        "username": "x", "password": "Abcdef1!xx", "role": "viewer"})
    rejected2 = st2 in (401, 403)
    priv_ok = priv_ok and rejected2
    pdetails.append(f"viewer POST /users -> {st2} (expect 401/403) rejected={rejected2}")
    # 清理
    call("DELETE", f"/api/v1/users/{test_user}", token=token)
else:
    if created is False and stc not in (200, 201):
        pdetails.append(f"create viewer failed HTTP {stc}: {short(cbody)}")
    priv_ok = False
add("5. 越权 (viewer→admin 端点)", "PASS" if priv_ok else "FAIL",
    "; ".join(pdetails) if pdetails else f"roles={roles}", short(pdetails or roles))

# ---------- 6. 畸形输入 ----------
mal_ok = True
mdetails = []
big = "A" * 100000
# 6a 超长字符串
st, body = call("POST", "/api/v1/diag/ping", token=token, body={"host": big, "count": 1})
no500 = st != 500
mal_ok = mal_ok and no500
mdetails.append(f"ping 100KB host -> {st} (not 500: {no500})")
# 6b null body
st, body = call("POST", "/api/v1/diag/ping", token=token, body=None)
# send raw null
st2, raw2 = http_raw("POST", "/api/v1/diag/ping",
                      headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
                      body="null")
no500b = st2 != 500
mal_ok = mal_ok and no500b
mdetails.append(f"ping body=null -> {st2} (not 500: {no500b})")
# 6c 数组代替对象
st3, raw3 = http_raw("POST", "/api/v1/diag/ping",
                      headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
                      body="[1,2,3]")
no500c = st3 != 500
mal_ok = mal_ok and no500c
mdetails.append(f"ping body=array -> {st3} (not 500: {no500c})")
# 6d dns resolve null
st4, raw4 = http_raw("POST", "/api/v1/dns/resolve",
                      headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
                      body="null")
no500d = st4 != 500
mal_ok = mal_ok and no500d
mdetails.append(f"dns/resolve body=null -> {st4} (not 500: {no500d})")
add("6. 畸形输入 (超长/null/数组)", "PASS" if mal_ok else "FAIL",
    "; ".join(mdetails), short(mdetails))

# ---------- 7. CSRF 提示 (无 Origin/Referer 的 POST 是否被拒?) ----------
# 本系统用 Bearer token，无 cookie 会话，CSRF 风险低；记录为 N/A 说明
st, _ = http_raw("POST", "/api/v1/diag/ping",
                  headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
                  body=json.dumps({"host": "127.0.0.1", "count": 1}))
csrf_note = "Bearer token 认证、无 cookie 会话，CSRF 不适用 (N/A)"
add("7. CSRF", "SKIP", csrf_note, "")

# ---------- 写报告 ----------
lines = []
lines.append("# 安全加固测试报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
passed = sum(1 for r in results if r["status"] == "PASS")
failed = sum(1 for r in results if r["status"] == "FAIL")
skipped = sum(1 for r in results if r["status"] == "SKIP")
lines.append(f"**汇总**: {len(results)} 项 / PASS {passed} / FAIL {failed} / SKIP {skipped}")
lines.append("")
lines.append("| # | 检查项 | 结果 |")
lines.append("|---|---|---|")
for i, r in enumerate(results, 1):
    lines.append(f"| {i} | {r['name']} | {r['status']} |")
lines.append("")
for i, r in enumerate(results, 1):
    lines.append(f"## {i}. {r['name']} — {r['status']}")
    lines.append("")
    if r["detail"]:
        lines.append(f"- 详情: {r['detail']}")
    if r["snippet"]:
        lines.append("")
        lines.append("```")
        lines.append(r["snippet"])
        lines.append("```")
    lines.append("")
# 严重漏洞判定
critical = []
if any(r["status"] == "FAIL" and "注入" in r["name"] for r in results):
    critical.append("命令注入")
if any(r["status"] == "FAIL" and "穿越" in r["name"] for r in results):
    critical.append("路径穿越")
if any(r["status"] == "FAIL" and "无 token" in r["name"] for r in results):
    critical.append("未授权访问")
lines.append("## 严重漏洞")
lines.append("")
if critical:
    lines.append("CRITICAL: " + ", ".join(critical))
else:
    lines.append("无 CRITICAL 级别漏洞发现。")
lines.append("")

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"\n汇总: {len(results)} 项 / PASS {passed} / FAIL {failed} / SKIP {skipped}")
print(f"报告: {DOC}")
sys.exit(0 if failed == 0 else 1)
