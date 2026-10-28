#!/usr/bin/env python3
"""持续并发压力测试：10 线程 × 60s 持续打 5 个读端点。"""
import json, os, subprocess, threading, time, urllib.request, statistics
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
DOC=ROOT/"docs"/"sustained-load-test.md"
BASE="http://127.0.0.1:9090"
ENDPOINTS=["/api/v1/terminals","/api/v1/online","/api/v1/system/info","/api/v1/health","/api/v1/system/services"]

def login():
    req=urllib.request.Request(BASE+"/api/v1/auth/login",data=json.dumps({"username":"admin","password":"admin123"}).encode(),headers={"Content-Type":"application/json"},method="POST")
    return json.load(urllib.request.urlopen(req))["token"]

def mgmtd_pid():
    out=subprocess.check_output(["pgrep","-x","mgmtd-cgo"],text=True).strip().split()
    # pick the one whose /proc has the listening socket? just first
    return int(out[0])

def rss_kb(pid):
    out=subprocess.check_output(["sudo","-n","ps","-o","rss=","-p",str(pid)],text=True).strip()
    return int(out)

def fd_count(pid):
    out=subprocess.check_output(["sudo","-n","bash","-c",f"ls /proc/{pid}/fd | wc -l"],text=True).strip()
    return int(out)

def main():
    token=login()
    pid=mgmtd_pid()
    rss0=rss_kb(pid); fd0=fd_count(pid)
    print(f"baseline pid={pid} rss={rss0}KB fd={fd0}")

    latencies=[]
    lock=threading.Lock()
    success=[0]; fail=[0]; errors={}
    stop=threading.Event()
    def worker():
        i=0
        while not stop.is_set():
            ep=ENDPOINTS[i % len(ENDPOINTS)]
            i+=1
            t0=time.time()
            try:
                req=urllib.request.Request(BASE+ep,headers={"Authorization":"Bearer "+token})
                with urllib.request.urlopen(req,timeout=10) as r:
                    r.read()
                    dt=(time.time()-t0)*1000
                    with lock:
                        latencies.append(dt)
                        success[0]+=1
            except Exception as e:
                dt=(time.time()-t0)*1000
                with lock:
                    latencies.append(dt)
                    fail[0]+=1
                    errors[type(e).__name__]=errors.get(type(e).__name__,0)+1
    threads=[threading.Thread(target=worker,daemon=True) for _ in range(10)]
    for th in threads: th.start()
    time.sleep(60)
    stop.set()
    for th in threads: th.join(timeout=10)
    time.sleep(1)  # let mgmtd settle
    rss1=rss_kb(pid); fd1=fd_count(pid)
    total=success[0]+fail[0]
    sr=success[0]/total*100 if total else 0
    l=sorted(latencies)
    def pct(p):
        if not l: return 0
        idx=int(len(l)*p/100)
        idx=min(idx,len(l)-1)
        return l[idx]
    p50=pct(50); p95=pct(95); p99=pct(99)
    rss_delta=rss1-rss0; fd_delta=fd1-fd0
    leak = rss_delta>50*1024 or fd_delta>100
    print(f"total={total} success={success[0]} fail={fail[0]} rate={sr:.2f}%")
    print(f"P50={p50:.1f}ms P95={p95:.1f}ms P99={p99:.1f}ms")
    print(f"RSS {rss0}->{rss1} KB delta={rss_delta}KB; FD {fd0}->{fd1} delta={fd_delta}")
    print(f"leak_suspect={leak} errors={errors}")

    lines=[]
    lines.append("# 持续并发压力测试报告")
    lines.append("")
    lines.append(f"> 生成: {time.strftime('%Y-%m-%d %H:%M:%S')}")
    lines.append("")
    lines.append(f"- 持续时长: 60s, 并发: 10 线程, 端点: {', '.join(ENDPOINTS)}")
    lines.append(f"- 总请求: {total}, 成功: {success[0]}, 失败: {fail[0]}, 成功率: {sr:.2f}%")
    lines.append(f"- 延迟: P50={p50:.1f}ms, P95={p95:.1f}ms, P99={p99:.1f}ms")
    lines.append(f"- mgmtd pid={pid}")
    lines.append(f"- RSS: {rss0} KB -> {rss1} KB (delta {rss_delta} KB)")
    lines.append(f"- FD: {fd0} -> {fd1} (delta {fd_delta})")
    lines.append(f"- 可疑泄漏: {'是' if leak else '否'} (阈值 RSS>50MB 或 FD>100)")
    lines.append(f"- 错误计数: {errors}")
    lines.append("")
    lines.append("## 结论")
    lines.append("")
    if fail[0]==0 and not leak:
        lines.append("持续 60s 负载下无 500/连接拒绝，无内存或句柄泄漏迹象。")
    else:
        lines.append("存在失败或可疑泄漏，需进一步排查。")
    lines.append("")
    DOC.parent.mkdir(parents=True,exist_ok=True)
    DOC.write_text("\n".join(lines))
    print(f"report: {DOC}")

if __name__=="__main__":
    main()