# 并发竞态检测报告 (race detection)

> 生成: 2026-10-28 21:15:39  BASE=http://127.0.0.1:9090

## 1. go test -race ./...

- 命令: `cd mgmtd && CGO_ENABLED=1 go test -race ./...`
- 耗时: 0.7s
- 结果: **75 个包 PASS / 0 FAIL / 0 DATA RACE 报告**
- 退出码: 0

**结论：全包 race clean — go test -race 未报告任何 DATA RACE。**

## 2. 真实并发探测

- 端点: GET /api/v1/portal/clients 与 GET /api/v1/terminals
- 并发: 10 线程 × 2 端点 = 20 个请求，总耗时 0.03s
- 结果: 全部 200，无 500/panic

| 端点 | 状态码 | 请求数 |
|------|--------|--------|
| `/api/v1/portal/clients` | 200 | 10 |
| `/api/v1/terminals` | 200 | 10 |

无异常响应（所有请求 200 OK）。

## 3. 总结

- race detector: clean
- 并发探测: 通过 (20/20 200)
- handler 共享状态（portalMgr/accountMgr/store）在当前测试负载下未暴露 data race；
  但 go test -race 仅覆盖单元测试路径，高并发下的真实流量仍建议持续监控。
