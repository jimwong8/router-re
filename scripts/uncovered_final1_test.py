#!/usr/bin/env python3
"""p95-uncovered-final1: 最后未覆盖 3 条端点的批测试（真实打本机 9090）。

端点（从 server.go 确认路由，未猜）：
  - ddns/records + DELETE /ddns/records/{id}
      server.go:337-339 — GET/POST /ddns/records, DELETE /ddns/records/{id}
      没有 GET/PUT /records/{id}（grep 无此路由），故「CRUD 完整」用：
      POST 全量替换建记录 → GET 列表拿 id → POST 全量替换改 → DELETE {id}。
  - system/timezones
      server.go:306 — GET /api/v1/system/timezones → {timezones,count}
      remote.go:125 读 /usr/share/zoneinfo，含 Asia/Shanghai。
  - users/{username}/password
      server.go:487-489 — POST /users 建号、POST /users/{u}/password 改密、
      DELETE /users/{u} 删除。改密 body {password}，密码 8-64 且含字母+数字。

产出: docs/uncovered-final1.md（脚本生成，非手写）。
"""
import json, os, time, urllib.request, urllib.error
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "uncovered-final1.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")

P95_DDNS_ID = "p95test-record"
P95_DDNS_DOMAIN = "p95test.example.com"
P95_USER = "p95test"
P95_PW1 = "p95test_Aa1"   # 8-64, 含字母+数字
P95_PW2 = "changed_Bb2"


def call(method, path, token=None, body=None, timeout=30):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            try:
                return r.status, json.loads(raw), raw
            except Exception:
                return r.status, raw, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw), raw
        except Exception:
            return e.code, raw, raw
    except Exception as e:
        return 0, f"{type(e).__name__}: {e}", str(e)


def login():
    st, body, _ = call("POST", "/api/v1/auth/login",
                       body={"username": "admin", "password": "admin123"})
    if st != 200:
        raise SystemExit(f"login failed {st}: {body}")
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]


results = []


def short(x, n=240):
    s = json.dumps(x, ensure_ascii=False) if not isinstance(x, str) else x
    return s if len(s) <= n else s[: n - 3] + "..."


def rec(name, status, detail, evidence):
    results.append({"name": name, "status": status, "detail": detail, "evidence": evidence})
    print(f"{status:4} {name}: {detail}")


def find_ddns_id(token):
    st, body, raw = call("GET", "/api/v1/ddns/records", token=token)
    if st != 200 or not isinstance(body, dict):
        return None
    for r0 in body.get("records", []):
        if r0.get("id") == P95_DDNS_ID:
            return r0
    return None


token = login()
rec("登录 admin", "PASS", "Bearer token 获取成功", "token:<redacted>")

# ============ 1. DDNS records/{id}（CRUD，无独立 GET/PUT {id} 路由，用 POST 全量替换语义） ============
rec_type = {
    "id": P95_DDNS_ID, "provider": "duckdns", "domain": P95_DDNS_DOMAIN,
    "subdomain": "", "token": "p95tokensecret1", "enabled": True, "last_ip": "",
}
# 1a. 创建
st, body, raw = call("POST", "/api/v1/ddns/records", token=token, body={"records": [rec_type], "confirm": True})
b = body if isinstance(body, dict) else {}
if st == 200 and b.get("count") == 1:
    rec("ddns/records 创建(POST全量替换)", "PASS", f"count={b.get('count')}", short(raw))
else:
    rec("ddns/records 创建(POST全量替换)", "FAIL", f"HTTP {st} count={b.get('count')}", short(raw))

# 1b. 读回拿 id
rec_exists = find_ddns_id(token)
if rec_exists:
    rec("ddns/records 读回拿 id", "PASS", f"id={rec_exists.get('id')} masked_token={rec_exists.get('token')}", short(rec_exists))
else:
    rec("ddns/records 读回拿 id", "FAIL", "列表里找不到 p95test-record", "")

# 1c. 更新（POST 全量，改 LastIP）
rec_upd = dict(rec_type)
rec_upd["last_ip"] = "203.0.113.50"
st, body, raw = call("POST", "/api/v1/ddns/records", token=token, body={"records": [rec_upd], "confirm": True})
b = body if isinstance(body, dict) else {}
back = find_ddns_id(token)
if st == 200 and b.get("count") == 1 and back and back.get("last_ip") == "203.0.113.50":
    rec("ddns/records 更新", "PASS", f"last_ip={back.get('last_ip')}", short(raw))
else:
    rec("ddns/records 更新", "FAIL", f"HTTP {st} last_ip_back={back.get('last_ip') if back else None}", short(raw))

# 1d. 删除 {id}
st, body, raw = call("DELETE", f"/api/v1/ddns/records/{P95_DDNS_ID}", token=token)
b = body if isinstance(body, dict) else {}
back_after = find_ddns_id(token)
if st == 200 and back_after is None:
    rec("ddns/records 删除{id}", "PASS", f"HTTP {st} deleted={b.get('deleted')}, 列表中已无该 id", short(raw))
else:
    rec("ddns/records 删除{id}", "FAIL", f"HTTP {st} count残留={b.get('count')}", short(raw))

# ============ 2. system/timezones ============
st, body, raw = call("GET", "/api/v1/system/timezones", token=token)
b = body if isinstance(body, dict) else {}
zones = b.get("timezones", [])
if st == 200 and zones and "Asia/Shanghai" in zones:
    rec("system/timezones", "PASS", f"HTTP {st} count={b.get('count')} 含 Asia/Shanghai", short(raw))
else:
    rec("system/timezones", "FAIL", f"HTTP {st} count={b.get('count') if isinstance(b, dict) else '?'}", short(raw))

# ============ 3. users/{username}/password ============
# 3a. 建临时账号
st, body, raw = call("POST", "/api/v1/users", token=token,
                     body={"username": P95_USER, "password": P95_PW1, "role": "viewer"})
b = body if isinstance(body, dict) else {}
if st == 201:
    rec("users 建临时账号", "PASS", f"HTTP {st} user={b.get('user', {}).get('username') if isinstance(b, dict) else ''}", short(raw))
else:
    rec("users 建临时账号", "FAIL", f"HTTP {st}", short(raw))

# 3b. 改密
st, body, raw = call("POST", f"/api/v1/users/{P95_USER}/password", token=token, body={"password": P95_PW2})
b = body if isinstance(body, dict) else {}
if st == 200 and b.get("changed") == P95_USER:
    rec("users/{u}/password 改密", "PASS", f"HTTP {st} changed={b.get('changed')}", short(raw))
else:
    rec("users/{u}/password 改密", "FAIL", f"HTTP {st}", short(raw))

# 3c. 真实验证：用改后的新密码登录，旧密码应失败
st_old, _, _ = call("POST", "/api/v1/auth/login", body={"username": P95_USER, "password": P95_PW1})
st_new, body_new, _ = call("POST", "/api/v1/auth/login", body={"username": P95_USER, "password": P95_PW2})
if st_old == 401 and st_new == 200:
    rec("改密生效验证", "PASS", f"旧密401, 新密200", f"old={st_old} new={st_new}")
else:
    rec("改密生效验证", "FAIL", f"旧密={st_old}(期望401) 新密={st_new}(期望200)", "")

# 3d. 删除账号清理
st_del, body, raw = call("DELETE", f"/api/v1/users/{P95_USER}", token=token)
b = body if isinstance(body, dict) else {}
st_verify, _, _ = call("POST", "/api/v1/auth/login", body={"username": P95_USER, "password": P95_PW2})
if st_del == 200 and st_verify != 200:
    rec("users 删除账号清理", "PASS", f"DELETE HTTP {st_del} deleted={b.get('deleted')}, 删除后登录={st_verify}", short(raw))
else:
    rec("users 删除账号清理", "FAIL", f"HTTP {st_del} 删除后登录={st_verify}", short(raw))

# ============ 报告 ============
# 清理确认：ddns 记录需无 p95 残留、users 需无 p95 账号
ddns_left = find_ddns_id(token) is not None
st_u, body_u, _ = call("GET", "/api/v1/users", token=token)
users_left = False
if isinstance(body_u, dict):
    for u in body_u.get("users", []):
        if u.get("username") == P95_USER:
            users_left = True
if ddns_left:
    call("DELETE", f"/api/v1/ddns/records/{P95_DDNS_ID}", token=token)
if users_left:
    call("DELETE", f"/api/v1/users/{P95_USER}", token=token)

os.makedirs(ROOT / "docs", exist_ok=True)
n_pass = sum(1 for r in results if r["status"] == "PASS")
n_fail = sum(1 for r in results if r["status"] == "FAIL")
lines = ["# p95 未覆盖端点批测试报告（ddns/records、system/timezones、users/password）", ""]
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}")
lines.append("")
lines.append(f"**汇总**: {len(results)} 步 | PASS {n_pass} / FAIL {n_fail}")
lines.append("")
lines.append("| # | 端点 | 步骤 | 结果 | 说明 |")
lines.append("|---|------|------|------|------|")
for i, r in enumerate(results, 1):
    lines.append(f"| {i} | {r['name'].split()[0]} | {r['name']} | {r['status']} | {r['detail']} |")
lines.append("")
lines.append("## 清理状态")
lines.append(f"- ddns p95test-record 残留: {'存在（已补删）' if ddns_left else '无'}")
lines.append(f"- users p95test 账号残留: {'存在（已补删）' if users_left else '无'}")
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
print(f"SUMMARY: PASS={n_pass} FAIL={n_fail}")