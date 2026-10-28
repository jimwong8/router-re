#!/usr/bin/env python3
"""portal_lifecycle_test.py — 认证门户会话管理端点生命周期测试 (p49).

串行：clients 初始 → authorize → clients 含 mac → revoke → clients 消失 →
config 读 → config 写+读回 → 负向 authorize 空 mac。

真实打本机 9090。applied=false（无 nft 权限）属于设计允许，不算失败。

用法:
  python3 scripts/portal_lifecycle_test.py
  BASE=http://127.0.0.1:9090 python3 scripts/portal_lifecycle_test.py

报告写到 docs/portal-lifecycle-test.md（由脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "portal-lifecycle-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
TEST_MAC = "aa:bb:cc:dd:ee:ff"
TEST_IP = "192.168.9.9"


def call(method, path, token=None, body=None, timeout=15):
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
    status, body, _, _ = call("POST", "/api/v1/auth/login",
                              body={"username": user, "password": pwd})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body}")
        sys.exit(1)
    if isinstance(body, dict):
        return body.get("token")
    return json.loads(body)["token"]


def snip(raw, n=260):
    s = (raw or "").replace("\n", " ")
    return s if len(s) <= n else s[:n] + "..."


def clients_list(token):
    st, data, _, raw = call("GET", "/api/v1/portal/clients", token=token)
    macs = []
    if isinstance(data, dict):
        for c in (data.get("clients") or []):
            if isinstance(c, dict):
                macs.append(c.get("mac"))
    cnt = data.get("count") if isinstance(data, dict) else None
    if cnt is None:
        cnt = len(macs)
    return st, data, macs, cnt, raw


def main():
    token = login()
    results = []

    # setup: 清理可能残留的测试 mac，保证初始状态干净
    call("POST", "/api/v1/portal/revoke", token=token,
         body={"mac": TEST_MAC})

    # ---------- Step 1: 初始 clients count ----------
    st, data, macs, cnt0, raw = clients_list(token)
    snippet1 = snip(raw)
    ok = (st == 200)
    results.append(("1. GET portal/clients 初始 count", "PASS" if ok else "FAIL",
                    f"HTTP {st} initial_count={cnt0} macs={macs}",
                    snippet1))

    # ---------- Step 2: authorize ----------
    st, data, _, raw = call("POST", "/api/v1/portal/authorize", token=token,
                            body={"mac": TEST_MAC, "ip": TEST_IP, "minutes": 30})
    snippet2 = snip(raw)
    client_ok = (st == 200 and isinstance(data, dict)
                 and isinstance(data.get("client"), dict))
    applied = data.get("applied") if isinstance(data, dict) else None
    results.append(("2. POST portal/authorize", "PASS" if client_ok else "FAIL",
                    f"HTTP {st} applied={applied} "
                    f"client={data.get('client') if isinstance(data, dict) else data}",
                    snippet2))

    # ---------- Step 3: clients 含该 mac，count 增加 ----------
    st, data, macs, cnt1, raw = clients_list(token)
    snippet3 = snip(raw)
    present = TEST_MAC in macs
    increased = (cnt1 == cnt0 + 1)
    ok = (st == 200 and present and increased)
    results.append(("3. GET portal/clients 含 mac/count+1", "PASS" if ok else "FAIL",
                    f"HTTP {st} count={cnt1} (init {cnt0}) present={present} "
                    f"increased={increased}",
                    snippet3))

    # ---------- Step 4: revoke ----------
    st, data, _, raw = call("POST", "/api/v1/portal/revoke", token=token,
                            body={"mac": TEST_MAC})
    snippet4 = snip(raw)
    revoked = (st == 200 and isinstance(data, dict)
               and data.get("revoked") is True)
    results.append(("4. POST portal/revoke", "PASS" if revoked else "FAIL",
                    f"HTTP {st} revoked={data.get('revoked') if isinstance(data, dict) else data}",
                    snippet4))

    # ---------- Step 5: clients 该 mac 消失，count 回落 ----------
    st, data, macs, cnt2, raw = clients_list(token)
    snippet5 = snip(raw)
    gone = TEST_MAC not in macs
    restored = (cnt2 == cnt0)
    ok = (st == 200 and gone and restored)
    results.append(("5. GET portal/clients mac消失/count恢复", "PASS" if ok else "FAIL",
                    f"HTTP {st} count={cnt2} (init {cnt0}) gone={gone} "
                    f"restored={restored}",
                    snippet5))

    # ---------- Step 6: config 读 ----------
    st, data, _, raw = call("GET", "/api/v1/portal/config", token=token)
    snippet6 = snip(raw)
    has_fields = (st == 200 and isinstance(data, dict)
                  and all(k in data for k in ("enabled", "portal_port",
                                             "default_minutes")))
    results.append(("6. GET portal/config 字段", "PASS" if has_fields else "FAIL",
                    f"HTTP {st} fields={list(data.keys()) if isinstance(data, dict) else data}",
                    snippet6))

    # ---------- Step 7: config 写+读回 ----------
    st, data, _, raw = call("POST", "/api/v1/portal/config", token=token,
                            body={"enabled": True, "portal_port": 8080,
                                  "default_minutes": 45})
    snippet7w = snip(raw)
    write_ok = (st == 200 and isinstance(data, dict)
                and data.get("enabled") is True
                and data.get("default_minutes") == 45)
    # 读回验证
    st2, d2, _, raw2 = call("GET", "/api/v1/portal/config", token=token)
    snippet7r = snip(raw2)
    readback_ok = (st2 == 200 and isinstance(d2, dict)
                   and d2.get("enabled") is True
                   and d2.get("default_minutes") == 45)
    ok = write_ok and readback_ok
    results.append(("7. POST portal/config 写+读回", "PASS" if ok else "FAIL",
                    f"write HTTP {st} (enabled={data.get('enabled') if isinstance(data, dict) else data}, "
                    f"minutes={data.get('default_minutes') if isinstance(data, dict) else data}); "
                    f"readback HTTP {st2} (enabled={d2.get('enabled') if isinstance(d2, dict) else d2}, "
                    f"minutes={d2.get('default_minutes') if isinstance(d2, dict) else d2})",
                    snippet7r))

    # ---------- Step 8: 负向 authorize 空 mac ----------
    st, data, _, raw = call("POST", "/api/v1/portal/authorize", token=token,
                            body={"mac": "", "ip": TEST_IP, "minutes": 30})
    snippet8 = snip(raw)
    code = data.get("code") if isinstance(data, dict) else None
    neg_ok = (st == 400 and str(code) == "10048")
    results.append(("8. 负向 authorize 空 mac → 400/10048",
                    "PASS" if neg_ok else "FAIL",
                    f"HTTP {st} code={code} (expect 400 / 10048)",
                    snippet8))

    # ---------- 报告 ----------
    passes = sum(1 for r in results if r[1] == "PASS")
    fails = sum(1 for r in results if r[1] == "FAIL")
    skips = sum(1 for r in results if r[1] == "SKIP")

    lines = []
    lines.append("# 认证门户会话管理生命周期测试报告 (portal-lifecycle-test)")
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
    lines.append("- applied=false 表示本机无 nft 权限，属设计允许（client 已记录），不算失败。")
    lines.append("- Step 7 将 default_minutes 设为 45 并读回验证。")
    lines.append("")
    DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")

    for step, st, detail, _ in results:
        print(f"{st:<4} {step} — {detail}")
    print(f"\n汇总: {len(results)} 步 / PASS {passes} / FAIL {fails} / SKIP {skips}")
    print(f"报告: {DOC}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
