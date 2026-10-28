#!/usr/bin/env python3
"""verify_pagination.py — 分页语义的运行时验证（不只探 200，要断言语义）。

## 为什么需要单独一个脚本

单测（p49_logs_pagination_test.go）钉的是 paginateEntries 纯函数的边界，
但它证明不了"HTTP 层把参数接对了、字段真的出现在响应里"。而只 curl 一眼
200 也证明不了语义对不对——实测踩过：`?limit=10` 返回 `total=10` 且
`has_more=false`，HTTP 200、字段齐全，但把 5000 条 journal 谎报成 10 条。
这种错只有断言语义才能抓到。

## 断言（两个端点同一套语义）

1. 不带参数：count == total，has_more=false（向后兼容，e2e 无参调用）
2. limit=N：count <= N
3. 窗口被填满：has_more=true；此时 logs 的 total 必须是 -1（说不清就说实话）
4. offset 生效：offset=最后一页时 has_more=false 且 count == total-offset

用法:
  python3 scripts/verify_pagination.py                 # 默认 http://127.0.0.1:9090
  BASE=... python3 scripts/verify_pagination.py
报告写到 docs/pagination-verify.md（脚本生成，勿手写）。
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
DOC = ROOT / "docs" / "pagination-verify.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")

results: list[dict] = []


def call(method: str, path: str, token: str | None = None, body: dict | None = None,
         timeout: int = 60) -> tuple[int, dict | str]:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            try:
                return r.status, json.loads(raw)
            except json.JSONDecodeError:
                return r.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, raw


def login() -> str:
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    st, body = call("POST", "/api/v1/auth/login", body={"username": user, "password": pwd})
    if st != 200 or not isinstance(body, dict) or not body.get("token"):
        print(f"登录失败 HTTP {st}: {body}")
        sys.exit(1)
    return body["token"]


def check(name: str, ok: bool, detail: str) -> None:
    results.append({"name": name, "ok": ok, "detail": detail})
    print(f"{'PASS' if ok else 'FAIL'} {name}: {detail}")


def main() -> int:
    token = login()

    # ---- /api/v1/system/services ----
    st, d = call("GET", "/api/v1/system/services", token=token, timeout=120)
    if st != 200 or not isinstance(d, dict):
        check("services 无参数", False, f"HTTP {st}")
    else:
        n = len(d.get("units") or [])
        check("services 无参数全量且 count==total",
              n == d.get("total") and d.get("has_more") is False,
              f"count={n} total={d.get('total')} has_more={d.get('has_more')}")
        total_svc = d.get("total") or 0

        st2, d2 = call("GET", "/api/v1/system/services?limit=10", token=token, timeout=120)
        d2 = d2 if isinstance(d2, dict) else {}
        n2 = len(d2.get("units") or [])
        check("services limit=10 只回 10 条且 total 是真总数",
              st2 == 200 and n2 == 10 and d2.get("total") == total_svc,
              f"count={n2} total={d2.get('total')}")

        last = max(total_svc - 5, 0)
        st3, d3 = call("GET", f"/api/v1/system/services?limit=10&offset={last}", token=token, timeout=120)
        d3 = d3 if isinstance(d3, dict) else {}
        n3 = len(d3.get("units") or [])
        check("services 最后一页 has_more=false 且条数正确",
              st3 == 200 and n3 == total_svc - last and d3.get("has_more") is False,
              f"offset={last} count={n3} 期望 {total_svc - last} has_more={d3.get('has_more')}")

        st4, d4 = call("GET", "/api/v1/system/services?limit=10000", token=token, timeout=120)
        d4 = d4 if isinstance(d4, dict) else {}
        n4 = len(d4.get("units") or [])
        check("services limit 超上限被夹住但不报错",
              st4 == 200 and n4 == total_svc,
              f"count={n4} 期望 {total_svc}")

    # ---- /api/v1/logs ----
    st, d = call("GET", "/api/v1/logs?limit=10", token=token, timeout=120)
    if st != 200 or not isinstance(d, dict):
        check("logs limit=10", False, f"HTTP {st}")
    else:
        n = len(d.get("entries") or [])
        # 窗口被填满时必须如实说"还有更多"，total 说不清就 -1
        filled = d.get("has_more") is True
        check("logs 窗口填满时 has_more=true 且 total=-1（不谎报总数）",
              n == 10 and filled and d.get("total") == -1,
              f"count={n} total={d.get('total')} has_more={d.get('has_more')}")
        check("logs 回带 limit/offset 便于前端翻页",
              d.get("limit") == 10 and d.get("offset") == 0,
              f"limit={d.get('limit')} offset={d.get('offset')}")

    # 取到末尾：用一个一定匹配不到任何行的 grep，窗口不会填满
    st, d = call("GET", "/api/v1/logs?limit=1000&grep=NOSUCHSTRING_%d" % int(time.time()),
                 token=token, timeout=120)
    if st == 200 and isinstance(d, dict):
        check("logs 取到末尾时 total 是真值、has_more=false",
              d.get("has_more") is False and d.get("total") == 0,
              f"count={d.get('count')} total={d.get('total')} has_more={d.get('has_more')}")

    # 无 token 仍然是 401（分页改动不能破坏鉴权）
    st, _ = call("GET", "/api/v1/logs?limit=10")
    check("logs 无 token 仍 401", st == 401, f"HTTP {st}")

    # ---- 写报告 ----
    lines = ["# 分页语义运行时验证报告", "",
             f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}", "",
             f"**汇总**: {len(results)} 项 / PASS {sum(1 for r in results if r['ok'])} / "
             f"FAIL {sum(1 for r in results if not r['ok'])}", "",
             "| 检查项 | 结果 | 实测 |", "|---|---|---|"]
    for r in results:
        lines.append(f"| {r['name']} | {'PASS' if r['ok'] else 'FAIL'} | {r['detail']} |")
    lines += ["", "## 说明", "",
              "服务列表能拿到全量，所以 `total` 始终是真值；日志走 `journalctl -n`，",
              "取到的只是尾部窗口，窗口被填满时无法确定总数，因此返回 `total=-1` +",
              "`has_more=true`——宁可说\"还有更多但不知道多少\"，也不谎报一个假总数。"]
    DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"\n汇总: {len(results)} 项 / FAIL {sum(1 for r in results if not r['ok'])}")
    print(f"报告: {DOC}")
    return 0 if all(r["ok"] for r in results) else 1


if __name__ == "__main__":
    sys.exit(main())
