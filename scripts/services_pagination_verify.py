#!/usr/bin/env python3
"""Services 页分页验证：首屏只渲染 50 行服务。"""
import json, urllib.request
from playwright.sync_api import sync_playwright
API="http://127.0.0.1:9090"; WEB="http://127.0.0.1:4173"
req=urllib.request.Request(API+"/api/v1/auth/login",data=json.dumps({"username":"admin","password":"admin123"}).encode(),headers={"Content-Type":"application/json"},method="POST")
token=json.load(urllib.request.urlopen(req))["token"]
with sync_playwright() as p:
    browser=p.chromium.launch(headless=True)
    page=browser.new_context().new_page()
    page.goto(WEB)
    page.evaluate(f"localStorage.setItem('router_token', {json.dumps(token)})")
    page.goto(WEB+"/services")
    page.wait_for_load_state("networkidle", timeout=30000)
    page.wait_for_timeout(2000)
    tables=page.locator("table")
    svc_rows = tables.nth(1).locator("tbody tr").count()
    print("services table rows:", svc_rows)
    print("pagination:", page.locator(".el-pagination").inner_text().replace("\n"," "))
    assert svc_rows == 50, f"expected 50, got {svc_rows}"
    browser.close()
print("PASS: first screen renders 50 service rows")