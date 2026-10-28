#!/usr/bin/env python3
"""audit_log_verify.py — 验证审计日志真的落盘。

产出 docs/audit-log-verify.md（脚本写，勿手写）。
"""
from __future__ import annotations
import json, os, subprocess, time, urllib.error, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "audit-log-verify.md"

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
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace")
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}"

# ---------- 1. 登录 ----------
login_status, login_body = call("POST", "/api/v1/auth/login",
                               body={"username": "admin", "password": "admin123"})
token = json.loads(login_body)["token"] if login_status == 200 else None
assert token, "login failed"

# ---------- 2. 找 audit 日志文件 ----------
# 从进程环境或默认路径
audit_path = None
candidates = ["/var/log/router-re/audit.jsonl", str(ROOT / "audit.jsonl"), "/tmp/audit.jsonl"]
# 也查 MGM_AUDIT_LOG
try:
    pid = subprocess.check_output(["pgrep", "-f", "./bin/mgmtd-cgo"], text=True).strip().split()[0]
    env_raw = open(f"/proc/{pid}/environ", "rb").read()
    for kv in env_raw.split(b"\x00"):
        if kv.startswith(b"MGM_AUDIT_LOG="):
            audit_path = kv.decode().split("=", 1)[1]
except Exception:
    pass

if audit_path is None:
    for c in candidates:
        if os.path.exists(c):
            audit_path = c
            break
if audit_path is None:
    audit_path = candidates[0]

# ---------- 3. 记录当前行数 ----------
def count_lines(p):
    try:
        with open(p) as f:
            return sum(1 for _ in f)
    except FileNotFoundError:
        return 0

def tail_lines(p, n=3):
    try:
        with open(p) as f:
            lines = f.readlines()
        return [l.rstrip() for l in lines[-n:]]
    except FileNotFoundError:
        return []

before_count = count_lines(audit_path)
before_tail = tail_lines(audit_path, 1)

# ---------- 4. 触发 auditWrap 写端点 ----------
# 用 ddns/config（action=ddns.config.set），body 最小
trigger_action = "ddns.config.set"
trigger_status, trigger_body = call("POST", "/api/v1/ddns/config", token=token,
                                    body={"enabled": False})
time.sleep(0.5)  # 等写入落盘

after_write_count = count_lines(audit_path)
new_lines_write = after_write_count - before_count
write_tail = tail_lines(audit_path, 1)

# 解析新行
new_action_ok = False
new_has_actor = False
new_has_ts = False
new_line_sample = ""
if new_lines_write > 0 and write_tail:
    new_line_sample = write_tail[-1]
    try:
        ev = json.loads(new_line_sample)
        new_action_ok = ev.get("action") == trigger_action
        new_has_actor = ev.get("actor") in ("admin", "-")
        new_has_ts = isinstance(ev.get("ts"), int) and ev["ts"] > 0
    except json.JSONDecodeError:
        pass

# ---------- 5. 触发读端点，断言行数不变 ----------
read_before = count_lines(audit_path)
read_status, read_body = call("GET", "/api/v1/system/info", token=token)
time.sleep(0.5)
read_after = count_lines(audit_path)
read_unchanged = (read_after == read_before)

# ---------- 6. 脱敏 ----------
def redact(s):
    # 脱敏 IP（保留 localhost）
    import re
    s = re.sub(r'\d+\.\d+\.\d+\.\d+', lambda m: m.group(0) if m.group(0) == "127.0.0.1" else "x.x.x.x", s)
    return s

# ---------- 7. 写报告 ----------
report = []
report.append("# Audit Log Verify")
report.append("")
report.append(f"- Base: {BASE}")
report.append(f"- Audit file: `{audit_path}`")
report.append(f"- Date: {__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
report.append("")
report.append("## 1. 文件定位")
report.append(f"- 路径: `{audit_path}`")
report.append(f"- 存在: {os.path.exists(audit_path)}")
report.append(f"- 初始行数: {before_count}")
report.append("")
report.append("## 2. 触发 auditWrap 写端点")
report.append(f"- 端点: POST /api/v1/ddns/config")
report.append(f"- action: `{trigger_action}`")
report.append(f"- HTTP 状态: {trigger_status}")
report.append(f"- 触发前行数: {before_count}")
report.append(f"- 触发后行数: {after_write_count}")
report.append(f"- 新增行数: {new_lines_write}")
report.append(f"- action 字段正确: {new_action_ok}")
report.append(f"- 含 actor: {new_has_actor}")
report.append(f"- 含 timestamp: {new_has_ts}")
report.append("")
report.append("### 新增日志行（脱敏）")
report.append("```")
report.append(redact(new_line_sample))
report.append("```")
report.append("")
report.append("## 3. 读端点不审计")
report.append(f"- 端点: GET /api/v1/system/info")
report.append(f"- HTTP 状态: {read_status}")
report.append(f"- 读前行数: {read_before}")
report.append(f"- 读后行数: {read_after}")
report.append(f"- 行数未变: {read_unchanged}")
report.append("")
report.append("## 4. 结论")
all_ok = new_lines_write > 0 and new_action_ok and new_has_ts and read_unchanged
report.append(f"- 审计日志落盘: {'PASS' if new_lines_write > 0 else 'FAIL'}")
report.append(f"- action 字段正确: {'PASS' if new_action_ok else 'FAIL'}")
report.append(f"- 读操作不审计: {'PASS' if read_unchanged else 'FAIL'}")
report.append(f"- 总体: {'PASS' if all_ok else 'FAIL'}")
report.append("")

OUT.write_text("\n".join(report), encoding="utf-8")
print("\n".join(report))