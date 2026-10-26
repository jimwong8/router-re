# 路由系统架构设计文档（R2 / working title）

> 日期：2026-10-27
> 状态：设计草案 v0.1（待评审）
> 前置：`iKuai-analysis-and-advanced-router-tech-report.md`（竞品解剖 + 技术对标）

---

## 1. 项目概述

模仿 iKuai 做一个**现代化、开源、安全**的路由系统。继承 iKuai 的「中文友好 + DPI 流控 + 全家桶功能 + 免费」，用现代架构重做（FRR 路由 + nftables flowtable + Go REST 后端 + Vue3 SPA + 零信任安全）。

### 1.1 关键决策（已定）
| 决策项 | 选择 | 理由 |
|---|---|---|
| 形态 | **OpenWrt 派生** | 成熟构建系统 + 驱动 + 包生态，避免自造内核/驱动轮子 |
| 管理后端 | **Go**（单二进制 `mgmtd`） | 并发好、交叉编译简单、部署为单文件、性能足够 |
| 前端 | **Vue3 + TypeScript + Vite + Element Plus** | 中文生态好、前后端分离 |
| 数据面 | **nftables + flowtable offload** | 性能/维护平衡，预留 XDP 扩展点 |
| 控制面 | **FRRouting**（MVP 后置） | 免自研路由协议 |
| 配置 | **UCI + 快照/回滚** | OpenWrt 标准，原子提交 |
| 认证 | **argon2id + 短期 JWT** | 消除 PBKDF2/MD5 可预测会话 |

### 1.2 明确不做（YAGNI）
- 不自己写内核/驱动（用 OpenWrt）
- 不自研 BGP/OSPF（用 FRR）
- MVP 不做 DPI（七层流控是 iKuai 强项但最重，放第二阶段）
- 不做云管理（放第三阶段）

---

## 2. 架构总览（平面分离）

```
┌─────────────────────────────────────────────────┐
│ 管理面 Management Plane                          │
│  ┌──────────────┐   REST(HTTPS)   ┌──────────┐  │
│  │ Vue3 SPA     │ ◄──────────────► │ mgmtd(Go)│  │
│  │ (浏览器)      │                  │ 单二进制  │  │
│  └──────────────┘                  └────┬─────┘  │
│        UCI 配置 + 快照/回滚 + 审计日志      │        │
└──────────────────────────────────────────┼─────┘
                                            │ netlink / UCI / 生成 nftables 规则
┌──────────────────────────────────────────▼─────┐
│ 控制面 Control Plane                            │
│  ┌─────────────────────────────────────────┐   │
│  │ 配置引擎 │ 策略引擎 │ 状态管理 │ FRR(后置) │   │
│  └─────────────────────────────────────────┘   │
└──────────────────────────────────────────┬─────┘
                                            │ 规则下发
┌──────────────────────────────────────────▼─────┐
│ 数据面 Data Plane                               │
│  nftables + flowtable offload + conntrack       │
│  （预留 XDP/AF_XDP 扩展点）                       │
└─────────────────────────────────────────────────┘
```

**核心原则**：管理面/控制面/数据面分离。管理面（mgmtd）只改配置和下发规则，不碰转发路径；数据面（nftables）独立转发，管理面宕机不影响转发（对齐 SD-WAN 的「数据面缓存策略、控制面宕机仍转发」）。

---

## 3. 组件设计

### 3.1 mgmtd（管理守护进程，Go）

**职责**：REST API 服务 + 认证 + 配置管理 + 规则生成。

**对外接口**（REST，HTTPS，本地 127.0.0.1 或管理口）：
```
POST /api/v1/auth/login          # 登录 → JWT
POST /api/v1/auth/logout
GET  /api/v1/system/info         # 系统信息
GET  /api/v1/interfaces          # 接口列表/状态
POST /api/v1/interfaces/{name}   # 配置接口（IP/网关/DNS）
GET  /api/v1/firewall/rules      # 防火墙规则
POST /api/v1/firewall/rules      # 增删改防火墙规则
GET  /api/v1/nat/rules           # NAT 规则
GET  /api/v1/wan/lb              # 多 WAN 负载均衡
POST /api/v1/dhcp/config         # DHCP 配置
POST /api/v1/config/apply        # 应用配置（原子提交 + 回滚）
POST /api/v1/config/rollback     # 回滚到上一个快照
GET  /api/v1/audit/logs          # 审计日志
```

**内部实现**：
- **配置层**：封装 UCI 读写。所有写操作走「快照 → 校验 → 提交 → 应用」事务，失败自动回滚（对齐 OpenWrt uapi 的原子写入模式）。
- **规则生成**：把抽象的防火墙/NAT 策略编译成 nftables 规则集（不直接暴露 shell 拼接）。
- **netlink**：接口地址/路由操作走 netlink，不用 shell 调 ip 命令。
- **无 shell 分发**：彻底废弃 iKuai 的 func_api.sh 模式，所有操作走 Go 内建库（netlink、UCI 绑定、nftables 库）。

**认证**：
- 密码 argon2id 哈希（盐随机）
- 登录成功发 JWT（短期，默认 15min）+ refresh token
- 会话不依赖 MD5 可预测值

**权限**：RBAC（admin/operator/viewer），每个 API 端点声明所需角色。

**审计**：所有写操作记录到审计日志（谁、何时、改了什么、结果）。

### 3.2 Web 前端（Vue3 SPA）

**职责**：中文管理界面，通过 REST 与 mgmtd 通信。

**页面结构**（对齐 iKuai 用户习惯）：
- 登录页
- 仪表盘（系统概览：CPU/内存/接口流量/在线设备）
- 网络设置（WAN/LAN 接口、DHCP、DNS）
- 防火墙（规则、NAT、端口映射）
- 多线负载（WAN 负载均衡策略）
- 流控（第二阶段 DPI）
- 系统（升级、备份/恢复、日志、审计）

**技术**：Vue3 + TS + Vite + Element Plus，axios 调 REST，JWT 放 httpOnly cookie 或 Authorization 头。

### 3.3 数据面

- **nftables**：mgmtd 生成规则集（input/forward/nat 链），启用 flowtable offload 提速已建立连接。
- **conntrack**：连接跟踪，状态化防火墙。
- **扩展点**：预留 XDP hook（ACL/DDoS 加速），接口抽象不绑死。

### 3.4 配置层

- **UCI**：OpenWrt 标准配置后端。
- **快照/回滚**：每次 apply 前存快照，失败或超时自动回滚。
- **配置校验**：schema 校验（拒绝非法值，防止注入/错误配置）。

---

## 4. 数据流

### 4.1 配置变更流（防火墙规则为例）
```
用户(浏览器) → POST /api/v1/firewall/rules {规则}
  → mgmtd 校验 schema → 写 UCI → 快照
  → 生成 nftables 规则集 → 原子替换 nftables
  → 成功：返回 200 + 审计记录
  → 失败：回滚 UCI + 恢复旧 nftables + 返回 500（含原因）
```

### 4.2 转发路径（不经过管理面）
```
数据包 → NIC → XDP(可选) → nftables(flowtable 命中→快速路径) 
  → conntrack → 转发决策 → NAT → 出接口
```
管理面（mgmtd）完全不在转发路径上。

---

## 5. 错误处理

- **配置失败回滚**：任何 apply 失败，恢复上一个快照，绝不留下半应用状态。
- **规则校验**：nftables 规则生成后先 `nft -c` 干跑校验，通过才提交（对齐 council-review 精神：先校验后应用）。
- **后端错误**：统一 JSON 错误结构 `{code, message, detail}`，前端可展示。
- **审计**：错误也记录，便于排查。

---

## 6. 测试策略

- **单元测试**：mgmtd 的规则生成（策略→nftables 规则）、UCI 事务、JWT 认证。
- **集成测试**：在 OpenWrt 容器/QEMU 里起 mgmtd + nftables，验证「配置→规则→实际转发」端到端。
- **安全测试**：认证绕过、注入（参数拼进命令）、越权（RBAC）。
- **端到端验收**：真机/VM 上验证「登录 → 配接口 → 配防火墙 → 实际拦截/放行流量」。

---

## 7. 分阶段规划

### 阶段 1（MVP，可运行最小系统）
- 基于 OpenWrt 构建，加 mgmtd + Web UI
- 功能：接口配置（WAN/LAN）、NAT、防火墙规则、DHCP、DNS、系统概览
- 安全：argon2id 认证 + JWT + RBAC + 审计
- 验收：能拨号/静态上网、能配置防火墙拦截、能配 DHCP 分配地址，全程 Web 操作

### 阶段 2（对标 iKuai 核心）
- 多 WAN 负载均衡（PCC/策略路由）
- DPI 七层流控（nDPI 或自研）
- VPN（WireGuard/OpenVPN/IPsec）
- FRR 动态路由（BGP/OSPF）

### 阶段 3（差异化）
- 云管理（四平面分离，边缘缓存策略）
- AC 无线管理
- 行为管理/审计强化

---

## 8. 关键决策记录（ADR 摘要）

| 决策 | 结论 | 理由 |
|---|---|---|
| 形态 | OpenWrt 派生 | 免自造内核/驱动，有包生态 |
| 后端语言 | Go | 单二进制、并发、嵌入式友好 |
| 废弃 shell 分发 | 全 Go 内建库 | 消除 iKuai 命令注入类漏洞根源 |
| 认证 | argon2id + JWT | 消除 PBKDF2/MD5 可预测会话 |
| 数据面 | nftables flowtable | 性能/维护平衡 |
| 路由 | FRR（后置） | 不自研 |

---

## 9. 待评审问题（open questions）

1. 项目命名（working title "R2" 待定）
2. 目标硬件基线（x86 通用？是否支持 ARM/工控机？）
3. 前端是否要移动端响应式（iKuai 有微信小程序云管理，本系统 MVP 是否做移动适配）
4. 是否要兼容 iKuai 配置导入（用户迁移成本）

---

*设计文档完。下一步：spec 自审 → 用户评审 → writing-plans 出实施计划。*
