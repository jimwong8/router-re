#!/usr/bin/env python3
"""dialog_smoke.py — 每个「新增/添加/生成」按钮都必须真的弹出对话框。

## 为什么

ui_acceptance 只比「接口 N 条 vs 页面 N 行」。Users.vue 的三个 el-dialog 用了
Element Plus 2.x 已移除的 `:visible`，四个按钮全都没反应，而行数一直是 8 页全 OK。
同类失效在别的页面一样会静默通过——只要列表渲染正确。

这个脚本不看行数，只看「点了有没有东西弹出来」。

## 用法

    python3 scripts/dialog_smoke.py [--web http://127.0.0.1:4173]

末行 `PASS n/m`；有按钮点了没反应就 exit 1，并列出是哪个页面哪个按钮。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.request

# 只挑「点开就弹窗」的页面；探测型按钮（执行 ping/traceroute）不在此列
PAGES = [
    ("/users", ["新建用户"]),
    ("/voucher", ["批量生成", "生成"]),
    ("/routing-policy", ["新增"]),
    ("/alerts", ["新增"]),
    ("/cert", ["生成自签证书", "生成"]),
    ("/security-plus", ["新增"]),
    ("/linkcheck", ["新增目标"]),
]

# 注意：不能用 offsetParent 判可见——Element Plus 的 overlay 是 position:fixed，
# 而 offsetParent 对 fixed 元素恒为 null，会把"正常打开"全判成没打开（实测 7/7 假 FAIL）。
VISIBLE_DIALOG_JS = (
    "[...document.querySelectorAll('.el-overlay')]"
    ".filter(e=>{const r=e.getBoundingClientRect();"
    "return r.width>0&&r.height>0&&getComputedStyle(e).visibility!=='hidden';}).length"
)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default=os.environ.get("BASE", "http://127.0.0.1:9090"))
    ap.add_argument("--web", default=os.environ.get("WEB", "http://127.0.0.1:4173"))
    args = ap.parse_args()
    base, web = args.base.rstrip("/"), args.web.rstrip("/")

    req = urllib.request.Request(
        base + "/api/v1/auth/login",
        data=json.dumps({"username": "admin", "password": "admin123"}).encode(),
        headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=20) as r:
        token = json.load(r)["token"]

    from playwright.sync_api import sync_playwright

    results: list[tuple[str, bool, str]] = []

    def check(name: str, ok: bool, detail: str = "") -> None:
        results.append((name, ok, detail))
        print(f"{'OK  ' if ok else 'FAIL'} {name}" + (f" — {detail}" if detail else ""))

    with sync_playwright() as pw:
        b = pw.chromium.launch()
        page = b.new_page()
        page.goto(web + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", token)

        for path, labels in PAGES:
            page.goto(web + path, wait_until="load")
            page.wait_for_timeout(3000)
            for label in labels:
                btn = page.locator(f'button:has-text("{label}")').first
                if btn.count() == 0:
                    check(f"{path} {label}", False, "按钮不存在")
                    continue
                try:
                    btn.click(timeout=5000)
                except Exception as e:  # noqa: BLE001
                    check(f"{path} {label}", False, f"点不动: {type(e).__name__}")
                    continue
                page.wait_for_timeout(900)
                n = page.evaluate(VISIBLE_DIALOG_JS)
                check(f"{path} {label} 弹出对话框", bool(n), f"可见 overlay={n}")
                # 关掉再试下一个，避免遮罩挡住后续点击
                if n:
                    page.keyboard.press("Escape")
                    page.wait_for_timeout(400)
                break  # 每页只验第一个命中的标签，避免重复弹同一个窗

        b.close()

    passed = sum(1 for _, ok, _ in results if ok)
    print(f"\nPASS {passed}/{len(results)}")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
