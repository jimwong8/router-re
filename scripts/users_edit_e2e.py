#!/usr/bin/env python3
"""users_edit_e2e.py — 验「Users 页改角色/停用」这个交互真的能用。

## 为什么不用 ui_acceptance

ui_acceptance 只比「接口返回 N 条 vs 页面渲染 N 行」。角色编辑是纯交互：
按钮在不在、弹窗开不开、选了新角色点保存有没有真的发出 PUT、表格有没有跟着变。
行数一致的页面，编辑功能可能整块是坏的（按钮绑了个空函数也照样 8 页全 OK）。

## 用法

    python3 scripts/users_edit_e2e.py [--base http://127.0.0.1:9090]

末行 `PASS n/5`；不对就 exit 1。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.request

TMP_USER = "ui_edit_probe"
TMP_PASS = "Probe12345!"


def api(base: str, method: str, path: str, token: str = "", body: dict | None = None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read().decode()
            return r.status, (json.loads(raw) if raw.strip() else None)
    except urllib.error.HTTPError as e:
        return e.code, None


def main() -> int:
    ap = argparse.ArgumentParser()
    # UI 走 Vite preview（4173，history 路由有 SPA 兜底）；9090 的 Go 只服务 /
    ap.add_argument("--base", default=os.environ.get("BASE", "http://127.0.0.1:9090"))
    ap.add_argument("--web", default=os.environ.get("WEB", "http://127.0.0.1:4173"), help="vite preview 地址")
    args = ap.parse_args()
    base = args.base.rstrip("/")
    web = args.web.rstrip("/")

    _, body = api(base, "POST", "/api/v1/auth/login", body={"username": "admin", "password": "admin123"})
    admin = (body or {}).get("token", "")
    if not admin:
        print("FAIL 拿不到 admin token")
        return 1

    # 准备一个可编辑的账号
    api(base, "DELETE", f"/api/v1/users/{TMP_USER}", admin)
    api(base, "POST", "/api/v1/users", admin,
        {"username": TMP_USER, "password": TMP_PASS, "role": "viewer"})

    from playwright.sync_api import sync_playwright

    results: list[tuple[str, bool, str]] = []

    def check(name: str, ok: bool, detail: str = "") -> None:
        results.append((name, ok, detail))
        print(f"{'OK  ' if ok else 'FAIL'} {name}" + (f" — {detail}" if detail else ""))

    with sync_playwright() as p:
        b = p.chromium.launch()
        page = b.new_page()
        # 与 ui_acceptance 一致：前端从 localStorage['router_token'] 取 token，
        # 先落一个再进页面（登录表单本身由 smoke_new_routes 覆盖，这里只验编辑交互）
        page.goto(web + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", admin)
        page.goto(web + "/users", wait_until="load")
        page.wait_for_timeout(3000)

        # 1. 目标行出现
        row = page.locator("tr", has_text=TMP_USER).first
        check("列表里能看到目标账号", row.count() > 0)

        # 2. 该行有「编辑」按钮
        edit_btn = row.locator('button:has-text("编辑")').first
        check("行内有编辑按钮", edit_btn.count() > 0)

        # 3. 点开后有角色下拉 + 启用开关
        edit_btn.click()
        page.wait_for_timeout(800)
        dialog = page.locator(".el-dialog:visible").first
        has_select = dialog.locator(".el-select").count() > 0
        has_switch = dialog.locator(".el-switch").count() > 0
        check("弹窗里有角色下拉和启用开关", has_select and has_switch,
              f"select={has_select} switch={has_switch}")

        # 4. 改成 operator 并保存，接口侧要真的变
        dialog.locator(".el-select").first.click()
        page.wait_for_timeout(400)
        page.locator('.el-select-dropdown__item:has-text("operator")').first.click()
        page.wait_for_timeout(300)
        dialog.locator('button:has-text("保存"), button:has-text("确定")').first.click()
        page.wait_for_timeout(1500)

        _, users = api(base, "GET", "/api/v1/users", admin)
        role = ""
        for u in (users or {}).get("users", []):
            if u.get("username") == TMP_USER:
                role = u.get("role", "")
        check("保存后后端角色真的变了", role == "operator", f"role={role or '缺失'}")

        # 5. 表格刷新后显示新角色（不是只有接口变了）
        page.wait_for_timeout(600)
        row2 = page.locator("tr", has_text=TMP_USER).first
        shown = "operator" in (row2.inner_text() if row2.count() else "")
        check("表格已刷新显示新角色", shown)

        b.close()

    # 清理
    api(base, "DELETE", f"/api/v1/users/{TMP_USER}", admin)

    passed = sum(1 for _, ok, _ in results if ok)
    print(f"\nPASS {passed}/{len(results)}")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
