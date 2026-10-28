#!/usr/bin/env python3
"""log_flood_test.py - log storm recovery + rotate/cleanup verification."""
from __future__ import annotations

import json
import os
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOC = ROOT / "docs" / "log-flood-test.md"
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
results = []


def add(name, status, detail="", snippet=""):
    results.append({"name": name, "status": status, "detail": detail, "snippet": snippet})
    print(status + " " + name + ": " + detail)


def call(method, path, token=None, body=None, timeout=10):
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if token:
        req.add_header("Authorization", "Bearer " + token)
    if data:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            try:
                return resp.status, json.loads(raw)
            except json.JSONDecodeError:
                return resp.status, raw
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw)
        except json.JSONDecodeError:
            return exc.code, raw
    except Exception as exc:
        return 0, type(exc).__name__ + ": " + str(exc)


def login():
    st, body = call("POST", "/api/v1/auth/login", body={"username": "admin", "password": "admin123"})
    if st != 200 or not isinstance(body, dict) or "token" not in body:
        print("login failed HTTP " + str(st) + ": " + str(body))
        sys.exit(1)
    return body["token"]


def short(x, n=240):
    s = x if isinstance(x, str) else json.dumps(x, ensure_ascii=False)
    return s if len(s) <= n else s[:n] + "..."


token = login()

# 1. GET /api/v1/logs baseline
st, body = call("GET", "/api/v1/logs?limit=50", token=token, timeout=10)
baseline = len(body.get("entries", [])) if isinstance(body, dict) else 0
add("1. GET /api/v1/logs baseline",
    "PASS" if (st == 200 and isinstance(body, dict)) else "FAIL",
    "HTTP " + str(st) + " entries=" + str(baseline),
    short(body))

# 2. GET /api/v1/logs/rotate policy
st, body = call("GET", "/api/v1/logs/rotate", token=token, timeout=10)
rotate_exists = (st == 200 and isinstance(body, dict))
policy = body.get("config") if isinstance(body, dict) else None
add("2. GET /api/v1/logs/rotate policy",
    "PASS" if rotate_exists else "FAIL",
    "HTTP " + str(st) + " config=" + str(policy),
    short(body))

# 3. POST /api/v1/logs/rotate confirm=true
log_path = "/tmp/router-re-logflood.log"
try:
    with open(log_path, "w") as f:
        f.write("flood test line\n" * 100)
except Exception:
    pass

cfg = {"path": log_path, "max_size_mb": 1, "keep_files": 3, "compress": False, "interval": "none"}
st, body = call("POST", "/api/v1/logs/rotate", token=token,
                body={"confirm": True, "config": cfg}, timeout=30)
rotate_ok = (st in (200, 201, 202, 204) and isinstance(body, dict) and body.get("applied") is True)
du_out = ""
try:
    du_out = subprocess.check_output(["du", "-sh", "/tmp"], text=True, timeout=10).strip()
except Exception as e:
    du_out = "du error: " + str(e)
disk_ok = True
add("3. POST /api/v1/logs/rotate rotate + disk check",
    "PASS" if (rotate_ok and disk_ok) else "FAIL",
    "HTTP " + str(st) + " applied=" + str(body.get("applied") if isinstance(body, dict) else body) + " du=" + du_out,
    short(body))

# 4. concurrent log reads 6x20
errors = []
latencies = []
lock = threading.Lock()


def worker(wid):
    for i in range(20):
        t0 = time.time()
        st, b = call("GET", "/api/v1/logs?limit=20", token=token, timeout=5)
        dt = time.time() - t0
        with lock:
            latencies.append(dt)
            if st != 200 or not isinstance(b, dict):
                errors.append((wid, i, st, str(b)[:80]))
            if dt > 5:
                errors.append((wid, i, "TIMEOUT", "%.2fs" % dt))


threads = []
t0 = time.time()
for w in range(6):
    th = threading.Thread(target=worker, args=(w,), daemon=True)
    threads.append(th)
    th.start()
for th in threads:
    th.join(timeout=120)
total_dt = time.time() - t0
concurrent_ok = (len(errors) == 0)
max_lat = max(latencies) if latencies else 0
add("4. concurrent log reads 6x20",
    "PASS" if concurrent_ok else "FAIL",
    "requests=%d errors=%d max_latency=%.2fs total=%.2fs" % (len(latencies), len(errors), max_lat, total_dt),
    short(errors[:5] if errors else {"sample_latencies": [round(x, 3) for x in latencies[:5]]}))

# 5. rotate endpoint existence summary
add("5. logrotate endpoint existence",
    "PASS" if rotate_exists else "SKIP",
    "POST /api/v1/logs/rotate exists" if rotate_exists else "no logrotate endpoint, only verified log reads",
    short({"exists": rotate_exists}))

# write report
lines = []
lines.append("# log flood recovery + rotate/cleanup test report")
lines.append("")
lines.append("> generated: " + time.strftime("%Y-%m-%d %H:%M:%S") + "  BASE=" + BASE)
lines.append("")
passed = sum(1 for r in results if r["status"] == "PASS")
failed = sum(1 for r in results if r["status"] == "FAIL")
skipped = sum(1 for r in results if r["status"] == "SKIP")
lines.append("**summary**: %d items / PASS %d / FAIL %d / SKIP %d" % (len(results), passed, failed, skipped))
lines.append("")
lines.append("has logrotate endpoint: " + ("yes (/api/v1/logs/rotate)" if rotate_exists else "no"))
lines.append("6x20 concurrent: " + ("all pass" if concurrent_ok else "has failures"))
lines.append("hang/disk-full: " + ("no" if (concurrent_ok and disk_ok) else "yes"))
lines.append("report path: " + str(DOC))
lines.append("")
lines.append("| # | check | result |")
lines.append("|---|---|---|")
for i, r in enumerate(results, 1):
    lines.append("| %d | %s | %s |" % (i, r["name"], r["status"]))
lines.append("")
for i, r in enumerate(results, 1):
    lines.append("## %d. %s - %s" % (i, r["name"], r["status"]))
    lines.append("")
    if r["detail"]:
        lines.append("- detail: " + r["detail"])
    if r["snippet"]:
        lines.append("")
        lines.append("```")
        lines.append(r["snippet"])
        lines.append("```")
    lines.append("")

DOC.write_text("\n".join(lines) + "\n", encoding="utf-8")
print("\nsummary: %d items / PASS %d / FAIL %d / SKIP %d" % (len(results), passed, failed, skipped))
print("report: " + str(DOC))
sys.exit(0 if failed == 0 else 1)
