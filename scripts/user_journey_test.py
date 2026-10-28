#!/usr/bin/env python3
"""端到端用户旅程：3 条完整运维流程。"""
import json, time, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"user-journey-test.md"
BASE="http://127.0.0.1:9090"

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

def short(x,n=180):
    s=json.dumps(x,ensure_ascii=False) if not isinstance(x,str) else x
    return s if len(s)<=n else s[:n-3]+"..."

token=login()
steps=[]  # list of (journey, step, status, detail, evidence)
def step(j,s,status,detail,ev):
    steps.append({"journey":j,"step":s,"status":status,"detail":detail,"evidence":ev})
    print(f"[{j}] {status:4} {s}: {detail}")

# ========== 旅程1：端口转发 ==========
J="J1-portmap"
# 1. POST 建规则
st,body,ev=call("POST","/api/v1/portmap/rules",token=token,body={"rules":[{"id":99999,"name":"journey1","proto":"tcp","ext_port":8080,"int_ip":"192.168.1.10","int_port":80,"enabled":True}]})
ok = st in (200,201) and isinstance(body,dict)
step(J,"1 POST 建 portmap 规则", "PASS" if ok else "FAIL", f"HTTP {st}", short(body))
# 2. GET 列表确认存在
st2,body2,ev2=call("GET","/api/v1/portmap/rules",token=token)
exists = isinstance(body2,dict) and any(r.get("ext_port")==8080 for r in body2.get("rules",[]))
step(J,"2 GET 列表确认存在", "PASS" if exists else "FAIL", f"HTTP {st2} exists={exists}", short(body2))
# 3. 验证 nft 预览含该规则
nft = body.get("nft","") if isinstance(body,dict) else ""
nft_ok = "8080" in str(nft) or "192.168.1.10" in str(nft)
step(J,"3 nft 预览含规则", "PASS" if nft_ok else "FAIL", f"nft contains 8080/192.168.1.10 = {nft_ok}", short(nft))
# 4. 删除
st3,body3,ev3=call("DELETE","/api/v1/portmap/rules/99999",token=token)
deleted = isinstance(body3,dict) and body3.get("deleted") is True
step(J,"4 DELETE 规则", "PASS" if deleted else "FAIL", f"HTTP {st3} deleted={body3.get('deleted') if isinstance(body3,dict) else body3}", short(body3))
# 5. GET 确认消失
st4,body4,ev4=call("GET","/api/v1/portmap/rules",token=token)
gone = isinstance(body4,dict) and not any(r.get("ext_port")==8080 for r in body4.get("rules",[]))
step(J,"5 GET 确认消失", "PASS" if gone else "FAIL", f"HTTP {st4} gone={gone}", short(body4))

# ========== 旅程2：家长控制时段 ==========
J="J2-timesched"
rule={"id":"journey2","name":"child-bedtime","days":[0,1,2,3,4,5,6],"start_min":1260,"end_min":1439,"targets":["192.168.1.100"],"action":"block","enabled":True}
st,body,ev=call("POST","/api/v1/policy/timeschedule",token=token,body={"rules":[rule],"confirm":False})
ok = st==200 and isinstance(body,dict) and body.get("dry_run") is True
step(J,"1 POST 预览 (dry_run)", "PASS" if ok else "FAIL", f"HTTP {st} dry_run={body.get('dry_run') if isinstance(body,dict) else body}", short(body))
# nft 预览
nft = body.get("nft","") if isinstance(body,dict) else ""
nft_ok = "journey2" in str(nft)
step(J,"2 nft 预览生成", "PASS" if nft_ok else "FAIL", f"nft ok={nft_ok}", short(nft))
# apply + save
st2,body2,ev2=call("POST","/api/v1/policy/timeschedule",token=token,body={"rules":[rule],"confirm":True})
applied = st2==200 and isinstance(body2,dict) and body2.get("applied") is True
step(J,"3 POST apply+save", "PASS" if applied else "FAIL", f"HTTP {st2} applied={body2.get('applied') if isinstance(body2,dict) else body2}", short(body2))
# GET 确认
st3,body3,ev3=call("GET","/api/v1/policy/timeschedule",token=token)
present = isinstance(body3,dict) and any(r.get("id")=="journey2" or r.get("name")=="child-bedtime" for r in body3.get("rules",[]))
step(J,"4 GET 确认存在", "PASS" if present else "FAIL", f"HTTP {st3} present={present}", short(body3))
# 删除：timeschedule 无 DELETE 端点
step(J,"5 删除 (无 DELETE 端点)", "SKIP", "timeschedule 仅支持 POST 全量替换/禁用，无 DELETE", "N/A")
step(J,"6 确认消失 (无法)", "SKIP", "无 DELETE 端点，无法确认消失", "N/A")

# ========== 旅程3：访客网络 ==========
J="J3-voucher"
MAC="aa:bb:cc:dd:ee:ff"
# 1. 建 voucher 60min
st,body,ev=call("POST","/api/v1/voucher/generate",token=token,body={"count":1,"minutes":60,"code_len":8})
code=None
if isinstance(body,dict) and body.get("vouchers"):
    code=body["vouchers"][0].get("code")
ok = st==200 and code is not None
step(J,"1 生成 60min voucher", "PASS" if ok else "FAIL", f"HTTP {st} code={code}", short(body))
# 2. portal/authorize 用 MAC 认证
st2,body2,ev2=call("POST","/api/v1/portal/authorize",token=token,body={"mac":MAC,"ip":"192.168.1.200","minutes":60})
authed = st2==200 and isinstance(body2,dict) and body2.get("client") is not None
step(J,"2 portal/authorize MAC", "PASS" if authed else "FAIL", f"HTTP {st2}", short(body2))
# 3. GET portal/clients 确认 MAC 在线
st3,body3,ev3=call("GET","/api/v1/portal/clients",token=token)
clients=body3.get("clients",[]) if isinstance(body3,dict) else []
online = any(c.get("mac","").lower().replace(":","")==MAC.replace(":","").lower() or c.get("mac")==MAC for c in clients)
step(J,"3 GET portal/clients 确认在线", "PASS" if online else "FAIL", f"HTTP {st3} online={online} count={len(clients)}", short(body3))
# 4. revoke
st4,body4,ev4=call("POST","/api/v1/portal/revoke",token=token,body={"mac":MAC})
revoked = st4==200 and isinstance(body4,dict) and body4.get("revoked") is True
step(J,"4 revoke MAC", "PASS" if revoked else "FAIL", f"HTTP {st4} revoked={body4.get('revoked') if isinstance(body4,dict) else body4}", short(body4))
# 5. GET 确认下线
st5,body5,ev5=call("GET","/api/v1/portal/clients",token=token)
clients5=body5.get("clients",[]) if isinstance(body5,dict) else []
offline = not any(c.get("mac","").lower().replace(":","")==MAC.replace(":","").lower() or c.get("mac")==MAC for c in clients5)
step(J,"5 GET 确认下线", "PASS" if offline else "FAIL", f"HTTP {st5} offline={offline} count={len(clients5)}", short(body5))
# cleanup voucher
if code:
    call("DELETE",f"/api/v1/voucher?code={code}",token=token)

# ========== 汇总 ==========
journeys={}
for s in steps:
    j=s["journey"]
    journeys.setdefault(j,[]).append(s)
lines=[]
lines.append("# 用户旅程端到端测试报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}")
lines.append("")
for j,ss in journeys.items():
    p=sum(1 for s in ss if s["status"]=="PASS")
    f=sum(1 for s in ss if s["status"]=="FAIL")
    sk=sum(1 for s in ss if s["status"]=="SKIP")
    lines.append(f"- {j}: {p} PASS / {f} FAIL / {sk} SKIP  ({'全通' if f==0 and sk==0 else '部分'})")
lines.append("")
total_p=sum(1 for s in steps if s["status"]=="PASS")
total_f=sum(1 for s in steps if s["status"]=="FAIL")
total_s=sum(1 for s in steps if s["status"]=="SKIP")
lines.append(f"**汇总**: {len(steps)} 步 | PASS {total_p} / FAIL {total_f} / SKIP {total_s}")
lines.append("")
lines.append("| # | 旅程 | 步骤 | 结果 | 说明 |")
lines.append("|---|------|------|------|------|")
for i,s in enumerate(steps,1):
    lines.append(f"| {i} | {s['journey']} | {s['step']} | {s['status']} | {s['detail']} |")
lines.append("")
for i,s in enumerate(steps,1):
    lines.append(f"## {i}. [{s['journey']}] {s['step']}")
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
print(f"TOTAL: PASS={total_p} FAIL={total_f} SKIP={total_s}")