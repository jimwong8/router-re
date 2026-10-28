# 管理面模块冒烟报告

> 生成: 2026-10-28 21:33:45  BASE=http://127.0.0.1:9090

**汇总**: 33 步 | PASS 24 / FAIL 5 / SKIP 4

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | [webadmin] GET /api/v1/webadmin | PASS | HTTP 200, 字段 ['config'] 存在 |
| 2 | [remote] GET /api/v1/remote/access | PASS | HTTP 200, 字段 ['access'] 存在 |
| 3 | [remote] GET /api/v1/remote/access/status | PASS | HTTP 200, 字段 ['ssh_active', 'ssh_port'] 存在 |
| 4 | [notify] GET /api/v1/notify/smtp | PASS | HTTP 200, 字段 ['config'] 存在 |
| 5 | [notify] GET /api/v1/notify/trap | PASS | HTTP 200, 字段 ['sink'] 存在 |
| 6 | [billing] GET /api/v1/billing/bills | PASS | HTTP 200, 字段 ['bills', 'count'] 存在 |
| 7 | [billing] GET /api/v1/billing/plans | PASS | HTTP 200, 字段 ['plans', 'count'] 存在 |
| 8 | [billing] GET /api/v1/billing/summary | PASS | HTTP 200, 字段 ['total_accounts'] 存在 |
| 9 | [accounting] GET /api/v1/accounting/status | PASS | HTTP 200, 字段 ['available'] 存在 |
| 10 | [accounting] GET /api/v1/accounting/report | PASS | HTTP 200, 字段 ['report', 'count'] 存在 |
| 11 | [users] GET /api/v1/users | PASS | HTTP 200, 字段 ['users', 'count'] 存在 |
| 12 | [route] GET /api/v1/route/bgp | PASS | HTTP 200, 字段 ['config'] 存在 |
| 13 | [route] GET /api/v1/route/ospf | PASS | HTTP 200, 字段 ['config'] 存在 |
| 14 | [route] GET /api/v1/route/static | PASS | HTTP 200, 字段 ['routes'] 存在 |
| 15 | 结构验证 /api/v1/billing/bills | PASS | 0 条 |
| 16 | 结构验证 /api/v1/billing/plans | PASS | 2 条 |
| 17 | 结构验证 /api/v1/accounting/report | PASS | 0 条 |
| 18 | 结构验证 /api/v1/users | PASS | 1 条, 字段齐全 |
| 19 | 结构验证 /api/v1/route/static | PASS | 0 条 |
| 20 | [webadmin] POST /api/v1/webadmin 空 body | PASS | HTTP 422 code=10361（已拒绝） |
| 21 | [remote] POST /api/v1/remote/access 空 body | PASS | HTTP 422 code=10141（已拒绝） |
| 22 | [notify] POST /api/v1/notify/smtp 空 body | PASS | HTTP 422 code=10311（已拒绝） |
| 23 | [notify] POST /api/v1/notify/trap 空 body | PASS | HTTP 422 code=10315（已拒绝） |
| 24 | [notify] POST /api/v1/notify/smtp/preview 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 25 | [billing] POST /api/v1/billing/plans 空 body | PASS | HTTP 400 code=10161（已拒绝） |
| 26 | [billing] POST /api/v1/billing/bills/generate 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 27 | [route] POST /api/v1/route/bgp 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 28 | [route] POST /api/v1/route/ospf 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 29 | [route] POST /api/v1/route/static 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 30 | [notify] notify/smtp 真发邮件 | SKIP | 需真实 SMTP 服务器；空 body 已测 Validate |
| 31 | [billing] billing/bills/generate 真扣费 | SKIP | 需真实计费数据；空 body 已测 Validate |
| 32 | [users] users 创建/删除 | SKIP | 账号操作风险高，仅 GET 列表，不创建/删 |
| 33 | [remote] remote/access 真下发 | SKIP | 需真实 SSH/网络环境；空 body 已测 Validate |

## 1. [webadmin] GET /api/v1/webadmin

- 结果: **PASS** — HTTP 200, 字段 ['config'] 存在

```
{"config": {"http_port": 80, "https_port": 443, "bind_addr": "127.0.0.1", "redirect_http": true, "idle_timeout_sec": 0}, "nginx": "server {\n    listen 127.0.0.1:80;\n    return 301 https://$host:4...
```

## 2. [remote] GET /api/v1/remote/access

- 结果: **PASS** — HTTP 200, 字段 ['access'] 存在

```
{"access": {"ssh_enabled": false, "ssh_port": 0, "web_enabled": false, "web_port": 0, "allow_sources": null, "telnet_enabled": false}, "warnings": null}
```

## 3. [remote] GET /api/v1/remote/access/status

- 结果: **PASS** — HTTP 200, 字段 ['ssh_active', 'ssh_port'] 存在

```
{"ssh_active": "inactive", "ssh_listen": "", "ssh_port": "22,((\"sshd\",pid=2448,fd=6)),6446,((\"ssh\",pid=3061943,fd=4)),22,((\"sshd\",pid=2448,fd=7))", "telnet_active": "inactive"}
```

## 4. [notify] GET /api/v1/notify/smtp

- 结果: **PASS** — HTTP 200, 字段 ['config'] 存在

```
{"config": {"host": "", "port": 0, "tls": "", "username": "", "password": "***", "from": "", "to": null, "enabled": false}}
```

## 5. [notify] GET /api/v1/notify/trap

- 结果: **PASS** — HTTP 200, 字段 ['sink'] 存在

```
{"sink": "authCommunity log,execute,net  default\n", "target": {"host": "", "port": 0, "version": "", "community": "***", "enabled": false}}
```

## 6. [billing] GET /api/v1/billing/bills

- 结果: **PASS** — HTTP 200, 字段 ['bills', 'count'] 存在

```
{"bills": [], "count": 0}
```

## 7. [billing] GET /api/v1/billing/plans

- 结果: **PASS** — HTTP 200, 字段 ['plans', 'count'] 存在

```
{"count": 2, "plans": [{"id": "", "name": "basic", "price_yuan": 9.9, "days": 30, "period_days": 0, "quota_mb": 1024}, {"id": "", "name": "pro", "price_yuan": 29.9, "days": 30, "period_days": 0, "q...
```

## 8. [billing] GET /api/v1/billing/summary

- 结果: **PASS** — HTTP 200, 字段 ['total_accounts'] 存在

```
{"expired": 0, "online": 0, "over_quota": 0, "revenue": 0, "total_accounts": 0}
```

## 9. [accounting] GET /api/v1/accounting/status

- 结果: **PASS** — HTTP 200, 字段 ['available'] 存在

```
{"acct_enabled": true, "available": true, "conntrack_total": 576, "entries_with_bytes": 540}
```

## 10. [accounting] GET /api/v1/accounting/report

- 结果: **PASS** — HTTP 200, 字段 ['report', 'count'] 存在

```
{"count": 0, "report": []}
```

## 11. [users] GET /api/v1/users

- 结果: **PASS** — HTTP 200, 字段 ['users', 'count'] 存在

```
{"count": 1, "users": [{"username": "admin", "role": "admin", "enabled": true, "created_at": 1793194091, "last_login": 0}]}
```

## 12. [route] GET /api/v1/route/bgp

- 结果: **PASS** — HTTP 200, 字段 ['config'] 存在

```
{"config": {"local_as": 0, "router_id": "", "peers": null, "networks": null}, "frr": "frr version 8.4\nfrr defaults traditional\nhostname router\nlog syslog informational\n!\n!\nline vty\n"}
```

## 13. [route] GET /api/v1/route/ospf

- 结果: **PASS** — HTTP 200, 字段 ['config'] 存在

```
{"config": {"router_id": "", "networks": null, "area": ""}, "frr": "frr version 8.4\nfrr defaults traditional\nhostname router\nlog syslog informational\n!\nrouter ospf\n!\nline vty\n"}
```

## 14. [route] GET /api/v1/route/static

- 结果: **PASS** — HTTP 200, 字段 ['routes'] 存在

```
{"commands": null, "routes": null}
```

## 15. 结构验证 /api/v1/billing/bills

- 结果: **PASS** — 0 条

```
{"bills": [], "count": 0}
```

## 16. 结构验证 /api/v1/billing/plans

- 结果: **PASS** — 2 条

```
{"count": 2, "plans": [{"id": "", "name": "basic", "price_yuan": 9.9, "days": 30, "period_days": 0, "quota_mb": 1024}, {"id": "", "name": "pro", "price_yuan": 29.9, "days": 30, "period_days": 0, "q...
```

## 17. 结构验证 /api/v1/accounting/report

- 结果: **PASS** — 0 条

```
{"count": 0, "report": []}
```

## 18. 结构验证 /api/v1/users

- 结果: **PASS** — 1 条, 字段齐全

```
{"username": "admin", "role": "admin", "enabled": true, "created_at": 1793194091, "last_login": 0}
```

## 19. 结构验证 /api/v1/route/static

- 结果: **PASS** — 0 条

```
{"commands": null, "routes": null}
```

## 20. [webadmin] POST /api/v1/webadmin 空 body

- 结果: **PASS** — HTTP 422 code=10361（已拒绝）

```
{"code": "10361", "detail": "http_port and https_port cannot both be 0 — that would lock you out of the management interface entirely", "message": "invalid listen config"}
```

## 21. [remote] POST /api/v1/remote/access 空 body

- 结果: **PASS** — HTTP 422 code=10141（已拒绝）

```
{"code": "10141", "detail": "ssh_port 0 超出 1-65535", "message": "validation failed"}
```

## 22. [notify] POST /api/v1/notify/smtp 空 body

- 结果: **PASS** — HTTP 422 code=10311（已拒绝）

```
{"code": "10311", "detail": "host required", "message": "invalid smtp config"}
```

## 23. [notify] POST /api/v1/notify/trap 空 body

- 结果: **PASS** — HTTP 422 code=10315（已拒绝）

```
{"code": "10315", "detail": "host required", "message": "invalid trap target"}
```

## 24. [notify] POST /api/v1/notify/smtp/preview 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
"From: \r\nTo: \r\nSubject: \r\nDate: Wed, 28 Oct 2026 21:33:45 +0800\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n"
```

## 25. [billing] POST /api/v1/billing/plans 空 body

- 结果: **PASS** — HTTP 400 code=10161（已拒绝）

```
{"code": "10161", "detail": "expected a plan object or {\"plans\":[...]}", "message": "invalid body"}
```

## 26. [billing] POST /api/v1/billing/bills/generate 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"count": 0, "generated": [], "skipped": null}
```

## 27. [route] POST /api/v1/route/bgp 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"config": {"local_as": 0, "router_id": "", "peers": null, "networks": null}, "frr": "frr version 8.4\nfrr defaults traditional\nhostname router\nlog syslog informational\n!\n!\nline vty\n"}
```

## 28. [route] POST /api/v1/route/ospf 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"config": {"router_id": "", "networks": null, "area": ""}, "frr": "frr version 8.4\nfrr defaults traditional\nhostname router\nlog syslog informational\n!\nrouter ospf\n!\nline vty\n"}
```

## 29. [route] POST /api/v1/route/static 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"commands": null, "routes": null}
```

## 30. [notify] notify/smtp 真发邮件

- 结果: **SKIP** — 需真实 SMTP 服务器；空 body 已测 Validate

```
N/A
```

## 31. [billing] billing/bills/generate 真扣费

- 结果: **SKIP** — 需真实计费数据；空 body 已测 Validate

```
N/A
```

## 32. [users] users 创建/删除

- 结果: **SKIP** — 账号操作风险高，仅 GET 列表，不创建/删

```
N/A
```

## 33. [remote] remote/access 真下发

- 结果: **SKIP** — 需真实 SSH/网络环境；空 body 已测 Validate

```
N/A
```
