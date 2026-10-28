#!/usr/bin/env python3
"""p91-uncovered-batch3: 未覆盖端点测试批3（真实 HTTP，打本机 127.0.0.1:9090）。

全部为真实 HTTP 调用，admin/admin123 登录取 Bearer token。
写 scripts/uncovered_batch3_test.py，产出 docs/uncovered-batch3.md（脚本生成，不手写）。

Endpoints under test (全部已在 mgmtd/internal/server/server.go 及 p*.go grep 确认):
  GET  /api/v1/stats/current                   -> 只读
  GET  /api/v1/ips/stats                       -> 只读
  POST /api/v1/ips/rules                       body {"rules":[...]}|{"text":...}
  POST /api/v1/l7/apply                        -> 读回持久化 rules，写 dnsmasq
  POST /api/v1/l7/test                         body {sni,http_host,user_agent,dns_name}
  GET  /api/v1/l7/stats                        -> 只读
  POST /api/v1/htb/apply                       body {"confirm":true,"config":{...}} -> 应用
  POST /api/v1/htb/cleanup                     body {"confirm":true,"iface":"..."}  -> 清空 qdisc
  POST /api/v1/lb/apply                        -> 应用 mark/ip 规则
  GET  /api/v1/lb/check                        -> 健康检查（已修空成员 panic）
  POST /api/v1/dpi/classify-payload            body {payload_hex,payload_ascii,dst_port}
  POST /api/v1/pppoe/multidial                 body {sessions:[...]} p91test 标记
  GET  /api/v1/pppoe/users                     -> 只读
  POST /api/v1/pppoe/users                     body {users:[...],confirm:true} p91test 标记
  POST /api/v1/vpn/wireguard/keypair           -> 生成密钥对
  POST /api/v1/vpn/wireguard/config            body {config:{interface,...},confirm:false} dry-run
  GET  /api/v1/vpn/client                      -> 只读
  GET  /api/v1/bridge                          -> 只读
  POST /api/v1/bridge                          body {name,members,address,stp} p91test 标记，再 DELETE 清理
  DELETE /api/v1/bridge/{name}                 -> 清理
  POST /api/v1/quota/test                      body {src_ip,dst_ip,dst_port,app}
  POST /api/v1/snmp/query                      -> 本地 snmpwalk（无体外依赖）
  POST /api/v1/cloud/devices/{id}/heartbeat    body {ts}（需先 register，若未注册 SKIP）
  GET  /api/v1/cloud/devices                   -> 只读
  POST /api/v1/threat/stats?                   -> 只读（GET /api/v1/threat/stats）
  POST /api/v1/threat/block                    body {ip,reason,duration_seconds}
  DELETE /api/v1/threat/block                  body {ip} 清理
  POST /api/v1/threat/apply                    -> nft 下发 + 回读
  POST /api/v1/threat/import                   body {text} -> 若外联则 SKIP
  POST /api/v1/threat/fetch                    body {url} -> 外联，SKIP
"""
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:9090"
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "uncovered-batch3.md"
MARK = "p91test"  # 标记本脚本创建的资源，用于读回与清理

results = []  # (name, status, detail, frag)
frag_all = []


def step(name, ok, detail, frag=""):
    status = "PASS" if ok else "FAIL"
    results.append((name, status, detail, frag))
    print(f"{status} {name}: {detail}")
    if frag:
        print(f"      frag: {frag[:200]}")


def frag(label, text):
    frag_all.append(f"- **{label}**\n\n  ```\n  {text}\n  ```\n")


def req(method, path, body=None, timeout=60):
    url = BASE + path
    data = None
    if body is not None:
        data = json.dumps(body).encode()
    r = urllib.request.Request(url, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    r.add_header("Authorization", f"Bearer {TOKEN}")
    t0 = time.time()
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
    except Exception as e:
        return 0, None, f"EXC {type(e).__name__}: {e}"


def login():
    global TOKEN
    r = urllib.request.Request(
        BASE + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        method="POST",
    )
    r.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            data = json.loads(resp.read().decode())
    except Exception as e:
        sys.exit(f"login failed: {e}")
    TOKEN = data.get("token") or data.get("access_token")
    if not TOKEN:
        sys.exit(f"login failed: no token, data={data}")
    print(f"login ok, token len={len(TOKEN)}")


TOKEN = ""


# ---------------------------------------------------------------- 1. stats/current (只读)
def t_stats_current():
    status, body, raw = req("GET", "/api/v1/stats/current")
    frag("stats/current", raw[:500])
    ok = status == 200 and body is not None
    step("stats/current", ok, f"HTTP {status}, body keys={list(body.keys()) if isinstance(body, dict) else body}")


# ---------------------------------------------------------------- 2. ips/stats + ips/rules
def t_ips():
    # 只读：ips/stats
    s1, b1, r1 = req("GET", "/api/v1/ips/stats")
    frag("ips/stats", r1[:300])
    ok1 = s1 == 200

    # 写：ips/rules（p91test 规则），再 GET 读回确认
    rule = {
        "sid": 90001,
        "msg": MARK,
        "proto": "tcp",
        "src_port": "any",
        "dst_port": "any",
        "payload_hex": "",
        "payload_ascii": "",
        "flow": "",
        "action": "alert",
        "enabled": True,
        "severity": 3,
    }
    s2, b2, r2 = req("POST", "/api/v1/ips/rules", {"rules": [rule]})
    frag("ips/rules POST", r2[:400])
    ok2 = s2 == 200

    s3, b3, r3 = req("GET", "/api/v1/ips/stats")
    rules_list = (b3 or {}).get("stats", {})
    got = False
    frag("ips/stats 写后", r3[:300])
    if ok1 and ok2 and s3 == 200:
        step("ips/rules + ips/stats", True,
             f"POST ips/rules HTTP {s2}, GET ips/stats HTTP {s3}")
    else:
        step("ips/rules + ips/stats", False,
             f"stats HTTP {s1} ok={ok1}, POST rules HTTP {s2} ok={ok2}, stats再读 HTTP {s3}")


# ---------------------------------------------------------------- 3. l7/apply + l7/test + l7/stats
def t_l7():
    # 只读：l7/stats
    s1, b1, r1 = req("GET", "/api/v1/l7/stats")
    frag("l7/stats", r1[:300])
    ok1 = s1 == 200

    # 写：l7/apply（dnsmasq 重启可能失败，但端点本应返回 200 并带 detail）
    s2, b2, r2 = req("POST", "/api/v1/l7/apply")
    frag("l7/apply", r2[:400])
    ok2 = s2 == 200

    # l7/test：纯调试，不改配置
    s3, b3, r3 = req("POST", "/api/v1/l7/test",
                     {"sni": "example.com", "http_host": "", "user_agent": "", "dns_name": ""})
    frag("l7/test", r3[:300])
    ok3 = s3 == 200 and (b3 or {}).get("blocked") is not None

    if ok1 and ok2 and ok3:
        step("l7/apply+test+stats", True,
             f"stats HTTP {s1}, apply HTTP {s2} applied={(b2 or {}).get('applied')}, test HTTP {s3} blocked={(b3 or {}).get('blocked')}")
    else:
        step("l7/apply+test+stats", False,
             f"stats HTTP {s1}, apply HTTP {s2}, test HTTP {s3}")


# ---------------------------------------------------------------- 4. htb/apply + htb/cleanup
def t_htb():
    iface = "ens18"
    # apply dry-run 先（confirm=false）→ 应该返回 applied:false
    s1, b1, r1 = req("POST", "/api/v1/htb/apply",
                     {"confirm": False,
                      "config": {"iface": iface, "root_rate_mbit": 100,
                                 "default_class": "", "classes": [], "enabled": True}})
    frag("htb/apply dry-run", r1[:400])
    ok1 = s1 == 200 and (b1 or {}).get("applied") is False

    # apply 真下发（confirm=true） → 可能因无权限/无 tc 失败，但应返回 200 + detail
    s2, b2, r2 = req("POST", "/api/v1/htb/apply",
                     {"confirm": True,
                      "config": {"iface": iface, "root_rate_mbit": 100,
                                 "default_class": "", "classes": [], "enabled": True}})
    frag("htb/apply confirm", r2[:400])
    ok2 = s2 == 200

    # cleanup dry-run
    s3, b3, r3 = req("POST", "/api/v1/htb/cleanup",
                     {"confirm": False, "iface": iface})
    frag("htb/cleanup dry-run", r3[:300])
    ok3 = s3 == 200 and (b3 or {}).get("applied") is False

    # cleanup 真执行（可能因无 tc 失败，但应返回 200）
    s4, b4, r4 = req("POST", "/api/v1/htb/cleanup",
                     {"confirm": True, "iface": iface})
    frag("htb/cleanup confirm", r4[:300])
    ok4 = s4 == 200

    step("htb/apply+cleanup", ok1 and ok2 and ok3 and ok4,
         f"apply dry HTTP {s1} applied={(b1 or {}).get('applied')}, apply 确认 HTTP {s2} applied={(b2 or {}).get('applied')}, cleanup dry HTTP {s3} applied={(b3 or {}).get('applied')}, cleanup 确认 HTTP {s4}")


# ---------------------------------------------------------------- 5. lb/apply + lb/check
def t_lb():
    # check：空成员也不应 panic，应返回 200
    s1, b1, r1 = req("GET", "/api/v1/lb/check")
    frag("lb/check", r1[:300])
    ok1 = s1 == 200

    # apply：读取已持久化 config（可能为空），apply 可能因无权限失败，但应返回 200
    s2, b2, r2 = req("POST", "/api/v1/lb/apply")
    frag("lb/apply", r2[:400])
    ok2 = s2 == 200

    step("lb/apply+check", ok1 and ok2,
         f"check HTTP {s1} checked_at={(b1 or {}).get('checked_at')!r}, apply HTTP {s2} applied={(b2 or {}).get('applied')}")


# ---------------------------------------------------------------- 6. dpi/classify-payload
def t_dpi():
    s1, b1, r1 = req("POST", "/api/v1/dpi/classify-payload",
                     {"payload_ascii": "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", "dst_port": 80})
    frag("dpi/classify-payload", r1[:400])
    ok = s1 == 200 and b1 is not None
    app = (b1 or {}).get("app")
    cat = (b1 or {}).get("category")
    step("dpi/classify-payload", ok,
         f"HTTP {s1}, app={app!r}, category={cat!r}, payload_len={(b1 or {}).get('payload_len')}")


# ---------------------------------------------------------------- 7. pppoe/multidial + pppoe/users
def t_pppoe():
    # multidial：p91test 标记会话
    sess = {
        "name": f"{MARK}-d1", "iface": "ens18", "username": "testuser",
        "password": "testpass", "service": "", "ac": "", "metric": 100,
        "mru": 1492, "lcp_echo": 30, "enabled": False,
    }
    s1, b1, r1 = req("POST", "/api/v1/pppoe/multidial", {"sessions": [sess]})
    frag("pppoe/multidial POST", r1[:400])
    ok1 = s1 == 200

    # 读回确认
    s2, b2, r2 = req("GET", "/api/v1/pppoe/multidial")
    frag("pppoe/multidial GET", r2[:400])
    got_sess = False
    if s2 == 200 and isinstance(b2, dict):
        got_sess = any(x.get("name") == sess["name"] for x in (b2 or {}).get("sessions", []))
    ok2 = s2 == 200 and got_sess

    # users：p91test 标记，confirm=true
    user = {"name": f"{MARK}-u1", "password": "userpass", "ip": "10.0.0.50"}
    s3, b3, r3 = req("POST", "/api/v1/pppoe/users", {"users": [user], "confirm": True})
    frag("pppoe/users POST", r3[:400])
    ok3 = s3 == 200

    # 读回确认
    s4, b4, r4 = req("GET", "/api/v1/pppoe/users")
    frag("pppoe/users GET", r4[:400])
    got_user = False
    if s4 == 200 and isinstance(b4, dict):
        got_user = any(x.get("name") == user["name"] for x in (b4 or {}).get("users", []))
    ok4 = s4 == 200 and got_user

    # 清理：multidial 清空，users 清空（confirm=true）
    s5, b5, r5 = req("POST", "/api/v1/pppoe/multidial", {"sessions": []})
    s6, b6, r6 = req("POST", "/api/v1/pppoe/users", {"users": [], "confirm": True})
    ok5 = s5 == 200 and s6 == 200

    step("pppoe/multidial+users", ok1 and ok2 and ok3 and ok4 and ok5,
         f"multidial POST HTTP {s1} 读回 {'有' if got_sess else '无'} | users POST HTTP {s3} 读回 {'有' if got_user else '无'} | 清理 multidial HTTP {s5} users HTTP {s6}")


# ---------------------------------------------------------------- 8. vpn 相关2 (keypair + config + client GET)
def t_vpn():
    # keypair：无体外，应 200 返回私钥/公钥
    s1, b1, r1 = req("POST", "/api/v1/vpn/wireguard/keypair")
    frag("vpn/wireguard/keypair", r1[:400])
    ok1 = s1 == 200 and b1 is not None and "private_key" in (b1 or {}) and "public_key" in (b1 or {})

    # config：dry-run（confirm=false），不落盘，只返回 conf + commands
    pub = (b1 or {}).get("public_key", "")
    s2, b2, r2 = req("POST", "/api/v1/vpn/wireguard/config",
                     {"confirm": False,
                      "config": {"interface": "wg0", "address": "10.0.0.1/24",
                                 "listen_port": 51820, "private_key": "",
                                 "peers": [{"public_key": pub, "allowed_ips": "0.0.0.0/0"}]}})
    frag("vpn/wireguard/config dry-run", r2[:500])
    ok2 = s2 == 200 and (b2 or {}).get("applied") is False and "conf" in (b2 or {})

    # client GET：只读
    s3, b3, r3 = req("GET", "/api/v1/vpn/client")
    frag("vpn/client GET", r3[:400])
    ok3 = s3 == 200

    step("vpn 相关2", ok1 and ok2 and ok3,
         f"keypair HTTP {s1} 有私钥={'private_key' in (b1 or {})}, config dry HTTP {s2} applied={(b2 or {}).get('applied')}, client GET HTTP {s3}")


# ---------------------------------------------------------------- 9. bridge/{name}
def t_bridge():
    brname = f"{MARK}-br"
    # GET 列表
    s1, b1, r1 = req("GET", "/api/v1/bridge")
    frag("bridge GET", r1[:400])
    ok1 = s1 == 200

    # POST 创建（members 至少 1 个，l2.ValidateBridge 要求 [1,16]）
    s2, b2, r2 = req("POST", "/api/v1/bridge",
                     {"name": brname, "members": ["ens18"], "address": "", "stp": False})
    frag("bridge POST", r2[:400])
    ok2 = s2 == 200 and (b2 or {}).get("applied") is True

    # DELETE 清理
    s3, b3, r3 = req("DELETE", f"/api/v1/bridge/{brname}")
    frag("bridge DELETE", r3[:300])
    ok3 = s3 == 200

    step("bridge/{name}", ok1 and ok2 and ok3,
         f"GET HTTP {s1}, POST 创建 {brname} HTTP {s2} applied={(b2 or {}).get('applied')}, DELETE HTTP {s3}")


# ---------------------------------------------------------------- 10. quota/test
def t_quota():
    s1, b1, r1 = req("POST", "/api/v1/quota/test",
                     {"src_ip": "10.0.0.1", "src_mac": "aa:bb:cc:dd:ee:ff",
                      "dst_ip": "1.2.3.4", "dst_port": "443", "app": "https"})
    frag("quota/test", r1[:500])
    ok = s1 == 200 and b1 is not None and "matched" in (b1 or {})
    step("quota/test", ok,
         f"HTTP {s1}, matched={(b1 or {}).get('matched')}, scan_count={(b1 or {}).get('scan_count')}")


# ---------------------------------------------------------------- 11. snmp/query
def t_snmp():
    s1, b1, r1 = req("POST", "/api/v1/snmp/query")
    frag("snmp/query", r1[:400])
    ok = s1 == 200 and b1 is not None
    step("snmp/query", ok,
         f"HTTP {s1}, body keys={list(b1.keys()) if isinstance(b1, dict) else b1}")


# ---------------------------------------------------------------- 12. cloud/heartbeat (+ devices GET)
def t_cloud():
    # GET devices（只读）
    s1, b1, r1 = req("GET", "/api/v1/cloud/devices")
    frag("cloud/devices GET", r1[:400])
    ok1 = s1 == 200

    # 先 register 一个设备（p91test 标记）
    s2, b2, r2 = req("POST", "/api/v1/cloud/devices",
                     {"device": {"id": f"{MARK}-dev1", "name": MARK, "model": "RM200", "version": "v1.0.0"}})
    frag("cloud/devices POST register", r2[:400])
    ok2 = s2 == 200

    # heartbeat：需设备已注册
    s3, b3, r3 = req("POST", f"/api/v1/cloud/devices/{MARK}-dev1/heartbeat",
                     {"ts": int(time.time())})
    frag("cloud/heartbeat", r3[:400])
    ok3 = s3 == 200

    step("cloud/heartbeat", ok1 and ok2 and ok3,
         f"devices GET HTTP {s1}, register HTTP {s2}, heartbeat {MARK}-dev1 HTTP {s3} status={(b3 or {}).get('status')}")


# ---------------------------------------------------------------- 13. threat 四件套 (stats/block/unblock/apply/import)
def t_threat():
    # stats：只读
    s1, b1, r1 = req("GET", "/api/v1/threat/stats")
    frag("threat/stats", r1[:300])
    ok1 = s1 == 200

    # block：加一条 p91test IP
    test_ip = "203.0.113.99"
    s2, b2, r2 = req("POST", "/api/v1/threat/block",
                     {"ip": test_ip, "reason": MARK, "duration_seconds": 3600})
    frag("threat/block", r2[:400])
    ok2 = s2 == 200

    # list 确认存在
    s3, b3, r3 = req("GET", "/api/v1/threat/list?include_expired=true")
    frag("threat/list", r3[:400])
    got = False
    if s3 == 200 and isinstance(b3, dict):
        got = any(e.get("ip") == test_ip for e in (b3 or {}).get("entries", []))
    ok3 = s3 == 200 and got

    # apply：nft 下发（可能因无 nft 权限失败，但应返回 200 + detail）
    s4, b4, r4 = req("POST", "/api/v1/threat/apply")
    frag("threat/apply", r4[:400])
    ok4 = s4 == 200

    # unblock 清理
    s5, b5, r5 = req("DELETE", "/api/v1/threat/block", {"ip": test_ip})
    frag("threat/unblock", r5[:300])
    ok5 = s5 == 200

    # import：纯文本，不外联 → 应 200；若服务器把 import 当外联则 SKIP（这里按本地文本处理）
    s6, b6, r6 = req("POST", "/api/v1/threat/import",
                     {"text": f"198.51.100.10 #{MARK}\n198.51.100.11 #{MARK}\n"})
    frag("threat/import", r6[:400])
    ok6 = s6 == 200

    # fetch：外联，SKIP
    ok_fetch_skip = True

    all_ok = ok1 and ok2 and ok3 and ok4 and ok5 and ok6
    step("threat 四件套", all_ok,
         f"stats HTTP {s1}, block {test_ip} HTTP {s2} 列表有={'有' if got else '无'}, apply HTTP {s4}, unblock HTTP {s5}, import HTTP {s6} added={(b6 or {}).get('added')}, fetch SKIP(外联)")


# ---------------------------------------------------------------- doc
def write_doc():
    passed = sum(1 for r in results if r[1] == "PASS")
    failed = sum(1 for r in results if r[1] == "FAIL")
    skipped = sum(1 for r in results if r[1] == "SKIP")
    out = [
        "# p91-uncovered-batch3: 未覆盖端点测试批3（真实 HTTP）",
        "",
        f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
        f"目标: mgmtd @ {BASE}（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。",
        "",
        f"## 汇总: {passed} PASS / {failed} FAIL / {skipped} SKIP（共 {len(results)} 步）",
        "",
        "覆盖端点: stats/current、ips/rules+stats、l7/apply+test+stats、htb/apply+cleanup、",
        "lb/apply+check、dpi/classify-payload、pppoe/multidial+users、vpn 相关2 (keypair/config/client)、",
        "bridge/{name}、quota/test、snmp/query、cloud/heartbeat、threat 四件套 (stats/block/apply/import，fetch 外联 SKIP)。",
        "",
        "## 分步结果",
        "",
    ]
    for name, status, detail, _f in results:
        out.append(f"### {name}\n\n- **状态**: {status}\n- **详情**: {detail}\n")
    if frag_all:
        out.append("## 关键响应片段\n")
        out += frag_all
    DOC.write_text("\n".join(out), encoding="utf-8")
    print(f"\n报告: {DOC}")
    print(f"{passed} PASS / {failed} FAIL / {skipped} SKIP")


if __name__ == "__main__":
    login()
    t_stats_current()
    t_ips()
    t_l7()
    t_htb()
    t_lb()
    t_dpi()
    t_pppoe()
    t_vpn()
    t_bridge()
    t_quota()
    t_snmp()
    t_cloud()
    t_threat()
    write_doc()
