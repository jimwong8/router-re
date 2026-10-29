#!/usr/bin/env python3
"""独立验证工人：对已完成任务的产出做「非实现者」审查。

为什么必须有这一层：
  四层判据（expect / min_lines / must_contain / must_build）只证明
  「东西在、不是空壳、能编译」，**不证明行为对**。而 MAST 数据显示
  验证类失败占 21.3%，其中「实现者自审」是 FM-3.2/FM-3.3 的直接成因。
  这是免费模型能安全放量的唯一前提 —— 没有它，越省钱越危险。

它做三件事：
  1. 静态空壳检测（TODO / panic("not implemented") / 占位返回 / 硬编码 stub）
  2. 编译 / vet（能跑就跑，跑不了如实标 skip）
  3. 用**与实现者不同**的模型审文件内容，判 PASS/FAIL 并给理由

用法：
    python3 scripts/verify_outputs.py                 # 验证所有未验证的 done 任务
    python3 scripts/verify_outputs.py --id p21-fwlog-wire
    python3 scripts/verify_outputs.py --recheck       # 连已验证的也重审
    python3 scripts/verify_outputs.py --dry-run       # 只打印将要做什么

判定写回队列的 `verified`（pass/fail/skip）与 `verify_notes`，幂等可重跑。
"""
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(os.environ.get("WORK_DIR", "/home/jimwong/projects/router-re"))
sys.path.insert(0, str(Path(__file__).resolve().parent))
import queue_lib as ql  # noqa: E402  并发安全的队列写入（lane 会同时改别的任务）

QUEUE = ROOT / "ops" / "tasks.jsonl"
LOG = ROOT / "logs" / "verify.log"
GATEWAY = os.environ.get("GATEWAY_URL", "http://10.100.1.17:8085/v1/chat/completions")
HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")
MUST_BUILD_DIR = os.environ.get("BUILD_DIR", "mgmtd")  # 新项目不是 Go 就改这里

# 验证者模型必须与实现者不同，否则等于自审（同模型审自己产物没意义）。
# 不能固定死一个模型：工人 pane 的模型会随时换（今天全切到 deepseek-v4-flash，
# 固定验证者就撞车全 SKIP，验证覆盖率悄悄归零）。
#
# 也不能只挑一个就不再退让：网关上游会整段耗尽配额——实测 deepseek/glm/sensenova
# 三条在 1.5K token 以上全部 429 insufficient_quota（小请求反而通过，所以"探活"
# 探不出来），只有 longcat-2.5-preview 能吃完 3.4K 的审查 prompt。固定挑一个
# 就会出现"验证者三次失败→SKIP"，验证覆盖率又一次悄悄归零。
#
# 所以这里是**有序候选表**：按顺序试，前一个模型调用失败就换下一个。
#   VERIFIER_POOL 环境变量（空格分隔）> 默认池
VERIFIER_MODEL = os.environ.get("VERIFIER_MODEL", "deepseek-v4-flash")
VERIFIER_POOL = (os.environ.get("VERIFIER_POOL",
    "deepseek-v4-flash glm-5.2 sensenova-6.8-flash-lite longcat-2.5-preview")
    .split())


def verifier_candidates(impl_models: set[str]) -> list[str]:
    """按优先级返回可用的验证者模型（排除当前所有实现者模型）。

    全撞车时退回 VERIFIER_MODEL（宁可自审提示也别一个都不试——真正的自审
    判定在 verify_one 里，这里只负责给顺序）。
    """
    cands = [m for m in VERIFIER_POOL if m not in impl_models]
    return cands or [VERIFIER_MODEL]


def pick_verifier(impl_models: set[str]) -> str:
    """挑首选验证者（保留给日志/兼容用）。"""
    return verifier_candidates(impl_models)[0]

MAX_FILE_CHARS = 12000     # 单文件送审上限，避免撑爆上下文
MAX_FILES = 3              # 单任务最多审几个文件


def _function_span(lines: list[str], name: str, max_lines: int = 120) -> tuple[int, int] | None:
    """在 Go/JS 源码里按函数名找到**整个函数体**的行范围。

    为什么不能只用固定窗口：`i-8 .. i+60` 必然把同文件相邻的函数一起带进去，
    审查者分不清哪段属于目标函数，就会拿别人的错误码/字段名判目标函数不合规
    （p50 连续两次误判都是这个原因）。

    只做括号配平，不引 parser：送审用，够准就行，错了也只是回到固定窗口。
    """
    pat = re.compile(r"^\s*(?:func\s+(?:\([^)]*\)\s*)?|(?:export\s+)?(?:async\s+)?function\s+)"
                     + re.escape(name) + r"\s*\(")
    arrow = re.compile(r"^\s*(?:const|let|var)\s+" + re.escape(name) + r"\s*=")
    for i, line in enumerate(lines):
        if not (pat.search(line) or arrow.search(line)):
            continue
        depth = 0
        started = False
        for j in range(i, min(len(lines), i + max_lines)):
            depth += lines[j].count("{") - lines[j].count("}")
            if "{" in lines[j]:
                started = True
            if started and depth <= 0:
                return (i, j + 1)
        return (i, min(len(lines), i + max_lines))
    return None


def extract_for_review(p: Path, limit: int = MAX_FILE_CHARS,
                       keywords: list[str] | None = None) -> str:
    """按文件类型抽取送审内容。

    三次实测教训，全部是同一个根因 —— **验证者看不到就等于不存在**：
    1. 按 6000 字符从头截断 LinkCheck.vue（227 行），只截到模板，看不到
       saveAll/probe/removeTarget → 判「缺少核心逻辑」，误判。
    2. 改成只给 <script> 段后，又看不到模板里的统计卡片 → 判「需要模板展示」，仍误判。
    3. p15.go（456 行 / 15,321 字符）关键 handler 在 243/264 行，正好落在
       「头一半 + 尾一半」的中间空洞里 → 判「函数列表无 handleGetTimeSchedule」，还是误判。

    所以策略是：关键词命中函数名时**只送那几个函数体**；否则能全文就给全文；
    超长时按 keywords 定位窗口，而不是盲目截头截尾。
    """
    text = p.read_text(encoding="utf-8", errors="replace")
    lines = text.split("\n")

    # 策略 0（最高优先）：关键词是函数名 → 只送函数体，不论文件多小。
    #
    # 为什么小文件也要切：p16.go 只有 9KB，整份送进去，里面十个 handler 各自的
    # 错误码混在一起，审查者看到 10480/10481 就判"delete handler 没用 10482/10483"
    # —— 连续两次误判同一个任务。**"文件不长"不等于"审查者分得清"**。
    if keywords:
        fn_spans = [s for s in (_function_span(lines, kw) for kw in keywords) if s]
        if fn_spans:
            fn_spans.sort()
            out, used = [], 0
            header = f"// {p.name}（仅送审指定函数，同文件其它函数未提供）\n"
            out.append(header)
            used += len(header)
            for lo, hi in fn_spans:
                chunk = "\n".join(lines[lo:hi])
                if used + len(chunk) > limit:
                    chunk = chunk[: max(0, limit - used)]
                if chunk:
                    out.append(f"// --- 第 {lo + 1}-{hi} 行 ---\n{chunk}")
                    used += len(chunk)
                if used >= limit:
                    break
            return "\n".join(out)

    if len(text) <= limit:
        return text

    # 策略 A：用任务声明的关键符号定位。
    #
    # 优先按**函数名精确切片**：`window = i-8 .. i+60` 会把同文件里相邻的
    # 其它 handler 一起送进去，审查者分不清哪段属于这个函数 —— 实测 p50 被
    # 判「用了 10480/10481 而不是 10482/10483」，其实那是同文件另一个 handler
    # 的错误码，delete handler 用的正是 10482/10483。切片能根治这类误判。
    if keywords:
        spans: list[tuple[int, int]] = []
        for kw in keywords:
            fn_span = _function_span(lines, kw)
            if fn_span:
                spans.append(fn_span)
                continue
            for i, line in enumerate(lines):
                if kw in line:
                    spans.append((max(0, i - 8), min(len(lines), i + 60)))
                    # 关键词常常是路由字符串（"PUT /api/v1/users/{username}"），
                    # 命中的那一行只是注册处，真正的实现可能在几千行之外。
                    # 实测 p81：审查者只看到路由行就判「handleUpdateUser 未实现」，
                    # 而它就在同文件 3411 行。沿路由把被引用的 handler 一起带上。
                    for h in re.findall(r"s\.(handle[A-Za-z0-9_]+)", line):
                        span = _function_span(lines, h)
                        if span:
                            spans.append(span)
                    break
        if spans:
            spans.sort()
            merged: list[list[int]] = []
            for lo, hi in spans:
                if merged and lo <= merged[-1][1]:
                    merged[-1][1] = max(merged[-1][1], hi)
                else:
                    merged.append([lo, hi])
            out, used = [], 0
            for lo, hi in merged:
                chunk = "\n".join(lines[lo:hi])
                if used + len(chunk) > limit:
                    chunk = chunk[: max(0, limit - used)]
                if chunk:
                    out.append(f"// --- 第 {lo + 1}-{hi} 行（含关键符号）---\n{chunk}")
                    used += len(chunk)
                if used >= limit:
                    break
            if out:
                return "\n\n".join(out)

    # 策略 B：.vue 模板与 script 各给一半
    if p.suffix == ".vue":
        head = text[: limit // 2]
        m = re.search(r"<script[^>]*>(.*?)</script>", text, re.S)
        if m:
            return f"{head}\n\n// ... [模板截断] ...\n\n// [<script> 段]\n{m.group(1)[: limit // 2]}"
        return head

    # 策略 C：头 + 尾
    half = limit // 2
    return (text[:half]
            + f"\n\n// ... [中间省略 {len(text) - limit} 字符] ...\n\n"
            + text[-half:])

# 空壳特征分两级。**误报比漏报更糟**：一个假 FAIL 会让人去"修"本来正确的代码。
#
# 硬空壳（判 FAIL）：这些只可能是"根本没做"
HARD_STUB_PATTERNS = [
    (r'panic\(\s*["\']not implemented', "panic not implemented"),
    (r'panic\(\s*["\']TODO', "panic TODO"),
    (r"\bnot\s+implemented\b", "not implemented"),
    (r"return\s+nil\s*//\s*(stub|placeholder|todo|not impl)", "return nil 占位"),
    (r"待实现|未实现|尚未实现", "中文待实现"),
    (r"TODO:\s*implement", "TODO: implement"),
]

# 软提示（只记录、不判 FAIL）：合法代码里也会有 TODO，判死会误杀
SOFT_STUB_PATTERNS = [
    (r"\bTODO\b", "TODO"),
    (r"\bFIXME\b", "FIXME"),
    (r"\bXXX\b", "XXX"),
]

# 注释语境下的 placeholder 才算空壳。
# 实测踩坑：裸匹配 `placeholder` 把 Vue 的 <el-input placeholder="模板名称">
# 全判成空壳（AuthPortal.vue 命中 5 处、LinkCheck.vue 命中 3 处），全是合法属性。
COMMENT_STUB_PATTERNS = [
    (r"(?://|#|/\*)\s*placeholder\b", "注释里的 placeholder"),
]


def log(msg: str) -> None:
    LOG.parent.mkdir(parents=True, exist_ok=True)
    line = f"[{time.strftime('%F %T')}] {msg}"
    print(line)
    with LOG.open("a", encoding="utf-8") as f:
        f.write(line + "\n")


def notify_pane(pane: str, msg: str) -> None:
    """往工人 pane 回贴一句话（坚哥在 konsole 对话框里直接看到进展）。

    失败只记日志不阻塞验证 —— 通知挂了不能让验证停摆。
    """
    if not pane:
        return
    try:
        r = subprocess.run(
            [HERDR, "agent", "prompt", pane, msg],
            capture_output=True, text=True, timeout=30,
        )
    except Exception as exc:  # noqa: BLE001
        log(f"WARN 回贴 {pane} 抛异常: {exc}")
        return
    if r.returncode != 0:
        log(f"WARN 回贴 {pane} rc={r.returncode}: {(r.stderr or r.stdout)[:120]}")


def pane_model(pane: str) -> str:
    """从 pane 尾部状态栏实时读实现者模型（☤ 后面的词）。

    读不到返回空字符串，调用方按"未知"处理（不判自审，只是不做自审保护）。
    宁可漏保护一次，不可把异模型审误判成自审而 SKIP —— 后者会让验证
    coverage 悄悄归零还不报任何错。
    """
    if not pane:
        return ""
    try:
        out = subprocess.run(
            [HERDR, "agent", "read", pane, "--lines", "3"],
            capture_output=True, text=True, timeout=15,
        ).stdout
    except Exception:  # noqa: BLE001
        return ""
    m = re.search(r"☤\s*([A-Za-z0-9._-]+)", out)
    return m.group(1) if m else ""


def load_queue() -> list[dict]:
    if not QUEUE.exists():
        return []
    out = []
    for line in QUEUE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def save_queue(tasks: list[dict]) -> None:
    tmp = QUEUE.with_suffix(".jsonl.tmp")
    tmp.write_text("\n".join(json.dumps(t, ensure_ascii=False) for t in tasks) + "\n",
                   encoding="utf-8")
    os.replace(tmp, QUEUE)


def load_key() -> str:
    for p in ("/home/jimwong/.gwkey", "/home/jimwong/.hermes/cache/scratch/.gwkey"):
        if os.path.exists(p):
            raw = open(p, encoding="utf-8").read().strip()
            if "=" in raw.split("\n")[0]:
                raw = raw.split("\n")[0].split("=", 1)[1]
            if raw.startswith("sk-"):
                return raw.strip()
    for cf in ("/home/jimwong/.hermes/config.yaml", "/home/jimwong/.hermes/.env"):
        if os.path.exists(cf):
            m = re.search(r"sk-[A-Za-z0-9]{20,}", open(cf, encoding="utf-8", errors="replace").read())
            if m:
                return m.group(0)
    return ""


def static_check(paths: list[Path]) -> tuple[list[str], list[str]]:
    """静态空壳检测。返回 (硬问题, 软提示)。

    硬问题 → FAIL；软提示只记录，不影响判定。分级的原因：TODO 在正常代码里
    也会出现（"// TODO: 支持 IPv6"），拿它判死会把完成度高的产出误杀掉。
    """
    hard, soft = [], []
    # expect 里可以写目录（如 mgmtd/internal/ha/）——曾经直接 read_text 会报
    # "Is a directory" 判成空壳，把做完的任务连判 3 次 giveup。目录要展开。
    expanded: list[Path] = []
    for p in paths:
        if p.is_dir():
            expanded.extend(sorted(x for x in p.rglob("*") if x.is_file()))
        else:
            expanded.append(p)
    for p in expanded:
        if p.suffix in (".md", ".txt", ".json"):
            continue
        try:
            text = p.read_text(encoding="utf-8", errors="replace")
        except OSError as e:
            hard.append(f"{p.name}: 读不了 ({e})")
            continue
        for pat, label in HARD_STUB_PATTERNS + COMMENT_STUB_PATTERNS:
            m = re.search(pat, text, re.IGNORECASE)
            if m:
                line_no = text[:m.start()].count("\n") + 1
                hard.append(f"{p.name}:{line_no} 命中空壳标记 [{label}]")
        for pat, label in SOFT_STUB_PATTERNS:
            m = re.search(pat, text)
            if m:
                line_no = text[:m.start()].count("\n") + 1
                soft.append(f"{p.name}:{line_no} {label}")
    return hard, soft


def route_dup_check() -> list[str]:
    """扫描 server.go 里重复注册的路由。

    为什么单独查：Go 的 ServeMux 在重复 pattern 时是**启动 panic**，编译和
    单测都可能照样绿（测试如果没走到完整注册路径就发现不了）。p114 的返工
    就往 server.go 里塞了两份 /api/v1/ha/config，肉眼扫过三次才看出来。
    """
    p = ROOT / "mgmtd/internal/server/server.go"
    if not p.exists():
        return []
    try:
        text = p.read_text(encoding="utf-8", errors="replace")
    except OSError:
        return []
    pats = re.findall(r'"((?:GET|POST|PUT|DELETE|PATCH) /api/v1/[^"]+)"', text)
    seen, dups = set(), []
    for pat in pats:
        if pat in seen and pat not in dups:
            dups.append(pat)
        seen.add(pat)
    return [f"重复注册路由 {d}（ServeMux 启动会 panic）" for d in dups]


def build_check() -> tuple[bool, str]:
    """编译检查。返回 (是否通过, 输出摘要)。"""
    try:
        r = subprocess.run(["go", "build", "./..."], cwd=ROOT / MUST_BUILD_DIR,
                           capture_output=True, text=True, timeout=240)
        if r.returncode == 0:
            return True, "build ok"
        return False, (r.stderr or r.stdout).strip()[:400]
    except FileNotFoundError:
        return True, "skip: go 不在 PATH"
    except subprocess.TimeoutExpired:
        return False, "build 超时 240s"


def call_verifier(key: str, prompt: str, model: str, timeout: int = 120) -> str:
    body = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": 1000,
        "temperature": 0,
    }).encode()
    req = urllib.request.Request(GATEWAY, data=body, headers={
        "Authorization": "Bearer " + key,
        "Content-Type": "application/json",
    })
    with urllib.request.urlopen(req, timeout=timeout) as r:
        d = json.load(r)
    return (d.get("choices") or [{}])[0].get("message", {}).get("content", "")


def llm_review(key: str, task: dict, files: list[tuple[Path, str]], model: str,
               evidence: list[tuple[Path, str]] | None = None) -> tuple[bool | None, str]:
    """让非实现者模型审产出。返回 (是否通过, 说明)。

    evidence 是产出附带的可执行证据（脚本跑出来的 .md 报告等）。审查者跑不了
    代码，只能读，所以"有没有真的验证过"必须靠证据文件说话；不给它证据，
    它会按"只看到代码、没看到运行结果"判 FAIL（p34 就这么被误判过）。
    """
    requirement = (task.get("prompt") or "")[:1500]
    body_parts = []
    for p, text in files:
        body_parts.append(f"### 文件 `{p.relative_to(ROOT)}`\n```\n{text}\n```")
    evidence_parts = []
    for p, text in (evidence or []):
        evidence_parts.append(f"### 证据 `{p.relative_to(ROOT)}`（产出脚本的真实运行记录）\n```\n{text}\n```")
    evidence_block = ""
    if evidence_parts:
        evidence_block = ("\n\n以下是产出附带的运行证据（脚本真实跑出来的报告），"
                          "判断「是否真的实现/验证过」以它为准：\n" + "".join(evidence_parts))

    # 用 JSON 输出，不用自由文本。
    # 实测教训：要求"只输出三行（PASS/FAIL + 理由 + 证据）"时，模型会把
    # prompt 里的"第一步：抄出函数名"复述成第一行，解析器拿到"第一步…"
    # 不以 PASS 开头就判 FAIL —— 一次全量重审凭空造出 5 个假 FAIL，
    # 而且同一任务前后结果不一致（p21-fwlog-wire 上次 PASS 这次 FAIL）。
    prompt = f"""你是代码审查员。判断下面的产出是否**真的实现了任务要求**。

任务要求：
{requirement}

实际产出：
{"".join(body_parts)}{evidence_block}

分析要点（内部思考，**不要把这些步骤写进输出**）：
1. 产出里实际有哪些函数名、哪些 API 调用；
2. 对照任务要求的每个功能点，找对应实现；
3. 函数名可能与要求不同（"删除"可能叫 removeTarget、"保存"可能叫 saveAll），按**行为**判断，不按字面命名;
4. 只有"要求里有、产出里确实找不到"的项才算缺失。
5. 如果给了运行证据，说明脚本已经被真实执行过并产出了结果——此时**不要**以
   "没看到运行结果"为由判 FAIL，证据里的数据就是运行结果。

判 FAIL 的门槛是"核心功能确实没实现"，不是命名不合预期或代码风格不同。

**只输出一个 JSON 对象，不要任何其他文字、不要 markdown 代码块、不要解释过程**：
{{"verdict": "PASS 或 FAIL", "reason": "一句话中文理由，不超过60字", "evidence": "你实际看到的关键函数名，逗号分隔"}}"""

    out = call_verifier(key, prompt, model)
    return parse_verdict(out)


def parse_verdict(out: str) -> tuple[bool | None, str]:
    """从验证者输出里解析判决。容错优先 —— 解析失败不能当成 FAIL。"""
    text = out.strip()
    # 去掉可能的 ```json 围栏
    text = re.sub(r"^```(?:json)?\s*|\s*```$", "", text, flags=re.M).strip()

    verdict = None
    reason = ""
    evidence = ""

    # 优先按 JSON 解析
    m = re.search(r"\{.*\}", text, re.S)
    if m:
        try:
            d = json.loads(m.group(0))
            verdict = str(d.get("verdict", "")).upper()
            reason = str(d.get("reason", ""))[:200]
            evidence = str(d.get("evidence", ""))[:200]
        except json.JSONDecodeError:
            pass

    # 兜底 1：JSON 被截断时，直接从片段里抠字段。
    # 实测：{"verdict":"PASS","reason":"所有要求功能均已实现... 被 max_tokens 截断后
    # json.loads 失败，若不兜底会误判成"无法解析"。
    if verdict not in ("PASS", "FAIL"):
        m2 = re.search(r'"verdict"\s*:\s*"?(PASS|FAIL)', text, re.I)
        if m2:
            verdict = m2.group(1).upper()
            mr = re.search(r'"reason"\s*:\s*"([^"]*)', text)
            if mr:
                reason = mr.group(1)[:200]
            me = re.search(r'"evidence"\s*:\s*"([^"]*)', text)
            if me:
                evidence = me.group(1)[:200]

    # 兜底 2：扫独立行找 PASS/FAIL
    if verdict not in ("PASS", "FAIL"):
        for line in text.split("\n"):
            s = line.strip().strip("*# ").strip()
            if s.upper().startswith("PASS"):
                verdict = "PASS"; break
            if s.upper().startswith("FAIL"):
                verdict = "FAIL"; break

    # 仍无法判断 -> 视为无法验证（skip），绝不默认 FAIL
    if verdict not in ("PASS", "FAIL"):
        return None, f"无法解析验证输出: {text[:120]}"

    if evidence:
        reason = f"{reason} | 证据: {evidence}"
    return verdict == "PASS", reason


def verify_one(task: dict, key: str, dry: bool, verifier: str,
               candidates: list[str] | None = None) -> str:
    """返回 pass / fail / skip。verifier 首选验证者，candidates 是完整候选顺序。"""
    cands = candidates or [verifier]
    verifier = cands[0]
    tid = task["id"]
    expect = task.get("expect") or []
    paths = [ROOT / p for p in expect]
    missing = [str(p.relative_to(ROOT)) for p in paths if not p.exists()]
    if missing:
        log(f"FAIL  {tid}  产出文件缺失: {', '.join(missing)}")
        return "fail"

    # expect 允许写目录（如 mgmtd/internal/ha/）：在源头展开成文件，否则下游
    # 三个消费者（空壳检测/送审抽取/must_contain）都会把目录当文件读而误判。
    expanded: list[Path] = []
    for p in paths:
        if p.is_dir():
            expanded.extend(sorted(x for x in p.rglob("*.go") if x.is_file()))
        else:
            expanded.append(p)
    paths = expanded

    # expect 空 = 运维类任务（verify_cmd 验收）：约定与 produced() 一致，
    # verify_cmd 跑过 rc=0 才能 done，而 done 的前提就是 verify_cmd 已过，
    # 所以这里直接 pass。没有 verify_cmd 也没有 expect 的任务调度器根本
    # 不会标 done，流不到这里——真流到了说明是手工改的，判 skip 警示。
    if not expect:
        if task.get("verify_cmd"):
            log(f"PASS  {tid}  运维类任务，verify_cmd 已过（调度器结算时执行）")
            task["verify_notes"] = "运维：verify_cmd 通过"
            return "pass"
        log(f"SKIP  {tid}  无 expect 也无 verify_cmd，无法验证")
        return "skip"

    # 断言验证者与实现者不同。
    # 实现者模型从 pane 尾部状态栏实时读（☤ <model>），不写死映射 ——
    # pane 的模型会换（如下午 pM 从 beam-free 切到 deepseek），写死的表一过期
    # 就会把"异模型审"误判成自审而 SKIP，等于验证 coverage 悄悄归零。
    impl_model = pane_model(task.get("pane", ""))
    if impl_model and impl_model == verifier:
        log(f"SKIP  {tid}  验证者与实现者同为 {impl_model}，等于自审，跳过")
        return "skip"

    # 1) 静态空壳检测（硬问题判 FAIL，软提示只记录）
    hard, soft = static_check(paths)
    # 1b) 路由重复注册（ServeMux 启动 panic，编译/单测都可能漏）
    hard += route_dup_check()
    if soft:
        log(f"NOTE  {tid}  软提示（不影响判定）: {'; '.join(soft[:3])}")
    if hard:
        log(f"FAIL  {tid}  空壳检测未过: {'; '.join(hard[:3])}")
        task["verify_notes"] = "空壳: " + "; ".join(hard[:3])
        return "fail"

    # 2) 编译
    ok, detail = build_check()
    if not ok:
        # 归因：并行开发时兄弟任务正在编辑别的文件，全局 build 会红。
        # 只要报错文件都不是本任务的产出，就不能判本任务死（曾经 p112/p114
        # 因为 p113 的 sms.go 半成品被连判 3 次 FAIL/giveup）。
        bad = set(re.findall(r"([\w./\-]+\.go):\d+:\d+:", detail))
        own = {str(p.relative_to(ROOT)) for p in paths}
        if bad and not (bad & own):
            log(f"SKIP  {tid}  编译错误来自其它任务文件 {sorted(bad)[:2]}，本次不判死")
            task["verify_notes"] = "推迟: 兄弟任务编译中间态 " + ", ".join(sorted(bad)[:3])
            return "skip"
        log(f"FAIL  {tid}  编译未过: {detail[:150]}")
        task["verify_notes"] = "编译: " + detail[:200]
        return "fail"

    # 3) 模型审查（只审代码类产出；纯文档跳过 LLM 审，静态即可）
    must_contain = task.get("must_contain") or {}
    code_files: list[tuple[Path, str]] = []
    evidence_files: list[tuple[Path, str]] = []
    for p in paths:
        rel = str(p.relative_to(ROOT))
        if p.suffix not in (".go", ".py", ".js", ".ts", ".vue"):
            # 非代码产出（如脚本跑出来的报告）当**证据**送给审查者。
            # 实测教训：p34 审的是「脚本+它的运行报告」，报告里明明写着真实
            # 重跑结果 24 OK/9 SKIP/0 FAIL，但旧逻辑只送 .py，审查者看不到
            # 证据就判「未真实运行脚本，无实际输出和报告」→ 假 FAIL。
            if p.suffix in (".md", ".txt", ".json", ".log"):
                try:
                    evidence_files.append((p, p.read_text(encoding="utf-8", errors="replace")[:4000]))
                except OSError:
                    pass
            continue
        # 把任务声明的关键符号传给抽取器：超长文件按它定位，避免关键 handler
        # 落在「头一半+尾一半」的中间空洞里被漏掉（p15.go 就是这么误判的）
        code_files.append((p, extract_for_review(p, keywords=must_contain.get(rel) or [])))
    code_files = code_files[:MAX_FILES]
    if not code_files:
        # 区分"真的是文档任务"和"只声明了一个哨兵文件"。
        # 后者（expect 只有 .pXX-done 这类标记）等于没有任何可审产出：既没有
        # 代码也没有报告，直接判 pass 就是免检。实测 p50/p74 都栽在这条上，日志
        # 写着"静态检查通过（未做语义审）"却计入 pass。
        # 这种情况给 skip 而不是 pass —— skip 会在汇总里显形，pass 不会。
        real_outputs = [p for p in paths if not p.name.startswith(".")]
        if not real_outputs:
            log(f"SKIP  {tid}  expect 里只有哨兵文件，没有可审产出（不能算通过）")
            task["verify_notes"] = ("任务只声明了标记文件，没有代码/报告可审；"
                                    "请给任务补 expect 指向真实产出，或人工核验后标 pass-manual")
            return "skip"
        log(f"PASS  {tid}  文档类产出，静态检查通过（未做语义审）")
        task["verify_notes"] = "文档：静态通过"
        return "pass"

    if dry:
        log(f"DRY   {tid}  将送审 {len(code_files)} 个代码文件给 {verifier}")
        return "skip"

    # 网关会间歇性 500 / 连接重置（尤其验证请求也走同一个池子），
    # 一次失败就 SKIP 会让验证覆盖率虚高，必须退避重试。
    #
    # 而且**必须换模型重试**：实测网关上游会整段耗尽配额，deepseek/glm/sensenova
    # 在 1.5K token 以上全部 429 insufficient_quota（小请求反而通过），只有
    # longcat 能吃完审查 prompt。同一个模型重试三次毫无意义，等于白白 SKIP。
    # 所以外层遍历候选模型，内层每个模型试 2 次。
    passed: bool | None = None
    reason = ""
    last_err: Exception | None = None
    used_model = verifier
    for model in cands:
        if model != verifier:
            log(f"SWITCH {tid}  换验证者 {verifier} -> {model}（前一个不可用）")
        for attempt in range(2):
            try:
                passed, reason = llm_review(key, task, code_files, model, evidence_files)
                last_err = None
                used_model = model
                if passed is None:
                    # 解析失败也要换下一个模型：实测 sensenova 会回散文
                    # （"我们只需要输出JSON。让我分析…"），此时不该直接 SKIP。
                    log(f"PARSE {tid}  [{model}] 输出无法解析，换下一个验证者")
                    continue
                break
            except Exception as e:  # noqa: BLE001
                last_err = e
                log(f"RETRY {tid}  [{model}] 调用失败({attempt + 1}/2): {type(e).__name__}")
                time.sleep(3 * (attempt + 1))
        if last_err is None and passed is not None:
            verifier = used_model
            break
        if passed is None:
            last_err = last_err or RuntimeError("所有候选都返回无法解析的输出")
    if last_err is not None:
        log(f"SKIP  {tid}  全部 {len(cands)} 个验证者候选都失败: "
            f"{type(last_err).__name__}: {last_err}")
        return "skip"

    if passed is None:
        log(f"SKIP  {tid}  验证输出无法解析，不计入 pass/fail: {reason[:100]}")
        return "skip"

    verdict = "PASS" if passed else "FAIL"
    log(f"{verdict}  {tid}  [{used_model} 审] {reason}")
    task["verify_notes"] = reason
    task["verified_by"] = used_model
    return "pass" if passed else "fail"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--id", default="", help="只验证指定任务")
    ap.add_argument("--recheck", action="store_true", help="已 verified 的也重审")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    tasks = load_queue()
    if not tasks:
        print("队列为空")
        return 0

    key = "" if args.dry_run else load_key()
    if not args.dry_run and not key:
        print("缺少网关 key，无法做语义审查（可用 --dry-run）")
        return 1

    targets = []
    for t in tasks:
        if t.get("status") != "done":
            continue
        if args.id and t.get("id") != args.id:
            continue
        if not args.recheck and t.get("verified"):
            # verified="pass-manual" = 人工核验通过但未做 LLM 语义审（当时网关
            # 不可用）。这类任务要在网关回血后自动升级为 LLM 审，不能跳过。
            if t.get("verified") != "pass-manual":
                continue
        targets.append(t)

    if not targets:
        print("没有需要验证的任务")
        return 0

    # 动态挑验证者：收集当前所有工人 pane 的实现者模型，从池里挑一个不同
    # 的。否则工人全用 deepseek-v4-flash 时固定验证者也撞车 → 全 SKIP，验证
    # 覆盖率归零（今天实测就是这个）。pane 集合从队列里 done 任务的 pane 字段
    # 汇总 + PANES 环境变量（若配了）并集，保证不漏。
    impl_models: set[str] = set()
    panes = {t.get("pane", "") for t in tasks if t.get("pane")}
    for raw in os.environ.get("PANES", "").split():
        panes.add(raw)
    for p in panes:
        m = pane_model(p)
        if m:
            impl_models.add(m)
    cands = verifier_candidates(impl_models)
    verifier = cands[0]
    if impl_models:
        log(f"实现者模型 {sorted(impl_models)} → 验证者候选 {cands}")
    else:
        log(f"读不到实现者模型，验证者候选 {cands}")

    counts = {"pass": 0, "fail": 0, "skip": 0}
    for t in targets:
        verdict = verify_one(t, key, args.dry_run, verifier, cands)
        counts[verdict] += 1
        if not args.dry_run and verdict in ("pass", "fail"):
            t["verified"] = verdict
            t["verified_at"] = time.time()
            notes = t.get("verify_notes") or ""

            if verdict == "fail":
                # 自动返工：把任务放回队列，并把失败原因顶到 prompt 最前面。
                # 只贴一条消息给工人是不够的——它接着领的是**下一条新任务**，
                # 旧任务就永远停在 fail 等人来 --recheck。闭环应该是：没过 → 重派。
                retries = (t.get("retries") or 0) + 1
                max_retries = t.get("max_retries") or 3
                if retries <= max_retries:
                    base = t.get("prompt") or ""
                    marker = "【上一轮验证未通过，本次必修】"
                    if marker in base:      # 避免多轮返工把 prompt 越堆越长
                        base = base.split(marker, 1)[0]
                    rework = (f"{marker}{notes[:500]}\n"
                              f"（这是第 {retries}/{max_retries} 次返工。先按上面的原因修，"
                              f"再确认原任务要求全部满足。）\n\n")
                    ql.update_task(
                        t["id"], verified=None, verified_at=None, verify_notes=notes,
                        verified_by=t.get("verified_by"),
                        status="pending", pane=None, retries=retries,
                        retry_after=time.time() + 10,
                        last_fail_reason=notes[:300],
                        prompt=rework + base,
                    )
                    counts["rework"] = counts.get("rework", 0) + 1
                    log(f"REWORK {t['id']} 验证没过，已放回队列重派（第 {retries}/{max_retries} 次）")
                    notify_pane(t.get("pane", ""), f"❌ {t['id']} 验证没过：{notes[:200]}")
                    time.sleep(2)
                    continue
                log(f"GIVEUP {t['id']} 已返工 {max_retries} 次仍不过，留给人看")
                notify_pane(t.get("pane", ""), f"⚠️ {t['id']} 返工 {max_retries} 次仍不过，需人工介入：{notes[:160]}")

            # 只改这一条，不能整表回写：lane 进程会并发改别的任务，
            # 用开头读到的那份快照整体覆盖会把它们的 dispatched 状态写回去
            #（等于任务被解除占用、被第二个工人重抢）。走 queue_lib 的锁。
            ql.update_task(t["id"], verified=verdict, verified_at=t["verified_at"],
                           verify_notes=notes, verified_by=t.get("verified_by"))
            # 工人对话框里能看到验证结论
            pane = t.get("pane", "")
            if verdict == "pass":
                notify_pane(pane, f"✅ {t['id']} 独立验证通过（verified=pass）")
        time.sleep(2)   # 低频，别把验证本身变成限流源

    log(f"VERIFY 结束 pass={counts['pass']} fail={counts['fail']} "
        f"skip={counts['skip']} rework={counts.get('rework', 0)}")

    if counts["fail"]:
        log("有任务验证失败，见上；修正后可用 --id <id> --recheck 重审")
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
