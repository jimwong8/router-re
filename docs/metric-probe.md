# 接口超时/慢查询探测报告

> 生成: 2026-10-28 23:20:59  BASE=http://127.0.0.1:9090

**测了 142 个无参 GET 端点**
- SLOW (>5s): 0
- VERY SLOW (>10s): 0
- 最慢: /api/v1/ddns/check = 591 ms (无 >5s)

## 1. 所有端点耗时 (ms, 降序)

| endpoint | HTTP | ms |
|---|---|---|
| /api/v1/ddns/check | 200 | 591 |
| /api/v1/ddns/ip | 200 | 268 |
| /api/v1/remote/access/status | 200 | 249 |
| /api/v1/ddns/status | 200 | 197 |
| /api/v1/pppoe/sessions | 200 | 166 |
| /api/v1/system/processes | 200 | 156 |
| /api/v1/snmp/status | 200 | 148 |
| /api/v1/dhcp/status | 200 | 136 |
| /api/v1/firewall/log/suggestions | 200 | 112 |
| /api/v1/firewall/log/analysis | 200 | 84 |
| /api/v1/pppoe/status | 200 | 84 |
| /api/v1/diagnostics/report | 200 | 70 |
| /api/v1/diagnostics/report/text | 200 | 65 |
| /api/v1/logs | 200 | 41 |
| /api/v1/nac/top-connections | 200 | 33 |
| /api/v1/ntp/status | 200 | 32 |
| /api/v1/ipv6/status | 200 | 31 |
| /api/v1/system/services | 200 | 31 |
| /api/v1/audit/events | 200 | 24 |
| /api/v1/ipv6/nd | 200 | 23 |
| /api/v1/dns/status | 200 | 21 |
| /api/v1/system/time | 200 | 20 |
| /api/v1/upnp/status | 200 | 16 |
| /api/v1/conntrack | 200 | 16 |
| /api/v1/bridge | 200 | 15 |
| /api/v1/security/arp | 200 | 13 |
| /api/v1/terminals | 200 | 11 |
| /api/v1/online | 200 | 10 |
| /api/v1/quota/nft | 200 | 10 |
| /api/v1/diag/interfaces | 200 | 8 |
| /api/v1/conntrack/terminals | 200 | 8 |
| /api/v1/security/arp/neighbors | 200 | 8 |
| /api/v1/portmap/rules | 200 | 8 |
| /api/v1/nac/neighbors | 200 | 8 |
| /api/v1/security/attack-guard | 200 | 7 |
| /api/v1/upnp/mappings | 200 | 7 |
| /api/v1/ipv6/addresses | 200 | 7 |
| /api/v1/accounting/status | 200 | 7 |
| /api/v1/system/timezones | 200 | 6 |
| /api/v1/policyroute/rules | 200 | 6 |
| /api/v1/clients | 200 | 5 |
| /api/v1/wan/status | 200 | 5 |
| /api/v1/vlan | 200 | 5 |
| /api/v1/ddns/records | 200 | 4 |
| /api/v1/interfaces | 200 | 3 |
| /api/v1/vpn/client | 200 | 3 |
| /api/v1/stats/current | 200 | 3 |
| /api/v1/route/bgp | 200 | 2 |
| /api/v1/firewall/rules | 200 | 2 |
| /api/v1/voucher/list | 200 | 2 |
| /api/v1/dns/config | 200 | 2 |
| /api/v1/config/export | 200 | 2 |
| /api/v1/dpi/apps | 200 | 2 |
| /api/v1/security/ddos | 200 | 2 |
| /api/v1/system/info | 200 | 2 |
| /api/v1/acl/rules | 200 | 2 |
| /api/v1/ddns/config | 200 | 2 |
| /api/v1/l7/rules | 200 | 2 |
| /api/v1/firmware/inspect | 422 | 2 |
| /api/v1/pppoe/users | 200 | 2 |
| /api/v1/ipv6/pd | 200 | 2 |
| /api/v1/radius/config | 200 | 2 |
| /api/v1/quota/policies | 200 | 2 |
| /api/v1/webadmin | 200 | 2 |
| /api/v1/nat/rules | 200 | 2 |
| /api/v1/accounting/report | 200 | 1 |
| /api/v1/policy/timeschedule | 200 | 1 |
| /api/v1/portal/clients | 200 | 1 |
| /api/v1/billing/bills | 200 | 1 |
| /api/v1/route/ospf | 200 | 1 |
| /api/v1/health | 200 | 1 |
| /api/v1/porttrigger | 200 | 1 |
| /api/v1/nac/connlimits | 200 | 1 |
| /api/v1/dhcp/config | 200 | 1 |
| /api/v1/ntp/config | 200 | 1 |
| /api/v1/logs/rotate | 200 | 1 |
| /api/v1/stats/interfaces | 200 | 1 |
| /api/v1/nac/config | 200 | 1 |
| /api/v1/dpi/classify | 400 | 1 |
| /api/v1/conntrack/acct | 200 | 1 |
| /api/v1/traffic/rates | 200 | 1 |
| /api/v1/accounts | 200 | 1 |
| /api/v1/cert | 200 | 1 |
| /api/v1/htb/config | 200 | 1 |
| /api/v1/lb/status | 200 | 1 |
| /api/v1/notify/trap | 200 | 1 |
| /api/v1/wan/links | 200 | 1 |
| /api/v1/alerts/rules | 200 | 1 |
| /api/v1/dpi/flows | 200 | 1 |
| /api/v1/linkcheck/status | 200 | 1 |
| /api/v1/dpi/rules | 200 | 1 |
| /api/v1/qos/limits | 200 | 1 |
| /api/v1/portal/config | 200 | 1 |
| /api/v1/dpi/mark-rules | 200 | 1 |
| /api/v1/ipv6/ra | 200 | 1 |
| /api/v1/quota/report | 200 | 1 |
| /api/v1/cloud/devices | 200 | 1 |
| /api/v1/stats/history | 200 | 1 |
| /api/v1/dpi/tracker-stats | 200 | 1 |
| /api/v1/webfilter/rules | 200 | 1 |
| /api/v1/users | 200 | 1 |
| /api/v1/threat/stats | 200 | 1 |
| /api/v1/billing/plans | 200 | 1 |
| /api/v1/routing/tables | 200 | 1 |
| /api/v1/portal/preview | 200 | 1 |
| /api/v1/bond | 200 | 1 |
| /api/v1/notify/smtp | 200 | 1 |
| /api/v1/mirror | 200 | 1 |
| /api/v1/snmp/config | 200 | 1 |
| /api/v1/alg | 200 | 1 |
| /api/v1/dpi/marks | 200 | 1 |
| /api/v1/routing/rules | 200 | 1 |
| /api/v1/ips/blocklist | 200 | 1 |
| /api/v1/interfaces/config | 200 | 1 |
| /api/v1/threat/list | 200 | 1 |
| /api/v1/route/static | 200 | 1 |
| /api/v1/lb/preview | 422 | 1 |
| /api/v1/ips/rules | 200 | 1 |
| /api/v1/pppoe/multidial | 200 | 1 |
| /api/v1/ips/stats | 200 | 1 |
| /api/v1/portal/templates | 200 | 1 |
| /api/v1/iface/tune | 422 | 1 |
| /api/v1/wol/targets | 200 | 1 |
| /api/v1/dhcp/server | 200 | 1 |
| /api/v1/htb/report | 200 | 1 |
| /api/v1/dhcp/leases | 200 | 1 |
| /api/v1/wireless/ssids | 200 | 1 |
| /api/v1/identity | 200 | 1 |
| /api/v1/htb/show | 422 | 1 |
| /api/v1/qos/client-limits | 200 | 1 |
| /api/v1/multiwan | 200 | 1 |
| /api/v1/mdns | 200 | 1 |
| /api/v1/linkcheck/targets | 200 | 1 |
| /api/v1/pppoe/config | 200 | 1 |
| /api/v1/lb/config | 200 | 1 |
| /api/v1/logs/syslog | 200 | 1 |
| /api/v1/remote/access | 200 | 1 |
| /api/v1/billing/summary | 200 | 1 |
| /api/v1/logs/sources | 200 | 1 |
| /api/v1/traffic/top | 200 | 1 |
| /api/v1/upnp/config | 200 | 1 |
| /api/v1/l7/stats | 200 | 1 |

## 2. SLOW 端点探测 (加大 page size)

(无 SLOW 端点)

## 3. diag/ping 外部依赖耗时

| host | HTTP | ms |
|---|---|---|
| 8.8.8.8 | 200 | 2096 |
| 127.0.0.1 | 200 | 2038 |

