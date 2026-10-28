#!/usr/bin/env python3
"""p92-threat-batch: threat intel endpoints (stats/block/import/fetch) — real HTTP vs 127.0.0.1:9090.

Contracts verified in mgmtd/internal/server/server.go + threat.go + threat/feeds.go:
  GET  /api/v1/threat/stats  -> 200 {total,active,expired,permanent}
  POST /api/v1/threat/block  body {ip,reason,duration_seconds} -> 200 {entry,added} / 422 code10161
  DELETE /api/v1/threat/block body {ip} -> 200 {ip,removed}
  POST /api/v1/threat/import body {text} (每行 IP/CIDR, #注释, 坏行跳过但不静默)
        -> 200 {added,skipped,parse_note} / 空文本 422 code10166
  POST /api/v1/threat/fetch  body {url} -> 拉取并导入 -> 200 {source,added,skipped}
        / 非http(s) scheme 或 缺 host -> 422 code10167 (SSRF 防护, 不外联)
        注: 任务书写 "GET threat/fetch"，真实路由为 POST（server.go L316），按真实路由测。

测试 IP 全部用 RFC 5737 TEST-NET-1 (192.0.2.0/24) —— 保留网段，非在用地址。
fetch 用本机 127.0.0.1 临时 feed server（不碰外部网络）。
只写 docs/threat-batch.md，不 git commit。
"""
import json
import sys
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

BASE = "http://127.0.0.1:9090"
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "threat-batch.md"

# TEST-NET-1 reserved addresses — safe to use as block entries
BLK_IP = "192.0.2.90"          # manual block
IMP_IPS = ["192.0.2.91", "192.0.2.92/30"]   # import (IP + CIDR)
FETCH_IPS = ["192.0.2.93", "192.0.2.94"]    # local feed
ALL_TEST_KEYS = [BLK_IP] + IMP_IPS + FETCH_IPS

TOKEN = ""
results = []
frags = []


def frag(label, text):
    frags.append(f"**{label}**\n\n```\n{text[:500]}\n```")


def step(name, status, detail):
    results.append((name, status, detail))
    print(f"{status} {name}: {detail}")


def req(method, path, body=None, timeout=30):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    r.add_header("Authorization", f"Bearer {TOKEN}")
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            raw = resp.read().decode()
            try:
                return resp.status, json.loads(raw), raw
            except json.JSONDecodeError:
                return resp.status, None, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw), raw
        except json.JSONDecodeError:
            return e.code, None, raw


def login():
    global TOKEN
    r = urllib.request.Request(BASE + "/api/v1/auth/login",
                               data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
                               method="POST")
    r.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(r, timeout=10) as resp:
        TOKEN = json.loads(resp.read().decode())["token"]
    print(f"login ok, token len={len(TOKEN)}")


def get_stats():
    s, b, raw = req("GET", "/api/v1/threat/stats")
    return s, b or {}, raw


# ---------------------------------------------------------------- 1. stats baseline
def t1_stats_baseline():
    global BASELINE
    s, b, raw = get_stats()
    ok = s == 200 and all(k in b for k in ("total", "active", "expired", "permanent"))
    BASELINE = b
    frag("GET threat/stats 基线", raw[:300])
    step("GET threat/stats 基线", "PASS" if ok else "FAIL",
         f"HTTP {s}, baseline={b} (期望 200 且含 total/active/expired/permanent)")


BASELINE = {}

# ---------------------------------------------------------------- 2. block + stats delta
def t2_block():
    s, b, raw = req("POST", "/api/v1/threat/block",
                    {"ip": BLK_IP, "reason": "p92test", "duration_seconds": 300})
    frag("POST threat/block", raw[:300])
    ok = s == 200 and b.get("added") is True and (b.get("entry") or {}).get("ip") == BLK_IP
    step("POST threat/block", "PASS" if ok else "FAIL",
         f"HTTP {s}, added={b.get('added')}, entry.ip={b.get('entry', {}).get('ip')!r}, "
         f"source={(b.get('entry') or {}).get('source')!r} (期望 200/added=true/source=manual)")

    s2, b2, raw2 = get_stats()
    d_total = b2.get("total", 0) - BASELINE.get("total", 0)
    d_active = b2.get("active", 0) - BASELINE.get("active", 0)
    ok2 = s2 == 200 and d_total == 1 and d_active == 1
    frag("block 后 GET threat/stats", raw2[:300])
    step("GET threat/stats (block 后)", "PASS" if ok2 else "FAIL",
         f"HTTP {s2}, stats={b2}, Δtotal={d_total} Δactive={d_active} (期望各 +1)")

# ---------------------------------------------------------------- 3. import
def t3_import():
    text = "\n".join([IMP_IPS[0], IMP_IPS[1], "# 注释行 p92test", "not-an-ip"])
    s, b, raw = req("POST", "/api/v1/threat/import", {"text": text})
    frag("POST threat/import", raw[:300])
    ok = s == 200 and b.get("added") == 2 and b.get("skipped") == 1 and "跳过" in str(b.get("parse_note"))
    step("POST threat/import", "PASS" if ok else "FAIL",
         f"HTTP {s}, added={b.get('added')} skipped={b.get('skipped')} "
         f"parse_note={str(b.get('parse_note'))[:80]!r} (期望 added=2 skipped=1, 坏行不静默)")

    # empty text must be rejected with 422 code 10166
    s2, b2, raw2 = req("POST", "/api/v1/threat/import", {"text": "  \n "})
    ok2 = s2 == 422 and str(b2.get("code")) == "10166"
    frag("import 空文本被拒", raw2[:200])
    step("POST threat/import 空文本", "PASS" if ok2 else "FAIL",
         f"HTTP {s2}, code={b2.get('code')} (期望 422/10166)")

    s3, b3, raw3 = get_stats()
    d_total = b3.get("total", 0) - BASELINE.get("total", 0)
    step("GET threat/stats (import 后)", "PASS" if s3 == 200 and d_total == 3 else "FAIL",
         f"HTTP {s3}, Δtotal={d_total} (期望累计 +3: block+import)")


# ---------------------------------------------------------------- 4. fetch (local feed, no external)
def t4_fetch():
    feed_body = "\n".join(FETCH_IPS) + "\n"

    class H(BaseHTTPRequestHandler):
        def do_GET(self):
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(feed_body)))
            self.end_headers()
            self.wfile.write(feed_body.encode())

        def log_message(self, *a):
            pass

    srv = HTTPServer(("127.0.0.1", 0), H)
    port = srv.server_address[1]
    th = threading.Thread(target=srv.serve_forever, daemon=True)
    th.start()
    try:
        url = f"http://127.0.0.1:{port}/p92feed"
        s, b, raw = req("POST", "/api/v1/threat/fetch", {"url": url}, timeout=40)
        frag("POST threat/fetch (本机 feed)", raw[:300])
        ok = s == 200 and b.get("added") == 2 and b.get("source") == url
        step("POST threat/fetch (本机127.0.0.1 feed)", "PASS" if ok else "FAIL",
             f"HTTP {s}, added={b.get('added')} source={b.get('source')!r} (期望 200/added=2, 未外联)")

        # SSRF guard: file:// scheme must be rejected, no external call
        s2, b2, raw2 = req("POST", "/api/v1/threat/fetch", {"url": "file:///etc/passwd"})
        ok2 = s2 == 422 and str(b2.get("code")) == "10167"
        frag("fetch SSRF 防护 (file:// 被拒)", raw2[:300])
        step("POST threat/fetch SSRF 防护", "PASS" if ok2 else "FAIL",
             f"HTTP {s2}, code={b2.get('code')} detail={str(b2.get('detail'))[:80]!r} (期望 422/10167 拒绝 file://)")
    finally:
        srv.shutdown()

    s3, b3, raw3 = get_stats()
    d_total = b3.get("total", 0) - BASELINE.get("total", 0)
    step("GET threat/stats (fetch 后)", "PASS" if s3 == 200 and d_total == 5 else "FAIL",
         f"HTTP {s3}, Δtotal={d_total} (期望累计 +5: 1 block + 2 import + 2 fetch)")


# ---------------------------------------------------------------- 5. cleanup
def t5_cleanup():
    leftover = []
    for key in ALL_TEST_KEYS:
        s, b, raw = req("DELETE", "/api/v1/threat/block", {"ip": key})
        ok = s == 200 and b.get("removed") is True
        if not ok:
            leftover.append(f"{key}: HTTP {s} removed={b.get('removed')}")
        frag(f"DELETE {key}", raw[:200])
    s, b, raw = get_stats()
    frag("最终 GET threat/stats", raw[:300])
    clean = (b.get("total") == BASELINE.get("total") and b.get("active") == BASELINE.get("active")
             and b.get("expired") == BASELINE.get("expired") and b.get("permanent") == BASELINE.get("permanent"))
    ok = not leftover and clean
    detail = f"{len(ALL_TEST_KEYS)} 条测试条目全部 removed=true, 最终 stats={b} 与基线 {BASELINE} 一致" if ok else \
        f"残留: {leftover or '无'}, stats={b} vs baseline={BASELINE}"
    step("清理 (DELETE 全部测试条目)", "PASS" if ok else "FAIL", detail)


# ---------------------------------------------------------------- doc
def write_doc():
    passed = sum(1 for r in results if r[1] == "PASS")
    failed = sum(1 for r in results if r[1] == "FAIL")
    out = [
        "# p92-threat-batch: 威胁情报端点测试",
        "",
        f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
        "",
        f"目标: mgmtd @ {BASE}（root 运行，admin 登录取 Bearer）。全部真实 HTTP 调用。",
        "",
        f"## 汇总: {passed} PASS / {failed} FAIL / 0 SKIP（共 {len(results)} 步）",
        "",
        "覆盖端点: GET threat/stats、POST threat/block、DELETE threat/block、POST threat/import、",
        "POST threat/fetch（本机 127.0.0.1 feed，零外联；任务书写的 GET 实为 POST，按 server.go L316 真实路由测）。",
        "fetch 无需 SKIP：外部源可换成临时本机 feed；另验 SSRF 防护（file:// 被 422 拒绝）。",
        "测试 IP 全部为 RFC 5737 TEST-NET-1 (192.0.2.0/24) 保留地址。",
        "",
        "## 分步结果",
        "",
    ]
    for name, status, detail in results:
        out.append(f"### {name}\n\n- **状态**: {status}\n- **详情**: {detail}\n")
    out.append("## 证据片段\n")
    out.extend(frags)
    DOC.write_text("\n".join(out), encoding="utf-8")
    print(f"\n报告: {DOC}")
    print(f"{passed} PASS / {failed} FAIL")


if __name__ == "__main__":
    login()
    t1_stats_baseline()
    t2_block()
    t3_import()
    t4_fetch()
    t5_cleanup()
    write_doc()
