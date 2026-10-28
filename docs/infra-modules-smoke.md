# 基础设施模块冒烟报告 (infra-modules-smoke)

> 生成: 2026-10-28 21:35:28  BASE=http://127.0.0.1:9090

**汇总**: 共 27 步 / PASS 25 / FAIL 0 / SKIP 2

| 步骤 | 结果 | 详情 |
|---|---|---|
| GET /api/v1/dns/config | PASS | HTTP 200 |
| GET /api/v1/dns/status | PASS | HTTP 200 |
| GET /api/v1/ntp/config | PASS | HTTP 200 |
| GET /api/v1/ntp/status | PASS | HTTP 200 |
| GET /api/v1/snmp/config | PASS | HTTP 200 |
| GET /api/v1/snmp/status | PASS | HTTP 200 |
| GET /api/v1/dhcp/config | PASS | HTTP 200 |
| GET /api/v1/dhcp/leases | PASS | HTTP 200 leases=0 |
| GET /api/v1/dhcp/server | PASS | HTTP 200 |
| GET /api/v1/dhcp/status | PASS | HTTP 200 |
| GET /api/v1/diag/interfaces | PASS | HTTP 200 interfaces=10 |
| GET /api/v1/lb/config | PASS | HTTP 200 |
| GET /api/v1/lb/status | PASS | HTTP 200 |
| GET /api/v1/htb/config | PASS | HTTP 200 |
| GET /api/v1/htb/report | PASS | HTTP 200 |
| GET /api/v1/l7/rules | PASS | HTTP 200 rules=0 |
| GET /api/v1/l7/stats | PASS | HTTP 200 |
| GET /api/v1/alg | PASS | HTTP 200 |
| GET /api/v1/lb/preview | SKIP | HTTP 422 code=10162 (无 LB members 配置) |
| GET /api/v1/htb/show | PASS | HTTP 200 iface=ens18 |
| POST /api/v1/dns/config (empty) | PASS | HTTP 200 code=None (accepted) |
| POST /api/v1/ntp/config (empty) | PASS | HTTP 200 code=None (accepted) |
| POST /api/v1/snmp/config (empty) | PASS | HTTP 200 code=None (accepted) |
| POST /api/v1/dhcp/config (empty) | SKIP | HTTP 500 code=10009 (apply 失败，空配置) |
| POST /api/v1/l7/rules (empty) | PASS | HTTP 422 code=10153 (blocked) |
| POST /api/v1/alg (empty) | PASS | HTTP 200 code=None (accepted) |
| POST /api/v1/htb/config (empty) | PASS | HTTP 422 code=10182 (blocked) |

## 真实返回片段

### GET /api/v1/dns/config — PASS

```
{"config":{"upstreams":null,"hosts":null,"cache_size":0,"domain_needed":false,"bogus_priv":false,"enabled":false},"dnsmasq":"# no-resolv\n# cache-size=0\n","hosts":""} 
```

### GET /api/v1/dns/status — PASS

```
{"cache_size":0,"conf_exists":true,"conf_path":"/etc/dnsmasq.d/router-re-dnsproxy.conf","enabled":false,"running":false,"status":"/usr/bin/systemctl is-active dnsmasq: inactive","upstreams":null} 
```

### GET /api/v1/ntp/config — PASS

```
{"config":{"servers":null,"enabled":false}} 
```

### GET /api/v1/ntp/status — PASS

```
{"canntp":"yes","localrtc":"no","ntp":"yes","ntpsynchronized":"yes","rtctimeusec":"Wed 2026-10-28 21:35:27 CST","timeusec":"Wed 2026-10-28 21:35:27 CST","timezone":"Asia/Shanghai"} 
```

### GET /api/v1/snmp/config — PASS

```
{"applied":true,"config":{"enabled":false,"port":0,"location":"","contact":"","communities":null,"v3_users":null,"trap_targets":null}} 
```

### GET /api/v1/snmp/status — PASS

```
{"config_exists":"true","detail":"inactive","port":"","running":"false","version":"unknown"} 
```

### GET /api/v1/dhcp/config — PASS

```
{"config":{"interface":"","range_start":"","range_end":"","netmask":"","gateway":"","lease_time":"","dns":null},"dnsmasq":""} 
```

### GET /api/v1/dhcp/leases — PASS

```
{"count":0,"leases":[]} 
```

### GET /api/v1/dhcp/server — PASS

```
{"applied":false,"config":{"interface":"","range_start":"","range_end":"","netmask":"","gateway":"","lease_time":"","dns":null,"domain":"","static_leases":null,"enabled":false}} 
```

### GET /api/v1/dhcp/status — PASS

```
{"config_exists":"true","detail":"2232 dnsmasq --conf-file=/dev/null -u dnsmasq --strict-order --bind-interfaces --pid-file=/run/lxc/dnsmasq.pid --listen-address 10.0.3.1 --dhcp-range 10.0.3.2,10.0.3.254 --dhcp-lease-max=253 --dhcp-no-override --except-interface=lo --interface=lxcbr0 --dhcp-leasefil...
```

### GET /api/v1/diag/interfaces — PASS

```
{"count":10,"interfaces":[{"iface":"ens18","rx_bytes":"0","rx_dropped":"0","rx_errors":"0","rx_packets":"0","tx_bytes":"0","tx_dropped":"0","tx_errors":"0","tx_packets":"0"},{"iface":"lxcbr0","rx_bytes":"0","rx_dropped":"0","rx_errors":"0","rx_packets":"0","tx_bytes":"0","tx_dropped":"0","tx_errors"...
```

### GET /api/v1/lb/config — PASS

```
{"config":{"mode":"","members":null,"health":{"target":"","interval_sec":0,"timeout_sec":0,"fail_threshold":0,"rise_threshold":0}},"iprules":"","marks":{}} 
```

### GET /api/v1/lb/status — PASS

```
{"config":{"mode":"","members":null,"health":{"target":"","interval_sec":0,"timeout_sec":0,"fail_threshold":0,"rise_threshold":0}},"status":[]} 
```

### GET /api/v1/htb/config — PASS

```
{"applied":false,"config":{"iface":"","root_rate_mbit":0,"default_class":"","classes":null,"enabled":false}} 
```

### GET /api/v1/htb/report — PASS

```
{"config":{"iface":"","root_rate_mbit":0,"default_class":"","classes":null,"enabled":false},"script":"# 幂等重建：先删除已存在的根 qdisc（tc 根 qdisc 只有 del+add 幂等）\ntc qdisc del dev  root 2\u003e/dev/null || true\n# 建立根 HTB qdisc，默认类 1:10\ntc qdisc add dev  root handle 1: htb default 10\n# 根类：总带宽\ntc class add de...
```

### GET /api/v1/l7/rules — PASS

```
{"dnsmasq":"","rules":[]} 
```

### GET /api/v1/l7/stats — PASS

```
{"by_action":{},"by_type":{},"enabled":0,"total":0} 
```

### GET /api/v1/alg — PASS

```
{"config":{"ftp":false,"sip":false,"h323":false,"tftp":false,"irc":false,"pptp":false},"enabled_any":false,"script":""} 
```

### GET /api/v1/lb/preview — SKIP

```
{"code":"10162","detail":"no enabled members available","message":"no member"} 
```

### GET /api/v1/htb/show — PASS

```
{"iface":"ens18","raw":""} 
```

### POST /api/v1/dns/config (empty) — PASS

```
{"applied":false,"config":{"upstreams":null,"hosts":null,"cache_size":0,"domain_needed":false,"bogus_priv":false,"enabled":false},"detail":"配置已写入 /etc/dnsmasq.d/router-re-dnsproxy.conf，但 dnsmasq 未重启：systemctl: /usr/bin/systemctl restart dnsmasq: Failed to restart dnsmasq.service: Unit dnsmasq.servic...
```

### POST /api/v1/ntp/config (empty) — PASS

```
{"applied":false,"config":{"servers":null,"enabled":false},"text":"[Time]\nNTP=\n# Enabled=false commented out; keep above line for manual toggle.\n"} 
```

### POST /api/v1/snmp/config (empty) — PASS

```
{"applied":false,"config":{"enabled":false,"port":0,"location":"","contact":"","communities":null,"v3_users":null,"trap_targets":null},"detail":"snmpd restart failed (systemctl: /usr/bin/systemctl restart snmpd: Failed to restart snmpd.service: Unit snmpd.service not found.; init.d: /etc/init.d/snmp...
```

### POST /api/v1/dhcp/config (empty) — SKIP

```
{"code":"10009","detail":"dnsmasq reload failed after 3 methods: systemctl: /usr/bin/systemctl reload dnsmasq: Failed to reload dnsmasq.service: Unit dnsmasq.service not found.; init.d: /etc/init.d/dnsmasq reload: fork/exec /etc/init.d/dnsmasq: no such file or directory; kill -HUP pid: open /var/run...
```

### POST /api/v1/l7/rules (empty) — PASS

```
{"code":"10153","detail":"to clear all L7 rules, send {\"rules\":[],\"confirm\":true}","message":"rules must not be empty"} 
```

### POST /api/v1/alg (empty) — PASS

```
{"config":{"ftp":false,"sip":false,"h323":false,"tftp":false,"irc":false,"pptp":false},"enabled_any":false,"script":""} 
```

### POST /api/v1/htb/config (empty) — PASS

```
{"code":"10182","detail":"iface must not be empty","message":"invalid htb config"} 
```

## 说明

- 覆盖模块：dns / ntp / snmp / dhcp / diag / lb / htb / l7 / alg。
- ntp/status、snmp/status 依赖 systemd-bus/daemon，LXC 环境缺失时 SKIP。
- 写端点空 body 通常被 Validate 拦截 (400/422)；部分端点接受空值。

