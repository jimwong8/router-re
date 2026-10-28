#!/usr/bin/env python3
"""审计日志端到端：操作留痕可读。"""
import json, time, urllib.request, urllib.error
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"audit-e2e-test.md"
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

# 0. 捕获基线
st_fw,b_fw,_=call("GET","/api/v1/firewall/rules",token=token)
baseline_fw_rules=list(b_fw.get("rules",[])) if isinstance(b_fw,dict) else []
st_ddns,b_ddns,_=call("GET","/api/v1/ddns/config",token=token)
baseline_ddns=b_ddns.get("config") if isinstance(b_ddns,dict) else None
st_qos,b_qos,_=call("GET","/api/v1/qos/limits",token=token)
baseline_qos=list(b_qos.get("limits") or []) if isinstance(b_qos,dict) else []

# baseline audit
st_a,b_a,_=call("GET","/api/v1/audit/events?limit=100000",token=token)
baseline_events=b_a.get("events",[]) if isinstance(b_a,dict) else []
baseline_count=len(baseline_events)
baseline_ts = baseline_events[0]["ts"] if baseline_events else 0
step("0 捕获基线 (fw/ddns/qos + audit)", "PASS" if st_a==200 else "FAIL",
     f"fw_rules={len(baseline_fw_rules)} qos={len(baseline_qos)} audit_count={baseline_count} latest_ts={baseline_ts}",
     short({"fw":baseline_fw_rules,"ddns":baseline_ddns,"qos":baseline_qos,"audit_count":baseline_count},300))

# 1. 三个写操作
# 1a ddns config
st1,b1,_=call("POST","/api/v1/ddns/config",token=token,body={"provider":"none","domain":"audit-test.example.com","subdomain":"t","token":"12345678","enabled":False})
step("1a POST ddns/config", "PASS" if st1==200 else "FAIL", f"HTTP {st1}", short(b1))

# 1b qos limits (empty, safe)
st2,b2,_=call("POST","/api/v1/qos/limits",token=token,body={"limits":[]})
step("1b POST qos/limits", "PASS" if st2==200 else "FAIL", f"HTTP {st2}", short(b2))

# 1c firewall rules (append marker, then restore)
marker_rule={"Chain":"forward","Src":"10.99.99.99","Dst":"","Proto":"tcp","DPort":"9999","Action":"accept"}
st3,b3,_=call("POST","/api/v1/firewall/rules",token=token,body={"rules":baseline_fw_rules+[marker_rule]})
step("1c POST firewall/rules", "PASS" if st3==200 else "FAIL", f"HTTP {st3}", short(b3))

# 2. 恢复基线 (firewall)
st_r,b_r,_=call("POST","/api/v1/firewall/rules",token=token,body={"rules":baseline_fw_rules})
step("1d 恢复 firewall 基线", "PASS" if st_r==200 else "FAIL", f"HTTP {st_r}", short(b_r))

# 3. GET audit → 查新增记录
st_a2,b_a2,_=call("GET","/api/v1/audit/events?limit=100000",token=token)
events=b_a2.get("events",[]) if isinstance(b_a2,dict) else []
if len(events) > baseline_count:
    new_events=events[:len(events)-baseline_count]
else:
    new_events=[e for e in events if e.get("ts",0) > baseline_ts]
actions={e.get("action") for e in new_events}
expected={"ddns.config.set","qos.limits.set","firewall.rules.set"}
present = expected.issubset(actions)
step("2 GET audit 新增记录", "PASS" if (st_a2==200 and present) else "FAIL",
     f"HTTP {st_a2} new={len(new_events)} actions={sorted(actions)} expected_subset={present}",
     short(new_events,500))

# 4. actor & ts 非空
ok_actor_ts = all(e.get("actor")=="admin" and e.get("ts",0)>0 for e in new_events if e.get("action") in expected)
step("3 审计记录含 actor=admin 与 ts", "PASS" if ok_actor_ts else "FAIL",
     f"all expected events have actor=admin & ts>0: {ok_actor_ts}",
     short([{"action":e.get("action"),"actor":e.get("actor"),"ts":e.get("ts"),"target":e.get("target")} for e in new_events if e.get("action") in expected],400))

# 汇总
lines=[]
lines.append("# 审计日志端到端测试报告")
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