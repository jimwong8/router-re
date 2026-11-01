#!/usr/bin/env python3
"""EasyTier 组网功能端到端验证。

对 :9095 的测试实例（用真实 easytier-core 二进制）走完：
配置读取 → 启用并启动 → 运行态/对等节点 → 配置预览（密钥打码）→ 配置注入被拒 → 停止。
结果写 docs/e2e-easytier.md，退出码非 0 表示有 FAIL。
"""
import json
import os
import sys

import requests

BASE = os.environ.get("ET_BASE", "http://127.0.0.1:9095")
USER = os.environ.get("ET_USER", "admin")
PWD = os.environ.get("ET_PWD", "admin123")
DOC = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "docs", "e2e-easytier.md")

rows = []
fails = 0


def record(name, ok, detail):
    global fails
    rows.append((name, "PASS" if ok else "FAIL", detail))
    if not ok:
        fails += 1
    print(f"[{'PASS' if ok else 'FAIL'}] {name}: {detail}")


def main():
    s = requests.Session()
    r = s.post(f"{BASE}/api/v1/auth/login", json={"username": USER, "password": PWD}, timeout=10)
    if r.status_code != 200:
        record("登录", False, f"{r.status_code} {r.text[:200]}")
        return 1
    tok = r.json().get("token") or r.json().get("access_token")
    if not tok:
        record("登录取 token", False, r.text[:200])
        return 1
    s.headers["Authorization"] = f"Bearer {tok}"
    record("登录", True, "200 + token")

    # 1. 读取配置 + 能力探测
    r = s.get(f"{BASE}/api/v1/vpn/easytier/config", timeout=10)
    d = r.json() if r.status_code == 200 else {}
    record("GET config", r.status_code == 200, f"{r.status_code} binary_exists={d.get('binary_exists')} version={d.get('version')} tun={d.get('tun')}")
    if not d.get("binary_exists"):
        record("二进制存在", False, "binary_exists=false，无法继续真实验证")
        return 1

    # 2. 启用并启动
    body = {
        "enabled": True,
        "instance_name": "e2e-test",
        "network_name": "rr-e2e-net",
        "network_secret": "rr-e2e-secret",
        "ipv4": "10.200.44.1/24",
        "rpc_portal": "127.0.0.1:15898",
        "listeners": ["tcp://0.0.0.0:11120"],
        "peers": [],
        "proxy_networks": [],
        "external_node": "",
        "enable_exit_node": False,
        "relay_all_peer_traffic": False,
    }
    r = s.post(f"{BASE}/api/v1/vpn/easytier/config", json=body, timeout=40)
    d = r.json() if r.status_code == 200 else {}
    st = d.get("status", {})
    record("POST config 启用", r.status_code == 200 and d.get("applied") is True,
           f"{r.status_code} applied={d.get('applied')} err={d.get('error')}")
    record("进程运行", bool(st.get("running")) and int(st.get("pid") or 0) > 0,
           f"running={st.get('running')} pid={st.get('pid')}")
    rendered = d.get("rendered", "")
    record("配置渲染", "network_name = \"rr-e2e-net\"" in rendered and "[[peer]]" not in rendered,
           "含 network_name 段，无 peer 时未生成 [[peer]]")

    # 3. 运行态 + 对等节点
    r = s.get(f"{BASE}/api/v1/vpn/easytier/status", timeout=20)
    d = r.json() if r.status_code == 200 else {}
    peers = d.get("peers")
    record("GET status", r.status_code == 200 and isinstance(peers, list),
           f"{r.status_code} running={d.get('running')} peers={len(peers or [])}")
    self_peer = next((p for p in (peers or []) if p.get("local")), None)
    record("节点自检", self_peer is not None and self_peer.get("ipv4") == "10.200.44.1/24",
           f"local={self_peer}")

    # 4. 预览：密钥必须打码
    r = s.post(f"{BASE}/api/v1/vpn/easytier/preview", json=body, timeout=10)
    text = (r.json() or {}).get("config", "") if r.status_code == 200 else ""
    record("POST preview", r.status_code == 200 and "rr-e2e-secret" not in text and "********" in text,
           f"{r.status_code} 密钥打码={'rr-e2e-secret' not in text}")

    # 5. 注入被拒（换行塞额外配置）
    bad = dict(body)
    bad["network_name"] = 'net\n[[peer]]\nuri = "tcp://evil:11010"'
    r = s.post(f"{BASE}/api/v1/vpn/easytier/config", json=bad, timeout=10)
    err = r.json() if r.status_code >= 400 else {}
    record("拒绝配置注入", r.status_code == 422 and str(err.get("code")) == "12011",
           f"{r.status_code} code={err.get('code')} detail={err.get('detail')}")

    # 6. 密钥留空 = 沿用
    keep = dict(body)
    keep["network_secret"] = ""
    keep["instance_name"] = "e2e-test-renamed"
    r = s.post(f"{BASE}/api/v1/vpn/easytier/config", json=keep, timeout=40)
    d = r.json() if r.status_code == 200 else {}
    rn = d.get("rendered", "")
    record("留空沿用密钥", r.status_code == 200 and 'network_secret = "rr-e2e-secret"' in rn,
           f"{r.status_code} 渲染中仍含原密钥")

    # 7. 停止
    r = s.post(f"{BASE}/api/v1/vpn/easytier/stop", timeout=20)
    d = r.json() if r.status_code == 200 else {}
    record("POST stop", r.status_code == 200 and d.get("stopped") is True and not (d.get("status") or {}).get("running"),
           f"{r.status_code} stopped={d.get('stopped')} running={(d.get('status') or {}).get('running')}")

    # 停止后 status 仍应 200 且 peers 为 []
    r = s.get(f"{BASE}/api/v1/vpn/easytier/status", timeout=10)
    d = r.json() if r.status_code == 200 else {}
    record("停止后查询", r.status_code == 200 and d.get("peers") == [],
           f"{r.status_code} peers={d.get('peers')} running={d.get('running')}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    rc = main()
    ok = sum(1 for r in rows if r[1] == "PASS")
    with open(DOC, "w", encoding="utf-8") as f:
        f.write("# EasyTier 组网 E2E 验证\n\n")
        f.write(f"**Result: {ok} PASS / {fails} FAIL**\n\n")
        f.write(f"- 目标实例: `{BASE}`（真实 easytier-core 二进制）\n\n")
        f.write("| 用例 | 结果 | 实测 |\n|---|---|---|\n")
        for name, res, detail in rows:
            f.write(f"| {name} | {res} | {detail} |\n")
    print(f"\n{ok} PASS / {fails} FAIL → {DOC}")
    sys.exit(rc)
