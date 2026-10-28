# IPv6 真实连通性端到端测试

- Base: http://127.0.0.1:9090
- Date: 2026-10-28 22:54:49

## 1. GET /api/v1/ipv6/addresses
- HTTP 200, count=4
- 全局地址: 有
  - ens18 2409:8a5c:6419:d991::ed7/128
  - ens18 2409:8a5c:6419:d991:c13:2c39:d3c4:ad62/64
  - lxcbr0 fc42:5009:ba4b:5ab0::1/64
  - tailscale0 fd7a:115c:a1e0::ea35:1c40/128
- 结果: PASS

## 2. GET /api/v1/ipv6/status
- HTTP 200
  - addr_count: 4
  - forwarding: 1
  - ipv6_enabled: yes
  - radvd_running: no
- 结果: PASS

## 3. GET /api/v1/ipv6/nd
- HTTP 200
- live: {"accept_ra": false, "dad_transmits": 1, "forwarding": true, "hop_limit": 64, "iface": "ens18", "mtu": 1500, "reachable_time_ms": 0, "retrans_timer_ms": 0}
- 结果: PASS

## 4a. POST /api/v1/ipv6/addresses 空 body
- HTTP 422 (期望 422) → PASS

## 4b. POST /api/v1/ipv6/nd 空 body
- HTTP 422 (期望 400/422) → PASS

## 5. 接口 vs 系统状态对比
- 系统全局 IPv6 地址数: 4
- 接口返回全局地址数: 2
- 匹配数: 2
- 一致性: PASS

### 系统 ip -6 addr (全局地址)
  - 2409:8a5c:6419:d991::ed7
  - 2409:8a5c:6419:d991:c13:2c39:d3c4:ad62
  - fc42:5009:ba4b:5ab0::1
  - fd7a:115c:a1e0::ea35:1c40

### 接口返回 (全局地址)
  - 2409:8a5c:6419:d991::ed7
  - 2409:8a5c:6419:d991:c13:2c39:d3c4:ad62

## 6. 环境判定
- 本机有全局 IPv6: 是
- 环境有真实全局 IPv6，接口与系统一致，路由可测

## 7. 汇总

| # | 测试项 | 结果 | 详情 |
|---|--------|------|------|
| 1 | GET /api/v1/ipv6/addresses | PASS | HTTP 200, count=4, has_global=True |
| 2 | GET /api/v1/ipv6/status | PASS | HTTP 200, ipv6_enabled=yes, forwarding=1 |
| 3 | GET /api/v1/ipv6/nd | PASS | HTTP 200, live keys=['accept_ra', 'dad_transmits', 'forwarding', 'hop_limit', 'iface', 'mtu'] |
| 4 | POST /api/v1/ipv6/addresses (empty) | PASS | HTTP 422 (期望 422) |
| 5 | POST /api/v1/ipv6/nd (empty) | PASS | HTTP 422 (期望 400/422) |
| 6 | 接口 vs 系统地址一致性 | PASS | 接口全局=2, 系统全局=4, 匹配=2 |

- PASS: 6 / FAIL: 0 / SKIP: 0
