#!/usr/bin/env python3
"""portal_flow_test.py — 认证门户全流程端到端验证 (p47).

串行执行：模板保存 → voucher 生成 → portal 登录 → portal 状态 →
voucher 校验 → portal 登出。真实打本机 9090。缺失 handler 如实记 SKIP。

用法:
  python3 scripts/portal_flow_test.py
  BASE=http://127.0.0.1:9090 python3 scripts/portal_flow_test.py

报告写到 docs/portal-flow-test.md（由脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "portal-flow-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


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


def snip(raw, n=240):
    s = (raw or "").replace("\n", " ")
    return s if len(s) <= n else s[:n] + "..."


def probe(path):
    """探测路由是否存在：GET + POST 都 404 才算未注册。"""
    s_get, _, _, _ = call("GET", path)
    s_post, _, _, _ = call("POST", path, body={})
    if s_get == 404 and s_post in (404, 405):
        return False, f"GET={s_get} POST={s_post}"
    return True, f"GET={s_get} POST={s_post}"


def main():
    token = login()
    results = []   # (step, status, detail, snippet)
    generated = {}  # code/password

    # ---------- Step 1: 模板保存 ----------
    status, data, dt, raw = call("GET", "/api/v1/portal/templates", token=token)
    snippet1 = snip(raw)
    if status != 200 or not isinstance(data, dict):
        results.append(("1. 选/保存 portal 模板", "FAIL",
                        f"GET templates HTTP {status}", snippet1))
    else:
        builtin = data.get("builtin") or []
        voucher_tpl = None
        for t in builtin:
            if isinstance(t, dict) and t.get("kind") == "voucher":
                voucher_tpl = t
                break
        if voucher_tpl is None and builtin:
            voucher_tpl = builtin[0]
        rt = dict(voucher_tpl or {})
        rt["id"] = "roundtrip"
        rt["name"] = "roundtrip portal"
        st2, d2, _, raw2 = call("POST", "/api/v1/portal/template", token=token,
                                 body={"template": rt})
        snippet1b = snip(raw2)
        if st2 == 200 and isinstance(d2, dict) and d2.get("saved") is True:
            results.append(("1. 选/保存 portal 模板", "PASS",
                            f"GET templates {status} (count={data.get('count')}); "
                            f"POST template {st2} saved=true id=roundtrip",
                            snippet1b))
        else:
            results.append(("1. 选/保存 portal 模板", "FAIL",
                            f"POST template HTTP {st2}: {d2}", snippet1b))

    # ---------- Step 2: voucher 生成 ----------
    body = {"count": 1, "code_len": 10, "minutes": 30, "device_limit": 1,
            "note": "p47 roundtrip"}
    st, data, dt, raw = call("POST", "/api/v1/voucher/generate", token=token, body=body)
    snippet2 = snip(raw)
    if st == 200 and isinstance(data, dict) and data.get("generated") == 1:
        vs = data.get("vouchers") or []
        if vs and isinstance(vs[0], dict):
            generated["code"] = vs[0].get("code")
            generated["password"] = vs[0].get("password")
        results.append(("2. voucher/generate 测试卡", "PASS",
                        f"HTTP {st} generated={data.get('generated')} "
                        f"code={generated.get('code')}",
                        snippet2))
    else:
        results.append(("2. voucher/generate 测试卡", "FAIL",
                        f"HTTP {st}: {data}", snippet2))

    # ---------- Step 3: portal/login ----------
    # mgmtd 无 /api/v1/portal/login 路由（门户认证由 /portal/auth 表单承载，非 mgmtd API）
    reg, detail = probe("/api/v1/portal/login")
    st, data, _, raw = call("POST", "/api/v1/portal/login", token=token,
                            body={"credential": generated.get("code")})
    snippet3 = snip(raw)
    if not reg:
        results.append(("3. portal/login 用卡登录", "SKIP",
                        "路由未注册 (mgmtd 无 /api/v1/portal/login；门户认证走 "
                        "/portal/auth 表单，非 mgmtd REST API) "
                        f"probe={detail} HTTP {st}",
                        snippet3))
    else:
        results.append(("3. portal/login 用卡登录",
                        "PASS" if st == 200 else "FAIL",
                        f"HTTP {st}: {data}", snippet3))

    # ---------- Step 4: portal/status ----------
    reg, detail = probe("/api/v1/portal/status")
    st, data, _, raw = call("GET", "/api/v1/portal/status", token=token)
    snippet4 = snip(raw)
    if not reg:
        results.append(("4. portal/status 查会话", "SKIP",
                        "路由未注册 (mgmtd 无 /api/v1/portal/status) "
                        f"probe={detail} HTTP {st}",
                        snippet4))
    else:
        results.append(("4. portal/status 查会话",
                        "PASS" if st == 200 else "FAIL",
                        f"HTTP {st}: {data}", snippet4))

    # ---------- Step 5: voucher list/verify (确认卡状态已用) ----------
    # portal login 不可用，故直接调 redeem 端点标记已用，再 list 确认 Used=true
    code = generated.get("code")
    pw = generated.get("password")
    redeem_ok = False
    if code and pw:
        rst, rdata, _, rraw = call("POST", "/api/v1/voucher/redeem", token=token,
                                   body={"code": code, "password": pw})
        snippet5r = snip(rraw)
        redeem_ok = (rst == 200 and isinstance(rdata, dict)
                     and rdata.get("ok") is True)
    else:
        rst, rdata, _, rraw = 0, "no code/password", 0, ""
        snippet5r = "skipped (no generated card)"

    lst, ldata, _, lraw = call("GET", "/api/v1/voucher/list", token=token)
    snippet5 = snip(lraw)
    used_flag = None
    if isinstance(ldata, dict):
        for v in (ldata.get("vouchers") or []):
            if isinstance(v, dict) and v.get("code") == code:
                used_flag = v.get("used")
                break
    if lst == 200 and redeem_ok and used_flag is True:
        results.append(("5. voucher list/verify 确认已用", "PASS",
                        f"redeem HTTP {rst} ok; list HTTP {lst}; "
                        f"card {code} used={used_flag}",
                        snippet5))
    elif lst == 200 and not redeem_ok:
        results.append(("5. voucher list/verify 确认已用", "SKIP",
                        f"portal login 不可用，未触发 redeem；list HTTP {lst} "
                        f"card {code} used={used_flag}",
                        snippet5))
    else:
        results.append(("5. voucher list/verify 确认已用", "FAIL",
                        f"redeem HTTP {rst}; list HTTP {lst}; used={used_flag}",
                        snippet5))

    # ---------- Step 6: portal/logout ----------
    reg, detail = probe("/api/v1/portal/logout")
    st, data, _, raw = call("POST", "/api/v1/portal/logout", token=token, body={})
    snippet6 = snip(raw)
    if not reg:
        results.append(("6. portal/logout 登出+status消失", "SKIP",
                        "路由未注册 (mgmtd 无 /api/v1/portal/logout) "
                        f"probe={detail} HTTP {st}",
                        snippet6))
    else:
        # 再查 status 确认会话消失
        st2, d2, _, r2 = call("GET", "/api/v1/portal/status", token=token)
        gone = (st2 in (401, 404) or (isinstance(d2, dict)
                and not d2.get("online")))
        results.append(("6. portal/logout 登出+status消失",
                        "PASS" if (st == 200 and gone) else "FAIL",
                        f"logout HTTP {st}; status-after HTTP {st2}", snippet6))

    # ---------- 报告 ----------
    passes = sum(1 for r in results if r[1] == "PASS")
    fails = sum(1 for r in results if r[1] == "FAIL")
    skips = sum(1 for r in results if r[1] == "SKIP")

    lines = []
    lines.append("# 认证门户全流程测试报告 (portal-flow-test)")
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
    lines.append("- SKIP 步骤：mgmtd(9090) 未注册 /api/v1/portal/{login,status,logout} 路由；")
    lines.append("  门户认证由 RenderHTML 生成的表单 POST /portal/auth 承载，不在 mgmtd REST API 层。")
    lines.append("- Step 5 在 portal login 不可用时，直接调 /api/v1/voucher/redeem 标记已用并 list 确认。")
    lines.append("")
    DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")

    for step, st, detail, _ in results:
        print(f"{st:<4} {step} — {detail}")
    print(f"\n汇总: {len(results)} 步 / PASS {passes} / FAIL {fails} / SKIP {skips}")
    print(f"报告: {DOC}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
