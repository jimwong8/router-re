#!/usr/bin/env python3
"""nftables 生成差异测试：改配置 → 对比 nft 预览 diff。"""
import json, time, urllib.request, urllib.error, difflib
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"nft-diff-test.md"
BASE="http://127.0.0.1:9090"
MARKER_IP="10.99.99.99"

def call(method,path,token=None,body=None):
    url=BASE+path
    data=json.dumps(body).encode() if body is not None else None
    req=urllib.request.Request(url,data=data,method=method)
    if token: req.add_header("Authorization","Bearer "+token)
    if data: req.add_header("Content-Type","application/json")
    try:
        with urllib.request.urlopen(req,timeout=30) as r:
            raw=r.read().decode("utf-8","replace")
            try: return r.status,json.loads(raw),raw
            except: return r.status,raw,raw
    except urllib.error.HTTPError as e:
        raw=e.read().decode("utf-8","replace")
        try: return e.code,json.loads(raw),raw
        except: return e.code,raw,raw
    except Exception as e:
        return 0,str(e),str(e)

def login():
    st,body,_=call("POST","/api/v1/auth/login",body={"username":"admin","password":"admin123"})
    return body["token"] if isinstance(body,dict) else None

def short(x,n=200):
    s=json.dumps(x,ensure_ascii=False) if not isinstance(x,str) else x
    return s if len(s)<=n else s[:n-3]+"..."

token=login()
steps=[]
def step(s,status,detail,ev):
    steps.append({"step":s,"status":status,"detail":detail,"evidence":ev})
    print(f"{status:4} {s}: {detail}")

# 1. 基线
st,b,_=call("GET","/api/v1/firewall/rules",token=token)
baseline_rules=list(b.get("rules",[])) if isinstance(b,dict) else []
baseline_nft=b.get("nft","") if isinstance(b,dict) else ""
step("1 GET 基线 nft 预览", "PASS" if st==200 else "FAIL", f"HTTP {st} rules={len(baseline_rules)} nft_len={len(baseline_nft)}", short(baseline_nft,300))

# 2. 追加 marker 规则
marker_rule={"Chain":"forward","Src":MARKER_IP,"Dst":"","Proto":"tcp","DPort":"9999","Action":"accept"}
new_rules=baseline_rules+[marker_rule]
st2,b2,_=call("POST","/api/v1/firewall/rules",token=token,body={"rules":new_rules})
new_nft=b2.get("nft","") if isinstance(b2,dict) else ""
marker_present = MARKER_IP in new_nft
step("2 POST 追加 marker 规则", "PASS" if (st2==200 and marker_present) else "FAIL", f"HTTP {st2} marker_present={marker_present}", short(new_nft,300))

# 3. diff：marker 行出现 + 基线旧规则未误删
diff_lines=list(difflib.unified_diff(baseline_nft.splitlines(), new_nft.splitlines(), lineterm="", fromfile="baseline", tofile="with_marker"))
diff_text="\n".join(diff_lines)
# 旧规则：基线每条规则的渲染行应在 new_nft 中仍存在
old_retained=True
for r in baseline_rules:
    # reconstruct a representative substring
    pass
# check baseline nft lines (non-header) all present in new_nft
baseline_content_lines=[l for l in baseline_nft.splitlines() if l.strip() and not l.strip().startswith("table") and not l.strip().startswith("}") and "chain" not in l]
retained = all(l in new_nft for l in baseline_content_lines) if baseline_content_lines else True
diff_ok = marker_present and retained
step("3 diff 可见新规则且旧规则保留", "PASS" if diff_ok else "FAIL", f"marker_present={marker_present} old_retained={retained}", diff_text[:600])

# 4. 删除 marker → 恢复基线
st3,b3,_=call("POST","/api/v1/firewall/rules",token=token,body={"rules":baseline_rules})
restored_nft=b3.get("nft","") if isinstance(b3,dict) else ""
restored = (st3==200) and (restored_nft==baseline_nft)
step("4 删除 marker 恢复基线", "PASS" if restored else "FAIL", f"HTTP {st3} restored_exact={restored}", short(restored_nft,300))

# 汇总
lines=[]
lines.append("# nftables 生成差异测试报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}")
lines.append("")
p=sum(1 for s in steps if s["status"]=="PASS")
f=sum(1 for s in steps if s["status"]=="FAIL")
lines.append(f"**汇总**: {len(steps)} 步 | PASS {p} / FAIL {f}")
lines.append("")
lines.append("| # | 步骤 | 结果 | 说明 |")
lines.append("|---|------|------|------|")
for i,s in enumerate(steps,1):
    lines.append(f"| {i} | {s['step']} | {s['status']} | {s['detail']} |")
lines.append("")
for i,s in enumerate(steps,1):
    lines.append(f"## {i}. {s['step']}")
    lines.append("")
    lines.append(f"- 结果: **{s['status']}** — {s['detail']}")
    lines.append("")
    lines.append("```")
    lines.append(s["evidence"])
    lines.append("```")
    lines.append("")
DOC.parent.mkdir(parents=True,exist_ok=True)
DOC.write_text("\n".join(lines))
print(f"\nreport: {DOC}")
print(f"TOTAL: PASS={p} FAIL={f}")