#!/usr/bin/env python3
"""config_persist_test.py — 验证配置重启后持久化。

写配置→重启 mgmtd→读回，断言标记值仍在。
产出 docs/config-persist-test.md（脚本写，勿手写）。
"""
from __future__ import annotations
import json, os, subprocess, time, urllib.error, urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASE = os.environ.get("BASE", "http://127.0.0.1:9090")
OUT = ROOT / "docs" / "config-persist-test.md"

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

def health_ok(timeout=30):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            with urllib.request.urlopen(BASE + "/api/v1/health", timeout=5) as r:
                if r.status == 200:
                    return True
        except Exception:
            pass
        time.sleep(1)
    return False

def restart_mgmtd():
    # 找 PID
    try:
        pid_out = subprocess.check_output(["pgrep", "-x", "mgmtd-cgo"], text=True).strip()
        pids = pid_out.split()
    except subprocess.CalledProcessError:
        pids = []
    if not pids:
        # fallback
        try:
            pid_out = subprocess.check_output(["pgrep", "-f", "./bin/mgmtd-cgo"], text=True).strip()
            pids = pid_out.split()
        except subprocess.CalledProcessError:
            pids = []
    if not pids:
        return False, "未找到 mgmtd-cgo 进程"
    pid = pids[0]
    # kill
    try:
        subprocess.run(["sudo", "-n", "kill", pid], check=True, timeout=10)
    except Exception as e:
        return False, f"kill 失败: {e}"
    time.sleep(2)
    # 启动
    env = {
        "MGM_SESSION_SECRET": "0123456789abcdef0123456789abcdef",
        "MGM_ADDR": ":9090",
        "MGM_CONFIG_FILE": "/tmp/router-re-config.json",
        "MGM_AUDIT_LOG": "/var/log/router-re/audit.jsonl",
    }
    cmd = ["sudo", "-n", "env"] + [f"{k}={v}" for k, v in env.items()] + \
          ["nohup", str(ROOT / "mgmtd" / "bin" / "mgmtd-cgo")]
    try:
        subprocess.Popen(cmd, cwd=str(ROOT / "mgmtd"),
                         stdout=open("/tmp/mgmtd-9090.log", "w"),
                         stderr=subprocess.STDOUT, start_new_session=True)
    except Exception as e:
        return False, f"启动失败: {e}"
    if not health_ok(30):
        return False, "重启后 health 不通"
    return True, "重启成功"

def main():
    token = login()
    report = []
    report.append("# Config Persist Test (重启后持久化)")
    report.append("")
    report.append(f"- Base: {BASE}")
    report.append(f"- Date: {__import__('datetime').datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    report.append("")

    # ---------- 1. 写配置 ----------
    report.append("## 1. 写配置（标记值）")
    report.append("")

    # ddns
    ddns_marker = {"provider": "p71test", "domain": "p71test.example.com", "enabled": True}
    s, b, _ = call("POST", "/api/v1/ddns/config", token=token, body=ddns_marker)
    ddns_write_ok = (s == 200)
    report.append(f"- POST /api/v1/ddns/config → HTTP {s} {'PASS' if ddns_write_ok else 'FAIL'}")
    if not ddns_write_ok:
        report.append(f"  body: {b}")

    # portal
    portal_marker = {"enabled": True, "portal_port": 9999, "default_minutes": 123}
    s, b, _ = call("POST", "/api/v1/portal/config", token=token, body=portal_marker)
    portal_write_ok = (s == 200)
    report.append(f"- POST /api/v1/portal/config → HTTP {s} {'PASS' if portal_write_ok else 'FAIL'}")
    if not portal_write_ok:
        report.append(f"  body: {b}")

    time.sleep(0.5)

    # ---------- 2. 重启前读回 ----------
    report.append("")
    report.append("## 2. 重启前读回")
    report.append("")

    s, ddns_before, _ = call("GET", "/api/v1/ddns/config", token=token)
    ddns_before_ok = (s == 200 and isinstance(ddns_before, dict)
                      and ddns_before.get("config", {}).get("provider") == "p71test")
    report.append(f"- GET /api/v1/ddns/config → provider={ddns_before.get('config',{}).get('provider') if isinstance(ddns_before,dict) else '?'} "
                 f"{'PASS' if ddns_before_ok else 'FAIL'}")

    s, portal_before, _ = call("GET", "/api/v1/portal/config", token=token)
    portal_before_ok = (s == 200 and isinstance(portal_before, dict)
                        and portal_before.get("portal_port") == 9999
                        and portal_before.get("default_minutes") == 123)
    report.append(f"- GET /api/v1/portal/config → port={portal_before.get('portal_port') if isinstance(portal_before,dict) else '?'} "
                 f"min={portal_before.get('default_minutes') if isinstance(portal_before,dict) else '?'} "
                 f"{'PASS' if portal_before_ok else 'FAIL'}")

    # ---------- 3. 重启 mgmtd ----------
    report.append("")
    report.append("## 3. 重启 mgmtd")
    report.append("")
    ok, msg = restart_mgmtd()
    report.append(f"- 重启: {msg} {'PASS' if ok else 'FAIL'}")
    if not ok:
        report.append("- 重启失败，后续验证跳过")
        OUT.write_text("\n".join(report), encoding="utf-8")
        print("\n".join(report))
        raise SystemExit(1)

    # 重新登录
    token = login()

    # ---------- 4. 重启后读回 ----------
    report.append("")
    report.append("## 4. 重启后读回")
    report.append("")

    s, ddns_after, _ = call("GET", "/api/v1/ddns/config", token=token)
    ddns_after_ok = (s == 200 and isinstance(ddns_after, dict)
                     and ddns_after.get("config", {}).get("provider") == "p71test"
                     and ddns_after.get("config", {}).get("domain") == "p71test.example.com")
    report.append(f"- GET /api/v1/ddns/config → provider={ddns_after.get('config',{}).get('provider') if isinstance(ddns_after,dict) else '?'} "
                 f"domain={ddns_after.get('config',{}).get('domain') if isinstance(ddns_after,dict) else '?'} "
                 f"{'PASS' if ddns_after_ok else 'FAIL'}")

    s, portal_after, _ = call("GET", "/api/v1/portal/config", token=token)
    portal_after_ok = (s == 200 and isinstance(portal_after, dict)
                       and portal_after.get("portal_port") == 9999
                       and portal_after.get("default_minutes") == 123)
    report.append(f"- GET /api/v1/portal/config → port={portal_after.get('portal_port') if isinstance(portal_after,dict) else '?'} "
                 f"min={portal_after.get('default_minutes') if isinstance(portal_after,dict) else '?'} "
                 f"{'PASS' if portal_after_ok else 'FAIL'}")

    # ---------- 5. 对比 ----------
    report.append("")
    report.append("## 5. 重启前后对比")
    report.append("")
    report.append(f"- ddns provider: before=p71test, after={'p71test' if ddns_after_ok else 'LOST'}")
    report.append(f"- portal port: before=9999, after={portal_after.get('portal_port') if isinstance(portal_after,dict) else '?'}")
    report.append(f"- portal minutes: before=123, after={portal_after.get('default_minutes') if isinstance(portal_after,dict) else '?'}")
    report.append("")

    all_ok = (ddns_write_ok and portal_write_ok and ddns_before_ok and portal_before_ok
              and ddns_after_ok and portal_after_ok)
    report.append("## 6. 结论")
    report.append(f"- 配置持久化: {'PASS' if all_ok else 'FAIL'}")
    report.append(f"- 重启后配置一致: {'是' if all_ok else '否'}")
    report.append("")

    OUT.write_text("\n".join(report), encoding="utf-8")
    print("\n".join(report))

if __name__ == "__main__":
    main()