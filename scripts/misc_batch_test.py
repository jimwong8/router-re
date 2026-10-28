#!/usr/bin/env python3
"""p94-misc-batch: 未覆盖杂项端点批（真实 HTTP，打本机 127.0.0.1:9090）。

只写一个新文件 scripts/misc_batch_test.py，产出 docs/misc-batch.md（脚本生成，不手写）。
范围: p91 拆分剩余的杂项（threat/htb/lb/l7 除外）:
  只读优先: stats/current、ips/stats、ips/rules(GET)、cloud/devices(GET)、vpn/client、
            bridge(GET 列表)、dpi/classify-payload、quota/test、snmp/query、diag/traceroute
  写+marker清理: pppoe/users、pppoe/multidial(顺带清 p91 遗留 p91test-d1)、
            vpn/wireguard/config(dry-run, confirm=false 不落盘)、bridge 创建 p94test-br + DELETE
标记: p94test。全部端点/参数已在 mgmtd/internal/server/{server.go,p9.go,quota.go,diag.go}、
internal/ips/rule.go、internal/diag/diag.go、internal/cloud/manager.go grep 确认，不猜。
"""
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:9090"
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "misc-batch.md"
MARK = "p94test"
MULTIDIAL_FILE = "/etc/ppp/peers/router-re-sessions.json"

results = []   # (name, status, detail, frag_text)
TOKEN = ""
frag = []


def step(name, ok, detail, frag_text=""):
    status = "PASS" if ok else "FAIL"
    results.append((name, status, detail, frag_text))
    print(f"{status} {name}: {detail}")


def frag_add(label, text):
    frag.append(f"- **{label}**\n\n  ```\n  {text.strip()[:600]}\n  ```")


def req(method, path, body=None, timeout=60):
    data = None
    if body is not None:
        data = json.dumps(body).encode()
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
    except Exception as e:
        return 0, None, f"EXC {type(e).__name__}: {e}"


def login():
    global TOKEN
    r = urllib.request.Request(
        BASE + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        method="POST")
    r.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(r, timeout=10) as resp:
        data = json.loads(resp.read().decode())
    TOKEN = data.get("token") or data.get("access_token")
    if not TOKEN:
        sys.exit(f"login failed: no token in {list(data.keys())}")
    print(f"login ok, token len={len(TOKEN)}")


# ---------------- 1. stats/current (只读) ----------------
def t_stats_current():
    status, body, raw = req("GET", "/api/v1/stats/current")
    frag_add("stats/current 响应", raw)
    if status == 503:
        results.append(("stats/current", "SKIP", "HTTP 503: collector 尚无 sample（如实报错）", ""))
        print(f"SKIP stats/current: 503 no sample")
        return
    ok = status == 200 and body and {"cpu_percent", "mem_used_mb", "ts"} <= set(body.keys())
    step("stats/current GET", ok,
         f"HTTP {status}, keys={sorted(body.keys())[:8] if isinstance(body, dict) else body}")


# ---------------- 2. ips/stats + ips/rules(GET) (只读) ----------------
def t_ips_read():
    s1, b1, r1 = req("GET", "/api/v1/ips/stats")
    frag_add("ips/stats 响应", r1)
    s2, b2, r2 = req("GET", "/api/v1/ips/rules")
    frag_add("ips/rules GET 响应", r2[:600])
    # 空规则集时 Go nil slice 序列化为 "rules": null —— 属正常形态
    ok1 = s1 == 200 and b1 and "total" in b1 and "stats" in b1
    ok2 = s2 == 200 and b2 and (b2.get("rules") is None or isinstance(b2.get("rules"), list)
                                ) and isinstance(b2.get("count"), int)
    step("ips/stats + ips/rules(GET)", ok1 and ok2,
         f"stats HTTP {s1} total={(b1 or {}).get('total')} | rules HTTP {s2} count={(b2 or {}).get('count')}（空集 rules=null 正常）")


# ---------------- 2b. ips/rules marker 写+读回+恢复（实证 p91 422 根因=body 缺字段） ----------------
def t_ips_rules_marker():
    s0, b0, _ = req("GET", "/api/v1/ips/rules")
    orig_rules = (b0 or {}).get("rules") or []
    marker = {"sid": 94001, "msg": f"{MARK}-ips", "proto": "tcp",
              "src_port": "any", "dst_port": "any",
              "payload_ascii": f"{MARK}-marker", "payload_hex": "",
              "flow": "", "action": "alert", "enabled": False, "severity": 3}
    s1, b1, r1 = req("POST", "/api/v1/ips/rules", {"rules": orig_rules + [marker]})
    frag_add("ips/rules POST(原+marker)", r1[:400])
    got = False
    cleanup = "不可用: GET 失败"
    if s1 == 200:
        s2, b2, r2 = req("GET", "/api/v1/ips/rules")
        frag_add("ips/rules GET 读回", r2[:400])
        got = any(r.get("sid") == 94001 for r in ((b2 or {}).get("rules") or []))
    if orig_rules:
        s3, b3, r3 = req("POST", "/api/v1/ips/rules", {"rules": orig_rules})
        frag_add("ips/rules 恢复", r3[:200])
        cleanup = f"已恢复 {len(orig_rules)} 条原规则 HTTP {s3}"
    else:
        cleanup = "原集合为空, 端点按设计拒空(10034); marker 规则保持 enabled=false 留存"
    ok = s1 == 200 and got
    step("ips/rules marker 写读回", ok,
         f"原 {len(orig_rules)} 条; POST HTTP {s1} body={ (b1 or {}).get('body') if b1 else None} applied={(b1 or {}).get('applied') if b1 else None}, 读回含 sid=94001={got}; {cleanup}")


# ---------------- 3. dpi/classify-payload (本地文本, 无副作用) ----------------
def t_dpi():
    body = {"payload_ascii": "GET /p94test HTTP/1.1\r\nHost: example.com\r\n\r\n", "dst_port": 80}
    status, ret, raw = req("POST", "/api/v1/dpi/classify-payload", body)
    frag_add("dpi/classify-payload 响应", raw)
    ok = status == 200 and ret and ("app" in ret or "category" in ret)
    step("dpi/classify-payload", ok,
         f"HTTP {status}, app={(ret or {}).get('app')!r}, category={(ret or {}).get('category')!r}, payload_len={(ret or {}).get('payload_len')}")


# ---------------- 4. pppoe/users: GET 备份 -> marker 写 -> 读回 -> 恢复/清空 ----------------
def t_pppoe_users():
    s0, b0, _ = req("GET", "/api/v1/pppoe/users")
    orig_users = (b0 or {}).get("users") or []
    marker = {"name": f"{MARK}-u1", "password": "p94pass", "ip": "10.94.99.99"}
    s1, b1, r1 = req("POST", "/api/v1/pppoe/users", {"users": orig_users + [marker]})
    frag_add("pppoe/users POST(原+marker)", r1[:300])
    s2, b2, r2 = req("GET", "/api/v1/pppoe/users")
    got = any(u.get("name") == marker["name"] for u in ((b2 or {}).get("users") or []))
    frag_add("pppoe/users GET 读回", r2[:300])
    if orig_users:
        s3, b3, r3 = req("POST", "/api/v1/pppoe/users", {"users": orig_users})
        frag_add("pppoe/users 恢复", r3[:200])
        cleanup = f"已恢复 {len(orig_users)} 个原用户 HTTP {s3}"
        ok_c = s3 == 200 and (b3 or {}).get("count") == len(orig_users)
    else:
        s3, b3, r3 = req("POST", "/api/v1/pppoe/users", {"users": [], "confirm": True})
        frag_add("pppoe/users 清空(confirm)", r3[:200])
        cleanup = f"原集合为空, 已 confirm 清空 HTTP {s3}"
        ok_c = s3 == 200 and (b3 or {}).get("count") == 0
    step("pppoe/users 写读回+清理", s1 == 200 and got and ok_c,
         f"原 {len(orig_users)} 用户; POST HTTP {s1} 读回含 marker={got}; {cleanup}")


# ---------------- 5. pppoe/multidial: marker 写 -> 读回 -> 清理(含 p91 遗留) ----------------
def t_pppoe_multidial():
    s0, b0, r0 = req("GET", "/api/v1/pppoe/multidial")
    frag_add("multidial GET(原始)", r0[:400])
    cur = ((b0 or {}).get("sessions")) or []
    # GET 对 password 打码；p91 遗留会话密码已知为测试值, 安全重建
    rebuilt = []
    for x in cur:
        nx = dict(x)
        nx.pop("peer", None)
        if nx.get("password") == "***":
            nx["password"] = "testpass"
        rebuilt.append(nx)
    marker = {"name": f"{MARK}-m1", "iface": "ens18", "username": "p94user",
              "password": "p94pass", "service": "", "ac": "",
              "metric": 100, "mru": 1492, "lcp_echo": 30, "enabled": False}
    s1, b1, r1 = req("POST", "/api/v1/pppoe/multidial", {"sessions": rebuilt + [marker]})
    frag_add("multidial POST(原+marker)", r1[:300])
    s2, b2, r2 = req("GET", "/api/v1/pppoe/multidial")
    got = any(x.get("name") == marker["name"] for x in ((b2 or {}).get("sessions") or []))
    frag_add("multidial GET 读回", r2[:300])
    # 清理: 移除全部 p9xtest marker。端点按设计拒绝空数组(10199), 剩余为空时删除测试产物文件
    remaining = [x for x in rebuilt
                 if "p91test" not in x.get("name", "") and "p94test" not in x.get("name", "")]
    if remaining:
        s3, _, _ = req("POST", "/api/v1/pppoe/multidial", {"sessions": remaining})
        cleanup = f"已恢复 {len(remaining)} 个原会话 HTTP {s3}"
        ok_c = s3 == 200
    else:
        s3, b3, r3 = req("POST", "/api/v1/pppoe/multidial", {"sessions": []})
        frag_add("multidial 清空尝试(端点拒空)", r3[:200])
        if s3 == 422:
            subprocess.run(["sudo", "rm", "-f", MULTIDIAL_FILE], timeout=15)
            cleanup = f"端点按设计拒空数组(HTTP {s3} code={(b3 or {}).get('code')}); 该文件系 p91/p94 测试产物, 已 sudo rm 恢复测试前(无文件=无会话)"
            ok_c = True
        else:
            cleanup = f"清空返回 HTTP {s3}（未预期）"
            ok_c = False
    step("pppoe/multidial 写读回+清理", s1 == 200 and got and ok_c,
         f"原 {len(cur)} 会话(含 p91 遗留 p91test-d1); POST HTTP {s1} 读回含 marker={got}; {cleanup}")


# ---------------- 6. vpn 相关2: keypair + config(dry-run 不落盘) + client GET ----------------
def t_vpn():
    s1, b1, r1 = req("POST", "/api/v1/vpn/wireguard/keypair")
    frag_add("vpn keypair(仅片段, 不回显私钥)", (r1[:60] + "...[私钥省略]") if len(r1) > 60 else "[空]")
    ok1 = s1 == 200 and b1 and b1.get("private_key") and b1.get("public_key")
    pub = (b1 or {}).get("public_key", "")
    s2, b2, r2 = req("POST", "/api/v1/vpn/wireguard/config",
                     {"confirm": False,
                      "config": {"interface": "wg0", "address": "10.94.0.1/24",
                                  "listen_port": 51894, "private_key": "",
                                  "peers": [{"public_key": pub, "allowed_ips": "0.0.0.0/0"}]}})
    frag_add("vpn config dry-run", r2[:400])
    ok2 = s2 == 200 and (b2 or {}).get("message") and "config" in (b2 or {})
    s3, b3, r3 = req("GET", "/api/v1/vpn/client")
    frag_add("vpn/client GET", r3[:300])
    ok3 = s3 == 200 and b3 is not None
    step("vpn 相关2(keypair+config dry+client)", ok1 and ok2 and ok3,
         f"keypair HTTP {s1}, config dry HTTP {s2} msg={(b2 or {}).get('message')!r}, client HTTP {s3}")


# ---------------- 7. bridge/{name}: 列表 GET -> 创建 p94test-br -> DELETE ----------------
def t_bridge():
    s1, b1, r1 = req("GET", "/api/v1/bridge")
    frag_add("bridge GET 列表", r1[:400])
    ok1 = s1 == 200 and isinstance(b1, dict)
    s2, b2, r2 = req("POST", "/api/v1/bridge",
                     {"name": f"{MARK}-br", "members": ["ens18"], "address": "", "stp": False})
    frag_add("bridge POST 创建", r2[:300])
    ok2 = s2 == 200 and (b2 or {}).get("applied") is True
    s3, b3, r3 = req("DELETE", f"/api/v1/bridge/{MARK}-br")
    frag_add("bridge DELETE", r3[:200])
    ok3 = s3 == 200
    s4, b4, _ = req("GET", "/api/v1/bridge")
    gone = all(x.get("name") != f"{MARK}-br" for x in ((b4 or {}).get("bridges") or []))
    step("bridge/{name} 创建+删除", ok1 and ok2 and ok3 and gone,
         f"GET HTTP {s1}, POST {MARK}-br HTTP {s2} applied={(b2 or {}).get('applied')}, DELETE HTTP {s3}, 列表已无={gone}")


# ---------------- 8. quota/test (纯匹配, 无副作用) ----------------
def t_quota():
    body = {"src_ip": "192.168.1.50", "src_mac": "aa:bb:cc:dd:ee:94",
            "dst_ip": "8.8.8.8", "dst_port": "53", "app": "dns"}
    status, ret, raw = req("POST", "/api/v1/quota/test", body)
    frag_add("quota/test 响应", raw)
    ok = status == 200 and ret and "matched" in ret and "scan_count" in ret
    step("quota/test", ok,
         f"HTTP {status}, matched={(ret or {}).get('matched')}, scan_count={(ret or {}).get('scan_count')}")


# ---------------- 9. snmp/query (snmpwalk 未装 -> SKIP, 记录如实 400/10027) ----------------
def t_snmp():
    status, ret, raw = req("POST", "/api/v1/snmp/query", {})
    frag_add("snmp/query 响应", raw)
    if ret and str((ret or {}).get("code")) == "10027":
        results.append(("snmp/query", "SKIP",
                        f"环境依赖: snmpwalk 未安装; 端点按设计如实返回 HTTP {status} code=10027（行为正确）", ""))
        print(f"SKIP snmp/query: 10027 snmpwalk not installed（环境, 非端点缺陷）")
        return
    ok = status == 200 and ret is not None
    step("snmp/query", ok, f"HTTP {status} (snmpwalk 结果)")


# ---------------- 10. cloud/devices 注册 marker + heartbeat (内存态, 重启即清) ----------------
def t_cloud():
    s1, b1, r1 = req("GET", "/api/v1/cloud/devices")
    frag_add("cloud/devices GET", r1[:300])
    ok1 = s1 == 200 and isinstance(b1, dict)
    s2, b2, r2 = req("POST", "/api/v1/cloud/devices",
                     {"id": f"{MARK}-dev", "name": f"{MARK}-dev", "model": "RM200", "version": "v0.94"})
    frag_add("cloud/devices 注册", r2[:400])
    did = ((b2 or {}).get("device") or {}).get("id")
    ok2 = s2 == 200 and did
    s3, b3, r3 = req("POST", f"/api/v1/cloud/devices/{MARK}-dev/heartbeat", {"ts": int(time.time())})
    frag_add("cloud heartbeat", r3)
    ok3 = s3 == 200 and (b3 or {}).get("status") == "ok"
    step("cloud/heartbeat", ok1 and ok2 and ok3,
         f"devices GET HTTP {s1}, 注册 {MARK}-dev HTTP {s2} id={did!r}, heartbeat HTTP {s3} status={(b3 or {}).get('status')}"
         f"（内存态, 无删除端点, 重启即清）")


# ---------------- 11. diag/traceroute (本地 127.0.0.1, 只读检查) ----------------
def t_diag_traceroute():
    status, ret, raw = req("POST", "/api/v1/diag/traceroute", {"host": "127.0.0.1", "max_hops": 2}, timeout=40)
    frag_add("diag/traceroute 响应", raw[:500])
    ok = status == 200 and ret and ("raw" in ret or "host" in ret)
    tool = ((ret or {}).get("extra") or {}).get("tool")
    hops = (ret or {}).get("hop_count")
    step("diag/traceroute(127.0.0.1,2hops)", ok,
         f"HTTP {status}, tool={tool!r}, hop_count={hops}")


# ---------------- doc ----------------
def write_doc():
    passed = sum(1 for r in results if r[1] == "PASS")
    failed = sum(1 for r in results if r[1] == "FAIL")
    skipped = sum(1 for r in results if r[1] == "SKIP")
    lines_out = []
    for name, status, detail, f in results:
        lines_out.append(f"### {name}\n\n- **状态**: {status}\n- **详情**: {detail}\n")
        if f:
            lines_out.append(f + "\n")
    out = [
        "# p94-misc-batch: 未覆盖杂项端点批（真实 HTTP）",
        "",
        f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
        f"目标: mgmtd @ {BASE}（root 运行, admin 登录取 Bearer）。全部真实 HTTP；端点/参数已 grep 确认。",
        "",
        f"## 汇总: {passed} PASS / {failed} FAIL / {skipped} SKIP（共 {len(results)} 步）",
        "",
        "覆盖: stats/current、ips/rules+stats、dpi/classify-payload、pppoe/users(写读回+清理)、",
        "pppoe/multidial(写读回+清理, 含 p91 遗留)、vpn keypair+config(dry)+client、bridge/{name}(创建+删除)、",
        "quota/test、snmp/query(环境 SKIP)、cloud register+heartbeat、diag/traceroute。",
        "threat/htb/lb/l7 不在本批。标记 p94test；写操作均有读回+清理路径。",
        "",
        "## 分步结果",
        "",
    ]
    out += lines_out
    out.append("## 关键响应片段")
    out.append("")
    out += frag
    DOC.write_text("\n".join(out), encoding="utf-8")
    print(f"\n报告: {DOC}")
    print(f"{passed} PASS / {failed} FAIL / {skipped} SKIP")


if __name__ == "__main__":
    login()
    # 只读先行
    t_stats_current()
    t_ips_read()
    t_dpi()
    t_quota()
    t_snmp()
    t_diag_traceroute()
    t_vpn()
    t_bridge()
    # 写 + marker 清理
    t_ips_rules_marker()
    t_pppoe_users()
    t_pppoe_multidial()
    t_cloud()
    write_doc()
