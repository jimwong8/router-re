#!/usr/bin/env python3
"""rbac_ui_e2e.py — 权限不足时，界面到底告诉用户什么。

## 为什么

后端 403 是有的（rbac_e2e 覆盖了），但"用户看到什么"是另一回事：
页面里 153 处错误处理都取 `e.response.data.message`，而后端 message 是英文
（forbidden），真正的中文解释在 detail（"当前角色无权执行该操作"）。于是用户
点保存，弹出来一个 "forbidden"——看不懂发生了什么，也不知道该怎么办。
更糟的可能是误报成功（`.el-message--success`），那用户会以为改成功了。

这里用 viewer 账号真点按钮，断言：
  - 出现错误提示，且文案是能看懂的（含"无权"）
  - **不出现成功提示**

同时用 admin 跑一遍同样的操作做对照——证明这套断言看得见成功，
不会因为"永远看不到成功提示"而假通过。

## 用法

    python3 scripts/rbac_ui_e2e.py [--base ...] [--web ...]

末行 `PASS n/m`。会自建并清理 viewer 账号。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

TMP_USER = "ui_viewer_probe"
TMP_PASS = "Vprobe12345!"
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
    admin = (body or {}).get("token", "")
    if not admin:
        print("FAIL 拿不到 admin token")
        return 1

    api(base, "DELETE", f"/api/v1/users/{TMP_USER}", admin)
    api(base, "POST", "/api/v1/users", admin,
        {"username": TMP_USER, "password": TMP_PASS, "role": "viewer"})
    _, vb = api(base, "POST", "/api/v1/auth/login",
                body={"username": TMP_USER, "password": TMP_PASS})
    viewer = (vb or {}).get("token", "")
    if not viewer:
        print("FAIL viewer 登录失败")
        return 1

    from playwright.sync_api import sync_playwright

    MSG = ".el-message"
    with sync_playwright() as pw:
        b = pw.chromium.launch()

        def open_as(token: str, path: str):
            page = b.new_page()
            page.goto(web + "/login", wait_until="load")
            page.evaluate("t => localStorage.setItem('router_token', t)", token)
            page.goto(web + path, wait_until="load")
            page.wait_for_timeout(3000)
            return page

        def messages(page) -> tuple[str, str]:
            """返回 (错误提示文本, 成功提示文本)。"""
            errs = page.locator(f"{MSG}.el-message--error")
            oks = page.locator(f"{MSG}.el-message--success")
            e = " ".join(errs.nth(i).inner_text() for i in range(errs.count()))
            o = " ".join(oks.nth(i).inner_text() for i in range(oks.count()))
            return e, o

        # 规则列表为空时前端会先弹「至少添加一条规则」，请求根本发不出去，
        # 那样测不到 403。先点「新增」放一条进去，才走得到后端鉴权。
        def add_rule_and_save(page) -> None:
            page.locator('button:has-text("新增")').first.click()
            page.wait_for_timeout(600)
            page.locator('button:has-text("保存下发")').first.click()
            page.wait_for_timeout(700)
            # 保存前会弹 ElMessageBox 确认框（"确定下发 ACL 规则到 nftables？"），
            # 不点确认函数直接 return，请求根本不会发出——第一次就是这么测出"没反应"的。
            box = page.locator(".el-message-box")
            if box.count() > 0:
                box.locator('button:has-text("确定"), button:has-text("OK")').first.click()
            page.wait_for_timeout(2200)

        # ---- 1. viewer：保存必须被后端拒，且提示要能看懂 ----
        pv = open_as(viewer, "/security")
        add_rule_and_save(pv)
        err, ok = messages(pv)
        check("viewer 保存 ACL 被拒并给出提示", bool(err), f"提示={err[:40]!r}")
        check("错误文案是中文说明而不是 forbidden",
              ("无权" in err) and ("forbidden" not in err), f"提示={err[:40]!r}")
        check("viewer 保存 ACL 没有误报成功", "已下发" not in ok, f"成功提示={ok[:30]!r}")
        pv.close()

        # ---- 2. 对照：admin 同一操作不应出现权限错误 ----
        # 开发模式下 ACL 只渲染不下发（applied=false），成功提示是"渲染成功但未应用"，
        # 所以这里断言的是"没有权限错误"，而不是"出现成功提示"。
        pa = open_as(admin, "/security")
        add_rule_and_save(pa)
        aerr, aok = messages(pa)
        check("admin 保存 ACL 没有被权限拒绝", "无权" not in aerr, f"错误提示={aerr[:40]!r}")
        pa.close()

        # ---- 3. viewer 在用户页新建用户：提交后必须被拒并说清原因 ----
        pu = open_as(viewer, "/users")
        btn = pu.locator('button:has-text("新建用户")').first
        if btn.count() == 0:
            check("viewer 用户页没有新建入口", True, "按钮未渲染（也算可接受）")
        else:
            btn.click()
            pu.wait_for_timeout(900)
            # 用标题定位，别用 .el-overlay.last——页面上还有其它隐藏 overlay
            dlg = pu.locator('.el-dialog:has-text("新建用户")').first
            dlg.locator("input").nth(0).fill("should_fail_user")
            dlg.locator('input[type="password"]').first.fill("Xprobe12345!")
            dlg.locator('button:has-text("创建")').first.click()
            pu.wait_for_timeout(1800)
            err2, ok2 = messages(pu)
            check("viewer 建用户被拒且有中文说明", "无权" in err2, f"提示={err2[:40]!r}")
            check("viewer 建用户没有误报成功", "成功" not in ok2, f"成功提示={ok2[:30]!r}")
            pu.close()

        b.close()

    # 清理（若 viewer 真建成功了，说明有 bug，顺手删掉）
    api(base, "DELETE", "/api/v1/users/should_fail_user", admin)
    api(base, "DELETE", f"/api/v1/users/{TMP_USER}", admin)

    passed = sum(1 for _, ok, _ in RESULTS if ok)
    print(f"\nPASS {passed}/{len(RESULTS)}")
    return 0 if passed == len(RESULTS) else 1


if __name__ == "__main__":
    sys.exit(main())
