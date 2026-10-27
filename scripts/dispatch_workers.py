#!/usr/bin/env python3
"""工人调度器：队列驱动，工人一空就自动派下一个。

## 为什么需要它

原先 `watch_workers.py` 只做一件事：工人空闲超过 N 分钟就发通知。
问题在于通知的接收者是我（助手/人），我一旦在忙别的，工人就空转 ——
实测出现过 pQ 在 23:01 就交了 voucher，直到 23:10 才被派下一单，
中间 9 分钟纯浪费。瓶颈不在工人，在"谁来决定下一个做什么"。

这个脚本把那个决定搬进队列：

    ops/tasks.jsonl  （我往这里塞任务）
            │
            ▼
      dispatch_workers.py  ──每 60s──►  空闲的 pane 立刻领走一条
            │
            ├─ 记录 dispatched_at，防止重复派发
            └─ 下次运行检查 expect_files：文件出现了就标 done，并自动填下一条

于是我的工作从"盯着谁空了"变成"把活排进队列 + 审查产出"。

## 队列格式（每行一个 JSON）

    {"id": "p18-linkcheck", "prompt": "完整的任务描述...",
     "expect": ["mgmtd/internal/linkcheck/probe.go"],
     "prefer": "w1:pN"}        # prefer 可省，省略则任意空闲 pane

字段：
    id       唯一标识，重复 id 会被忽略
    prompt   派给 herdr agent prompt 的全文
    expect   产出文件（相对项目根），全部存在才算完成；可省
    prefer   指定 pane；被占用时可以让别的 pane 接

## 状态机

    pending ──派发──► dispatched ──产出出现──► done
                            │
                            └──空闲且超时仍无产出──► failed（可重新入队）

## 安全阀

- 一个 pane 同时只能有一个 dispatched 任务，绝不叠加
- pane_state 不是 idle 绝不派发（unknown 当作忙，避免打断）
- 派发命令失败 → 立刻退回 pending，不吞掉任务
- 每轮最多派 N 条（默认 4），避免一次性把队列灌空
"""
from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
QUEUE = Path(os.environ.get("QUEUE_PATH", str(ROOT / "ops" / "tasks.jsonl")))
LOG = Path(os.environ.get("DISPATCH_LOG", str(ROOT / "logs" / "dispatch.log")))
STATE = Path(os.environ.get("DISPATCH_STATE", str(ROOT / ".dispatch-state.json")))
WORKDIR = Path(os.environ.get("WORK_DIR", str(ROOT)))

# PANES 支持两种写法，坚哥报哪种都行：
#   pane 写法（herdr-konsole 里直接看到的）：w1:pN w1:pQ w1:pP
#   会话 ID 写法（长串如 abc123...）：自动映射到 pane（见 resolve_panes()）。
# 数量即规模：一拖二就报 2 个，一拖三报 3 个，一拖四报 4 个，脚本智能识别。
PANES_RAW = os.environ.get("PANES", "w1:pN w1:pQ w1:pP w1:pM").split()
MAX_PER_ROUND = int(os.environ.get("MAX_PER_ROUND", "0"))  # 0 = 自动 = pane 数
# pane 的模型档位。默认全 free（机械执行）；哪个 pane 是强模型就把它的 id 写进
# STRONG_PANES（如 "w1:pM"），plan/verify 类任务只派给它们。
# 为什么改成环境变量：以前写死 PANE_TIER，每换一批 pane 就要改代码。
STRONG_PANES = set(os.environ.get("STRONG_PANES", "w1:pM").split())
JUDGEMENT_TIERS = {"plan", "verify"}
STALE_MINUTES = float(os.environ.get("STALE_MINUTES", "20"))  # 派发后多久没产出算失败
# 工人已经 idle（它认为做完了）但产出不达标，多久之后释放它。
# 没有这个，会出现"任务卡在 WAIT 占着 pane，pane 又领不到新活"的死局：
# 实测 pQ 写了 97 行（要求 120）后认为自己完成，之后一直空转。
IDLE_STUCK_MINUTES = float(os.environ.get("IDLE_STUCK_MINUTES", "4"))
HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")

# 终端里的忙/闲标记。与 watch_workers.py 保持一致。
# 判忙闲的标记。这些字符串来自实测的终端尾部文本，不是猜的：
#
#   busy（真的在干活）
#     "esc interrupt"                 带 ☤ 前缀，等待可打断
#     "⏳ waiting on <model>"           上游慢，还在等第一个 chunk
#     "Working / Preparing / Thinking"
#     "☤ ❯ type to redirect this turn" 这一行在输出过程中出现
#
#   idle（可以派活）
#     "Type / to browse commands"      无历史时的空提示
#     "Ctrl+P for the palette"
#     "❯ Explain this error and how..." 有历史时的空提示（实测 pP 停在这里）
#
# 顺序很关键：先判 busy 再判 idle，因为忙的时候也可能出现 ❯ 提示行。
BUSY_MARKERS = [
    "esc interrupt",
    "⏳ waiting on",
    "waiting for the first stream chunk",
    "working", "preparing", "thinking",
    "type to redirect this turn",
]
IDLE_MARKERS = [
    "type / to browse commands",
    "ctrl+p commands",          # OpenCode 状态栏的空闲写法（实测 pM）
    "ctrl+p for the palette",
    "❯ explain this error",
    "❯ ask anything",
    "❯ type a message",
]

# 阻塞态：上游限流/重试。这不是 busy，也不是 idle —— 是"卡住了"。
#
# 为什么必须单独判：实测 pM（OpenCode）撞上游 TPM 后，屏幕上是
#     ⚠ Retry due · attempt 5 · channel rate gate: no available channel
# 而底部状态栏同时有 "ctrl+p commands"，命中了 IDLE_MARKERS。
# 原逻辑先判 busy 再判 idle，这两行都不在 BUSY_MARKERS 里，于是 pM 被判成
# **idle**，调度器继续往里派活 —— 新任务和卡住的旧任务一起堆在同一个死锁里。
# 所以 blocked 必须单独成一态，且优先级高于 idle。
BLOCKED_MARKERS = [
    "retry due",
    "retrying in",
    "channel rate gate",
    "no available channel",
    "provider circuit open",
    "inference exceeds tpm/rpm",
]

# 上下文水位线。超过就先重置会话再派活，理由：
#
# 网关侧 deepseek-v4-flash 的 safe_context_window 只有 5333，而上游真实
# TPM 限制更硬 —— 实测 140K tokens 的请求被上游 429（inference exceeds
# tpm/rpm limit），66K 以下可以通过。OpenCode 会话涨到 139.4K 之后，
# 每次请求都超限 → 所有 channel 进 rate gate → no available channel → 无限重试。
# 更糟的是想 /compact 也发不出去（压缩本身要一次大请求），形成死锁。
# 所以在水位线以下就重置，不要等它卡死。
MAX_CONTEXT_PCT = float(os.environ.get("MAX_CONTEXT_PCT", "60"))

# 两种状态栏格式，实测各客户端的写法不同：
#   OpenCode : "139.4K (70%) · $0.07  ctrl+p commands"
#   Hermes   : "☤ beam-free · 15% · ◎ 90% · 17h 48m · ~"
# Hermes 那条正则要求百分号两侧都是 ·，这样就不会误抓到 "◎ 90%"（那是别的指标）。
CONTEXT_PATTERNS = [
    re.compile(r"(\d+(?:\.\d+)?)K\s*\((\d+)%\)"),
    re.compile(r"·\s*~?(\d+(?:\.\d+)?)%\s*·"),
]

# ---------- 硬预算与退避重试 ----------
#
# 硬预算：任务必须在声明的时间内结束，不管它自认为做没做完。
# MAST FM-1.5「不知道终止条件」是 41.8% 系统设计类失败的一项；真实事故里
# 两个 agent 互相对话 11 天烧掉 $47,000，就是因为没有外部强制的终止条件。
# 本机实测最长的两个任务跑了 27 和 49 分钟（都靠"迟到产出"才捞回来），
# 说明长尾任务必须有上限。
MAX_WALL_MINUTES = float(os.environ.get("MAX_WALL_MINUTES", "20"))

# 退避重试：失败后不能立刻回队列。
# 实测网关容量：并发 4 -> 83%，并发 8 -> 17%（错误变成 rpm exhausted）。
# 上游 RPM 是滑动窗口桶，失败后立即重试会继续打满桶、把整个池子拖死。
# 必须让桶先回血。
RETRY_BACKOFF_BASE = float(os.environ.get("RETRY_BACKOFF_BASE", "30"))
RETRY_BACKOFF_MAX = float(os.environ.get("RETRY_BACKOFF_MAX", "240"))
MAX_RETRIES = int(os.environ.get("MAX_RETRIES", "3"))


def now() -> float:
    return time.time()


def stamp(t: float | None = None) -> str:
    return time.strftime("%H:%M:%S", time.localtime(t if t is not None else now()))


def log(msg: str) -> None:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    with LOG.open("a", encoding="utf-8") as f:
        f.write(f"[{stamp()}] {msg}\n")
    print(msg)


def notify_pane(pane: str, msg: str) -> bool:
    """往工人 pane 里回贴一句话，让坚哥在 konsole 对话框里直接看到进展。

    为什么用 agent prompt 而不是 pane send-text：send-text 只打字不提交，
    prompt 才是语义化提交。工人 pane 是 AI 会话，收到后会正常回应，
    不会打断它正在做的事（它处于 idle 才会被派新活，busy 时根本走不到这里）。
    失败只记日志不阻塞调度 —— 通知挂了不能让派活停摆。
    """
    if not pane:
        return False
    try:
        r = subprocess.run(
            [HERDR, "agent", "prompt", pane, msg],
            capture_output=True, text=True, timeout=30,
        )
    except Exception as exc:  # noqa: BLE001
        log(f"WARN 回贴 {pane} 抛异常: {exc}")
        return False
    if r.returncode != 0:
        log(f"WARN 回贴 {pane} rc={r.returncode}: {(r.stderr or r.stdout)[:120]}")
        return False
    return True


# ---------- 队列 ----------

def load_queue() -> list[dict]:
    """读队列。坏行跳过并记录，不让一行脏数据卡死整个调度。"""
    if not QUEUE.exists():
        return []
    tasks = []
    for i, line in enumerate(QUEUE.read_text(encoding="utf-8").splitlines(), 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        try:
            task = json.loads(line)
        except json.JSONDecodeError as exc:
            log(f"WARN 队列第 {i} 行不是合法 JSON，已跳过: {exc}")
            continue
        # 补默认 status。不补会出事：派发分支用 `get("status") != "pending"`，
        # 缺失时得到 None 从而被跳过；但汇总统计用 `get("status", "pending")`
        # 又把它算成 pending —— 任务计数里有、却永远不会被派发。
        task.setdefault("status", "pending")
        tasks.append(task)
    return tasks


def save_queue(tasks: list[dict]) -> None:
    QUEUE.parent.mkdir(parents=True, exist_ok=True)
    tmp = QUEUE.with_suffix(".tmp")
    with tmp.open("w", encoding="utf-8") as f:
        for t in tasks:
            f.write(json.dumps(t, ensure_ascii=False) + "\n")
    tmp.replace(QUEUE)  # 原子替换，避免写到一半被读


def load_state() -> dict:
    if not STATE.exists():
        return {}
    try:
        return json.loads(STATE.read_text(encoding="utf-8"))
    except Exception:
        return {}


def save_state(st: dict) -> None:
    STATE.write_text(json.dumps(st, ensure_ascii=False, indent=2), encoding="utf-8")


# ---------- 工人状态 ----------

def read_tail(pane: str, lines: int = 8) -> str:
    """读 pane 尾部文本。所有状态判定都基于同一份快照，避免多次读不一致。"""
    try:
        return subprocess.run(
            [HERDR, "agent", "read", pane, "--lines", str(lines)],
            capture_output=True, text=True, timeout=15,
        ).stdout
    except Exception as exc:  # noqa: BLE001
        log(f"WARN herdr read {pane} 失败: {exc}")
        return ""


def pane_state(pane: str) -> str:
    """读 pane 尾部判状态：busy / idle / blocked / unknown。

    判定顺序：blocked > busy > idle。blocked 必须排在 idle 前面，
    因为卡在重试时状态栏同时带着 idle 标记（见 BLOCKED_MARKERS 注释）。
    """
    out = read_tail(pane, 8)
    if not out.strip():
        return "unknown"
    # 大小写不敏感：同一个 TUI 在不同场景下会写 "Ctrl+P" / "ctrl+p"，
    # 区分大小写会让工人被误判成 unknown 从而永远领不到活。
    low = out.lower()
    if any(m.lower() in low for m in BLOCKED_MARKERS):
        return "blocked"
    if any(m.lower() in low for m in BUSY_MARKERS):
        return "busy"
    if any(m.lower() in low for m in IDLE_MARKERS):
        return "idle"
    return "unknown"


def context_pct(pane: str) -> float | None:
    """读状态栏里的上下文水位，如 "139.4K (70%)" 或 "· 15% ·" -> 70.0 / 15.0。

    读不到时返回 None 而不是 0：把"不知道"当成"很健康"会让水位保护失效。
    """
    out = read_tail(pane, 14)
    hits: list[tuple[int, float]] = []
    for pat in CONTEXT_PATTERNS:
        for m in pat.finditer(out):
            # 取最后一个非 None 捕获组：OpenCode 是第 2 组（百分比），
            # Hermes 是第 1 组。用 groups() 过滤而不是 lastindex ——
            # lastindex 在没有任何组匹配时是 None，直接 group() 会抛异常。
            groups = [g for g in m.groups() if g is not None]
            if not groups:
                continue
            hits.append((m.end(), float(groups[-1])))
    if not hits:
        return None
    hits.sort()          # 按出现位置排序，取状态栏里最靠后的那个（最新渲染）
    return hits[-1][1]


def reset_context(pane: str) -> bool:
    """把上下文超标的会话重置掉。返回是否成功。

    先 esc 停掉正在进行的重试，再 /new —— 顺序不能反：重试中的 TUI 会把
    /new 当成新的 turn 输入，而 esc 之后它才回到可接受命令的状态。
    """
    log(f"RESET {pane} 上下文水位过高，先 esc 再 /new")
    try:
        subprocess.run([HERDR, "agent", "send-keys", pane, "esc"],
                       capture_output=True, text=True, timeout=15)
    except Exception as exc:  # noqa: BLE001
        log(f"WARN {pane} esc 失败: {exc}")
    time.sleep(2)
    ok = dispatch(pane, "/new")
    time.sleep(3)
    log(f"RESET {pane} {'完成' if ok else '失败'}")
    return ok


def deps_satisfied(task: dict, tasks: list[dict]) -> bool:
    """依赖未全部 done 的任务不许派发。

    为什么要这个：没有依赖排序时，UI 任务会和它依赖的 API 契约并行启动，
    UI 只能照着猜的字段写，等契约改了再返工——这比串行更慢（Agentastic
    七阶段的第一阶段就是「任务图而不是提示列表」）。
    """
    ids_done = {t.get("id") for t in tasks if t.get("status") == "done"}
    for dep in task.get("depends_on") or []:
        if dep not in ids_done:
            return False
    return True


def retry_ready(task: dict, t: float) -> bool:
    """退避窗口未到的任务本轮不派。

    未设置 retry_after 视为就绪 —— 首次派发不该被退避逻辑误挡。
    """
    ra = task.get("retry_after")
    if ra is None:
        return True
    return t >= float(ra)


def pane_identities() -> dict[str, str]:
    """pane_id -> 会话标识。一次 herdr 调用拿全量，避免每个 pane 读一次。

    坚哥报的可能是 pane 号（w1:pN），也可能是会话 ID（herdr 里那串长的）。
    这里把两者都建成索引：pane_id 本身 + terminal_title 里能看到的标识。
    """
    try:
        r = subprocess.run([HERDR, "agent", "list"],
                           capture_output=True, text=True, timeout=15)
        agents = json.loads(r.stdout).get("result", {}).get("agents", [])
    except Exception:  # noqa: BLE001
        return {}
    out = {}
    for a in agents:
        pid = a.get("pane_id", "")
        if pid:
            out[pid] = pid
            # terminal_id 也建索引，万一坚哥报的是那串 term_xxx
            tid = a.get("terminal_id", "")
            if tid:
                out[tid] = pid
    return out


def resolve_panes() -> list[str]:
    """把 PANES_RAW 解析成真正的 pane_id 列表。

    - 已经是 w1:pX 格式：直接用。
    - 别的格式（会话 ID / terminal_id）：查 pane_identities() 映射。
    - 映射不到：在日志里 WARN 并跳过，不让一个错 ID 卡死整轮。
    数量即规模：一拖二就是 2 个，一拖三 3 个，一拖四 4 个，不用再配别的东西。
    """
    table = pane_identities()
    panes = []
    for raw in PANES_RAW:
        if re.match(r"^w\d+:p[A-Z]+$", raw):
            panes.append(raw)
        elif raw in table:
            panes.append(table[raw])
        else:
            log(f"WARN PANES 里 {raw!r} 不是 pane 号也映射不到会话，已跳过")
    return panes


def pane_tier(pane: str) -> str:
    """plan / verify 只给强模型。默认全 free，STRONG_PANES 环境变量指定强 pane。"""
    return "strong" if pane in STRONG_PANES else "free"


def wall_limit_minutes(task: dict) -> float:
    """任务级硬预算，缺省回落到全局 STALE_MINUTES。

    任务可以自己声明 max_wall_minutes（长任务放宽），但永远有上限。
    """
    v = task.get("max_wall_minutes")
    try:
        return float(v) if v is not None else STALE_MINUTES
    except (TypeError, ValueError):
        return STALE_MINUTES


def dispatch(pane: str, prompt: str) -> bool:
    """派活。返回是否成功。命令本身失败必须如实返回，不能假装派了。"""
    try:
        r = subprocess.run(
            [HERDR, "agent", "prompt", pane, prompt],
            capture_output=True, text=True, timeout=60,
        )
    except Exception as exc:  # noqa: BLE001
        log(f"ERROR 派发到 {pane} 抛异常: {exc}")
        return False
    if r.returncode != 0:
        log(f"ERROR 派发到 {pane} rc={r.returncode}: {(r.stderr or r.stdout)[:200]}")
        return False
    return True


def produced(task: dict) -> bool:
    """产出验收。三道关，缺一不可。

    为什么要三道：最早只查"文件存在"，结果一个任务 1 分钟就被标成 DONE。
    文件确实存在，但那是工人写的空壳，或者写了一半。文件存在只能说明
    "动手了"，说明不了"做对了"。

    1. expect      —— 文件存在
    2. min_lines   —— {"path": 行数}，挡住空壳（一个 200 行的 handler 不可能 3 行写完）
    3. must_contain —— {path: [必须出现的字符串]}，比行数精确
    4. must_build  —— 跑一次 go build，挡住写不出来的代码
    """
    expect = task.get("expect") or []
    if not expect:
        return False
    if not all((ROOT / p).exists() for p in expect):
        return False

    for path, need in (task.get("min_lines") or {}).items():
        f = ROOT / path
        if not f.exists():
            return False
        try:
            got = sum(1 for _ in f.open(encoding="utf-8", errors="replace"))
        except OSError:
            return False
        if got < int(need):
            log(f"WAIT  {task['id']}: {path} 只有 {got} 行（要求 >= {need}）")
            return False

    # must_contain：文件里必须出现的字符串。比 min_lines 精确 ——
    # 行数只是"写了多少"的粗代理，实测出现过 97 行的完整实现被 120 行门槛
    # 挡住（p18b.go 三个 handler 全在、CSP 头也有）。真正该断言的是
    # "关键东西在不在"，不是"够不够长"。
    for path, needles in (task.get("must_contain") or {}).items():
        f = ROOT / path
        if not f.exists():
            return False
        try:
            text = f.read_text(encoding="utf-8", errors="replace")
        except OSError:
            return False
        for needle in needles:
            if needle not in text:
                log(f"WAIT  {task['id']}: {path} 缺少关键内容 {needle!r}")
                return False

    if task.get("must_build"):
        for workdir, cmd in (task.get("build_cmds") or [("mgmtd", ["go", "build", "./..."])]):
            try:
                r = subprocess.run(cmd, cwd=ROOT / workdir, capture_output=True, text=True, timeout=240)
            except subprocess.TimeoutExpired:
                log(f"WAIT  {task['id']}: 构建超时")
                return False
            if r.returncode != 0:
                log(f"WAIT  {task['id']}: 构建失败 {(r.stderr or '')[:160]}")
                return False

    return True


# ---------- 主流程 ----------

def main() -> int:
    tasks = load_queue()
    if not tasks:
        return 0

    state = load_state()
    t = now()

    # 1) 结算：已派发的任务，产出出现就标 done
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        if produced(task):
            task["status"] = "done"
            task["done_at"] = t
            log(f"DONE  {task['id']}  (pane={task.get('pane')}, 耗时 {int((t - task.get('dispatched_at', t)) / 60)} 分钟)")
            state.pop(task.get("pane", ""), None)
            # 工人对话框里能看到进展：结算时回贴一句话
            notify_pane(task.get("pane", ""), f"✅ {task['id']} 已验收通过（done）")

    # 1.5) 迟到产出：非 done 状态但文件后来才出现 -> 翻回 done
    #
    # 实测：p20-api-doc 派发 23 分钟无产出被判 failed，但工人在那之后才把
    # docs/api-reference.md 写出来（314 行 / 276 端点）。不翻回来的话，
    # 实际已完成的活会一直挂在失败列表里，人看到 failed 就会重复派同一个
    # 任务，白烧一遍 token。
    #
    # 覆盖 pending 也要检查：加了退避重试后，超时任务会先退回 pending，
    # 若产出在这段窗口里出现，同样不该再派一次。
    for task in tasks:
        st = task.get("status")
        if st == "done":
            continue
        if produced(task):
            task["status"] = "done"
            task["done_at"] = t
            task["late"] = True
            log(f"DONE(LATE) {task['id']}  原状态 {st}，产出后来才出现，翻回 done")
            state.pop(task.get("pane", ""), None)
            # 工人对话框里能看到进展：结算时回贴一句话
            notify_pane(task.get("pane", ""), f"✅ {task['id']} 已验收通过（done）")

    # 2) 超时无产出 -> 退避重试；重试耗尽才 failed
    #
    # 硬预算来自任务自己的 max_wall_minutes，缺省用 STALE_MINUTES。
    # 失败后不立刻回队列：上游 RPM 是滑动窗口桶，立即重派会继续打满桶，
    # 把整个池子拖死（实测并发 8 时成功率从 83% 掉到 17%）。
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        limit = wall_limit_minutes(task)
        age_min = (t - task.get("dispatched_at", t)) / 60
        if age_min > limit and not produced(task):
            n = int(task.get("retried") or 0) + 1
            task["retried"] = n
            if n <= MAX_RETRIES:
                delay = min(RETRY_BACKOFF_BASE * (2 ** (n - 1)), RETRY_BACKOFF_MAX)
                task["status"] = "pending"
                task["retry_after"] = t + delay
                task["fail_reason"] = f"派发 {int(age_min)} 分钟无产出（硬预算 {int(limit)} 分钟）"
                log(f"RETRY {task['id']}  第 {n}/{MAX_RETRIES} 次，{int(delay)}s 后重试"
                    f"（{task['fail_reason']}）")
            else:
                task["status"] = "failed"
                task["failed_at"] = t
                task["fail_reason"] = (
                    f"派发 {int(age_min)} 分钟无产出，重试 {MAX_RETRIES} 次仍失败"
                )
                log(f"FAIL  {task['id']}  {task['fail_reason']} (pane={task.get('pane')})")
            state.pop(task.get("pane", ""), None)

    # 2.5) 工人已经空闲但产出不达标 -> 释放 pane，任务标 failed
    #
    # 死局场景：工人写了 97 行（要求 120）后自己认为完成并回到 idle，
    # 调度器因为 min_lines 不通过而卡在 WAIT。此时任务仍占着 pane，
    # 该 pane 领不到新活 —— 工人和任务互相等待，纯浪费。
    # 判据：pane 已 idle + 距离派发已超过 IDLE_STUCK_MINUTES。
    for task in tasks:
        if task.get("status") != "dispatched":
            continue
        pane = task.get("pane", "")
        age_min = (t - task.get("dispatched_at", t)) / 60
        if age_min < IDLE_STUCK_MINUTES:
            continue
        if pane_state(pane) != "idle":
            continue
        n = int(task.get("retried") or 0) + 1
        task["retried"] = n
        task["fail_reason"] = (
            f"工人已空闲但产出不达标（{int(age_min)} 分钟）: "
            f"没通过 {'/'.join(task.get('min_lines') or {}) or '文件存在性'} 检查"
        )
        if n <= MAX_RETRIES:
            delay = min(RETRY_BACKOFF_BASE * (2 ** (n - 1)), RETRY_BACKOFF_MAX)
            task["status"] = "pending"
            task["retry_after"] = t + delay
            log(f"RELEASE {task['id']}  工人 {pane} 已空闲但产出不达标，释放 pane；"
                f"{int(delay)}s 后第 {n}/{MAX_RETRIES} 次重试")
        else:
            task["status"] = "failed"
            task["failed_at"] = t
            log(f"RELEASE {task['id']}  工人 {pane} 已空闲但产出不达标，重试耗尽 -> failed")
        state.pop(pane, None)

    # 3) 哪些 pane 手上还有活（dispatched 未结算）
    busy_panes = {task.get("pane") for task in tasks if task.get("status") == "dispatched"}

    # 4) 派发：空闲 pane 领 pending 任务
    # PANES 在这里解析（不是模块顶层）：坚哥每次报的 pane 可能不一样，
    # 环境变量 PANES 由启动器按本轮传入，脚本只负责解析 + 智能识别规模。
    panes = resolve_panes()
    if not panes:
        log("WARN 没有可用的 pane，本轮跳过派发（只做结算）")
    max_round = MAX_PER_ROUND if MAX_PER_ROUND > 0 else len(panes)  # 0 = 自动 = pane 数
    dispatched_now = 0
    for pane in panes:
        if dispatched_now >= max_round:
            break
        if pane in busy_panes:
            continue

        st = pane_state(pane)

        # 上游限流重试中的 pane：先判断是不是上下文打爆造成的死锁。
        # 是 -> 重置会话（否则永远出不来，因为压缩本身也要一次超限请求）；
        # 不是 -> 本轮跳过，让重试自己恢复，绝不往里再压任务。
        if st == "blocked":
            pct = context_pct(pane)
            if pct is not None and pct >= MAX_CONTEXT_PCT:
                log(f"BLOCKED {pane} 上下文 {pct:.0f}% >= {MAX_CONTEXT_PCT:.0f}%，判定 TPM 死锁，重置会话")
                if not reset_context(pane):
                    continue
                st = pane_state(pane)
            else:
                log(f"BLOCKED {pane} 上游限流重试中（上下文 {pct if pct is not None else '?'}%），本轮跳过")
                continue

        if st != "idle":
            continue

        # 空闲但水位已高：先重置再派，避免新任务一进去就撞 TPM 上限。
        pct = context_pct(pane)
        if pct is not None and pct >= MAX_CONTEXT_PCT:
            log(f"RESET {pane} 上下文 {pct:.0f}% >= {MAX_CONTEXT_PCT:.0f}%，重置后再派")
            if not reset_context(pane):
                continue
            if pane_state(pane) != "idle":
                continue

        # 优先取指定了本 pane 的，其次取没指定 prefer 的。
        # 四道门禁：有 prompt、依赖已完成、退避窗口已过、tier 匹配 pane 档位。
        def eligible(task: dict) -> bool:
            if task.get("status") != "pending":
                return False
            if not (task.get("prompt") or "").strip():
                return False
            if not retry_ready(task, t) or not deps_satisfied(task, tasks):
                return False
            # 判断类任务不给免费模型：给它也做不对，还占着 pane 烧 token
            if task.get("tier") in JUDGEMENT_TIERS and pane_tier(pane) != "strong":
                return False
            return True

        cand = None
        for task in tasks:
            if eligible(task) and task.get("prefer") == pane:
                cand = task
                break
        if cand is None:
            for task in tasks:
                if eligible(task) and not task.get("prefer"):
                    cand = task
                    break
        if cand is None:
            # 这个 pane 没有能领的活 —— 但不能 break 跳出整个循环。
            # 加了 tier 过滤后，"本 pane 领不了"不代表"别的 pane 也领不了"：
            # free pane 遇到 tier=verify 会没候选，若在此 break，
            # 后面的 strong pane 永远轮不到，判断类任务会永远卡在 pending。
            continue

        if dispatch(pane, cand.get("prompt") or ""):
            cand["status"] = "dispatched"
            cand["pane"] = pane
            cand["dispatched_at"] = t
            state[pane] = cand["id"]
            dispatched_now += 1
            log(f"DISPATCH {cand['id']} -> {pane}")
            # 工人对话框里能看到新活：派发时回贴任务名
            notify_pane(pane, f"📥 新任务 {cand['id']} 已派给你，开始干活吧")
        else:
            # 派发失败：保持 pending，下一轮重试（不吞任务）
            log(f"RETRY {cand['id']} 派发失败，保持 pending")

    save_queue(tasks)
    save_state(state)

    # 汇总
    counts: dict[str, int] = {}
    for task in tasks:
        counts[task.get("status", "pending")] = counts.get(task.get("status", "pending"), 0) + 1
    summary = " ".join(f"{k}={v}" for k, v in sorted(counts.items()))
    if dispatched_now or counts.get("pending"):
        log(f"SUMMARY {summary} | 本轮派发 {dispatched_now}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
