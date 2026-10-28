#!/usr/bin/env python3
"""conntrack 字节统计（acct）功能端到端验证。"""
import json, time, urllib.request, urllib.error, os
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"acct-verify-test.md"
BASE="http://127.0.0.1:9090"
ACCT_PATH="/proc/sys/net/netfilter/nf_conntrack_acct"
CONNTRACK_PATH="/proc/net/nf_conntrack"

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

# 1. GET acct 配置
st,b,_=call("GET","/api/v1/conntrack/acct",token=token)
if st==200 and isinstance(b,dict):
    on=b.get("on"); persisted=b.get("persisted"); path=b.get("path")
    ok = (on is True)
    step("1 GET /api/v1/conntrack/acct", "PASS" if ok else "FAIL",
         f"HTTP {st} on={on} persisted={persisted} path={path}", short(b))
else:
    step("1 GET /api/v1/conntrack/acct", "FAIL", f"HTTP {st}", short(b))

# 2. 系统真实 sysctl
sysctl_val=None
try:
    sysctl_val=open(ACCT_PATH).read().strip()
except Exception as e:
    sysctl_val=f"ERR:{e}"
api_on = b.get("on") if isinstance(b,dict) else None
consistent = (sysctl_val=="1") == (api_on is True)
if sysctl_val.startswith("ERR"):
    step("2 系统 sysctl nf_conntrack_acct", "SKIP", f"无法读取 {ACCT_PATH}: {sysctl_val}", sysctl_val)
else:
    step("2 系统 sysctl vs 接口", "PASS" if consistent else "FAIL",
         f"sysctl={sysctl_val} api_on={api_on} consistent={consistent}", f"sysctl={sysctl_val}")

# 3. GET conntrack 字节统计结构
st2,b2,_=call("GET","/api/v1/conntrack?limit=1000",token=token)
if st2==200 and isinstance(b2,dict) and b2.get("available") is True:
    entries=b2.get("entries",[])
    has_bytes = all(isinstance(e.get("bytes"),int) or isinstance(e.get("bytes"),(int,)) for e in entries) if entries else True
    # bytes field may come as int from JSON (Python int). check type name
    types={type(e.get("bytes")).__name__ for e in entries} if entries else set()
    some_gt0 = any((e.get("bytes") or 0)>0 for e in entries)
    step("3 GET /api/v1/conntrack 字节字段", "PASS" if (len(entries)>=0 and (not entries or types<= {"int"})) else "FAIL",
         f"HTTP {st2} count={len(entries)} types={types} some_bytes_gt0={some_gt0}", short(b2,300))
else:
    reason=b2.get("reason") if isinstance(b2,dict) else b2
    step("3 GET /api/v1/conntrack", "SKIP" if st2==200 else "FAIL", f"HTTP {st2} available={b2.get('available') if isinstance(b2,dict) else b2} reason={reason}", short(b2,300))

# 4. 系统 vs 接口字节对比
# 读 /proc/net/nf_conntrack，按第一组 bytes= 求和；对比 API entries 求和（API <= 系统，因 limit）
system_bytes=0; system_count=0; read_ok=False; read_err=""
# try direct read first, then sudo
for attempt in ("direct","sudo"):
    try:
        if attempt=="direct":
            fh=open(CONNTRACK_PATH)
        else:
            import subprocess
            r=subprocess.run(["sudo","-n","cat",CONNTRACK_PATH],capture_output=True,text=True,timeout=10)
            if r.returncode!=0:
                read_err=r.stderr.strip() or "sudo failed"
                continue
            fh=None
            content=r.stdout
            read_ok=True
            # process content
            for line in content.splitlines():
                line=line.strip()
                if not line or line.startswith("#"): continue
                m=None
                for tok in line.split():
                    if tok.startswith("bytes="):
                        m=tok.split("=",1)[1]; break
                if m is not None:
                    try: system_bytes+=int(m)
                    except: pass
                system_count+=1
            break
        if fh is not None:
            with fh:
                for line in fh:
                    line=line.strip()
                    if not line or line.startswith("#"): continue
                    m=None
                    for tok in line.split():
                        if tok.startswith("bytes="):
                            m=tok.split("=",1)[1]; break
                    if m is not None:
                        try: system_bytes+=int(m)
                        except: pass
                    system_count+=1
            read_ok=True
            break
    except Exception as e:
        read_err=str(e)
        continue
api_bytes=sum(e.get("bytes",0) or 0 for e in (b2.get("entries",[]) if isinstance(b2,dict) else []))
api_count=len(b2.get("entries",[]) if isinstance(b2,dict) else [])
if system_count<0:
    step("4 系统 vs 接口字节对比", "SKIP", f"无法读取 {CONNTRACK_PATH}", "N/A")
elif not (b2.get("available") if isinstance(b2,dict) else False):
    step("4 系统 vs 接口字节对比", "SKIP", "接口不可用，跳过对比", "N/A")
else:
    # API 受 limit 截断，system_bytes >= api_bytes 应成立
    consistent = system_bytes >= api_bytes
    step("4 系统 vs 接口字节对比", "PASS" if consistent else "FAIL",
         f"system_count={system_count} api_count={api_count} system_bytes={system_bytes} api_bytes={api_bytes} consistent(system>=api)={consistent}",
         f"system_bytes={system_bytes} api_bytes={api_bytes}")

# 汇总
lines=[]
lines.append("# conntrack 字节统计（acct）端到端验证报告")
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