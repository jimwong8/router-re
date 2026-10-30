# SSTP 应用案例与技术参考（全网检索，2026-10-31）

检索口径：英文/俄文/中文社区 + 厂商文档 + 学术测量论文 + 开源项目文档。
每条结论后面标了出处，未标出处的都是我自己的推断，会明确写成"推断"。

---

## 一、真实落地案例

### 1. 云厂商：cloudimg 的 accel-ppp Azure 镜像（把 SSTP 定为 Azure 上的推荐传输）

- 该镜像明确写：「SSTP (TCP 443) — Yes — the recommended mode on Azure. Runs over TLS on a
  single TCP port, so it traverses the Azure virtual network and most restrictive client
  networks. Works with the Windows built in SSTP client and with sstp-client on Linux.」
- 同页对比了 L2TP（UDP 1701，可用）、PPPoE（Azure 不传二层，不可用）、PPTP（明确不建议：
  「PPTP's MS-CHAPv2 and MPPE construction is considered broken」）。
- 生产路径（可直接抄的参数）：
  `/usr/local/sbin/accel-pppd`、`/usr/local/bin/accel-cmd`、
  `/etc/accel-ppp/accel-ppp.conf`、证书 `/etc/accel-ppp/ssl/`、
  用户库 `/etc/ppp/chap-secrets`、管理 CLI 只绑 127.0.0.1:2000/2001。
- 客户端命令（Linux，sstp-client）：
  `sudo sstpc --cert-warn --user '<u>' --password '<p>' <ip>:443 noauth nodetach usepeerdns require-mschap-v2 noipdefault defaultroute`
- 站点给了 `SSTP_CERT_SHA256=48:48:B0:...` 这种证书指纹形式——正是 Windows SSTP 客户端
  「证书哈希」绑定用的同一套东西。
  来源：https://cloudimg.co.uk/guides/accel-ppp-on-ubuntu-24-04-azure

### 2. Docker 化的 accel-ppp SSTP（utoro/accel-ppp）

- 关键点不是 Docker 本身，而是它把运行必要条件写死了：
  `--device /dev/ppp --cap-add=NET_ADMIN -p 443:443`，证书放 `/accel-ppp/certs/fullchain.pem`
  与 `privkey.pem`，没给证书就自动生成自签。
- 这说明：**SSTP 服务端的硬门槛是 `/dev/ppp` + NET_ADMIN**，和我们 OpenVPN 要 `/dev/net/tun`
  是同一类问题（宿主机 cgroup 放行）。
  来源：https://hub.docker.com/r/utoro/accel-ppp

### 3. 论坛实战：证书链/哈希配错导致「无法验证证书签名」

- accel-ppp 官方论坛帖（俄语）里，用户 [sstp] 段配了 `ssl-ca-file / ssl-pemfile / ssl-keyfile`
  + `cert-hash-proto=sha256`，Windows 客户端仍报「не удаётся проверить подпись сертификата」。
  社区给的排查路径是：CA 必须导入 Windows「受信任的根证书颁发机构 → 本地计算机」，
  且 `cert-hash-proto` 与客户端实际支持的哈希要一致。
- 对我们有用的结论：**证书链的正确性比协议本身更容易出错**，
  且「自签 CA + 手动导入」在 Windows 客户端上是无法绕过的步骤（除非用 cert hash 指纹绑定）。
  来源：https://accel-ppp.org/forum/viewtopic.php?t=2350

### 4. Microsoft 自己在退场（重要反面案例）

- Azure VPN Gateway 文档：「Due to limited capability and suboptimal performance, we're
  retiring SSTP protocol」，并写明 **SSTP 最多 128 个并发连接，与网关 SKU 无关**。
- 迁移路径：加 IKEv2 或换成 OpenVPN（「SSTP and OpenVPN are both TLS-based protocol,
  they can't coexist on the same gateway」——注意这条是 Azure 网关的实现限制，不是协议限制；
  accel-ppp 里 SSTP 与其它协议共存没有这个问题）。
  来源：https://learn.microsoft.com/en-us/azure/vpn-gateway/ikev2-openvpn-from-sstp
- 判读：SSTP 不是"先进协议"，它是"最后的兼容手段"。做产品时定位要准确——
  **它解决的是"只能 443 且必须 Windows 原生"的场景，不是性能场景**。

### 5. 学术测量：野外 SSTP 服务器确实存在但规模小

- 《Characterizing the VPN Ecosystem in the Wild》（arXiv 2302.06566）对 IPv6 空间扫描：
  约 120 万 TCP/443 响应主机中，识别出 2070 台 OpenVPN、**949 台 SSTP**；
  其中 852 台 SSTP 真正响应，97% 开在 443。
- 论文表格里 SSTP 一行：Transport=TCP、Port=443、(D)TLS=TLS、
  「Server detection possible」——这句话很关键：**SSTP 是可被识别的**（见第六节）。
  来源：https://arxiv.org/pdf/2302.06566

### 6. 中文/俄文社区对"能不能翻墙"的实测口径

- Habr 文章直接点破：审查方要确认你是不是 SSTP，只需按协议标准向你的服务器发一个
  `SSTP_DUPLEX_POST /sra_{BA195980-CD49-458b-9E23-C84EE0ADCD75}/` 请求，
  **SSTP 服务端会老实地承认自己是 SSTP**——所以它对主动探测没有隐蔽性。
  来源：https://habr.com/ru/articles/710980/
- Hacker News 上的中国用户经验：「I was in China last April, SSTP worked fine (though
  long-lived connections tend to become slower over time, and then need a few minutes of
  cool down before being usable again).」——能用，但长连接会退化（这和 TCP-in-TCP 的
  meltdown 特征一致，见第五节）。
  来源：https://news.ycombinator.com/item?id=15590007

---

## 二、协议流程（MS-SSTP 官方，逐步）

MS-SSTP Overview 原文顺序：

1. 客户端向服务器 TCP 443 建连；
2. SSL/TLS 握手；
3. HTTPS request-response 完成（这就是那个 `SSTP_DUPLEX_POST` 请求）；
4. SSTP 协商开始（控制包：参数协商 + 防中间人）；
5. PPP 协商启动，PPP 认证完成或跳过；
6. SSTP 协商完成；
7. PPP 协商完成；
8. 进入就绪态，开始传 IP 包。

来源：https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-sstp/70adc1df-c4fe-4b02-8872-f1d8b9ad806a

原文还写明设计初衷：PPTP 被 GRE 端口封锁、L2TP/IPsec 被酒店类网络拦截，
SSTP 用 HTTPS 穿越「most firewalls and web proxies」。

---

## 三、accel-ppp 服务端技术细节（比官方文档页更细的部分）

### 配置项（文档原文，逐条可查）

`[sstp]`：
- `bind=` IP / `unix:path` / `unix:@abstract`
- `port=` 默认 443
- `accept=ssl,proxy`（proxy = PROXY protocol v1/v2，用于前置反代/负载均衡透传真实源 IP）
- `ssl-pemfile=` `ssl-keyfile=` `ssl-ca-file=` `ssl-dhparam=` `ssl-ecdh-curve=` `ssl-ciphers=`
- `cert-hash-proto=sha1,sha256`、`cert-hash-sha1=`、`cert-hash-sha256=`
- `timeout=60`、`hello-interval=60`（Echo-Request 保活，超时无响应就断）
- `ppp-mode=auto|async|ppposeq`、`ppp-max-mtu=1452`（最大 4087）
- `ifname=sstp%d`、`ip-pool=`、`session-timeout=`

`[modules]` 里必须启用 `sstp`、对应的 auth 模块（`auth_mschap_v2` 等）、`ippool`。
SSTP 数据面在用户态（PPTP/L2TP/PPPoE 走内核），所以 SSTP 的性能上限主要吃 CPU 和
单核 TLS 加解密——这条与第五节 TCP meltdown 叠加，是 SSTP 吞吐不如 WireGuard 的根本原因。

来源：https://docs.accel-ppp.org/configuration/sstp.html

### 监控/运维接口（可直接接进 mgmtd）

- `accel-cmd show sessions`：列 ifname/username/ip/type/state/uptime/rx-bytes/tx-bytes，
  type 会明确显示 `sstp`（配置项 `sessions-columns` 可改默认列）。
- `accel-cmd show stat`：守护进程统计。
- `accel-cmd terminate ip <addr>` / `terminate username <u>`：踢人。
- TCP 2000 是 accel-cmd 接口，2001 是 telnet，可用 `[cli] password=` 设口令。
- RADIUS CoA/DM 也能断会话（`radclient -x 127.0.0.1:3799 disconnect`）。
  来源：https://docs.accel-ppp.org/guides/control_features.html
  与 https://docs.accel-ppp.org/configuration/cli.html

结论：**mgmtd 接 SSTP 状态不需要解析日志文件**，直接 `accel-cmd show sessions` 拿结构化表格，
比现在我们 `cat /var/log/openvpn/rr-status.log` 再解析 CSV 更干净。

### 认证与用户体系

- 支持 PAP / CHAP-MD5 / MS-CHAPv1 / MS-CHAPv2；**不支持 EAP**；不支持 PPP 压缩。
- 用户来源二选一：RADIUS 或 pppd 兼容的 `chap-secrets`；
  官方明确警告两者同时加载没有可靠的自动回退（按模块注册顺序被查询）。
  来源：https://github.com/accel-ppp/accel-ppp

### 已知坑

- **PPTP 出树内核模块与内核自带 `ip_gre` 冲突**，不能混用；也不能与 poptop 的 pptpd 混跑
  （官方 README 原文）。我们如果只跑 SSTP，可以完全不编 PPTP 模块，绕开这个坑。
- SSTP 的 `ppp-mode=ppposeq` 需要额外内核模块 `ppposeq`；`auto` 模式没有就用 async PPP。
  切换 ppp-mode 需要重启守护进程（不是 reload）。
- `/dev/ppp` 不存在时 accel-pppd 起不来——和 `/dev/net/tun` 是同一类宿主授权问题。

---

## 四、客户端侧技术细节（sstp-client，Linux）

- `sstpc [options] <server> [ppp options]`；`<server>` 后可跟 pppd 参数。
- 关键选项：`--ca-cert`（校验服务端 CA）、`--ca-dir`、`--cert-warn`（忽略证书警告）、
  `--host`（连 IP 但按域名做 TLS 校验/SNI）、`--proxy`（走 HTTP 代理）、
  `--ipparam`（pppd 回调 socket 名，用于把 MPPE 密钥传给 sstpc）。
- **MPPE 密钥回传是有状态的一步**：sstp-client 的 pppd 插件在 IP-UP 之后把 MPPE 密钥
  交给 sstpc，sstpc 用这些密钥算 `SSTP_CONNECTED` 消息的哈希；算错服务器立刻断连。
  这也是社区里「连上但几秒后掉线」的常见原因。
  来源：https://manpages.debian.org/testing/sstp-client/sstpc.8
  与 sstp-client-devel 邮件列表讨论。
- 两种用法：命令行直连（仅 MSCHAPv2）/ pppd 插件模式（`pty "sstpc ... --nolaunchpppd"`）。
- Windows 原生客户端：建 VPN 连接时类型选 SSTP，服务器地址填域名（证书 CN 必须匹配），
  或在连接属性里填服务器证书的 SHA256 指纹做绑定。

---

## 五、性能：SSTP 的结构性弱点（必须提前告知用户）

- SSTP = TCP-in-TCP。外层 TCP 丢包会重传/退避，内层 TCP 不知道，继续按原速率发，
  外层缓冲区被填满 → 双方互相等 → 吞吐塌陷（TCP meltdown）。
  OpenVPN 官方 FAQ 明确「Use UDP for the tunnel to avoid this issue」；
  encapsulated.network 的说法更直接：「TCP Meltdown is structural to SSTP and has no
  configuration fix」。
  来源：https://openvpn.net/as-docs/faq-tcp-meltdown/ 、
  https://encapsulated.network/what-is-sstp
- 这不代表 SSTP 不能用：在丢包率低的有线/光纤链路上影响有限；在移动网络、卫星、
  高延迟链路上体验会明显劣化（HN 那位用户说的「长连接变慢，要冷却几分钟」就是这个）。
- 横向性能参考（非 SSTP 专项，但给了 TLS 类 VPN 的位次）：
  Tübingen 的对比实测里 IPsec(AES-NI) > WireGuard > OpenVPN，OpenVPN 即使关掉加密
  吞吐也没有明显改善 → 瓶颈不在加密本身而在用户态数据通路；
  OS3 的 1Gbit/s 实测中 OpenVPN AES-GCM ≈ 922 Mbit/s、WireGuard-C 917 Mbit/s、
  但 OpenVPN AES-CBC 只有 824 Mbit/s、64 字节小包场景全体腰斩到 170-180 Mbit/s。
  SSTP 比 OpenVPN 多一层 PPP + 多一次 TCP 栈拷贝，位次只会更靠后。
  来源：https://publikationen.uni-tuebingen.de/xmlui/bitstream/handle/10900/100430/performance_comparison_of_vpn_solutions.pdf 、
  https://rp.os3.nl/2019-2020/p71/report.pdf

---

## 六、安全与可检测性（这条对我们最要紧）

### 优点

- 服务端证书 + 可选客户端证书（`ssl-ca-file` 开启客户端证书校验）；
- SSTP 控制包带 Crypto Binding（cert hash 参与 Compound MAC），设计上防中间人；
- 认证走 MS-CHAPv2（比 PPTP 时代的裸 MPPE 强，但 MS-CHAPv2 本身已被证明可离线爆破
  ——云厂商镜像也把 PPTP 的 MS-CHAPv2/MPPE 标为 broken，用在 SSTP 里属于"能用但不算强"）。

### 缺点（决定它在中国的定位）

- **协议指纹明确**：`SSTP_DUPLEX_POST /sra_{BA195980-CD49-458b-9E23-C84EE0ADCD75}/` 是标准规定的
  HTTP 请求方法+固定 URL；审查方发一个探测包，真 SSTP 服务器必然按协议回应 → 被确认为 VPN。
  （Habr 文章的实验性描述）
- 因此 SSTP **能骗过"只看端口"的防火墙，骗不过"会主动探测"的审查系统**。
  它的定位是"过企业防火墙/酒店网络"，不是"过 GFW"。
- 学术测量也把 SSTP 列在「Server detection possible」一栏（arXiv 2302.06566 表 1）。

**对 router-re 的含义**：如果目标用户是"出差在酒店/客户现场只能出 443"，
SSTP 是正解；如果目标是"在国内直连境外节点翻墙"，SSTP 不比 OpenVPN-TCP 443 更隐蔽，
反而因为协议指纹更容易被主动探测确认。这点必须写进产品说明，不能含糊。

---

## 七、可借鉴的具体做法（按价值排序）

1. **用 accel-cmd 而不是日志文件取状态**：`accel-cmd show sessions` 直接给 username/ip/type/
   uptime/rx-bytes/tx-bytes，字段和我们现在 OpenVPN status 的
   `common_name/real_address/virtual_address/bytes/connected_since` 几乎一一对应，
   转换层很薄。
2. **管理接口只绑回环**：cloudimg 镜像把 `accel-cmd` 绑 127.0.0.1:2000、telnet 绑 127.0.0.1:2001
   并设口令——照抄这个做法，不要暴露到 0.0.0.0。
3. **证书复用现有 PKI**：我们已经有 `/var/lib/router-re/ovpn/pki`（CA + 服务端 + 客户端证书 + CN 白名单），
   accel-ppp 只要 `ssl-pemfile=<server.crt> ssl-keyfile=<server.key> ssl-ca-file=<ca.crt>`，
   不需要再造一套。
4. **认证先用 chap-secrets**（`/etc/ppp/chap-secrets`，权限 0600），
   由 mgmtd 按用户表生成；等用户量大再上 RADIUS。不要一上来引 RADIUS。
5. **`accept=ssl,proxy`**：如果将来在 443 前面放 nginx/HAProxy 做 SNI 分流，
   用 PROXY protocol 把真实源 IP 传进来，否则所有客户端看起来都来自反代 IP。
6. **hello-interval 保活**：默认 60s 发 Echo-Request，能比较快发现半开连接；
   移动端场景可以适当调低（如 30s），代价是流量和唤醒。
7. **客户端配置模板**：Windows 用「证书指纹绑定」比「让用户导入 CA」体验好得多；
   Linux 用 sstpc 的 `--ca-cert` + `verify` 选项。

---

## 八、结论（一句话版）

SSTP 在 router-re 里的正确角色是：**"只能走 443、且客户端是 Windows 原生"场景下的兜底协议**，
技术上用 accel-pppd 单独跑一个守护进程、复用现有 PKI 与用户表即可，工作量可控；
但它有两条硬限制必须提前说清楚——**TCP-in-TCP 导致高丢包链路吞吐塌陷**，
以及**协议指纹可被主动探测确认（不适合作为对抗 GFW 的手段）**。

参考链接汇总

- MS-SSTP Overview: https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-sstp/70adc1df-c4fe-4b02-8872-f1d8b9ad806a
- accel-ppp [sstp] 配置: https://docs.accel-ppp.org/configuration/sstp.html
- accel-ppp 控制接口: https://docs.accel-ppp.org/guides/control_features.html
- accel-ppp CLI/sessions 列: https://docs.accel-ppp.org/configuration/cli.html
- accel-ppp 源码: https://github.com/accel-ppp/accel-ppp
- Azure 生产镜像指南: https://cloudimg.co.uk/guides/accel-ppp-on-ubuntu-24-04-azure
- accel-ppp 论坛证书问题: https://accel-ppp.org/forum/viewtopic.php?t=2350
- Azure VPN Gateway SSTP 退役: https://learn.microsoft.com/en-us/azure/vpn-gateway/ikev2-openvpn-from-sstp
- VPN 生态测量论文: https://arxiv.org/pdf/2302.06566
- TCP meltdown（OpenVPN 官方）: https://openvpn.net/as-docs/faq-tcp-meltdown/
- SSTP 全面分析（含 meltdown/可检测性）: https://encapsulated.network/what-is-sstp
- 主动探测 SSTP 的具体方法（Habr）: https://habr.com/ru/articles/710980/
- sstpc 手册: https://manpages.debian.org/testing/sstp-client/sstpc.8
- Docker 化 accel-ppp: https://hub.docker.com/r/utoro/accel-ppp
- VPN 性能对比（Tübingen）: https://publikationen.uni-tuebingen.de/xmlui/bitstream/handle/10900/100430/performance_comparison_of_vpn_solutions.pdf
- VPN 性能对比（OS3, 1Gbit/s）: https://rp.os3.nl/2019-2020/p71/report.pdf
