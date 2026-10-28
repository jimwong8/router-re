# conntrack 字节统计（acct）端到端验证报告

> 生成: 2026-10-28 23:30:42

**汇总**: 4 步 | PASS 4 / FAIL 0 / SKIP 0

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 1 GET /api/v1/conntrack/acct | PASS | HTTP 200 on=True persisted=True path=/proc/sys/net/netfilter/nf_conntrack_acct |
| 2 | 2 系统 sysctl vs 接口 | PASS | sysctl=1 api_on=True consistent=True |
| 3 | 3 GET /api/v1/conntrack 字节字段 | PASS | HTTP 200 count=1000 types={'int'} some_bytes_gt0=True |
| 4 | 4 系统 vs 接口字节对比 | PASS | system_count=4218 api_count=1000 system_bytes=87031653 api_bytes=77088822 consistent(system>=api)=True |

## 1. 1 GET /api/v1/conntrack/acct

- 结果: **PASS** — HTTP 200 on=True persisted=True path=/proc/sys/net/netfilter/nf_conntrack_acct

```
{"on": true, "path": "/proc/sys/net/netfilter/nf_conntrack_acct", "persisted": true}
```

## 2. 2 系统 sysctl vs 接口

- 结果: **PASS** — sysctl=1 api_on=True consistent=True

```
sysctl=1
```

## 3. 3 GET /api/v1/conntrack 字节字段

- 结果: **PASS** — HTTP 200 count=1000 types={'int'} some_bytes_gt0=True

```
{"available": true, "count": 1000, "entries": [{"proto": "udp", "src": "100.111.28.64", "dst": "223.5.5.5", "src_port": 57839, "dst_port": 53, "state": "", "bytes": 65, "packets": 1, "timeout_sec": 0}, {"proto": "tcp", "src": "127.0.0.1", "dst": "127.0.0.1", "src_port": 52998, "dst_port": 9090, "...
```

## 4. 4 系统 vs 接口字节对比

- 结果: **PASS** — system_count=4218 api_count=1000 system_bytes=87031653 api_bytes=77088822 consistent(system>=api)=True

```
system_bytes=87031653 api_bytes=77088822
```
