# DHCP 租约全生命周期验证报告

> 生成: 2026-10-28 23:38:27

**汇总**: 7 步 | PASS 4 / FAIL 0 / SKIP 3

| # | 步骤 | 结果 | 说明 |
|---|------|------|------|
| 1 | 1 GET /api/v1/dhcp/config | PASS | HTTP 200 interface= range=- |
| 2 | 2 GET /api/v1/dhcp/leases | PASS | HTTP 200 count=0 |
| 3 | 3 GET /api/v1/dhcp/server | PASS | HTTP 200 enabled=False static_leases=0 |
| 4 | 4 GET /api/v1/dhcp/status | PASS | HTTP 200 config_exists=true |
| 5 | 5 租约字段结构完整 | SKIP | 当前无真实租约（router-re 自身 DHCP 未配置/无客户端），仅验证端点结构 |
| 6 | 6 静态保留生命周期 | SKIP | 未发现 /api/v1/dhcp/reservations 或 reservation 端点，无法验证建→确认→删 |
| 7 | 7 真实租约分配/续租/释放 | SKIP | router-re DHCP 未配置（range 空），无真实客户端；LXC dnsmasq 独立运行，不归属本 API |

## 1. 1 GET /api/v1/dhcp/config

- 结果: **PASS** — HTTP 200 interface= range=-

```
{"config": {"interface": "", "range_start": "", "range_end": "", "netmask": "", "gateway": "", "lease_time": "", "dns": null}, "dnsmasq": ""}
```

## 2. 2 GET /api/v1/dhcp/leases

- 结果: **PASS** — HTTP 200 count=0

```
{"count": 0, "leases": []}
```

## 3. 3 GET /api/v1/dhcp/server

- 结果: **PASS** — HTTP 200 enabled=False static_leases=0

```
{"applied": false, "config": {"interface": "", "range_start": "", "range_end": "", "netmask": "", "gateway": "", "lease_time": "", "dns": null, "domain": "", "static_leases": null, "enabled": false}}
```

## 4. 4 GET /api/v1/dhcp/status

- 结果: **PASS** — HTTP 200 config_exists=true

```
{"config_exists": "true", "detail": "2232 dnsmasq --conf-file=/dev/null -u dnsmasq --strict-order --bind-interfaces --pid-file=/run/lxc/dnsmasq.pid --listen-address 10.0.3.1 --dhcp-range 10.0.3.2,10.0.3.254 --dhcp-lease-max=253 --dhcp-no-override --except-interface=lo --interface=lxcbr0 --dhcp-le...
```

## 5. 5 租约字段结构完整

- 结果: **SKIP** — 当前无真实租约（router-re 自身 DHCP 未配置/无客户端），仅验证端点结构

```
leases=[]
```

## 6. 6 静态保留生命周期

- 结果: **SKIP** — 未发现 /api/v1/dhcp/reservations 或 reservation 端点，无法验证建→确认→删

```
N/A
```

## 7. 7 真实租约分配/续租/释放

- 结果: **SKIP** — router-re DHCP 未配置（range 空），无真实客户端；LXC dnsmasq 独立运行，不归属本 API

```
config empty
```
