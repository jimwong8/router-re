# 分页一致性审计报告

> 生成: 2026-10-28 23:31:19  BASE=http://127.0.0.1:9090

**测了 65 个列表端点**
- 支持分页: 1 / 不支持: 0 / 无法判定(数据<=5): 64
- 重/漏: dup_total=3248, missing_total=3701
- PASS: 4, FAIL: 1

| endpoint | N | collected | dedup | dup | missing | supports | status |
|---|---|---|---|---|---|---|---|
| /api/v1/accounting/report | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/accounts | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/acl/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/alerts/rules | 1 | 1 | 1 | 0 | 0 | None | PASS |
| /api/v1/audit/events | 3708 | 3255 | 7 | 3248 | 3701 | True | FAIL |
| /api/v1/billing/bills | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/billing/plans | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/bridge | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/clients | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/cloud/devices | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/conntrack | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/conntrack/terminals | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/ddns/check | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/ddns/records | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/dhcp/leases | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/diag/interfaces | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/diagnostics/report | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/dpi/flows | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/dpi/marks | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/dpi/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/firewall/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/interfaces | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/ipv6/addresses | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/l7/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/lb/status | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/linkcheck/targets | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/logs | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/logs/sources | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/multiwan | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/nac/connlimits | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/nac/neighbors | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/nac/top-connections | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/online | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/policy/timeschedule | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/policyroute/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/portal/clients | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/portal/templates | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/portmap/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/porttrigger | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/pppoe/multidial | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/qos/limits | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/quota/policies | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/quota/report | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/routing/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/routing/tables | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/security/arp/neighbors | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/stats/history | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/stats/interfaces | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/system/processes | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/system/services | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/system/timezones | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/terminals | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/threat/list | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/traffic/rates | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/traffic/top | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/upnp/mappings | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/users | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/vlan | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/voucher/list | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/vpn/client | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/wan/links | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/wan/status | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/webfilter/rules | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/wireless/ssids | 0 | 0 | 0 | 0 | 0 | None | SKIP |
| /api/v1/wol/targets | 0 | 0 | 0 | 0 | 0 | None | SKIP |

## /api/v1/accounting/report — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=100000 -> HTTP 200 items=0 (N=0)

## /api/v1/accounts — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=100000 -> HTTP 200 items=0 (N=0)

## /api/v1/acl/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=100000 -> HTTP 200 items=0 (N=0)

## /api/v1/alerts/rules — PASS

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=100000 -> HTTP 200 items=1 (N=1)

## /api/v1/audit/events — FAIL

- N=3708 collected=3255 dedup=7 dup=3248 missing=3701 supports=True
- boundary:
  - limit=0 -> HTTP 0 items=0
  - offset>N -> HTTP 0 items=0
  - limit=100000 -> HTTP 0 items=0 (N=3708)

## /api/v1/billing/bills — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/billing/plans — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/bridge — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/clients — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/cloud/devices — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/conntrack — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/conntrack/terminals — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/ddns/check — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/ddns/records — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/dhcp/leases — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/diag/interfaces — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/diagnostics/report — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/dpi/flows — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/dpi/marks — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/dpi/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/firewall/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/interfaces — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/ipv6/addresses — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/l7/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/lb/status — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/linkcheck/targets — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/logs — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/logs/sources — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/multiwan — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/nac/connlimits — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/nac/neighbors — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/nac/top-connections — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/online — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/policy/timeschedule — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/policyroute/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/portal/clients — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/portal/templates — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/portmap/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/porttrigger — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/pppoe/multidial — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/qos/limits — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/quota/policies — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/quota/report — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/routing/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/routing/tables — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/security/arp/neighbors — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/stats/history — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/stats/interfaces — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/system/processes — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/system/services — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/system/timezones — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/terminals — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/threat/list — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/traffic/rates — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/traffic/top — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/upnp/mappings — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/users — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/vlan — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/voucher/list — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/vpn/client — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/wan/links — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/wan/status — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/webfilter/rules — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/wireless/ssids — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

## /api/v1/wol/targets — SKIP

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:

