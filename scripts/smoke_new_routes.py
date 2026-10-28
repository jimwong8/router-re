#!/usr/bin/env python3
"""smoke_new_routes.py — 对 p18+ 新增路由做逐条冒烟验证。

每个路由断言「预期 HTTP 码 + 关键响应字段」，与泛覆盖 e2e_test.py 不同：
这里逐个核对新端点的行为，而不是只探 200。

用法:
  python3 scripts/smoke_new_routes.py        # 默认 http://127.0.0.1:9090
  BASE=... python3 scripts/smoke_new_routes.py

报告写到 docs/smoke-new-routes.md（由脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "smoke-new-routes.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")


def call(method: str, path: str, token: str | None = None, body: dict | None = None,
         timeout: int = 15) -> tuple[int, dict | str, float]:
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


def login() -> str:
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    status, body, _ = call("POST", "/api/v1/auth/login",
                           body={"username": user, "password": pwd})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body}")
        sys.exit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]


# 每条: (method, path, body_or_None, 预期码(可集合), 断言函数或None, 说明)
def has_fields(required: set):
    return lambda d: all(k in d for k in required), f"字段 {sorted(required)} 存在"


ROUTES = [
    # ---- p18b 认证门户 ----
    ("GET", "/api/v1/portal/templates", None, {200}, has_fields({"count", "builtin", "current"}), "门户模板列表"),
    ("GET", "/api/v1/portal/preview", None, {200}, None, "门户预览(HTML)"),
    ("POST", "/api/v1/portal/template", {}, {422}, None, "门户模板空body→422(Validate挡)"),
    # ---- p21 防火墙日志分析 ----
    ("GET", "/api/v1/firewall/log/analysis", None, {200},
     lambda d: ("summary" in d and "count" in d) if d.get("available") else ("reason" in d),
     "fwlog分析(可用→验summary/count，不可用→验reason)"),
    ("GET", "/api/v1/firewall/log/suggestions", None, {200}, lambda d: d.get("applied") is False, "fwlog建议(applied=false)"),
    # ---- p26 终端列表 ----
    ("GET", "/api/v1/terminals", None, {200}, has_fields({"count", "terminals"}), "终端聚合列表"),
    # ---- voucher 卡密 ----
    ("GET", "/api/v1/voucher/list", None, {200}, has_fields({"count", "vouchers"}), "卡密列表"),
    ("POST", "/api/v1/voucher/redeem", {"code": "", "password": ""}, {422}, None, "空卡密核销→422(防枚举统一错误)"),
    ("POST", "/api/v1/voucher/generate", {"count": 0}, {422}, None, "生成0张→422校验"),
    # ---- linkcheck 链路探测 ----
    ("GET", "/api/v1/linkcheck/targets", None, {200}, None, "探测目标列表"),
    ("GET", "/api/v1/linkcheck/status", None, {200}, None, "探测状态"),
    ("POST", "/api/v1/linkcheck/targets", {}, {422}, None, "空targets→422"),
    # ---- ddns ----
    ("GET", "/api/v1/ddns/records", None, {200}, None, "DDNS记录"),
    ("GET", "/api/v1/ddns/status", None, {200}, None, "DDNS状态"),
    ("GET", "/api/v1/ddns/ip", None, {200}, None, "DDNS当前IP"),
    ("GET", "/api/v1/ddns/check", None, {200}, None, "DDNS检查"),
    # ---- vpn/client ----
    ("GET", "/api/v1/vpn/client", None, {200}, None, "VPN客户端"),
    # ---- security ----
    ("GET", "/api/v1/security/arp/neighbors", None, {200}, None, "ARP邻居"),
    ("GET", "/api/v1/security/attack-guard", None, {200}, None, "攻击防护配置"),
    ("GET", "/api/v1/security/ddos", None, {200}, None, "DDoS配置"),
    # ---- timeschedule ----
    ("GET", "/api/v1/policy/timeschedule", None, {200}, None, "时间计划"),
    # ---- accounting / conntrack acct ----
    ("GET", "/api/v1/accounting/status", None, {200}, None, "计费状态"),
    ("GET", "/api/v1/accounting/report", None, {200}, None, "计费报告"),
    ("GET", "/api/v1/conntrack/acct", None, {200}, None, "conntrack计费开关"),
    # ---- 无法安全触发的动作端点：只验「已注册」(未注册404/已注册非404) ----
    ("POST", "/api/v1/wol/wake", {}, None, None, "WOL唤醒(已注册即PASS)"),
    ("POST", "/api/v1/firmware/apply", {}, None, None, "固件升级(已注册即PASS)"),
    ("POST", "/api/v1/firmware/confirm", {}, None, None, "固件确认(已注册即PASS)"),
    ("POST", "/api/v1/firmware/plan", {}, None, None, "固件计划(已注册即PASS)"),
]


def route_registered(path: str) -> bool:
    """探测路由是否注册：给 GET 路径发 HEAD/GET，非 404 即认为注册。
    对 POST 端点用一个极小请求，只要不是 404/405 就说明路由存在。"""
    status, body, _ = call("GET", path)
    if status == 404:
        return False
    if status == 405:  # 方法不允许但路由存在
        return True
    return True


def main() -> int:
    token = login()
    results = []
    passes = fails = 0

    for method, path, body, expect, check, note in ROUTES:
        if expect is None:
            # 只验注册
            reg = route_registered(path)
            ok = reg
            detail = "已注册" if reg else "未注册(404)"
            results.append((path, note, "PASS" if ok else "FAIL", detail))
            passes += ok
            fails += (not ok)
            continue

        status, data, dt = call(method, path, token=token, body=body)
        ok = status in expect
        detail = f"HTTP {status} (预期 {expect})"
        if ok and check is not None:
            if isinstance(check, tuple):
                fn, _ = check
            else:
                fn = check
            try:
                fok = fn(data)
                if not fok:
                    ok = False
                    detail += f" 关键字段缺失: {json.dumps(data)[:120]}"
                else:
                    detail += " 关键字段OK"
            except Exception as e:  # noqa: BLE001
                ok = False
                detail += f" 断言异常: {e}"
        results.append((path, note, "PASS" if ok else "FAIL", detail))
        passes += ok
        fails += (not ok)

    # 写报告
    lines = []
    lines.append("# 新路由冒烟报告 (smoke-new-routes)")
    lines.append("")
    lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
    lines.append("")
    lines.append(f"**汇总**: 覆盖 {len(results)} 条 / PASS {passes} / FAIL {fails}")
    lines.append("")
    lines.append("| 路由 | 说明 | 结果 | 详情 |")
    lines.append("|---|---|---|---|")
    for path, note, res, detail in results:
        lines.append(f"| `{path}` | {note} | {res} | {detail} |")
    lines.append("")
    DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")

    # 打印
    for path, note, res, detail in results:
        print(f"{res:<4} {detail:<40} {path} — {note}")
    print(f"\n汇总: {len(results)} 条路由 / PASS {passes} / FAIL {fails}")
    print(f"报告: {DOC}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
