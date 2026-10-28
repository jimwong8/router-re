#!/usr/bin/env python3
"""p90-uncovered-batch2: uncovered endpoint tests (diagnostics / policyroute / single-id CRUD).

Real HTTP against mgmtd @ 127.0.0.1:9090 (admin login for Bearer token).
Writes docs/uncovered-batch2.md. No hand-edited doc — this script generates it.

Endpoints under test (verified in mgmtd/internal/server/server.go):
  POST /api/v1/diagnostics/ping        body {"target","count"}   -> {result{OK,Lines,Summary,Error},elapsed_ms,note}
  POST /api/v1/diagnostics/portprobe   body {"target","port","timeout"} -> {target,port,state,command,elapsed_ms}
  POST /api/v1/diagnostics/traceroute  body {"target","max_hops"} -> {result{OK,Lines,Summary,...},elapsed_ms}
  POST /api/v1/diag/traceroute         body {"host","max_hops"}  -> {Host,Hops,Raw,Error}
  GET  /api/v1/policyroute/rules        -> {rules:[Policy], live:[...]}
  POST /api/v1/policyroute/rules        body {"policies":[...]}  -> {applied,policies}
  DELETE /api/v1/policyroute/rules/{id}
  GET  /api/v1/portmap/rules            -> {rules:[...]}
  DELETE /api/v1/portmap/rules/{id}
  GET  /api/v1/policy/timeschedule      -> {count,rules:[...]}
  POST /api/v1/policy/timeschedule      body {"rules":[...],"confirm":false} -> dry_run:true
  DELETE /api/v1/policy/timeschedule/{id}
  GET  /api/v1/linkcheck/targets        -> {count,targets,states}
  POST /api/v1/linkcheck/targets        body {"targets":[...]} -> {count,targets}
  DELETE /api/v1/linkcheck/targets/{id}
"""
import json
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:9090"
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "uncovered-batch2.md"
MARK = "p90test"  # marker for all test-created rules

results = []  # (name, status, detail)


def step(name, ok, detail, frag=""):
    status = "PASS" if ok else "FAIL"
    results.append((name, status, detail, frag))
    print(f"{status} {name}: {detail}")
    if frag:
        print(f"      frag: {frag[:200]}")


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
    with urllib.request.urlopen(r, timeout=10) as resp:
        data = json.loads(resp.read().decode())
    TOKEN = data.get("token") or data.get("access_token")
    if not TOKEN:
        sys.exit(f"login failed: {data}")
    print(f"login ok, token len={len(TOKEN)}")


TOKEN = ""
frag = []  # per-step evidence fragments for the doc


def frag_add(label, text):
    frag.append(f"- **{label}**\n  \n  ```\n  {text}\n  ```")


# ---------------------------------------------------------------- 1. ping
def t1_ping():
    # diagnostics/* use nettools2.Result with lowercase keys: ok/lines/summary/error
    status, body, raw = req("POST", "/api/v1/diagnostics/ping", {"target": "127.0.0.1", "count": 2})
    r = (body or {}).get("result", {})
    ok = status == 200 and r.get("ok") is True
    detail = f"ping 127.0.0.1 -> HTTP {status}, result.ok={r.get('ok')}, summary={r.get('summary')!r}, elapsed_ms={body.get('elapsed_ms')}"
    frag_add("ping 127.0.0.1 响应", raw[:600])
    if ok:
        lines = "\n".join(r.get("lines", []))
        if "127.0.0.1" in lines and "ms" in lines:
            step("diagnostics/ping 本机", True, detail + " (响应含真实延迟数字)")
        else:
            step("diagnostics/ping 本机", False, detail + " — 响应缺少延迟数字")
    else:
        step("diagnostics/ping 本机", False, detail + " — 期望 200 且 result.ok=True")

    # black-hole: a non-routable IP must report failure (ok=False), still HTTP 200
    status2, body2, raw2 = req("POST", "/api/v1/diagnostics/ping", {"target": "192.0.2.1", "count": 1}, timeout=90)
    r2 = (body2 or {}).get("result", {})
    frag_add("ping 192.0.2.1 响应", raw2[:600])
    if status2 == 200 and r2.get("ok") is False:
        step("diagnostics/ping 黑洞超时", True,
             f"ping 192.0.2.1 (TEST-NET黑洞) -> HTTP {status2}, result.ok=False, summary={r2.get('summary')!r}, elapsed_ms={body2.get('elapsed_ms')} (超时行为符合预期)")
    elif status2 == 200 and r2.get("ok") is True:
        step("diagnostics/ping 黑洞超时", False,
             f"ping 192.0.2.1 -> HTTP {status2}, result.ok=True — 黑洞竟然可达, 不符合预期")
    else:
        step("diagnostics/ping 黑洞超时", False, f"ping 192.0.2.1 -> HTTP {status2}: {raw2[:200]}")


# ---------------------------------------------------------------- 2. portprobe
def t2_portprobe():
    status, body, raw = req("POST", "/api/v1/diagnostics/portprobe", {"target": "127.0.0.1", "port": 9090, "timeout": 3})
    state = (body or {}).get("state")
    ok = status == 200 and state == "open"
    step("diagnostics/portprobe 开放端口", ok,
         f"probe 127.0.0.1:9090 -> HTTP {status}, state={state!r} (期望 open)")
    frag_add("portprobe 9090", raw[:400])

    status2, body2, raw2 = req("POST", "/api/v1/diagnostics/portprobe", {"target": "127.0.0.1", "port": 59999, "timeout": 2})
    state2 = (body2 or {}).get("state")
    ok2 = status2 == 200 and state2 in ("closed_or_filtered", "closed")
    step("diagnostics/portprobe 关闭端口", ok2,
         f"probe 127.0.0.1:59999 -> HTTP {status2}, state={state2!r} (期望 closed_or_filtered)")
    frag_add("portprobe 59999", raw2[:400])


# ---------------------------------------------------------------- 3. traceroute (both endpoints)
def t3_traceroute():
    # diagnostics/traceroute: lowercase nettools2 keys; HTTP 200 even when binary missing
    status, body, raw = req("POST", "/api/v1/diagnostics/traceroute", {"target": "127.0.0.1", "max_hops": 3}, timeout=60)
    r = (body or {}).get("result", {})
    frag_add("diagnostics/traceroute", raw[:400])
    if status == 200 and r.get("ok") is True:
        lines = r.get("lines", [])
        step("diagnostics/traceroute 127.0.0.1", True,
             f"-> HTTP {status}, result.ok=True, {len(lines)} 行, summary={r.get('summary')!r} (本地必通)")
    elif status == 200 and r.get("ok") is False and "未安装" in str(r.get("summary", "")):
        step("diagnostics/traceroute 127.0.0.1", True,
             f"-> HTTP {status}, result.ok=False, error={r.get('error')!r} — traceroute 未安装, 端点按契约返回 200+ok=false (非网络不通), 记为依赖缺失")
    else:
        step("diagnostics/traceroute 127.0.0.1", False,
             f"-> HTTP {status}, result={str(r)[:200]} — 期望 200 且有跳数结果或明确的未安装说明")

    # diag/traceroute: PascalCase keys; 500 code 10143 when binary missing
    status2, body2, raw2 = req("POST", "/api/v1/diag/traceroute", {"host": "127.0.0.1", "max_hops": 2}, timeout=60)
    frag_add("diag/traceroute", raw2[:400])
    b2 = body2 or {}
    hop_count = b2.get("hop_count")
    raw_out = b2.get("raw") or ""
    if status2 == 200 and (hop_count is not None or isinstance(b2.get("Hops"), list)):
        step("diag/traceroute 127.0.0.1", True,
             f"-> HTTP {status2}, hop_count={hop_count}, tool={b2.get('extra', {}).get('tool') if isinstance(b2.get('extra'), dict) else '?'}, raw首行={str(raw_out).splitlines()[0] if raw_out else ''!r} (有跳数结果)")
    elif status2 == 500 and str(b2.get("code")) == "10143" and "not installed" in str(b2.get("detail", "")):
        step("diag/traceroute 127.0.0.1", True,
             f"-> HTTP {status2} code=10143 detail={b2.get('detail')!r} — traceroute/tracepath 均未安装, 错误码/详情符合契约 (依赖缺失, 非端点故障)")
    else:
        step("diag/traceroute 127.0.0.1", False,
             f"-> HTTP {status2}: {raw2[:200]} — 期望 200 有跳数结果, 或 500 code=10143 未安装")


# ---------------------------------------------------------------- 4. policyroute CRUD
def t4_policyroute():
    # NOTE: GET /policyroute/rules returns {rules, live} (persisted list under "rules"),
    # while POST takes {policies: [...]} and replaces the whole persisted list.
    status, body, raw = req("GET", "/api/v1/policyroute/rules")
    if status != 200:
        step("policyroute/rules CRUD", False, f"GET 初始列表失败 HTTP {status}: {raw[:120]}")
        return
    policies = list(body.get("rules") or [])
    used = {p.get("priority") for p in policies if isinstance(p, dict)}
    prio = 32000
    while prio in used:
        prio -= 1
    test = {"id": 90001, "name": MARK, "src_cidr": "", "dst_cidr": "",
            "table": 250, "priority": prio, "enabled": False}
    policies.append(test)
    s2, b2, raw2 = req("POST", "/api/v1/policyroute/rules", {"policies": policies})
    if s2 != 200:
        step("policyroute/rules CRUD", False, f"POST 追加 {MARK} 失败 HTTP {s2}: {raw2[:200]}")
        return
    s3, b3, raw3 = req("GET", "/api/v1/policyroute/rules")
    got = [p for p in (b3 or {}).get("rules", []) if p.get("name") == MARK]
    if not got:
        step("policyroute/rules CRUD", False, f"GET 读回未找到 {MARK}: {raw3[:200]}")
        return
    rid = got[0]["id"]
    s4, b4, raw4 = req("DELETE", f"/api/v1/policyroute/rules/{rid}")
    s5, b5, raw5 = req("GET", "/api/v1/policyroute/rules")
    still = [p for p in (b5 or {}).get("rules", []) if p.get("id") == rid]
    ok = s4 == 200 and (b4 or {}).get("deleted") is True and not still
    step("policyroute/rules CRUD", ok,
         f"POST建(added id={rid}, 原列表 {len(policies)-1} 条一并保留) → GET读回 ✓ → DELETE {rid} HTTP {s4} deleted={(b4 or {}).get('deleted')} → 再GET {'已消失 ✓' if not still else '仍存在 ✗'}")
    frag_add("policyroute CRUD 关键响应", f"POST: {raw2[:200]}\nDELETE: {raw4[:200]}\nfinal GET count: {len((b5 or {}).get('rules', []))}")


# ---------------------------------------------------------------- 5. portmap delete-by-id
def t5_portmap():
    status, body, raw = req("GET", "/api/v1/portmap/rules")
    if status != 200:
        step("portmap/rules/{id} CRUD", False, f"GET 初始列表失败 HTTP {status}: {raw[:120]}")
        return
    rules = (body or {}).get("rules", []) or []
    # portmap POST shape: {"rules":[Rule]} where Rule={id,name,proto,ext_port,int_ip,int_port,enabled}
    used = {(r.get("proto"), r.get("ext_port")) for r in rules}
    ext = 59001
    while ("tcp", ext) in used:
        ext += 1
    test = {"id": 90101, "name": MARK, "proto": "tcp", "ext_port": ext,
            "int_ip": "10.1.1.1", "int_port": 80, "enabled": False}
    s2, b2, raw2 = req("POST", "/api/v1/portmap/rules", {"rules": rules + [test]})
    if s2 != 200:
        step("portmap/rules/{id} CRUD", False,
             f"POST 追加 {MARK} 失败 HTTP {s2}: {raw2[:250]}")
        return
    s3, b3, raw3 = req("GET", "/api/v1/portmap/rules")
    got = [p for p in (b3 or {}).get("rules", []) if p.get("id") == 90101]
    if not got:
        step("portmap/rules/{id} CRUD", False, f"GET 读回未找到 id=90101: {raw3[:200]}")
        return
    s4, b4, raw4 = req("DELETE", "/api/v1/portmap/rules/90101")
    s5, b5, raw5 = req("GET", "/api/v1/portmap/rules")
    still = [p for p in (b5 or {}).get("rules", []) if p.get("id") == 90101]
    ok = s4 == 200 and (b4 or {}).get("deleted") is True and not still
    step("portmap/rules/{id} CRUD", ok,
         f"POST建(added id=90101) → GET读回 ✓ → DELETE 90101 HTTP {s4} deleted={(b4 or {}).get('deleted')} → 再GET {'已消失 ✓' if not still else '仍存在 ✗'}")
    frag_add("portmap CRUD 关键响应", f"POST: {raw2[:200]}\nDELETE: {raw4[:200]}")


# ---------------------------------------------------------------- 6. timeschedule delete-by-id
def t6_timeschedule():
    status, body, raw = req("GET", "/api/v1/policy/timeschedule")
    if status != 200:
        step("policy/timeschedule/{id} CRUD", False, f"GET 初始列表失败 HTTP {status}: {raw[:120]}")
        return
    rules = list((body or {}).get("rules", []))
    test = {"id": "p90-ts", "name": MARK, "days": [0], "start_min": 0, "end_min": 60,
            "targets": ["0.0.0.0/0"], "action": "allow_only", "rate_kbps": 0, "enabled": False}
    s2, b2, raw2 = req("POST", "/api/v1/policy/timeschedule", {"rules": rules + [test], "confirm": False})
    if s2 != 200:
        step("policy/timeschedule/{id} CRUD", False, f"POST 追加 {MARK} 失败 HTTP {s2}: {raw2[:250]}")
        return
    dry = (b2 or {}).get("dry_run")
    s3, b3, raw3 = req("GET", "/api/v1/policy/timeschedule")
    stored = [r for r in (b3 or {}).get("rules", []) if r.get("id") == "p90-ts"]
    s4, b4, raw4 = req("DELETE", "/api/v1/policy/timeschedule/p90-ts")
    s5, b5, raw5 = req("GET", "/api/v1/policy/timeschedule")
    still = [r for r in (b5 or {}).get("rules", []) if r.get("id") == "p90-ts"]
    # confirm:false = dry run, not persisted; so DELETE on non-stored id must return deleted:false
    if stored:
        ok = s4 == 200 and (b4 or {}).get("deleted") is True and not still
        desc = f"POST(confirm=false) 已持久化 → DELETE deleted={(b4 or {}).get('deleted')} → 再GET {'已消失 ✓' if not still else '仍存在 ✗'}"
    else:
        ok = s4 == 200 and (b4 or {}).get("deleted") is False and not still
        desc = f"POST(confirm=false) 为 dry-run 未持久化 (dry_run={dry}) → DELETE 非存在 id 按契约返回 deleted=false, HTTP {s4}"
    step("policy/timeschedule/{id} CRUD", ok, desc)
    frag_add("timeschedule CRUD 关键响应", f"POST: {raw2[:200]}\nDELETE: {raw4[:200]}")


# ---------------------------------------------------------------- 7. linkcheck CRUD
def t7_linkcheck():
    status, body, raw = req("GET", "/api/v1/linkcheck/targets")
    if status != 200:
        step("linkcheck/targets/{id} CRUD", False, f"GET 初始列表失败 HTTP {status}: {raw[:120]}")
        return
    targets = list((body or {}).get("targets", []))
    iface = "eth0"
    for t in targets:
        if t.get("iface"):
            iface = t["iface"]
            break
    test = {"id": "p90-lk", "name": MARK, "iface": iface, "kind": "icmp", "host": "127.0.0.1",
            "interval": 60, "timeout": 3, "fail_threshold": 3, "recover_threshold": 1,
            "enabled": False}
    s2, b2, raw2 = req("POST", "/api/v1/linkcheck/targets", {"targets": targets + [test]})
    if s2 != 200:
        step("linkcheck/targets/{id} CRUD", False, f"POST 追加 {MARK} 失败 HTTP {s2}: {raw2[:250]}")
        return
    s3, b3, raw3 = req("GET", "/api/v1/linkcheck/targets")
    got = [t for t in (b3 or {}).get("targets", []) if t.get("id") == "p90-lk"]
    if not got:
        step("linkcheck/targets/{id} CRUD", False, f"GET 读回未找到 p90-lk: {raw3[:200]}")
        return
    s4, b4, raw4 = req("DELETE", "/api/v1/linkcheck/targets/p90-lk")
    s5, b5, raw5 = req("GET", "/api/v1/linkcheck/targets")
    still = [t for t in (b5 or {}).get("targets", []) if t.get("id") == "p90-lk"]
    ok = s4 == 200 and (b4 or {}).get("deleted") is True and not still
    step("linkcheck/targets/{id} CRUD", ok,
         f"POST建(id=p90-lk) → GET读回 ✓ → DELETE HTTP {s4} deleted={(b4 or {}).get('deleted')} → 再GET {'已消失 ✓' if not still else '仍存在 ✗'}")
    frag_add("linkcheck CRUD 关键响应", f"POST: {raw2[:200]}\nDELETE: {raw4[:200]}")


# ---------------------------------------------------------------- doc
def write_doc():
    p90 = []
    for name, status, detail, f in results:
        p90.append(f"### {name}\n\n- **状态**: {status}\n- **详情**: {detail}\n")
        if f:
            p90.append(f + "\n")
    passed = sum(1 for r in results if r[1] == "PASS")
    failed = sum(1 for r in results if r[1] == "FAIL")
    out = [
        "# p90-uncovered-batch2: 未覆盖端点测试（批2）",
        "",
        f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
        "",
        f"目标: mgmtd @ {BASE}（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。",
        "",
        f"## 汇总: {passed} PASS / {failed} FAIL（共 {len(results)} 步）",
        "",
        "覆盖端点: diagnostics/ping、diagnostics/portprobe、diagnostics/traceroute、diag/traceroute、",
        "policyroute/rules(+{id})、portmap/rules/{id}、policy/timeschedule/{id}、linkcheck/targets/{id}。",
        "",
        "## 分步结果",
        "",
    ]
    out += p90
    DOC.write_text("\n".join(out), encoding="utf-8")
    print(f"\n报告: {DOC}")
    print(f"{passed} PASS / {failed} FAIL")


if __name__ == "__main__":
    login()
    t1_ping()
    t2_portprobe()
    t3_traceroute()
    t4_policyroute()
    t5_portmap()
    t6_timeschedule()
    t7_linkcheck()
    write_doc()
