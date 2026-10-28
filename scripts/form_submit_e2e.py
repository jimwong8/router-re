#!/usr/bin/env python3
"""form_submit_e2e.py — 弹窗能开不等于能存。

## 为什么

`dialog_smoke` 只验「点了按钮有没有弹出对话框」。弹窗开得出来、字段也填得进去，
但保存按钮可能绑了个空函数、可能调错接口、可能后端 200 而表格不刷新——这些都在
数行数和"弹窗可见"两层判据之外。

这个脚本对三个最典型的表单做到「填 → 存 → 后端真的变了 → 页面真的刷新了」：

  1. /alerts          新增告警规则（规则名 + 匹配内容）
  2. /voucher         批量生成卡密（数量 2）
  3. /linkcheck       新增链路检测目标（名称 + 接口 + 主机）

## 用法

    python3 scripts/form_submit_e2e.py [--base ...] [--web ...]

末行 `PASS n/m`。每个用例都会清理自己造的数据，可重复跑。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

RESULTS: list[tuple[str, bool, str]] = []


def api(base: str, method: str, path: str, token: str = "", body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            raw = r.read().decode()
            return r.status, (json.loads(raw) if raw.strip() else None)
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read().decode() or "null")
        except Exception:  # noqa: BLE001
            return e.code, None
    except Exception as e:  # noqa: BLE001
        return 0, {"error": str(e)}


def check(name: str, ok: bool, detail: str = "") -> None:
    RESULTS.append((name, ok, detail))
    print(f"{'OK  ' if ok else 'FAIL'} {name}" + (f" — {detail}" if detail else ""))


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default=os.environ.get("BASE", "http://127.0.0.1:9090"))
    ap.add_argument("--web", default=os.environ.get("WEB", "http://127.0.0.1:4173"))
    args = ap.parse_args()
    base, web = args.base.rstrip("/"), args.web.rstrip("/")

    _, body = api(base, "POST", "/api/v1/auth/login",
                  body={"username": "admin", "password": "admin123"})
    token = (body or {}).get("token", "")
    if not token:
        print("FAIL 拿不到 admin token")
        return 1

    from playwright.sync_api import sync_playwright

    with sync_playwright() as pw:
        b = pw.chromium.launch()
        page = b.new_page()
        page.goto(web + "/login", wait_until="load")
        page.evaluate("t => localStorage.setItem('router_token', t)", token)

        def open_page(path: str) -> None:
            page.goto(web + path, wait_until="load")
            page.wait_for_timeout(3000)

        def click_save(dialog) -> None:
            dialog.locator('button:has-text("保存"), button:has-text("确定"), '
                           'button:has-text("创建"), button:has-text("生成")').last.click()
            page.wait_for_timeout(1800)

        # ---------- 1. 告警规则 ----------
        _, rules_before = api(base, "GET", "/api/v1/alerts/rules", token)
        orig_rules = (rules_before or {}).get("rules") or []
        name = "e2e-表单规则"
        open_page("/alerts")
        page.locator('button:has-text("新增")').first.click()
        page.wait_for_timeout(800)
        dlg = page.locator(".el-overlay").filter(has=page.locator(f'text=新增告警规则')).first
        dlg.locator('input[placeholder="如 SSH 暴力破解"]').fill(name)
        dlg.locator('input[placeholder="关键字，或正则表达式"]').fill("e2e-pattern")
        click_save(dlg)
        _, rules_after = api(base, "GET", "/api/v1/alerts/rules", token)
        names = [r.get("name") for r in ((rules_after or {}).get("rules") or [])]
        check("告警规则保存后后端有这条", name in names, f"rules={len(names)}")
        page.wait_for_timeout(500)
        check("告警页表格出现新规则", name in page.inner_text("body"))
        api(base, "POST", "/api/v1/alerts/rules", token, {"rules": orig_rules})  # 还原

        # ---------- 2. 卡密批量生成 ----------
        _, list_before = api(base, "GET", "/api/v1/voucher/list", token)
        before_n = len((list_before or {}).get("vouchers") or (list_before or {}).get("list") or [])
        open_page("/voucher")
        page.locator('button:has-text("批量生成")').first.click()
        page.wait_for_timeout(800)
        dlg = page.locator(".el-overlay").filter(has=page.locator("text=批量生成卡密")).first
        # el-input-number 的输入框：数量默认可能就是 1，这里明确填 2
        num = dlg.locator("input.el-input__inner").first
        num.fill("2")
        num.press("Tab")
        page.wait_for_timeout(300)
        click_save(dlg)
        _, list_after = api(base, "GET", "/api/v1/voucher/list", token)
        rows = (list_after or {}).get("vouchers") or (list_after or {}).get("list") or []
        after_n = len(rows)
        check("批量生成后卡密多了 2 条", after_n >= before_n + 2, f"{before_n} → {after_n}")
        # 清理本轮生成的卡密：不清理的话每跑一次回归就永久多 2 条，
        # 跑几十轮之后列表里全是 e2e 垃圾。
        def codes(rows_):
            return {r.get("code") for r in rows_ if isinstance(r, dict)}
        new_codes = codes(rows) - codes((list_before or {}).get("vouchers") or (list_before or {}).get("list") or [])
        for c in new_codes:
            api(base, "DELETE", f"/api/v1/voucher?code={c}", token)
        _, final = api(base, "GET", "/api/v1/voucher/list", token)
        final_n = len((final or {}).get("vouchers") or (final or {}).get("list") or [])
        check("生成的卡密已清理", final_n == before_n, f"{final_n} 条（基线 {before_n}）")

        # ---------- 3. 链路检测目标 ----------
        open_page("/linkcheck")
        page.locator('button:has-text("新增目标")').first.click()
        page.wait_for_timeout(800)
        dlg = page.locator(".el-overlay").filter(has=page.locator("text=新增链路检测目标")).first
        tname = "e2e-链路目标"
        dlg.locator("input").nth(0).fill(tname)      # 名称
        dlg.locator("input").nth(1).fill("eth0")     # 接口
        # 类型下拉默认 tcp/ping，不动；主机
        dlg.locator('input[placeholder="8.8.8.8 或 example.com"]').fill("127.0.0.1")
        # 这个页面是两段式：弹窗「确定」只把目标加进本地列表（提示"记得保存生效"），
        # 真正落库要再点页面上那个独立的「保存」（saveAll 会 POST 整个 targets 列表）。
        # 只点「确定」的话后端还是 0 条——一开始就是这么误判成保存坏了的。
        dlg.locator('button:has-text("确定")').click()
        page.wait_for_timeout(600)
        page.locator('.el-card button:has-text("保存")').first.click()
        page.wait_for_timeout(1800)
        _, targets = api(base, "GET", "/api/v1/linkcheck/targets", token)
        tlist = (targets or {}).get("targets") or []
        check("链路目标保存后后端有这条", any(t.get("name") == tname for t in tlist),
              f"targets={len(tlist)}")
        page.wait_for_timeout(500)
        check("链路检测页显示新目标", tname in page.inner_text("body"))
        for t in tlist:  # 清理
            if t.get("name") == tname and t.get("id") is not None:
                api(base, "DELETE", f"/api/v1/linkcheck/targets/{t['id']}", token)

        b.close()

    passed = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\nPASS {passed}/{len(RESULTS)}")
    return 0 if passed == len(RESULTS) else 1


if __name__ == "__main__":
    sys.exit(main())
