# REALITY / SNI 实测审计记录（2026-10-31）

对象：路由器 10.100.1.110（sing-box 1.14.0）上的 19 条 REALITY 出站 + 两台落地服务器
（jp48 209.33.171.48:37651、hk87 149.104.27.87:30746）。
方法：openssl s_client 直探（真实 SNI / 伪造 SNI 对照）、Clash API 延迟测试、
服务器端 journal、以及一台独立 glibc 客户端（sing-box 1.14.0，跑在 10.100.1.18）。

---

## 发现 1：客户端 SNI 指向的目标站不支持该名字的 HTTP/2

`openssl s_client -connect 209.33.171.48:37651 -servername apple.com -alpn h2`：

    error:0A000460:SSL routines:ssl3_read_bytes:tlsv1 alert no application protocol (alert 120)
    no peer certificate available

直连 `apple.com:443` 用同一个 SNI 得到完全相同的 alert，而目标站证书的 SAN 只有：

    DNS:www.apple.com.cn
    DNS:images.apple.com
    DNS:www.apple.com

即 **`apple.com`（裸域）既不在 SAN 里，也不提供 h2**。Xray 文档要求 `serverNames` 取自目标
证书 SAN、且目标要支持 TLS1.3+HTTP/2，所以原来的 `apple.com` 属于配置违规。

已改（两侧同步）：
- jp48 `/etc/s-box/sb.json`：`tls.server_name` 与 `reality.handshake.server` → `www.apple.com`
- 110 `/etc/sing-box/config.json`：`JP-VLESS` / `HK87-VLESS` 的 `server_name` → `www.apple.com`

备份：`/etc/s-box/sb.json.bak.sni.<ts>`、`/etc/sing-box/config.json.bak.sni.<ts>`。

---

## 发现 2：jp48 的 REALITY 密钥对与客户端不匹配

服务器日志（改动前）反复出现：

    inbound/vless[vless-sb]: process connection from 100.111.28.64:xxxxx:
        TLS handshake: REALITY: processed invalid connection

客户端 110 的 `public_key` 与服务器 `private_key` 不是同一对。已用
`sing-box generate reality-keypair` 重新生成并同步两侧：

    private: eKfM6HXQc9u48sYs-OO16cIdfec8da_IvMDud6u-WVY   (jp48)
    public : ghVpFqeTm5CAtgDRYRicf2_VHJtnUKjAHJZRSjuZm0s   (110 JP-VLESS)

**端到端验证（18 上独立客户端，不经过路由器）**：

    直接连 jp48 REALITY → curl https://ipinfo.io/ip
    输出 209.33.171.48   ← 真实走通，说明服务端配置 + 密钥 + SNI 三者已正确

---

## 发现 3：SNI fuzzing 自检——我们的 REALITY 在 SNI 层面不可区分

对同一台服务器分别用「正确 SNI」和「编造子域」探测，并与真实目标站对照：

| 探测点 | SNI | 证书 SHA256 前 17 位 | ALPN |
|---|---|---|---|
| jp48:37651 | www.apple.com | CD:56:3F:57:16:C2 | h2 |
| jp48:37651 | notareal-zzz9.apple.com | CD:56:3F:57:16:C2 | （空） |
| www.apple.com:443 | notareal-zzz9.apple.com | CD:56:3F:57:16:C2 | （空） |

三行证书指纹完全一致，行为也一致 → 按「真 SNI vs 编造子域」的经典 SNI fuzzing 手法，
探测方看到的就是一台 Apple 服务器，**没有暴露出 REALITY 特征**。

（注意：这只覆盖"认证前"的探测面。认证后的流量特征、IP 信誉、共享人数仍是对抗重点，
伊朗 200 人共享入口被封 IP 的案例说明了这一点。）

---

## 发现 4（未修复，路由层）：110 上所有 VLESS/REALITY 出站都超时

Clash API 延迟测试（控制项与实验项混跑）：

    HK-TUIC      {"delay": 113}      ← QUIC 出站正常
    JP-HY2       {"delay": 462}      ← QUIC 出站正常
    JP-VLESS     {"message":"Timeout"}
    HK87-VLESS   {"message":"Timeout"}
    GLAXY-HK     {"message":"Timeout"}

同时抓 jp48 日志：测试期间**没有一条 REALITY 入站连接**，说明流量根本没离开路由器。

路由证据（110 上）：

    ip route get 209.33.171.48
      209.33.171.48 via 172.19.0.2 dev tun0 table 2022 src 172.19.0.1

以及一个对照实验：在 110 上另起一个未打 mark 的 sing-box 客户端实例（连同一台 jp48），
curl 出口 IP 得到的是 **103.240.198.142（HK）** 而不是 JP —— 说明它的出站被自己的
TUN 抓走、绕回主实例又从别的节点出去了。即：**路由器自身 TCP 出站没有带上
fwmark 365 的旁路标记**，被 `ip rule 9001 lookup 2022` 送进 tun0。

影响：110 上所有基于 TCP 的出站（VLESS/REALITY、Trojan 等）实际不可用；
QUIC（Hysteria2/TUIC）不受影响，这解释了为什么日常上网"看起来正常"。

修复方向（尚未执行，需单独窗口做）：
1. 确认 `route.default_mark: 365` 是否真的作用于出站 socket（抓包看 mark 位）；
2. 若 sing-box 未打标，加 nft 规则在 output 链最前面放行 `meta mark 365`，
   或给这些服务器 IP 加 `ip rule ... lookup main`（本次临时加过，未被采纳，已删除）；
3. 修完后重跑 Clash API 延迟测试，要求 VLESS 项返回 delay 而非 Timeout。

---

## 变更清单（可回滚）

| 位置 | 文件 | 改动 | 备份 |
|---|---|---|---|
| jp48 | /etc/s-box/sb.json | server_name apple.com→www.apple.com；handshake.server 同；private_key 换新 | sb.json.bak.sni.<ts> |
| 110 | /etc/sing-box/config.json | JP-VLESS/HK87-VLESS server_name→www.apple.com；JP-VLESS public_key 换新 | config.json.bak.sni.<ts> |

验证配置合法：两侧均 `sing-box check` 通过；jp48 `systemctl restart sing-box` 后 active；
110 `/etc/init.d/sing-box restart` 后 7890/9090 正常，经代理访问 ipinfo 仍可用（当前出口 HK）。
