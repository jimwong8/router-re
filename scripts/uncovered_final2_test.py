#!/usr/bin/env python3
"""p96-uncovered-final2: 最后 2 条未覆盖端点的批测试（vlan + wol/targets，真实打本机 9090）。

端点（从 server.go 确认路由，非猜）：
  - VLAN
      GET  /api/v1/vlan            handleGetVLAN      -> {vlans:[]}
      POST /api/v1/vlan            handleSetVLAN      body: 单个VLAN对象 或 {vlans:[...]}
      DELETE /api/v1/vlan/{name}   handleDeleteVLAN   path {name} -> `ip link del {name}`
      注：没有 GET /api/v1/vlan/{name}，“确认”用 GET 列表查名。
      ApplyVLAN 会真实执行 `ip link add link <parent> name <name> type vlan id <vid>`
        （root 下发），清理用 DELETE 返回 ip link del。选择 parent=ens18、name=p96test 进行
        隔离（仅增 p96test 子接口，不触业务）。
  - WOL
      GET  /api/v1/wol/targets   handleGetWOLTargets  -> {targets:[]}
      POST /api/v1/wol/targets   handleSetWOLTargets  body: {targets:[...]} 全量替换
      注：没有 DELETE /wol/targets/{id}，清理用 POST {targets:[]} 全量替换清空。
      WOLTarget{Name,MAC(6 字节),Iface,Broadcast(可选)}，MAC=00:11:22:33:44:55。

产出: docs/uncovered-final2.md（脚本生成，非手写）。
"""
import json, os, time, urllib.request, urllib.error
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "uncovered-final2.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
VLAN_IFACE = os.environ.get("VLAN_IFACE", "p96test")
VLAN_PARENT = os.environ.get("VLAN_PARENT", "ens18")
VLAN_VID = int(os.environ.get("VLAN_VID", "100"))
WOL_NAME = "p96test"
WOL_MAC = "00:11:22:33:44:55"
WOL_IFACE = os.environ.get("WOL_IFACE", "ens18")


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
    b = body if isinstance(body, dict) else {}
    tok = b.get("token")
    if not tok:
        raise SystemExit(f"no token in login response: {body}")
    return tok


def d(obj):
    return obj if isinstance(obj, dict) else {}


results = []


def rec(name, status, detail, evidence):
    results.append({"name": name, "status": status, "detail": detail, "evidence": evidence})
    print(f"{status:4} {name}: {detail}")


def short(x, n=220):
    s = json.dumps(x, ensure_ascii=False) if not isinstance(x, str) else x
    return s if len(s) <= n else s[: n - 3] + "..."


def vlan_exists(token, name):
    st, body, _ = call("GET", "/api/v1/vlan", token=token)
    if st != 200:
        return None
    for v in d(body).get("vlans", []):
        if v.get("name") == name:
            return v
    return None


def wol_exists(token, name):
    st, body, _ = call("GET", "/api/v1/wol/targets", token=token)
    if st != 200:
        return None
    for t in d(body).get("targets", []):
        if t.get("name") == name:
            return t
    return None


token = login()
rec("登录 admin", "PASS", "Bearer token 获取成功", "token:<redacted>")

# ============ 1. VLAN ============
# 1a. 建一条 p96test vlan（POST 单个对象；服务器真实下发 ip link add link ens18 ...）
vlan_body = {"parent": VLAN_PARENT, "vid": VLAN_VID, "name": VLAN_IFACE}
st, body, raw = call("POST", "/api/v1/vlan", token=token, body=vlan_body)
b = d(body)
applied = b.get("applied")
actual = b.get("vlans", [])
found_in_apply = any(v.get("name") == VLAN_IFACE for v in (actual or []))
if st == 200 and applied and found_in_apply:
    rec("vlan POST 建 p96test", "PASS", f"applied={applied} in_returned_list", short(raw))
else:
    rec("vlan POST 建 p96test", "FAIL", f"HTTP {st} applied={applied} in_list={found_in_apply}", raw)

# 1b. GET 列表确认（源码无 GET /{name} 独立路由，用列表查名）
v = vlan_exists(token, VLAN_IFACE)
if v:
    rec("vlan GET 列表确认", "PASS", f"name={v.get('name')} parent={v.get('parent')} vid={v.get('vid')}", json.dumps(v, ensure_ascii=False))
else:
    rec("vlan GET 列表确认", "FAIL", f"列表中未找到 {VLAN_IFACE}", "")

# 1c. DELETE 清理
st, body, raw = call("DELETE", f"/api/v1/vlan/{VLAN_IFACE}", token=token)
b = d(body)
after = vlan_exists(token, VLAN_IFACE)
if st == 200 and after is None:
    rec("vlan DELETE 清理", "PASS", f"HTTP {st} deleted={b.get('deleted')} 列表中已无", short(raw))
else:
    rec("vlan DELETE 清理", "FAIL", f"HTTP {st} deleted={b.get('deleted')} 仍在列表={after is not None}", raw)

# ============ 2. wol/targets ============
# 2a. GET 列表
st, body, raw = call("GET", "/api/v1/wol/targets", token=token)
b = d(body)
if st == 200 and isinstance(b.get("targets"), list):
    rec("wol GET 列表", "PASS", f"HTTP {st} count={b.get('count')}", short(raw))
else:
    rec("wol GET 列表", "FAIL", f"HTTP {st} count={b.get('count') if isinstance(b, dict) else '?'}", raw)

# 2b. POST 建一条 p96test 目标（全量替换；MAC=00:11:22:33:44:55 6 字节合法）
wol_t = {"name": WOL_NAME, "mac": WOL_MAC, "iface": WOL_IFACE}
st, body, raw = call("POST", "/api/v1/wol/targets", token=token, body={"targets": [wol_t]})
b = d(body)
t = wol_exists(token, WOL_NAME)
if st == 200 and b.get("count") == 1 and t is not None:
    rec("wol POST 建 p96test", "PASS", f"count={b.get('count')} mac={t.get('mac') if t else None}", short(raw))
else:
    rec("wol POST 建 p96test", "FAIL", f"HTTP {st} count={b.get('count') if isinstance(b, dict) else '?'} in_list={t is not None}", raw)

# 2c. GET 确认
t = wol_exists(token, WOL_NAME)
if t and t.get("mac") == WOL_MAC and t.get("iface") == WOL_IFACE:
    rec("wol GET 确认", "PASS", f"name={t.get('name')} mac={t.get('mac')} iface={t.get('iface')}", json.dumps(t, ensure_ascii=False))
else:
    rec("wol GET 确认", "FAIL", f"t={t}", raw)

# 2d. 清理：POST {targets:[]} 全量替换（源码无 DELETE 端点）。wol 文件写 0600，服务器自持。
st, body, raw = call("POST", "/api/v1/wol/targets", token=token, body={"targets": []})
b = d(body)
t_after = wol_exists(token, WOL_NAME)
if st == 200 and t_after is None and b.get("count") == 0:
    rec("wol 清理（POST 空目标）", "PASS", f"count={b.get('count')} 列表中已无 {WOL_NAME}", short(raw))
else:
    rec("wol 清理（POST 空目标）", "FAIL", f"HTTP {st} count={b.get('count') if isinstance(b, dict) else '?'} still_present={t_after is not None}", raw)

# ============ 报告 ============
os.makedirs(ROOT / "docs", exist_ok=True)
n_pass = sum(1 for r in results if r["status"] == "PASS")
n_fail = sum(1 for r in results if r["status"] == "FAIL")
lines = ["# p96 未覆盖端点批测试报告（vlan、wol/targets）", ""]
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
v_clean = vlan_exists(token, VLAN_IFACE) is None
lines.append(f"- vlan `{VLAN_IFACE}` 残留: {'存在' if not v_clean else '已删除/干净'}")
t_clean = wol_exists(token, WOL_NAME) is None
lines.append(f"- wol `{WOL_NAME}` 残留: {'存在' if not t_clean else '已删除/干净'}")
lines.append(f"- 全部清理干净: {'YES' if v_clean and t_clean else 'NO'}")
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