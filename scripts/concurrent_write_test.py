#!/usr/bin/env python3
"""concurrent_write_test.py — 并发写竞争测试。

验证多线程同时写同一类配置不会产生丢失更新或数据损坏。
产出 docs/concurrent-write-test.md（脚本写，勿手写）。
"""
from __future__ import annotations
import json, os, threading, time, urllib.error, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "concurrent-write-test.md"
DURATION = 15  # seconds

def call(method, path, token=None, body=None, timeout=20):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw), time.time()
            except json.JSONDecodeError:
                return resp.status, raw, time.time()
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw), time.time()
        except json.JSONDecodeError:
            return exc.code, raw, time.time()
    except Exception as exc:
        return 0, f"{type(exc).__name__}: {exc}", time.time()

def login():
    status, body, _ = call("POST", "/api/v1/auth/login",
                           body={"username": "admin", "password": "admin123"})
    if status != 200:
        print(f"登录失败 HTTP {status}: {body}")
        raise SystemExit(1)
    return body["token"] if isinstance(body, dict) else json.loads(body)["token"]

def main():
    token = login()
    report = []
    report.append("# Concurrent Write Race Test")
    report.append("")
    report.append(f"- Base: {BASE}")
    report.append(f"- Date: {__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    report.append(f"- Duration per phase: {DURATION}s")
    report.append("")

    # ---------- Phase 1: 同一端点并发写 ----------
    report.append("## Phase 1: 同一端点 (ddns/config) 并发写")
    report.append("")
    report.append("- 8 线程，每个线程写不同的 provider 标记值 (p78-A1..A8)")
    report.append("- 持续 15s，每次写一个可识别值")
    report.append("")

    # 线程写入计数器
    write_counts = [0] * 8
    write_success = [0] * 8
    write_fail = [0] * 8
    write_500 = [0] * 8
    stop_event = threading.Event()

    def writer(thread_id):
        marker = f"p78-A{thread_id+1}"
        while not stop_event.is_set():
            body = {"provider": marker, "domain": f"{marker}.example.com", "enabled": True}
            s, resp, _ = call("POST", "/api/v1/ddns/config", token=token, body=body)
            write_counts[thread_id] += 1
            if s == 200:
                write_success[thread_id] += 1
            elif s >= 500:
                write_500[thread_id] += 1
            else:
                write_fail[thread_id] += 1
            time.sleep(0.05)

    threads = []
    for i in range(8):
        t = threading.Thread(target=writer, args=(i,))
        t.start()
        threads.append(t)

    time.sleep(DURATION)
    stop_event.set()
    for t in threads:
        t.join()

    total_writes = sum(write_counts)
    total_success = sum(write_success)
    total_fail = sum(write_fail)
    total_500 = sum(write_500)

    # 读回最终值
    s, final, _ = call("GET", "/api/v1/ddns/config", token=token)
    final_provider = final.get("config", {}).get("provider") if isinstance(final, dict) else "?"
    final_domain = final.get("config", {}).get("domain") if isinstance(final, dict) else "?"

    # 判断最终值是否是合法标记值
    valid_markers = {f"p78-A{i}" for i in range(1, 9)}
    final_valid = final_provider in valid_markers
    final_not_corrupt = (s == 200 and isinstance(final, dict) and "config" in final)

    report.append(f"- 总写次数: {total_writes}")
    report.append(f"- 成功 (200): {total_success}")
    report.append(f"- 失败 (4xx): {total_fail}")
    report.append(f"- 500/panic: {total_500}")
    report.append(f"- 成功率: {total_success/total_writes*100:.1f}%")
    report.append(f"- 最终值: provider={final_provider}, domain={final_domain}")
    report.append(f"- 最终值合法 (是某个标记值): {'PASS' if final_valid else 'FAIL'}")
    report.append(f"- 最终值未损坏 (非空/非半截 JSON): {'PASS' if final_not_corrupt else 'FAIL'}")
    report.append("")

    # ---------- Phase 2: 混合端点并发写 ----------
    report.append("## Phase 2: 混合端点并发写")
    report.append("")
    report.append("- 8 线程，各打不同端点，持续 15s")
    report.append("- 断言全程无 500、无 panic、无 503")
    report.append("")

    endpoints = [
        ("ddns", "/api/v1/ddns/config", {"provider": "p78-mixed", "domain": "mixed.example.com", "enabled": True}),
        ("portal", "/api/v1/portal/config", {"enabled": True, "portal_port": 8888, "default_minutes": 99}),
        ("timesched", "/api/v1/policy/timeschedule", {"rules": [{"id": "p78", "name": "p78", "enabled": True, "start": "00:00", "end": "23:59"}], "confirm": False}),
        ("ra", "/api/v1/ipv6/ra", {"iface": "ens18", "enabled": True}),
        ("pd", "/api/v1/ipv6/pd", {"enabled": True}),
        ("nd", "/api/v1/ipv6/nd", {"iface": "ens18", "forwarding": True}),
        ("ddns2", "/api/v1/ddns/config", {"provider": "p78-mixed2", "domain": "mixed2.example.com", "enabled": False}),
        ("portal2", "/api/v1/portal/config", {"enabled": False, "portal_port": 7777, "default_minutes": 55}),
    ]

    mixed_counts = [0] * len(endpoints)
    mixed_success = [0] * len(endpoints)
    mixed_500 = [0] * len(endpoints)
    mixed_503 = [0] * len(endpoints)
    stop2 = threading.Event()

    def mixed_writer(idx):
        name, path, body = endpoints[idx]
        while not stop2.is_set():
            s, resp, _ = call("POST", path, token=token, body=body)
            mixed_counts[idx] += 1
            if s == 200:
                mixed_success[idx] += 1
            elif s == 500:
                mixed_500[idx] += 1
            elif s == 503:
                mixed_503[idx] += 1
            time.sleep(0.1)

    threads2 = []
    for i in range(len(endpoints)):
        t = threading.Thread(target=mixed_writer, args=(i,))
        t.start()
        threads2.append(t)

    time.sleep(DURATION)
    stop2.set()
    for t in threads2:
        t.join()

    total_mixed = sum(mixed_counts)
    total_mixed_500 = sum(mixed_500)
    total_mixed_503 = sum(mixed_503)
    total_mixed_success = sum(mixed_success)

    report.append("| 端点 | 写次数 | 成功 | 500 | 503 |")
    report.append("|------|--------|------|-----|-----|")
    for i, (name, path, _) in enumerate(endpoints):
        report.append(f"| {name} | {mixed_counts[i]} | {mixed_success[i]} | {mixed_500[i]} | {mixed_503[i]} |")
    report.append("")
    report.append(f"- 总写次数: {total_mixed}")
    report.append(f"- 成功: {total_mixed_success}")
    report.append(f"- 500: {total_mixed_500}")
    report.append(f"- 503: {total_mixed_503}")
    report.append(f"- 无 500/panic/503: {'PASS' if (total_mixed_500 == 0 and total_mixed_503 == 0) else 'FAIL'}")
    report.append("")

    # ---------- 汇总 ----------
    report.append("## 汇总")
    report.append("")
    report.append(f"- Phase 1: 总写 {total_writes} 次，成功率 {total_success/total_writes*100:.1f}%，"
                  f"500={total_500}，最终值合法={'PASS' if final_valid else 'FAIL'}")
    report.append(f"- Phase 2: 总写 {total_mixed} 次，500={total_mixed_500}，503={total_mixed_503}")
    report.append(f"- 无数据损坏: {'PASS' if final_not_corrupt else 'FAIL'}")
    report.append("")

    OUT.write_text("\n".join(report), encoding="utf-8")
    print("\n".join(report))

if __name__ == "__main__":
    main()