#!/usr/bin/env python3
import re, os, json

ROOT = "/home/jimwong/projects/router-re"
WEB = os.path.join(ROOT, "web", "src")
SERVER = os.path.join(ROOT, "mgmtd", "internal", "server")

# 1. extract api.get/post paths + d.xxx fields from web/src
frontend = {}
for dirpath, _, files in os.walk(WEB):
    for f in files:
        if not f.endswith((".js", ".ts", ".vue")):
            continue
        p = os.path.join(dirpath, f)
        src = open(p).read()
        for m in re.finditer(r"api\.(get|post)\s*\(\s*['\"]([^'\"]+)['\"]", src):
            path = m.group(2)
            # find following d.xxx within 500 chars
            idx = m.end()
            snippet = src[idx:idx+800]
            fields = set(re.findall(r"\bd\.(\w+)", snippet))
            frontend.setdefault(path, set()).update(fields)

# 2. scan server handlers for writeJSON keys
backend = {}
for dirpath, _, files in os.walk(SERVER):
    for f in files:
        if not f.endswith(".go"):
            continue
        p = os.path.join(dirpath, f)
        src = open(p).read()
        # find writeJSON(w, status, map[string]interface{}{...})
        for m in re.finditer(r'writeJSON\([^)]*map\[string\]interface\{\}\{([^}]*)\}', src, re.S):
            body = m.group(1)
            keys = set(re.findall(r'"(\w+)"\s*:', body))
            # try to associate with a route? We'll just accumulate globally
            backend.setdefault("*", set()).update(keys)

# Build report: for each frontend path, check if its fields are in backend keys
all_backend_keys = backend.get("*", set())
missing = []
passn = 0
failn = 0
for path, fields in frontend.items():
    for fld in fields:
        if fld in all_backend_keys:
            passn += 1
        else:
            failn += 1
            missing.append((path, fld))

lines = ["# Field Contract Report", "", f"PASS: {passn}", f"FAIL: {failn}", "", "## Missing fields (frontend reads, backend may not return)", ""]
for path, fld in missing:
    lines.append(f"- {path}: `{fld}`")
with open(os.path.join(ROOT, "docs", "field-contract.md"), "w") as f:
    f.write("\n".join(lines) + "\n")
print(f"missing={failn} report=docs/field-contract.md")
