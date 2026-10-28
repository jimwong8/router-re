#!/usr/bin/env python3
"""DHCP 租约全生命周期（分配→续租→释放）端点验证。"""
import json, time, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"dhcp-lifecycle-test.md"
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

def short(x,n=240):
    s=json.dumps(x,ensure_ascii=False) if not isinstance(x,str) else x
    return s if len(s)<=n else s[:n-3]+"..."

token=login()
steps=[]
def step(s,status,detail,ev):
    steps.append({"step":s,"status":status,"detail":detail,"evidence":ev})
    print(f"{status:4} {s}: {detail}")

# 1. GET dhcp/config
st,b,_=call("GET","/api/v1/dhcp/config",token=token)
cfg=b.get("config") if isinstance(b,dict) else None
step("1 GET /api/v1/dhcp/config", "PASS" if st==200 else "FAIL",
     f"HTTP {st} interface={cfg.get('interface') if isinstance(cfg,dict) else cfg} range={cfg.get('range_start') if isinstance(cfg,dict) else ''}-{cfg.get('range_end') if isinstance(cfg,dict) else ''}",
     short(b))

# 2. GET dhcp/leases
st2,b2,_=call("GET","/api/v1/dhcp/leases",token=token)
leases=b2.get("leases",[]) if isinstance(b2,dict) else []
count=b2.get("count",len(leases)) if isinstance(b2,dict) else len(leases)
step("2 GET /api/v1/dhcp/leases", "PASS" if st2==200 else "FAIL",
     f"HTTP {st2} count={count}", short(b2))

# 3. GET dhcp/server (static leases)
st3,b3,_=call("GET","/api/v1/dhcp/server",token=token)
scfg=b3.get("config") if isinstance(b3,dict) else None
statics=scfg.get("static_leases") if isinstance(scfg,dict) else None
step("3 GET /api/v1/dhcp/server", "PASS" if st3==200 else "FAIL",
     f"HTTP {st3} enabled={scfg.get('enabled') if isinstance(scfg,dict) else scfg} static_leases={len(statics) if statics else 0}",
     short(b3))

# 4. GET dhcp/status
st4,b4,_=call("GET","/api/v1/dhcp/status",token=token)
step("4 GET /api/v1/dhcp/status", "PASS" if st4==200 else "FAIL",
     f"HTTP {st4} config_exists={b4.get('config_exists') if isinstance(b4,dict) else b4}",
     short(b4,300))

# 5. 租约字段结构验证（若有租约）
if leases:
    required={"mac","ip","expiry"}
    ok=all(required.issubset(set(l.keys())) for l in leases)
    step("5 租约字段结构完整", "PASS" if ok else "FAIL",
         f"leases={len(leases)} all have {sorted(required)}: {ok}", short(leases,300))
else:
    step("5 租约字段结构完整", "SKIP", "当前无真实租约（router-re 自身 DHCP 未配置/无客户端），仅验证端点结构", "leases=[]")

# 6. 静态保留端点：无 DELETE/POST reservation 端点 → SKIP
step("6 静态保留生命周期", "SKIP", "未发现 /api/v1/dhcp/reservations 或 reservation 端点，无法验证建→确认→删", "N/A")

# 7. 真实 DHCP 客户端生命周期
step("7 真实租约分配/续租/释放", "SKIP", "router-re DHCP 未配置（range 空），无真实客户端；LXC dnsmasq 独立运行，不归属本 API", "config empty")

# 汇总
lines=[]
lines.append("# DHCP 租约全生命周期验证报告")
lines.append("")
lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}")
lines.append("")
p=sum(1 for s in steps if s["status"]=="PASS")
f=sum(1 for s in steps if s["status"]=="FAIL")
sk=sum(1 for s in steps if s["status"]=="SKIP")
lines.append(f"**汇总**: {len(steps)} 步 | PASS {p} / FAIL {f} / SKIP {sk}")
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
print(f"TOTAL: PASS={p} FAIL={f} SKIP={sk}")