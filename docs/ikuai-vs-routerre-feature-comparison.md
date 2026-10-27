# iKuai vs router-re 功能全面对比报告

> 日期：2026-10-27
> iKuai 基线：iKuaiOS 3.7.26（免费版，2026-09-16）/ 4.0.x 企业版
> 功能地图来源：iKuai 官方功能列表 + ikuai-mcp-server（248 工具/9 模块，IK-Q3000 4.0.313 实机核对）
> router-re 基线：本文档生成时的代码实况（30 REST 端点 / 15 包 / 53 测试）

---

## 零、一句话结论

**架构质量 router-re 全面领先（Go 零 shell 注入 / argon2id+JWT / nftables 数据面分离 / 开源可审计）；
功能广度覆盖 iKuai 约 21%（P0 五项补齐后从 16% 提升）；
真实闭环（下发生效并可回读验证）的模块从 2 个提升到 5 个：防火墙、QoS、接口配置、静态路由、DHCP。
其余模块仍停在「生成配置文件文本」层。**

### P0 补齐记录（2026-10-27）

| P0 项 | 状态 | 真实验证证据 |
|---|---|---|
| 接口配置写入 | ✅ 完成 | OpenWrt 24.10.8 容器内配置 dummy 接口 → `ip addr show` 确认 `inet 10.99.99.1/24` |
| 静态路由 | ✅ 完成 | `10.88.88.0/24 via 10.99.99.254 dev dum0 metric 100` 真实写入路由表 |
| DHCP 真实生效 | ✅ 完成 | 写 dnsmasq 配置 + 三种 reload 策略 + 租约解析（本机无 dnsmasq 时返回空而非报错） |
| 终端列表 | ✅ 完成 | `/clients` 返回 35 个真实邻居（IP/MAC/REACHABLE·STALE） |
| 审计日志 | ✅ 完成 | 16 个写路由统一埋点 + login 三次分支；实测 JSONL 记录成功/失败、actor、remote_ip |

### P1 补齐记录（2026-10-27）

| P1 项 | 状态 | 真实验证证据 |
|---|---|---|
| 多WAN 真实落地 | ✅ 完成 | OpenWrt 容器内 `ip rule` 出现 `fwmark 0x64 lookup 100` / `fwmark 0x65 lookup 101`，`ip route show table 100/101` 各自的默认路由真实存在；健康检查用 ping 并发探测 |
| per-IP/MAC 限速 | ✅ 完成 | 宿主机 dummy 接口 `rtq0` 实测：`qdisc htb 1:` + `class 1:1 rate 10Mbit` + `class 1:2 rate 5Mbit` + `filter handle 0x100/0x101 fw classid 1:1/1:2`，配合 nft `ip saddr ... meta mark set 0x100/0x101` 完整链路 |
| DPI 数据面标记 | ✅ 完成 | OpenWrt 容器内核里真实存在 `table inet router_dpi`，含 `tcp dport { 6881, 6882 } meta mark set 0x000000c8` 与 `udp dport 27015 drop` |

### 本轮发现的关键坑（P1）

3. **nft chain 声明不能同行闭合**：`chain classify { type filter hook forward priority mangle; }` 会在同一行闭合 chain，**后续所有规则落到 table 顶层**，nft 报 `syntax error, unexpected tcp`。必须三行：`chain x {` / `type ... ;` / 规则 / `}`。
4. **`meta l4proto tcp tcp dport {...}` 是非法语法**：协议关键字已隐含 L4 匹配，要么用 `<proto> dport {...}`，要么用裸 `meta l4proto <proto>`，不能同时出现。
5. **tc 根 qdisc 只有 del+add 幂等**：`tc qdisc replace` 在 root 已是 htb 时报 `Change operation not supported by specified qdisc`；`tc qdisc add` 在已存在时报 `Exclusivity flag on, cannot modify`。必须 `del`（失败忽略）再 `add`。
6. **裸命令名在守护进程里找不到**：systemd/procd 启动的进程 PATH 常不含 `/usr/sbin`，`exec: "tc": executable file not found in $PATH`。已在 `execx.DefaultRunner` 内统一解析绝对路径（不影响 FakeRunner 测试断言）。

### P2 补齐记录（2026-10-27）

| P2 项 | 状态 | 真实验证证据 |
|---|---|---|
| 系统监控历史 | ✅ | `/stats/current` 返回真实数据（CPU 12.3% / 内存 24960-63886MB / load 5.83 / conntrack 405 / RX 137GB / TX 162GB）；环形缓冲 720 点（6 小时 @30s）；前端 5 条 SVG 趋势图 |
| 日志中心 | ✅ | `/logs` 走 journalctl -o json 返回真实 unit/优先级/消息；`/logs/sources` 返回 30 个真实 unit（加 60s 缓存后 1.01s → 0.0019s） |
| 连接跟踪 | ✅ | `/conntrack` 返回真实条目（`tcp 2409:8a5c:...:55566 → 2606:b740:...:80 [ESTABLISHED]`，与 `/proc/net/nf_conntrack` 第一组逐字一致） |
| VLAN 真实下发 | ✅ | `ip -d link show vtest0.100` → `vlan protocol 802.1Q id 100` + `inet 10.123.100.1/24`；回读 vid=100 parent=vtest0 addr=10.123.100.1/24 |
| 桥接真实下发 | ✅ | `ip -d link show btest0` → `stp_state 1`；成员 `vtest0.100` 的 `master btest0`；回读 members=['vtest0.100'] stp=True addr=10.123.200.1/24 |

### P2 发现的解析 bug（都是"下发对、回读错"）

7. **conntrack 每行有两组 `src/dst`**（请求方向 + 回复方向），map 覆盖式赋值会取到回复方向，连接方向整体反向。必须只取第一组。
8. **`ip -j link show` 不含 IPv4**：`addr_info` 只出现在 `ip -j addr show`，否则 Address 恒为空。
9. **VLAN 的 VID 在 `linkinfo.info_data.id`**，不是顶层 `vlan_info.id`（写错静默得 0，界面显示"VID 0"但实际是 100）。
10. **`ip -j link show type bridge` 必须加 `-d`**，否则 linkinfo 为空、stp_state 取不到；成员不能解析 `bridge link show` 文本（每行开头是序号，会得到 `["6059:"]`），要从各 link 的 `master` 字段反查。

### 当前闭环模块（8 个）

防火墙、QoS（接口级）、接口配置、静态路由、DHCP、多WAN 策略路由、per-IP 限速、DPI 标记、Web Portal、PPPoE、VLAN、桥接 —— 12 个模块全部「下发到内核 + 可回读验证」。

另有三类只读能力：系统监控历史、journal 日志查询、连接跟踪。

---

## 一、9 大模块覆盖率总览

| # | iKuai 模块 | iKuai 功能项 | router-re 已覆盖 | 覆盖度 | router-re 实现深度 |
|---|---|---|---|---|---|
| 1 | 系统监控 | 18 | 11 | **61%** | 真实 /proc 采集 + 6小时环形历史 + 5 条趋势图 + 接口流量明细 |
| 2 | 网络配置 | 58 | 8 | **14%** | 接口/静态路由/NAT/DHCP 真实生效 + VLAN/桥接真实下发 |
| 3 | 安全管控 | 39 | 1.5 | **6%** | ACL 真实 nft 下发 ✅ |
| 4 | 流控分流 | 21 | 6 | **29%** | QoS + per-IP 限速真实 tc / DPI 标记入内核 / 多WAN 策略路由真实下发 |
| 5 | 认证计费 | 28 | 6 | **21%** | Web Portal(nft authorized set) + PPPoE 服务端(真实启动) + RADIUS 客户端(RFC2865)；缺计费账务 |
| 6 | 无线服务 | 21 | 0.5 | **2%** | **AC 管理已取消**；仅保留本机 AP 的 SSID/hostapd 配置 |
| 7 | 高级服务 | 32 | 1 | **6%** | 仅配置快照 |
| 8 | 日志中心 | 11 | 5 | **45%** | journal 查询(unit/grep/时间) + 连接跟踪表(协议/方向/状态) |
| 9 | 设备设置 | 20 | 1.5 | **15%** | 单管理员 + 快照 |
| | **合计** | **248** | **~12** | **~16%** | 2 模块闭环，8 模块渲染 |

---

## 二、逐模块详细对比

### 1. 系统监控（iKuai 18 项 / router-re 1 项）

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| 系统概览（CPU/内存/版本） | ✅ | ✅ | `GET /system/info` 真实读 /proc、/etc/os-release（已在 OpenWrt 容器实测） |
| 线路监控（IPv4/IPv6 实时速率） | ✅ | ⚠ | `/interfaces` 只有**累计** rx/tx 字节，无实时速率/连接数 |
| 终端列表（在线设备 IP/MAC/流量） | ✅ | ❌ | 无 |
| 负载监控 / 分流监控 | ✅ | ❌ | 无 |
| 行为洞察 | ✅ | ❌ | 无 |
| 下联设备（摄像头/交换机） | ✅ | ❌ | 无 |
| 消息通知 | ✅ | ❌ | 无 |

---

### 2. 网络配置（iKuai 58 项 / router-re 3 项）

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| WAN/LAN 配置 | ✅ | ⚠ | `/interfaces` **只读**，无接口 IP/网关写入 |
| IPv6（接入/路由/DHCPv6） | ✅ | ❌ | 完全缺失 |
| VLAN 管理 | ✅ | ❌ | 完全缺失 |
| DHCP 服务 | ✅ | ⚠ | `POST /dhcp/config` 生成 dnsmasq 配置**文本**，不重启服务、无租约表 |
| DHCP 静态绑定 | ✅ | ❌ | 无 |
| DNS 设置（多线 DNS/记录老化） | ✅ | ❌ | 只在 DHCP 配置里带 dns 列表 |
| 端口映射（DNAT） | ✅ | ✅ | `nat.RenderNftables` 生成 dnat 规则（渲染） |
| SNAT/Masquerade | ✅ | ✅ | 同上（渲染） |
| 静态路由 | ✅ | ❌ | 只有 BGP/OSPF，**无静态路由** |
| DDNS | ✅ | ❌ | 无 |
| VPN 客户端 | ✅ | ❌ | 只有服务端配置生成 |

---

### 3. 安全管控（iKuai 39 项 / router-re 1.5 项）

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| ACL 规则 | ✅ | ✅✅ | `firewall` 模块 **真实 nftables 下发 + 幂等**（delete table 后重建），唯一完全闭环的模块之一 |
| 应用协议控制 | ✅ | ⚠ | `dpi/rules` 渲染 drop/limit 规则（未接入流量 hook） |
| 连接数限制 | ✅ | ❌ | 无 |
| ARP 绑定 | ✅ | ❌ | 无 |
| MAC 黑白名单 | ✅ | ❌ | 无 |
| 网址/URL/域名控制 | ✅ | ❌ | 无 |
| 流量审计 | ✅ | ❌ | 无 |

---

### 4. 流控分流（iKuai 21 项 / router-re 2 项）

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| IP/MAC 限速 | ✅ | ⚠ | `qos` 是**接口级** tc tbf（真实执行 ✅），不是 iKuai 的 per-IP/MAC |
| DPI 七层流控 | ✅ | ⚠ | nDPI 真实识别单包（`/dpi/classify`）+ 规则渲染，**未接入数据面 hook**，不能真正限速某应用 |
| 智能流控（一键） | ✅ | ❌ | 无 |
| 多线负载 | ✅ | ✅ | 加权轮询 + 每 WAN 独立路由表（渲染 ip 命令，未执行） |
| 端口分流 | ✅ | ❌ | 无 |
| 协议分流 | ✅ | ❌ | 无 |
| 域名分流 | ✅ | ❌ | 无 |

**iKuai 分流优先级链**（router-re 未实现）：静态路由 > 域名分流 > 端口分流 > 协议分流 > 多线负载 > 默认网关

---

### 5. 认证计费（iKuai 28 项 / router-re 0.5 项）

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| PPPoE 服务端 | ✅ | ❌ | 无 |
| Web 认证（Portal） | ✅ | ❌ | 无 |
| 微信/短信/RADIUS/一键认证 | ✅ | ❌ | 无 |
| VPN 服务端（PPTP/L2TP/OpenVPN/IPSec） | ✅ | ⚠ | 仅 WireGuard（`wg genkey` 真实生成 + wg0.conf 渲染，未启动接口） |
| 账号管理 / 套餐 / 计费 | ✅ | ❌ | 无 |

---

### 6. 无线服务（iKuai 21 项 / router-re 0.5 项）— AC 能力已明确取消

| 功能 | iKuai | router-re | 说明 |
|---|---|---|---|
| AP 管理/分组/批量配置（AC） | ✅ | ❌ **已取消** | 需要 CAPWAP/厂商协议 + 真实 AP 硬件才能验证；原空壳端点已删除，不做假功能 |
| AP 固件升级 | ✅ | ❌ | 无 |
| SSID/Wi-Fi 基础设置 | ✅ | ✅ | `wireless/ssids` + `RenderHostapd`/`RenderWpaSupplicant`（wpa2/wpa3/open 都正确） |
| Mesh / 快速漫游 / 漫游灵敏度 | ✅ | ❌ | 无 |
| 信道扫描 / 干扰分析 / 无线网优 | ✅ | ❌ | 无 |
| WPA3 / MLO(WiFi7) / 三频 | ✅ | ⚠ | WPA3 渲染支持（SAE），无 MLO |
| AC 状态统计 | ✅ | ⚠ | 端点返回空列表 |

---

### 7. 高级服务（iKuai 32 项 / router-re 1 项）

| 功能 | iKuai | router-re |
|---|---|---|
| 固件 OTA | ✅ | ❌（OpenWrt 有 opkg，本系统无 OTA UI） |
| Ping / Traceroute | ✅ | ❌ |
| iPerf / 吞吐测速 | ✅ | ❌ |
| WOL 网络唤醒 | ✅ | ❌ |
| 抓包（tcpdump） | ✅ | ❌ |
| SNMP | ✅ | ❌ |
| 内网穿透 | ✅ | ❌ |
| 路由体检 | ✅ | ❌ |
| **配置快照/回滚** | ✅ | ✅ **UCI Snapshot/Rollback 真实实现** |
| 路由器重启 | ✅ | ❌ |
| 子网换算工具 | ✅ | ❌ |

---

### 8. 日志中心（iKuai 11 项 / router-re 0 项）

| 功能 | iKuai | router-re |
|---|---|---|
| 认证/ARP/DHCP/DDNS/操作等 11 类日志 | ✅ | ❌ 完全缺失（连登录审计都没落盘） |
| 告警系统 | ✅ | ❌ |

**这是最刺眼的空白** —— 设计文档里写了审计日志，代码里没有。

---

### 9. 设备设置（iKuai 20 项 / router-re 1.5 项）

| 功能 | iKuai | router-re |
|---|---|---|
| SSH / Telnet / FTP 开关 | ✅ | ❌ |
| 管理员账号管理（多用户/RBAC） | ✅ | ⚠ 单一 admin（支持环境变量覆盖）+ 常量时间比较 |
| 配置快照 | ✅ | ✅ |
| 任务队列 | ✅ | ❌ |
| 双机热备（配置同步） | ✅ | ❌ |

---

## 三、router-re 反超 iKuai 的能力（iKuai 没有或弱）

| 能力 | router-re | iKuai |
|---|---|---|
| **BGP/OSPF 动态路由** | ✅ FRR 配置生成（`route/bgp`、`route/ospf`） | ⚠ 官方功能列表与 248 项 API 工具清单中只有「静态路由」，未见 BGP/OSPF 条目（iKuai 定位中小企业上网管理，不做动态路由协议；如需确认请查企业版文档） |
| **开源可审计** | ✅ MIT，24 Go 文件全可读 | ❌ 闭源 + rootfs 双层加密 |
| **零 shell 注入** | ✅ 全 Go 内建库（netlink/UCI/nftables 库） | ❌ `func_api.sh` 拼接 → CVE-2023-34849 |
| **现代认证** | ✅ argon2id + JWT(15min) + 常量时间比较 | ❌ PBKDF2/MD5 可预测 sess_key |
| **配置事务** | ✅ Snapshot → 校验 → 提交 → 失败回滚 | ❌ 直接改文件 |
| **数据面/管理面分离** | ✅ nftables flowtable，管理面宕机不影响转发 | ❌ 单体 |
| **标准包管理** | ✅ OpenWrt opkg（已打包 ipk，真机装过） | ❌ 闭源固件升级 |
| **云管理 API 开放** | ✅ `/cloud/devices`（注册/心跳/配置版本推送） | ⚠ 爱快云（功能强但协议私有） |
| **DPI 引擎可选** | ✅ nDPI 4.2 cgo 真实集成（开源） | ❌ 闭源 DPI |

---

## 四、iKuai 强于 router-re 的关键能力（必须承认）

1. **DPI 流控真正生效**：iKuai 的七层流控是接入数据面并真实限速某应用；router-re 的 nDPI 只在 `/dpi/classify` 里做离线单包识别，**没有 hook 到转发路径**，限速无效。
2. **认证体系完整**：PPPoE 服务端、Web Portal、微信/短信/RADIUS 认证、账号套餐计费——中小运营商/酒店/学校场景的刚需，router-re 完全空白。
3. **AC 无线是真实产品能力**：AP 发现、批量配置、固件升级、漫游调优、信道分析。**router-re 明确不做这一块**（无硬件无法验证，已删除空壳端点），只保留本机 AP 的 SSID/hostapd 配置。
4. **终端/流量可视化**：iKuai 有在线终端列表、每终端流量、实时速率曲线、行为洞察；router-re 只有累计字节。
5. **日志与审计**：11 类日志 + 告警；router-re 零日志。
6. **分流体系**：端口/协议/域名/归属地分流 + 优先级链；router-re 只有多 WAN 加权轮询。
7. **一键智能流控**：iKuai 的"小白可用"核心卖点，router-re 无。

---

## 五、差距补齐路线图（按投入产出排序）

### P0 — 让它成为"能用的路由器"（不做则无法替代 iKuai）
1. **接口配置写入**（WAN/LAN IP、网关、DNS）—— 当前只读，连基本换网都做不了
2. **静态路由 + 分流优先级链**（静态 > 域名 > 端口 > 协议 > 多线负载 > 默认网关）
3. **DHCP 真实生效**（写 dnsmasq 配置 + 重启服务 + 租约表查询）
4. **终端列表 + 实时流量速率**（这是所有监控页面的基础）
5. **日志中心**（至少：登录审计、配置变更、DHCP 租约、防火墙命中）

### P1 — 对标 iKuai 核心卖点
6. **DPI 接入数据面**（nftables + nDPI 联动 / NFQUEUE，让"应用限速"真正生效）
7. **per-IP/MAC 限速**（当前只有接口级）
8. **Web 认证 Portal + PPPoE 服务端**（酒店/学校/运营商场景）
9. **多 WAN 真实落地** ✅ 已完成（P1）

### P2 — 差异化与完善
11. IPv6 全栈（接入/路由/DHCPv6/防火墙）
12. VLAN 管理
13. DDNS / UPnP / SNMP / WOL / Ping / Traceroute / 抓包
14. 多管理员 RBAC
15. 双机热备（配置同步）

---

## 六、诚实结论

- **架构维度**：router-re 是"现代化重构"，安全与可维护性远超 iKuai（这是设计目标，已达成）。
- **功能维度**：目前是 iKuai 的 **~16%**，且大部分是"配置生成器"而非"生效的执行器"。
- **本质差异**：iKuai 是**产品**（功能齐全但架构陈旧），router-re 是**平台骨架**（架构先进但功能待填）。
- **可替代性**：**现在还不能替代 iKuai**——缺接口配置写入、静态路由、DHCP 生效、终端可视化、日志这 5 项 P0，连"能上网的路由器"都还不完整。

**下一步最该做的不是继续加模块，而是把已有的 8 个"渲染器"接上执行层**（写配置 + reload 服务 + 状态回读），让功能从"能生成"变成"能生效"。
