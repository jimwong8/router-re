#!/usr/bin/env python3
"""断言 /api/v1/logs 的 grep 能搜到 -n 窗口之外的老日志。"""
import json, urllib.request
API="http://127.0.0.1:9090"
def call(method,path,token=None):
    url=API+path
    req=urllib.request.Request(url,method=method)
    if token: req.add_header("Authorization","Bearer "+token)
    try:
        with urllib.request.urlopen(req,timeout=30) as r: return r.status,json.loads(r.read())
    except urllib.error.HTTPError as e: return e.code,json.loads(e.read())
tok=call("POST","/api/v1/auth/login",None)[1] if False else None
# login
req=urllib.request.Request(API+"/api/v1/auth/login",data=json.dumps({"username":"admin","password":"admin123"}).encode(),headers={"Content-Type":"application/json"},method="POST")
tok=json.load(urllib.request.urlopen(req))["token"]
def get(path): 
    rq=urllib.request.Request(API+path,headers={"Authorization":"Bearer "+tok})
    with urllib.request.urlopen(rq,timeout=30) as r: return json.load(r)
base=get("/api/v1/logs?limit=5")
entries=base.get("entries",[])
assert len(entries)>=2, "need >=2 entries"
newest=entries[0]; older=entries[-1]
term=None
for w in older.get("message","").split():
    w=w.strip(".,;:!?()[]{}\"'")
    if len(w)>=4 and w not in newest.get("message",""):
        term=w; break
assert term, "no distinguishing term"
got=get(f"/api/v1/logs?limit=1&grep={term}")
g=got.get("entries",[])
assert len(g)>0, f"grep {term!r} returned 0 (beyond-window search failed)"
assert term in g[0].get("message",""), "returned entry lacks term"
assert not (g[0].get("message")==newest.get("message") and g[0].get("ts")==newest.get("ts")), "returned newest only"
print(f"PASS: grep {term!r} found beyond-window entry")