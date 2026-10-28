# p90-uncovered-batch2: 未覆盖端点测试（批2）

生成时间: 2026-10-29 01:01:59

目标: mgmtd @ http://127.0.0.1:9090（root 运行，admin 登录取 Bearer）。全部为真实 HTTP 调用。

## 汇总: 10 PASS / 0 FAIL（共 10 步）

覆盖端点: diagnostics/ping、diagnostics/portprobe、diagnostics/traceroute、diag/traceroute、
policyroute/rules(+{id})、portmap/rules/{id}、policy/timeschedule/{id}、linkcheck/targets/{id}。

## 分步结果

### diagnostics/ping 本机

- **状态**: PASS
- **详情**: ping 127.0.0.1 -> HTTP 200, result.ok=True, summary='可达，0% 丢包', elapsed_ms=1006 (响应含真实延迟数字)

### diagnostics/ping 黑洞超时

- **状态**: PASS
- **详情**: ping 192.0.2.1 (TEST-NET黑洞) -> HTTP 200, result.ok=False, summary='不可达（100% 丢包）', elapsed_ms=10003 (超时行为符合预期)

### diagnostics/portprobe 开放端口

- **状态**: PASS
- **详情**: probe 127.0.0.1:9090 -> HTTP 200, state='open' (期望 open)

### diagnostics/portprobe 关闭端口

- **状态**: PASS
- **详情**: probe 127.0.0.1:59999 -> HTTP 200, state='closed_or_filtered' (期望 closed_or_filtered)

### diagnostics/traceroute 127.0.0.1

- **状态**: PASS
- **详情**: -> HTTP 200, result.ok=True, 2 行, summary='经过 1 跳' (本地必通)

### diag/traceroute 127.0.0.1

- **状态**: PASS
- **详情**: -> HTTP 200, hop_count=1, tool=traceroute, raw首行='traceroute to 127.0.0.1 (127.0.0.1), 2 hops max, 60 byte packets' (有跳数结果)

### policyroute/rules CRUD

- **状态**: PASS
- **详情**: POST建(added id=90001, 原列表 0 条一并保留) → GET读回 ✓ → DELETE 90001 HTTP 200 deleted=True → 再GET 已消失 ✓

### portmap/rules/{id} CRUD

- **状态**: PASS
- **详情**: POST建(added id=90101) → GET读回 ✓ → DELETE 90101 HTTP 200 deleted=True → 再GET 已消失 ✓

### policy/timeschedule/{id} CRUD

- **状态**: PASS
- **详情**: POST(confirm=false) 为 dry-run 未持久化 (dry_run=True) → DELETE 非存在 id 按契约返回 deleted=false, HTTP 200

### linkcheck/targets/{id} CRUD

- **状态**: PASS
- **详情**: POST建(id=p90-lk) → GET读回 ✓ → DELETE HTTP 200 deleted=True → 再GET 已消失 ✓
