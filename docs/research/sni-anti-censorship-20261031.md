# SNI 反审查研究报告

日期：2026-10-31
方法：USENIX/NDSS/FOCI 论文原文、gfw.report 实验数据、Xray/ sing-box / zapret / GoodbyeDPI
官方文档与 issue、Azure/Cloudflare 官方公告。凡我没有核到的，明确标注"未核实"。

---

## 0. 一句话结论

SNI 反审查不是"一个技巧"，是**一条从"骗过按包匹配的 DPI"到"让服务器根本不像代理"的梯子**。
2026 年的有效排序大致是：

    REALITY（借真站证书） > QUIC SNI 切片（quic-go ≥0.52 默认） > ECH（受制于 DNS）
    > TLS/TCP 分段（只对弱 DPI 有效） > 域前置（已被 CDN 大面积关闭） > 纯"换 SNI"（无效）

对我们（sing-box 路由器 + 境外 VPS 出口）最实际的两条：**REALITY 做主通道，SNI 切片/分段做兜底**。

---

## 1. 审查方到底怎么用 SNI 封锁

### 1.1 TCP/TLS：不是"一个防火墙"，是至少三个中间盒

Paderborn 的《Transport Layer Obscurity: Circumventing SNI Censorship on the TLS-Layer》
（TLS_Obscurity.pdf）通过可控测试向量把 GFW 拆成了三个独立中间盒：

| 中间盒 | 行为 | 解析特点 | 可绕过的点 |
|---|---|---|---|
| MB-1（主） | ClientHello 后注入 RST/ACK，并做约 90 秒残留封锁 | TCP 重组窗口约 10,240 字节；校验 TLS 版本、Extensions Length、SNI Name Length、NameType | 改动这些字段之一即可让它解析失败 |
| MB-2 | 单个不带 ACK 的 RST，无残留封锁 | 只看前 10 字节；分段发生在 512 字节之后就不解析 | 把 ClientHello 切在 512 字节后 |
| MB-New | RST/ACK + 最长 360 秒残留封锁（空路由） | 解析较宽，但只取 SNI 第一项 | 组合技巧才有效 |

关键实验结论（原文）：
- 三个中间盒**同时触发**，但可用不同测试向量单独隔离（例如 wikipedia.org + TLS 版本 0x0304 只触发 MB-1）。
- 「TCP 分段」和「TLS record 分段」是**两种不同技术**，单独用都可能被重组解析；
  zapret issue #238 里作者的实测也是这个结论：`tlsrec` 分段后仍在同一个 TCP 包里，DPI 照样拼起来。
- 只要让**首包不完整**（split 到不同 TCP segment），服务端做完整重组仍能正常握手，DPI 常常匹配不到。

### 1.2 QUIC：2024 年 4 月起按 SNI 封（USENIX Security '25）

Zohaib et al.《Exposing and Circumventing SNI-based QUIC Censorship of the GFW》要点：
- GFW 会**解密 QUIC Initial**（初始密钥任何观察者都能算），拿到里面 TLS ClientHello 的 SNI。
- 命中黑名单后，按三元组（源 IP、目的 IP、目的 UDP 端口）封 **180 秒**。
- 它**不重组**分片的 Initial：ClientHello 跨 CRYPTO frame 切开、Initial 跨 datagram 切开、
  或在前面先发一个非 QUIC 的 UDP 包，都能绕。
- 它有端口启发式：只处理「源端口 > 目的端口」的连接。
- 弱点：解密开销大。研究者用 1200 kpps 打，命中黑名单的连接有 60% 以上没被封——**算力是它的瓶颈**。
- 该机制可被武器化：伪造受害者源 IP 发一个带黑名单 SNI 的 Initial，就能让受害者和目标服务器互相被封 180 秒。

### 1.3 ESNI / ECH：被"围魏救赵"

FOCI 2025《Encrypted Client Hello (ECH) in Censorship Circumvention》实测：
- 640,694 个 Tranco TLS 1.3 服务器中，51.28% 能解析 ECH 扩展，但只有 43% 真能完成 ECH 握手，
  其中几乎全是 Cloudflare（278,040 台）；非 Cloudflare 只有 **6 台**。
- Cloudflare 的 ECH 配置一律用固定外层 SNI `cloudflare-ech.com` → 审查方可直接封这个外层名。
- 俄罗斯 TSPU 自 2024-11 起就这么干：丢弃同时含 ECH 扩展 + 外层 SNI `cloudflare-ech.com` 的 ClientHello。
- 中国/伊朗**没有直接封 ECH 扩展**，而是封**加密 DNS**（DoH/DoT）——拿不到 ECHConfig 就用不了 ECH。
  中国封 `mozilla.cloudflare-dns.com`（TLS 和 QUIC 都封，残留 360s / 180s）。
- 结论原文：「ECH's contribution to censorship circumvention is currently limited」。
- 另一个 2026 的 IFIP 论文称 ECH 已标准化为 RFC 9849（**这个 RFC 号我没有独立核实**），
  并给出 110 万 ECH 部署中 99.99% 归 Cloudflare。
- 可绕法：俄罗斯用「TCP 分段 + TLS record 分段 **组合**」可绕过 ECH 封锁（单独任一无效）。

### 1.4 主动探测：SNI 不是唯一指纹

- REALITY 的经典检测是 **SNI fuzzing**：先用真实 SNI 连一次，再用一个不存在的子域连一次，
  真 CDN 两次行为一致，REALITY 服务器通常露馅（不同的 cipher 顺序/证书回退差异）。
- SSTP 更直白：发 `SSTP_DUPLEX_POST /sra_{BA195980-...}/` 就自报身份（见 SSTP 研究报告）。
- Shadowsocks 那条路线（2023 USENIX）说明：全加密流量本身也会被"熵/可打印字符比例"被动识别。

---

## 2. 反审查技术谱系（按"对抗强度"排序）

### 层级 1：让 DPI 解析不了首包（低成本，快失效）

**TCP 分段 / 分片**（zapret、GoodbyeDPI、tpws）
- zapret：`--dpi-desync=multisplit|multidisorder|fakedsplit`，`--dpi-desync-split-pos`
  指定切点（如 `--dpi-desync-split-tls=sni`，或 `split-pos=1`）。
- GoodbyeDPI：`-e N` HTTPS 分段、`--frag-by-sni`（在 SNI 值前切）、`--native-frag`、
  `--reverse-frag`（反序发送，对付"必须先收全再匹配"的 DPI）。
- 假包（fake）：`--dpi-desync-fooling=badsum|md5sig|hopbyhop`，把带敏感 SNI 的假包发给 DPI，
  但 TTL/校验和让它到不了服务器；`--dpi-desync-fake-tls=...bin` 可放一个任意指纹的 ClientHello。
- 实测限制（原文档自己写的）：超过 8 段容易出问题；Linux 有发送缓冲会把片段合并，
  需要 `TCP_NODELAY` + 多次 send；**TLS record 分段 ≠ TCP 分段**，前者常被重组。
- GoodbyeDPI 2024 的 commit 说沙特/UAE 的 DPI 开始"在会话任意位置搜 SNI 字节"，
  于是改成"扩展头结束、SNI 值开始前"切——说明这是一场持续对抗，不是一劳永逸。

**QUIC SNI 切片**（这是目前性价比最高的一招）
- quic-go v0.52.0（2025-05-23）默认把 ClientHello 跨 CRYPTO frame 切开，
  使单个 frame 不含完整 SNI → GFW 不重组，直接漏过。
- 已经默认跟进：sing-box 1.12、Xray 25.6.8、Hysteria 2.6.2（据 radiosilence/sbr issue #9 与
  USENIX 论文 Table 7）。
- Firefox 137 也默认开了（`network.http.http3.sni-slicing`）。
- 这意味着：**如果你的 sing-box 是 1.12+，Hysteria2/TUIC 走 QUIC 时 SNI 已经是切片的**，
  不需要额外配置。但要注意：Salamander 混淆会把流量变成随机 UDP，某些网络直接丢；纯 QUIC 切片更稳。

### 层级 2：换个"合法的 SNI"（域前置 / CDN 借用）

**域前置（Domain Fronting）**
- 原理：TLS SNI 填合法大站（如 cdn.azureedge.net），HTTP Host 填真实目标，CDN 按 Host 路由。
- 现状：**基本已死**。微软 2024-01-08 起在 Azure Front Door / Azure CDN 阻断 HTTP Host 与 SNI 不匹配的请求；
  Cloudflare 早就禁止；ATT&CK 专门加了 T1090.004 的检测策略（SNI 与 Host 不一致）。
- 只在自建 CDN 或未加固的边缘上还有残余价值，不适合做产品依赖。

### 层级 3：让 SNI 根本不存在 / 不存在真实域名

**ECH**
- 依赖 DNS 拿 ECHConfig → 中国封 DoH/DoT，等于掐断前置条件。
- 唯一大规模部署方是 Cloudflare，且外层 SNI 固定 → 俄罗斯直接封外层名。
- 适合"对象是国内访问 Cloudflare 站点"的隐私场景，**不适合当自建代理的主力穿墙手段**。

**REALITY（Xray / sing-box）**
- 原理：不自己签证书，而是**借用真实站点的证书与握手**。客户端用真实站点的 SNI（如 www.microsoft.com），
  REALITY 服务端在握手过程中注入预共享密钥（x25519），合法客户端据此完成握手并在其上开隧道；
  非授权探测者看到的是那个真站点的证书与行为。
- 官方文档要求（xtls.github.io `transports/reality`）：
  - `dest`/`target`：被借用的站点，必须支持 TLS 1.3 + HTTP/2，且国内可达；
  - `serverNames`：必须是目标证书 SAN 里的名字，**不支持通配符**；可含空串（接受无 SNI）；
  - `privateKey`/`publicKey`：`xray x25519` 生成；
  - `shortIds`：客户端身份；
  - `fingerprint`：必填，REALITY 依赖 uTLS 改底层 TLS 参数，不能用 `unsafe`；
  - 可选 `mldsa65Seed`/`mldsa65Verify`（后量子签名，要求目标证书 > 3500 字节，否则本身成指纹）。
- 真实世界的坑（Xray issue #2778，伊朗 MCI）：
  两台配置完全相同的 VPS，一个小流量（5-7 人）、一个 200+ 人高流量，
  **高流量那台的 IP 被封了**。说明 REALITY 的抗探测 ≠ 抗流量特征/抗 IP 封禁；
  共享人数多、瞬时流量大，仍会被盯上。
- 检测面：SNI fuzzing（改子域再连）能看出行为差异；所以 `serverNames` 必须与目标 SAN 严格一致，
  不要自己编子域。

### 层级 4：连"像什么"都不追求（全加密 + 流量整形）

- Shadowsocks-2022 / VMess+WS+TLS 之类，在 2023 年后的被动检测面前需要额外整形（padding/切片）。
- 这类方案的价值是"没有固定协议指纹"，代价是被"熵检测"盯上时要靠流量形态对抗。

---

## 3. 各方案有效性对照（基于本文引用的实测）

| 手段 | 对抗"按 SNI 黑名单 RST" | 对抗"主动探测" | 对抗"流量特征/熵检测" | 部署成本 | 备注 |
|---|---|---|---|---|---|
| TCP/TLS 分段（zapret/GoodbyeDPI） | 有效（弱 DPI） | 无效 | 无效 | 极低（路由器上就能跑） | 只改首包形态，端口/证书照旧 |
| QUIC SNI 切片 | 有效（GFW 不重组） | 不适用 | 部分 | 零（升级库即可） | quic-go 0.52+ / sing-box 1.12+ |
| 域前置 | 有效 | 弱 | 弱 | 中 | 主流 CDN 已关闭，2024 起不可依赖 |
| ECH | 间接（封 DNS） | 弱 | 中 | 高（要 CF） | 中国封 DoH/DoT 使其不可用 |
| REALITY | 有效（SNI 是合法站） | 较强（探测看到真站） | 中（靠 IP 信誉） | 中 | 高流量/多人共享仍会被封 IP |
| 纯"换 SNI 成允许域名" | 无效 | 无效 | 无效 | 低 | 证书不匹配，TLS 直接失败 |

---

## 4. 落到我们现有环境（sing-box 110 / gateway 17 / vgpool 18）

### 4.1 现状可做的三件事（按优先级）

1. **主通道改 REALITY**
   - 服务端：hk87 / jp48 上跑 Xray 或 sing-box 的 REALITY inbound，
     `target` 选一个「TLS1.3 + HTTP/2 + 国内可达 + 证书 SAN 明确」的站（如 www.microsoft.com:443），
     `serverNames` 严格等于其 SAN，`shortIds` 每人一个。
   - 客户端：路由器 110 的 sing-box（1.14）支持 REALITY，直接 `type: vless` + `tls.reality`。
   - 关键纪律：**一个 REALITY 入口不要超过几十个共享用户**（伊朗案例的教训），
     高流量入口单独拆 IP。

2. **QUIC 路线确认版本**
   - `sing-box version` ≥ 1.12 → Hysteria2/TUIC 已默认 SNI 切片；
   - 低于此版本先升级，否则在 GFW 面前等于把 SNI 明文递过去。

3. **兜底加一层分段**
   - 110 上若还留着 OpenVPN/SSTP 这类"SNI 是自家域名"的入口，
     在路由器侧跑 zapret 的 `nfqws`，对 443 出向做 `--dpi-desync=fake,split2 --dpi-desync-split-tls=sni`。
   - 只对"按包匹配"的 DPI 有效，别指望它对抗 QUIC SNI 封（QUIC 要切片，不是分段）。

### 4.2 不建议投入的

- 域前置：Azure 2024 已封，Cloudflare 早封，ATT&CK 有检测规则，产品化没有前途。
- ECH 作为主力：依赖 DoH，国内 DoH 被封；只在"访问 Cloudflare 站点"的隐私场景有意义。
- 纯换 SNI（证书与 SNI 不符）：TLS 握手直接失败，等于没做。

### 4.3 检测对抗的运维纪律（比技术选型更重要）

- SNI fuzzing 探测是 REALITY 的主要识别手段 → serverNames 不要自造子域，和目标 SAN 严格一致。
- 高流量 = 高风险（伊朗 200 人入口被封 IP）→ 入口分散、限额、异常流量告警。
- 别在同一 IP 上同时开"真站点服务 + 代理"，一旦被探测对比就露馅。
- 保留可切换的备入口（不同 ASN/不同证书目标），REALITY 被封时能换 `target` 而不是换协议。

---

## 5. 与我们 SSTP 研究的关系

- SSTP 的 SNI 是**自己的域名**，一旦域名进黑名单，443 上的 SSTP 直接 RST，
  且 `SSTP_DUPLEX_POST` 主动探测能确认身份 → **在 SNI 反审查这条线上，SSTP 是最弱的一档**。
- REALITY 恰恰相反：SNI 用别人的合法域名，探测看到的是真站 → 这才是"SNI 反审查"的正解。
- 结论：如果目标包含"穿越审查"，SSTP 只能作为"酒店/企业网只能出 443"的兼容入口，
  **不要指望它承担反审查职责**。

---

## 6. 需要进一步实测的点（我还没做）

1. 在 110 上跑 `sing-box version` 确认是否 ≥1.12（决定 QUIC 切片是否默认生效）。
2. 在 hk87/jp48 上挑 target 并做 `xray tls ping`，确认目标 TLS1.3/H2/证书长度/是否拒绝无 SNI。
3. 用 `openssl s_client -servername <fake>` 对比 REALITY 服务端与目标站的行为差异（自查是否会被 SNI fuzzing 识别）。
4. 若要在路由器上做分段，先在 110 的 nftables 里做一条最小规则，只对特定目标 IP 生效，避免全局误伤。

---

## 参考

- TLS_Obscurity（Paderborn，GFW 三中间盒）: https://ris.uni-paderborn.de/download/59824/59826/TLS_Obscurity.pdf
- USENIX Sec '25 QUIC SNI 审查: https://gfw.report/publications/usenixsecurity25/en/ ；PDF: https://usenix.org/system/files/usenixsecurity25-zohaib.pdf
- FOCI 2025 ECH 实测: https://petsymposium.org/foci/2025/foci-2025-0016.pdf
- IFIP Networking 2026 ECH 生态（RFC 9849 之说）: https://dl.ifip.org/db/conf/networking/networking2026/1571261276.pdf
- Xray REALITY 文档: https://xtls.github.io/en/config/transports/reality.html
- REALITY 被伊朗封 IP 的实测 issue: https://github.com/XTLS/Xray-core/issues/2778
- zapret 文档（分段/假包）: https://github.com/bol-van/zapret/blob/master/docs/readme.en.md
- zapret issue #238（TLS record vs TCP 分段）: https://github.com/bol-van/zapret/issues/238
- GoodbyeDPI（含 --frag-by-sni 与沙特/UAE 案例）: https://github.com/ValdikSS/GoodbyeDPI
- quic-go SNI slicing 与工具跟进: https://github.com/radiosilence/sbr/issues/9
- Azure 域前置关闭公告: https://learn.microsoft.com/en-us/answers/questions/1421101/
- ATT&CK T1090.004 检测策略: https://attack.mitre.org/detectionstrategies/DET0196/
- GFW 全加密流量检测（2023）: https://gfw.report/publications/usenixsecurity23/zh/
