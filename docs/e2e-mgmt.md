# E2E Management Verification Report

**Result: 35 PASS / 0 FAIL / 8 SKIP**

| Endpoint | Status | HTTP Code | Key Fields / Reason |
|----------|--------|-----------|---------------------|
| accounting/report | PASS | 200 | count=0, report=[] |
| accounting/status | PASS | 200 | acct_enabled=True, available=False, conntrack_total=0, entries_with_bytes=0 |
| accounts/bind-mac (dry) | SKIP | 405 | method not allowed (405) |
| accounts/reset-usage (dry) | SKIP | 405 | method not allowed (405) |
| billing/bills | PASS | 200 | bills=[], count=0 |
| billing/generate (dry) | SKIP | 404 | endpoint not found (404) |
| billing/plans | PASS | 200 | count=2, plans=[{'id': '', 'name': 'basic', 'price_yuan |
| billing/summary | PASS | 200 | expired=0, online=0, over_quota=0, revenue=9.9 |
| auth/sms/config | PASS | 200 | configured=False |
| auth/sms/send (dry) | PASS | 422 | validated (HTTP 422) |
| auth/sms/verify (dry) | PASS | 422 | validated (HTTP 422) |
| auth/wechat/config | PASS | 200 | appid=wx1234567890abcdef, appsecret=ab****90, secret_set=True |
| auth/wechat/login (dry) | SKIP | 502 | server error 502 |
| ha/status | PASS | 200 | last_error=, last_result=, last_sync=0001-01-01T00:00:00Z, peer_url= |
| ha/import (dry) | PASS | 401 | validated (HTTP 401) |
| ha/sync (dry) | PASS | 400 | validated (HTTP 400) |
| tasks | PASS | 200 | tasks=[{'id': 'task-1', 'type': 'echo', 'param, total=3 |
| tasks/cancel (dry) | PASS | 409 | validated (HTTP 409) |
| tools/capture/start (dry) | PASS | 422 | validated (HTTP 422) |
| tools/capture/status | PASS | 200 | captures=[], count=0 |
| tools/capture/stop (dry) | SKIP | 404 | endpoint not found (404) |
| tools/capture/download (dry) | SKIP | 404 | endpoint not found (404) |
| tools/iperf/run (dry) | PASS | 422 | validated (HTTP 422) |
| tools/iperf/status | PASS | 200 | host=, port=0, duration=0, protocol= |
| alerts/rules | PASS | 200 | count=1, rules=[{'id': 'r1', 'name': 'SSH', 'pattern':  |
| notify/smtp | PASS | 200 | config={'host': '', 'port': 0, 'tls': '', 'user |
| notify/trap (dry) | PASS | 422 | validated (HTTP 422) |
| firewall/log/analysis | PASS | 200 | available=False, reason=/usr/sbin/nft list ruleset: Operation no |
| firewall/log/suggestions | PASS | 200 | applied=False, available=False, note=这些只是建议，不会自动封禁, reason=/usr/sbin/nft list ruleset: Operation no |
| htb/report | PASS | 200 | config={'iface': 'qosdummy', 'root_rate_mbit': , script=# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+ |
| dpi/apps | PASS | 200 | apps={} |
| dpi/flows | PASS | 200 | count=0, flows=[] |
| dpi/mark-rules | PASS | 200 | app_ports={}, nft=table inet router_dpi {
    chain classi |
| dpi/marks | PASS | 200 | rules=[] |
| dpi/rules | PASS | 200 | nft=table inet router_dpi {
  chain classify, rules=[] |
| dpi/tracker-stats | PASS | 200 | stats={'evicted': 0, 'max': 10000, 'total': 10 |
| clients | PASS | 200 | clients=[{'ip': '10.100.1.168', 'mac': 'bc:24:11, count=24 |
| identity | PASS | 200 | identity={'hostname': '', 'domain': '', 'location |
| online/kick (dry) | PASS | 422 | validated (HTTP 422) |
| cert | PASS | 200 | detail=尚未配置证书，Web 管理可能用自签或明文。可用 POST /cert/self, exists=False, path=/etc/ssl/router-re/cert.pem |
| cert/self-signed (dry) | SKIP | 500 | server error 500 |
| users/password (dry) | SKIP | 405 | method not allowed (405) |
| webadmin | PASS | 200 | config={'http_port': 80, 'https_port': 443, 'bi, nginx=server {
    listen 127.0.0.1:80;
    re |

## Notes
- Dry-run endpoints probed with empty body to check shape.
- Hardware/mutating-dependent endpoints marked SKIP with reason.
- No results fabricated.
