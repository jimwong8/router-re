#!/usr/bin/env python3
"""rbac_e2e.py — 角色鉴权的端到端验证。

## 为什么需要它

把角色鉴权接到请求链上之前，`rbac.Can` 策略写得对、单测也全绿，但**没有任何
请求走到过它**，于是 viewer 账号能直接 `POST /api/v1/firewall/rules` 改防火墙。
单测只证明策略函数本身正确，证明不了「中间件真的调了它」。

这个脚本从 HTTP 层打真实实例，覆盖三种角色 × 读写两条路径，外加清理。

## 用法

    python3 scripts/rbac_e2e.py [--base http://127.0.0.1:9090]

末行输出 `PASS n/8`；任何一项不符就 exit 1。报告写 docs/rbac-e2e.md。
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ADMIN_USER, ADMIN_PASS = "admin", "admin123"
TMP_VIEWER, TMP_OPERATOR = "e2e_viewer", "e2e_operator"
TMP_PASS = "E2etest12345!"

results: list[tuple[str, str, str, bool]] = []


def call(base: str, method: str, path: str, token: str | None = None,
         body: dict | None = None) -> tuple[int, dict | list | None]:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read().decode("utf-8", "replace")
            return r.status, (json.loads(raw) if raw.strip() else None)
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw) if raw.strip() else None
        except json.JSONDecodeError:
            return e.code, None
    except Exception as e:  # noqa: BLE001
        return 0, {"error": f"{type(e).__name__}: {e}"}


def login(base: str, user: str, pw: str) -> str:
    st, body = call(base, "POST", "/api/v1/auth/login", body={"username": user, "password": pw})
    if st != 200 or not isinstance(body, dict):
        return ""
    return body.get("token") or ""


def check(name: str, expect: int, st: int) -> None:
    ok = st == expect
    results.append((name, str(expect), str(st), ok))
    print(f"{'OK  ' if ok else 'FAIL'} {name}: 期望 {expect} 实际 {st}")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://127.0.0.1:9090")
    args = ap.parse_args()
    base = args.base.rstrip("/")

    admin = login(base, ADMIN_USER, ADMIN_PASS)
    if not admin:
        print(f"FAIL 无法以 admin 登录 {base}")
        return 1

    # 先清理上次残留（重复跑不能卡在 409）
    for u in (TMP_VIEWER, TMP_OPERATOR):
        call(base, "DELETE", f"/api/v1/users/{u}", admin)
        # 建号是 POST /api/v1/users（body 带 username），只有删除才是 /users/{username}
        call(base, "POST", "/api/v1/users", admin,
             {"username": u, "password": TMP_PASS, "role": "viewer" if u == TMP_VIEWER else "operator"})

    viewer = login(base, TMP_VIEWER, TMP_PASS)
    operator = login(base, TMP_OPERATOR, TMP_PASS)
    if not viewer or not operator:
        print("FAIL 测试账号登录失败")
        return 1

    rule = {"rules": [{"chain": "input", "proto": "tcp", "dport": "8", "action": "accept"}]}
    try:
        st, _ = call(base, "GET", "/api/v1/system/info", viewer)
        check("viewer 读系统信息", 200, st)

        st, _ = call(base, "POST", "/api/v1/firewall/rules", viewer, rule)
        check("viewer 改防火墙被拒", 403, st)

        st, _ = call(base, "POST", "/api/v1/config/restore", viewer, {"confirm": True})
        check("viewer 恢复配置被拒", 403, st)

        st, _ = call(base, "GET", "/api/v1/users", viewer)
        check("viewer 读用户列表被拒", 403, st)

        st, _ = call(base, "POST", "/api/v1/firewall/rules", operator, rule)
        check("operator 改防火墙放行", 200, st)

        st, _ = call(base, "GET", "/api/v1/users", operator)
        check("operator 管用户被拒", 403, st)

        st, _ = call(base, "GET", "/api/v1/users", admin)
        check("admin 读用户列表", 200, st)

        # 清理并确认真的删掉了
        for u in (TMP_VIEWER, TMP_OPERATOR):
            call(base, "DELETE", f"/api/v1/users/{u}", admin)
        st, body = call(base, "GET", "/api/v1/users", admin)
        names = {u.get("username") for u in (body or {}).get("users", [])} if isinstance(body, dict) else set()
        cleanup_ok = st == 200 and TMP_VIEWER not in names and TMP_OPERATOR not in names
        check("清理测试账号", 200, st if cleanup_ok else 0)
    finally:
        # 清理残余防火墙规则，别给实例留垃圾
        call(base, "POST", "/api/v1/firewall/rules", admin, {"rules": []})

    passed = sum(1 for *_, ok in results if ok)
    total = len(results)
    write_report(passed, total)
    print(f"\nPASS {passed}/{total}")
    return 0 if passed == total else 1


def write_report(passed: int, total: int) -> None:
    lines = [
        "# RBAC 端到端验证",
        "",
        "验证「角色鉴权是否真的接在请求链上」——策略函数 rbac.Can 单独有单测，",
        "但曾经因为没接线，viewer 仍能改防火墙；单测证明不了这件事。",
        "",
        f"- 结果：{passed}/{total}",
        "",
        "| 检查项 | 期望 | 实际 | 结论 |",
        "|---|---|---|---|",
    ]
    for name, exp, act, ok in results:
        lines.append(f"| {name} | {exp} | {act} | {'PASS' if ok else 'FAIL'} |")
    lines.append("")
    out = ROOT / "docs" / "rbac-e2e.md"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("\n".join(lines), encoding="utf-8")
    print(f"报告: {out}")


if __name__ == "__main__":
    sys.exit(main())
