# 写端点空输入审计

生成于 2026-10-28 02:29:50｜共测 112 个写端点｜跳过高危 25 个

方法：对每个 POST/DELETE 发送空 JSON `{}`。空 body 被校验拒绝 = 零副作用；
同时对空 body 返回 2xx 的端点可疑（空操作当成功，或未解码请求体）。

## 分类统计

| 类别 | 数量 |
|---|---|
| 4xx 已校验（健康） | 80 |
| 2xx 接受空body（可疑） | 22 |
| 404 路由/资源不存在 | 6 |
| 5xx 崩溃（真bug） | 3 |
| 401/403 鉴权 | 1 |

## 可疑端点（空 body 也 2xx）

| 状态 | 方法 | 路径 | 响应片段 |
|---|---|---|---|
| 200 | POST | `/api/v1/l7/test` | `{"blocked":false,"by":"","hit":null}
` |
| 200 | POST | `/api/v1/l7/apply` | `{"applied":false,"detail":"open /etc/dnsmasq.d/router-re-l7.` |
| 200 | POST | `/api/v1/dpi/classify-payload` | `{"app":"Unknown","category":"Unspecified","payload_len":0}
` |
| 200 | POST | `/api/v1/portal/revoke` | `{"nft":"table inet portal {\n\tset authorized {\n\t\ttype et` |
| 200 | POST | `/api/v1/portal/config` | `{"default_minutes":60,"enabled":false,"portal_port":8080}
` |
| 200 | POST | `/api/v1/pppoe/users` | `{"count":0,"users":null}
` |
| 200 | DELETE | `/api/v1/threat/block` | `{"ip":"","removed":false}
` |
| 200 | POST | `/api/v1/threat/import` | `{"added":0,"parse_note":"","skipped":0}
` |
| 200 | POST | `/api/v1/ddns/records` | `{"count":0,"records":[]}
` |
| 200 | POST | `/api/v1/porttrigger` | `{"count":0,"nft":"","rules":null}
` |
| 200 | POST | `/api/v1/alg` | `{"config":{"ftp":false,"sip":false,"h323":false,"tftp":false` |
| 200 | POST | `/api/v1/notify/smtp/preview` | `From: 
To: 
Subject: 
Date: Wed, 28 Oct 2026 02:29:46 +08` |
| 200 | POST | `/api/v1/routing/rules` | `{"count":0,"rules":null}
` |
| 200 | POST | `/api/v1/alerts/test` | `{"count":0,"matched":[]}
` |
| 200 | POST | `/api/v1/billing/bills/generate` | `{"count":0,"generated":[],"skipped":null}
` |
| 200 | POST | `/api/v1/quota/policies` | `{"count":0,"policies":null,"report":""}
` |
| 200 | POST | `/api/v1/quota/test` | `{"ctx":{"SrcIP":"","SrcMAC":"","DstIP":"","DstPort":"","App"` |
| 200 | POST | `/api/v1/nac/connlimits/apply` | `{"applied":false,"detail":"/usr/sbin/nft -f -: netlink: Erro` |
| 200 | POST | `/api/v1/lb/check` | `{"checked_at":null,"status":[]}
` |
| 200 | POST | `/api/v1/lb/apply` | `{"applied":false,"config":{"mode":"","members":null,"health"` |
| 200 | POST | `/api/v1/ips/test` | `{"hits":null,"matched":false,"rule":null}
` |
| 200 | POST | `/api/v1/ips/blocklist` | `{"applied":false,"blocklist":[],"detail":"/usr/sbin/nft -f -` |

## 崩溃端点（5xx / 连接失败）

| 状态 | 方法 | 路径 | 响应片段 |
|---|---|---|---|
| 500 | POST | `/api/v1/wol/targets` | `{"code":"10149","detail":"mkdir /etc/router-re: permission d` |
| 500 | POST | `/api/v1/threat/apply` | `{"code":"10163","detail":"/usr/sbin/nft -f -: netlink: Error` |
| 500 | POST | `/api/v1/cert/self-signed` | `{"code":"10337","detail":"mkdir /etc/ssl/router-re: permissi` |

## 全部明细

| 状态 | 方法 | 路径 | 分类 |
|---|---|---|---|
| 401 | POST | `/api/v1/auth/login` | 401/403 鉴权 |
| 404 | POST | `/api/v1/dns/resolve` | 404 路由/资源不存在 |
| 422 | POST | `/api/v1/upnp/mappings` | 4xx 已校验（健康） |
| 404 | DELETE | `/api/v1/upnp/mappings` | 404 路由/资源不存在 |
| 422 | POST | `/api/v1/l7/rules` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/l7/test` | 2xx 接受空body（可疑） |
| 200 | POST | `/api/v1/l7/apply` | 2xx 接受空body（可疑） |
| 400 | POST | `/api/v1/ddns/update` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/snmp/query` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/cloud/devices` | 4xx 已校验（健康） |
| 404 | POST | `/api/v1/cloud/devices/{id}/heartbeat` | 404 路由/资源不存在 |
| 422 | POST | `/api/v1/dpi/rules` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/dpi/marks` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/dpi/classify-payload` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/dpi/mark-rules` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/radius/test` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/portal/authorize` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/portal/revoke` | 2xx 接受空body（可疑） |
| 200 | POST | `/api/v1/portal/config` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/pppoe/config` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/pppoe/users` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/portmap/rules` | 4xx 已校验（健康） |
| 400 | DELETE | `/api/v1/portmap/rules/{id}` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/policyroute/rules` | 4xx 已校验（健康） |
| 400 | DELETE | `/api/v1/policyroute/rules/{id}` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/remote/access` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/system/time` | 4xx 已校验（健康） |
| 500 | POST | `/api/v1/wol/targets` | 5xx 崩溃（真bug） |
| 422 | POST | `/api/v1/wol/wake` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/threat/block` | 4xx 已校验（健康） |
| 200 | DELETE | `/api/v1/threat/block` | 2xx 接受空body（可疑） |
| 500 | POST | `/api/v1/threat/apply` | 5xx 崩溃（真bug） |
| 200 | POST | `/api/v1/threat/import` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/threat/fetch` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/system/services/action` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/system/processes/kill` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/security/ddos` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/policy/timeschedule` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diagnostics/ping` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diagnostics/traceroute` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diagnostics/portprobe` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/ddns/records` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/voucher/generate` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/voucher/redeem` | 4xx 已校验（健康） |
| 422 | DELETE | `/api/v1/voucher` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/linkcheck/targets` | 4xx 已校验（健康） |
| 404 | POST | `/api/v1/linkcheck/probe` | 404 路由/资源不存在 |
| 422 | POST | `/api/v1/portal/template` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/webadmin` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/identity` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/multiwan` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/multiwan/probe` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/bond` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/iface/tune` | 4xx 已校验（健康） |
| 500 | POST | `/api/v1/cert/self-signed` | 5xx 崩溃（真bug） |
| 422 | POST | `/api/v1/online/kick` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/porttrigger` | 2xx 接受空body（可疑） |
| 200 | POST | `/api/v1/alg` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/mdns` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/firmware/plan` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/firmware/apply` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/notify/smtp` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/notify/smtp/preview` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/notify/trap` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/routing/tables` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/routing/rules` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/alerts/rules` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/alerts/test` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/logs/rotate` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/logs/syslog` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/logs/syslog/test` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/billing/bills/generate` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/config/preview` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/config/restore` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/ipv6/nd` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/ipv6/pd` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/mirror` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/pppoe/multidial` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/htb/config` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/htb/apply` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/htb/cleanup` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/quota/policies` | 2xx 接受空body（可疑） |
| 409 | POST | `/api/v1/quota/apply` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/quota/test` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/acl/rules` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/security/arp` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/webfilter/rules` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/security/attack-guard` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/ipv6/addresses` | 4xx 已校验（健康） |
| 422 | DELETE | `/api/v1/ipv6/addresses` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/ipv6/ra` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diag/ping` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diag/traceroute` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/diag/dns` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/nac/config` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/nac/connlimits` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/nac/connlimits/apply` | 2xx 接受空body（可疑） |
| 422 | POST | `/api/v1/lb/config` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/lb/check` | 2xx 接受空body（可疑） |
| 200 | POST | `/api/v1/lb/apply` | 2xx 接受空body（可疑） |
| 400 | POST | `/api/v1/accounts` | 4xx 已校验（健康） |
| 404 | DELETE | `/api/v1/accounts/{username}` | 404 路由/资源不存在 |
| 404 | POST | `/api/v1/accounts/{username}/reset-usage` | 404 路由/资源不存在 |
| 422 | POST | `/api/v1/accounts/{username}/bind-mac` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/accounts/import` | 4xx 已校验（健康） |
| 400 | POST | `/api/v1/billing/plans` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/users` | 4xx 已校验（健康） |
| 409 | DELETE | `/api/v1/users/{username}` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/users/{username}/password` | 4xx 已校验（健康） |
| 422 | POST | `/api/v1/ips/rules` | 4xx 已校验（健康） |
| 200 | POST | `/api/v1/ips/test` | 2xx 接受空body（可疑） |
| 200 | POST | `/api/v1/ips/blocklist` | 2xx 接受空body（可疑） |

## 跳过的高危端点（需人工单独验证）

- POST `/api/v1/qos/client-limits`
- POST `/api/v1/qos/limits`
- POST `/api/v1/dns/config`
- POST `/api/v1/upnp/config`
- POST `/api/v1/ddns/config`
- POST `/api/v1/ntp/config`
- POST `/api/v1/firewall/rules`
- POST `/api/v1/nat/rules`
- POST `/api/v1/wan/links`
- POST `/api/v1/route/bgp`
- POST `/api/v1/route/ospf`
- POST `/api/v1/dhcp/config`
- POST `/api/v1/dhcp/server`
- POST `/api/v1/snmp/config`
- POST `/api/v1/wireless/ssids`
- POST `/api/v1/vlan`
- DELETE `/api/v1/vlan/{name}`
- POST `/api/v1/bridge`
- DELETE `/api/v1/bridge/{name}`
- POST `/api/v1/radius/config`
- POST `/api/v1/vpn/wireguard/keypair`
- POST `/api/v1/vpn/wireguard/config`
- POST `/api/v1/interfaces/config`
- POST `/api/v1/route/static`
- POST `/api/v1/vpn/client`
