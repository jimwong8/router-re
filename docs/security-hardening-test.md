# 安全加固测试报告

> 生成: 2026-10-28 22:14:34  BASE=http://127.0.0.1:9090

**汇总**: 7 项 / PASS 6 / FAIL 0 / SKIP 1

| # | 检查项 | 结果 |
|---|---|---|
| 1 | 1. 无 token 访问敏感端点 | PASS |
| 2 | 2. 篡改 token | PASS |
| 3 | 3. 路径穿越 | PASS |
| 4 | 4. 命令注入 | PASS |
| 5 | 5. 越权 (viewer→admin 端点) | PASS |
| 6 | 6. 畸形输入 (超长/null/数组) | PASS |
| 7 | 7. CSRF | SKIP |

## 1. 1. 无 token 访问敏感端点 — PASS

- 详情: GET /api/v1/users -> 401; GET /api/v1/config/export -> 401; POST /api/v1/firmware/apply -> 401; GET /api/v1/ddns/config -> 401; GET /api/v1/dns/config -> 401; POST /api/v1/diag/ping -> 401; GET /api/v1/system/services -> 401; GET /api/v1/logs -> 401

```
["GET /api/v1/users -> 401", "GET /api/v1/config/export -> 401", "POST /api/v1/firmware/apply -> 401", "GET /api/v1/ddns/config -> 401", "GET /api/v1/dns/config -> 401", "POST /api/v1/diag/ping -> 401...
```

## 2. 2. 篡改 token — PASS

- 详情: GET /api/v1/users -> 401; GET /api/v1/logs -> 401

```
["GET /api/v1/users -> 401", "GET /api/v1/logs -> 401"]
```

## 3. 3. 路径穿越 — PASS

- 详情: GET /api/v1/../etc/passwd -> 307 leak=False; GET /api/v1/..%2f..%2fetc%2fpasswd -> 404 leak=False

```
["GET /api/v1/../etc/passwd -> 307 leak=False", "GET /api/v1/..%2f..%2fetc%2fpasswd -> 404 leak=False"]
```

## 4. 4. 命令注入 — PASS

- 详情: ping host='127.0.0.1;id' -> 422 uid_leak=False; dns name='127.0.0.1;id' -> 500 uid_leak=False; ping host='$(id)' -> 422 uid_leak=False; dns name='$(id)' -> 500 uid_leak=False; ping host='127.0.0.1 | id' -> 422 uid_leak=False; dns name='127.0.0.1 | id' -> 500 uid_leak=False; ping host='127.0.0.1; cat /etc/passwd' -> 422 uid_leak=False; dns name='127.0.0.1; cat /etc/passwd' -> 500 uid_leak=False

```
["ping host='127.0.0.1;id' -> 422 uid_leak=False", "dns name='127.0.0.1;id' -> 500 uid_leak=False", "ping host='$(id)' -> 422 uid_leak=False", "dns name='$(id)' -> 500 uid_leak=False", "ping host='127...
```

## 5. 5. 越权 (viewer→admin 端点) — PASS

- 详情: viewer GET /users -> 403 (expect 401/403) rejected=True; viewer POST /users -> 403 (expect 401/403) rejected=True

```
["viewer GET /users -> 403 (expect 401/403) rejected=True", "viewer POST /users -> 403 (expect 401/403) rejected=True"]
```

## 6. 6. 畸形输入 (超长/null/数组) — PASS

- 详情: ping 100KB host -> 422 (not 500: True); ping body=null -> 422 (not 500: True); ping body=array -> 400 (not 500: True); dns/resolve body=null -> 404 (not 500: True)

```
["ping 100KB host -> 422 (not 500: True)", "ping body=null -> 422 (not 500: True)", "ping body=array -> 400 (not 500: True)", "dns/resolve body=null -> 404 (not 500: True)"]
```

## 7. 7. CSRF — SKIP

- 详情: Bearer token 认证、无 cookie 会话，CSRF 不适用 (N/A)

## 严重漏洞

无 CRITICAL 级别漏洞发现。

