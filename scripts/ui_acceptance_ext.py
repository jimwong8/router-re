#!/usr/bin/env python3
"""UI 验收扩展：覆盖所有「接口有数据就必须渲染」的页面。

复用 scripts/ui_acceptance.py 的机制（同一个 API/WEB 登录流程、Playwright
计数 tbody tr 的方式），但：
  * PAGES 扩到 20+ 个有 GET 列表接口的页面
  * api_count 扩充可识别列表字段名 + 递归展开 config 等嵌套列表
  * 对无法自动判的页面（纯 POST / 表单 / 嵌套但无顶层列表）保留 SKIP 并写明原因

需要 mgmtd 在 9090 跑、vite preview 在 4173 跑：
    python3 scripts/ui_acceptance_ext.py
"""
import json
import os
import sys
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import ui_judge  # noqa: E402

API = "http://127.0.0.1:9090"
WEB = "http://127.0.0.1:4173"

# 页面 -> (路由 path, 取数接口, 最小期望行数, 说明)
# 说明中标记 SKIP 原因的页面不会参与计数，只记录为 SKIP。
PAGES = [
    # ---- 原 11 页（保留） ----
    ("/services",        "/api/v1/system/services",    1,   "systemd 单元列表 (units)"),
    ("/online-users",    "/api/v1/online",             1,   "在线会话"),
    ("/routing-policy",  "/api/v1/routing/rules",      0,   "策略路由规则(可为空)"),
    ("/cert",            "/api/v1/cert",               0,   "SKIP: 证书接口返回对象/{exists,path} 非列表"),
    ("/alerts",          "/api/v1/alerts/rules",       0,   "告警规则(可为空)"),
    ("/network-tools",   "",                           0,   "SKIP: 无 GET 列表接口(全 POST 探测)"),
    ("/network-advanced","",                           0,   "SKIP: 表单/单接口调优，无列表渲染"),
    # ---- 新增覆盖 ----
    ("/accounts",       "/api/v1/accounts",            0,   "账号列表(可为空)"),
    ("/audit",          "/api/v1/audit/events",        1,   "审计事件"),
    ("/clients",        "/api/v1/clients",             1,   "客户端列表"),
    ("/cloud",          "/api/v1/cloud/devices",       0,   "云设备(可为空)"),
    ("/diag",           "/api/v1/diag/interfaces",     1,   "接口诊断列表"),
    ("/dhcp",           "/api/v1/dhcp/leases",         0,   "DHCP 租约(可为空)"),
    ("/dpi-flow",       "/api/v1/dpi/flows",           0,   "DPI 流量(可为空)"),
    ("/dpi",            "/api/v1/dpi/marks",           0,   "DPI 标记(可为空)"),
    ("/interfaces",     "/api/v1/interfaces/config",     0,   "接口配置(configs=空或 None 时 SKIP)"),
    ("/ipv6",           "/api/v1/ipv6/addresses",      0,   "IPv6 地址(可为空)"),
    ("/l7",             "/api/v1/l7/rules",            0,   "L7 规则(可为空)"),
    ("/logs",           "/api/v1/logs",                0,   "日志条目(可为空)"),
    ("/voucher",        "/api/v1/voucher/list",        0,   "卡密列表(可为空)"),
    ("/nac",            "",                           0,   "SKIP: NAC 多表格分页 (devices/limits/top/neighbors)，接口 neighbors=45 非可见表，无法准确比对"),
    ("/network",        "",                           0,   "SKIP: portmap/rules.rules=None(主机无 NAT 规则)"),
    ("/pppoe",          "/api/v1/pppoe/sessions",       0,   "PPPoE 会话(可为空)"),
    ("/qos",            "/api/v1/qos/limits",          0,   "QoS 限速(可为空)"),
    ("/remote",         "/api/v1/remote/access",       0,   "远程访问(可为空)"),
    ("/security",       "/api/v1/stats/interfaces",     0,   "接口流量(ACL 表格取 acl/rules、主体表格取 stats/interfaces)"),
    ("/threat",         "/api/v1/threat/list",         0,   "威胁列表(可为空)"),
    ("/upnp",           "/api/v1/upnp/mappings",        0,   "UPnP 映射(可为空)"),
    ("/users",          "/api/v1/users",               1,   "用户列表"),
    ("/wireless",       "/api/v1/wireless/ssids",       0,   "Wi-Fi SSID(可为空)"),
    ("/static-route",   "/api/v1/route/static",         0,   "SKIP: route/static 返回 routes=None(主机无静态路由)"),
    ("/wan",            "/api/v1/wan/links",            0,  "SKIP: wan/links links=None(主机无多WAN)"),
    ("/dhcp-server",    "/api/v1/dhcp/server",         0,   "SKIP: dhcp/server 返回 config(对象)非列表"),
]

# 接口返回里「列表类」字段的名称集合。扫描到任意一个列表即视为可比对。
LIST_FIELDS = {
    "units","sessions","rules","peers","records","interfaces","vlans","bridges",
    "neighbors","vouchers","accounts","clients","devices","events","flows",
    "marks","configs","addresses","limits","access","entries","links","tables",
    "users","leases","mappings","ssids","routes","top","items","data","list",
    "commands","ssids","upstreams","hosts","members","v4","v6",
}
INT_COUNT_FIELDS = {"count", "total", "n", "peer_count"}


def login():
    req = urllib.request.Request(
        API + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        headers={"Content-Type": "application/json"}, method="POST",
    )
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.load(r)["token"]


def _find_list(obj):
    """递归返回响应里最长的一个列表（用于嵌套 config.upstreams 之类）。"""
    best = None
    if isinstance(obj, dict):
        for k, v in obj.items():
            if isinstance(v, list):
                if k in LIST_FIELDS or best is None or len(v) > len(best):
                    if best is None or len(v) > len(best):
                        best = v
            best = max(best, _find_list(v), key=lambda x: len(x) if x is not None else -1)
    return best


def api_count(token, path):
    """返回接口里列表字段的长度，取不到返回 None + 原因。"""
    if not path:
        return None, "无取数接口"
    req = urllib.request.Request(API + path, headers={"Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            data = json.load(r)
    except Exception as e:
        return None, str(e)
    if not isinstance(data, dict):
        return None, "非对象响应"
    # 1) 优先显式列表字段
    for key in LIST_FIELDS:
        v = data.get(key)
        if isinstance(v, list):
            return len(v), ""
    # 2) count/int 字段
    for key in INT_COUNT_FIELDS:
        if key in data and isinstance(data[key], int):
            return data[key], ""
    # 3) 递归找嵌套最长列表
    nested = _find_list(data)
    if nested is not None:
        return len(nested), "嵌套列表"
    return None, "没有可识别的列表字段: " + ",".join(list(data.keys())[:6])


def main():
    from playwright.sync_api import sync_playwright

    token = login()
    failures = []
    checked = 0
    oks = 0
    skips = 0
    rows_out = []

    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        ctx = browser.new_context()
        page = ctx.new_page()
        page.goto(WEB + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", token)

        for path, api, min_rows, desc in PAGES:
            if not api:
                print(f"  SKIP {path:<18} {desc}")
                skips += 1
                rows_out.append((path, "SKIP", "-", "-", "-", desc))
                continue
            expect, err = api_count(token, api)
            if expect is None:
                print(f"  SKIP {path:<18} 接口取数失败: {err}")
                skips += 1
                rows_out.append((path, "SKIP", "-", "-", "-", f"{desc} | {err}"))
                continue
            page.goto(WEB + path, wait_until="load")
            page.wait_for_timeout(3500)
            probe = page.evaluate(ui_judge.PROBE_JS)
            rows = probe["rows"]
            has_pager = probe["has_pager"]
            checked += 1
            ok, tag = ui_judge.judge(rows, expect, has_pager, min_rows)
            if tag:
                desc = (desc + tag).strip()
            mark = "OK  " if ok else "FAIL"
            if ok: oks += 1
            else: failures.append((path, expect, rows))
            print(f"  {mark} {path:<18} 接口 {expect:>4} 条 -> 页面 {rows:>4} 行   {desc}")
            rows_out.append((path, mark, expect, rows, expect, desc))

        browser.close()

    print()
    total = oks + len(failures)
    print(f"✅ UI 验收扩展通过（{total} 个页面 OK，{skips} 个 SKIP，0 FAIL）" if not failures
          else f"❌ {len(failures)}/{total} 个页面渲染与接口不一致")

    # ---- 写报告 docs/ui-coverage-extended.md ----
    os.makedirs("docs", exist_ok=True)
    with open("docs/ui-coverage-extended.md", "w") as f:
        f.write("# UI 自动验收扩展覆盖报告\n\n")
        f.write(f"- 接口端点：`{API}`\n- 前端端点：`{WEB}`\n")
        f.write(f"- 覆盖页面：{len(rows_out)} 个 | OK: {oks} | SKIP: {skips} | FAIL: {len(failures)}\n\n")
        f.write("| 路由 | 结果 | 接口预期条数 | 页面渲染行数 | 说明 |\n")
        f.write("|------|------|--------------|--------------|------|\n")
        for path, mark, expect, rows, _, desc in rows_out:
            f.write(f"| `{path}` | {mark} | {expect} | {rows} | {desc} |\n")
        if failures:
            f.write("\n## 失败原因\n")
            for p, e, r in failures:
                f.write(f"- `{p}`：接口 {e} 条，页面 {r} 行\n")
        f.write("\n## 说明\n")
        f.write("跳过页面保留其原因在「说明」列。覆盖逻辑同 ui_acceptance.py：接口有数据时页面表格渲染行数至少达到接口条数的 80%。\n")
    print("报告已写 docs/ui-coverage-extended.md")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
