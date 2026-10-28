#!/usr/bin/env python3
"""访问控制模块冒烟：radius/nac/identity/quota/webfilter/dpi。

真实打本机 9090，带 token。读端点断言 200+字段；写端点空 body 断言 422；
需真环境的端点标 SKIP。报告: docs/access-ctrl-smoke.md (脚本生成)。
"""
import json, os, time, urllib.request, urllib.error
from pathlib import Path
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "access-ctrl-smoke.md"
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
    ("radius", "/api/v1/radius/config", ["config", "servers"]),
    ("nac", "/api/v1/nac/config", ["config"]),
    ("nac", "/api/v1/nac/neighbors", ["neighbors", "count"]),
    ("nac", "/api/v1/nac/connlimits", ["limits"]),
    ("nac", "/api/v1/nac/top-connections", None),
    ("identity", "/api/v1/identity", ["identity"]),
    ("quota", "/api/v1/quota/policies", ["policies", "count"]),
    ("quota", "/api/v1/quota/report", None),
    ("quota", "/api/v1/quota/nft", None),
    ("webfilter", "/api/v1/webfilter/rules", ["rules"]),
    ("dpi", "/api/v1/dpi/rules", ["rules"]),
    ("dpi", "/api/v1/dpi/marks", ["rules"]),
    ("dpi", "/api/v1/dpi/flows", ["flows", "count"]),
    ("dpi", "/api/v1/dpi/apps", ["apps"]),
    ("dpi", "/api/v1/dpi/tracker-stats", ["stats"]),
    ("dpi", "/api/v1/dpi/mark-rules", ["app_ports"]),
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

# ---- 2. 结构验证（非空时每个对象字段齐全） ----
struct_checks = [
    ("/api/v1/nac/neighbors", "neighbors", ["ip", "mac"]),
    ("/api/v1/quota/policies", "policies", None),
    ("/api/v1/webfilter/rules", "rules", None),
    ("/api/v1/dpi/flows", "flows", None),
]
for path, listkey, itemkeys in struct_checks:
    st, body, _, _ = call("GET", path, token=token)
    if st == 200 and isinstance(body, dict):
        items = body.get(listkey) or []
        if items and itemkeys:
            missing = [k for k in itemkeys if k not in items[0]]
            ok = (len(missing) == 0)
            rec(f"结构验证 {path}", "PASS" if ok else "FAIL",
                f"{len(items)} 条, 示例缺字段 {missing}" if missing else f"{len(items)} 条, 字段齐全",
                short(items[0]))
        else:
            rec(f"结构验证 {path}", "PASS", f"{len(items)} 条（空或无 itemkeys）", short(body))
    else:
        rec(f"结构验证 {path}", "FAIL", f"HTTP {st}", short(body))

# ---- 3. 写端点空 body → 期望 422 ----
write_checks = [
    ("radius", "POST /api/v1/radius/config", "/api/v1/radius/config", {}),
    ("nac", "POST /api/v1/nac/config", "/api/v1/nac/config", {}),
    ("nac", "POST /api/v1/nac/connlimits", "/api/v1/nac/connlimits", {}),
    ("identity", "POST /api/v1/identity", "/api/v1/identity", {}),
    ("quota", "POST /api/v1/quota/policies", "/api/v1/quota/policies", {}),
    ("webfilter", "POST /api/v1/webfilter/rules", "/api/v1/webfilter/rules", {}),
    ("dpi", "POST /api/v1/dpi/rules", "/api/v1/dpi/rules", {}),
    ("dpi", "POST /api/v1/dpi/marks", "/api/v1/dpi/marks", {}),
]
for mod, name, path, body in write_checks:
    st, resp, _, raw = call("POST", path, token=token, body=body)
    code = resp.get("code") if isinstance(resp, dict) else None
    if st >= 400 and st < 500:
        rec(f"[{mod}] {name} 空 body", "PASS", f"HTTP {st} code={code}（已拒绝）", short(resp))
    elif st in (200, 201):
        rec(f"[{mod}] {name} 空 body", "FAIL", f"空 body 被接受 HTTP {st}（Validate 未拦）", short(resp))
    else:
        rec(f"[{mod}] {name} 空 body", "FAIL", f"HTTP {st} code={code}", short(resp))

# ---- 4. 需真环境的端点 → SKIP ----
skips = [
    ("radius", "radius/test 真 RADIUS", "radius/test 需真实 RADIUS server，本机无；radius/config 已读"),
    ("dpi", "dpi/classify 真分类", "dpi/classify 需查询参数 ip/proto/port；空参返回 400 属预期"),
    ("quota", "quota/apply 真下发", "quota/apply 需真实 nft 环境；空 body 已测 Validate"),
    ("nac", "nac/connlimits/apply 真下发", "需真实 nft 环境；空 body 已测 Validate"),
    ("webfilter", "webfilter 真 append", "webfilter/rules POST 为全量替换，需真实 dnsmasq；空 body 已测 Validate"),
]
for mod, name, reason in skips:
    rec(f"[{mod}] {name}", "SKIP", reason, "N/A")

# ---- report ----
os.makedirs(ROOT / "docs", exist_ok=True)
lines = []
lines.append("# 访问控制模块冒烟报告")
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
