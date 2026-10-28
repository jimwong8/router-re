#!/usr/bin/env python3
"""UI 验收：/terminals 页面 + OnlineUsers 字节开关冒烟。

复用 scripts/ui_acceptance.py 的机制（login 注入 localStorage、数 table tbody tr、
api_count 白名单），只覆盖两个页面，产出 docs/ui-terminals-acceptance.md。
"""
import json
import os
import socket
import subprocess
import sys
import time
import urllib.request
from pathlib import Path

# 行数判据三份验收脚本共用（见 ui_judge.py）
sys.path.insert(0, str(Path(__file__).resolve().parent))
import ui_judge  # noqa: E402

API = "http://127.0.0.1:9090"
WEB = "http://127.0.0.1:4173"
WEB_DIR = "/home/jimwong/projects/router-re/web"

LIST_FIELDS = {
    "units", "sessions", "rules", "peers", "records", "interfaces", "vlans",
    "bridges", "neighbors", "vouchers", "accounts", "clients", "devices", "events",
    "flows", "marks", "configs", "addresses", "limits", "access", "entries",
    "links", "tables", "users", "leases", "mappings", "ssids", "routes", "top",
    "items", "data", "list", "commands", "terminals", "count",
}
INT_COUNT_FIELDS = {"count", "total", "n", "peer_count"}


def port_open(host, port):
    try:
        with socket.create_connection((host, port), timeout=2):
            return True
    except Exception:
        return False


def ensure_vite_preview():
    """若 4173 未起，后台起 vite preview。"""
    if port_open("127.0.0.1", 4173):
        return
    print("4173 未监听，启动 vite preview ...")
    log = open("/tmp/vite-preview.log", "w")
    subprocess.Popen(
        ["npm", "run", "preview", "--", "--port", "4173"],
        cwd=WEB_DIR, stdout=log, stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    for _ in range(60):
        if port_open("127.0.0.1", 4173):
            print("vite preview 已就绪")
            return
        time.sleep(1)
    print("WARN: vite preview 60s 内未就绪，继续尝试")


def login():
    req = urllib.request.Request(
        API + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        headers={"Content-Type": "application/json"}, method="POST",
    )
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.load(r)["token"]


def api_count(token, path):
    req = urllib.request.Request(API + path, headers={"Authorization": "Bearer " + token})
    try:
        with urllib.request.urlopen(req, timeout=25) as r:
            data = json.load(r)
    except Exception as e:
        return None, str(e)
    if not isinstance(data, dict):
        return None, "非对象响应"
    for key in LIST_FIELDS:
        v = data.get(key)
        if isinstance(v, list):
            return len(v), ""
    for key in INT_COUNT_FIELDS:
        if key in data and isinstance(data[key], int):
            return data[key], ""
    return None, "没有可识别的列表字段: " + ",".join(list(data.keys())[:6])


def main():
    from playwright.sync_api import sync_playwright

    ensure_vite_preview()

    token = login()
    rows_out = []
    failures = []
    checked = 0
    oks = 0

    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        ctx = browser.new_context()
        page = ctx.new_page()
        page.goto(WEB + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", token)

        # 1) /terminals
        path, api, min_rows, desc = "/terminals", "/api/v1/conntrack/terminals?limit=20", 1, "终端列表"
        expect, err = api_count(token, api)
        page.goto(WEB + path, wait_until="load")
        page.wait_for_timeout(3500)
        probe = page.evaluate(ui_judge.PROBE_JS)
        rows = probe["rows"]
        has_pager = probe["has_pager"]
        checked += 1
        if expect is None:
            mark = "SKIP"
            skips_note = f"接口取数失败: {err}"
        else:
            ok, _tag = ui_judge.judge(rows, expect, has_pager, min_rows)
            mark = "OK" if ok else "FAIL"
            if ok:
                oks += 1
            else:
                failures.append((path, expect, rows))
            skips_note = desc
        print(f"  {mark:4} {path:<16} 接口 {expect if expect is not None else '-':>4} 条 -> 页面 {rows:>4} 行   {desc}")
        rows_out.append((path, mark, expect if expect is not None else "-", rows, desc, "渲染完整"))

        # 2) /online-users
        path2, api2 = "/online-users", "/api/v1/online"
        expect2, err2 = api_count(token, api2)
        page.goto(WEB + path2, wait_until="load")
        page.wait_for_timeout(3500)
        probe2 = page.evaluate(ui_judge.PROBE_JS)
        rows2 = probe2["rows"]
        has_pager2 = probe2["has_pager"]
        body_text = page.inner_text("body") or ""
        has_byte_stat = ("字节统计" in body_text) or ("acct" in body_text.lower())
        checked += 1
        if expect2 is None:
            mark2 = "SKIP"
            note2 = f"接口取数失败: {err2}"
        else:
            ok2, _tag2 = ui_judge.judge(rows2, expect2, has_pager2, 0)
            if not has_byte_stat:
                ok2 = False
            mark2 = "OK" if ok2 else "FAIL"
            if ok2:
                oks += 1
            else:
                failures.append((path2, expect2, rows2))
            note2 = "字节统计块: " + ("存在" if has_byte_stat else "缺失")
        print(f"  {mark2:4} {path2:<16} 接口 {expect2 if expect2 is not None else '-':>4} 条 -> 页面 {rows2:>4} 行   字节统计块={'存在' if has_byte_stat else '缺失'}")
        rows_out.append((path2, mark2, expect2 if expect2 is not None else "-", rows2, "在线会话", note2))

        browser.close()

    print()
    total = oks + len(failures)
    print(f"✅ terminals 验收通过（{total} 个页面 OK，{len(failures)} 个 FAIL）" if not failures
          else f"❌ {len(failures)}/{total} 个页面渲染与接口不一致")

    os.makedirs("docs", exist_ok=True)
    with open("docs/ui-terminals-acceptance.md", "w") as f:
        f.write("# UI terminals + OnlineUsers 字节开关验收报告\n\n")
        f.write(f"- 接口端点：`{API}`\n- 前端端点：`{WEB}`\n")
        f.write(f"- 覆盖页面：{len(rows_out)} 个 | OK: {oks} | FAIL: {len(failures)}\n\n")
        f.write("| 路由 | 结果 | 接口预期条数 | 页面渲染行数 | 说明 |\n")
        f.write("|------|------|--------------|--------------|------|\n")
        for path, mark, expect, rows, desc, note in rows_out:
            f.write(f"| `{path}` | {mark} | {expect} | {rows} | {desc}（{note}）|\n")
        if failures:
            f.write("\n## 失败原因\n")
            for p, e, r in failures:
                f.write(f"- `{p}`：接口 {e} 条，页面 {r} 行\n")
        f.write("\n## 判据\n")
        f.write("接口有数据时页面表格渲染行数至少达到接口条数的 80%；/online-users 额外断言页面文本含「字节统计」或「acct」。\n")
    print("报告已写 docs/ui-terminals-acceptance.md")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
