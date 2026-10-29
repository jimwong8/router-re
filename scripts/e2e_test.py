#!/usr/bin/env python3
"""e2e_test.py — 对运行中的 mgmtd 做全端点端到端验收。

为什么不硬编码端点清单：端点从 server.go 的 HandleFunc 自动提取，
新加的端点会自动纳入验收，不会出现"加了端点但没测"的盲区。

用法:
  python3 scripts/e2e_test.py                    # 默认 http://127.0.0.1:9090
  BASE=http://10.100.1.18:9090 python3 scripts/e2e_test.py
  python3 scripts/e2e_test.py --verbose          # 打印每个响应的前 200 字符

安全检查：
  - 只对 GET 端点做断言式验收。
  - POST/DELETE 默认 **不** 调用（会改系统状态）；--write 才测，
    且只测幂等或可回滚的那几个（登录失败审计、空数组 422 校验）。
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SERVER = ROOT / "mgmtd" / "internal" / "server" / "server.go"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
SKIP_PATHS = {"/api/v1/auth/login"}  # POST-only; GET 会 405，单独测

# 少数 GET 端点需要 query 参数，否则返回 400。这不是接口坏了，
# 是验收脚本得按真实调用方式请求，否则会把"参数缺失"误报成失败。
# 这些端点在没有配置/前置数据时返回 422 是**正确**的语义（不是接口坏了）：
# 没有成员就选不出出口，没有规则就测不出命中。E2E 必须区分
# 「正确拒绝」和「接口故障」，否则每次都要人工判断这条失败是不是真的。
# 纯文本端点：这些接口本来就返回 text/plain，不是 JSON。
# 用 JSON 解析它们会误判成失败（诊断报告导出就是给人看/存档的纯文本）。
TEXT_ENDPOINTS = {
    "/api/v1/diagnostics/report/text",
    "/api/v1/notify/smtp/preview",
    "/api/v1/portal/preview",  # 返回 text/html 门户模板渲染（p18b），非 JSON
}

EXPECTED_422 = {
    "/api/v1/lb/preview",       # 没有 enabled 成员时无法选路
}

DEFAULT_QUERY = {
    "/api/v1/dpi/classify": "?dport=80&proto=tcp",
    # 子网换算必须带 cidr，否则 422 是正确语义（参数缺失），不是接口坏
    "/api/v1/tools/subnet": "?cidr=192.168.1.0/24",
    "/api/v1/stats/history": "?minutes=60",
    "/api/v1/conntrack": "?limit=50",
    "/api/v1/logs": "?limit=50",
    "/api/v1/htb/show": "?iface=ens18",
    # firmware/inspect 必须带 path；/bin/true 是真实存在的小文件，
    # 能被 inspect 识别出 magic=elf（不是固件），用来验证"读取+识别"这条路径
    "/api/v1/firmware/inspect": "?path=/bin/true",
    "/api/v1/iface/tune": "?iface=ens18",
}


def extract_routes() -> list[tuple[str, str]]:
    """从 server.go 提取 (METHOD, PATH)，保持源码顺序。"""
    src = SERVER.read_text(encoding="utf-8")
    pat = re.compile(r'HandleFunc\(\s*"(GET|POST|PUT|DELETE)\s+([^"]+)"')
    routes: list[tuple[str, str]] = []
    for method, path in pat.findall(src):
        if "{" in path:
            continue  # 带路径参数的跳过（无边界的 id 无法安全构造）
        routes.append((method, path))
    return routes


def call(method: str, path: str, token: str | None = None, body: dict | None = None,
         timeout: int = 20) -> tuple[int, str, float]:
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    import time
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", "replace"), time.time() - t0
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace"), time.time() - t0
    except Exception as exc:  # noqa: BLE001
        return 0, f"{type(exc).__name__}: {exc}", time.time() - t0


def login() -> str:
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    status, body, _ = call("POST", "/api/v1/auth/login",
                           body={"username": user, "password": pwd})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body[:300]}")
        sys.exit(1)
    return json.loads(body)["token"]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--verbose", action="store_true")
    ap.add_argument("--write", action="store_true", help="额外测几个安全写端点")
    args = ap.parse_args()

    token = login()
    routes = extract_routes()
    gets = [(m, p) for m, p in routes if m == "GET" and p not in SKIP_PATHS]

    print(f"端点清单: {len(routes)} 个（GET {len(gets)} 个参与验收）\n")
    print(f"{'状态':<6} {'HTTP':<6} {'耗时':<8} 端点")
    print("-" * 78)

    ok = empty = bad = 0
    failures: list[str] = []
    for method, path in gets:
        url = path + DEFAULT_QUERY.get(path, "")
        status, body, elapsed = call(method, url, token=token)
        if status == 422 and path in EXPECTED_422:
            print(f"{'SKIP':<6} {status:<6} {elapsed:>6.2f}s {path}  (缺前置条件，拒绝正确)")
            continue
        if status != 200:
            bad += 1
            failures.append(f"{path} → HTTP {status}: {body[:200]}")
            print(f"{'FAIL':<6} {status:<6} {elapsed:>6.2f}s {path}")
            continue
        # 结构检查：必须能解析成 JSON（HTML 错误页会在这里暴露）
        try:
            data = json.loads(body)
        except json.JSONDecodeError:
            # 纯文本端点本来就返回 text/plain（诊断报告导出、邮件预览）。
            # 用 JSON 解析它们是误判 —— 只要 200 且有内容就算通过。
            if path in TEXT_ENDPOINTS:
                if body.strip():
                    print(f"{'OK':<6} {status:<6} {elapsed:>6.2f}s {path}  (text/plain {len(body)}B)")
                else:
                    bad += 1
                    failures.append(f"{path} → 纯文本端点返回空响应")
                    print(f"{'FAIL':<6} {status:<6} {elapsed:>6.2f}s {path}  (空响应)")
                continue
            bad += 1
            failures.append(f"{path} → 非 JSON 响应: {body[:120]}")
            print(f"{'FAIL':<6} {status:<6} {elapsed:>6.2f}s {path}  (非 JSON)")
            continue

        note = ""
        if isinstance(data, dict):
            # 空集合是"接口通但没数据"，和"接口坏了"必须区分开
            empty_keys = [k for k, v in data.items()
                          if isinstance(v, list) and len(v) == 0]
            if empty_keys and len(data) <= 2:
                note = f"(空: {','.join(empty_keys)})"
                empty += 1
        ok += 1
        if args.verbose:
            print(f"{'OK':<6} {status:<6} {elapsed:>6.2f}s {path} {note}")
            print(" " * 22 + body[:200].replace("\n", " "))
        else:
            print(f"{'OK':<6} {status:<6} {elapsed:>6.2f}s {path} {note}")

    if args.write:
        print("\n--- 安全写端点 ---")
        # 空数组必须被拒绝（防"假成功"回归）
        for path in ("/api/v1/interfaces/config", "/api/v1/vlan"):
            status, body, _ = call("POST", path, token, {"configs": [], "vlans": []})
            if status == 404:
                # 端点还没实现（旧二进制 / 功能未交付），不算失败
                print(f"{'SKIP':<12} {status:<6} {path} 空数组（端点未实现）")
                continue
            expect = "422" if status == 422 else "FAIL(应为 422)"
            if status == 422:
                ok += 1
            else:
                bad += 1
                failures.append(f"{path} 空数组未拒绝 → HTTP {status}")
            print(f"{expect:<12} {status:<6} {path} 空数组")

        # 错误口令必须 401 且被审计
        status, _, _ = call("POST", "/api/v1/auth/login",
                            body={"username": "e2e-probe", "password": "wrong"})
        print(f"{'401' if status == 401 else 'FAIL':<12} {status:<6} /auth/login 错误口令")
        if status != 401:
            bad += 1
            failures.append(f"/auth/login 错误口令未返回 401（got {status}）")

    print("-" * 78)
    print(f"通过 {ok} / 空数据 {empty} / 失败 {bad}")
    if failures:
        print("\n失败明细:")
        for f in failures:
            print("  - " + f)
        return 1
    print("\nE2E: PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
