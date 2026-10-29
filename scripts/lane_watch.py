#!/usr/bin/env python3
"""lane_watch.py — 把 pane_brief 的异常项推到 Telegram。

为什么不用 Hermes 自己的 cron 投递：网关的 Telegram 适配器在这台机器上
一直连不上 api.telegram.org（国内网络，日志里刷 "Telegram network error
attempt 8/10"）。这里走 jp48 的 SSH SOCKS5 隧道，与网关解耦——
网关挂了、Telegram 插件掉了，告警照样能发出去。

判定复用 pane_brief.collect()，不另写一套：两边规则一旦分叉，就会出现
「面板说卡住、通知说正常」。

用法：
    python3 scripts/lane_watch.py            # 只打印，不发
    python3 scripts/lane_watch.py --notify   # 有问题才发，带冷却
"""
from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pane_brief as pb  # noqa: E402

ROOT = pb.ROOT
OPS = pb.OPS
STATE = OPS / ".lane-watch-state.json"
SOCKS = os.environ.get("LANE_WATCH_SOCKS", "socks5h://127.0.0.1:11080")
# 同一个问题持续存在时，多久提醒一次（避免每 5 分钟刷屏）
REMIND_SECONDS = int(os.environ.get("LANE_WATCH_REMIND", "1800"))


def env_value(path: Path, key: str) -> str:
    try:
        for line in path.read_text(encoding="utf-8").splitlines():
            if line.startswith(key + "="):
                return line.split("=", 1)[1].strip().strip('"').strip("'")
    except OSError:
        pass
    return ""


def problems(d: dict) -> dict[str, str]:
    """返回 {pane: 指纹}，只含有问题的窗口。

    指纹包含「类别 + 任务 + 停滞分钟数」，所以：同一个问题继续存在时指纹不变
    （走冷却），问题变了（卡住→进程死、换任务）立刻再报一次。
    """
    out = {}
    for r in d["panes"]:
        kind = r["kind"]
        # 有任务却不动、进程死、卡住，才算要打扰人；闲置和无任务忙碌不报
        bad = kind in ("dead", "stuck")
        if bad:
            stall_min = int((r.get("stall_sec") or 0) // 60)
            out[r["pane"]] = f"{kind}|{r.get('task') or '-'}|{stall_min}"
    return out


def render(d: dict, cur: dict[str, str], prev: dict[str, str]) -> str:
    lines = []
    for r in d["panes"]:
        if r["pane"] in cur:
            lines.append(f"⚠ {r['pane']}  {r['label']}  任务={r.get('task') or '-'}"
                         f"  停滞={r.get('stall_sec') or 0:.0f}s")
            if r.get("last_line"):
                lines.append(f"   最后一行：{r['last_line'][:100]}")
        elif r["pane"] in prev:
            lines.append(f"✅ {r['pane']} 已恢复正常（{r['label']}）")
    lines.append("")
    lines.append(f"队列：待派 {d['pending']} · 进行中 {d['dispatched']} · 待验证 {d['verify_queue']}")
    return "\n".join(lines)


def send(text: str) -> tuple[bool, str]:
    """用 curl 走 SOCKS5 发消息（不引入 PySocks 依赖）。"""
    token = env_value(Path.home() / ".hermes/.env", "TELEGRAM_BOT_TOKEN")
    chat = env_value(Path.home() / ".hermes/.env", "TELEGRAM_HOME_CHAT_ID")
    if not token or not chat:
        return False, "缺少 TELEGRAM_BOT_TOKEN / TELEGRAM_HOME_CHAT_ID"
    r = subprocess.run(
        ["curl", "-s", "-m", "25", "-x", SOCKS,
         "-X", "POST", f"https://api.telegram.org/bot{token}/sendMessage",
         "--data-urlencode", f"chat_id={chat}",
         "--data-urlencode", f"text={text}"],
        capture_output=True, text=True)
    out = (r.stdout or "").strip()
    try:
        j = json.loads(out)
    except Exception:  # noqa: BLE001
        return False, out[:200] or r.stderr[:200]
    if j.get("ok"):
        return True, f"message_id={j['result']['message_id']}"
    return False, str(j.get("description"))[:200]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--notify", action="store_true", help="有问题时发 Telegram")
    ap.add_argument("--force", action="store_true", help="忽略冷却，立刻发")
    args = ap.parse_args()

    d = pb.collect()
    cur = problems(d)
    try:
        state = json.loads(STATE.read_text(encoding="utf-8"))
    except Exception:  # noqa: BLE001
        state = {}
    prev: dict[str, str] = state.get("panes") or {}
    last_sent: float = state.get("last_sent") or 0

    changed = cur != prev
    due = (time.time() - last_sent) >= REMIND_SECONDS
    text = ""
    if cur:
        if changed or due or args.force:
            text = render(d, cur, prev)
    elif prev:
        # 从「有问题」变成「全好」：发一条恢复通知
        text = render(d, cur, prev)

    if not args.notify:
        print(text or "全部正常")
        return 0

    if not text:
        print(f"[{time.strftime('%H:%M:%S')}] 无变化，跳过（当前问题 {len(cur)} 个）")
        return 0

    ok, detail = send(text)
    print(f"[{time.strftime('%H:%M:%S')}] 发送{'成功' if ok else '失败'}: {detail}")
    if ok:
        STATE.write_text(json.dumps({"panes": cur, "last_sent": time.time()},
                                    ensure_ascii=False, indent=2), encoding="utf-8")
        return 0
    # 发送失败时保留旧状态，下一轮会重试（否则会以为已通知过而静默）
    return 3


if __name__ == "__main__":
    sys.exit(main())
