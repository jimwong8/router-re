# WAN 接入模块冒烟报告

> 生成: 2026-10-28 21:21:51  BASE=http://127.0.0.1:9090

**汇总**: 23 步 | PASS 18 / FAIL 1 / SKIP 4

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | [wan] GET /api/v1/wan/links | PASS | HTTP 200, 字段 ['links'] 存在 |
| 2 | [wan] GET /api/v1/wan/status | PASS | HTTP 200 |
| 3 | [multiwan] GET /api/v1/multiwan | PASS | HTTP 200 |
| 4 | [upnp] GET /api/v1/upnp/mappings | PASS | HTTP 200, 字段 ['mappings'] 存在 |
| 5 | [upnp] GET /api/v1/upnp/status | PASS | HTTP 200 |
| 6 | [upnp] GET /api/v1/upnp/config | PASS | HTTP 200 |
| 7 | [portmap] GET /api/v1/portmap/rules | PASS | HTTP 200, 字段 ['rules'] 存在 |
| 8 | [pppoe] GET /api/v1/pppoe/status | PASS | HTTP 200 |
| 9 | [pppoe] GET /api/v1/pppoe/sessions | PASS | HTTP 200, 字段 ['sessions', 'count'] 存在 |
| 10 | [pppoe] GET /api/v1/pppoe/config | PASS | HTTP 200 |
| 11 | [porttrigger] GET /api/v1/porttrigger | PASS | HTTP 200 |
| 12 | 结构验证 /api/v1/upnp/mappings | PASS | 列表为空（无映射），端点正常 |
| 13 | 结构验证 /api/v1/portmap/rules | PASS | 列表为空（无映射），端点正常 |
| 14 | [wan] POST /api/v1/wan/links 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 15 | [multiwan] POST /api/v1/multiwan 空 body | PASS | HTTP 422 code=10366 |
| 16 | [upnp] POST /api/v1/upnp/mappings 空 body | PASS | HTTP 422 code=10143 |
| 17 | [portmap] POST /api/v1/portmap/rules 空 body | PASS | HTTP 422 code=10114 |
| 18 | [pppoe] POST /api/v1/pppoe/config 空 body | PASS | HTTP 422 code=10020 |
| 19 | [porttrigger] POST /api/v1/porttrigger 空 body | PASS | HTTP 422 code=10347 |
| 20 | [pppoe] pppoe 真拨号 | SKIP | 无 connect/dial 端点；pppoe 需真实 PPPoE 环境，本机无 |
| 21 | [upnp] upnp 真发现 | SKIP | 无 discover 端点；upnp 需真实网关，本机无 |
| 22 | [portmap] portmap 真规则 | SKIP | portmap/rules POST 为 apply，需真实 nft 环境；空 body 已测 Validate |
| 23 | [multiwan] multiwan/probe 真探测 | SKIP | multiwan/probe 需真实多 WAN 出口；空 body 已测 Validate |

## 1. [wan] GET /api/v1/wan/links

- 结果: **PASS** — HTTP 200, 字段 ['links'] 存在

```
{"commands": [], "links": null}
```

## 2. [wan] GET /api/v1/wan/status

- 结果: **PASS** — HTTP 200

```
{"status": []}
```

## 3. [multiwan] GET /api/v1/multiwan

- 结果: **PASS** — HTTP 200

```
{"count": 2, "members": [{"name": "wan1", "iface": "ens18", "gateway": "10.100.1.1", "weight": 3, "enabled": true}, {"name": "wan2", "iface": "ens18", "gateway": "10.100.1.1", "weight": 1, "enabled...
```

## 4. [upnp] GET /api/v1/upnp/mappings

- 结果: **PASS** — HTTP 200, 字段 ['mappings'] 存在

```
{"mappings": [], "nft": ""}
```

## 5. [upnp] GET /api/v1/upnp/status

- 结果: **PASS** — HTTP 200

```
{"active_count": 0, "installed": false, "running": false, "status": "/usr/bin/systemctl is-active miniupnpd: inactive"}
```

## 6. [upnp] GET /api/v1/upnp/config

- 结果: **PASS** — HTTP 200

```
{"config": {"enabled": false, "external_iface": "", "internal_iface": "", "allow_cidr": "", "max_mappings": 0, "lease_seconds": 0, "natpmp": false}, "miniupnpd": "ext_ifname=eth0\nlistening_ip=br-l...
```

## 7. [portmap] GET /api/v1/portmap/rules

- 结果: **PASS** — HTTP 200, 字段 ['rules'] 存在

```
{"nft": "", "rules": null}
```

## 8. [pppoe] GET /api/v1/pppoe/status

- 结果: **PASS** — HTTP 200

```
{"detail": "pppoe-server not running", "running": false, "status": "stopped"}
```

## 9. [pppoe] GET /api/v1/pppoe/sessions

- 结果: **PASS** — HTTP 200, 字段 ['sessions', 'count'] 存在

```
{"count": 0, "sessions": null}
```

## 10. [pppoe] GET /api/v1/pppoe/config

- 结果: **PASS** — HTTP 200

```
{"config": {"interface": "", "local_ip": "", "pool_start": "", "pool_end": "", "dns": null, "service_name": ""}, "options_path": "/etc/ppp/pppoe-server-options", "options_text": "", "render_error":...
```

## 11. [porttrigger] GET /api/v1/porttrigger

- 结果: **PASS** — HTTP 200

```
{"count": 0, "nft": "", "rules": []}
```

## 12. 结构验证 /api/v1/upnp/mappings

- 结果: **PASS** — 列表为空（无映射），端点正常

```
{"mappings": [], "nft": ""}
```

## 13. 结构验证 /api/v1/portmap/rules

- 结果: **PASS** — 列表为空（无映射），端点正常

```
{"nft": "", "rules": null}
```

## 14. [wan] POST /api/v1/wan/links 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"applied": true, "commands": [], "links": null}
```

## 15. [multiwan] POST /api/v1/multiwan 空 body

- 结果: **PASS** — HTTP 422 code=10366

```
{"code": "10366", "detail": "probe type must be icmp/http/dns, got \"\"", "message": "invalid probe"}
```

## 16. [upnp] POST /api/v1/upnp/mappings 空 body

- 结果: **PASS** — HTTP 422 code=10143

```
{"code": "10143", "detail": "protocol must be tcp or udp, got \"\"", "message": "validation failed"}
```

## 17. [portmap] POST /api/v1/portmap/rules 空 body

- 结果: **PASS** — HTTP 422 code=10114

```
{"code": "10114", "detail": "POST /portmap/rules must carry at least one rule", "message": "empty rule set"}
```

## 18. [pppoe] POST /api/v1/pppoe/config 空 body

- 结果: **PASS** — HTTP 422 code=10020

```
{"code": "10020", "detail": "interface and service_name required", "message": "invalid pppoe config"}
```

## 19. [porttrigger] POST /api/v1/porttrigger 空 body

- 结果: **PASS** — HTTP 422 code=10347

```
{"code": "10347", "detail": "to clear all port triggers, send {rules:[],confirm:true}", "message": "rules must not be empty"}
```

## 20. [pppoe] pppoe 真拨号

- 结果: **SKIP** — 无 connect/dial 端点；pppoe 需真实 PPPoE 环境，本机无

```
N/A
```

## 21. [upnp] upnp 真发现

- 结果: **SKIP** — 无 discover 端点；upnp 需真实网关，本机无

```
N/A
```

## 22. [portmap] portmap 真规则

- 结果: **SKIP** — portmap/rules POST 为 apply，需真实 nft 环境；空 body 已测 Validate

```
N/A
```

## 23. [multiwan] multiwan/probe 真探测

- 结果: **SKIP** — multiwan/probe 需真实多 WAN 出口；空 body 已测 Validate

```
N/A
```
