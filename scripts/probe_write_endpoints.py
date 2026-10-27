#!/usr/bin/env python3
"""写端点的空输入健壮性探测：对全部 POST/DELETE 发空 body，分类结果。

为什么先做这个而不是直接填有效数据：
  137 个写端点里有 `interfaces/config`、`firewall/rules`、`wan/links` 这类
  会真的改系统网络的。**空 body 被校验拒绝 = 零副作用**，是最安全的批量手段，
  同时能一次回答三个问题：

  1. 哪些端点有输入校验（4xx + 明确错误码）—— 健康的
  2. 哪些端点对空 body 也返回 2xx —— 可疑：要么空操作被当作成功，
     要么根本没解码请求体（空壳 handler）
  3. 哪些端点 5xx —— 真 bug：没处理空输入

用法：
    python3 scripts/probe_write_endpoints.py            # 全量
    python3 scripts/probe_write_endpoints.py --limit 40 # 先跑一部分
    python3 scripts/probe_write_endpoints.py --only /api/v1/vlan
"""
from __future__ import annotations

import argparse
import json
import os
import re
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from collections import Counter
from pathlib import Path

ROOT = Path("/home/jimwong/projects/router-re")
SERVER_GO = ROOT / "mgmtd" / "internal" / "server" / "server.go"
WORKDIR = "/tmp/mgmprobe"
PORT = 28091

# 明确会改系统状态的端点，空 body 也**不要碰**（宁可漏测也不误伤）
DANGEROUS = [
    "/interfaces/config", "/firewall/rules", "/wan/links", "/route/",
    "/nat/rules", "/vlan", "/bridge", "/vpn/", "/wireless/ssids",
    "/dhcp/config", "/dhcp/server", "/qos/", "/dns/config", "/upnp/config",
    "/snmp/config", "/radius/config", "/ddns/config", "/ntp/config",
]


def extract_write_routes() -> list[tuple[str, str]]:
    """从 server.go 提取 (METHOD, PATH)。只取 POST/PUT/DELETE/PATCH。"""
    text = SERVER_GO.read_text(encoding="utf-8")
    out = []
    for m in re.finditer(r'HandleFunc\("(POST|PUT|DELETE|PATCH) ([^"]+)"', text):
        out.append((m.group(1), m.group(2)))
    return out


def is_dangerous(path: str) -> bool:
    return any(d in path for d in DANGEROUS)


def concrete_path(path: str) -> str:
    """把 {name}/{id} 这类路径参数替换成探针值。"""
    return re.sub(r"\{[^}]+\}", "probe-nonexistent", path)


BASE = ""


def req(method: str, path: str, token: str, body: dict | None = None,
        timeout: int = 10) -> tuple[int, str]:
    url = BASE + path
    data = json.dumps(body if body is not None else {}).encode()
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(r, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")[:200]
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")[:200]
    except Exception as e:  # noqa: BLE001
        return 0, f"{type(e).__name__}: {e}"


def wait_port(port: int, timeout: float = 20.0) -> bool:
    end = time.time() + timeout
    while time.time() < end:
        with socket.socket() as s:
            s.settimeout(0.5)
            if s.connect_ex(("127.0.0.1", port)) == 0:
                return True
        time.sleep(0.3)
    return False


def main() -> int:
    global BASE
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--only", default="")
    ap.add_argument("--include-dangerous", action="store_true")
    ap.add_argument("--out", default=str(ROOT / "docs" / "write-endpoint-audit.md"))
    ap.add_argument("--strict", action="store_true",
                    help="有问题时返回非 0（CI 门禁用）；默认只出报告不判失败")
    args = ap.parse_args()

    BASE = f"http://127.0.0.1:{PORT}"
    routes = extract_write_routes()
    if args.only:
        routes = [r for r in routes if args.only in r[1]]
    skipped = [r for r in routes if is_dangerous(r[1]) and not args.include_dangerous]
    routes = [r for r in routes if args.include_dangerous or not is_dangerous(r[1])]
    if args.limit:
        routes = routes[: args.limit]

    print(f"写端点 {len(routes)} 个待测，跳过高危 {len(skipped)} 个\n")
    if not routes:
        return 0

    env = dict(os.environ,
               MGM_ADDR=f"127.0.0.1:{PORT}",
               MGM_SESSION_SECRET="probe-secret-at-least-32-bytes-long-1234",
               MGM_ADMIN_USER="admin",
               MGM_ADMIN_PASSWORD="probepass")
    for k in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        env.pop(k, None)

    os.makedirs(WORKDIR, exist_ok=True)
    proc = subprocess.Popen(["/tmp/mgmtd-probe"], cwd=WORKDIR, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                            text=True, start_new_session=True)
    rows: list[tuple[str, str, int, str]] = []
    try:
        if not wait_port(PORT):
            print("服务未就绪")
            return 1

        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        login = urllib.request.Request(
            BASE + "/api/v1/auth/login",
            data=json.dumps({"username": "admin", "password": "probepass"}).encode(),
            headers={"Content-Type": "application/json"}, method="POST")
        with opener.open(login, timeout=10) as r:
            token = json.load(r).get("token", "")
        if not token:
            print("登录失败，拿不到 token")
            return 1

        for method, path in routes:
            st, body = req(method, concrete_path(path), token)
            rows.append((method, path, st, body))
            time.sleep(0.05)

    finally:
        try:
            os.killpg(os.getpgid(proc.pid), 15)
        except ProcessLookupError:
            pass

    # 分类
    def bucket(st: int) -> str:
        if st == 0:
            return "连接失败"
        if 200 <= st < 300:
            return "2xx 接受空body（可疑）"
        if st == 404:
            return "404 路由/资源不存在"
        if st in (401, 403):
            return "401/403 鉴权"
        if 400 <= st < 500:
            return "4xx 已校验（健康）"
        return "5xx 崩溃（真bug）"

    counts = Counter(bucket(st) for _, _, st, _ in rows)
    print("=== 分类统计 ===")
    for k, v in counts.most_common():
        print(f"  {v:3d}  {k}")

    suspicious = [r for r in rows if 200 <= r[2] < 300]
    crashes = [r for r in rows if r[2] >= 500 or r[2] == 0]

    print(f"\n=== 可疑（空 body 也 2xx，共 {len(suspicious)}）===")
    for m, p, st, b in suspicious[:20]:
        print(f"  {st} {m:6s} {p}   {b[:80]}")

    print(f"\n=== 崩溃（5xx/连不上，共 {len(crashes)}）===")
    for m, p, st, b in crashes[:20]:
        print(f"  {st} {m:6s} {p}   {b[:100]}")

    # 写报告
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    lines = ["# 写端点空输入审计", "",
             f"生成于 {time.strftime('%F %T')}｜共测 {len(rows)} 个写端点"
             f"｜跳过高危 {len(skipped)} 个", "",
             "方法：对每个 POST/DELETE 发送空 JSON `{}`。空 body 被校验拒绝 = 零副作用；",
             "同时对空 body 返回 2xx 的端点可疑（空操作当成功，或未解码请求体）。", "",
             "## 分类统计", "", "| 类别 | 数量 |", "|---|---|"]
    for k, v in counts.most_common():
        lines.append(f"| {k} | {v} |")

    lines += ["", "## 可疑端点（空 body 也 2xx）", ""]
    if suspicious:
        lines += ["| 状态 | 方法 | 路径 | 响应片段 |", "|---|---|---|---|"]
        for m, p, st, b in suspicious:
            lines.append(f"| {st} | {m} | `{p}` | `{b[:60].replace('|', '/')}` |")
    else:
        lines.append("无。")

    lines += ["", "## 崩溃端点（5xx / 连接失败）", ""]
    if crashes:
        lines += ["| 状态 | 方法 | 路径 | 响应片段 |", "|---|---|---|---|"]
        for m, p, st, b in crashes:
            lines.append(f"| {st} | {m} | `{p}` | `{b[:60].replace('|', '/')}` |")
    else:
        lines.append("无。")

    lines += ["", "## 全部明细", "", "| 状态 | 方法 | 路径 | 分类 |", "|---|---|---|---|"]
    for m, p, st, _b in rows:
        lines.append(f"| {st} | {m} | `{p}` | {bucket(st)} |")

    if skipped:
        lines += ["", "## 跳过的高危端点（需人工单独验证）", ""]
        for m, p in skipped:
            lines.append(f"- {m} `{p}`")

    out.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"\n报告已写入 {out}")

    if args.strict and (suspicious or crashes):
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
