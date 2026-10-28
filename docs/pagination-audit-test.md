# 分页一致性审计报告

> 生成: 2026-10-28 23:44:12  BASE=http://127.0.0.1:9090

**测了 65 个列表端点**
- 支持分页: 8 / 不支持: 30 / 无法判定(数据<=5): 27
- 重/漏: dup_total=8310, missing_total=1266
- PASS: 28, FAIL: 7

| endpoint | N | collected | dedup | dup | missing | supports | status |
|---|---|---|---|---|---|---|---|
| /api/v1/accounting/report | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/accounts | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/acl/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/alerts/rules | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/audit/events | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/billing/bills | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/billing/plans | 2 | 2 | 2 | 0 | 0 | False | NOPAGE |
| /api/v1/bridge | 2 | 2 | 2 | 0 | 0 | False | NOPAGE |
| /api/v1/clients | 22 | 220 | 22 | 198 | 0 | False | NOPAGE |
| /api/v1/cloud/devices | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/conntrack | 10 | 100 | 17 | 83 | 0 | False | NOPAGE |
| /api/v1/conntrack/terminals | 34 | 34 | 26 | 8 | 8 | True | FAIL |
| /api/v1/ddns/check | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/ddns/records | 5 | 5 | 5 | 0 | 0 | False | NOPAGE |
| /api/v1/dhcp/leases | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/diag/interfaces | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/diagnostics/report | 8 | 8 | 8 | 0 | 0 | True | PASS |
| /api/v1/dpi/flows | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/dpi/marks | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/dpi/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/firewall/rules | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/interfaces | 10 | 100 | 12 | 88 | 0 | False | NOPAGE |
| /api/v1/ipv6/addresses | 4 | 4 | 4 | 0 | 0 | False | NOPAGE |
| /api/v1/l7/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/lb/status | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/linkcheck/targets | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/logs | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/logs/sources | 30 | 300 | 30 | 270 | 0 | False | NOPAGE |
| /api/v1/multiwan | 2 | 2 | 2 | 0 | 0 | False | NOPAGE |
| /api/v1/nac/connlimits | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/nac/neighbors | 34 | 340 | 34 | 306 | 0 | False | NOPAGE |
| /api/v1/nac/top-connections | 20 | 200 | 29 | 171 | 0 | True | FAIL |
| /api/v1/online | 22 | 230 | 23 | 207 | 0 | True | FAIL |
| /api/v1/policy/timeschedule | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/policyroute/rules | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/portal/clients | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/portal/templates | 4 | 4 | 4 | 0 | 0 | False | NOPAGE |
| /api/v1/portmap/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/porttrigger | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/pppoe/multidial | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/qos/limits | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/quota/policies | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/quota/report | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/routing/rules | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/routing/tables | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/security/arp/neighbors | 34 | 340 | 34 | 306 | 0 | False | NOPAGE |
| /api/v1/stats/history | 26 | 260 | 26 | 234 | 0 | False | NOPAGE |
| /api/v1/stats/interfaces | 10 | 100 | 11 | 89 | 0 | True | FAIL |
| /api/v1/system/processes | 887 | 150 | 66 | 84 | 821 | True | FAIL |
| /api/v1/system/services | 526 | 100 | 100 | 0 | 426 | True | FAIL |
| /api/v1/system/timezones | 601 | 6010 | 601 | 5409 | 0 | False | NOPAGE |
| /api/v1/terminals | 39 | 39 | 28 | 11 | 11 | True | FAIL |
| /api/v1/threat/list | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/traffic/rates | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/traffic/top | 10 | 100 | 10 | 90 | 0 | False | NOPAGE |
| /api/v1/upnp/mappings | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/users | 2 | 2 | 2 | 0 | 0 | False | NOPAGE |
| /api/v1/vlan | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/voucher/list | 34 | 340 | 34 | 306 | 0 | False | NOPAGE |
| /api/v1/vpn/client | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/wan/links | 2 | 2 | 2 | 0 | 0 | False | NOPAGE |
| /api/v1/wan/status | 1 | 1 | 1 | 0 | 0 | False | NOPAGE |
| /api/v1/webfilter/rules | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/wireless/ssids | 0 | 0 | 0 | 0 | 0 | None | PASS |
| /api/v1/wol/targets | 0 | 0 | 0 | 0 | 0 | None | PASS |

## /api/v1/accounting/report — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/accounts — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/acl/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/alerts/rules — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/audit/events — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=100
  - offset>N -> HTTP 200 items=5
  - limit=1000 -> HTTP 200 items=100 (N=10)

## /api/v1/billing/bills — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/billing/plans — NOPAGE

- N=2 collected=2 dedup=2 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=2
  - offset>N -> HTTP 200 items=2
  - limit=1000 -> HTTP 200 items=2 (N=2)

## /api/v1/bridge — NOPAGE

- N=2 collected=2 dedup=2 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=2
  - offset>N -> HTTP 200 items=2
  - limit=1000 -> HTTP 200 items=2 (N=2)

## /api/v1/clients — NOPAGE

- N=22 collected=220 dedup=22 dup=198 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=22
  - offset>N -> HTTP 200 items=22
  - limit=1000 -> HTTP 200 items=22 (N=22)

## /api/v1/cloud/devices — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/conntrack — NOPAGE

- N=10 collected=100 dedup=17 dup=83 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=100
  - offset>N -> HTTP 200 items=5
  - limit=1000 -> HTTP 200 items=100 (N=10)

## /api/v1/conntrack/terminals — FAIL

- N=34 collected=34 dedup=26 dup=8 missing=8 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=34
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=34 (N=34)

## /api/v1/ddns/check — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/ddns/records — NOPAGE

- N=5 collected=5 dedup=5 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=5
  - offset>N -> HTTP 200 items=5
  - limit=1000 -> HTTP 200 items=5 (N=5)

## /api/v1/dhcp/leases — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/diag/interfaces — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/diagnostics/report — PASS

- N=8 collected=8 dedup=8 dup=0 missing=0 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=8
  - offset>N -> HTTP 200 items=8
  - limit=1000 -> HTTP 200 items=8 (N=8)

## /api/v1/dpi/flows — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/dpi/marks — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/dpi/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/firewall/rules — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/interfaces — NOPAGE

- N=10 collected=100 dedup=12 dup=88 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/ipv6/addresses — NOPAGE

- N=4 collected=4 dedup=4 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=4
  - offset>N -> HTTP 200 items=4
  - limit=1000 -> HTTP 200 items=4 (N=4)

## /api/v1/l7/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/lb/status — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/linkcheck/targets — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/logs — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=100
  - offset>N -> HTTP 200 items=5
  - limit=1000 -> HTTP 200 items=100 (N=10)

## /api/v1/logs/sources — NOPAGE

- N=30 collected=300 dedup=30 dup=270 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=30
  - offset>N -> HTTP 200 items=30
  - limit=1000 -> HTTP 200 items=30 (N=30)

## /api/v1/multiwan — NOPAGE

- N=2 collected=2 dedup=2 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=2
  - offset>N -> HTTP 200 items=2
  - limit=1000 -> HTTP 200 items=2 (N=2)

## /api/v1/nac/connlimits — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/nac/neighbors — NOPAGE

- N=34 collected=340 dedup=34 dup=306 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=34
  - offset>N -> HTTP 200 items=34
  - limit=1000 -> HTTP 200 items=34 (N=34)

## /api/v1/nac/top-connections — FAIL

- N=20 collected=200 dedup=29 dup=171 missing=0 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=20
  - offset>N -> HTTP 200 items=20
  - limit=1000 -> HTTP 200 items=20 (N=20)

## /api/v1/online — FAIL

- N=22 collected=230 dedup=23 dup=207 missing=0 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=23
  - offset>N -> HTTP 200 items=23
  - limit=1000 -> HTTP 200 items=23 (N=22)

## /api/v1/policy/timeschedule — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/policyroute/rules — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/portal/clients — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/portal/templates — NOPAGE

- N=4 collected=4 dedup=4 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=4
  - offset>N -> HTTP 200 items=4
  - limit=1000 -> HTTP 200 items=4 (N=4)

## /api/v1/portmap/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/porttrigger — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/pppoe/multidial — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/qos/limits — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/quota/policies — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/quota/report — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/routing/rules — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/routing/tables — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/security/arp/neighbors — NOPAGE

- N=34 collected=340 dedup=34 dup=306 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=34
  - offset>N -> HTTP 200 items=34
  - limit=1000 -> HTTP 200 items=34 (N=34)

## /api/v1/stats/history — NOPAGE

- N=26 collected=260 dedup=26 dup=234 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=26
  - offset>N -> HTTP 200 items=26
  - limit=1000 -> HTTP 200 items=26 (N=26)

## /api/v1/stats/interfaces — FAIL

- N=10 collected=100 dedup=11 dup=89 missing=0 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/system/processes — FAIL

- N=887 collected=150 dedup=66 dup=84 missing=821 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=15
  - offset>N -> HTTP 200 items=15
  - limit=1000 -> HTTP 200 items=15 (N=887)

## /api/v1/system/services — FAIL

- N=526 collected=100 dedup=100 dup=0 missing=426 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=200
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=100 (N=526)

## /api/v1/system/timezones — NOPAGE

- N=601 collected=6010 dedup=601 dup=5409 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=601
  - offset>N -> HTTP 200 items=601
  - limit=1000 -> HTTP 200 items=601 (N=601)

## /api/v1/terminals — FAIL

- N=39 collected=39 dedup=28 dup=11 missing=11 supports=True
- boundary:
  - limit=0 -> HTTP 200 items=39
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=39 (N=39)

## /api/v1/threat/list — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/traffic/rates — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/traffic/top — NOPAGE

- N=10 collected=100 dedup=10 dup=90 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=10
  - offset>N -> HTTP 200 items=10
  - limit=1000 -> HTTP 200 items=10 (N=10)

## /api/v1/upnp/mappings — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/users — NOPAGE

- N=2 collected=2 dedup=2 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=2
  - offset>N -> HTTP 200 items=2
  - limit=1000 -> HTTP 200 items=2 (N=2)

## /api/v1/vlan — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/voucher/list — NOPAGE

- N=34 collected=340 dedup=34 dup=306 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=34
  - offset>N -> HTTP 200 items=34
  - limit=1000 -> HTTP 200 items=34 (N=34)

## /api/v1/vpn/client — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/wan/links — NOPAGE

- N=2 collected=2 dedup=2 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=2
  - offset>N -> HTTP 200 items=2
  - limit=1000 -> HTTP 200 items=2 (N=2)

## /api/v1/wan/status — NOPAGE

- N=1 collected=1 dedup=1 dup=0 missing=0 supports=False
- boundary:
  - limit=0 -> HTTP 200 items=1
  - offset>N -> HTTP 200 items=1
  - limit=1000 -> HTTP 200 items=1 (N=1)

## /api/v1/webfilter/rules — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/wireless/ssids — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

## /api/v1/wol/targets — PASS

- N=0 collected=0 dedup=0 dup=0 missing=0 supports=None
- boundary:
  - limit=0 -> HTTP 200 items=0
  - offset>N -> HTTP 200 items=0
  - limit=1000 -> HTTP 200 items=0 (N=0)

