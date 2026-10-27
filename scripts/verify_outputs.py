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

ROOT = Path("/home/jimwong/projects/router-re")
QUEUE = ROOT / "ops" / "tasks.jsonl"
LOG = ROOT / "logs" / "verify.log"
GATEWAY = os.environ.get("GATEWAY_URL", "http://10.100.1.17:8085/v1/chat/completions")
HERDR = shutil.which("herdr") or str(Path.home() / ".local/bin/herdr")

# 验证者模型必须与实现者不同，否则等于自审。
# 工人主力是 beam-free（opencode-zen 免费层），所以验证者默认用 deepseek-v4-flash。
VERIFIER_MODEL = os.environ.get("VERIFIER_MODEL", "deepseek-v4-flash")

MAX_FILE_CHARS = 12000     # 单文件送审上限，避免撑爆上下文
MAX_FILES = 3              # 单任务最多审几个文件


def extract_for_review(p: Path, limit: int = MAX_FILE_CHARS,
                       keywords: list[str] | None = None) -> str:
    """按文件类型抽取送审内容。

    三次实测教训，全部是同一个根因 —— **验证者看不到就等于不存在**：
    1. 按 6000 字符从头截断 LinkCheck.vue（227 行），只截到模板，看不到
       saveAll/probe/removeTarget → 判「缺少核心逻辑」，误判。
    2. 改成只给 <script> 段后，又看不到模板里的统计卡片 → 判「需要模板展示」，仍误判。
    3. p15.go（456 行 / 15,321 字符）关键 handler 在 243/264 行，正好落在
       「头一半 + 尾一半」的中间空洞里 → 判「函数列表无 handleGetTimeSchedule」，还是误判。

    所以策略是：能全文就给全文；超长时**优先按 keywords（任务的 must_contain）
    定位关键代码周围的窗口**，而不是盲目截头截尾。
    """
    text = p.read_text(encoding="utf-8", errors="replace")
    if len(text) <= limit:
        return text

    lines = text.split("\n")

    # 策略 A：用任务声明的关键符号定位，取每个符号附近一整段函数体
    if keywords:
        spans: list[tuple[int, int]] = []
        for kw in keywords:
            for i, line in enumerate(lines):
                if kw in line:
                    spans.append((max(0, i - 8), min(len(lines), i + 60)))
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
    for p in paths:
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


def build_check() -> tuple[bool, str]:
    """编译检查。返回 (是否通过, 输出摘要)。"""
    try:
        r = subprocess.run(["go", "build", "./..."], cwd=ROOT / "mgmtd",
                           capture_output=True, text=True, timeout=240)
        if r.returncode == 0:
            return True, "build ok"
        return False, (r.stderr or r.stdout).strip()[:400]
    except FileNotFoundError:
        return True, "skip: go 不在 PATH"
    except subprocess.TimeoutExpired:
        return False, "build 超时 240s"


def call_verifier(key: str, prompt: str, timeout: int = 120) -> str:
    body = json.dumps({
        "model": VERIFIER_MODEL,
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


def llm_review(key: str, task: dict, files: list[tuple[Path, str]]) -> tuple[bool | None, str]:
    """让非实现者模型审产出。返回 (是否通过, 说明)。"""
    requirement = (task.get("prompt") or "")[:1500]
    body_parts = []
    for p, text in files:
        body_parts.append(f"### 文件 `{p.relative_to(ROOT)}`\n```\n{text}\n```")

    # 用 JSON 输出，不用自由文本。
    # 实测教训：要求"只输出三行（PASS/FAIL + 理由 + 证据）"时，模型会把
    # prompt 里的"第一步：抄出函数名"复述成第一行，解析器拿到"第一步…"
    # 不以 PASS 开头就判 FAIL —— 一次全量重审凭空造出 5 个假 FAIL，
    # 而且同一任务前后结果不一致（p21-fwlog-wire 上次 PASS 这次 FAIL）。
    prompt = f"""你是代码审查员。判断下面的产出是否**真的实现了任务要求**。

任务要求：
{requirement}

实际产出：
{"".join(body_parts)}

分析要点（内部思考，**不要把这些步骤写进输出**）：
1. 产出里实际有哪些函数名、哪些 API 调用；
2. 对照任务要求的每个功能点，找对应实现；
3. 函数名可能与要求不同（"删除"可能叫 removeTarget、"保存"可能叫 saveAll），按**行为**判断，不按字面命名;
4. 只有"要求里有、产出里确实找不到"的项才算缺失。

判 FAIL 的门槛是"核心功能确实没实现"，不是命名不合预期或代码风格不同。

**只输出一个 JSON 对象，不要任何其他文字、不要 markdown 代码块**：
{{"verdict": "PASS 或 FAIL", "reason": "一句话中文理由，不超过60字", "evidence": "你实际看到的关键函数名，逗号分隔"}}"""

    out = call_verifier(key, prompt)
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


def verify_one(task: dict, key: str, dry: bool) -> str:
    """返回 pass / fail / skip。"""
    tid = task["id"]
    expect = task.get("expect") or []
    paths = [ROOT / p for p in expect]
    missing = [str(p.relative_to(ROOT)) for p in paths if not p.exists()]
    if missing:
        log(f"FAIL  {tid}  产出文件缺失: {', '.join(missing)}")
        return "fail"

    # 断言验证者与实现者不同。
    # 实现者模型从 pane 尾部状态栏实时读（☤ <model>），不写死映射 ——
    # pane 的模型会换（如下午 pM 从 beam-free 切到 deepseek），写死的表一过期
    # 就会把"异模型审"误判成自审而 SKIP，等于验证 coverage 悄悄归零。
    impl_model = pane_model(task.get("pane", ""))
    if impl_model and impl_model == VERIFIER_MODEL:
        log(f"SKIP  {tid}  验证者与实现者同为 {impl_model}，等于自审，跳过")
        return "skip"

    # 1) 静态空壳检测（硬问题判 FAIL，软提示只记录）
    hard, soft = static_check(paths)
    if soft:
        log(f"NOTE  {tid}  软提示（不影响判定）: {'; '.join(soft[:3])}")
    if hard:
        log(f"FAIL  {tid}  空壳检测未过: {'; '.join(hard[:3])}")
        task["verify_notes"] = "空壳: " + "; ".join(hard[:3])
        return "fail"

    # 2) 编译
    ok, detail = build_check()
    if not ok:
        log(f"FAIL  {tid}  编译未过: {detail[:150]}")
        task["verify_notes"] = "编译: " + detail[:200]
        return "fail"

    # 3) 模型审查（只审代码类产出；纯文档跳过 LLM 审，静态即可）
    must_contain = task.get("must_contain") or {}
    code_files: list[tuple[Path, str]] = []
    for p in paths:
        if p.suffix not in (".go", ".py", ".js", ".ts", ".vue"):
            continue
        rel = str(p.relative_to(ROOT))
        # 把任务声明的关键符号传给抽取器：超长文件按它定位，避免关键 handler
        # 落在「头一半+尾一半」的中间空洞里被漏掉（p15.go 就是这么误判的）
        code_files.append((p, extract_for_review(p, keywords=must_contain.get(rel) or [])))
    code_files = code_files[:MAX_FILES]
    if not code_files:
        log(f"PASS  {tid}  文档类产出，静态检查通过（未做语义审）")
        task["verify_notes"] = "文档：静态通过"
        return "pass"

    if dry:
        log(f"DRY   {tid}  将送审 {len(code_files)} 个代码文件给 {VERIFIER_MODEL}")
        return "skip"

    # 网关会间歇性 500 / 连接重置（尤其验证请求也走同一个池子），
    # 一次失败就 SKIP 会让验证覆盖率虚高，必须退避重试。
    passed: bool | None = None
    reason = ""
    last_err: Exception | None = None
    for attempt in range(3):
        try:
            passed, reason = llm_review(key, task, code_files)
            last_err = None
            break
        except Exception as e:  # noqa: BLE001
            last_err = e
            log(f"RETRY {tid}  验证者调用失败({attempt + 1}/3): {type(e).__name__}")
            time.sleep(5 * (attempt + 1))
    if last_err is not None:
        log(f"SKIP  {tid}  验证者三次失败: {type(last_err).__name__}: {last_err}")
        return "skip"

    if passed is None:
        log(f"SKIP  {tid}  验证输出无法解析，不计入 pass/fail: {reason[:100]}")
        return "skip"

    verdict = "PASS" if passed else "FAIL"
    log(f"{verdict}  {tid}  [{VERIFIER_MODEL} 审] {reason}")
    task["verify_notes"] = reason
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
            continue
        targets.append(t)

    if not targets:
        print("没有需要验证的任务")
        return 0

    log(f"VERIFY 开始，共 {len(targets)} 个任务，验证者模型 {VERIFIER_MODEL}")
    counts = {"pass": 0, "fail": 0, "skip": 0}
    for t in targets:
        verdict = verify_one(t, key, args.dry_run)
        counts[verdict] += 1
        if not args.dry_run and verdict in ("pass", "fail"):
            t["verified"] = verdict
            t["verified_at"] = time.time()
            # 工人对话框里能看到验证结论：PASS 报喜，FAIL 把原因贴回去让它改
            pane = t.get("pane", "")
            if verdict == "pass":
                notify_pane(pane, f"✅ {t['id']} 独立验证通过（verified=pass）")
            else:
                notify_pane(pane, f"❌ {t['id']} 验证没过：{(t.get('verify_notes') or '')[:200]}，请按这个修")
        time.sleep(2)   # 低频，别把验证本身变成限流源

    if not args.dry_run:
        save_queue(tasks)
    log(f"VERIFY 结束 pass={counts['pass']} fail={counts['fail']} skip={counts['skip']}")

    if counts["fail"]:
        log("有任务验证失败，见上；修正后可用 --id <id> --recheck 重审")
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
