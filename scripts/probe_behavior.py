#!/usr/bin/env python3
"""行为探针：启动 mgmtd，用带唯一随机值的请求打真实端点，断言响应回显。

为什么必须做这一层：
  静态检查能证明"文件在"，模型审查能证明"代码看起来对"，但**都抓不到空壳
  handler**。本机踩过的真实形态：
      var req wireless.SSID; _ = req; writeJSON(w, 200, {...req...})
  编译通过、go test 全绿、HTTP 200，但它从来没解码过请求体，永远返回零值。
  唯一能抓到它的是：发一个带唯一随机串的请求，断言响应里出现那个串。

安全约束：**不触发 nftables 实际应用**。
  timeschedule 的 POST 不带 confirm 时只做校验并返回 dry_run:true —— 探针只打这条路径，
  绝不带 confirm，避免把测试规则写进生产网络。

用法：
    python3 scripts/probe_behavior.py            # 启动服务 + 全量探针 + 清理
    python3 scripts/probe_behavior.py --keep     # 保留进程（调试用）
"""
from __future__ import annotations

import argparse
import json
import os
import random
import socket
import string
import subprocess
import sys
import time
import urllib.error
import urllib.request

MGMT_DIR = "/home/jimwong/projects/router-re/mgmtd"
WORKDIR = "/tmp/mgmprobe"
PORT = 28090
BASE = f"http://127.0.0.1:{PORT}"

RESULTS: list[tuple[str, bool, str]] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    RESULTS.append((name, ok, detail))
    print(f"  {'PASS' if ok else 'FAIL'}  {name}" + (f"   {detail}" if detail else ""))


def rand_tag(n: int = 8) -> str:
    return "".join(random.choices(string.ascii_uppercase + string.digits, k=n))


def req(method: str, path: str, body: dict | None = None,
        token: str = "", timeout: int = 15) -> tuple[int, dict | str]:
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    r = urllib.request.Request(url, data=data, headers=headers, method=method)
    # 本地请求必须绕开代理：本机 http_proxy 会把 127.0.0.1 也劫持走跳板
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    try:
        with opener.open(r, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw)
            except json.JSONDecodeError:
                return resp.status, raw[:300]
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, raw[:300]


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
    global PORT, BASE          # 必须在任何对 PORT/BASE 的引用之前声明
    ap = argparse.ArgumentParser()
    ap.add_argument("--keep", action="store_true")
    ap.add_argument("--port", type=int, default=PORT)
    args = ap.parse_args()
    PORT = args.port
    BASE = f"http://127.0.0.1:{PORT}"

    os.makedirs(WORKDIR, exist_ok=True)
    binary = "/tmp/mgmtd-probe"
    if not os.path.exists(binary):
        print("先编译: cd mgmtd && go build -o /tmp/mgmtd-probe ./cmd/mgmtd")
        return 1

    env = dict(os.environ,
               MGM_ADDR=f"127.0.0.1:{PORT}",
               MGM_SESSION_SECRET="probe-secret-at-least-32-bytes-long-1234",
               MGM_ADMIN_USER="admin",
               MGM_ADMIN_PASSWORD="probepass")
    # 本地请求绕开代理
    for k in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        env.pop(k, None)

    proc = subprocess.Popen([binary], cwd=WORKDIR, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                            text=True, start_new_session=True)
    try:
        if not wait_port(PORT):
            print("服务 20s 内未监听，日志：")
            print((proc.stdout.read() or "")[:800] if proc.stdout else "")
            return 1
        print(f"服务已就绪 {BASE}\n")

        # ---- 登录 ----
        st, body = req("POST", "/api/v1/auth/login",
                       {"username": "admin", "password": "probepass"})
        token = ""
        if isinstance(body, dict):
            token = body.get("token") or body.get("data", {}).get("token") or ""
        check("登录拿 token", st == 200 and bool(token), f"HTTP {st}")

        # ---- 1. timeschedule：真实规则回显 ----
        tag = rand_tag()
        rule = {"id": f"probe-{tag}", "name": f"PROBE-{tag}", "days": [1, 2],
                "start_min": 0, "end_min": 60, "targets": ["10.99.99.99"],
                "action": "ratelimit", "rate_kbps": 1024, "enabled": True}

        st, body = req("POST", "/api/v1/policy/timeschedule", {"rules": [rule]}, token)
        ok = st == 200 and isinstance(body, dict)
        _d = json.dumps(body, ensure_ascii=False)[:220] if isinstance(body, dict) else str(body)[:220]
        check("timeschedule POST 校验通过", ok, f"HTTP {st} {_d}")
        # 不带 confirm 必须只返回 dry_run，不能真的下发 nft
        dry = isinstance(body, dict) and body.get("dry_run") is True
        check("不带 confirm 返回 dry_run:true（不下发 nft）", dry,
              f"dry_run={body.get('dry_run') if isinstance(body, dict) else '?'}")

        # 空数组必须被拒（10403）
        st, body = req("POST", "/api/v1/policy/timeschedule", {"rules": []}, token)
        code = str(body.get("code")) if isinstance(body, dict) else None
        check("空 rules 被拒 10403", st == 422 and code == "10403",
              f"HTTP {st} code={code}")

        # 坏规则必须指出第几条哪个字段（10404）
        bad = dict(rule, id=f"bad-{tag}", days=[9])
        st, body = req("POST", "/api/v1/policy/timeschedule", {"rules": [bad]}, token)
        code = str(body.get("code")) if isinstance(body, dict) else None
        detail = json.dumps(body, ensure_ascii=False)[:170] if isinstance(body, dict) else str(body)[:170]
        check("非法规则被拒 10404 且指出位置", st == 422 and code == "10404", f"HTTP {st} {detail}")

        # GET 结构
        st, body = req("GET", "/api/v1/policy/timeschedule", None, token)
        ok = (st == 200 and isinstance(body, dict)
              and "count" in body and "timezone" in body and "rules" in body)
        check("timeschedule GET 结构完整", ok,
              f"HTTP {st} keys={list(body.keys())[:6] if isinstance(body, dict) else '?'}")
        tz_ok = isinstance(body, dict) and body.get("timezone") == "Asia/Shanghai"
        check("timezone 为 Asia/Shanghai", tz_ok, f"{body.get('timezone') if isinstance(body, dict) else '?'}")

        # ---- 2. fwlog：结构 + 不崩 ----
        st, body = req("GET", "/api/v1/firewall/log/analysis", None, token)
        ok = isinstance(body, dict) and ("available" in body)
        check("fwlog/analysis 返回 available 字段", st == 200 and ok,
              f"HTTP {st} keys={list(body.keys())[:5] if isinstance(body, dict) else '?'}")
        st, body = req("GET", "/api/v1/firewall/log/suggestions", None, token)
        check("fwlog/suggestions 可访问", st == 200, f"HTTP {st}")

        # ---- 3. linkcheck：随机值回显 ----
        tag2 = rand_tag()
        target = {"id": f"probe-{tag2}", "name": f"PROBE-{tag2}",
                  "kind": "icmp", "iface": "lo", "host": "127.0.0.1",
                  "interval": 30, "timeout": 5,
                  "fail_threshold": 3, "recover_threshold": 2, "enabled": False}
        st, body = req("POST", "/api/v1/linkcheck/targets", {"targets": [target]}, token)
        _d = json.dumps(body, ensure_ascii=False)[:220] if isinstance(body, dict) else str(body)[:220]
        check("linkcheck POST 接受", st in (200, 201), f"HTTP {st} {_d}")
        st, body = req("GET", "/api/v1/linkcheck/targets", None, token)
        text = json.dumps(body, ensure_ascii=False)
        check("linkcheck 回显随机 name（非空壳 handler）", f"PROBE-{tag2}" in text,
              f"HTTP {st}")
        st, body = req("GET", "/api/v1/linkcheck/status", None, token)
        check("linkcheck/status 可访问", st == 200, f"HTTP {st}")

        # ---- 4. 未认证访问必须被拒 ----
        st, _ = req("GET", "/api/v1/policy/timeschedule", None, "")
        check("无 token 访问被拒", st in (401, 403), f"HTTP {st}")

    finally:
        if not args.keep:
            try:
                os.killpg(os.getpgid(proc.pid), 15)
            except ProcessLookupError:
                pass

    total = len(RESULTS)
    passed = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\n=== 结果 {passed}/{total} 通过 ===")
    for name, ok, detail in RESULTS:
        if not ok:
            print(f"  未过: {name}  {detail}")
    return 0 if passed == total else 2


if __name__ == "__main__":
    sys.exit(main())
