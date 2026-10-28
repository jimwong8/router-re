#!/usr/bin/env python3
"""WAN 接入模块冒烟：pppoe/wan/multiwan/upnp/portmap/porttrigger。

真实打本机 9090，带 token。读端点断言 200+字段；写端点空 body 断言 422；
需真环境的端点标 SKIP。报告: docs/wan-access-smoke.md (脚本生成)。
"""
import json, os, time, urllib.request, urllib.error
from pathlib import Path
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "wan-access-smoke.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")

def call(method, path, token=None, body=None, timeout=20):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token: req.add_header("Authorization", f"Bearer {token}")
    if data: req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            try: return r.status, json.loads(raw), time.time()-t0, raw
            except: return r.status, raw, time.time()-t0, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try: return e.code, json.loads(raw), time.time()-t0, raw
        except: return e.code, raw, time.time()-t0, raw
    except Exception as e:
        return 0, f"{type(e).__name__}: {e}", time.time()-t0, str(e)

def login():
    st, body, _, _ = call("POST", "/api/v1/auth/login", body={"username":"admin","password":"admin123"})
    if st != 200:
        raise SystemExit(f"login failed {st}: {body}")
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]

def short(x, n=200):
    s = json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n-3]+"..."

token = login()
results = []
def rec(name, status, detail, evidence):
    results.append({"name": name, "status": status, "detail": detail, "evidence": evidence})
    print(f"{status:4} {name}: {detail}")

# ---- 1. 读端点 ----
read_checks = [
    ("wan", "/api/v1/wan/links", ["links"]),
    ("wan", "/api/v1/wan/status", None),
    ("multiwan", "/api/v1/multiwan", None),
    ("upnp", "/api/v1/upnp/mappings", ["mappings"]),
    ("upnp", "/api/v1/upnp/status", None),
    ("upnp", "/api/v1/upnp/config", None),
    ("portmap", "/api/v1/portmap/rules", ["rules"]),
    ("pppoe", "/api/v1/pppoe/status", None),
    ("pppoe", "/api/v1/pppoe/sessions", ["sessions", "count"]),
    ("pppoe", "/api/v1/pppoe/config", None),
    ("porttrigger", "/api/v1/porttrigger", None),
]
for mod, path, keys in read_checks:
    st, body, _, raw = call("GET", path, token=token)
    ok = (st == 200)
    if ok and isinstance(body, dict) and keys:
        missing = [k for k in keys if k not in body]
        if missing:
            ok = False
            note = f"HTTP {st} 但缺字段 {missing}"
        else:
            note = f"HTTP {st}, 字段 {keys} 存在"
    else:
        note = f"HTTP {st}"
    rec(f"[{mod}] GET {path}", "PASS" if ok else "FAIL", note, short(body))

# ---- 2. upnp/portmap 结构验证 ----
for path, listkey, itemkeys in [("/api/v1/upnp/mappings", "mappings", ["protocol", "lan_ip", "external_port", "internal_port"]),
                                 ("/api/v1/portmap/rules", "rules", None)]:
    st, body, _, _ = call("GET", path, token=token)
    if st == 200 and isinstance(body, dict):
        items = body.get(listkey) or []
        if items:
            sample = items[0]
            if itemkeys:
                missing = [k for k in itemkeys if k not in sample]
                ok = (len(missing) == 0)
                rec(f"结构验证 {path}", "PASS" if ok else "FAIL",
                    f"{len(items)} 条, 示例缺字段 {missing}" if missing else f"{len(items)} 条, 字段齐全",
                    short(sample))
            else:
                rec(f"结构验证 {path}", "PASS", f"{len(items)} 条", short(sample))
        else:
            rec(f"结构验证 {path}", "PASS", "列表为空（无映射），端点正常", short(body))

# ---- 3. 写端点空 body → 期望 422 ----
write_checks = [
    ("wan", "POST /api/v1/wan/links", "/api/v1/wan/links", {}),
    ("multiwan", "POST /api/v1/multiwan", "/api/v1/multiwan", {}),
    ("upnp", "POST /api/v1/upnp/mappings", "/api/v1/upnp/mappings", {}),
    ("portmap", "POST /api/v1/portmap/rules", "/api/v1/portmap/rules", {}),
    ("pppoe", "POST /api/v1/pppoe/config", "/api/v1/pppoe/config", {}),
    ("porttrigger", "POST /api/v1/porttrigger", "/api/v1/porttrigger", {}),
]
for mod, name, path, body in write_checks:
    st, resp, _, raw = call("POST", path, token=token, body=body)
    code = resp.get("code") if isinstance(resp, dict) else None
    if st == 422:
        rec(f"[{mod}] {name} 空 body", "PASS", f"HTTP 422 code={code}", short(resp))
    elif st in (200, 201):
        rec(f"[{mod}] {name} 空 body", "FAIL", f"空 body 被接受 HTTP {st}（Validate 未拦）", short(resp))
    else:
        rec(f"[{mod}] {name} 空 body", "FAIL", f"HTTP {st} code={code}", short(resp))

# ---- 4. 需真环境的端点 → SKIP ----
skips = [
    ("pppoe", "pppoe 真拨号", "无 connect/dial 端点；pppoe 需真实 PPPoE 环境，本机无"),
    ("upnp", "upnp 真发现", "无 discover 端点；upnp 需真实网关，本机无"),
    ("portmap", "portmap 真规则", "portmap/rules POST 为 apply，需真实 nft 环境；空 body 已测 Validate"),
    ("multiwan", "multiwan/probe 真探测", "multiwan/probe 需真实多 WAN 出口；空 body 已测 Validate"),
]
for mod, name, reason in skips:
    rec(f"[{mod}] {name}", "SKIP", reason, "N/A")

# ---- report ----
os.makedirs(ROOT / "docs", exist_ok=True)
lines = []
lines.append("# WAN 接入模块冒烟报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
n_pass = sum(1 for r in results if r["status"] == "PASS")
n_fail = sum(1 for r in results if r["status"] == "FAIL")
n_skip = sum(1 for r in results if r["status"] == "SKIP")
lines.append(f"**汇总**: {len(results)} 步 | PASS {n_pass} / FAIL {n_fail} / SKIP {n_skip}")
lines.append("")
lines.append("| # | 步骤 | 结果 | 说明 |")
lines.append("|---|------|------|------|")
for i, r in enumerate(results, 1):
    lines.append(f"| {i} | {r['name']} | {r['status']} | {r['detail']} |")
lines.append("")
for i, r in enumerate(results, 1):
    lines.append(f"## {i}. {r['name']}")
    lines.append("")
    lines.append(f"- 结果: **{r['status']}** — {r['detail']}")
    lines.append("")
    lines.append("```")
    lines.append(r["evidence"])
    lines.append("```")
    lines.append("")

with open(DOC, "w") as f:
    f.write("\n".join(lines))
print(f"report: {DOC}")
print(f"SUMMARY: PASS={n_pass} FAIL={n_fail} SKIP={n_skip}")
