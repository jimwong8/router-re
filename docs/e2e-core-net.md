# E2E Core Network Verification Report

**Result: 11 PASS / 0 FAIL / 0 SKIP**

| Endpoint | Status | HTTP Code | Key Fields / Reason |
|----------|--------|-----------|---------------------|
| wan/links | PASS | 200 | commands=['ip route add default via  dev eth0 tab, links=[{'name': 'wan1', 'interface': 'eth0', ' |
| wan/status | PASS | 200 | status=[{'name': 'wan1', 'interface': 'eth0', ' |
| route/bgp | PASS | 200 | config={'local_as': 0, 'router_id': '', 'peers', frr=frr version 8.4
frr defaults traditional |
| route/ospf | PASS | 200 | config={'router_id': '', 'networks': None, 'are, frr=frr version 8.4
frr defaults traditional |
| route/static | PASS | 200 | commands=None, routes=None |
| routing/rules | PASS | 200 | count=1, rendered=['ip rule add priority 100 from 10.100.1, rules=[{'priority': 100, 'from': '10.100.1.0/2 |
| traffic/rates | PASS | 200 | elapsed_sec=505.196208343, rates=[{'iface': 'ens18', 'rx_bps': 34775.8884, ts=1793275715 |
| traffic/top | PASS | 200 | rates=[{'iface': 'ens18', 'rx_bps': 34775.8884 |
| stats/interfaces | PASS | 200 | interfaces=[{'name': 'ens18', 'rx_bytes': 150027010 |
| interfaces | PASS | 200 | interfaces=[{'name': 'ens18', 'ipaddr': '10.100.1.1 |
| diag/interfaces | PASS | 200 | count=11, interfaces=[{'iface': 'ens18', 'rx_bytes': '0', 'rx |

## Notes
- Hardware-dependent endpoints marked SKIP with reason.
- No results fabricated.
