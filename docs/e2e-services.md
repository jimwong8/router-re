# E2E Services Verification Report

**Result: 22 PASS / 0 FAIL / 1 SKIP**

| Endpoint | Status | HTTP Code | Key Fields / Reason |
|----------|--------|-----------|---------------------|
| services | PASS | 200 | available=True, count=534, has_more=False, offset=0 |
| services/status | PASS | 200 | services=[{'name': 'ssh', 'enabled': False, 'runn |
| system/services/action (skip-exec) | SKIP | 0 | mutating action, skipped for safety |
| system/processes | PASS | 200 | n=15, top=[{'pid': 2160255, 'user': 'jimwong', 'cp, total=852 |
| system/time | PASS | 200 | config={'timezone': '', 'ntp_servers': None, 'e, status={'LocalRTC': 'no', 'NTP': 'yes', 'NTPSyn |
| remote/access | PASS | 200 | access={'ssh_enabled': False, 'ssh_port': 0, 'w, warnings=None |
| remote/access/status | PASS | 200 | ftp_active=inactive, ftp_port=21, ssh_active=inactive, ssh_listen= |
| pppoe/config | PASS | 200 | config={'interface': '', 'local_ip': '', 'pool_, options_path=/etc/ppp/pppoe-server-options, options_text=, render_error=pppoe interface must not be empty |
| pppoe/sessions | PASS | 200 | count=0, sessions=None |
| pppoe/status | PASS | 200 | detail=pppoe-server not running, running=False, status=stopped |
| radius/config | PASS | 200 | conf=auth_order radius
login_tries 4
authserv, config={'server': '', 'port': 0, 'secret': '', , servers=	
 |
| ddns/ip | PASS | 200 | ip=103.240.198.142 |
| ddns/status | PASS | 200 | status={'canntp': 'yes', 'localrtc': 'no', 'ntp |
| upnp/config | PASS | 200 | config={'enabled': False, 'external_iface': '',, miniupnpd=ext_ifname=eth0
listening_ip=br-lan
enab |
| upnp/mappings | PASS | 200 | mappings=[], nft= |
| upnp/status | PASS | 200 | active_count=0, installed=False, running=False, status=/usr/bin/systemctl is-active miniupnpd:  |
| snmp/config | PASS | 200 | applied=True, config={'enabled': False, 'port': 0, 'location' |
| snmp/status | PASS | 200 | config_exists=true, detail=inactive, port=, running=false |
| ntp/status | PASS | 200 | canntp=yes, localrtc=no, ntp=yes, ntpsynchronized=yes |
| mdns | PASS | 200 | avahi=, config={'in_ifaces': None, 'out_ifaces': None,  |
| bond | PASS | 200 | config={'name': '', 'slaves': None, 'mode': '',, live={}, script=ip link add  type bond mode active-backu |
| mirror | PASS | 200 | cleanup=tc qdisc del dev  ingress 2>/dev/null ||, config={'source_iface': '', 'target_iface': '',, script= |
| porttrigger | PASS | 200 | count=0, nft=, rules=[] |

## Notes
- Hardware/mutating-dependent endpoints marked SKIP with reason.
- No results fabricated.
