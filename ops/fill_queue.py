import json

p = "ops/tasks.jsonl"
tasks = [json.loads(l) for l in open(p) if l.strip()]
have = {t["id"] for t in tasks}

C = ("先读 mgmtd/internal/server/p15.go 学 handler 约定：writeJSON(w,200,map[string]interface{}{...})；"
     "错误用 writeJSON(w,422,errBody(码,\"简述\",\"原因\"))；配置读写 s.store.Get/Set；"
     "执行命令用 (execx.DefaultRunner{}).Run(\"cmd\", args...)，参数一律 slice 不拼 shell。")

FWLOG = """任务：防火墙日志分析包。只写两个文件。

第一步 `cd /home/jimwong/projects/router-re/mgmtd && mkdir -p internal/fwlog`

写 internal/fwlog/analyze.go (package fwlog)，包含：
- type Event { Time int64; SrcIP, DstIP string; SrcPort, DstPort int; Proto string; Action string; Iface, Chain string; Len int }
- type Stat { IP string; Total, Dropped int; Bytes int64 }
- type Rule { SrcIP string; Count int; Reason string }
- func ParseLines(lines []string) []Event   // 解析 nft/iptables LOG 行，解析不了的跳过，不报错不 panic
- func TopTalkers(evts []Event, n int) []Stat   // 按 SrcIP 聚合，按 Dropped 降序
- func Summarize(evts []Event) map[string]interface{}   // 总数 / 按动作 / 按协议 / 按链
- func Suspicious(evts []Event, window int) []string    // 同一源IP在 window 秒内 drop 超过 50 次 -> 返回该IP
- func SuggestBlock(evts []Event, threshold int) []Rule  // 生成"建议封禁"清单，**只给建议，绝不自动执行**

写 analyze_test.go，8 个测试：
1 ParseLines 解析真实 nft LOG 行（样例含 SRC= / DST= / PROTO=）
2 ParseLines 对垃圾行不 panic 且跳过
3 TopTalkers 按 Dropped 降序
4 TopTalkers 的 n 超过长度不越界
5 Summarize 统计正确（总数 / 各动作数）
6 Suspicious 对同一IP高频 drop 返回它，低频不返回
7 SuggestBlock 阈值生效
8 SuggestBlock 的返回类型里没有任何"执行命令"字段（只给建议）

做完跑 `cd /home/jimwong/projects/router-re/mgmtd && CGO_ENABLED=1 go build ./internal/fwlog/ && CGO_ENABLED=1 go test ./internal/fwlog/ -v` 必须全过。
不改 server.go，不 git commit。只回：fwlog 完成，N 个测试通过。"""

SCHED = """任务：时间段策略接线。只写一个文件 mgmtd/internal/server/p20.go。

**前置检查，很重要**：先跑
  ls mgmtd/internal/timesched/
  grep -n 'handleGetTimeSchedule\\|handleSetTimeSchedule' mgmtd/internal/server/p15.go
如果 p15.go 里已经有这两个 handler，**不要重复实现**，只在回复里说明「已存在，未新增」，然后停手。

如果确实缺，实现：
- GET /api/v1/policy/timeschedule -> {count, timezone, now, rules:[Rule 加上 active_now 字段]}
  （用 time.LoadLocation("Asia/Shanghai") + timesched.Active 计算 active_now）
- POST /api/v1/policy/timeschedule，body {rules:[Rule]}
  - 逐个 timesched.Validate，失败返回 422 指出第几条、哪个字段
  - 空数组拒绝，提示「要停用请把 enabled 设为 false，而不是清空列表」
  - 不带 confirm 只返回 nft 预览（dry_run:true）
  - 带 confirm 才 nft -f - 应用并落盘，返回 applied / output

""" + C + """

错误码从 10470 起。不改 server.go，不 git commit。
做完 `cd /home/jimwong/projects/router-re/mgmtd && CGO_ENABLED=1 go build ./...` 必须 rc=0。
只回：p20.go 完成，并说明是否复用了已有实现。"""

UIAUDIT = """任务：全站 UI 验收并写报告 docs/ui-audit-report.md。

**这是实测任务，不是写文档任务。**

1. `cd /home/jimwong/projects/router-re && ls web/src/views/*.vue | wc -l` 确认页面数
2. 读 `scripts/ui_acceptance.py`，理解它怎么比对「接口条数 vs 渲染行数」
3. **扩展它的 PAGES 列表**，把这些页面都加进去（每个要配对应的取数接口）：
   /voucher /ddns /services /network-tools /vpn-client /security-plus
   /online-users /cert /network-advanced
   如果某页还没接路由，跳过并在报告里记下
4. 实际跑 `cd /home/jimwong/projects/router-re && timeout 300 python3 scripts/ui_acceptance.py`
5. 报告 docs/ui-audit-report.md 内容：
   - 表格：页面 / 接口条数 / 渲染行数 / 结论(OK/FAIL/SKIP)
   - 对 FAIL 和 SKIP 说明原因
   - 一节「发现的问题」，列出实际看到的不一致
   - 一节「未能验证的页面」，诚实列出

**硬要求**：报告里的数字必须是脚本真实输出，不许编。脚本跑不起来就如实写「无法运行」+ 错误信息。
可以改 scripts/ui_acceptance.py 的 PAGES 列表。不改其它文件。不 git commit。
只回：ui-audit 完成，N 个页面 OK，M 个有问题。"""

APIDOC = """任务：从代码生成 API 参考文档 docs/api-reference.md。

**必须从源码提取，不许手写猜。**

1. `cd /home/jimwong/projects/router-re && grep -oE 'HandleFunc\\("(GET|POST|PUT|DELETE|PATCH) /api/v1/[^"]+"' mgmtd/internal/server/server.go | wc -l`
   拿到真实端点数
2. 提取所有 (方法, 路径) 和紧随其后的 handler 名
3. 对每条，到 mgmtd/internal/server/*.go 里找 handler 实现，
   读它的 writeJSON 结构，提取响应字段（map key 或 struct 的 json tag）
4. 生成 markdown：
   - 顶部：端点总数、按方法分组统计
   - 表格：方法 / 路径 / handler / 主要响应字段 / 是否有请求体
   - 一节「认证」：说明 Authorization: Bearer <token>，登录端点除外
   - 一节「错误格式」：errBody(码, message, detail) 的形状

**硬要求**：端点数必须和 grep 出来的数字一致，不一致说明漏了，要补齐。
不确定的响应字段写「（未解析）」，不要编造。
不改其它文件，不 git commit。只回：api-reference.md 完成，覆盖 N 个端点。"""

new = [
    {"id": "p20-firewall-log",
     "expect": ["mgmtd/internal/fwlog/analyze.go", "mgmtd/internal/fwlog/analyze_test.go"],
     "must_contain": {"mgmtd/internal/fwlog/analyze.go": ["func ParseLines", "func TopTalkers", "func SuggestBlock"]},
     "prompt": FWLOG},
    {"id": "p20-schedule-wire",
     "expect": ["mgmtd/internal/server/p20.go"],
     "must_contain": {"mgmtd/internal/server/p20.go": ["timesched"]},
     "must_build": True,
     "build_cmds": [["mgmtd", ["go", "build", "./..."]]],
     "prompt": SCHED},
    {"id": "p20-ui-audit",
     "expect": ["docs/ui-audit-report.md"],
     "must_contain": {"docs/ui-audit-report.md": ["页面", "行数"]},
     "prompt": UIAUDIT},
    {"id": "p20-api-doc",
     "expect": ["docs/api-reference.md"],
     "must_contain": {"docs/api-reference.md": ["/api/v1/", "GET", "POST"]},
     "prompt": APIDOC},
]

for t in new:
    if t["id"] not in have:
        t["status"] = "pending"
        tasks.append(t)

with open(p, "w") as f:
    for t in tasks:
        f.write(json.dumps(t, ensure_ascii=False) + "\n")

print(f"队列 {len(tasks)} 条，pending {sum(1 for t in tasks if t.get('status') == 'pending')} 条")
for t in tasks:
    if t.get("status") == "pending":
        print("  PENDING", t["id"])
