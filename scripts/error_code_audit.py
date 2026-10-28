#!/usr/bin/env python3
"""error-code-audit: scan mgmtd for 104xx codes, check conflicts/gaps, write docs/error-code-audit.md"""
import os, re, datetime
from collections import defaultdict
ROOT = "/home/jimwong/projects/router-re"
MGMTD = os.path.join(ROOT, "mgmtd")
DOC = os.path.join(ROOT, "docs", "error-code-audit.md")

pattern = re.compile(r'errBody\(\s*(104\d{2})\s*,\s*("(?:[^"\\]|\\.)*"|`[^`]*`)')

code_msg = defaultdict(set)
code_files = defaultdict(set)
code_count = defaultdict(int)

for dirpath, _, files in os.walk(MGMTD):
    for fn in files:
        if not fn.endswith(".go"):
            continue
        path = os.path.join(dirpath, fn)
        rel = os.path.relpath(path, MGMTD)
        with open(path, encoding="utf-8", errors="replace") as f:
            text = f.read()
        for m in pattern.finditer(text):
            code = m.group(1)
            msg = m.group(2)
            code_msg[code].add(msg)
            code_files[code].add(rel)
            code_count[code] += 1

all_codes = sorted(code_msg.keys(), key=lambda x: int(x))
code_ints = [int(c) for c in all_codes]
minc, maxc = min(code_ints), max(code_ints)

conflicts = {c: msgs for c, msgs in code_msg.items() if len(msgs) > 1}

full_range = set(str(i) for i in range(10400, maxc + 1))
missing = sorted(full_range - set(all_codes), key=lambda x: int(x))

multi_file = {c: fs for c, fs in code_files.items() if len(fs) > 1}

single_gaps = []
for i in range(10400, maxc):
    if str(i) not in all_codes and str(i-1) in all_codes and str(i+1) in all_codes:
        single_gaps.append(str(i))

doc_path = os.path.join(ROOT, "docs", "e2e-audit-fixes.md")
doc_codes = set()
if os.path.exists(doc_path):
    with open(doc_path, encoding="utf-8") as f:
        for m in re.finditer(r'\b(104\d{2})\b', f.read()):
            doc_codes.add(m.group(1))

not_in_doc = sorted(set(all_codes) - doc_codes, key=lambda x: int(x))
in_doc_not_code = sorted(doc_codes - set(all_codes), key=lambda x: int(x))

lines = []
lines.append("# 错误码审计报告 (104xx)")
lines.append("")
lines.append("> 扫描: " + MGMTD + "  (mgmtd Go 源码)")
lines.append("> 生成: " + datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S"))
lines.append("")
lines.append("## 1. 概览")
lines.append("")
lines.append("- 实际使用 code 数量: **" + str(len(all_codes)) + "**")
lines.append("- 范围: **" + str(minc) + " ~ " + str(maxc) + "**")
lines.append("- 冲突 (同一 code 多个不同 message): **" + str(len(conflicts)) + "**")
lines.append("- 跳号 (10400~" + str(maxc) + " 范围内缺失): **" + str(len(missing)) + "**")
lines.append("- 在 docs/e2e-audit-fixes.md 中未记录的 code: **" + str(len(not_in_doc)) + "**")
lines.append("- 文档记录但源码未使用的 code: **" + str(len(in_doc_not_code)) + "**")
lines.append("")
lines.append("## 2. 冲突详情 (同一 code, 多个 message)")
lines.append("")
if conflicts:
    for c in sorted(conflicts, key=lambda x: int(x)):
        lines.append("- `" + c + "`:")
        for m in sorted(conflicts[c]):
            lines.append("  - " + m)
else:
    lines.append("(无)")
lines.append("")
lines.append("## 3. 跳号 (10400 ~ " + str(maxc) + ")")
lines.append("")
if missing:
    lines.append(", ".join(missing))
else:
    lines.append("(无)")
lines.append("")
lines.append("## 3b. 单点跳号 (潜在 bug: N-1 与 N+1 都存在但 N 缺失)")
lines.append("")
if single_gaps:
    lines.append(", ".join(single_gaps))
else:
    lines.append("(无)")
lines.append("")
lines.append("## 4. 多文件重复定义 (信息性)")
lines.append("")
if multi_file:
    for c in sorted(multi_file, key=lambda x: int(x)):
        lines.append("- `" + c + "`: " + ", ".join(sorted(multi_file[c])))
else:
    lines.append("(无)")
lines.append("")
lines.append("## 5. 源码中有但文档未记录")
lines.append("")
if not_in_doc:
    lines.append(", ".join(not_in_doc))
else:
    lines.append("(无)")
lines.append("")
lines.append("## 6. 文档记录但源码未使用")
lines.append("")
if in_doc_not_code:
    lines.append(", ".join(in_doc_not_code))
else:
    lines.append("(无)")
lines.append("")
lines.append("## 7. 全部 code 清单 (code -> messages, files)")
lines.append("")
lines.append("| code | messages | files |")
lines.append("|---|---|---|")
for c in all_codes:
    msgs = " / ".join(sorted(code_msg[c]))
    fs = ", ".join(sorted(code_files[c]))
    lines.append("| " + c + " | " + msgs + " | " + fs + " |")
lines.append("")

os.makedirs(os.path.dirname(DOC), exist_ok=True)
with open(DOC, "w", encoding="utf-8") as f:
    f.write("\n".join(lines) + "\n")

print("codes=" + str(len(all_codes)) + " range=" + str(minc) + "-" + str(maxc) + " conflicts=" + str(len(conflicts)) + " missing=" + str(len(missing)) + " not_in_doc=" + str(len(not_in_doc)))
print("report: " + DOC)
