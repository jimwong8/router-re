# 访问控制模块冒烟报告

> 生成: 2026-10-28 21:26:54  BASE=http://127.0.0.1:9090

**汇总**: 33 步 | PASS 27 / FAIL 1 / SKIP 5

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | [radius] GET /api/v1/radius/config | PASS | HTTP 200, 字段 ['config', 'servers'] 存在 |
| 2 | [nac] GET /api/v1/nac/config | PASS | HTTP 200, 字段 ['config'] 存在 |
| 3 | [nac] GET /api/v1/nac/neighbors | PASS | HTTP 200, 字段 ['neighbors', 'count'] 存在 |
| 4 | [nac] GET /api/v1/nac/connlimits | PASS | HTTP 200, 字段 ['limits'] 存在 |
| 5 | [nac] GET /api/v1/nac/top-connections | PASS | HTTP 200 |
| 6 | [identity] GET /api/v1/identity | PASS | HTTP 200, 字段 ['identity'] 存在 |
| 7 | [quota] GET /api/v1/quota/policies | PASS | HTTP 200, 字段 ['policies', 'count'] 存在 |
| 8 | [quota] GET /api/v1/quota/report | PASS | HTTP 200 |
| 9 | [quota] GET /api/v1/quota/nft | PASS | HTTP 200 |
| 10 | [webfilter] GET /api/v1/webfilter/rules | PASS | HTTP 200, 字段 ['rules'] 存在 |
| 11 | [dpi] GET /api/v1/dpi/rules | PASS | HTTP 200, 字段 ['rules'] 存在 |
| 12 | [dpi] GET /api/v1/dpi/marks | PASS | HTTP 200, 字段 ['rules'] 存在 |
| 13 | [dpi] GET /api/v1/dpi/flows | PASS | HTTP 200, 字段 ['flows', 'count'] 存在 |
| 14 | [dpi] GET /api/v1/dpi/apps | PASS | HTTP 200, 字段 ['apps'] 存在 |
| 15 | [dpi] GET /api/v1/dpi/tracker-stats | PASS | HTTP 200, 字段 ['stats'] 存在 |
| 16 | [dpi] GET /api/v1/dpi/mark-rules | PASS | HTTP 200, 字段 ['app_ports'] 存在 |
| 17 | 结构验证 /api/v1/nac/neighbors | PASS | 34 条, 字段齐全 |
| 18 | 结构验证 /api/v1/quota/policies | PASS | 0 条（空或无 itemkeys） |
| 19 | 结构验证 /api/v1/webfilter/rules | PASS | 0 条（空或无 itemkeys） |
| 20 | 结构验证 /api/v1/dpi/flows | PASS | 0 条（空或无 itemkeys） |
| 21 | [radius] POST /api/v1/radius/config 空 body | FAIL | 空 body 被接受 HTTP 200（Validate 未拦） |
| 22 | [nac] POST /api/v1/nac/config 空 body | PASS | HTTP 422 code=10151（已拒绝） |
| 23 | [nac] POST /api/v1/nac/connlimits 空 body | PASS | HTTP 400 code=10150（已拒绝） |
| 24 | [identity] POST /api/v1/identity 空 body | PASS | HTTP 422 code=10363（已拒绝） |
| 25 | [quota] POST /api/v1/quota/policies 空 body | PASS | HTTP 422 code=10173（已拒绝） |
| 26 | [webfilter] POST /api/v1/webfilter/rules 空 body | PASS | HTTP 422 code=10121（已拒绝） |
| 27 | [dpi] POST /api/v1/dpi/rules 空 body | PASS | HTTP 422 code=10043（已拒绝） |
| 28 | [dpi] POST /api/v1/dpi/marks 空 body | PASS | HTTP 422 code=10043（已拒绝） |
| 29 | [radius] radius/test 真 RADIUS | SKIP | radius/test 需真实 RADIUS server，本机无；radius/config 已读 |
| 30 | [dpi] dpi/classify 真分类 | SKIP | dpi/classify 需查询参数 ip/proto/port；空参返回 400 属预期 |
| 31 | [quota] quota/apply 真下发 | SKIP | quota/apply 需真实 nft 环境；空 body 已测 Validate |
| 32 | [nac] nac/connlimits/apply 真下发 | SKIP | 需真实 nft 环境；空 body 已测 Validate |
| 33 | [webfilter] webfilter 真 append | SKIP | webfilter/rules POST 为全量替换，需真实 dnsmasq；空 body 已测 Validate |

## 1. [radius] GET /api/v1/radius/config

- 结果: **PASS** — HTTP 200, 字段 ['config', 'servers'] 存在

```
{"conf": "auth_order radius\nlogin_tries 4\nauthserver 127.0.0.1:1812\nacctserver 127.0.0.1:1812\nservers /etc/radiusclient/servers\ndictionary /usr/share/radiusclient/dictionary\n", "config": {"se...
```

## 2. [nac] GET /api/v1/nac/config

- 结果: **PASS** — HTTP 200, 字段 ['config'] 存在

```
{"config": {"mode": "", "devices": null}, "nft": "table inet router_nac {\n\tchain forward {\n\t\ttype filter hook forward priority filter;\n\t\t# NAC: default accept; only matched devices are drop...
```

## 3. [nac] GET /api/v1/nac/neighbors

- 结果: **PASS** — HTTP 200, 字段 ['neighbors', 'count'] 存在

```
{"count": 34, "neighbors": [{"mac": "bc:24:11:76:6f:e2", "name": "ens18", "ip": "10.100.1.168", "policy": "", "note": "STALE"}, {"mac": "bc:24:11:92:50:52", "name": "ens18", "ip": "10.100.1.131", "...
```

## 4. [nac] GET /api/v1/nac/connlimits

- 结果: **PASS** — HTTP 200, 字段 ['limits'] 存在

```
{"limits": [], "nft": "table inet router_connlimit {\n\tchain forward {\n\t\ttype filter hook forward priority filter;\n\t\t# connlimit: default accept; only matched flows are dropped.\n\t\tct stat...
```

## 5. [nac] GET /api/v1/nac/top-connections

- 结果: **PASS** — HTTP 200

```
{"count": 20, "top": [{"ip": "127.0.0.1", "count": 29}, {"ip": "10.100.1.18", "count": 12}, {"ip": "10.100.1.110", "count": 7}, {"ip": "10.100.1.221", "count": 3}, {"ip": "172.217.221.95", "count":...
```

## 6. [identity] GET /api/v1/identity

- 结果: **PASS** — HTTP 200, 字段 ['identity'] 存在

```
{"identity": {"hostname": "", "domain": "", "location": ""}}
```

## 7. [quota] GET /api/v1/quota/policies

- 结果: **PASS** — HTTP 200, 字段 ['policies', 'count'] 存在

```
{"count": 0, "policies": [], "report": ""}
```

## 8. [quota] GET /api/v1/quota/report

- 结果: **PASS** — HTTP 200

```
{"count": 0, "report": [""]}
```

## 9. [quota] GET /api/v1/quota/nft

- 结果: **PASS** — HTTP 200

```
{"exists": false, "nft": "", "note": "内核中暂无 router_quota 表（尚未下发）"}
```

## 10. [webfilter] GET /api/v1/webfilter/rules

- 结果: **PASS** — HTTP 200, 字段 ['rules'] 存在

```
{"dnsmasq": "", "rules": []}
```

## 11. [dpi] GET /api/v1/dpi/rules

- 结果: **PASS** — HTTP 200, 字段 ['rules'] 存在

```
{"nft": "table inet router_dpi {\n  chain classify {\n    type filter hook forward priority mangle;\n  }\n}\n", "rules": []}
```

## 12. [dpi] GET /api/v1/dpi/marks

- 结果: **PASS** — HTTP 200, 字段 ['rules'] 存在

```
{"rules": []}
```

## 13. [dpi] GET /api/v1/dpi/flows

- 结果: **PASS** — HTTP 200, 字段 ['flows', 'count'] 存在

```
{"count": 0, "flows": []}
```

## 14. [dpi] GET /api/v1/dpi/apps

- 结果: **PASS** — HTTP 200, 字段 ['apps'] 存在

```
{"apps": {}}
```

## 15. [dpi] GET /api/v1/dpi/tracker-stats

- 结果: **PASS** — HTTP 200, 字段 ['stats'] 存在

```
{"stats": {"evicted": 0, "max": 10000, "total": 10000, "tracked": 0}}
```

## 16. [dpi] GET /api/v1/dpi/mark-rules

- 结果: **PASS** — HTTP 200, 字段 ['app_ports'] 存在

```
{"app_ports": {}, "nft": "table inet router_dpi {\n    chain classify {\n        type filter hook forward priority mangle; policy accept;\n    }\n}\n"}
```

## 17. 结构验证 /api/v1/nac/neighbors

- 结果: **PASS** — 34 条, 字段齐全

```
{"mac": "bc:24:11:76:6f:e2", "name": "ens18", "ip": "10.100.1.168", "policy": "", "note": "STALE"}
```

## 18. 结构验证 /api/v1/quota/policies

- 结果: **PASS** — 0 条（空或无 itemkeys）

```
{"count": 0, "policies": [], "report": ""}
```

## 19. 结构验证 /api/v1/webfilter/rules

- 结果: **PASS** — 0 条（空或无 itemkeys）

```
{"dnsmasq": "", "rules": []}
```

## 20. 结构验证 /api/v1/dpi/flows

- 结果: **PASS** — 0 条（空或无 itemkeys）

```
{"count": 0, "flows": []}
```

## 21. [radius] POST /api/v1/radius/config 空 body

- 结果: **FAIL** — 空 body 被接受 HTTP 200（Validate 未拦）

```
{"conf": "auth_order radius\nlogin_tries 4\nauthserver 127.0.0.1:1812\nacctserver 127.0.0.1:1812\nservers /etc/radiusclient/servers\ndictionary /usr/share/radiusclient/dictionary\n", "config": {"se...
```

## 22. [nac] POST /api/v1/nac/config 空 body

- 结果: **PASS** — HTTP 422 code=10151（已拒绝）

```
{"code": "10151", "detail": "mode must be off, whitelist or blacklist, got \"\"", "message": "validation failed"}
```

## 23. [nac] POST /api/v1/nac/connlimits 空 body

- 结果: **PASS** — HTTP 400 code=10150（已拒绝）

```
{"code": "10150", "detail": "json: cannot unmarshal object into Go value of type []nac.Limit", "message": "invalid body"}
```

## 24. [identity] POST /api/v1/identity 空 body

- 结果: **PASS** — HTTP 422 code=10363（已拒绝）

```
{"code": "10363", "detail": "hostname required", "message": "invalid identity"}
```

## 25. [quota] POST /api/v1/quota/policies 空 body

- 结果: **PASS** — HTTP 422 code=10173（已拒绝）

```
{"code": "10173", "detail": "to clear all quota policies, send {policies:[],confirm:true}", "message": "policies must not be empty"}
```

## 26. [webfilter] POST /api/v1/webfilter/rules 空 body

- 结果: **PASS** — HTTP 422 code=10121（已拒绝）

```
{"code": "10121", "detail": "POST /webfilter/rules must carry at least one rule", "message": "empty rule set"}
```

## 27. [dpi] POST /api/v1/dpi/rules 空 body

- 结果: **PASS** — HTTP 422 code=10043（已拒绝）

```
{"code": "10043", "detail": "rules must contain at least one entry (expected {\"rules\":[...]}", "message": "dpi mark apply failed"}
```

## 28. [dpi] POST /api/v1/dpi/marks 空 body

- 结果: **PASS** — HTTP 422 code=10043（已拒绝）

```
{"code": "10043", "detail": "rules must contain at least one entry (expected {\"rules\":[...]})", "message": "dpi mark apply failed"}
```

## 29. [radius] radius/test 真 RADIUS

- 结果: **SKIP** — radius/test 需真实 RADIUS server，本机无；radius/config 已读

```
N/A
```

## 30. [dpi] dpi/classify 真分类

- 结果: **SKIP** — dpi/classify 需查询参数 ip/proto/port；空参返回 400 属预期

```
N/A
```

## 31. [quota] quota/apply 真下发

- 结果: **SKIP** — quota/apply 需真实 nft 环境；空 body 已测 Validate

```
N/A
```

## 32. [nac] nac/connlimits/apply 真下发

- 结果: **SKIP** — 需真实 nft 环境；空 body 已测 Validate

```
N/A
```

## 33. [webfilter] webfilter 真 append

- 结果: **SKIP** — webfilter/rules POST 为全量替换，需真实 dnsmasq；空 body 已测 Validate

```
N/A
```
