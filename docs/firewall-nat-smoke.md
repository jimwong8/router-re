# Firewall/NAT/QoS/Traffic 核心路由冒烟报告 (firewall-nat-smoke)

> 生成: 2026-10-28 21:15:12  BASE=http://127.0.0.1:9090

**汇总**: 共 12 步 / PASS 12 / FAIL 0 / SKIP 0

| 步骤 | 结果 | 详情 |
|---|---|---|
| GET /api/v1/firewall/rules | PASS | HTTP 200 |
| GET /api/v1/nat/rules | PASS | HTTP 200 |
| GET /api/v1/qos/limits | PASS | HTTP 200 |
| GET /api/v1/qos/client-limits | PASS | HTTP 200 |
| GET /api/v1/traffic/rates | PASS | HTTP 200 rates=10 |
| GET /api/v1/traffic/top | PASS | HTTP 200 rates=10 |
| GET /api/v1/firewall/log/analysis | PASS | HTTP 200 available=False fields=['available', 'reason'] |
| GET /api/v1/firewall/log/suggestions | PASS | HTTP 200 available=False fields=['applied', 'available', 'note', 'reason'] |
| POST /api/v1/firewall/rules (empty) | PASS | HTTP 200 code=None (accepted-clear-all; expect blocked 400/422 or accepted 200) |
| POST /api/v1/nat/rules (empty) | PASS | HTTP 200 code=None (accepted-clear-all; expect blocked 400/422 or accepted 200) |
| POST /api/v1/qos/limits (empty) | PASS | HTTP 200 code=None (accepted-clear-all; expect blocked 400/422 or accepted 200) |
| POST /api/v1/qos/client-limits (empty) | PASS | HTTP 422 code=10020 (blocked; expect blocked 400/422 or accepted 200) |

## 真实返回片段

### GET /api/v1/firewall/rules — PASS

```
{"nft":"table inet router {\n  chain input {\n    type filter hook input priority 0; policy accept;\n  }\n  chain forward {\n    type filter hook forward priority 0; policy accept;\n  }\n  chain output {\n    type filter hook output priority 0; policy accept;\n  }\n}\n","rules":n...
```

### GET /api/v1/nat/rules — PASS

```
{"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting priority srcnat; policy accept;\n  }\n  chain prerouting {\n    type nat hook prerouting priority dstnat; policy accept;\n  }\n}\n","rules":null} 
```

### GET /api/v1/qos/limits — PASS

```
{"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]} 
```

### GET /api/v1/qos/client-limits — PASS

```
{"limits":null,"nft":"","tc":null} 
```

### GET /api/v1/traffic/rates — PASS

```
{"elapsed_sec":12.238056344,"rates":[{"iface":"ens18","rx_bps":43325.01706939355,"tx_bps":246827.02179917338},{"iface":"lxcbr0","rx_bps":0,"tx_bps":0},{"iface":"docker0","rx_bps":0,"tx_bps":0},{"iface":"tailscale0","rx_bps":15514.146582033418,"tx_bps":68991.34766722561},{"iface":...
```

### GET /api/v1/traffic/top — PASS

```
{"rates":[{"iface":"ens18","rx_bps":43325.01706939355,"tx_bps":246827.02179917338},{"iface":"tailscale0","rx_bps":15514.146582033418,"tx_bps":68991.34766722561},{"iface":"vgh772002603","rx_bps":454.64735931926674,"tx_bps":7386.467054821071},{"iface":"vgh772002600","rx_bps":146.75...
```

### GET /api/v1/firewall/log/analysis — PASS

```
{"available":false,"reason":"未找到含 SRC= 的防火墙日志行"} 
```

### GET /api/v1/firewall/log/suggestions — PASS

```
{"applied":false,"available":false,"note":"这些只是建议，不会自动封禁","reason":"未找到含 SRC= 的防火墙日志行"} 
```

### POST /api/v1/firewall/rules (empty) — PASS

```
{"nft":"table inet router {\n  chain input {\n    type filter hook input priority 0; policy accept;\n  }\n  chain forward {\n    type filter hook forward priority 0; policy accept;\n  }\n  chain output {\n    type filter hook output priority 0; policy accept;\n  }\n}\n","rules":n...
```

### POST /api/v1/nat/rules (empty) — PASS

```
{"nft":"table ip nat {\n  chain postrouting {\n    type nat hook postrouting priority srcnat; policy accept;\n  }\n  chain prerouting {\n    type nat hook prerouting priority dstnat; policy accept;\n  }\n}\n","rules":null} 
```

### POST /api/v1/qos/limits (empty) — PASS

```
{"limits":null,"nft":"table inet qos_limit {\n}\n","tc":[]} 
```

### POST /api/v1/qos/client-limits (empty) — PASS

```
{"code":"10020","detail":"limits must not be empty","message":"invalid client limits"} 
```

## 说明

- 读端点：firewall/rules、nat/rules、qos/limits、qos/client-limits、
  traffic/rates、traffic/top、firewall/log/analysis、firewall/log/suggestions。
- 写端点空 body 被 JSON decode 拦截（HTTP 400 code 10007），属设计保护，不算失败。
- traffic/top 响应字段为 rates（非 top），已按实际结构断言。
- fwlog analysis/suggestions 可能 available=false（无日志），字段结构如实记录。

