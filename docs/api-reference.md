# API 参考文档

> 自动生成：从 `mgmtd/internal/server/server.go` 路由注册与 handler 实现提取。


## 概览

- 端点总数：**276**

- 按方法：GET 139，POST 127，DELETE 10


## 认证

除 `POST /api/v1/auth/login`（登录）与 `GET /api/v1/health`（健康检查）外，所有端点需要在请求头携带：

```
Authorization: Bearer <token>
```

token 由登录端点返回。未携带或无效时返回 401。


## 错误格式

handler 通过 `errBody(code, message, detail)` 构造错误响应，形状为：

```json
{"code": <int>, "message": "<简短描述>", "detail": "<详细原因>"}
```

HTTP 状态码通常为 400（JSON 解析失败）、422（校验失败）、404（资源不存在）、500（执行失败）。


## 端点列表

| 方法 | 路径 | Handler | 主要响应字段 | 请求体 |
|------|------|---------|--------------|--------|
| GET | `/api/v1/health` | `inline` | status | 无 |
| POST | `/api/v1/auth/login` | `handleLogin` | token | 有 |
| GET | `/api/v1/system/info` | `handleSystemInfo` | （未解析） | 无 |
| GET | `/api/v1/qos/limits` | `handleGetQoSLimits` | limits, nft, tc | 无 |
| GET | `/api/v1/qos/client-limits` | `handleGetClientLimits` | limits, nft, tc | 无 |
| POST | `/api/v1/qos/client-limits` | `handleSetClientLimits` | limits, nft, tc | 有 |
| POST | `/api/v1/qos/limits` | `handleSetQoSLimits` | limits, nft, tc | 有 |
| GET | `/api/v1/interfaces` | `handleInterfaces` | interfaces | 无 |
| GET | `/api/v1/ddns/config` | `handleGetDDNSConfig` | config | 无 |
| GET | `/api/v1/dns/config` | `handleGetDNSConfig` | config, dnsmasq, hosts | 无 |
| POST | `/api/v1/dns/config` | `handleSetDNSConfig` | applied, config, detail, dnsmasq | 有 |
| POST | `/api/v1/dns/resolve` | `handleDNSResolve` | ips, name | 有 |
| GET | `/api/v1/dns/status` | `handleGetDNSStatus` | cache_size, conf_exists, conf_path, enabled, running, status, upstreams | 无 |
| GET | `/api/v1/upnp/config` | `handleGetUPNPConfig` | config, miniupnpd | 无 |
| POST | `/api/v1/upnp/config` | `handleSetUPNPConfig` | applied, config, detail | 有 |
| GET | `/api/v1/upnp/mappings` | `handleGetUPNPMappings` | mappings, nft | 无 |
| POST | `/api/v1/upnp/mappings` | `handleAddUPNPMapping` | applied, detail, mappings, nft | 有 |
| DELETE | `/api/v1/upnp/mappings` | `handleDeleteUPNPMapping` | applied, detail, mappings, nft | 有 |
| GET | `/api/v1/upnp/status` | `handleGetUPNPStatus` | active_count, installed, running, status | 无 |
| GET | `/api/v1/l7/rules` | `handleGetL7Rules` | dnsmasq, rules | 无 |
| POST | `/api/v1/l7/rules` | `handleSetL7Rules` | applied, detail, dnsmasq, rules | 有 |
| POST | `/api/v1/l7/test` | `handleL7Test` | blocked, by, hit | 有 |
| GET | `/api/v1/l7/stats` | `handleGetL7Stats` | by_action, by_type, enabled, total | 无 |
| POST | `/api/v1/l7/apply` | `handleL7Apply` | applied, detail, dnsmasq | 无 |
| POST | `/api/v1/ddns/config` | `handleSetDDNSConfig` | config, saved | 有 |
| POST | `/api/v1/ddns/update` | `handleDDNSUpdate` | Alibaba, Cloudflare, detail, ip, ok, provider | 无 |
| GET | `/api/v1/ddns/ip` | `handleGETDDNSIP` | ip | 无 |
| GET | `/api/v1/ntp/config` | `handleGetNTPConfig` | config | 无 |
| POST | `/api/v1/ntp/config` | `handleSetNTPConfig` | applied, config, reason, text | 有 |
| GET | `/api/v1/ntp/status` | `handleNTPStatus` | （未解析） | 无 |
| GET | `/api/v1/firewall/rules` | `handleGetFirewallRules` | nft, rules | 无 |
| POST | `/api/v1/firewall/rules` | `handleSetFirewallRules` | nft, rules | 有 |
| GET | `/api/v1/nat/rules` | `handleGetNATRules` | nft, rules | 无 |
| POST | `/api/v1/nat/rules` | `handleSetNATRules` | nft, rules | 有 |
| GET | `/api/v1/wan/links` | `handleGetWANLinks` | commands, links | 无 |
| POST | `/api/v1/wan/links` | `handleSetWANLinks` | applied, commands, links | 有 |
| GET | `/api/v1/route/bgp` | `handleGetBGP` | config, frr | 无 |
| POST | `/api/v1/route/bgp` | `handleSetBGP` | config, frr | 有 |
| GET | `/api/v1/route/ospf` | `handleGetOSPF` | config, frr | 无 |
| POST | `/api/v1/route/ospf` | `handleSetOSPF` | config, frr | 有 |
| GET | `/api/v1/dhcp/config` | `handleGetDHCPConfig` | config, dnsmasq | 无 |
| GET | `/api/v1/dhcp/leases` | `handleGetDHCPLeases` | count, leases | 无 |
| GET | `/api/v1/clients` | `handleGetClients` | clients, count | 无 |
| POST | `/api/v1/dhcp/config` | `handleSetDHCPConfig` | applied, config, dnsmasq | 有 |
| GET | `/api/v1/dhcp/server` | `handleGetDHCPServer` | applied, config | 无 |
| POST | `/api/v1/dhcp/server` | `handleSetDHCPServer` | applied, config, detail, dnsmasq | 有 |
| GET | `/api/v1/dhcp/status` | `handleGetDHCPStatus` | （未解析） | 无 |
| GET | `/api/v1/snmp/config` | `handleGetSNMPConfig` | applied, config | 无 |
| POST | `/api/v1/snmp/config` | `handleSetSNMPConfig` | applied, config, detail, snmpd | 有 |
| GET | `/api/v1/snmp/status` | `handleGetSNMPStatus` | （未解析） | 无 |
| POST | `/api/v1/snmp/query` | `handlePostSNMPQuery` | （未解析） | 无 |
| GET | `/api/v1/cloud/devices` | `handleGetCloudDevices` | devices | 无 |
| POST | `/api/v1/cloud/devices` | `handlePostCloudDevices` | device, devices | 无 |
| POST | `/api/v1/cloud/devices/{id}/heartbeat` | `handlePostCloudHeartbeat` | id, status | 有 |
| GET | `/api/v1/wireless/ssids` | `handleGetWirelessSSIDs` | ssids | 无 |
| POST | `/api/v1/wireless/ssids` | `handlePostWirelessSSIDs` | hostapd, message, ssid, ssids | 无 |
| GET | `/api/v1/dpi/rules` | `handleGetDPIRules` | nft, rules | 无 |
| POST | `/api/v1/dpi/rules` | `handleSetDPIRules` | applied, error, nft, rules, tc | 有 |
| GET | `/api/v1/dpi/marks` | `handleGetDPIMarks` | rules | 无 |
| POST | `/api/v1/dpi/marks` | `handleSetDPIMarks` | applied, error, nft, rules, tc | 有 |
| GET | `/api/v1/dpi/classify` | `handleDPIClassify` | category, dport, protocol | 无 |
| GET | `/api/v1/dpi/flows` | `handleGetDPIFlows` | count, flows | 无 |
| GET | `/api/v1/dpi/apps` | `handleGetDPIApps` | apps | 无 |
| GET | `/api/v1/dpi/tracker-stats` | `handleGetDPITrackerStats` | stats | 无 |
| POST | `/api/v1/dpi/classify-payload` | `handleClassifyPayload` | app, category, payload_len | 有 |
| GET | `/api/v1/dpi/mark-rules` | `handleGetDPIMarkRules` | app_ports, nft | 无 |
| POST | `/api/v1/dpi/mark-rules` | `handleSetDPIMarkRules` | app_ports, applied, detail, listing, nft | 有 |
| GET | `/api/v1/vlan` | `handleGetVLAN` | vlans | 无 |
| POST | `/api/v1/vlan` | `handleSetVLAN` | applied, vlans | 无 |
| DELETE | `/api/v1/vlan/{name}` | `handleDeleteVLAN` | deleted | 无 |
| GET | `/api/v1/bridge` | `handleGetBridge` | bridges | 无 |
| POST | `/api/v1/bridge` | `handleSetBridge` | applied, bridges | 有 |
| DELETE | `/api/v1/bridge/{name}` | `handleDeleteBridge` | deleted | 无 |
| GET | `/api/v1/radius/config` | `handleGetRadiusConfig` | conf, config, servers | 无 |
| POST | `/api/v1/radius/config` | `handleSetRadiusConfig` | conf, config, servers | 有 |
| POST | `/api/v1/radius/test` | `handleRadiusTest` | （未解析） | 有 |
| POST | `/api/v1/vpn/wireguard/keypair` | `handleWireGuardKeypair` | private_key, public_key | 无 |
| POST | `/api/v1/vpn/wireguard/config` | `handleWireGuardConfig` | config, message | 有 |
| GET | `/api/v1/interfaces/config` | `handleGetInterfacesConfig` | configs | 无 |
| POST | `/api/v1/interfaces/config` | `handleSetInterfacesConfig` | applied, configs | 有 |
| GET | `/api/v1/route/static` | `handleGetStaticRoutes` | commands, routes | 无 |
| POST | `/api/v1/route/static` | `handleSetStaticRoutes` | commands, routes | 有 |
| GET | `/api/v1/audit/events` | `handleGetAuditEvents` | events | 无 |
| GET | `/api/v1/wan/status` | `handleWANStatus` | status | 无 |
| GET | `/api/v1/portal/clients` | `handleGetPortalClients` | clients | 无 |
| POST | `/api/v1/portal/authorize` | `handlePortalAuthorize` | applied, client, nft | 有 |
| POST | `/api/v1/portal/revoke` | `handlePortalRevoke` | nft, revoked | 有 |
| GET | `/api/v1/portal/config` | `handleGetPortalConfig` | （未解析） | 无 |
| POST | `/api/v1/portal/config` | `handleSetPortalConfig` | default_minutes, enabled, portal_port | 有 |
| GET | `/api/v1/pppoe/config` | `handleGetPPPoEConfig` | config, options_path, options_text, render_error, secrets_path, secrets_text, users | 无 |
| POST | `/api/v1/pppoe/config` | `handleSetPPPoEConfig` | （未解析） | 有 |
| GET | `/api/v1/pppoe/users` | `handleGetPPPoEUsers` | count, users | 无 |
| POST | `/api/v1/pppoe/users` | `handleSetPPPoEUsers` | count, users | 有 |
| GET | `/api/v1/pppoe/sessions` | `handleGetPPPoESessions` | count, sessions | 无 |
| GET | `/api/v1/pppoe/status` | `handleGetPPPoEStatus` | detail, running, status | 无 |
| GET | `/api/v1/conntrack` | `handleGetConntrack` | available, count, entries, summary | 无 |
| GET | `/api/v1/logs` | `handleGetLogs` | count, entries | 无 |
| GET | `/api/v1/logs/sources` | `handleGetLogSources` | count, sources | 无 |
| GET | `/api/v1/portmap/rules` | `handleGetPortmapRules` | nft, rules | 无 |
| POST | `/api/v1/portmap/rules` | `handleSetPortmapRules` | applied, nft, rules | 有 |
| DELETE | `/api/v1/portmap/rules/{id}` | `handleDeletePortmapRule` | applied, deleted, rules | 无 |
| GET | `/api/v1/policyroute/rules` | `handleGetPolicyRouteRules` | live, rules | 无 |
| POST | `/api/v1/policyroute/rules` | `handleSetPolicyRouteRules` | applied, policies | 有 |
| DELETE | `/api/v1/policyroute/rules/{id}` | `handleDeletePolicyRouteRule` | applied, deleted, policies | 无 |
| GET | `/api/v1/stats/current` | `handleStatsCurrent` | （未解析） | 无 |
| GET | `/api/v1/stats/history` | `handleStatsHistory` | count, samples | 无 |
| GET | `/api/v1/stats/interfaces` | `handleStatsInterfaces` | interfaces | 无 |
| GET | `/api/v1/traffic/rates` | `handleTrafficRates` | elapsed_sec, rates, ts | 无 |
| GET | `/api/v1/traffic/top` | `handleTrafficTop` | rates | 无 |
| GET | `/api/v1/remote/access` | `handleGetRemoteAccess` | access, warnings | 无 |
| POST | `/api/v1/remote/access` | `handleSetRemoteAccess` | access, applied, pending_manual, reason, warnings | 有 |
| GET | `/api/v1/remote/access/status` | `handleRemoteAccessStatus` | （未解析） | 无 |
| GET | `/api/v1/system/time` | `handleGetSystemTime` | config, status | 无 |
| POST | `/api/v1/system/time` | `handleSetSystemTime` | applied, reason, status, time | 有 |
| GET | `/api/v1/system/timezones` | `handleListTimezones` | count, timezones | 无 |
| GET | `/api/v1/wol/targets` | `handleGetWOLTargets` | count, targets | 无 |
| POST | `/api/v1/wol/targets` | `handleSetWOLTargets` | count, targets | 有 |
| POST | `/api/v1/wol/wake` | `handleWOLWake` | iface, mac, woken | 有 |
| GET | `/api/v1/threat/list` | `handleThreatList` | count, entries, expired_count, now | 无 |
| POST | `/api/v1/threat/block` | `handleThreatBlock` | added, entry | 有 |
| DELETE | `/api/v1/threat/block` | `handleThreatUnblock` | ip, removed | 有 |
| POST | `/api/v1/threat/apply` | `handleThreatApply` | applied, count, script, v4_count, v6_count | 无 |
| POST | `/api/v1/threat/import` | `handleThreatImport` | added, errors, parse_note, skipped | 有 |
| POST | `/api/v1/threat/fetch` | `handleThreatFetch` | added, errors, skipped, source | 有 |
| GET | `/api/v1/threat/stats` | `handleThreatStats` | active, expired, permanent, total | 无 |
| GET | `/api/v1/webadmin` | `handleGetWebAdmin` | config, nginx | 无 |
| GET | `/api/v1/system/services` | `handleListServices` | available, count, units | 无 |
| POST | `/api/v1/system/services/action` | `handleServiceAction` | action, active_after, applied, argv, error, output, unit | 有 |
| GET | `/api/v1/system/processes` | `handleListProcesses` | n, top, total | 无 |
| POST | `/api/v1/system/processes/kill` | `handleKillProcess` | applied, error, output, pid, signal | 有 |
| GET | `/api/v1/security/ddos` | `handleGetDDoS` | config, source | 无 |
| POST | `/api/v1/security/ddos` | `handleSetDDoS` | applied, dry_run, error, hint, nft, output, save_error | 有 |
| GET | `/api/v1/policy/timeschedule` | `handleGetTimeSchedule` | count, now, rules, timezone | 无 |
| POST | `/api/v1/policy/timeschedule` | `handleSetTimeSchedule` | applied, count, dry_run, error, nft, output, save_error | 有 |
| POST | `/api/v1/diagnostics/ping` | `handlePing` | （未解析） | 有 |
| POST | `/api/v1/diagnostics/traceroute` | `handleTraceroute` | （未解析） | 有 |
| POST | `/api/v1/diagnostics/portprobe` | `handlePortProbe` | command, elapsed_ms, note, output, port, state, target | 有 |
| GET | `/api/v1/vpn/client` | `handleGetVPNClient` | config, has_config, hint, interface, live, live_error, peer_count, peers | 无 |
| POST | `/api/v1/vpn/client` | `handleSetVPNClient` | applied, commands, commands_run, conf, config_path, dry_run, error, hint, save_error | 有 |
| GET | `/api/v1/ddns/records` | `handleListDDNS` | count, providers, records | 无 |
| POST | `/api/v1/ddns/records` | `handleSetDDNS` | count, records | 有 |
| GET | `/api/v1/ddns/check` | `handleDDNSCheck` | available, current_ip, need_update, note, records, source | 无 |
| GET | `/api/v1/voucher/list` | `handleListVouchers` | count, unused, used, vouchers | 无 |
| POST | `/api/v1/voucher/generate` | `handleGenerateVouchers` | generated, note, vouchers | 有 |
| POST | `/api/v1/voucher/redeem` | `handleRedeemVoucher` | （未解析） | 有 |
| DELETE | `/api/v1/voucher` | `handleDeleteVoucher` | remaining, removed | 无 |
| GET | `/api/v1/linkcheck/targets` | `handleListLinkCheck` | count, states, targets | 无 |
| POST | `/api/v1/linkcheck/targets` | `handleSetLinkCheck` | count, targets | 有 |
| POST | `/api/v1/linkcheck/probe` | `handleProbeLink` | dns, http, icmp, id, ok, output, reason, state | 有 |
| GET | `/api/v1/linkcheck/status` | `handleLinkCheckStatus` | count, down, states, up | 无 |
| GET | `/api/v1/portal/templates` | `handleListPortalTemplates` | builtin, count, current | 无 |
| POST | `/api/v1/portal/template` | `handleSetPortalTemplate` | saved, template | 有 |
| GET | `/api/v1/portal/preview` | `handlePreviewPortal` | （未解析） | 无 |
| GET | `/api/v1/firewall/log/analysis` | `handleFwLogAnalysis` | available, count, suggestions, summary, suspicious, top_talkers | 无 |
| GET | `/api/v1/firewall/log/suggestions` | `handleFwLogSuggestions` | applied, available, note, suggestions | 无 |
| POST | `/api/v1/webadmin` | `handleSetWebAdmin` | config, detail, nginx | 有 |
| GET | `/api/v1/identity` | `handleGetIdentity` | identity | 无 |
| POST | `/api/v1/identity` | `handleSetIdentity` | applied, detail, identity | 有 |
| GET | `/api/v1/multiwan` | `handleGetMultiWAN` | count, members, probe, states | 无 |
| POST | `/api/v1/multiwan` | `handleSetMultiWAN` | +itoa(i)+, count, members, probe | 有 |
| POST | `/api/v1/multiwan/probe` | `handleMultiWANProbeResult` | state, usable, usable_count, warning | 有 |
| GET | `/api/v1/bond` | `handleGetBond` | config, live, script | 无 |
| POST | `/api/v1/bond` | `handleSetBond` | applied, detail, script | 有 |
| GET | `/api/v1/iface/tune` | `handleGetIfaceTune` | iface, live | 无 |
| POST | `/api/v1/iface/tune` | `handleSetIfaceTune` | applied, detail | 有 |
| GET | `/api/v1/cert` | `handleGetCert` | days_left, exists, info, path, valid, warning | 无 |
| POST | `/api/v1/cert/self-signed` | `handleSelfSignedCert` | cert_bytes, cert_path, detail, info, key_bytes, key_path | 有 |
| GET | `/api/v1/online` | `handleGetOnline` | byte_counting, byte_counting_note, conntrack_entries, count, sessions | 无 |
| POST | `/api/v1/online/kick` | `handleKickOnline` | command, detail, ip, kicked | 有 |
| GET | `/api/v1/porttrigger` | `handleGetPortTrigger` | count, nft, rules | 无 |
| POST | `/api/v1/porttrigger` | `handleSetPortTrigger` | +itoa(i)+, count, nft, rules | 有 |
| GET | `/api/v1/alg` | `handleGetALG` | config, enabled_any, script | 无 |
| POST | `/api/v1/alg` | `handleSetALG` | config, enabled_any, script | 有 |
| GET | `/api/v1/mdns` | `handleGetMDNS` | avahi, config | 无 |
| POST | `/api/v1/mdns` | `handleSetMDNS` | avahi, config | 有 |
| GET | `/api/v1/firmware/inspect` | `handleFirmwareInspect` | image | 无 |
| POST | `/api/v1/firmware/plan` | `handleFirmwarePlan` | （未解析） | 有 |
| POST | `/api/v1/firmware/apply` | `handleFirmwareApply` | applied, detail, note | 有 |
| GET | `/api/v1/diagnostics/report` | `handleDiagReportJSON` | （未解析） | 无 |
| GET | `/api/v1/diagnostics/report/text` | `handleDiagReportText` | （未解析） | 无 |
| GET | `/api/v1/notify/smtp` | `handleGetSMTP` | config | 无 |
| POST | `/api/v1/notify/smtp` | `handleSetSMTP` | config | 有 |
| POST | `/api/v1/notify/smtp/preview` | `handleSMTPPreview` | （未解析） | 有 |
| GET | `/api/v1/notify/trap` | `handleGetTrap` | sink, target | 无 |
| POST | `/api/v1/notify/trap` | `handleSetTrap` | sink, target | 有 |
| GET | `/api/v1/routing/tables` | `handleGetPolicyTables` | count, rendered, tables | 无 |
| POST | `/api/v1/routing/tables` | `handleSetPolicyTables` | +itoa(i)+, count, tables | 有 |
| GET | `/api/v1/routing/rules` | `handleGetPolicyRules` | count, rendered, rules | 无 |
| POST | `/api/v1/routing/rules` | `handleSetPolicyRules` | +itoa(i)+, count, rules | 有 |
| GET | `/api/v1/alerts/rules` | `handleGetAlertRules` | count, rules | 无 |
| POST | `/api/v1/alerts/rules` | `handleSetAlertRules` | +strconv.Itoa(i)+, count, rules | 有 |
| POST | `/api/v1/alerts/test` | `handleTestAlerts` | count, matched | 有 |
| GET | `/api/v1/logs/rotate` | `handleGetRotate` | config, logrotate | 无 |
| POST | `/api/v1/logs/rotate` | `handleSetRotate` | applied, logrotate, path | 有 |
| GET | `/api/v1/logs/syslog` | `handleGetSyslog` | rsyslog, target | 无 |
| POST | `/api/v1/logs/syslog` | `handleSetSyslog` | rsyslog, target | 有 |
| POST | `/api/v1/logs/syslog/test` | `handleTestSyslog` | detail, ok | 有 |
| GET | `/api/v1/billing/bills` | `handleGetBills` | bills, count | 无 |
| POST | `/api/v1/billing/bills/generate` | `handleGenerateBills` | count, generated, skipped | 有 |
| GET | `/api/v1/config/export` | `handleExportConfig` | （未解析） | 无 |
| POST | `/api/v1/config/preview` | `handlePreviewRestore` | （未解析） | 有 |
| POST | `/api/v1/config/restore` | `handleRestoreConfig` | count, detail, restored | 有 |
| GET | `/api/v1/ipv6/nd` | `handleGetIPv6ND` | config, live | 无 |
| POST | `/api/v1/ipv6/nd` | `handleSetIPv6ND` | applied, config, detail | 有 |
| GET | `/api/v1/ipv6/pd` | `handleGetIPv6PD` | config, dhcp6c | 无 |
| POST | `/api/v1/ipv6/pd` | `handleSetIPv6PD` | applied, config, detail, dhcp6c | 有 |
| GET | `/api/v1/mirror` | `handleGetMirror` | cleanup, config, script | 无 |
| POST | `/api/v1/mirror` | `handleSetMirror` | applied, detail, dry_run, script | 有 |
| GET | `/api/v1/pppoe/multidial` | `handleGetMultiDial` | path, sessions | 无 |
| POST | `/api/v1/pppoe/multidial` | `handleSetMultiDial` | +strconv.Itoa(i)+, count, detail, path | 有 |
| GET | `/api/v1/accounting/status` | `handleAccountingStatus` | acct_enabled, available, conntrack_total, entries_with_bytes, reason | 无 |
| GET | `/api/v1/accounting/report` | `handleAccountingReport` | count, report | 无 |
| GET | `/api/v1/htb/config` | `handleGetHTBConfig` | applied, config | 无 |
| POST | `/api/v1/htb/config` | `handleSetHTBConfig` | config, script | 有 |
| POST | `/api/v1/htb/apply` | `handleHTBApply` | applied, detail, dry_run, script | 有 |
| POST | `/api/v1/htb/cleanup` | `handleHTBCleanup` | applied, iface | 有 |
| GET | `/api/v1/htb/show` | `handleHTBShow` | iface, raw | 无 |
| GET | `/api/v1/htb/report` | `handleHTBReport` | config, script | 无 |
| GET | `/api/v1/quota/policies` | `handleGetQuotaPolicies` | count, policies, report | 无 |
| POST | `/api/v1/quota/policies` | `handleSetQuotaPolicies` | count, policies, report | 有 |
| POST | `/api/v1/quota/apply` | `handleQuotaApply` | applied, count, nft | 有 |
| POST | `/api/v1/quota/test` | `handleQuotaTest` | ctx, hit, matched, policy_id, scan_count | 有 |
| GET | `/api/v1/quota/report` | `handleQuotaReport` | count, report | 无 |
| GET | `/api/v1/quota/nft` | `handleQuotaNft` | exists, nft | 无 |
| GET | `/api/v1/acl/rules` | `handleGetACLRules` | count, rules | 无 |
| POST | `/api/v1/acl/rules` | `handleSetACLRules` | applied, nft, rules | 有 |
| GET | `/api/v1/security/arp` | `handleGetArp` | iface, status, sysctls | 无 |
| POST | `/api/v1/security/arp` | `handleSetArp` | applied, iface, sysctls | 有 |
| GET | `/api/v1/security/arp/neighbors` | `handleGetNeighbors` | count, neighbors | 无 |
| GET | `/api/v1/webfilter/rules` | `handleGetWebfilterRules` | dnsmasq, rules | 无 |
| POST | `/api/v1/webfilter/rules` | `handleSetWebfilterRules` | applied, detail, dnsmasq, rules | 有 |
| GET | `/api/v1/security/attack-guard` | `handleGetAttackGuard` | config, nft | 无 |
| POST | `/api/v1/security/attack-guard` | `handleSetAttackGuard` | applied, config, detail, nft | 有 |
| GET | `/api/v1/ipv6/addresses` | `handleGetIPv6Addresses` | addresses | 无 |
| POST | `/api/v1/ipv6/addresses` | `handleSetIPv6Addresses` | addresses, applied | 有 |
| DELETE | `/api/v1/ipv6/addresses` | `handleDeleteIPv6Address` | addr, deleted, iface | 有 |
| GET | `/api/v1/ipv6/ra` | `handleGetIPv6RA` | config, radvd | 无 |
| POST | `/api/v1/ipv6/ra` | `handleSetIPv6RA` | applied, config, detail, radvd | 有 |
| GET | `/api/v1/ipv6/status` | `handleGetIPv6Status` | （未解析） | 无 |
| POST | `/api/v1/diag/ping` | `handleDiagPing` | （未解析） | 有 |
| POST | `/api/v1/diag/traceroute` | `handleDiagTraceroute` | （未解析） | 有 |
| POST | `/api/v1/diag/dns` | `handleDiagDNS` | （未解析） | 有 |
| GET | `/api/v1/diag/interfaces` | `handleDiagInterfaces` | count, interfaces | 无 |
| GET | `/api/v1/nac/config` | `handleGetNACConfig` | config, nft | 无 |
| POST | `/api/v1/nac/config` | `handleSetNACConfig` | applied, config, detail, nft | 有 |
| GET | `/api/v1/nac/neighbors` | `handleGetNACNeighbors` | count, neighbors | 无 |
| GET | `/api/v1/nac/connlimits` | `handleGetNACConnlimits` | limits, nft | 无 |
| POST | `/api/v1/nac/connlimits` | `handleSetNACConnlimits` | limits, nft, saved | 有 |
| POST | `/api/v1/nac/connlimits/apply` | `handleApplyNACConnlimits` | applied, detail, limits, nft | 无 |
| GET | `/api/v1/nac/top-connections` | `handleGetNACTopConnections` | count, top | 无 |
| GET | `/api/v1/lb/config` | `handleGetLBConfig` | config, iprules, marks | 无 |
| POST | `/api/v1/lb/config` | `handleSetLBConfig` | applied, config, detail, iprules, marks | 有 |
| GET | `/api/v1/lb/status` | `handleGetLBStatus` | config, status | 无 |
| POST | `/api/v1/lb/check` | `handleLBCheck` | checked_at, status | 无 |
| POST | `/api/v1/lb/apply` | `handleLBApply` | applied, config, detail, iprules, marks, rules | 无 |
| GET | `/api/v1/lb/preview` | `handleLBPreview` | hash_key, member, mode | 无 |
| GET | `/api/v1/ddns/status` | `handleGetDDNSStatus` | status | 无 |
| GET | `/api/v1/accounts` | `handleGetAccounts` | accounts, count | 无 |
| POST | `/api/v1/accounts` | `handleSetAccounts` | accounts, created, errors, updated | 无 |
| DELETE | `/api/v1/accounts/{username}` | `handleDeleteAccount` | accounts, deleted, username | 无 |
| POST | `/api/v1/accounts/{username}/reset-usage` | `handleResetAccountUsage` | account, reset, username | 无 |
| POST | `/api/v1/accounts/{username}/bind-mac` | `handleBindAccountMAC` | account, bound, username | 有 |
| POST | `/api/v1/accounts/import` | `handleImportAccounts` | created, errors, total | 有 |
| GET | `/api/v1/billing/plans` | `handleGetBillingPlans` | count, plans | 无 |
| POST | `/api/v1/billing/plans` | `handleSetBillingPlans` | applied, count, plans | 无 |
| GET | `/api/v1/billing/summary` | `handleGetBillingSummary` | expired, online, over_quota, revenue, total_accounts | 无 |
| GET | `/api/v1/users` | `handleGetUsers` | count, users | 无 |
| POST | `/api/v1/users` | `handleCreateUser` | user | 有 |
| DELETE | `/api/v1/users/{username}` | `handleDeleteUser` | deleted | 无 |
| POST | `/api/v1/users/{username}/password` | `handleSetUserPassword` | changed | 有 |
| GET | `/api/v1/ips/rules` | `handleGetIPSRules` | count, rules | 无 |
| POST | `/api/v1/ips/rules` | `handleSetIPSRules` | applied, count, failures, rules | 有 |
| GET | `/api/v1/ips/stats` | `handleGetIPSStats` | stats, total | 无 |
| POST | `/api/v1/ips/test` | `handleTestIPSRule` | hits, matched, rule | 有 |
| GET | `/api/v1/ips/blocklist` | `handleGetIPSBlocklist` | count, ips | 无 |
| POST | `/api/v1/ips/blocklist` | `handleSetIPSBlocklist` | applied, blocklist, detail, listing, nft | 有 |
