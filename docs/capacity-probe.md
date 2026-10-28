# 容量/分页压力探针报告 (capacity-probe)

> 生成: 2026-10-28 19:57:13  BASE=http://127.0.0.1:9090

## 1. 单次 GET 计时 + 响应大小

| 端点 | HTTP | 耗时(ms) | 响应字节 | 条数 | 有分页 |
|---|---|---|---|---|---|
| `/api/v1/system/services` | 200 | 28.6 | 73873 | 526 | 否 |
| `/api/v1/logs` | 200 | 47.7 | 17436 | 100 | 是 |
| `/api/v1/terminals` | 200 | 16.2 | 4186 | 39 | 否 |

## 2. 分页 limit 三档对比（仅 /api/v1/logs 支持）

| 端点 | limit=10 | limit=100 | limit=1000 | 结论 |
|---|---|---|---|---|
| `/api/v1/system/services` | — | — | — | 无分页参数，全量返回 |
| `/api/v1/logs` | 25.6ms | 46.2ms | 169.3ms | limit 越大耗时递增 |
| `/api/v1/terminals` | — | — | — | 无分页参数，全量返回 |

## 3. /api/v1/terminals 连续 20 次抖动探测

- n=20  min=8.5ms  max=18.3ms  avg=11.0ms
- max/avg = 1.66  → 无明显抖动 (<=5)

## 4. 优化建议（仅建议，未改代码）

- `/api/v1/system/services`：无分页，systemctl 全量返回；小内存路由上建议加 `limit`/`unit` 过滤或前端虚拟滚动。
- `/api/v1/logs`：已支持 `limit`（上限 5000），建议前端默认 limit=200 并提供分页。
- `/api/v1/terminals`：聚合三源无分页；34 条时 harmless，但 ARP/conntrack 大时可能膨胀，建议按 iface/source 过滤或分页。
- 若 max/avg > 5 记为抖动，需排查 GC/锁竞争；本报告按实测结论。
