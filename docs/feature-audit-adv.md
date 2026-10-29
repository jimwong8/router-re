# router-re 功能核对进阶报告 (p100-feature-audit-adv)

> **核对范围**: `docs/ikuai-vs-routerre-feature-comparison.md` 第 5 类「认证计费」（行 386-390）+ 第 7 类「高级服务」（行 412-422）+ 第 9 类「设备设置」（行 441-445），共 **17 个** 标注为 ❌ 的条目
> **核对方法**: ① grep `mgmtd/internal/` 代码查找包/函数/端点 ② 实打 `127.0.0.1:9090` 真实 HTTP 端点（Bearer token 验证）
> **判定标准**: ✅ 已实现 / ⚠ 部分实现 / ❌ 真缺失
> **核对时间**: 2026-10-29

---

## 统计汇总

| 类别 | 文档标注 | 核对结果 | 翻案(✅) | 部分(⚠) | 属实(❌) |
|------|----------|----------|----------|---------|----------|
| 5. 认证计费 | 5 项 (4❌+1⚠) | 3✅ 1⚠ 1❌ | 3 | 1 | 1 |
| 7. 高级服务 | 9 项 ❌ | 5✅ 0⚠ 4❌ | 5 | 0 | 4 |
| 9. 设备设置 | 3 项 ❌ | 0✅ 1⚠ 2❌ | 0 | 1 | 2 |
| **合计** | **17 项** | **8✅ 2⚠ 7❌** | **8** | **2** | **7** |

> **翻案/部分率**: 59% (10/17) — 文档中标注 ❌ 的功能中，过半在代码层面已实现或部分实现且端点可达

---

## 逐项核对明细

### 5. 认证计费

| # | 功能 | 文档判定 | 核对结论 | 代码证据 | 实测端点 |
|---|------|----------|----------|----------|----------|
| 1 | **PPPoE 服务端** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/pppoe/` (config.go/apply.go/sessions.go)<br>`server.go:310-315,2083-2211` | `GET/POST /api/v1/pppoe/config`<br>`GET/POST /api/v1/pppoe/users`<br>`GET /api/v1/pppoe/sessions`<br>`GET /api/v1/pppoe/status`<br>`GET/POST /api/v1/pppoe/multidial` → **全 OK** |
| 2 | **Web 认证（Portal）** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/portal/` + `mgmtd/internal/authportal/`<br>`server.go:304-309,396-398,1949-2042` | `GET /api/v1/portal/clients`<br>`POST /api/v1/portal/authorize`<br>`POST /api/v1/portal/revoke`<br>`GET/POST /api/v1/portal/config`<br>`GET/POST /api/v1/portal/templates`<br>`GET /api/v1/portal/preview` → **全 OK** |
| 3 | **微信/短信/RADIUS/一键认证** | ❌ 无 | ⚠ **部分实现** | `mgmtd/internal/radius/` (client.go/apply.go)<br>`server.go:293-295,1895-1943`<br>`authportal/template.go` 内置 `wechat`/`sms`/`voucher`/`password` kind<br>**无 wechat/sms 网关对接包** | `GET/POST /api/v1/radius/config` → OK<br>`POST /api/v1/radius/test` → OK<br>**RADIUS 完整实现；微信/短信/一键仅为模板占位，无实际网关对接** |
| 4 | **VPN 服务端** (PPTP/L2TP/OpenVPN/IPSec) | ⚠ 仅 WireGuard | ⚠ **确认属实** | `mgmtd/internal/vpn/` (wireguard keypair/config 生成)<br>`mgmtd/internal/vpnclient/` (客户端配置)<br>`server.go:296-297,375-376,912-950` | `POST /api/v1/vpn/wireguard/keypair` → OK<br>`POST /api/v1/vpn/wireguard/config` → OK<br>`GET/POST /api/v1/vpn/client` → OK<br>**仅 WireGuard 客户端能力(生成密钥/渲染配置/下发)，非 VPN 服务端；无 PPTP/L2TP/OpenVPN/IPSec 任何代码** |
| 5 | **账号管理 / 套餐 / 计费** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/account/` (account.go/manager.go)<br>`mgmtd/internal/billing/` (billing.go/bill.go)<br>`mgmtd/internal/voucher/` (voucher.go)<br>`server.go:516-524,2970-2990, p17.go` | `GET/POST /api/v1/accounts` → OK<br>`DELETE /api/v1/accounts/{username}`<br>`POST /api/v1/accounts/{username}/reset-usage`<br>`POST /api/v1/accounts/{username}/bind-mac`<br>`POST /api/v1/accounts/import`<br>`GET/POST /api/v1/billing/plans` → OK<br>`GET /api/v1/billing/summary` → OK<br>`GET/POST /api/v1/billing/bills/generate` → OK<br>`GET/POST/DELETE /api/v1/voucher/*` → OK<br>`GET /api/v1/accounting/status/report` → OK |

---

### 7. 高级服务

| # | 功能 | 文档判定 | 核对结论 | 代码证据 | 实测端点 |
|---|------|----------|----------|----------|----------|
| 6 | **固件 OTA** | ❌ 无 OTA UI | ✅ **已实现** | `mgmtd/internal/firmware/` (image.go/upgrade.go)<br>`server.go:423-426, p11.go:31-94` | `GET /api/v1/firmware/inspect?path=...` → ERR(10301 路径不存在，端点存在)<br>`POST /api/v1/firmware/plan` → 端点存在<br>`POST /api/v1/firmware/apply` → 端点存在<br>`POST /api/v1/firmware/confirm` → 端点存在<br>**完整 OTA 流程：检查→计划→应用→确认(双分区/回滚机制在 upgrade.go)** |
| 7 | **Ping / Traceroute** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/server/p15.go:406-483` (runNetTool 统一骨架)<br>`server.go:371-373` | `POST /api/v1/diagnostics/ping` → OK<br>`POST /api/v1/diagnostics/traceroute` → OK<br>`POST /api/v1/diagnostics/portprobe` → OK<br>`POST /api/v1/diag/ping` → 端点存在 |
| 8 | **iPerf / 吞吐测速** | ❌ 无 | ❌ **真缺失** | 无 iperf 包、无端点 | `GET /api/v1/iperf` → 404<br>`GET /api/v1/speedtest` → 404<br>`GET /api/v1/throughput` → 404 |
| 9 | **WOL 网络唤醒** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/server/remote.go:13-204`<br>`server.go:347-349` | `GET/POST /api/v1/wol/targets` → OK<br>`POST /api/v1/wol/wake` → OK |
| 10 | **抓包（tcpdump）** | ❌ 无 | ❌ **真缺失** | 无 tcpdump/pcap 包、无抓包端点；仅有端口镜像 `GET/POST /api/v1/mirror` | `GET /api/v1/capture` → 404<br>`GET /api/v1/packet-capture` → 404<br>`GET /api/v1/tcpdump` → 404 |
| 11 | **SNMP** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/snmp/` (config.go)<br>`server.go:266-269,3603-3688` | `GET/POST /api/v1/snmp/config` → OK<br>`GET /api/v1/snmp/status` → OK<br>`POST /api/v1/snmp/query` → OK (需 snmpwalk 已装) |
| 12 | **内网穿透** | ❌ 无 | ❌ **真缺失** | 无 frp/nps/stun/turn 相关包、无端点 | `GET /api/v1/frp` → 404<br>`GET /api/v1/natpunch` → 404<br>`GET /api/v1/tunnel` → 404 |
| 13 | **路由体检** | ❌ 无 | ✅ **已实现** | `mgmtd/internal/diagreport/` (report.go)<br>`server.go:215,427-428, p11.go:106-135` | `GET /api/v1/health` → OK<br>`GET /api/v1/diagnostics/report` → OK<br>`GET /api/v1/diagnostics/report/text` → OK<br>**返回磁盘/内存/接口/负载/邻居/nft/路由/服务/失败单元 8 大 section** |
| 14 | **路由器重启** | ❌ 无 | ❌ **真缺失** | 无 reboot 端点、无 systemctl reboot 调用 | `POST /api/v1/system/reboot` → 404<br>`POST /api/v1/reboot` → 404 |
| 15 | **子网换算工具** | ❌ 无 | ❌ **真缺失** | 无 subnet/ipcalc 包、无端点 | `GET /api/v1/tools/subnet` → 404<br>`GET /api/v1/subnet/calc` → 404 |

---

### 9. 设备设置

| # | 功能 | 文档判定 | 核对结论 | 代码证据 | 实测端点 |
|---|------|----------|----------|----------|----------|
| 16 | **SSH / Telnet / FTP 开关** | ❌ 无 | ⚠ **部分实现** | `mgmtd/internal/server/remote.go:31-75`<br>`server.go:341-343`<br>`remote.Access` 含 `ssh_enabled`/`ssh_port`/`telnet_enabled`/`web_enabled`<br>**无 FTP 字段/逻辑** | `GET /api/v1/remote/access` → OK<br>`{"access":{"ssh_enabled":false,"ssh_port":0,"telnet_enabled":false,"web_enabled":false,...}}`<br>`GET /api/v1/remote/access/status` → OK (返回 ssh/telnet 监听状态)<br>**SSH/Telnet 开关完整实现，仅缺 FTP** |
| 17 | **任务队列** | ❌ 无 | ❌ **真缺失** | 无 taskqueue/jobqueue/worker 包、无端点 | `GET /api/v1/taskqueue` → 404<br>`GET /api/v1/jobs` → 404<br>`GET /api/v1/queue` → 404 |
| 18 | **双机热备（配置同步）** | ❌ 无 | ❌ **真缺失** | 无 ha/failover/cluster/sync 包、无端点 | `GET /api/v1/ha` → 404<br>`GET /api/v1/failover` → 404<br>`GET /api/v1/cluster` → 404 |

---

## 关键发现总结

### ✅ **8 项翻案**（文档 ❌ → 实为已实现）
1. **PPPoE 服务端** — 完整实现：配置/用户/会话/状态/多拨，含 chap-secrets 渲染与 nft 应用
2. **Web Portal 认证** — 完整实现：客户端授权/撤销、配置、内置 4 种模板(密码/卡密/短信/微信)、预览
3. **账号/套餐/计费/卡密** — 完整实现：账号增删改/用量重置/MAC 绑定/批量导入、套餐管理、出账、账务报表、Voucher 生成核销
4. **固件 OTA** — 完整实现：镜像检查/升级计划/应用/双分区确认回滚流程
5. **Ping/Traceroute/PortProbe** — 完整实现：统一 `runNetTool` 骨架，含防火墙提示
6. **WOL 网络唤醒** — 完整实现：目标持久化 + MAC/IP 唤醒
7. **SNMP** — 完整实现：配置下发(snmpd.conf 渲染)/状态查询/snmpwalk 实时查询
8. **路由体检** — 完整实现：8 大诊断 section，JSON/文本双格式输出

### ⚠ **2 项部分实现**
| 功能 | 实现程度 | 缺口 |
|------|----------|------|
| **微信/短信/RADIUS/一键认证** | RADIUS ✅ 完整；Portal 模板含 wechat/sms/voucher kind | 无微信/短信网关对接代码；Voucher 可作"一键认证"替代 |
| **SSH / Telnet / FTP 开关** | SSH/Telnet ✅ 完整 (remote/access 端点) | **无 FTP** 字段与服务管理逻辑 |
| **VPN 服务端** | WireGuard 客户端配置生成 ✅ | 非服务端模式；无 PPTP/L2TP/OpenVPN/IPSec |

### ❌ **7 项属实缺失**
| 功能 | 缺失原因 | 建议优先级 |
|------|----------|------------|
| iPerf / 吞吐测速 | 无 iperf3 集成 | **P1** — 网络运维刚需 |
| 抓包（tcpdump） | 无 pcap 库/端点 | **P1** — 故障排查核心 |
| 内网穿透 | 无 frp/nps/stun 客户端 | **P2** — 远程运维场景 |
| 路由器重启 | 无 systemctl reboot 端点 | **P1** — 基础运维 |
| 子网换算工具 | 纯算法，易补齐 | **P2** — 运维辅助 |
| 任务队列 | 无异步任务框架 | **P3** — 扩展性 |
| 双机热备 | 无 HA/状态同步机制 | **P3** — 高可用 |

---

## 端点速查表（回归测试用）

```bash
# 认证计费
GET  /api/v1/pppoe/config
GET  /api/v1/pppoe/users
GET  /api/v1/pppoe/sessions
GET  /api/v1/pppoe/status
GET  /api/v1/portal/config
GET  /api/v1/portal/clients
GET  /api/v1/portal/templates
GET  /api/v1/radius/config
POST /api/v1/radius/test
GET  /api/v1/voucher/list
GET  /api/v1/accounts
POST /api/v1/accounts
GET  /api/v1/billing/plans
GET  /api/v1/billing/summary
GET  /api/v1/accounting/report

# 高级服务
GET  /api/v1/firmware/inspect?path=/tmp/fw.bin
POST /api/v1/firmware/plan
POST /api/v1/diagnostics/ping        {"target":"1.1.1.1","count":3}
POST /api/v1/diagnostics/traceroute  {"target":"1.1.1.1"}
POST /api/v1/diagnostics/portprobe   {"target":"1.1.1.1","port":53}
GET  /api/v1/wol/targets
POST /api/v1/wol/wake                {"mac":"aa:bb:cc:dd:ee:ff"}
GET  /api/v1/snmp/config
GET  /api/v1/snmp/status
POST /api/v1/snmp/query
GET  /api/v1/health
GET  /api/v1/diagnostics/report

# VPN (仅 WireGuard 客户端配置)
POST /api/v1/vpn/wireguard/keypair
POST /api/v1/vpn/wireguard/config
GET  /api/v1/vpn/client

# 设备设置
GET  /api/v1/remote/access
GET  /api/v1/remote/access/status
POST /api/v1/remote/access          {"ssh_enabled":true,"ssh_port":2222,"telnet_enabled":true}
```

---

## 修正建议（给指挥位合并主文档用）

1. **更新第 5 类「认证计费」**:
   - PPPoE 服务端：❌ → ✅
   - Web 认证 Portal：❌ → ✅
   - 微信/短信/RADIUS/一键认证：❌ → ⚠ (说明改为"RADIUS 完整；微信/短信仅模板占位，无网关对接")
   - VPN 服务端：保持 ⚠ (说明改为"仅 WireGuard 客户端配置生成，非服务端")
   - 账号/套餐/计费：❌ → ✅

2. **更新第 7 类「高级服务」**:
   - 固件 OTA：❌ → ✅
   - Ping/Traceroute：❌ → ✅
   - iPerf：保持 ❌
   - WOL：❌ → ✅
   - 抓包：保持 ❌
   - SNMP：❌ → ✅
   - 内网穿透：保持 ❌
   - 路由体检：❌ → ✅
   - 路由器重启：保持 ❌
   - 子网换算：保持 ❌

3. **更新第 9 类「设备设置」**:
   - SSH/Telnet/FTP 开关：❌ → ⚠ (说明改为"SSH/Telnet 完整，FTP 缺失")
   - 任务队列：保持 ❌
   - 双机热备：保持 ❌

---

**报告路径**: `docs/feature-audit-adv.md`  
**不修改主文档**，由指挥位合并。

---

## 复核更新（2026-10-29，按代码实测）

下面这张表**覆盖**本文档上方的历史判定；历史上标 ❌ 的项已在 p106–p114 批次里补齐，
⚠ 项经代码核实也已真正生效（不再是"只渲染不下发"）。

| 原判定 | 当前状态 | 证据 |
|---|---|---|
| iPerf 吞吐测速 ❌ | ✅ 已实现 | `internal/iperf` + `POST /api/v1/tools/iperf/run`、`GET .../status` |
| 抓包 tcpdump ❌ | ✅ 已实现 | `server/tools_capture.go`（4 端点，CommandContext 保证 stop 真能杀进程，600s/512MB 硬上限） |
| 内网穿透 frp ❌ | ✅ 已实现 | `internal/frp` + `frp/apply\|status\|stop` |
| 路由器重启 ❌ | ✅ 已实现 | `POST /api/v1/system/reboot`，confirm=false → 422/10487 |
| 子网换算 ❌ | ✅ 已实现 | `GET /api/v1/tools/subnet`，返回 network/netmask/broadcast/first_host/last_host/host_count |
| 任务队列 ❌ | ✅ 已实现 | `internal/taskq` + `GET/POST /api/v1/tasks`、`.../cancel` |
| 双机热备 ❌ | ✅ 已实现（最小可用） | `internal/ha` + `ha/config\|sync\|import\|status`（X-HA-Secret 校验） |
| 微信/短信网关 ⚠ | ✅ 短信已接通，微信仍为模板 | `internal/sms` + `auth/sms/send\|verify`（503 未配置 / 429 限流 / 10526 无码） |
| 网址过滤 ⚠（只渲染） | ✅ 真正下发 | 域名→dnsmasq（address=/domain/0.0.0.0 + reload），IP→nft `ipv4_addr`；原实现把域名塞进 nft 集合，nft 加载即报错 |
| 应用协议控制 ⚠（只渲染） | ✅ 真正下发 | `dpi.ApplyMarks` 已在 `handleSetDPIRules` 调用 |
| DHCP 服务 ⚠（不重启） | ✅ 已重启 | `dhcp.Applier.Apply` 三路兜底（systemctl reload / init.d / kill -HUP） |
| MAC 黑白名单 ❌ | ✅ 已实现 | 端点实际是 `GET/POST /api/v1/acl/mac-filter`（审计只查了 `security/mac*` 才误判） |
| FTP 开关 ⚠ | ✅ 已实现 | `remote.Access.FtpEnabled/FtpPort`，端口与 SSH/Web 冲突拒绝保存，明文警告 |
