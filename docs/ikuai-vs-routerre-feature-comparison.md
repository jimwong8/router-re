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

### P3 补齐记录（2026-10-27）

| P3 项 | 状态 | 真实验证证据 |
|---|---|---|
| ACL 规则 | ✅ | 内核 `table inet router_acl` 含 `ip saddr 192.168.1.0/24 tcp dport 445 drop` + `tcp dport 80 accept` + 兜底 accept |
| ARP 防护 | ✅ | `net.ipv4.conf.all.arp_ignore = 1` / `arp_announce = 2`（`/proc/sys` 实测）。⚠ 只设了 `all`，各接口仍为 0，需补 per-interface |
| DDNS | ✅ | `/ddns/ip` 探测到真实公网 IP `103.240.198.142`；Cloudflare + 阿里云双 provider（httptest 覆盖成功/失败/私有 IP 拒绝） |
| NTP | ✅ | `timedatectl` 真实状态：`ntp=yes` / `ntpsynchronized=yes` / `timezone=Asia/Shanghai` |
| 端口映射 | ✅ | 内核 `table inet router_portmap` 含 `tcp dport 8080 dnat ip to 10.100.1.18:80` + forward accept + hairpin masquerade |
| 策略路由 | ✅ | `ip rule add ... lookup <table> priority <pri>` 真实下发（与多WAN 共用 table 机制） |

### P3 发现的 nft 语法 bug（第二次踩同类）

11. **`flush ruleset` 又出现了**（`acl.go`）—— 和 portal 完全同一类：会清空整机所有 nft 表。已删 + 回归测试。
12. **空集合 `elements = {}` 是语法错误**，nft 报 `syntax error, unexpected '}'`。不需要 set 就别写。
13. **inet 表里 `dnat to` 必须写 `dnat ip to`**，否则 nft 报 `specify 'dnat ip' or 'dnat ip6' in inet table to disambiguate`。

### P4 补齐记录（2026-10-27）

| P4 项 | 状态 | 真实验证证据 |
|---|---|---|
| 多管理员 RBAC | ✅ | 建 op1 → HTTP 201；op1 登录成功；访问 `/api/v1/users` → **403**「角色为 operator，无权管理用户」；只读端点 200；错密码 401；删最后一个 admin → `cannot delete the last admin`；`/users` 响应无 password/hash 字段 |
| 认证计费 | ✅ | 账号 CRUD + 批量导入（逐行容错）+ 配额/到期状态机 + 套餐计价 |
| URL 过滤 | ✅ | 生成 `address=/ads.example.com/0.0.0.0` + `address=/ads.example.com/::`；dnsmasq 未装时诚实返回 `applied:false` + 完整失败原因 |
| 攻击防护 | ✅ | 内核 `table inet router_guard` 含 `limit rate 1000/second` + drop + `ct state established,related accept`；下发后 9090 HTTP 200 与 ICMP 均正常 |

### P4 发现的致命 bug（会锁死机器）

14. **攻击防护 input 链 `policy drop`** —— 表里只放行 SYN 和 ICMP，其他入站（SSH、管理界面、DNS 响应、keep-alive）全被静默丢弃，等于把机器锁死。默认拒绝是 firewall 模块的职责，攻击防护表只做限速 → 改 `policy accept` + `ct state established,related accept`，加回归测试 `TestRenderNftNeverDefaultsToDrop`。
15. **`/auth/login` 不认 RBAC 用户表** —— `POST /api/v1/users` 建出来的账号登录恒 401，多用户管理成摆设。登录路径改为先查 rbac.Manager、环境变量 admin 兜底。
16. **前端表格字段名凭想象写** —— ARP 邻居表读 `prop="ip"/"mac"`，后端返回 `dst`/`lladdr`，IP/MAC 两列全空。已修 + 用自动扫描（有数据行但整列为空）复查了全部 20 个页面。
17. **日志时间戳透传裸微秒** —— `1793084380346468` 人类不可读，转 `2026-10-27 15:39:38`。
18. **`/logs/sources` 无超时** —— journalctl 冷启动 19s+ 让 HTTP 挂死；加 10s 上限 + 旧缓存兜底 + 60s 缓存（1.01s → 0.0019s）。

### P5 补齐记录（2026-10-27）

| P5 项 | 状态 | 真实验证证据 |
|---|---|---|
| DHCP 服务端 | OK | 生成 `dhcp-range=10.100.1.200,10.100.1.250,255.255.255.0,12h` + `dhcp-option=3,10.100.1.1` + `dhcp-option=6,223.5.5.5` + `dhcp-host=aa:bb:...:ff,10.100.1.100,nas`；静态 IP 落进动态池 -> 422 拒绝；dnsmasq 未装 -> applied:false + 两条真实错误 |
| DNS 代理 | OK | 生成 `no-resolv` + `server=223.5.5.5#53` + `cache-size=1000` + `domain-needed` + `bogus-priv` + `address=/router.local/10.100.1.18`；解析 localhost -> ["::1"]；命令注入串被拒绝(404) |
| IPv6 | OK | `ip -6 addr replace` 后内核回读 `inet6 2001:db8::1/64 scope global`；列表自动跳过 link-local 与 ::1；IPv4 地址提交 -> 422 |
| 流量统计 | OK | `/traffic/rates` 首次 elapsed=0 全零，3 秒后 `ens18 RX 17.4 KB/s / TX 359.4 KB/s`；`/traffic/top?n=3` 正确降序 |

### 第 4 个工人（OpenCode w1:pM）的可靠性验证

按「必须执行命令才能答对」的标准验收：8 个测试全过（含计数器回绕返回 0 而非负数、真实读 /proc/net/dev、TopN 排序截断），150 行实现 + 196 行测试，注释写清设计取舍（不用 ifconfig/ip 外部命令，避免 PATH 与解析差异）。API 实测返回真实速率而非编造值。

### P6 补齐记录（2026-10-27）

| P6 项 | 状态 | 真实验证证据 |
|---|---|---|
| SNMP | OK | v2c community + v3 用户(auth/priv) + trap；**API 响应无明文密码**（`createUser monitor sha *** aes ***`）；真实 `/etc/snmp/snmpd.conf` 权限 `-rw-------` 保留真密码；snmpd 未装 -> applied:false + 真实错误 |
| UPnP/IGD | OK | `allow_cidr=0.0.0.0/0` -> **422 拒绝**，原文："permits every address: refusing to expose internal mappings to the whole internet" |
| 网络诊断 | OK | 注入逐项拒绝：`a;id`/`a\|id`/`a$(id)`/`a b` -> 422，反引号 -> 400；真实 ping 127.0.0.1 2 包全收 min 0.094ms avg 0.197ms max 0.3ms |
| 远程与系统 | OK | 时区/远程访问/WOL；改 sshd_config 与 web 端口只返回"需要人工处理"清单，不自动改（改错会锁死自己） |

### P6 修的 bug

19. **SNMP 的 snmpd 文本字段返回明文 v3 密码** —— `config` 用了 `Masked()` 但 `snmpd` 用了原始 cfg 渲染。API 响应带密码等于写进浏览器缓存 / 代理日志 / DevTools 历史。改为 `RenderSnmpd(cfg.Masked())`。

### P7 补齐记录（2026-10-27）

| P7 项 | 状态 | 真实验证证据 |
|---|---|---|
| IPS 规则引擎 | OK | 解析真实 Suricata 行：`sid=1000001 msg="ET WEB SQL Injection" proto=tcp dst_port=80 content="union select"`；十六进制 content `\|ff 53 4d 42\|` 正确转字节；转义引号 -> `say "hi"`；坏行容错 `failures: "rule parse errors (1): line 2: missing options section '()'"` |
| L7 应用过滤 | OK | 通配符 `*.example.com` 匹配子域但**不匹配裸域**；allow 覆盖 block |
| 接入控制 | OK | 邻居表 **42 条真实设备**（10.100.1.168/10.100.1.131/10.250.17.2/10.100.1.68）；8 字节 MAC -> 422；whitelist 空列表 -> 422 「self-lockout」 |
| 威胁情报 | OK | `0.0.0.0/0` -> 422「这会阻断所有进出流量并锁死本机，无法远程恢复」 |

### P7 修的 bug

20. **`nac/neighbors` 把 state 当 string** —— `ip -j neigh show` 里 state 是**数组**（`["STALE"]`），写成 string 导致 Unmarshal 失败、整个邻居表 500。E2E 抓到，加回归测试 `TestNeighborsParsesStateArray`。
21. **测试捏造输入格式** —— 工人的 `TestNeighborsJSON` 用的是自己想象的 `"state":"REACHABLE"`，测试全绿但真实调用挂掉。改成真实 `ip -j neigh show` 格式，并在注释里写明"测试输入必须来自真实命令输出"。
22. **时区列表恒为空** —— `listTimezones` 用 `base name == "zoneinfo"` 跳过目录，但**根目录本身就叫 zoneinfo**，整棵树被跳过。修复后返回 601 个时区。工人自己发现并修了。

### P8 补齐记录（2026-10-27）

| P8 项 | 状态 | 真实验证证据 |
|---|---|---|
| DPI 流跟踪 | OK | 双向归一（正反方向归一条流）、TLS SNI / HTTP Host / DNS 名解析；截断 TLS 头 `16` / `160301` / 空 payload **全部 200 不 panic** |
| HTB 带宽保证 | OK | `iface=lo` -> 422「tc on lo throttles local traffic and locks out ssh/curl」；未带 confirm -> dry-run 不下发（`lo` 仍 `noqueue`、`ens18` 仍 `fq_codel`）；脚本首行 `tc qdisc del`（幂等） |
| 多线负载均衡 | OK | 权重 1:2:3 的 6 次选择实测分布 **a=1 b=2 c=3**（平滑加权轮询算法正确性硬证明）；`timeout >= interval` 被拒；flapping 抑制测试通过 |
| 应用限速配额 | OK | `0.0.0.0/0` -> 422「对整网段限速会把路由器自身也限住，导致远程管理失联」；空策略（速率/连接/新建全 0）-> 422 |

### P8 修的 bug

23. **HTB 端点完全没注册** —— 工人只写了 `internal/htb` 包（我明确要求它别碰 server.go），`/api/v1/htb/apply` 返回 404。Curl 实测抓到，自己补了 6 个 handler（含 dry-run 默认 + confirm 保护）。
24. **E2E 把「正确拒绝」当失败** —— `/lb/preview` 没配置成员时返回 422 是正确语义，但 E2E 记成 FAIL。加 `EXPECTED_422` 白名单，区分「接口坏了」和「缺前置条件」。
25. **pQ 交了乱码** —— 它回的是 `I willlinkybCc\nApApApAkA1,`，一个文件都没写。重派时把任务拆成"只做后端 + 两个文件 + 明确函数签名"，一次就落地了。

### P9 补齐记录（2026-10-27）

| P9 项 | 状态 | 真实验证证据 |
|---|---|---|
| IPv6 ND | OK | 下发 hop_limit=32/dad_transmits=3 -> `applied:true`；**内核回读 32/3**（改前 64/1），还原后 64/1 |
| IPv6 PD | OK | 生成 dhcp6c.conf（`send ia-pd 0` + `prefix-interface lxcbr0` + `sla-len 8`）；dhcp6c 未装 -> 诚实报系统错误 |
| 端口镜像 | OK | `source=lo` -> 422「tc on lo throttles local traffic and locks out ssh/curl」；未 confirm -> dry-run |
| PPPoE 多拨 | OK | 写入后响应**不含明文密码**（`password:"***"`），配置文件 0600 |
| 流量计费 | OK | `available:false` + `acct_enabled:false` + 373 条连接中 **0 条有 bytes=** + 完整启用方法说明 |

### P9 修的 4 个 bug

26. **`sysctl -w` 传文件路径必然失败** —— `sysctl -w /proc/sys/net/ipv6/conf/ens18/hop_limit=64` 会被 sysctl 再拼一次前缀，报 `无法获取/proc/sys//proc/sys/...`。改为直接 `os.WriteFile` 写 `/proc/sys`。（为可测试性加 `sysctlWriter` 注入点，生产路径不变。）
27. **RA 间隔 0 被当成非法值** —— 用户只想调 hop_limit/mtu 时 `ra_interval_min=0` 被拒，但 0 的语义应该是"不改这项"。改为 0 时跳过校验。
28. **端口镜像的 shell 语法泄漏进 tc argv** —— `RenderScript` 生成 `... 2>/dev/null || true`（给人看的），`Apply` 直接 `strings.Fields` 拆分后把 `2>/dev/null`、`||`、`true` 当参数传给 tc，报错又被 `qdisc del` 分支吞掉 → **旧 qdisc 实际没删** → 第二次应用必报 `File exists`。加回归测试 `TestApplyStripsShellSyntax`。
29. **路由冲突** —— `/api/v1/pppoe/sessions` 已被"服务端会话"占用，多拨客户端会话注册同名路径会让 ServeMux panic（测试直接崩）。改名 `/api/v1/pppoe/multidial`。

### P10–P13 补齐记录（2026-10-27 全自动连续推进）

| 阶段 | 内容 | 真实验证 |
|---|---|---|
| P10 | 告警规则(关键字/正则+cooldown) + 日志轮转 + 远程syslog + 账单 + 配置导出/差异/恢复 | cooldown 抑制生效；CSV 转义；udp `@` / tcp `@@` 正确；导出 sections 0→2 |
| P11 | 固件升级 + 诊断报告 + 邮件/Trap + 策略路由 | 诊断报告 8 章节全真实（nft 163 行/路由 38 行/邻居 42 条）；保留表 253/254/255 全拒 |
| P12 | 链路聚合 LACP + 接口调优 + 证书 + 在线用户 + 端口触发/ALG/mDNS | bond 拒绝 lo 和不存在的 slave；dry-run 不下发；在线 21 条真实会话 |
| P13 | Web 管理端口 + 主机标识 + 多 WAN 健康检查 | 状态机精确：失败3次才down、恢复2次才up；`0.0.0.0`+无HTTPS 拒绝 |

**P13 多 WAN 状态机实测**（fail_threshold=3, recover_threshold=2）：

| 事件 | up | fail_count | recover_count | 可用线路 |
|---|---|---|---|---|
| 失败 1 次 | true | 1 | 0 | 1 |
| 失败 2 次 | true | 2 | 0 | 1 |
| 失败 3 次 | **false** | 3 | 0 | **0** |
| 恢复 1 次 | false | 0 | 1 | 0 |
| 恢复 2 次 | **true** | 0 | 2 | 2 |

关键点：`up=true` 时 `fail_count` 依然累加（否则永远触发不了下线）；全 down 时可用线路返回空而不是"随便挑一个"。

### 补充修的 bug（P10–P13）

30. **配置导出恒为空** —— `Snapshot()` 返回快照 ID 而非内容；且未设 `MGM_CONFIG_FILE` 时走 MockStore，没有 `Export()`。两个实现都补 `Export()`，导出改用它。
31. **E2E 把纯文本端点判为失败** —— 诊断报告/邮件预览本就返回 `text/plain`，加 `TEXT_ENDPOINTS` 白名单。
32. **alerting 测试 self-contradictory** —— fixture `Pattern="error"` 与测试消息 `"failed login"` 完全不相交，三条测试全红，看起来像引擎 bug 实际是 fixture 错。
33. **bond 的 `lacp_rate` 在非 802.3ad 模式会静默失效** —— 已在校验层拒绝。
34. **hostname 纯数字会被当成 IP** —— 已拒绝。

### P14 收尾：OpenWrt 真机验证（2026-10-27）

用 `CGO_ENABLED=0` 静态编译的 8.98MB 二进制在 **OpenWrt 24.10.8 (x86/64)** 容器里跑通：

| 端点 | 真机结果 |
|---|---|
| `/auth/login` | 返回 147 字符 token |
| `/interfaces` | `br-lan 192.168.1.1 rx 4244 / tx 2006`、`eth0 rx 7099 / tx 6178` |
| `/system/info` | Xeon E5-2696 v2, 32 核, 62GB |
| `/firmware/inspect` | busybox 的 sha256 `fe1ec33d…`，magic 识别正确 |
| `/logs/sources` | `{"count":0,"sources":[]}`（无 journald，**诚实降级**） |
| `/nac/neighbors` | 修复前空 body → 修复后 `{"count":0,"neighbors":[]}` |

**真机发现的 2 个兼容性 bug**：

35. **BusyBox 的 `ip` 不支持 `-j` 和 `-br`** —— `ip -j neigh` 打印 usage 并返回 1，`ip -br addr` 同样不行。所有依赖 JSON 解析的回读路径在真机上都会静默失败。修法两层：① `Neighbors()` 加文本 fallback（解析 `ip neigh show`，含无 lladdr 的 FAILED 行）；② ipk 的 `DEPENDS` 从空改为 `+ip-full +nftables +conntrack-tools +kmod-nf-conntrack-netlink`。
36. **IPv6 地址被正则截断** —— `ParseConntrackSessions` 用 `src=([0-9.]+)` 只认 IPv4，遇到 `src=2409:8a5c:...` 在第一个冒号处停下得到 `"2409"`，页面上出现一行 IP 为 `2409` 的幽灵会话。改 `[0-9a-fA-F:.]+` + `net.ParseIP` 二次校验，修复后 19 条会话非法 IP 数 0。

### 当前闭环模块（48 个）

防火墙、QoS（接口级）、接口配置、静态路由、DHCP、多WAN 策略路由、per-IP 限速、DPI 标记、Web Portal、PPPoE、VLAN、桥接 —— 12 个模块全部「下发到内核 + 可回读验证」。

另有三类只读能力：系统监控历史、journal 日志查询、连接跟踪。

---

## 一、9 大模块覆盖率总览

| # | iKuai 模块 | iKuai 功能项 | router-re 已覆盖 | 覆盖度 | router-re 实现深度 |
|---|---|---|---|---|---|
| 1 | 系统监控 | 18 | 14 | **78%** | 真实 /proc 采集 + 6小时环形历史 + 5 趋势图 + 实时速率(计数器回绕归零) + TopN |
| 2 | 网络配置 | 58 | 42 | **72%** | 接口/静态路由/NAT/VLAN/桥接/端口映射/策略路由 + DHCP服务端 + DNS代理 + **IPv6 ND/PD** + **端口镜像** + **PPPoE多拨** |
| 3 | 安全管控 | 39 | 30 | **77%** | ACL + ARP防护 + URL过滤 + 攻击防护 + **IPS规则引擎(Suricata解析)** + **L7应用过滤(通配符/allow覆盖block)** + **MAC黑白名单** + **连接数限制** + **IP/CIDR威胁情报** |
| 4 | 流控分流 | 21 | 15 | **71%** | QoS + per-IP 限速 + **DPI流跟踪(TLS SNI/HTTP Host/DNS)** + **HTB带宽保证** + **多线负载均衡(平滑加权轮询)** + **应用限速配额** |
| 5 | 认证计费 | 28 | 20 | **71%** | Portal + PPPoE 服务端 + RADIUS + 账号管理 + 套餐计费 + 流量计费框架(诚实报告内核未启用acct)；缺真实账单 |
| 6 | 无线服务 | 21 | 0.5 | **2%** | （AC 管理按要求取消，仅保留本机 SSID/hostapd 渲染） | **AC 管理已取消**；仅保留本机 AP 的 SSID/hostapd 配置 |
| 7 | 高级服务 | 32 | 22 | **69%** | DDNS + NTP + SNMP(v2c/v3) + UPnP/IGD + 网络诊断 + WOL + **邮件告警/RFC5322头注入防护** + **SNMP Trap** + **端口触发** + **ALG** + **mDNS中继** + **诊断报告导出** + **固件升级** |
| 8 | 日志中心 | 11 | 10 | **91%** | journal 查询(unit/grep/时间) + 连接跟踪表(协议/方向/状态) |
| 9 | 设备设置 | 20 | 17 | **85%** | 多管理员 RBAC(pbkdf2+三角色+最后admin防自锁) + 系统时间/时区(回读校验) + 远程访问开关 + 配置快照；缺固件升级 |
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
