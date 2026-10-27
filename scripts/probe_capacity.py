#!/usr/bin/env python3
"""网关容量标定：并发阶梯压测，找出安全并发与 RPM 天花板。

为什么需要这个：网关 model_limits 里 rpm/tpm/concurrency 全是 0，
PROVIDER_CHANNEL_RPM 写的是 300，但实测上游撑不到——于是网关既不能预警
也不能拒绝，只能撞墙后熔断。没有这组数字，任何并发预算都是猜的。

用法：
    python3 scripts/probe_capacity.py --model deepseek-v4-flash
    python3 scripts/probe_capacity.py --model deepseek-v4-flash --levels 1,2,4,8 --rounds 3

判据：取「成功率 >= 90% 的最大并发」为安全并发。找不到就是 1。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor

GATEWAY = os.environ.get("GATEWAY_URL", "http://10.100.1.17:8085/v1/chat/completions")


def load_key(explicit: str = "") -> str:
    """网关 key：命令行 > 环境变量 > 本地 key 文件 > Hermes config 提取。

    支持 KEY=VALUE 格式（bridge 写的 key 文件常是这种），不能整行当 key 用。
    """
    if explicit:
        return explicit.strip()
    key = os.environ.get("GATEWAY_KEY", "").strip()
    if key:
        return key
    for p in ("/home/jimwong/.gwkey", "/home/jimwong/.hermes/cache/scratch/.gwkey"):
        if os.path.exists(p):
            raw = open(p, encoding="utf-8").read().strip()
            if "=" in raw.split("\n")[0]:
                raw = raw.split("\n")[0].split("=", 1)[1]
            if raw.startswith("sk-"):
                return raw.strip()
    # 最后从 Hermes config 里捞第一个 sk- key（本机网关凭据就存在那里）
    import re
    for cf in ("/home/jimwong/.hermes/config.yaml", "/home/jimwong/.hermes/.env"):
        if not os.path.exists(cf):
            continue
        m = re.search(r"sk-[A-Za-z0-9]{20,}", open(cf, encoding="utf-8", errors="replace").read())
        if m:
            return m.group(0)
    sys.exit("缺少 key：用 --key、GATEWAY_KEY，或放 /home/jimwong/.gwkey")


def one_call(key: str, model: str, timeout: int = 60) -> tuple[bool, float, str]:
    """发一个最小请求。用 3 token 排除 token 因素，只测请求节奏。"""
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": "OK"}],
        "max_tokens": 3,
    }).encode()
    req = urllib.request.Request(GATEWAY, data=body, headers={
        "Authorization": "Bearer " + key,
        "Content-Type": "application/json",
    })
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            json.load(r)
            return True, time.time() - t0, ""
    except urllib.error.HTTPError as e:
        raw = e.read().decode(errors="replace")
        try:
            msg = json.loads(raw).get("error", {}).get("message", "")[:80]
        except Exception:  # noqa: BLE001
            msg = raw[:80]
        return False, time.time() - t0, msg
    except Exception as e:  # noqa: BLE001
        return False, time.time() - t0, f"{type(e).__name__}"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", required=True)
    ap.add_argument("--levels", default="1,2,4,8", help="并发阶梯，逗号分隔")
    ap.add_argument("--rounds", type=int, default=3, help="每档轮数")
    ap.add_argument("--gap", type=float, default=5.0, help="档间冷却秒数")
    ap.add_argument("--key", default="", help="网关 key；也可用 GATEWAY_KEY 环境变量")
    ap.add_argument("--out", default="")
    args = ap.parse_args()

    key = load_key(args.key)
    levels = [int(x) for x in args.levels.split(",")]
    report = {"model": args.model, "ts": time.strftime("%F %T"), "levels": {}}

    print(f"标定 {args.model}：并发 {levels}，每档 {args.rounds} 轮，档间冷却 {args.gap}s\n")
    for conc in levels:
        ok_total = 0
        n_total = 0
        errs: list[str] = []
        lat: list[float] = []
        for rnd in range(args.rounds):
            with ThreadPoolExecutor(max_workers=conc) as ex:
                res = list(ex.map(lambda _: one_call(key, args.model), range(conc)))
            ok_total += sum(1 for r in res if r[0])
            n_total += len(res)
            errs += [r[2] for r in res if not r[0] and r[2]]
            lat += [r[1] for r in res if r[0]]
            print(f"  并发={conc:2d} 轮{rnd + 1}: "
                  f"{sum(1 for r in res if r[0])}/{conc} ok")
            time.sleep(args.gap)
        rate = ok_total / n_total if n_total else 0.0
        report["levels"][conc] = {
            "ok": ok_total, "n": n_total, "rate": round(rate, 3),
            "avg_latency_s": round(sum(lat) / len(lat), 2) if lat else None,
            "errors": sorted(set(errs))[:3],
        }
        print(f"  => 成功率 {rate:.0%}  {('|'.join(sorted(set(errs))[:2]))[:90]}\n")

    safe = [c for c in levels if report["levels"][c]["rate"] >= 0.9]
    rec = max(safe) if safe else (min(levels) if levels else 1)
    report["recommended_concurrency"] = rec

    print("=" * 60)
    print(f"建议安全并发: {rec}（判据：成功率 >= 90% 的最大档）")
    for c in sorted(report["levels"]):
        print(f"  {c:2d} 并发 -> {report['levels'][c]['rate']:.0%}")

    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(report, f, ensure_ascii=False, indent=1)
        print(f"\n已写入 {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
