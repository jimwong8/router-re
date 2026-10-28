#!/usr/bin/env python3
"""并发核销 8 张卡密，验证不会互相覆盖。

修复前：16 张并发核销，14 张 used_at 被抹回 0（单元测试实测）。
修复后：全部 8 张都必须是 used=true。
"""
import json
import urllib.request
from concurrent.futures import ThreadPoolExecutor

BASE = "http://127.0.0.1:9090"


def call(method, path, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read() or b"{}")
    except Exception as e:  # noqa: BLE001
        try:
            return e.code, json.loads(e.read() or b"{}")
        except Exception:  # noqa: BLE001
            return 0, {"error": str(e)}


_, body = call("POST", "/api/v1/auth/login", {"username": "admin", "password": "admin123"})
tok = body["token"]

N = 8
st, gen = call("POST", "/api/v1/voucher/generate",
               {"count": N, "minutes": 60, "note": "并发核销验证"}, tok)
if st != 200:
    raise SystemExit(f"生成失败: {st} {gen}")
vs = gen["vouchers"]
codes = [v["code"] for v in vs]
print(f"生成 {len(vs)} 张")

# 同时打出去：越多并发越容易暴露整份写回的覆盖
with ThreadPoolExecutor(max_workers=N) as ex:
    futs = [ex.submit(call, "POST", "/api/v1/voucher/redeem",
                      {"code": v["code"], "password": v["password"]}, tok) for v in vs]
    results = [f.result() for f in futs]

ok_n = sum(1 for st, _ in results if st == 200)
print(f"核销成功 {ok_n}/{N}")

_, lst = call("GET", "/api/v1/voucher/list", token=tok)
rows = {r["code"]: r for r in (lst.get("list") or lst.get("vouchers") or [])}
unused = [c for c in codes if not (rows.get(c) or {}).get("used")]

for c in codes:
    call("DELETE", f"/api/v1/voucher?code={c}", token=tok)

if unused:
    print(f"FAIL 有 {len(unused)} 张核销后 used 仍为 false：{unused}")
    print("     这些卡可以再白用一次")
    raise SystemExit(1)
print(f"PASS {N}/{N} 张卡核销后都是 used=true（并发无覆盖），已清理")
