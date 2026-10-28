#!/usr/bin/env python3
"""p89-uncovered-batch1: uncovered endpoint tests (accounts/import/bind-mac/reset-usage, users/password, alerts/test, logs/sources, logs/syslog+syslog/test).

Real HTTP against mgmtd @ 127.0.0.1:9090 (admin login for Bearer token).
Writes docs/uncovered-batch1.md. No hand-edited doc — this script generates it.

Endpoints under test (verified in mgmtd/internal/server/server.go):
  POST /api/v1/accounts/import        body {"lines":[...]}                  -> {created,errors,total}
  POST /api/v1/accounts/{u}/bind-mac  body {"mac":...}                      -> {bound,username,account}
  POST /api/v1/accounts/{u}/reset-usage body {}                             -> {reset,username,account}
  POST /api/v1/users/{u}/password     body {"password":...}                 -> {changed}  (admin only)
  POST /api/v1/alerts/test            body {"unit","message"}               -> {matched,count}  (dry-run only)
  GET  /api/v1/logs/sources                                    -> {sources:[...],count}
  GET  /api/v1/logs/syslog                                     -> {target,rsyslog}
  POST /api/v1/logs/syslog/test   body {SyslogTarget}                 -> {ok,detail}  (real syslog send)

Uses marker value "p89test" for all test-created accounts / users.
"""
import json
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

BASE = "http://127.0.0.1:9090"
ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "uncovered-batch1.md"
MARK = "p89test"  # marker for all test-created accounts / users

results = []  # (name, status, detail, frag)

TOKEN = ""
frag = []  # per-step evidence fragments for the doc


def step(name, ok, detail, frag_text=""):
    status = "PASS" if ok else "FAIL"
    results.append((name, status, detail, frag_text))
    print(f"{status} {name}: {detail}")
    if frag_text:
        print(f"      frag: {frag_text[:200]}")


def frag_add(label, text):
    frag.append(f"- **{label}**\n  \n  ```\n  {text}\n  ```")


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


# ---------------------------------------------------------------- 1. accounts/import
def t1_accounts_import():
    lines = [
        f"{MARK}-acct1,Passw0rd!,pppoe,basic,100,30",
        f"{MARK}-acct2,PortalPw9,portal,premium,200,60",
        "# comment line is skipped",
        "",  # blank line skipped
        f"{MARK}-acct3,bad",  # too few fields -> error line, not abort
    ]
    status, body, raw = req("POST", "/api/v1/accounts/import", {"lines": lines})
    frag_add("accounts/import 响应", raw[:800])
    if status != 200:
        step("accounts/import", False, f"HTTP {status}: {raw[:200]}")
        return
    created = (body or {}).get("created")
    errors = (body or {}).get("errors")
    total = (body or {}).get("total")
    # 2 valid lines should be created (comment+blank skipped), 1 bad line -> error, total=5 input lines
    ok = created == 2 and total == 5 and isinstance(errors, list) and len(errors) == 1
    step("accounts/import", ok,
         f"created={created}, total={total}, errors={errors} (期望 created=2, total=5, 1条错误行)")
    # cleanup note: no delete account endpoint; accounts remain with p89test marker


# ---------------------------------------------------------------- 2. accounts/bind-mac
def t2_bind_mac():
    username = f"{MARK}-acct1"
    status, body, raw = req("POST", f"/api/v1/accounts/{username}/bind-mac",
                            {"mac": "AA:BB:CC:DD:EE:01"})
    frag_add("accounts/bind-mac 响应", raw[:800])
    if status != 200:
        step("accounts/bind-mac", False, f"HTTP {status}: {raw[:200]}")
        return
    bound = (body or {}).get("bound")
    acct = (body or {}).get("account", {})
    # server intentionally normalizes MAC to lowercase (manager.go BindMAC: strings.ToLower)
    ok = bound is True and acct.get("bound_mac") == "aa:bb:cc:dd:ee:01"
    step("accounts/bind-mac", ok,
         f"bound={bound}, account.bound_mac={acct.get('bound_mac')!r} (期望 bound=true, mac已写入并小写化)")


# ---------------------------------------------------------------- 3. accounts/reset-usage
def t3_reset_usage():
    username = f"{MARK}-acct1"
    # bind-mac already called; used_mb defaults to 0 on import. Reset is idempotent on zero.
    status, body, raw = req("POST", f"/api/v1/accounts/{username}/reset-usage", {})
    frag_add("accounts/reset-usage 响应", raw[:800])
    if status != 200:
        step("accounts/reset-usage", False, f"HTTP {status}: {raw[:200]}")
        return
    reset = (body or {}).get("reset")
    acct = (body or {}).get("account", {})
    ok = reset is True and acct.get("username") == username
    step("accounts/reset-usage", ok,
         f"reset={reset}, username={acct.get('username')!r}, used_mb={acct.get('used_mb')!r} (期望 reset=true)")


# ---------------------------------------------------------------- 4. users/password
def t4_users_password():
    # Try changing a non-existent user -> expect 409 (user not found). Real HTTP, no side effects.
    status, body, raw = req("POST", f"/api/v1/users/{MARK}-nonexistent/password",
                            {"password": "NewPassw0rd!"})
    frag_add("users/password 409 用户不存在", raw[:800])
    if status != 409:
        step("users/password(不存在用户)", False, f"期望 409, 实际 HTTP {status}: {raw[:200]}")
        return
    step("users/password(不存在用户->409)", True,
         f"HTTP {status}, code={(body or {}).get('code')}, detail={(body or {}).get('detail')!r} (期望 409 user not found)")

    # Weak password (no digit) -> expect 422
    status2, body2, raw2 = req("POST", f"/api/v1/users/admin/password",
                               {"password": "nodigits"})
    frag_add("users/password 弱密码 422", raw2[:800])
    ok2 = status2 == 422
    step("users/password(弱密码->422)", ok2,
         f"HTTP {status2}: {raw2[:200]} (期望 422 weak password)")


# ---------------------------------------------------------------- 5. alerts/test (dry-run only)
def t5_alerts_test():
    # alerting.Match only compares against stored rules; no external send. Safe dry-run.
    status, body, raw = req("POST", "/api/v1/alerts/test",
                            {"unit": "nginx.service", "message": f"{MARK} test alert probe"})
    frag_add("alerts/test 响应", raw[:800])
    if status != 200:
        step("alerts/test", False, f"HTTP {status}: {raw[:200]}")
        return
    matched = (body or {}).get("matched")
    count = (body or {}).get("count")
    ok = status == 200 and isinstance(matched, list) and isinstance(count, int)
    step("alerts/test", ok,
         f"matched={matched!r}, count={count} (纯 dry-run，无外发告警，HTTP 200)")


# ---------------------------------------------------------------- 6. logs/sources
def t6_logs_sources():
    status, body, raw = req("GET", "/api/v1/logs/sources")
    frag_add("logs/sources 响应", raw[:800])
    if status != 200:
        step("logs/sources", False, f"HTTP {status}: {raw[:200]}")
        return
    sources = (body or {}).get("sources")
    count = (body or {}).get("count")
    ok = status == 200 and isinstance(sources, list) and count == len(sources)
    step("logs/sources", ok,
         f"sources 长度={len(sources) if isinstance(sources, list) else 'n/a'}, count={count} (HTTP 200)")


# ---------------------------------------------------------------- 7. logs/syslog GET + /test
def t7_logs_syslog():
    # 7a GET /logs/syslog -> {target, rsyslog}
    status, body, raw = req("GET", "/api/v1/logs/syslog")
    frag_add("logs/syslog GET 响应", raw[:800])
    if status != 200:
        step("logs/syslog GET", False, f"HTTP {status}: {raw[:200]}")
        return
    target = (body or {}).get("target")
    rsyslog = (body or {}).get("rsyslog")
    step("logs/syslog GET", status == 200,
         f"target={target!r}, rsyslog={rsyslog!r} (HTTP 200，单条目标，无分页参数)")

    # 7b POST /logs/syslog/test -> real syslog send to localhost:514 UDP (likely no listener)
    tgt = {
        "name": f"{MARK}-syslog",
        "address": "127.0.0.1:514",
        "protocol": "udp",
        "format": "rfc5424",
        "facility": "local0",
        "enabled": True,
    }
    status2, body2, raw2 = req("POST", "/api/v1/logs/syslog/test", tgt, timeout=30)
    frag_add("logs/syslog/test 响应", raw2[:800])
    ok2 = status2 == 200 and "ok" in (body2 or {})
    detail2 = f"ok={(body2 or {}).get('ok')}, detail={(body2 or {}).get('detail')!r}"
    step("logs/syslog/test", ok2, f"HTTP {status2}, {detail2} (真实 syslog 发送尝试)")


# ---------------------------------------------------------------- doc
def write_doc():
    lines_out = []
    for name, status, detail, f in results:
        lines_out.append(f"### {name}\n\n- **状态**: {status}\n- **详情**: {detail}\n")
        if f:
            lines_out.append(f + "\n")
    passed = sum(1 for r in results if r[1] == "PASS")
    failed = sum(1 for r in results if r[1] == "FAIL")
    skipped = sum(1 for r in results if r[1] == "SKIP")
    out = [
        "# p89-uncovered-batch1: 未覆盖端点测试（批1）",
        "",
        f"生成时间: {time.strftime('%Y-%m-%d %H:%M:%S')}",
        "",
        f"目标: mgmtd @ {BASE}（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。",
        "",
        f"## 汇总: {passed} PASS / {failed} FAIL / {skipped} SKIP（共 {len(results)} 步）",
        "",
        "覆盖端点: accounts/import、accounts/{u}/bind-mac、accounts/{u}/reset-usage、",
        "users/{u}/password、alerts/test、logs/sources、logs/syslog(+syslog/test)。",
        "",
        "## 分步结果",
        "",
    ]
    out += lines_out
    DOC.write_text("\n".join(out), encoding="utf-8")
    print(f"\n报告: {DOC}")
    print(f"{passed} PASS / {failed} FAIL / {skipped} SKIP")


if __name__ == "__main__":
    login()
    t1_accounts_import()
    t2_bind_mac()
    t3_reset_usage()
    t4_users_password()
    t5_alerts_test()
    t6_logs_sources()
    t7_logs_syslog()
    write_doc()
