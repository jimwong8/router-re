#!/usr/bin/env python3
"""Tailscale 功能端到端验证。

两个目标：
  :9096 — 隔离的 tailscaled（userspace 网络 + 独立 socket），可以安全地 up/down；
  :9097 — 系统自带的 tailscaled（只读！验证真实节点的解析与展示）。

结果写 docs/e2e-tailscale.md，退出码非 0 表示有 FAIL。
"""
import json
import os
import sys

import requests

BASE_ISO = os.environ.get("TS_BASE_ISO", "http://127.0.0.1:9096")
BASE_SYS = os.environ.get("TS_BASE_SYS", "http://127.0.0.1:9097")
USER = os.environ.get("TS_USER", "admin")
PWD = os.environ.get("TS_PWD", "admin123")
DOC = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "docs", "e2e-tailscale.md")

rows = []
fails = 0


def record(name, ok, detail):
    global fails
    rows.append((name, "PASS" if ok else "FAIL", detail))
    if not ok:
        fails += 1
    print(f"[{'PASS' if ok else 'FAIL'}] {name}: {detail}")


def login(base):
    s = requests.Session()
    r = s.post(f"{base}/api/v1/auth/login", json={"username": USER, "password": PWD}, timeout=10)
    tok = (r.json() or {}).get("token") if r.status_code == 200 else None
    if tok:
        s.headers["Authorization"] = f"Bearer {tok}"
    return s, tok


def main():
    # ---------- 隔离实例：可写 ----------
    s, tok = login(BASE_ISO)
    record("登录（隔离实例）", bool(tok), f"token={'有' if tok else '无'}")
    if not tok:
        return 1

    r = s.get(f"{BASE_ISO}/api/v1/vpn/tailscale/config", timeout=10)
    d = r.json() if r.status_code == 200 else {}
    st = d.get("status") or {}
    record("GET config", r.status_code == 200 and d.get("binary_exists") and d.get("socket_exists"),
           f"{r.status_code} binary={d.get('binary_exists')} socket_exists={d.get('socket_exists')} system_socket={d.get('system_socket')}")
    record("识别为隔离 socket", d.get("system_socket") is False, f"system_socket={d.get('system_socket')}")
    record("初始状态可读", st.get("reachable") is True,
           f"reachable={st.get('reachable')} backend_state={st.get('backend_state')}")

    body = {
        "enabled": True,
        "auth_key": "tskey-auth-bogus0000000000000000000000",
        "hostname": "router-e2e",
        "advertise_routes": ["192.168.77.0/24"],
        "advertise_exit_node": False,
        "accept_routes": True,
        "accept_dns": True,
        "ssh": False,
        "login_server": "",
        "force_reauth": False,
    }

    r = s.post(f"{BASE_ISO}/api/v1/vpn/tailscale/preview", json=body, timeout=10)
    d = r.json() if r.status_code == 200 else {}
    args = d.get("args") or []
    joined = " ".join(args)
    record("POST preview", r.status_code == 200 and "--advertise-routes=192.168.77.0/24" in joined,
           f"{r.status_code} {joined}")
    record("预览里密钥打码", "tskey-auth-bogus" not in joined and "tskey-********" in joined,
           joined)

    # 假 key 必须被真实 CLI 拒绝，且错误要回传（不能 applied=true 骗人）
    r = s.post(f"{BASE_ISO}/api/v1/vpn/tailscale/config", json=body, timeout=120)
    d = r.json() if r.status_code == 200 else {}
    err = (d.get("error") or "")
    record("假 key 被真实拒绝", r.status_code == 200 and d.get("applied") is False and err != "",
           f"{r.status_code} applied={d.get('applied')} error={err[:140]}")
    record("失败后状态如实", (d.get("status") or {}).get("backend_state") != "Running",
           f"backend_state={(d.get('status') or {}).get('backend_state')}")

    # 校验：坏主机名
    bad = dict(body)
    bad["hostname"] = "-bad_host"
    r = s.post(f"{BASE_ISO}/api/v1/vpn/tailscale/config", json=bad, timeout=20)
    err = r.json() if r.status_code >= 400 else {}
    record("拒绝非法主机名", r.status_code == 422 and str(err.get("code")) == "12021",
           f"{r.status_code} code={err.get('code')} detail={err.get('detail')}")

    # 断开：隔离实例上安全
    r = s.post(f"{BASE_ISO}/api/v1/vpn/tailscale/down", json={}, timeout=60)
    d = r.json() if r.status_code == 200 else {}
    record("POST down", r.status_code == 200 and d.get("down") is True,
           f"{r.status_code} down={d.get('down')} state={(d.get('status') or {}).get('backend_state')}")

    r = s.get(f"{BASE_ISO}/api/v1/vpn/tailscale/status", timeout=10)
    d = r.json() if r.status_code == 200 else {}
    record("停止后仍可读状态", r.status_code == 200 and (d.get("status") or {}).get("reachable") is True,
           f"{r.status_code} state={(d.get('status') or {}).get('backend_state')}")

    # ---------- 系统实例：只读 ----------
    s2, tok2 = login(BASE_SYS)
    r = s2.get(f"{BASE_SYS}/api/v1/vpn/tailscale/config", timeout=10)
    d = r.json() if r.status_code == 200 else {}
    st = d.get("status") or {}
    record("系统节点识别", d.get("system_socket") is True, f"system_socket={d.get('system_socket')} socket={d.get('socket')}")
    record("系统节点在线", st.get("backend_state") == "Running",
           f"backend_state={st.get('backend_state')} version={st.get('version')}")
    peers = st.get("peers") or []
    record("真实节点解析", st.get("peer_count", 0) > 10 and len(peers) == st.get("peer_count"),
           f"peer_count={st.get('peer_count')} online={st.get('online_count')} 首行={peers[0].get('hostname') if peers else None}")
    self_ips = (st.get("self") or {}).get("ips") or []
    record("本机地址解析", len(self_ips) >= 1, f"self={st.get('self', {}).get('hostname')} ips={self_ips}")
    return 0 if fails == 0 else 1


if __name__ == "__main__":
    rc = main()
    ok = sum(1 for r in rows if r[1] == "PASS")
    with open(DOC, "w", encoding="utf-8") as f:
        f.write("# Tailscale 接入 E2E 验证\n\n")
        f.write(f"**Result: {ok} PASS / {fails} FAIL**\n\n")
        f.write(f"- 隔离实例 `{BASE_ISO}`：独立 tailscaled（userspace 网络、独立 socket），可安全 up/down\n")
        f.write(f"- 系统实例 `{BASE_SYS}`：接系统 tailscaled，**只读**，验证真实节点解析\n\n")
        f.write("| 用例 | 结果 | 实测 |\n|---|---|---|\n")
        for name, res, detail in rows:
            f.write(f"| {name} | {res} | {str(detail).replace('|', '/')} |\n")
    print(f"\n{ok} PASS / {fails} FAIL → {DOC}")
    sys.exit(rc)
