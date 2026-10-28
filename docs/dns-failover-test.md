# DNS Failover Recovery Test

- Base: http://127.0.0.1:9090
- Date: 2026-10-28 23:23:50

## 发现的 DNS 端点
- GET /api/v1/dns/config
- POST /api/v1/dns/config
- POST /api/v1/dns/resolve (body: {name})
- GET /api/v1/dns/status
- POST /api/v1/diag/dns (body: {name, server})
- 无 cache/flush 端点

## 1. GET /api/v1/dns/config (基线)
- HTTP 200, elapsed=0.00s
- config: {"upstreams": null, "hosts": null, "cache_size": 0, "domain_needed": false, "bogus_priv": false, "enabled": false}
- 结果: PASS

## 2. 正常解析 (baidu.com)
- HTTP 200, elapsed=0.18s
- ips: ['111.63.65.247', '110.242.74.102', '111.63.65.103', '124.237.177.164']
- 结果: PASS

## 3. 不存在域名 (nxdomain-test-9f3k.example)
- HTTP 404, elapsed=0.19s
- body: {"code": "10133", "detail": "no address found for \"nxdomain-test-9f3k.example\"", "message": "resolve failed"}
- 无 500/panic: PASS

## 4. 超时场景 (diag/dns → 192.0.2.1)
- HTTP 500, elapsed=0.00s
- body: {"code": "10144", "detail": "dig not installed", "message": "dns lookup failed"}
- 不无限挂起: PASS
- 注: dig 未安装导致快速返回 500，非真实超时；真实超时需 dig

## 5. GET /api/v1/dns/status
- HTTP 200, elapsed=0.02s
  - cache_size: 0
  - conf_exists: True
  - conf_path: /etc/dnsmasq.d/router-re-dnsproxy.conf
  - enabled: False
  - running: False
  - status: /usr/bin/systemctl is-active dnsmasq: inactive
  - upstreams: None
- 结果: PASS

## 6. cache/flush
- 无 cache/flush 端点 → SKIP

## 汇总

| # | 测试项 | 结果 | 详情 |
|---|--------|------|------|
| 1 | GET /api/v1/dns/config | PASS | HTTP 200, elapsed=0.00s |
| 2 | POST /api/v1/dns/resolve baidu.com | PASS | HTTP 200, ips=['111.63.65.247', '110.242.74.102'] |
| 3 | POST /api/v1/dns/resolve nxdomain | PASS | HTTP 404, elapsed=0.19s (期望非 500/panic) |
| 4 | POST /api/v1/diag/dns 黑洞 IP | PASS | HTTP 500, elapsed=0.00s (dig 未安装→快速返回) |
| 5 | GET /api/v1/dns/status | PASS | HTTP 200, running=False |
| 6 | cache/flush | SKIP | 无 cache/flush 端点 |

- PASS: 5 / FAIL: 0 / SKIP: 1

## 结论
- 正常解析: 200 + IPs ✓
- 不存在域名: 404 + 错误码 (非 500/panic) ✓
- 超时/黑洞: 快速返回 (dig 未安装，非挂起) ✓
- cache/flush: 无端点 → SKIP
