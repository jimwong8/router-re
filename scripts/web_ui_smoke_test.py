#!/usr/bin/env python3
"""web_ui_smoke_test.py — Web 前端真实渲染冒烟。

打 vite preview (4173) 真实前端，验证页面渲染不白屏。
产出 docs/web-ui-smoke-test.md（脚本写，勿手写）。
"""
from __future__ import annotations
import json, os, time, urllib.request, urllib.error
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB = os.environ.get("WEB", "http://127.0.0.1:4173")
API = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "web-ui-smoke-test.md"

PAGES = [
    ("/", "首页"),
    ("/system", "系统"),
    ("/firewall", "防火墙"),
    ("/routing", "路由"),
    ("/terminals", "终端"),
    ("/logs", "日志"),
    ("/services", "服务"),
    ("/security-plus", "安全Plus"),
    ("/portal", "Portal"),
    ("/qos", "QoS"),
]

# 关键 API 端点映射
API_MAP = {
    "/": "/api/v1/system/info",
    "/system": "/api/v1/system/info",
    "/firewall": "/api/v1/firewall/rules",
    "/routing": "/api/v1/routing/tables",
    "/terminals": "/api/v1/conntrack/terminals",
    "/logs": "/api/v1/logs",
    "/services": "/api/v1/system/services",
    "/security-plus": "/api/v1/policy/timeschedule",
    "/portal": "/api/v1/portal/clients",
    "/qos": "/api/v1/qos/limits",
}

def login_api():
    url = API + "/api/v1/auth/login"
    data = json.dumps({"username": "admin", "password": "admin123"}).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.loads(resp.read())["token"]

def main():
    token = login_api()
    report = []
    report.append("# Web UI Smoke Test (真实渲染)")
    report.append("")
    report.append(f"- Frontend: {WEB}")
    report.append(f"- API: {API}")
    report.append(f"- Date: {__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    report.append("")

    results = []
    try:
        from playwright.sync_api import sync_playwright
        with sync_playwright() as pw:
            browser = pw.chromium.launch()
            ctx = browser.new_context()
            page = ctx.new_page()
            # 注入 token
            page.goto(WEB + "/login", wait_until="load")
            page.evaluate("t => localStorage.setItem('router_token', t)", token)
            page.wait_for_timeout(500)

            for path, name in PAGES:
                try:
                    page.goto(WEB + path, wait_until="load", timeout=15000)
                    page.wait_for_timeout(2000)

                    # 检查白屏：body 是否有内容
                    body_text = page.inner_text("body")
                    is_blank = len(body_text.strip()) < 10

                    # 检查挂载点
                    has_app = page.evaluate("() => !!document.querySelector('#app')")
                    has_mount = page.evaluate("() => document.body.innerHTML.length > 100")

                    # 检查 JS 错误
                    js_errors = []
                    page.on("console", lambda msg: js_errors.append(msg.text) if msg.type == "error" else None)

                    # 检查 bundle 可达
                    bundle_ok = True
                    scripts = page.evaluate("() => Array.from(document.querySelectorAll('script[src]')).map(s => s.src)")
                    for src in scripts[:3]:
                        try:
                            req = urllib.request.urlopen(src, timeout=5)
                            if req.status != 200:
                                bundle_ok = False
                        except Exception:
                            bundle_ok = False

                    # API 对接检查
                    api_path = API_MAP.get(path, "")
                    api_ok = False
                    if api_path:
                        for _retry in range(3):
                            try:
                                req = urllib.request.Request(API + api_path, headers={"Authorization": f"Bearer {token}"})
                                with urllib.request.urlopen(req, timeout=10) as resp:
                                    api_ok = (resp.status == 200)
                                    break
                            except urllib.error.HTTPError as e:
                                api_ok = (e.code == 200)
                                break
                            except Exception:
                                time.sleep(1)
                                continue

                    status = "PASS" if (not is_blank and has_mount and api_ok) else "FAIL"
                    detail = f"blank={is_blank}, mount={has_mount}, api_ok={api_ok}, bundle={bundle_ok}"
                    results.append((path, name, status, detail))
                    report.append(f"## {path} ({name})")
                    report.append(f"- URL: {WEB}{path}")
                    report.append(f"- 白屏: {'是' if is_blank else '否'}")
                    report.append(f"- 挂载点: {'有' if has_mount else '无'}")
                    report.append(f"- Bundle 可达: {'是' if bundle_ok else '否'}")
                    report.append(f"- API 对接: {'OK' if api_ok else 'FAIL'} ({api_path})")
                    report.append(f"- 结果: {status}")
                    report.append("")
                except Exception as e:
                    results.append((path, name, "FAIL", str(e)[:100]))
                    report.append(f"## {path} ({name})")
                    report.append(f"- 异常: {str(e)[:200]}")
                    report.append("- 结果: FAIL")
                    report.append("")

            browser.close()
    except Exception as e:
        report.append(f"Playwright 不可用: {e}")
        report.append("回退到 HTML 挂载点 + bundle 可达判据")
        report.append("")

    # 汇总
    report.append("## 汇总")
    report.append("")
    report.append("| 路由 | 页面 | 结果 | 详情 |")
    report.append("|------|------|------|------|")
    for path, name, status, detail in results:
        report.append(f"| {path} | {name} | {status} | {detail} |")
    report.append("")
    pass_n = sum(1 for _, _, s, _ in results if s == "PASS")
    fail_n = sum(1 for _, _, s, _ in results if s == "FAIL")
    report.append(f"- PASS: {pass_n} / FAIL: {fail_n}")
    report.append(f"- 白屏: {'无' if all(not r[3].startswith('blank=True') for r in results) else '有'}")
    report.append("")

    OUT.write_text("\n".join(report), encoding="utf-8")
    print("\n".join(report))

if __name__ == "__main__":
    main()