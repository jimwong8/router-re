#!/usr/bin/env python3
"""p93-qos-batch: 6 限速/负载端点批测试（htb/lb/l7），真实打本机 9090。

端点来源：grep server.go 确认路由表，不猜：
  POST /api/v1/htb/apply     body {confirm, config:{iface,root_rate_mbit,default_class,enabled,classes[]}}
  POST /api/v1/htb/cleanup   body {confirm, iface}     (ValidateIface 拒绝 lo/空/空格)
  POST /api/v1/lb/apply      no body (读 saved config)  → resp {applied, marks, iprules, rules, detail}
  POST /api/v1/lb/check      no body                    → resp {checked_at, status[]}
  POST /api/v1/l7/apply      no body (读 saved rules)   → resp {applied, detail, dnsmasq}
  POST /api/v1/l7/test       body {sni,http_host,user_agent,dns_name} → resp {hit, blocked, by}

前置（setup，非 6 个测试端点）：
  POST /api/v1/auth/login    body {username, password}  → resp {token}
  POST /api/v1/lb/config     body Config{Mode,Members[],Health}
  POST /api/v1/l7/rules      body {rules:[...], confirm:bool}
"""
import json, os, re, subprocess, sys, time, urllib.request, urllib.error
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "qos-batch.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
QOS_IFACE = os.environ.get("QOS_IFACE", "qosdummy")

# iface 白名单校验 — 杜绝 shell 拼接注入
if not re.fullmatch(r"[A-Za-z0-9_.-]{1,32}", QOS_IFACE):
    raise SystemExit(f"refusing unsafe QOS_IFACE={QOS_IFACE!r}")

results = []


# ── HTTP 客户端 ──────────────────────────────────────────────
def call(method, path, token=None, body=None, timeout=30):
    url = BASE + path
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            try:
                return r.status, json.loads(raw), time.time() - t0, raw
            except Exception:
                return r.status, raw, time.time() - t0, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw), time.time() - t0, raw
        except Exception:
            return e.code, raw, time.time() - t0, raw
    except Exception as e:
        return 0, f"{type(e).__name__}: {e}", time.time() - t0, str(e)


def login():
    st, body, _, _ = call("POST", "/api/v1/auth/login",
                          body={"username": "admin", "password": "admin123"})
    if st != 200:
        raise SystemExit(f"login failed {st}: {body}")
    tok = body["token"] if isinstance(body, dict) else json.loads(body)["token"]
    return tok


# ── 子命令执行（argv 列表，禁止 shell 拼接） ─────────────────
def sh(cmd):
    return subprocess.run(cmd, capture_output=True, text=True)


def shroot(cmd):
    """以 sudo -n 前缀执行 argv 列表（root 特权）。"""
    return subprocess.run(["sudo", "-n"] + cmd, capture_output=True, text=True)


def short(x, n=300):
    s = json.dumps(x, ensure_ascii=False) if not isinstance(x, str) else x
    return s if len(s) <= n else s[:n - 3] + "..."


results = []


def rec(name, status, detail, evidence):
    results.append({"name": name, "status": status, "detail": detail, "evidence": evidence})
    print(f"{status:4} {name}: {detail}")


# ── 预清理：删除上次轮次残留 ───────────────────────────────
def preclean():
    """清理历史残留：LB fwmark 规则（循环删全）、nft 表、htb qdisc、l7 conf。"""
    # 删除所有 fwmark 相关 ip rule（可能有多条同 mark 的残留）
    out = sh(["ip", "rule", "show"]).stdout
    seen = set()
    for line in out.splitlines():
        m = re.search(r'fwmark\s+(0x[0-9a-fA-F]+|\d+)\s+lookup\s+(\d+)', line)
        if m:
            key = (m.group(1), m.group(2))
            if key in seen:
                continue
            seen.add(key)
            mark_raw = m.group(1)
            lookup = m.group(2)
            try:
                mark_dec = int(mark_raw, 0)  # 自动识别 0x96 或 150
            except ValueError:
                continue
            # 循环删除该 mark 的所有规则，直到 ip rule del 失败（说明已无匹配）
            for _ in range(50):  # 安全上限
                r = shroot(["ip", "rule", "del", "fwmark", str(mark_dec), "lookup", lookup])
                if r.returncode != 0:
                    break
    # 删除残留 nft 表
    nft_out = shroot(["nft", "list", "tables"]).stdout
    for tbl in ["router_lb_marks", "router_l7"]:
        if f" {tbl}" in nft_out or f"\t{tbl}" in nft_out or f"inet {tbl}" in nft_out:
            shroot(["nft", "delete", "table", "inet", tbl])
    # 清理 htb qdisc
    shroot(["tc", "qdisc", "del", "dev", QOS_IFACE, "root"])
    # 清理 htb qdisc on lo（防御性，确保 lo 永远无 htb）
    shroot(["tc", "qdisc", "del", "dev", "lo", "root"])  # 防御性，lo 永远无 htb
    # 清理 l7 dnsmasq conf
    shroot(["rm", "-f", "/etc/dnsmasq.d/router-re-l7.conf"])


preclean()
token = login()
rec("登录 admin", "PASS", "Bearer token 获取成功", "token:<redacted>")

# ── 1. HTB ─────────────────────────────────────────────────
htb_cfg = {
    "iface": QOS_IFACE,
    "root_rate_mbit": 100.0,
    "default_class": "p93test",
    "enabled": True,
    "classes": [
        {"name": "p93test", "rate_mbit": 20.0, "ceil_mbit": 80.0,
         "priority": 2, "match_ports": [8080], "match_dst_ips": ["127.0.0.1"]}
    ],
}

# dry-run 确认协议
st, body, _, raw = call("POST", "/api/v1/htb/apply", token=token,
                        body={"confirm": False, "config": htb_cfg})
applied = body.get("applied") if isinstance(body, dict) else None
dry = body.get("dry_run") if isinstance(body, dict) else None
if st == 200 and applied is False and dry is True:
    rec("htb/apply dry-run", "PASS", "confirm=false 仅生成脚本不真下发", short(body))
else:
    rec("htb/apply dry-run", "FAIL", f"HTTP {st} applied={applied} dry_run={dry}", short(raw))

# 真下发（p93test 标记）
st, body, _, raw = call("POST", "/api/v1/htb/apply", token=token,
                        body={"confirm": True, "config": htb_cfg})
applied = body.get("applied") if isinstance(body, dict) else None
detail = body.get("detail", "") if isinstance(body, dict) else ""
if st == 200 and applied is True:
    rec("htb/apply 真下发(p93test标记)", "PASS", f"applied=true {detail}", short(body))
else:
    rec("htb/apply 真下发(p93test标记)", "FAIL", f"HTTP {st} applied={applied}", short(raw))

# 读回确认
st, body, _, raw = call("GET", "/api/v1/htb/config", token=token)
cfg_back = body.get("config", {}) if isinstance(body, dict) else {}
if st == 200 and cfg_back.get("enabled") and cfg_back.get("iface") == QOS_IFACE:
    rec("htb/config 读回确认", "PASS",
        f"enabled={cfg_back.get('enabled')} iface={cfg_back.get('iface')} classes={len(cfg_back.get('classes', []))}",
        short(body))
else:
    rec("htb/config 读回确认", "FAIL", f"HTTP {st}", short(raw))

# 真实 tc 状态确认（非 HTTP 伪造）
tcout = shroot(["tc", "qdisc", "show", "dev", QOS_IFACE])
htb_live = "htb" in tcout.stdout
rec("htb 真实 tc 状态", "PASS" if htb_live else "FAIL",
    f"tc qdisc show dev {QOS_IFACE} 含 htb" if htb_live else "tc 未见 htb",
    tcout.stdout.strip() or tcout.stderr.strip())

# cleanup
st, body, _, raw = call("POST", "/api/v1/htb/cleanup", token=token,
                        body={"confirm": True, "iface": QOS_IFACE})
applied = body.get("applied") if isinstance(body, dict) else None
if st == 200 and applied is True:
    rec("htb/cleanup", "PASS", f"applied=true iface={QOS_IFACE}", short(body))
else:
    rec("htb/cleanup", "FAIL", f"HTTP {st} applied={applied}", short(raw))

# cleanup 后确认干净
tcout2 = shroot(["tc", "qdisc", "show", "dev", QOS_IFACE])
htb_gone = "htb" not in tcout2.stdout
rec("htb 清理后确认空", "PASS" if htb_gone else "FAIL",
    "tc 无 htb qdisc" if htb_gone else "tc 仍见 htb",
    tcout2.stdout.strip() or tcout2.stderr.strip())

# ── 2. LB ──────────────────────────────────────────────────
lb_cfg = {
    "mode": "weighted",
    "members": [{"name": "wan1", "iface": "ens18", "gateway": "10.100.1.1",
                 "weight": 2, "enabled": True}],
    "health": {"target": "10.100.1.1", "interval_sec": 5, "timeout_sec": 1,
               "fail_threshold": 3, "rise_threshold": 2},
}

# setup: 保存 lb 配置
st, body, _, raw = call("POST", "/api/v1/lb/config", token=token, body=lb_cfg)
if st == 200 and isinstance(body, dict) and body.get("applied"):
    rec("lb/config 保存(setup)", "PASS", f"HTTP {st} marks={body.get('marks')}", short(body))
else:
    rec("lb/config 保存(setup)", "FAIL", f"HTTP {st}", short(raw))

# lb/apply — 无 body，读 saved config，真下发 nft mark 表 + ip rule
st, body, _, raw = call("POST", "/api/v1/lb/apply", token=token)
b = body if isinstance(body, dict) else {}
applied = b.get("applied")
marks = b.get("marks", {})
detail = b.get("detail", "")
rec("lb/apply", "PASS" if st == 200 and applied else "FAIL",
    f"HTTP {st} applied={applied} marks={marks} {detail}", short(body))

# lb/check — 确认健康检查真实执行（ping 10.100.1.1）
st, body, _, raw = call("POST", "/api/v1/lb/check", token=token)
b = body if isinstance(body, dict) else {}
status = b.get("status", [])
if st == 200 and isinstance(status, list):
    up = status[0].get("up") if status else "?"
    rec("lb/check 确认", "PASS",
        f"HTTP {st} status条数={len(status)} up={up}", short(status) if status else short(b))
else:
    rec("lb/check 确认", "FAIL", f"HTTP {st}", short(raw))

# LB 清理 — 删除 ALL 匹配的 ip rule（循环，可能有多条同 mark 残留）
clean_detail = []
clean_ok = True
if not isinstance(marks, dict) or not marks:
    # 回退：从 /api/v1/lb/config 读 marks
    st2, b2, _, _ = call("GET", "/api/v1/lb/config", token=token)
    if isinstance(b2, dict):
        marks = b2.get("marks", {})
if not marks:
    clean_ok = False
    clean_detail.append("未能取得 lb marks，ip rule fwmark 无法精确清理")
for name, m in (marks or {}).items():
    if not str(m).isdigit():
        clean_detail.append(f"{name}: 非法 mark {m!r}")
        clean_ok = False
        continue
    # 循环删除该 mark 的所有 ip rule（ip rule add 可能产生重复）
    for _ in range(50):
        r = shroot(["ip", "rule", "del", "fwmark", str(m), "lookup", str(m)])
        present = sh(["ip", "rule", "show"]).stdout
        # 检查残留：同时匹配十进制和十六进制形式
        hex_mark = f"0x{m:x}"
        if f"fwmark {m}" in present or f"fwmark {hex_mark}" in present:
            if r.returncode != 0:
                break  # del 失败但仍残留，停止循环
        else:
            break  # 已全部清除
    # 最终确认
    after = sh(["ip", "rule", "show"]).stdout
    hex_mark = f"0x{m:x}"
    if f"fwmark {m}" in after or f"fwmark {hex_mark}" in after:
        clean_ok = False
        clean_detail.append(f"{name}(fwmark {m}) 未清除")
    else:
        clean_detail.append(f"{name}(fwmark {m}) 已清除")
# 删除 nft mark 表
nft_out = shroot(["nft", "list", "tables"]).stdout
if "router_lb_marks" in nft_out:
    shroot(["nft", "delete", "table", "inet", "router_lb_marks"])
    clean_detail.append("nft router_lb_marks 已删")
else:
    clean_detail.append("nft router_lb_marks 无残留")
rec("lb 清理", "PASS" if clean_ok else "FAIL", "; ".join(clean_detail), f"marks={marks}")

# ── 3. L7 ──────────────────────────────────────────────────
l7_rules = [
    {"id": "p93test", "name": "p93test-block", "match_type": "dns",
     "pattern": "p93test.example.com", "action": "block", "enabled": True},
]

# setup: 保存 l7 规则
st, body, _, raw = call("POST", "/api/v1/l7/rules", token=token, body={"rules": l7_rules})
if st == 200 and isinstance(body, dict):
    rec("l7/rules 保存(setup)", "PASS", f"HTTP {st} rules={len(body.get('rules', []))}", short(body))
else:
    rec("l7/rules 保存(setup)", "FAIL", f"HTTP {st}", short(raw))

# l7/apply — 写 dnsmasq conf，重启 dnsmasq（本机 dnsmasq inactive，按契约判读）
st, body, _, raw = call("POST", "/api/v1/l7/apply", token=token)
b = body if isinstance(body, dict) else {}
applied = b.get("applied")
detail = b.get("detail", "")
l7conf = "/etc/dnsmasq.d/router-re-l7.conf"
conf_written = os.path.exists(l7conf)
rec("l7/apply", "PASS" if st == 200 and applied and conf_written else
    ("PARTIAL" if st == 200 and conf_written else "FAIL"),
    f"HTTP {st} applied={applied} conf写入={conf_written} {detail}", short(raw))

# l7/test — 打测试流量（本地评估请求上下文，dns_name=p93test.example.com 应命中阻断）
tst = {"dns_name": "p93test.example.com", "sni": "p93test.example.com"}
st, body, _, raw = call("POST", "/api/v1/l7/test", token=token, body=tst)
b = body if isinstance(body, dict) else {}
hit = b.get("hit")
blocked = b.get("blocked")
by = b.get("by")
if st == 200 and hit and blocked:
    rec("l7/test 命中阻断", "PASS",
        f"hit={bool(hit)} blocked={blocked} by={by}", short(body))
else:
    rec("l7/test 命中阻断", "FAIL", f"HTTP {st} hit={hit} blocked={blocked}", short(raw))

# l7/test 反例 — 无关域名不应命中
st2, body2, _, raw2 = call("POST", "/api/v1/l7/test", token=token,
                           body={"dns_name": "ok.example.org"})
hit2 = body2.get("hit") if isinstance(body2, dict) else None
if st2 == 200 and not hit2:
    rec("l7/test 不命中反例", "PASS", "无关域名未被命中", short(body2))
else:
    rec("l7/test 不命中反例", "FAIL", f"HTTP {st2} hit={hit2}", short(raw2))

# L7 清理 — 清空规则 → re-apply 复写空 conf
st, body, _, raw = call("POST", "/api/v1/l7/rules", token=token,
                        body={"rules": [], "confirm": True})
if st == 200:
    rec("l7/rules 清理", "PASS", "rules 已清空", short(body))
else:
    rec("l7/rules 清理", "FAIL", f"HTTP {st}", short(raw))

st, body, _, raw = call("POST", "/api/v1/l7/apply", token=token)
b = body if isinstance(body, dict) else {}
conf_path = Path("/etc/dnsmasq.d/router-re-l7.conf")
conf_empty = conf_path.exists() and conf_path.read_text() == ""
rec("l7/apply 清理复写", "PASS" if st == 200 and conf_empty else "FAIL",
    f"HTTP {st} applied={b.get('applied')} conf为空={conf_empty}", short(raw))

# ── 报告生成 ───────────────────────────────────────────────
os.makedirs(ROOT / "docs", exist_ok=True)
n_pass = sum(1 for r in results if r["status"] == "PASS")
n_fail = sum(1 for r in results if r["status"] == "FAIL")
n_skip = sum(1 for r in results if r["status"] == "SKIP")
n_partial = sum(1 for r in results if r["status"] == "PARTIAL")

# 终态清理验证
tc_clean = shroot(["tc", "qdisc", "show", "dev", QOS_IFACE])
iprule = sh(["ip", "rule", "show"])
nft_now = shroot(["nft", "list", "tables"]).stdout
l7conf_exists = os.path.exists("/etc/dnsmasq.d/router-re-l7.conf")
lo_clean = "htb" not in shroot(["tc", "qdisc", "show", "dev", "lo"]).stdout

lines = []
lines.append("# p93-QoS 批测试报告（htb/lb/l7）")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}  BASE={BASE}  iface={QOS_IFACE}（dummy，安全）")
lines.append("")
lines.append(f"**汇总**: {len(results)} 步 | PASS {n_pass} / FAIL {n_fail} / PARTIAL {n_partial} / SKIP {n_skip}")
lines.append("")
lines.append("| # | 步骤 | 结果 | 说明 |")
lines.append("|---|------|------|------|")
for i, r in enumerate(results, 1):
    lines.append(f"| {i} | {r['name']} | {r['status']} | {r['detail']} |")
lines.append("")
lines.append("## 清理状态（终态）")
lines.append("")
lines.append("### HTB")
lines.append(f"- `{QOS_IFACE}` tc qdisc: `{tc_clean.stdout.strip() or '<none>'}` → {'干净' if 'htb' not in tc_clean.stdout else '⚠残留'}")
lines.append(f"- `lo` tc qdisc: `{shroot(['tc','qdisc','show','dev','lo']).stdout.strip() or '<none>'}` → {'干净' if lo_clean else '⚠残留'}")
lines.append("")
lines.append("### LB")
# ip rule 残留：检查是否存在 router_lb_marks 相关的 fwmark 规则
iprule_lines = [l for l in iprule.stdout.splitlines() if re.search(r'fwmark\s+(0x[0-9a-fA-F]+|\d+)\s+lookup\s+\d+', l)]
lines.append(f"- `ip rule` fwmark 残留: {'<none>' if not iprule_lines else '⚠' + '; '.join(iprule_lines)} → {'干净' if not iprule_lines else '⚠残留'}")
lines.append(f"- nft table 残留: `{'router_lb_marks' if 'router_lb_marks' in nft_now else '<none>'}` → {'干净' if 'router_lb_marks' not in nft_now else '⚠残留'}")
lines.append("")
lines.append("### L7")
if l7conf_exists:
    content = conf_path.read_text()
    lines.append(f"- `/etc/dnsmasq.d/router-re-l7.conf`: 存在（{len(content)} 字节）{'→ ⚠残留' if content else '→ 干净（空文件）'}")
else:
    lines.append("- `/etc/dnsmasq.d/router-re-l7.conf`: 已删 / 干净")
lines.append("")
lines.append("## 详情")
lines.append("")
for i, r in enumerate(results, 1):
    lines.append(f"### {i}. {r['name']}")
    lines.append("")
    lines.append(f"- 结果: **{r['status']}** — {r['detail']}")
    lines.append("")
    lines.append("```")
    lines.append(r["evidence"])
    lines.append("```")
    lines.append("")

with open(DOC, "w") as f:
    f.write("\n".join(lines))

print(f"\nreport: {DOC}")
print(f"SUMMARY: PASS={n_pass} FAIL={n_fail} PARTIAL={n_partial} SKIP={n_skip}")
sys.exit(0 if n_fail == 0 and n_partial == 0 else 1)
