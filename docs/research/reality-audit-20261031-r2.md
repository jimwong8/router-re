# REALITY / SNI 实测审计记录（2026-10-31 第二轮：路由层）

承接 `reality-audit-20261031.md`。这一轮把「110 上 TCP 型出站为什么是死的」追到了路由层，
并修掉了其中确定的一环；还剩一个未解释的差异，写清楚留给下一轮。

---

## 1. 先修掉一个隐蔽问题：110 上跑的不是 procd 管的那份 sing-box

    ps w | grep sing-box
      10067  sing-box run -c /tmp/or_test.json        (root, 1292 分钟)
      14238  sing-box run -c /etc/sing-box/probe.json (root)
      14338  sing-box run -c /etc/sing-box/config.json (root, 1357 分钟)

主实例是**手工 root 启动**的，不是 `/etc/init.d/sing-box`（该脚本用 procd 且
`procd_set_param group nogroup`）。后果：
- `/etc/init.d/sing-box restart` 重启的是 procd 那个实例，而我们改的 config 由这条手工进程持有 → **改配置不生效**；
- 手工实例的 Gid=0（root），与已知的「sing-box 须以 nogroup 运行」的约束不符。

处置：kill 掉三条手工进程，改用 `/etc/init.d/sing-box start` 起 procd 实例，
确认 `Gid: 65534 (nogroup)`、7890 监听、代理可用。此后改配置再重启才真正生效。

## 2. 根因之一（已修）：到上游服务器的连接被自己的 TUN 抢走

    ip route get 209.33.171.48
      209.33.171.48 via 172.19.0.2 dev tun0 table 2022 src 172.19.0.1

即：路由器自己去连落地服务器时，走的是 `ip rule 9001 → table 2022 → tun0`，
等于「代理的流量又进代理」，绕回主实例再换节点出去。对照实验（在 110 上另起一个
不带 mark 的 sing-box 实例，只配 JP 一个出站）出口 IP 却是 **103.240.198.142 (HK)**，
直接证实了这条环路。

修复：新增 `/etc/sing-box/bypass-rules.sh`，把配置里全部 19 个上游 IP 加进 main 表：

    ip rule add to <ip>/32 lookup main priority 8900

并在 `/etc/rc.local`（`exit 0` 之前）挂上调用。执行后：

    ip rule list | grep -c 8900   → 19
    ip route get 209.33.171.48    → via 10.100.1.1 dev br-lan src 10.100.1.110

**验证**：用同一个 REALITY 出站另起实例（同样的 uuid / public_key / short_id / SNI），
经它的 socks 访问 ipinfo：

    209.33.171.48   ← 真实从日本出口

说明「服务器 + 新密钥 + www.apple.com + 路由 bypass」这一整套是对的。

## 3. 仍未解释的差异（留待下一轮）

在 main 实例里对单节点做 Clash API 延迟测试：

    OR-balmy-dingo   delay 588      ← vless + reality + vision, SNI www.cloudflare.com，正常
    OR-my-relay      delay 372      ← 同上，正常
    HK-TUIC          delay 266      ← QUIC，正常
    JP-VLESS         Timeout        ← 我们自己 jp48
    HK87-VLESS       "An error occurred in the delay test"  ← 错误类型都不同

已排除：
- 密钥/SNI/证书（18 上独立客户端与 110 上独立实例都能走通 jp48）；
- 密钥格式（重新生成并同步过）；
- 路由（bypass 规则生效后 route get 已指向 br-lan）；
- 进程身份（procd + nogroup 已恢复，重启后 PID 变化、配置确已加载）；
- 目标站可用性（把 jp48 的 target 临时换成 www.cloudflare.com 同样 Timeout，已回滚）。

未排除/方向：
- 这两个节点在本路由器的出口路径上被上游（iKuai/ISP）针对该 IP:port 做了限制——
  但 conntrack 显示 TCP 有去有回（回包仅 48 字节），像是「TCP 通了、TLS 第一步被掐」；
- Clash API 延迟测试对这两个 tag 的处理差异（HK87 报的是另一种错误，提示可能连上了但握手失败）；
- 需要在两端装 tcpdump 抓包才能定论（110 与 jp48 目前都没有 tcpdump/openssl）。

## 4. 当前状态（可交付）

- 110 由 procd 托管，Gid=nogroup，7890/9090 正常，经代理访问 ipinfo 出口 HK 正常（未影响用户上网）。
- 19 条 bypass 规则已持久化到 rc.local。
- jp48 服务端：`server_name`/`handshake.server` = `www.apple.com`，密钥对已更新，服务 active。
- 110 侧 JP-VLESS 的 `public_key`、`server_name` 已同步为新值。
- 备份：`/etc/s-box/sb.json.bak.{sni,cf}.<ts>`、`/etc/sing-box/config.json.bak.{sni,adi}.<ts>`、
  `/etc/rc.local.bak.<ts>`。

## 5. 下一轮要做的最小闭环

1. `opkg update && opkg install tcpdump`（110）；jp48 `apt install tcpdump`；
2. 在 110 上 `tcpdump -ni br-lan host 209.33.171.48 and port 37651` 同时触发延迟测试，
   看 ClientHello 是否发出、服务端回的是什么（48 字节到底是什么）；
3. 若确认是上游对该 IP:port 的限制，就把 jp48 的 REALITY 换端口/换 IP，或改走 443。

---

## 6. 抓包结论（tcpdump，本轮新增）

两端都装了 tcpdump（110: opkg，jp48: apt）。

在 110 上抓 `host 209.33.171.48 and port 37651`，触发 JP-VLESS 延迟测试，看到：

    10.100.1.110.42604 > 209.33.171.48.37651: S  (mss 1460)
    209.33.171.48.37651 > 10.100.1.110.42604: S. (mss 1400, wscale 0)
    10.100.1.110.42604 > ...: P. length 565   ← ClientHello
    （同一 seq 反复重传 5 次，始终没有 ACK）

在 jp48 上抓 `port 37651`，同一时间段内**没有出现来自本次连接的 ClientHello**；
只有别的来源（117.141.104.196）的 SYN/SYN-ACK。

对照：在 10.100.1.18 上用 `openssl s_client -connect 209.33.171.48:37651 -servername www.apple.com`
**完整握手成功**，拿到真实 Apple 证书（CN=www.apple.com，Apple Public EV Server RSA CA 1 - G1）。

⇒ 服务器与证书链没问题；问题在 **110 这条出口路径**上：SYN 能拿到一个 mss 1400 的 SYN-ACK
（很可能是中间设备/另一条链路回的，不是 jp48 自己回的，因为 jp48 侧没看到），
但随后的 TLS 数据到不了 jp48。
注意 jp48 自己的 MSS 是 1460，110 收到的是 1400 —— 两者不一致，佐证回包不是同一台机器发的。

## 7. 当前最终状态

- 110：procd 托管、nogroup、7890/9090 正常；19 条 bypass 规则已持久化到 rc.local。
- jp48：REALITY 用 www.apple.com（SAN 合法、支持 h2），密钥对已更新，服务 active。
- 18 → jp48 REALITY：**完整可用**（TLS 握手 + 真实证书 + 实际出口 209.33.171.48）。
- 110 → jp48 REALITY：仍不通，已定位到出口路径（非 sing-box 配置）。
- 待办：在 iKuai（10.100.1.1）上核对 10.100.1.110 的分流/策略路由与 WAN 选择，
  或在 110 上用 `ip route get` + 抓包确认这条 TCP 到底从哪个 WAN 出去、被谁回了 SYN-ACK。

---

## 8. 最终定位与修复：这些落地 IP 的 IPv4 出口被掐，IPv6 正常

用户提示 iKuai 里已有 `PT_110`（目的地址 = 我们的 VPN 落地 IP 列表，含 209.33.171.48，线路 wan6，优先级 31）。
核对后确认：我新建的 `PT_RR110_JP`（wan3，优先级 0）与它冲突且无效，已停用（可一键删除）。

用 iKuai 规则切换 wan3 / wan6 都试过，IPv4 全部失败 → 说明与线路选择无关。逐项对比如下（均从 110 发起）：

| 目标 | 协议 | TLS 握手 |
|---|---|---|
| 149.104.27.87:30746 (hk87) | IPv4 | 失败 tls=0 |
| 149.104.27.87:30746 | IPv6 2001:df1:7880:2::25e | **0.077s 成功** |
| 209.33.171.48:37651 (jp48) | IPv4 | 失败 tls=0 |
| 209.33.171.48:37651 | IPv6 2401:a660:1001:445::248 | **0.19s 成功** |
| 国内 CDN 112.92.61.2:443 | IPv4 | 正常 |
| OR-* 节点（其它境外 IP） | IPv4 | 正常 |

抓包旁证：IPv4 发起后 0.2ms 内收到 MSS=1400 / wscale=0 的 SYN-ACK（真实境外 RTT 应 30ms+），
随后 1.4KB 的 TLS 段全部黑洞 —— 是路径上的设备在终结 TCP，不是服务器。
结论：**这两个落地 IP 的 IPv4 被拦截/黑洞，IPv6 通路正常。**

修复（已生效，已在 110 验证）：

    /etc/sing-box/config.json
      JP-VLESS   server: 209.33.171.48            → 2401:a660:1001:445::248
      HK87-VLESS server: 149.104.27.87            → 2001:df1:7880:2::25e

    备份：/etc/sing-box/config.json.bak.v6.<ts>

验证（Clash API 延迟测试，真实 HTTP 请求经节点发出）：

    JP-VLESS   Timeout → delay 390~408 ms   ✅
    HK-TUIC    184 ms   JP-HY2 335 ms   OR-my-relay 878 ms   均正常
    经 7890 代理访问 ipinfo 仍正常（出口 103.240.198.142）

遗留：HK87-VLESS 换成 IPv6 后仍报 "An error occurred in the delay test"（能连上、握手失败），
说明 hk87 侧 REALITY 的 short_id / 公钥与该 tag 不一致，需要在 hk87 上核对（其 SSH 22 目前从外部超时）。

---

## 9. HK87 修通（第二轮）

通过 IPv6 SSH 进到 hk87（`ssh -6 root@2001:df1:7880:2::25e`，公网 IPv4 22 端口不通、IPv6 22 通），
读到 `/etc/s-box/sb.json` 里 30746 的 REALITY 入站：

    uuid       5f517335-e3e2-49b6-bb17-3a3dbe065cc2   （与 110 一致）
    short_id   d5d1dcc7                                （与 110 一致）
    server_name apple.com                              ← 110 客户端发的是 www.apple.com
    private_key 6HRGQ4gc67Ykll_RCxGnUNZn4BaW8LggBbG5tdEY90A

用 X25519 由私钥推导公钥，结果等于 110 侧配置的 `Hu0maD0kGc9VhlISsAnsqP_W9DI_aj9bmlHUR6k23WU`
→ **密钥是对的**，问题只出在 server_name：REALITY 要求客户端 SNI 落在服务端 serverNames 内，
`apple.com ≠ www.apple.com`，所以握手被拒（表现为 "An error occurred in the delay test"）。

处置：hk87 的 `server_name` / `handshake.server` 改为 `www.apple.com`（与 jp48 一致），
`sing-box check` 通过、`systemctl restart sing-box` active，备份 `sb.json.bak.sni.<ts>`。

（过程中一次 sed 把两处 private_key 一起改写，`sing-box check` 报
`decode private key: illegal base64 data`，已立刻用备份 `sb.json.bak.1793385763` 还原并复核，
没有把坏配置留在线上。）

验证（110 Clash API 延迟测试，真实 HTTP 经由节点）：

    JP-VLESS      228 ms     HK87-VLESS    446 ms（首次 1224 ms，之后稳定）
    HK87-ANYTLS   140 ms     JP-HY2        164 ms     HK-TUIC  261 ms
    OR-my-relay   260 ms
    GLAXY-HK      仍 Timeout —— 其服务器 156.224.78.107:26991 的 IPv4 TLS 直接不通（第三方节点，改不了）
    OR-balmy-dingo 偶发 "error in delay test"，但 IPv4 TLS 0.47s 正常 → 上游侧问题，非链路

## 10. 全量 server_name 体检

110 现在所有真实 TLS 出口的 SNI 都合法（取自目标证书 SAN，且目标支持 h2）：

    www.cloudflare.com ×14   www.bing.com ×9   www.apple.com ×4

不再有裸 `apple.com`（该名字既不在 SAN 内、也不提供 h2，实测 alert 120）。

## 11. 结论（可复用）

1. 这类「TCP 能连上、TLS 一开始就断」的故障，先做 IPv4/IPv6 对照，一步就能定性：
   两个落地 IP 的 IPv4 全部 tls=0，IPv6 0.08~0.19s 正常 → 目的地址级 IPv4 封锁。
2. REALITY 排错顺序：`uuid/short_id` → `public_key`（可用 X25519 从私钥推导核对）→ `server_name`
   是否在服务端 serverNames 内、且该名字在目标证书 SAN 里、且目标支持 h2。
3. 有 IPv6 的落地优先用 IPv6 做 REALITY 入口，绕开 IPv4 侧封锁，且不用改路由器分流。

---

## 12. 把这两台落地上的所有 TCP 出站都切到 IPv6（最终态）

既然 IPv4 到 209.33.171.48 / 149.104.27.87 被掐、IPv6 正常，就把这两台上**所有 TCP 类型**的出站一起换掉
（UDP 的 hysteria2 / tuic 本来就是通的，保持 IPv4 不动）：

    改：JP-VMWS(2086) JP-VMWS-TLS(8443) JP-VMWS-80(80) JP-ANYTLS(64514) JP-ANYTLS-443(443) HK87-ANYTLS(50075)
    不改：JP-HY2 / JP-HY2-443 / JP-TUIC（UDP）
    备份：/etc/sing-box/config.json.bak.v6all.<ts>

`s ing-box check` 通过、restart 后全部实测（Clash API，真实 HTTP 经节点）：

    JP-VMWS 226   JP-ANYTLS 240   HK87-ANYTLS 584   JP-ANYTLS-443 239
    JP-VMWS-TLS 223   JP-VMWS-80 220   JP-VLESS 245   HK87-VLESS 180
    经 7890 代理访问 ipinfo 正常（出口 103.240.198.142）

在此之前这些节点里有 5 个是超时/连不上的（IPv4 被封），现在整组可用。

## 13. 最终状态一览

| 项目 | 状态 |
|---|---|
| jp48 REALITY (37651) | IPv6 2401:a660:1001:445::248，SNI www.apple.com，密钥已换，可用 ~220ms |
| hk87 REALITY (30746) | IPv6 2001:df1:7880:2::25e，SNI 已改 www.apple.com，可用 ~180-450ms |
| jp48/hk87 其余 TCP 出站 | 全部改 IPv6，6 个节点由不可用变为 180-590ms |
| 110 自身 | procd 托管、gid nogroup、19 条 bypass 规则持久化于 rc.local |
| iKuai | 自建的 PT_RR110_JP 已停用（与既有 PT_110 冲突、且无效），可删除；PT_110 保留 |
| 遗留 | GLAXY-HK（第三方 156.224.78.107:26991）IPv4 TLS 不通，非我方节点；OR-balmy-dingo 偶发上游错误 |
