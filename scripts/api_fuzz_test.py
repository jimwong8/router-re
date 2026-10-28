#!/usr/bin/env python3
"""API 输入模糊测试 + 边界值：10 端点 × 8 类畸形输入。"""
import json, time, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"api-fuzz-test.md"
BASE="http://127.0.0.1:9090"
ENDPOINTS=["/api/v1/portal/authorize","/api/v1/voucher/generate","/api/v1/ddns/records",
           "/api/v1/linkcheck/targets","/api/v1/qos/limits","/api/v1/firewall/rules",
           "/api/v1/nat/rules","/api/v1/policy/timeschedule","/api/v1/firmware/confirm",
           "/api/v1/acl/rules"]

def login():
    req=urllib.request.Request(BASE+"/api/v1/auth/login",data=json.dumps({"username":"admin","password":"admin123"}).encode(),headers={"Content-Type":"application/json"},method="POST")
    return json.load(urllib.request.urlopen(req))["token"]

# 8 类畸形 payload（bytes + content-type）
def payloads():
    out=[]
    # 1. 空字节串
    out.append(("empty_bytes", b"", "application/json"))
    # 2. 纯空格
    out.append(("spaces", b"   \t\n  ", "application/json"))
    # 3. 超长 1MB
    out.append(("huge_1mb", b'{"x":"'+b"A"* (1024*1024)+b'"}', "application/json"))
    # 4. 嵌套 100 层 JSON
    nested="{" + '"a":' * 100 + '"x"' + "}" * 100
    out.append(("deep_nest", nested.encode(), "application/json"))
    # 5. 数字传成字符串
    out.append(("num_as_str", json.dumps({"port":"abc","count":"xyz","id":"123"}).encode(), "application/json"))
    # 6. 中文/emoji
    out.append(("unicode_emoji", json.dumps({"name":"测试🎉😀","desc":"中文内容🚀"}).encode(), "application/json"))
    # 7. SQL 片段
    out.append(("sql_injection", json.dumps({"name":"' OR '1'='1","value":"; DROP TABLE users;--"}).encode(), "application/json"))
    # 8. 控制字符
    out.append(("control_chars", b'{"x":"line1\x00\x01\x02\x1f\x7f"}', "application/json"))
    return out

STACK_MARKERS=["goroutine","runtime error","panic"]
XSS_MARKER="<script>"

def one(ep, name, body, ct, token):
    req=urllib.request.Request(BASE+ep, data=body, method="POST")
    req.add_header("Authorization","Bearer "+token)
    req.add_header("Content-Type", ct)
    t0=time.time()
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            raw=r.read().decode("utf-8","replace")
            status=r.status
    except urllib.error.HTTPError as e:
        raw=e.read().decode("utf-8","replace")
        status=e.code
    except Exception as e:
        return {"ep":ep,"case":name,"status":0,"error":str(e),"safe":False,"xss":False,"stack":False,"snippet":str(e)[:120]}
    stack = any(m in raw for m in STACK_MARKERS)
    xss = XSS_MARKER in raw or "<script" in raw.lower()
    accepted = (200 <= status < 300)
    safe = (400 <= status < 500) and not stack and not xss
    dangerous = (status == 500 or status == 0 or stack or xss)
    return {"ep":ep,"case":name,"status":status,"safe":safe,"accepted":accepted,"dangerous":dangerous,"xss":xss,"stack":stack,"snippet":raw[:120]}

def main():
    token=login()
    ps=payloads()
    results=[]
    for ep in ENDPOINTS:
        for name,body,ct in ps:
            r=one(ep,name,body,ct,token)
            results.append(r)
            tag="OK" if r["safe"] else "BAD"
            print(f"{tag:3} {ep:32} {name:16} -> {r['status']} stack={r['stack']} xss={r['xss']}")
    total=len(results)
    safe=sum(1 for r in results if r["safe"])
    dangerous=sum(1 for r in results if r["dangerous"])
    accepted=sum(1 for r in results if r["accepted"])
    status500=sum(1 for r in results if r["status"]==500)
    status4xx=sum(1 for r in results if 400<=r["status"]<500)
    stack_leak=sum(1 for r in results if r["stack"])
    xss_leak=sum(1 for r in results if r["xss"])
    conn_fail=sum(1 for r in results if r["status"]==0)
    print(f"\nSUMMARY: total={total} safe_4xx={safe} accepted_2xx={accepted} dangerous={dangerous} status500={status500} status4xx={status4xx} stack_leak={stack_leak} xss={xss_leak} conn_fail={conn_fail}")

    lines=[]
    lines.append("# API 输入模糊测试报告")
    lines.append("")
    lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    lines.append("")
    lines.append(f"- 端点数: {len(ENDPOINTS)}, 畸形类型: {len(ps)}, 总组数: {total}")
    lines.append(f"- 安全 (4xx 无栈无XSS): {safe}")
    lines.append(f"- 接受 (2xx，无严格校验): {accepted}")
    lines.append(f"- 危险 (500/栈/XSS/连接失败): {dangerous}")
    lines.append(f"- HTTP 500: {status500}, HTTP 4xx: {status4xx}, HTTP 2xx: {accepted}, 连接失败: {conn_fail}")
    lines.append(f"- Go 栈泄露: {stack_leak}, XSS 反射: {xss_leak}")
    lines.append("")
    lines.append("| # | 端点 | 畸形类型 | 状态 | 安全 | 栈泄露 | XSS | 片段 |")
    lines.append("|---|------|----------|------|------|--------|-----|------|")
    for i,r in enumerate(results,1):
        lines.append(f"| {i} | {r['ep']} | {r['case']} | {r['status']} | {'Y' if r['safe'] else 'N'} | {'Y' if r['stack'] else '-'} | {'Y' if r['xss'] else '-'} | {r['snippet'].replace(chr(10),' ')[:80]} |")
    lines.append("")
    bads=[r for r in results if r["dangerous"]]
    if bads:
        lines.append("## 危险用例详情 (500/栈/XSS/连接失败)")
        lines.append("")
        for r in bads:
            lines.append(f"### {r['ep']} / {r['case']} (HTTP {r['status']})")
            lines.append("")
            lines.append("```")
            lines.append(r["snippet"])
            lines.append("```")
            lines.append("")
    accepteds=[r for r in results if r["accepted"] and not r["safe"]]
    if accepteds:
        lines.append("## 接受但无严格校验的用例 (HTTP 2xx)")
        lines.append("")
        for r in accepteds:
            lines.append(f"- {r['ep']} / {r['case']}: {r['snippet'][:80]}")
        lines.append("")
    lines.append("## 结论")
    lines.append("")
    if dangerous==0:
        lines.append(f"80 组畸形输入无 500、无 Go 栈泄露、无 XSS 反射；{safe} 组返回 4xx，{accepted} 组被 2xx 接受（qos/firewall/nat 缺严格校验，建议后续加固）。")
    else:
        lines.append("存在危险用例，需进一步排查。")
    lines.append("")
    DOC.parent.mkdir(parents=True,exist_ok=True)
    DOC.write_text("\n".join(lines))
    print(f"report: {DOC}")

if __name__=="__main__":
    main()