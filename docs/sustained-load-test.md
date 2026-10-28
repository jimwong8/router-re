# 持续并发压力测试报告

> 生成: 2026-10-28 22:14:35

- 持续时长: 60s, 并发: 10 线程, 端点: /api/v1/terminals, /api/v1/online, /api/v1/system/info, /api/v1/health, /api/v1/system/services
- 总请求: 9239, 成功: 9239, 失败: 0, 成功率: 100.00%
- 延迟: P50=30.4ms, P95=241.6ms, P99=379.1ms
- mgmtd pid=3110702
- RSS: 33436 KB -> 43356 KB (delta 9920 KB)
- FD: 7 -> 7 (delta 0)
- 可疑泄漏: 否 (阈值 RSS>50MB 或 FD>100)
- 错误计数: {}

## 结论

持续 60s 负载下无 500/连接拒绝，无内存或句柄泄漏迹象。
