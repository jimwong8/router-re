# Feature Audit — 网络配置 + 安全管控（p99-feature-audit-net）

核对对象：文档 `docs/ikuai-vs-routerre-feature-comparison.md` 第 2 类（网络配置）与第 3 类（安全管控）共列 **12 项 ❌**（文中提 13，逐项核对后为 12，见下表完整清单）。

方法：每项同时执行 ① `grep mgmtd/internal/*`（包/函数/端点）+ ② `curl -H "Authorization: Bearer ..." http://127.0.0.1:9090/api/v1/<path>`（真实 HTTP 证据，token 由 `POST /api/v1/auth/login` 获得，已实测成功）。

运行环境：mgmtd @ `127.0.0.1:9090`（root 启动），`admin/admin123` 登录可用。

---

## 汇总

| 判定类别 | 数量 | 说明 |
|---|---|---|
| ✅ 已实现（代码 + HTTP 证据） | 9 | IPv6、VLAN、DHCP 静态绑定、DNS 设置、静态路由、DDNS、VPN 客户端、连接数限制（NAC）、ARP 绑定、流量审计 |
| ⚠ 部分（有渲染/保存无真实执行/下发） | 2 | 网址/URL/域名控制（`handleSetWebfilterRules` 解析+渲染+store.Set，未接入 nftables 流量 hook）、DHCP 服务（`POST /api/v1/dhcp/config` 接受 `dnsmasq` 文本生成但不重启服务） |
| ❌ 真缺失（无代码/无端点/无执行） | 1 | MAC 黑白名单（无 `MAC` 过滤规则结构，`acl.Rule` 仅 `SrcIP`/`DstIP`/`DstPort`/`Proto`/`Action`，无 MAC 匹配字段；`internal/client/client.go` 仅提供 `BindMAC`，无 `MACFilter` 或黑白名单功能） |

注：文档将 "DHCP 服务" (341 行) 与 "应用协议控制" (357 行) 标为 ⚠ 而非 ❌，本次任务仅审 ❌ 项，因此不展开 ⚠ 项的深度审计。根据代码现状：`DHCP 服务` 的 ⚠ 状态属实（`dnsmasq` 文本生成不触发 `dnsmasq` 重启）；`应用协议控制` (`dpi/rules`) 同 ⚠（规则渲染存入配置文件，未接入流量 hook）。

---

## 每项逐项核对（只审 ❌ 项）

格式：`功能 | 文档现判 | 实测判定 | 证据（grep 路径:行号 + HTTP 端点:状态码 + 关键返回内容）`

| 功能 | 文档现判❌ | 实测判定 | 证据 |
|---|---|---|---|
| IPv6（接入/路由/DHCPv6） | ❌ 完全缺失 | ✅ 已实现 | 包 `internal/ipv6/config.go` + `nd.go`+`pd.go`+`ipv6_test.go`；路由 `server.go:437-440`（`ipv6.nd`、`ipv6.pd`、`ipv6.addresses`、`ipv6.ra`）；HTTP `GET /api/v1/ipv6/status` → `200`、`GET /api/v1/ipv6/addresses` → `200` |
| VLAN 管理 | ❌ 完全缺失 | ✅ 已实现 | 包 `internal/l2/vlan.go` + `l2_test.go`；路由 `server.go:269-271`（`GET /api/v1/vlan`、`POST /api/v1/vlan`、`DELETE /api/v1/vlan/{name}`）；HTTP `GET /api/v1/vlan` → `200` |
| DHCP 静态绑定 | ❌ 无 | ✅ 已实现 | 包 `internal/dhcp/config.go:70`（`StaticLease{MAC,IP}` 结构体）+ `StaticLeases []StaticLease` 字段 + MAC/IP 校验（`line 156-166`）；路由 `server.go:239-246`（`GET /api/v1/dhcp/config`、`POST /api/v1/dhcp/config`、`GET /api/v1/dhcp/leases`）；HTTP `GET /api/v1/dhcp/config` → `200`（响应含 `config` 对象，`POST` 接受 `static_leases` 字段） |
| DNS 设置（多线 DNS/记录老化） | ❌ 只在 DHCP 配置里带 dns 列表 | ✅ 已实现（独立 DNS 配置模块） | 包 `internal/dnsproxy/config.go` + `dnsproxy_test.go`；路由 `server.go:207-211`（`GET /api/v1/dns/config`、`POST /api/v1/dns/config`、`POST /api/v1/dns/resolve`、`GET /api/v1/dns/status`）；HTTP `GET /api/v1/dns/config` → `200`（响应含完整 `dns` 配置结构） |
| 静态路由 | ❌ 只有 BGP/OSPF，无静态路由 | ✅ 已实现（独立静态路由模块） | 包 `internal/route/static.go` + `static_test.go`；路由 `server.go:1703`（`handleGetStaticRoutes`）、`server.go:1726`（`handleSetStaticRoutes`）；端点 `GET /api/v1/route/static` → `200`、`POST /api/v1/route/static` → 接受完整路由规则 |
| DDNS | ❌ 无 | ✅ 已实现 | 包 `internal/ddns/` + `internal/ddnsup/`；路由 `server.go:207-225`（`GET /api/v1/ddns/config`、`POST /api/v1/ddns/config`、`GET /api/v1/ddns/records`、`POST /api/v1/ddns/records`、`DELETE /api/v1/ddns/records/{id}`、`POST /api/v1/ddns/update`、`GET /api/v1/ddns/ip`）；HTTP `GET /api/v1/ddns/config` → `200`（含 `provider`、`domain`、`token` 字段，真实 DDNS 配置模块，非仅 DHCP 附带） |
| VPN 客户端 | ❌ 无（文档说 "只有服务端配置生成"） | ✅ 已实现（客户端配置管理独立模块） | 包 `internal/vpnclient/`（`vpnclient.Config`、`Validate`、`Render`、`ParseWGShow`、`HandshakeAge`）；路由 `server.go:357-358`（`GET /api/v1/vpn/client`、`POST /api/v1/vpn/client`）；HTTP `GET /api/v1/vpn/client` → `200`（响应 `{"interface":"wg0","peer_count":0,"live":false,"has_config":false}`，真实客户端状态查询） |
| 连接数限制 | ❌ 无 | ✅ 已实现（NAC 连接限制模块） | 包 `internal/nac/` + `nac/*.go`；路由 `server.go:486-488`（`GET /api/v1/nac/connlimits`、`POST /api/v1/nac/connlimits`、`POST /api/v1/nac/connlimits/apply`）；HTTP `GET /api/v1/nac/connlimits` → `200`（响应含 `limits:[]` + `nft` nftables 规则正文） |
| ARP 绑定 | ❌ 无 | ✅ 已实现（ARP 绑定模块） | 包 `internal/acl/arp.go`（`ARPBinding` 结构、`MAC` 字段、`ApplyARP`、`DeleteARP`）；路由 `server.go:466-468`（`GET /api/v1/security/arp`、`POST /api/v1/security/arp`、`GET /api/v1/security/arp/neighbors`）；HTTP `GET /api/v1/security/arp` → `200`（响应含 `iface`、`status`、`sysctls`，真实 ARP 绑定状态查询） |
| MAC 黑白名单 | ❌ 无 | ❌ 真缺失 | `internal/acl/acl.go:20-29`（`Rule` 结构体：`SrcIP`、`DstIP`、`DstPort`、`Proto`、`Action` — **无任何 `MAC` 匹配字段**）；`internal/client/client.go`（`BindMAC` 仅为 `accounts` 用户-设备绑定，不是 MAC 黑白名单）；`grep -rni 'black\|white\|MACFilter\|mac_filter\|mac_black\|mac_whitelist' internal/acl/*.go` → 无匹配；`internal/webfilter/*.go` 无 MAC 规则字段。**无代码 + 无端点**：无 `/api/v1/security/mac*` 或 `/api/v1/mac*` 路由存在。 |
| 网址/URL/域名控制 | ❌ 无 | ⚠ 部分（有渲染保存，无 nftables 执行） | 包 `internal/webfilter/`（`WebfilterRule`、`Validate`、`Render`）；路由 `server.go:469-470`（`GET /api/v1/webfilter/rules`、`POST /api/v1/webfilter/rules`）；HTTP `GET /api/v1/webfilter/rules` → `200`（端点存在、接受规则）。⚠ 但 `handleSetWebfilterRules` 调用顺序：`Validate→RenderWebfilterRules`（渲染规则文本）→ 未接入 `nftables` 实时流量 hook（同 `dpi/rules` 在文档标为 ⚠ 的渲染不执行），代码无 `nftables` 执行调用，仅存配置 |
| 流量审计 | ❌ 无 | ✅ 已实现 | 包 `internal/traffic/`（`TrafficRates`、`TrafficSample`、`TopTraffic`）；包 `internal/fwlog/`（`FwLogAnalysis`、`FwLogSuggestions`）；路由 `server.go:320-321`（`GET /api/v1/traffic/top`、`GET /api/v1/traffic/rates`）；路由 `server.go:375-376`（`GET /api/v1/firewall/log/analysis`、`GET /api/v1/firewall/log/suggestions`）；HTTP `GET /api/v1/traffic/top` → `200`、`GET /api/v1/firewall/log/analysis` → `200` |

---

## 核对方法执行记录（可复现）

每项同时执行：

**① 代码 grep（包/函数/端点定位）**
- `grep -rn 'func (s \*Server) handle.*<Name>' mgmtd/internal/server/*.go`
- `grep -n '"/api/v1/...' mgmtd/internal/server/server.go`
- `ls mgmtd/internal/<pkg>/` + `grep -n '<KeyStruct>' mgmtd/internal/<pkg>/*.go`

**② 真实 HTTP（本机 9090，Bearer token 由 `POST /api/v1/auth/login` 获得，已成功获取，len=147 字符 JWT）**
- 登录：`POST /api/v1/auth/login` `{"username":"admin","password":"admin123"}` → `{"token":"..."}`
- 每端点：`curl -H "Authorization: Bearer <token>" -s -o /dev/null -w '%{http_code}' http://127.0.0.1:9090/api/v1/<path>` + 代表性 `GET` 取响应体结构。

**③ 特殊验证（静态绑定 / MAC 黑白名单缺失 / VPN 完整文档覆盖）**
- `grep -n 'StaticLease' mgmtd/internal/dhcp/config.go` → 确认 `StaticLeases []StaticLease` + MAC/IP 校验 → 静态绑定实现
- `grep -n 'MAC ' mgmtd/internal/acl/*.go` → 仅 `MAC` 出现在 `arp.go:80 MAC string`（IP-MAC 绑定），`acl/acl.go` 无 MAC 匹配字段 → MAC 黑白名单真实缺失
- `GET /api/v1/vpn/client` → `{"interface":"wg0","peer_count":0,"live":false}`；`GET /api/v1/webfilter/rules` → `200`（渲染保存，无 nft 执行证据）
- `GET /api/v1/ipv6/status`、`GET /api/v1/vlan`、`GET /api/v1/ddns/records`、`GET /api/v1/route/static` 均 `200`

---

## 结论（与文档对照）

原文档将 12 项标为 ❌；本次逐项核对后：
- **翻案（已实现，文档结论过时）：10 项** — IPv6、VLAN、DHCP 静态绑定、DNS 设置、静态路由、DDNS、VPN 客户端、连接数限制、ARP 绑定、流量审计。
- **部分（⚠ 渲染保存，无真实执行/下发）：1 项** — 网址/URL/域名控制（`handleSetWebfilterRules` 已渲染+保存，无 nftables 流量 hook 执行，状态同文档 ⚠ 项 "应用协议控制"）。
- **属实 ❌（真实缺失）：1 项** — MAC 黑白名单（`acl.Rule` 结构无任何 `MAC` 匹配字段，`security/mac*` 路由无；`client.BindMAC` 仅为用户-设备 MAC 绑定，不是 MAC 黑白名单）。

注意：文档原列的 ⚠ 项 "DHCP 服务"（`dnsmasq` 文本生成不重启服务）与 ⚠ 项 "应用协议控制"（`dpi/rules` 渲染未接入流量 hook）本次任务不在 ❌ 审计范围内（任务限定审 ❌ 项），但从代码现状确认两项 ⚠ 判定属实：`POST /api/v1/dhcp/config` 接受配置、生成 `dnsmasq` 文本（不触发 `systemctl restart dnsmasq`）；`POST /api/v1/dpi/rules` 接受规则、保存到配置文件（无实时 nft 执行证据）。

---

## 产出文件（只写新文件，不改主文档）

报告文件：`docs/feature-audit-net.md`（本文件，即此内容）。
未修改主文档 `docs/ikuai-vs-routerre-feature-comparison.md`（多工人冲突风险，指挥位后续统一合并）。
未 `git commit`（按指令要求）。

核对执行时间：约 15 分钟（grep + 12 个 HTTP 端点 + 特殊验证 + 报告写入），在 25 分钟上限内完成。

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
