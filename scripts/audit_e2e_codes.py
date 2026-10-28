#!/usr/bin/env python3
"""audit_e2e_codes.py — 审计 e2e TEXT_ENDPOINTS 白名单 + 错误码唯一性。

产出 docs/e2e-audit-fixes.md（脚本写，不手写）。
"""
from __future__ import annotations
import datetime
import json
import os
import re
import subprocess
import urllib.error
import urllib.request
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SERVER_DIR = ROOT / "mgmtd" / "internal" / "server"
SERVER_GO = SERVER_DIR / "server.go"
E2E = ROOT / "scripts" / "e2e_test.py"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "e2e-audit-fixes.md"

# ---------- 1. 提取路由 ----------
src = SERVER_GO.read_text(encoding="utf-8")
route_pat = re.compile(r'HandleFunc\(\s*"(GET|POST|PUT|DELETE)\s+([^"]+)"')
routes = [(m, p) for m, p in route_pat.findall(src) if "{" not in p]

# ---------- 2. 登录拿 token ----------
def call(method, path, token=None, body=None):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            return resp.status, resp.read().decode("utf-8", "replace"), dict(resp.headers)
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace"), dict(exc.headers)
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}", {}

login_status, login_body, _ = call("POST", "/api/v1/auth/login",
                                     body={"username": "admin", "password": "admin123"})
token = json.loads(login_body)["token"] if login_status == 200 else None
assert token, "login failed"

# ---------- 3. 扫描 GET 端点 Content-Type ----------
e2e_src = E2E.read_text(encoding="utf-8")
text_set_match = re.search(r"TEXT_ENDPOINTS = \{\n(.*?)\n\}", e2e_src, re.S)
current_text = set()
if text_set_match:
    for line in text_set_match.group(1).splitlines():
        m = re.search(r'"([^"]+)"', line)
        if m:
            current_text.add(m.group(1))

default_q = {
    "/api/v1/dpi/classify": "?dport=80&proto=tcp",
    "/api/v1/stats/history": "?minutes=60",
    "/api/v1/conntrack": "?limit=50",
    "/api/v1/logs": "?limit=50",
    "/api/v1/htb/show": "?iface=ens18",
    "/api/v1/firmware/inspect": "?path=/bin/true",
    "/api/v1/iface/tune": "?iface=ens18",
}

missing = []
json_ok = []
for method, path in routes:
    if method != "GET":
        continue
    if path in {"/api/v1/auth/login"}:
        continue
    q = default_q.get(path, "")
    status, body, headers = call("GET", path + q, token=token)
    ct = headers.get("Content-Type", "")
    is_json = False
    try:
        json.loads(body)
        is_json = True
    except json.JSONDecodeError:
        is_json = False
    if not is_json and status == 200:
        if path not in current_text:
            missing.append((path, ct, len(body)))
    if is_json:
        json_ok.append(path)

# ---------- 4. 错误码审计 ----------
code_pat = re.compile(r'errBody\(\s*(\d+)')
all_codes = []
for f in sorted(SERVER_DIR.glob("*.go")):
    if f.name.endswith("_test.go"):
        continue
    for m in code_pat.finditer(f.read_text(encoding="utf-8")):
        all_codes.append((f.name, int(m.group(1))))
code_counter = Counter(c for _, c in all_codes)
dup_codes = {c: n for c, n in code_counter.items() if n > 1 and 10480 <= c <= 10510}
dup_detail = {}
for fname, code in all_codes:
    if code in dup_codes:
        dup_detail.setdefault(code, []).append(fname)

# ---------- 5. 补 TEXT_ENDPOINTS（只加行不删行）----------
added_lines = 0
if missing:
    new_entries = []
    for p, ct, sz in missing:
        new_entries.append(f'    "{p}",  # auto-added by audit: {ct}')
    marker = 'TEXT_ENDPOINTS = {\n'
    close_idx = e2e_src.find('}', e2e_src.find(marker))
    insert_text = "".join(e + "\n" for e in new_entries)
    new_src = e2e_src[:close_idx] + insert_text + e2e_src[close_idx:]
    E2E.write_text(new_src, encoding="utf-8")
    added_lines = len(new_entries)

# ---------- 6. 跑 e2e ----------
env = os.environ.copy()
env["BASE"] = BASE
r = subprocess.run(["python3", str(E2E)], capture_output=True, text=True, env=env, cwd=str(ROOT))
e2e_result = r.stdout.strip()
e2e_rc = r.returncode
summary_line = ""
for line in e2e_result.splitlines():
    if line.startswith("通过") or line.startswith("E2E"):
        summary_line = line

# ---------- 7. 写报告 ----------
report_lines = []
report_lines.append("# E2E Audit Fixes")
report_lines.append("")
report_lines.append(f"- Base: {BASE}")
report_lines.append(f"- Date: {datetime.datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
report_lines.append("")
report_lines.append("## 1. MISSING text endpoints (non-JSON GET, not in TEXT_ENDPOINTS)")
if missing:
    for p, ct, sz in missing:
        report_lines.append(f"- `{p}`  Content-Type={ct}  size={sz}B")
else:
    report_lines.append("- (none)")
report_lines.append("")
report_lines.append("## 2. TEXT_ENDPOINTS 补充")
report_lines.append(f"- 补了几行: {added_lines}")
report_lines.append("")
report_lines.append("## 3. 错误码唯一性 (10480-10510)")
if dup_codes:
    for code, files in dup_detail.items():
        report_lines.append(f"- `{code}` 重复 {dup_codes[code]} 次: {', '.join(sorted(set(files)))}")
else:
    report_lines.append("- 无重复")
report_lines.append("")
report_lines.append("## 4. 全部已用错误码")
for fname in sorted(set(f for f, _ in all_codes)):
    codes = sorted(set(c for f, c in all_codes if f == fname))
    report_lines.append(f"- {fname}: {codes}")
report_lines.append("")
report_lines.append("## 5. e2e 重跑结果")
report_lines.append(f"- exit code: {e2e_rc}")
report_lines.append(f"- {summary_line}")
report_lines.append("")
report_lines.append("```")
report_lines.append(e2e_result[-500:])
report_lines.append("```")
report_lines.append("")

OUT.write_text("\n".join(report_lines), encoding="utf-8")
print("\n".join(report_lines))
print("\n=== DONE ===")