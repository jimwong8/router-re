#!/usr/bin/env python3
"""backup_restore_test.py — 备份/恢复/导出功能验证。

真实打本机 9090，验证 /api/v1/config/export → /api/v1/config/restore 链路。
产出 docs/backup-restore-test.md（脚本生成）。
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
DOC = ROOT / "docs" / "backup-restore-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def _is_json(s):
    try:
        json.loads(s)
        return True
    except Exception:
        return False


def short(x, n=300):
    s = x if isinstance(x, str) else json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n] + "..."


results = []


def add(name, status, detail="", snippet=""):
    results.append({"name": name, "status": status, "detail": detail, "snippet": snippet})
    print(f"{status} {name}: {detail}")


def call(method, path, token=None, body=None, timeout=30):
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


token = login()

# ---------- 1. 导出备份 ----------
st, bundle = call("GET", "/api/v1/config/export", token=token)
# 保留原始响应字节，用于恢复时原样发送（避免 JSON 重序列化改变 section 字节导致 checksum 失配）
raw_export = None
try:
    req = urllib.request.Request(BASE + "/api/v1/config/export", headers={"Authorization": f"Bearer {token}"})
    with urllib.request.urlopen(req, timeout=30) as resp:
        raw_export = resp.read().decode("utf-8")
except Exception:
    raw_export = json.dumps(bundle, ensure_ascii=False) if bundle is not None else None

export_ok = (st == 200 and isinstance(bundle, dict))
sections = bundle.get("sections", {}) if isinstance(bundle, dict) else {}
has_keys = len(sections) > 0

add("1. GET /api/v1/config/export",
    "PASS" if (export_ok and has_keys) else "FAIL",
    f"HTTP {st}, sections={len(sections)}, has_keys={has_keys}",
    short({"sections_count": len(sections), "sample_keys": list(sections.keys())[:8]}))

# 保存备份到文件（原始字节）
backup_path = Path("/tmp/scratchpad/router-re-backup.json")
if raw_export:
    backup_path.parent.mkdir(parents=True, exist_ok=True)
    backup_path.write_text(raw_export, encoding="utf-8")

# ---------- 2. 备份结构完整性 ----------
struct_ok = export_ok and len(sections) > 0 and all(isinstance(k, str) and "." in k for k in sections.keys())
valid_json = all(isinstance(v, str) and (v == "" or _is_json(v)) for v in sections.values()) if sections else False
add("2. 备份结构完整性 (JSON合法/非空/含section.option键)",
    "PASS" if (struct_ok and valid_json) else "FAIL",
    f"sections={len(sections)}, all_keys_have_dot={all('.' in k for k in sections.keys())}, valid_json={valid_json}",
    short(list(sections.keys())[:12]))

# ---------- 3. 修改一项配置 (ntp enabled) ----------
st0, ntp0 = call("GET", "/api/v1/ntp/config", token=token)
orig_enabled = ntp0.get("config", {}).get("enabled", False) if isinstance(ntp0, dict) else False
new_enabled = not orig_enabled
st, _ = call("POST", "/api/v1/ntp/config", token=token, body={"servers": None, "enabled": new_enabled})
change_ok = (st == 200)
stv, ntpv = call("GET", "/api/v1/ntp/config", token=token)
changed_verified = isinstance(ntpv, dict) and ntpv.get("config", {}).get("enabled") == new_enabled
add("3. 修改配置 (ntp enabled -> %s)" % new_enabled,
    "PASS" if (change_ok and changed_verified) else "FAIL",
    f"POST={st} verify_GET={stv} enabled_now={ntpv.get('config', {}).get('enabled') if isinstance(ntpv, dict) else None}",
    short(ntpv))

# ---------- 4. 从备份恢复 (原样发送，保留 checksum) ----------
restore_body = '{"confirm":true,"bundle":' + (raw_export or json.dumps(bundle)) + '}'
req = urllib.request.Request(BASE + "/api/v1/config/restore", data=restore_body.encode(), method="POST")
req.add_header("Authorization", f"Bearer {token}")
req.add_header("Content-Type", "application/json")
try:
    with urllib.request.urlopen(req, timeout=30) as resp:
        st = resp.status
        rbody = json.loads(resp.read().decode())
except urllib.error.HTTPError as exc:
    st = exc.code
    try:
        rbody = json.loads(exc.read().decode())
    except Exception:
        rbody = exc.read().decode()
restore_ok = (st == 200 and isinstance(rbody, dict))
restored_list = rbody.get("restored", []) if isinstance(rbody, dict) else []
add("4. POST /api/v1/config/restore (confirm=true)",
    "PASS" if (restore_ok and len(restored_list) > 0) else "FAIL",
    f"HTTP {st} restored={len(restored_list)}",
    short(rbody))

# ---------- 5. 恢复后重新导出对比 ----------
st2, bundle2 = call("GET", "/api/v1/config/export", token=token)
sections2 = bundle2.get("sections", {}) if isinstance(bundle2, dict) else {}
stv2, ntpv2 = call("GET", "/api/v1/ntp/config", token=token)
restored_ntp = isinstance(ntpv2, dict) and ntpv2.get("config", {}).get("enabled") == orig_enabled
sections_equal = (sections == sections2)
add("5. 恢复后重导出对比",
    "PASS" if (restored_ntp and sections_equal) else "FAIL",
    f"ntp restored to orig={restored_ntp} (orig={orig_enabled}), sections_equal={sections_equal}",
    short({"orig_enabled": orig_enabled, "after_enabled": ntpv2.get('config', {}).get('enabled') if isinstance(ntpv2, dict) else None}))

# ---------- 6. preview 差异端点 (原样发送 bundle) ----------
preview_body = raw_export or json.dumps(bundle)
reqp = urllib.request.Request(BASE + "/api/v1/config/preview", data=preview_body.encode(), method="POST")
reqp.add_header("Authorization", f"Bearer {token}")
reqp.add_header("Content-Type", "application/json")
try:
    with urllib.request.urlopen(reqp, timeout=30) as resp:
        stp = resp.status
        plan = json.loads(resp.read().decode())
except urllib.error.HTTPError as exc:
    stp = exc.code
    try:
        plan = json.loads(exc.read().decode())
    except Exception:
        plan = exc.read().decode()
preview_ok = (stp == 200)
add("6. POST /api/v1/config/preview",
    "PASS" if preview_ok else "FAIL",
    f"HTTP {stp}",
    short(plan))

# ---------- 写报告 ----------
lines = []
lines.append("# 备份/恢复/导出功能验证报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
passed = sum(1 for r in results if r["status"] == "PASS")
failed = sum(1 for r in results if r["status"] == "FAIL")
skipped = sum(1 for r in results if r["status"] == "SKIP")
lines.append(f"**汇总**: {len(results)} 项 / PASS {passed} / FAIL {failed} / SKIP {skipped}")
lines.append("")
lines.append("有无备份端点: 是（/api/v1/config/export、/api/v1/config/preview、/api/v1/config/restore）")
lines.append(f"备份文件: {backup_path}")
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

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"\n汇总: {len(results)} 项 / PASS {passed} / FAIL {failed} / SKIP {skipped}")
print(f"报告: {DOC}")
sys.exit(0 if failed == 0 else 1)
