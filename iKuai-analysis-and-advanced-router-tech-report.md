# iKuai 优缺点深度分析 + 先进路由技术参考报告

> 目标：模仿 iKuai 重新做一个路由系统。本报告先做「竞品解剖 + 技术对标」，为后续架构设计打底。
> 依据：① iKuai 4.0.314 固件第一手逆向成果（双层加密、云管理协议、Web 认证、授权码、内核模块均已还原）；② 全网深度检索（MikroTik RouterOS / OpenWrt / VyOS / pfSense / FRRouting / eBPF-XDP-DPDK-VPP / SD-WAN / SASE）。
> 日期：2026-10-27

---

## 一、iKuai 技术架构剖析（基于逆向第一手成果）

### 1.1 系统分层
| 层 | 实现 | 说明 |
|---|---|---|
| 内核 | Linux 5.10.194（hpxiong@ikuai8 定制） | 私有补丁 `initrd_length`；rootfs 双层加密保护 |
| 内核模块 | `ik_core.ko`（1.2MB，not stripped，1797 符号） | 授权码解密、SD-WAN bypass、URL 过滤、认证 |
| Web 服务 | openresty（nginx+lua） | 管理后台，80/90/443 对外 |
| 管理逻辑 | `webman.lua`（47KB）+ `ikrest.lua` | 认证、路由、权限、REST 分发 |
| REST 后端 | `func_api.sh`（shell） | **把 func_name/action 拼成 shell 命令执行** |
| 守护进程 | `pmd`（包管理）、`ik_rc_client`（云管理） | 各自独立进程 |
| 配置存储 | `/etc/config` + config_db（SQL 初始化脚本） | 键值 + 数据库混合 |

### 1.2 关键机制（逆向还原）
1. **rootfs 双层加密**：第一层 SHA256 HKDF 链（5 个 context label）+ 3 轮置换流密码；第二层 map*.enc 用 AES-256-CBC + 硬编码口令 `bcfae016-ea75-4569-b9e6`
2. **授权码**：AES-128-CBC，密钥 `MD5(g_ik_release_info 产品/版本字段)`
3. **云管理**：`ik_rc_client` → `librc_core.so`，protobuf 消息（auth_req: gwid/token），AES-256-CBC + EVP_BytesToKey(sha1) 加密会话，mTLS 连 as-v4.ikuai8.com:9444
4. **Web 认证**：PBKDF2-SHA256（100k 迭代）验证密码 + MD5 sess_key cookie

---

## 二、iKuai 优点

1. **中文友好、上手门槛低**：全中文 Web 界面，小白可用（对标 MikroTik 全英文 + WinBox 学习曲线）
2. **DPI 七层流控精准**：应用级识别，非简单限速（官方明确「七层流控更耗 CPU 但更精准」）
3. **功能全家桶**：多线负载、协议分流、AC 无线管理、十余种认证（PPPoE/Web/RADIUS/微信等）、行为管理、审计、VPN（IPsec/OpenVPN/WireGuard/PPTP/L2TP）
4. **免费 + 软路由生态**：永久免费 iKuaiOS，x86 平台，被调侃「路由的尽头是爱快」
5. **云管理**：爱快云 + 微信小程序扫码绑定，远程管理
6. **支持虚拟机**：可虚拟 OpenWrt 补插件，弥补闭源生态不足
7. **内置 AC 管理**：AP 集中管理、快速漫游、国家码/频宽设置

---

## 三、iKuai 缺点（重点：架构 + 安全）

### 3.1 架构性缺陷
1. **REST 后端用 shell 分发**（`func_api.sh`）：每个 API 调用都 `shell.run("/usr/ikuai/function/<func_name> <action> ...")`——性能差（每请求 fork 进程）、且是命令注入温床
2. **单体无平面分离**：管理面/控制面/数据面全混在 Linux 内核 + shell + lua，没有现代路由器「控制面/数据面解耦」的设计
3. **陈旧组件**：内核 5.10、OpenSSL 1.0.0（已停止维护）

### 3.2 安全缺陷（逆向发现，且被公开 CVE 印证）
| 缺陷 | 证据 | 公开佐证 |
|---|---|---|
| 默认弱密码 | admin/admin（MD5=21232f29...） | 官方下载页明示 |
| 命令注入 | func_api.sh 无过滤拼接；ikrest.lua /command/file | **CVE-2023-34849**（ActionLogin 未授权命令注入）、**CVE-2022-40469**（认证后 RCE） |
| SQL 注入历史 | telnet 登录验证脚本未过滤 | CNNVD-201608-553/554/555 |
| 硬编码密钥 ×6 | 备份/升级/SD-WAN 密钥明文在脚本 | — |
| 物理后门 | `ik_console_passwd_clean`（grub 1s 超时，物理可重置 admin） | — |
| 会话可预测 | sess_key = MD5(now..body..port..addr) | — |
| 停维组件 | libssl.so.1.0.0 | 4.0.24 审计报 CRITICAL CVSS 7.5 |
| 安全靠混淆 | rootfs 双层加密（闭源保护，非安全设计） | — |

### 3.3 生态/开放性缺陷
1. **闭源**：无法审计、无法二次开发、无第三方插件体系
2. **无包管理**：不像 OpenWrt 有 opkg/apk 包生态
3. **社区弱**：技术资料少，出问题难排查

---

## 四、先进路由技术全景（对标参考）

### 4.1 数据面（转发性能）
| 技术 | 性能 | 取舍 | 适用 |
|---|---|---|---|
| nftables flowtable | 软 offload 2-3x 提速 | 保留完整 netfilter，最易落地 | **首选** |
| eBPF/XDP | 早期过滤，纳秒级 drop | 内核 verifier 保证安全，保留工具链 | DDoS 缓解、ACL 加速 |
| AF_XDP | 11.3 Mpps/core（zero-copy） | 保留 Linux 控制面，DPDK 级性能 | 高吞吐进阶 |
| DPDK | 20+ Mpps/core | 专用核 100% 轮询、失去 Linux 工具、硬件锁 | 电信级 NFV |
| VPP | 批量处理，完整协议栈 | 代码量大、学习陡 | 服务商网络 |

**结论**：新一代系统应「先 nftables flowtable（够用+易维护），预留 XDP/AF_XDP 扩展点」，不必一上来 DPDK。

### 4.2 控制面（路由协议）
- **FRRouting**：BGP/OSPFv2/v3/IS-IS/RIP/PIM/LDP/BFD/VRRP/PBR 全协议，多 daemon + zebra 中间件架构（单 daemon 崩溃不影响其他），mgmtd 走 YANG 北向配置，GPLv2
- **结论**：直接集成 FRR，**绝不自研路由协议**（自研 BGP/OSPF 是重复造轮子且必出 bug）

### 4.3 管理面（Web/API 架构）—— 关键差异点
| 方案 | 架构 | 评价 |
|---|---|---|
| iKuai | openresty + lua + shell 分发 | 反面教材：shell 分发 = 注入 + 性能差 |
| OpenWrt LuCI→ucode | Lua 服务端 → 静态 HTML+JS XHR + ubus | 演进方向正确 |
| OpenWrt **uapi**（25.12+） | 原生 REST API，REST→ubus，原子写入，Bearer auth，分级 scope | **最佳实践范本** |
| oui | Vue3+Element Plus+Vite 前端 + Lua-eco 后端，前后端分离 | 现代 SPA 参考 |
| nasnet-panel | React SPA + Go API 单容器 | 轻量参考 |

**结论**：SPA（Vue3/React）前端 + 二进制 REST 后端（Go/Rust）+ 结构化 IPC（ubus 风格），**彻底抛弃 shell 分发**。

### 4.4 云管理 / SD-WAN
- **四平面分离**：管理面（GUI/配置）+ 控制面（路由/策略分发）+ 数据面（转发）+ 编排面（设备 onboarding）
- Cisco：vManage/vSmart/vBond/vEdge + OMP（BGP 衍生）+ ZTP + BFD
- **关键设计**：数据面边缘设备缓存策略，控制面宕机仍转发（「管理面宕机 4 小时事故，数据面宕机=分支断网」）
- **结论**：若做云管理，务必分离控制面/数据面，边缘缓存策略 + 证书 onboarding

### 4.5 安全（零信任）
- **零信任（NIST 800-207）**：never trust, always verify，逐请求验证身份/设备/上下文
- **SASE/ZTNA**：SD-WAN + SSE 融合，最小权限
- **结论**：默认拒绝、无硬编码密钥、强认证（argon2id 优于 PBKDF2）、证书管理、最小攻击面

---

## 五、新一代路由系统架构建议（初稿，待细化）

### 5.1 分层架构
```
┌─ 管理面 ─────────────────────────────┐
│  Vue3 SPA 前端  │  Go/Rust REST API  │
│        结构化 IPC（ubus 风格，非 shell）│
└───────────────┬──────────────────────┘
┌─ 控制面 ──────▼──────────────────────┐
│  FRR（BGP/OSPF/...） │ 配置引擎（UCI/YANG）│
│  策略引擎 │ 状态管理                   │
└───────────────┬──────────────────────┘
┌─ 数据面 ──────▼──────────────────────┐
│  Linux netfilter + nftables flowtable │
│  （预留 XDP/AF_XDP 扩展）              │
└──────────────────────────────────────┘
      （可选）云管理层：四平面分离，边缘缓存策略
```

### 5.2 关键技术选型
| 组件 | 选型 | 理由 |
|---|---|---|
| 基础 OS | OpenWrt 或 Debian+定制 | OpenWrt 有成熟构建+包生态 |
| 数据面 | nftables + flowtable offload | 性能/维护平衡 |
| 控制面 | FRRouting | 免自研路由协议 |
| 管理后端 | Go 或 Rust（二进制，非 shell） | 安全 + 性能 |
| 前端 | Vue3/React SPA | 前后端分离 |
| IPC | ubus / 自定义 Unix socket + 结构化消息 | 替代 shell 分发 |
| 配置 | UCI 或 YANG/netconf | 原子写入 + 回滚 |
| 认证 | argon2id + 短期 token（非 MD5 可预测） | 安全 |
| 密钥 | 无硬编码，启动时生成/从 TPM 派生 | 消除硬编码密钥 |

### 5.3 差异化定位建议
1. **继承 iKuai 优点**：中文界面、DPI 流控、AC 管理、多线负载、云管理
2. **补 iKuai 短板**：开源（可审计可扩展）、安全（零信任/无硬编码/强认证）、现代架构（REST 后端非 shell、FRR 路由、flowtable 性能）
3. **对标竞品**：OpenWrt（生态）+ MikroTik（专业路由）+ VyOS（CLI 企业级）的交叉地带，主打「中文 + 安全 + 现代架构 + 开源」

---

## 六、关键结论（TL;DR）

1. **iKuai 的核心竞争力是「中文 + DPI 流控 + 全家桶功能 + 免费」**，不是技术架构——它的架构（shell 分发、单体、闭源加密）恰恰是短板
2. **iKuai 的安全问题系统性**：默认弱密码、命令注入（有 CVE）、SQL 注入历史、硬编码密钥、物理后门、可预测会话——全是「默认不安全」的设计
3. **先进路由技术共识**：控制面/数据面/管理面分离；数据面用 flowtable/XDP 加速；控制面用 FRR；管理面用二进制 REST + 结构化 IPC（不用 shell）；安全走零信任
4. **重新做一个路由系统的正确姿势**：继承 iKuai 的功能广度 + 中文体验，但架构上彻底现代化（FRR + nftables flowtable + Go/Rust REST + SPA + 零信任安全 + 开源）

---

*报告完。下一步建议：进入架构设计（brainstorming → 设计文档 → spec），确定目标形态（OpenWrt 派生 vs 全新发行版 vs 管理面板）、第一阶段 MVP 范围。*
