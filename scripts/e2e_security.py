#!/usr/bin/env python3
"""安全面端到端：逐端点断言状态码，失败即非零退出。

之前这脚本只把 (method, path, code) 抄进 Markdown，永远 rc=0——
`webfilter/apply` 返回 500、`nac/apply` 返回 404 都被当成"记录"而不是失败，
等于没有测试。现在每个端点都带期望状态码。

约定：
- 200/2xx 表示接口正常返回；
- 409 表示需要 confirm（破坏性操作的正确防护）；
- 403 + 10503 表示目标目录不可写（非 root 开发实例的正常结果，
  但它必须是 403 而不是 500——500 会让人以为是服务端 bug）；
- 500 一律 FAIL；404 一律 FAIL（说明路由不存在，脚本或产品有一方错了）。
"""
import json
import os
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
TOKEN = None

# (method, path, body, 期望状态码集合, 说明)
EXPECT = [
    ("GET", "/api/v1/nac/config", None, {200}, "NAC 全局配置"),
    ("GET", "/api/v1/nac/connlimits", None, {200}, "连接数限制"),
    ("POST", "/api/v1/nac/connlimits/apply", {}, {200, 409}, "下发连接限制"),
    ("GET", "/api/v1/nac/top-connections", None, {200}, "Top 连接"),
    ("GET", "/api/v1/acl/mac-filter", None, {200}, "MAC 过滤"),
    ("GET", "/api/v1/quota/policies", None, {200}, "配额策略"),
    ("GET", "/api/v1/quota/report", None, {200}, "配额用量"),
    ("POST", "/api/v1/quota/apply", {}, {409}, "破坏性下发必须要求 confirm"),
    ("GET", "/api/v1/quota/nft", None, {200}, "配额 nft 表"),
    ("GET", "/api/v1/security/attack-guard", None, {200}, "攻击防护"),
    ("GET", "/api/v1/security/ddos", None, {200}, "DDoS 防护"),
    ("GET", "/api/v1/alg", None, {200}, "ALG"),
    ("GET", "/api/v1/ips/blocklist", None, {200}, "IPS 黑名单"),
    ("POST", "/api/v1/ips/test", {"ip": "1.2.3.4"}, {200}, "IPS 测试命中"),
    ("GET", "/api/v1/webfilter/rules", None, {200}, "域名过滤规则"),
    ("POST", "/api/v1/webfilter/apply", {"rules": []}, {200, 403}, "下发过滤（403=目录不可写）"),
    ("GET", "/api/v1/webfilter/apply-status", None, {200}, "下发状态"),
]

# 需要真实网卡/邻居表，容器或开发机上不可用，跳过但显式记录。
HARDWARE = {"/api/v1/security/arp", "/api/v1/security/neighbors", "/api/v1/nac/neighbors"}


def login():
    global TOKEN
    data = json.dumps({"username": "admin", "password": "admin123"}).encode()
    r = urllib.request.Request(BASE + "/api/v1/auth/login", data=data,
                               headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(r) as resp:
        TOKEN = json.loads(resp.read())["token"]


def req(method, path, body=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method)
    r.add_header("Authorization", "Bearer " + TOKEN)
    if data is not None:
        r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r) as resp:
            raw = resp.read()
            try:
                return resp.status, json.loads(raw)
            except Exception:
                return resp.status, raw.decode(errors="replace")[:200]
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw.decode(errors="replace")[:200]
    except Exception as e:  # 连接失败
        return 0, str(e)


def errcode(data):
    if isinstance(data, dict):
        return str(data.get("code", "") or "")
    return ""


def main():
    try:
        login()
    except Exception as e:
        print(f"登录失败，无法测安全面: {e}")
        return 1

    rows, fails = [], 0
    for method, path, body, want, note in EXPECT:
        code, data = req(method, path, body)
        ok = code in want
        # 403 必须是"明确的权限错误"而不是别的 403
        if ok and code == 403 and errcode(data) != "10503":
            ok = False
        if not ok:
            fails += 1
        rows.append((method, path, code, "PASS" if ok else "FAIL", note, data))
        time.sleep(0.1)

    for path in HARDWARE:
        rows.append(("GET", path, "SKIP", "SKIP", "hardware-dependent", None))

    lines = ["# E2E Security Report", "",
             f"- Base: {BASE}",
             f"- 通过: {sum(1 for r in rows if r[3] == 'PASS')} / "
             f"失败: {fails} / 跳过: {sum(1 for r in rows if r[3] == 'SKIP')}",
             "",
             "| Method | Path | Code | Result | 说明 |",
             "|---|---|---|---|---|"]
    for m, p, c, res, note, data in rows:
        lines.append(f"| {m} | {p} | {c} | {res} | {note} |")

    out = os.path.join(os.path.dirname(__file__), "..", "docs", "e2e-security.md")
    with open(out, "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")

    for m, p, c, res, note, data in rows:
        if res == "FAIL":
            detail = ""
            if isinstance(data, dict):
                detail = str(data.get("detail") or data.get("message") or "")[:120]
            print(f"FAIL {m} {p} → {c} {detail}")

    print(f"汇总: {len(rows)} 项 / FAIL {fails}")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
