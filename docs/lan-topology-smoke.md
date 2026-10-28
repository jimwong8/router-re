# 局域网拓扑模块冒烟报告 (lan-topology-smoke)

> 生成: 2026-10-28 21:21:29  BASE=http://127.0.0.1:9090

**汇总**: 共 18 步 / PASS 18 / FAIL 0 / SKIP 0

| 步骤 | 结果 | 详情 |
|---|---|---|
| GET /api/v1/vlan | PASS | HTTP 200 vlans=0 |
| GET /api/v1/bridge | PASS | HTTP 200 bridges=2 |
| GET /api/v1/bond | PASS | HTTP 200 live_keys=[] |
| GET /api/v1/mdns | PASS | HTTP 200 |
| GET /api/v1/mirror | PASS | HTTP 200 |
| GET /api/v1/ipv6/addresses | PASS | HTTP 200 addresses=4 |
| GET /api/v1/ipv6/status | PASS | HTTP 200 |
| GET /api/v1/ipv6/nd | PASS | HTTP 200 live_keys=['accept_ra', 'dad_transmits', 'forwarding', 'hop_limit', 'iface', 'mtu', 'reachable_time_ms', 'retrans_timer_ms'] |
| GET /api/v1/ipv6/pd | PASS | HTTP 200 |
| GET /api/v1/ipv6/ra | PASS | HTTP 200 |
| bond live 接口信息 | PASS | HTTP 200 live={} |
| ipv6/status 真实状态 | PASS | HTTP 200 keys=['addr_count', 'forwarding', 'ipv6_enabled', 'radvd_running'] |
| POST /api/v1/vlan (empty) | PASS | HTTP 422 code=10051 (blocked) |
| POST /api/v1/bridge (empty) | PASS | HTTP 422 code=10061 (blocked) |
| POST /api/v1/bond (empty) | PASS | HTTP 422 code=10331 (blocked) |
| POST /api/v1/mdns (empty) | PASS | HTTP 422 code=10348 (blocked) |
| POST /api/v1/mirror (empty) | PASS | HTTP 422 code=10197 (blocked) |
| POST /api/v1/ipv6/addresses (empty) | PASS | HTTP 422 code=10132 (blocked) |

## 真实返回片段

### GET /api/v1/vlan — PASS

```
{"vlans":[]} 
```

### GET /api/v1/bridge — PASS

```
{"bridges":[{"name":"lxcbr0","members":[],"address":"10.0.3.1/24","stp":false},{"name":"docker0","members":[],"address":"172.17.0.1/16","stp":false}]} 
```

### GET /api/v1/bond — PASS

```
{"config":{"name":"","slaves":null,"mode":"","miimon":0,"lacp_rate":"","xmit_hash":"","mtu":0},"live":{},"script":"ip link add  type bond mode active-backup\nip link set  up\n"} 
```

### GET /api/v1/mdns — PASS

```
{"avahi":"","config":{"in_ifaces":null,"out_ifaces":null,"enabled":false}} 
```

### GET /api/v1/mirror — PASS

```
{"cleanup":"tc qdisc del dev  ingress 2\u003e/dev/null || true\ntc qdisc del dev  root 2\u003e/dev/null || true\n","config":{"source_iface":"","target_iface":"","direction":"","enabled":false},"script":""} 
```

### GET /api/v1/ipv6/addresses — PASS

```
{"addresses":[{"iface":"ens18","addr":"2409:8a5c:6419:d991::ed7","prefix_len":128},{"iface":"ens18","addr":"2409:8a5c:6419:d991:c13:2c39:d3c4:ad62","prefix_len":64},{"iface":"lxcbr0","addr":"fc42:5009:ba4b:5ab0::1","prefix_len":64},{"iface":"tailscale0","addr":"fd7a:115c:a1e0::ea35:1c40","prefix_len...
```

### GET /api/v1/ipv6/status — PASS

```
{"addr_count":"4","forwarding":"1","ipv6_enabled":"yes","radvd_running":"no"} 
```

### GET /api/v1/ipv6/nd — PASS

```
{"config":{"iface":"","ra_interval_min":0,"ra_interval_max":0,"hop_limit":0,"mtu":0,"reachable_time_ms":0,"retrans_timer_ms":0,"dad_transmits":0,"accept_ra":false,"forwarding":false},"live":{"accept_ra":false,"dad_transmits":1,"forwarding":true,"hop_limit":64,"iface":"ens18","mtu":1500,"reachable_ti...
```

### GET /api/v1/ipv6/pd — PASS

```
{"config":{"upstream_iface":"","downstream_iface":"","prefix_len":0,"subnet_id_bits":0,"enabled":false},"dhcp6c":"# interface  {\n# \tsend rapid-commit;\n# \trequest domain-name-servers, domain-name;\n# \tsend ia-pd 0;\n# };\n# id-assoc pd 0 {\n# \tprefix-interface  {\n# \t\tid 0;\n# \t\tsla-id 0;\n...
```

### GET /api/v1/ipv6/ra — PASS

```
{"config":{"iface":"","enabled":false,"managed_flag":false,"other_flag":false,"max_interval":0,"prefix":"","prefix_len":0,"valid_lifetime":0,"preferred_lifetime":0},"radvd":"# interface  {\n# \tAdvSendAdvert on;\n# \tMinRtrAdvInterval 0.0;\n# \tMaxRtrAdvInterval 0;\n# \tAdvManagedFlag off;\n# \tAdvO...
```

### bond live 接口信息 — PASS

```
{"config":{"name":"","slaves":null,"mode":"","miimon":0,"lacp_rate":"","xmit_hash":"","mtu":0},"live":{},"script":"ip link add  type bond mode active-backup\nip link set  up\n"} 
```

### ipv6/status 真实状态 — PASS

```
{"addr_count":"4","forwarding":"1","ipv6_enabled":"yes","radvd_running":"no"} 
```

### POST /api/v1/vlan (empty) — PASS

```
{"code":"10051","detail":"expected a VLAN object or {\"vlans\":[...]}","message":"invalid vlan request"} 
```

### POST /api/v1/bridge (empty) — PASS

```
{"code":"10061","detail":"name required","message":"bridge validation failed"} 
```

### POST /api/v1/bond (empty) — PASS

```
{"code":"10331","detail":"name required","message":"invalid bond config"} 
```

### POST /api/v1/mdns (empty) — PASS

```
{"code":"10348","detail":"至少需要一个入接口 (in_ifaces)","message":"invalid mdns config"} 
```

### POST /api/v1/mirror (empty) — PASS

```
{"code":"10197","detail":"source_iface must not be empty","message":"invalid mirror config"} 
```

### POST /api/v1/ipv6/addresses (empty) — PASS

```
{"code":"10132","detail":"POST /ipv6/addresses must carry at least one address","message":"empty list"} 
```

## 说明

- 覆盖模块：vlan / bridge / bond / mdns / mirror / ipv6 (addresses/status/nd/pd/ra)。
- bond/bridge 无独立 status 端点，GET /bond 返回 live 字段作为当前接口信息。
- 写端点空 body 通常被 Validate 拦截 (400/422)；部分端点接受空值。

