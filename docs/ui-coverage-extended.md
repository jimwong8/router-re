# UI 自动验收扩展覆盖报告

- 接口端点：`http://127.0.0.1:9090`
- 前端端点：`http://127.0.0.1:4173`
- 覆盖页面：33 个 | OK: 24 | SKIP: 9 | FAIL: 0

| 路由 | 结果 | 接口预期条数 | 页面渲染行数 | 说明 |
|------|------|--------------|--------------|------|
| `/services` | OK   | 534 | 50 | systemd 单元列表 (units) [分页] |
| `/online-users` | OK   | 2 | 2 | 在线会话 |
| `/routing-policy` | OK   | 1 | 2 | 策略路由规则(可为空) |
| `/cert` | SKIP | - | - | SKIP: 证书接口返回对象/{exists,path} 非列表 | 没有可识别的列表字段: detail,exists,path |
| `/alerts` | OK   | 1 | 1 | 告警规则(可为空) |
| `/network-tools` | SKIP | - | - | SKIP: 无 GET 列表接口(全 POST 探测) |
| `/network-advanced` | SKIP | - | - | SKIP: 表单/单接口调优，无列表渲染 |
| `/accounts` | OK   | 2 | 2 | 账号列表(可为空) |
| `/audit` | OK   | 100 | 100 | 审计事件 |
| `/clients` | OK   | 24 | 24 | 客户端列表 |
| `/cloud` | OK   | 0 | 0 | 云设备(可为空) |
| `/diag` | OK   | 11 | 11 | 接口诊断列表 |
| `/dhcp` | OK   | 0 | 1 | DHCP 租约(可为空) |
| `/dpi-flow` | OK   | 0 | 0 | DPI 流量(可为空) |
| `/dpi` | OK   | 0 | 0 | DPI 标记(可为空) |
| `/interfaces` | SKIP | - | - | 接口配置(configs=空或 None 时 SKIP) | 没有可识别的列表字段: configs |
| `/ipv6` | OK   | 4 | 8 | IPv6 地址(可为空) |
| `/l7` | OK   | 0 | 0 | L7 规则(可为空) |
| `/logs` | OK   | 100 | 100 | 日志条目(可为空) |
| `/voucher` | OK   | 69 | 69 | 卡密列表(可为空) |
| `/nac` | SKIP | - | - | SKIP: NAC 多表格分页 (devices/limits/top/neighbors)，接口 neighbors=45 非可见表，无法准确比对 |
| `/network` | SKIP | - | - | SKIP: portmap/rules.rules=None(主机无 NAT 规则) |
| `/pppoe` | OK   | 0 | 0 | PPPoE 会话(可为空) |
| `/qos` | OK   | 0 | 0 | QoS 限速(可为空) |
| `/remote` | SKIP | - | - | 远程访问(可为空) | 没有可识别的列表字段: access,warnings |
| `/security` | OK   | 11 | 38 | 接口流量(ACL 表格取 acl/rules、主体表格取 stats/interfaces) |
| `/threat` | OK   | 0 | 0 | 威胁列表(可为空) |
| `/upnp` | OK   | 0 | 1 | UPnP 映射(可为空) |
| `/users` | OK   | 3 | 3 | 用户列表 |
| `/wireless` | OK   | 0 | 0 | Wi-Fi SSID(可为空) |
| `/static-route` | SKIP | - | - | SKIP: route/static 返回 routes=None(主机无静态路由) | 没有可识别的列表字段: commands,routes |
| `/wan` | OK   | 1 | 10 | SKIP: wan/links links=None(主机无多WAN) |
| `/dhcp-server` | SKIP | - | - | SKIP: dhcp/server 返回 config(对象)非列表 | 没有可识别的列表字段: applied,config |

## 说明
跳过页面保留其原因在「说明」列。覆盖逻辑同 ui_acceptance.py：接口有数据时页面表格渲染行数至少达到接口条数的 80%。
