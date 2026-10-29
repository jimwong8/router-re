# Field Contract Report

PASS: 126
FAIL: 36

## Missing fields (frontend reads, backend may not return)

- /api/v1/vpn/client: `peers`
- /api/v1/vpn/client: `peer_count`
- /api/v1/vpn/client: `live_error`
- /api/v1/vpn/client: `has_config`
- /api/v1/cert: `days_left`
- /api/v1/cert: `warning`
- /api/v1/cert: `valid`
- /api/v1/conntrack/terminals: `items`
- /api/v1/system/services: `items`
- /api/v1/ips/rules: `fail`
- /api/v1/ips/rules: `hits`
- /api/v1/ips/test: `hits`
- /api/v1/interfaces: `hostname`
- /api/v1/interfaces: `os_version`
- /api/v1/interfaces: `mem_total_kb`
- /api/v1/interfaces: `cpu_model`
- /api/v1/interfaces: `mem_avail_kb`
- /api/v1/interfaces: `cpu_cores`
- /api/v1/system/services/action: `active_after`
- /api/v1/system/services/action: `items`
- /api/v1/system/services/action: `processes`
- /api/v1/system/processes: `items`
- /api/v1/system/processes: `processes`
- /api/v1/system/info: `hostname`
- /api/v1/system/info: `os_version`
- /api/v1/system/info: `mem_total_kb`
- /api/v1/system/info: `cpu_model`
- /api/v1/system/info: `mem_avail_kb`
- /api/v1/system/info: `cpu_cores`
- /api/v1/online: `byte_counting`
- /api/v1/online: `conntrack_entries`
- /api/v1/online: `byte_counting_note`
- /api/v1/online/kick: `byte_counting_note`
- /api/v1/conntrack/acct: `byte_counting_note`
- /api/v1/firmware/plan: `steps`
- /api/v1/firmware/plan: `risks`
