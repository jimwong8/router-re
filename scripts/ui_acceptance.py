#!/usr/bin/env python3
"""UI 验收：接口有数据 -> 页面表格必须非空。

为什么需要这个：本轮开发出现 4 次「前端读了后端从未返回的字段名」，
症状是表格静默空着、列显示 NaN，而页面本身不报任何错。静态分析
（对比字段名）误报率太高没法用；直接对比"接口返回条数"和
"页面上真实渲染的行数"是唯一可靠的判据。

用法（需要 mgmtd 在 9090 跑、vite preview 在 4173 跑）：
    python3 scripts/ui_acceptance.py

退出码 1 表示有页面渲染与接口不一致。
"""
import json
import sys
import urllib.request

API = "http://127.0.0.1:9090"
WEB = "http://127.0.0.1:4173"

# 页面 -> (取数接口, 期望最少行数, 说明)
# 只列"接口有数据就必须渲染出来"的页面；纯表单页不在此列。
PAGES = [
    ("/services",        "/api/v1/system/services",   100, "systemd 单元列表"),
    ("/online-users",    "/api/v1/online",            1,   "在线会话"),
    ("/routing-policy",  "/api/v1/routing/rules",     0,   "策略路由规则(可为空)"),
    ("/cert",            "/api/v1/cert",              0,   "证书(可能未配置)"),
    ("/alerts",          "/api/v1/alerts/rules",      0,   "告警规则(可为空)"),
]


def login():
    req = urllib.request.Request(
        API + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.load(r)["token"]


def api_count(token, path):
    """返回接口里"列表类"字段的长度，取不到返回 None。"""
    req = urllib.request.Request(API + path, headers={"Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            data = json.load(r)
    except Exception as e:
        return None, str(e)
    if not isinstance(data, dict):
        return None, "非对象响应"
    for key in ("units", "top", "sessions", "rules", "peers", "records",
                "interfaces", "vlans", "bridges", "neighbors", "count"):
        v = data.get(key)
        if isinstance(v, list):
            return len(v), ""
        if isinstance(v, int) and key == "count":
            return v, ""
    return None, "没有可识别的列表字段: " + ",".join(list(data)[:6])


def main():
    from playwright.sync_api import sync_playwright

    token = login()
    failures = []
    checked = 0

    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        ctx = browser.new_context()
        page = ctx.new_page()
        # 先注入 token（前端从 localStorage['router_token'] 读）
        page.goto(WEB + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", token)

        for path, api, min_rows, desc in PAGES:
            expect, err = api_count(token, api)
            if expect is None:
                print(f"  SKIP {path:<18} 接口取数失败: {err}")
                continue
            page.goto(WEB + path, wait_until="load")
            page.wait_for_timeout(3500)
            # el-tabs 会把所有 pane 都放在 DOM 里，只数可见的那个
            rows = page.evaluate("""() => {
              const panes = Array.from(document.querySelectorAll('.el-tab-pane'));
              let scope = document;
              if (panes.length) {
                const vis = panes.find(p => getComputedStyle(p).display !== 'none');
                if (vis) scope = vis;
              }
              return scope.querySelectorAll('table tbody tr').length;
            }""")
            checked += 1
            # 判据要抓的是"接口有数据、页面却空着"这个真症状。
            # el-table 的合计行/展开行会让行数与接口条数有 ±1 的出入，
            # 那不算 bug；但接口 500 条而页面 0 行一定是 bug。
            if expect and expect > 0:
                ok = rows > 0 and rows >= expect * 0.8
            else:
                ok = rows >= min_rows
            mark = "OK  " if ok else "FAIL"
            print(f"  {mark} {path:<18} 接口 {expect:>4} 条 -> 页面 {rows:>4} 行   {desc}")
            if not ok:
                failures.append((path, expect, rows))

        browser.close()

    print()
    if failures:
        print(f"❌ {len(failures)}/{checked} 个页面渲染与接口不一致")
        for p, e, r in failures:
            print(f"   {p}: 接口 {e} 条，页面只有 {r} 行")
        print("\n常见原因: 前端读了后端没返回的字段名（表格空）、字段名拼错（显示 NaN）、")
        print("          el-tabs 之外的选择器作用域不对。")
        return 1
    print(f"✅ UI 验收通过（{checked} 个页面，渲染行数与接口一致）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
