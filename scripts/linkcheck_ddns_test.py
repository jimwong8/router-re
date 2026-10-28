#!/usr/bin/env python3
"""linkcheck + DDNS 状态流转闭环验证。

真实打本机 9090：建 target → 探测 → 看 states；建 ddns record → check/update。
报告: docs/linkcheck-ddns-test.md (脚本生成)。
"""
import json
import os
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "linkcheck-ddns-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")

def call(method, path, token=None, body=None, timeout=30):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw), time.time() - t0, raw
            except json.JSONDecodeError:
                return resp.status, raw, time.time() - t0, raw
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), time.time() - t0, raw
        except json.JSONDecodeError:
            return exc.code, raw, time.time() - t0, raw
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}", time.time() - t0, str(exc)

def login():
    user = os.environ.get("MGM_ADMIN_USER", "admin")
    pwd = os.environ.get("MGM_ADMIN_PASSWORD", "admin123")
    st, body, _, _ = call("POST", "/api/v1/auth/login", body={"username": user, "password": pwd})
    if st != 200:
        print(f"登录失败 HTTP {st}: {body}")
        raise SystemExit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]

def short(x, n=400):
    s = json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n-3] + "..."

token = login()
results = []

def record(name, status, detail, evidence):
    results.append({"name": name, "status": status, "detail": detail, "evidence": evidence})
    print(f"{status:4} {name}: {detail}")

# ---------------------------------------------------------------- 1. linkcheck target
tgt_id = "p48-icmp-1"
target = {
    "id": tgt_id, "name": "p48 roundtrip", "iface": "lo", "kind": "icmp",
    "host": "127.0.0.1", "interval": 5, "timeout": 3,
    "fail_threshold": 2, "recover_threshold": 1, "enabled": True,
}
st, resp, _, raw = call("POST", "/api/v1/linkcheck/targets", token=token, body={"targets": [target]})
write_ok = (st == 200)
# GET to confirm exists
st2, rd, _, _ = call("GET", "/api/v1/linkcheck/targets", token=token)
found = False
if isinstance(rd, dict):
    found = any(t.get("id") == tgt_id for t in (rd.get("targets") or []))
step1_ok = write_ok and (st2 == 200) and found
record("1. linkcheck 建 target (POST targets)", "PASS" if step1_ok else "FAIL",
       f"write={st} read={st2} found={found}",
       f"write_resp={short(resp)}\nread_targets={short(rd)}")

# ---------------------------------------------------------------- 2. probe + status
# probe
st_p, resp_p, _, _ = call("POST", "/api/v1/linkcheck/probe", token=token, body={"id": tgt_id}, timeout=20)
probe_ok = (st_p == 200)
# wait a bit for state to settle (probe is synchronous, but give a moment)
time.sleep(2)
st_s, rd_s, _, _ = call("GET", "/api/v1/linkcheck/status", token=token)
state_entry = None
if isinstance(rd_s, dict):
    states = rd_s.get("states") or {}
    if isinstance(states, dict):
        state_entry = states.get(tgt_id)
step2_ok = probe_ok and (st_s == 200) and (state_entry is not None)
record("2. linkcheck probe + status (states 含该 target)",
       "PASS" if step2_ok else "FAIL",
       f"probe={st_p} status={st_s} state_present={state_entry is not None}",
       f"probe_resp={short(resp_p)}\nstatus={short(rd_s)}")
probe_evidence = (st_p, resp_p, state_entry)

# ---------------------------------------------------------------- 3. ddns record
rec_id = "p48-ddns-1"
ddns_rec = {
    "id": rec_id, "provider": "cloudflare", "domain": "example.com",
    "subdomain": "p48", "token": "p48roundtriptoken1234", "enabled": True,
}
st3, resp3, _, _ = call("POST", "/api/v1/ddns/records", token=token, body={"records": [ddns_rec]})
write_ddns_ok = (st3 == 200)
st3r, rd3, _, _ = call("GET", "/api/v1/ddns/records", token=token)
found_ddns = False
if isinstance(rd3, dict):
    found_ddns = any(r.get("id") == rec_id for r in (rd3.get("records") or []))
step3_ok = write_ddns_ok and (st3r == 200) and found_ddns
record("3. ddns 建 record (POST records)", "PASS" if step3_ok else "FAIL",
       f"write={st3} read={st3r} found={found_ddns}",
       f"write_resp={short(resp3)}\nread_records={short(rd3)}")

# ---------------------------------------------------------------- 4. ddns check + update (structure, not 500)
# check
st4c, rd4c, _, _ = call("GET", "/api/v1/ddns/check", token=token)
check_ok = (st4c in (200,))  # structured response
# update (needs config; expect structured 400 "ddns not configured" or 200)
st4u, rd4u, _, _ = call("POST", "/api/v1/ddns/update", token=token, body={})
update_structured = (st4u in (200, 400, 422, 503))  # not 500 crash
step4_ok = check_ok and update_structured
record("4. ddns check + update 结构 (非 500 崩)", "PASS" if step4_ok else "FAIL",
       f"check={st4c} update={st4u}",
       f"check_resp={short(rd4c)}\nupdate_resp={short(rd4u)}")

# ---------------------------------------------------------------- 5. cleanup
# No DELETE endpoints exist -> leave data, report.
record("5. 清理 (DELETE 端点)", "SKIP",
       "linkcheck/ddns 均无 DELETE 端点，新建的 target/record 留存",
       "无 DELETE /api/v1/linkcheck/targets 或 /api/v1/ddns/records 端点")

# ---------------------------------------------------------------- report
os.makedirs(ROOT / "docs", exist_ok=True)
lines = []
lines.append("# linkcheck + DDNS 状态流转闭环验证报告")
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

# extra: status states raw
lines.append("## 附：linkcheck status 原始 states")
lines.append("")
lines.append("```json")
lines.append(json.dumps(rd_s if isinstance(rd_s, dict) else rd_s, ensure_ascii=False, indent=2))
lines.append("```")
lines.append("")

with open(DOC, "w") as f:
    f.write("\n".join(lines))
print(f"报告已写 {DOC}")
print(f"SUMMARY: PASS={n_pass} FAIL={n_fail} SKIP={n_skip}")
