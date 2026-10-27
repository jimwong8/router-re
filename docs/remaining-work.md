# router-re 剩余工作（未完成项）

> 依据：对 `mgmtd/internal` 与 `docs/ikuai-vs-routerre-feature-comparison.md` 的核对（grep/ls/proc 读取），不确定项标「未核实」。

| 模块 | 缺口 | 影响 | 阻塞原因 | 建议 |
|---|---|---|---|---|
| 无线服务 (wireless) | 仅本机 SSID/hostapd 配置渲染；AC（Access Controller）管理按要求永久移除 | 无法管理下联 AP/瘦 AP，仅能配本机 AP | 设计决定（见 `internal/wireless/ac.go` 注释） | 如需 AC，另立项目；当前保持现状 |
| DPI (dpi/dpiflow) | 仅用户态流跟踪（TLS SNI/HTTP Host/DNS），**未接入 NFQUEUE 数据面** | 无法在转发路径上实时分类/拦截，仅渲染 nft 规则文本 | 需内核 NFQUEUE + 用户态队列；`grep NFQUEUE` 在 dpi 包中无命中 | 接入 NFQUEUE + bypass，避免用户态挂掉断网 |
| 流量字节计数 (online) | 依赖 `nf_conntrack_acct`，当前 `/proc/sys/net/netfilter/nf_conntrack_acct = 0`（已核实） | 在线用户页 rx/tx 全为 0（非无流量，是无统计） | 内核参数未开启 | `echo 1 > /proc/sys/net/netfilter/nf_conntrack_acct` 并持久化 |
| 认证门户 (authportal) | 包已实现（模板校验/渲染，含 XSS 与伪协议防护），**未接线到 server**（无 `/api/v1/authportal` 路由） | 门户模板无法通过管理 API 配置/渲染 | 待接线 | 在 server.go 注册路由并加前端页 |
| 终端列表 | 无 `/api/v1/terminals` 或等价端点（grep server.go 无 terminals/devices 列表） | 无法列出在线设备 IP/MAC/流量 | 未实现 | 基于 ARP/conntrack 聚合实现 |
| 固件升级 (firmware) | 有 inspect/plan/apply；apply 真执行 `sysupgrade`（需 confirm=true），plan 仅生成计划不执行 | 升级会重启设备并中断网络；无真正的 dry-run 预演 beyond plan | 设计（confirm 门禁） | 保留；必要时加 sha256/板型二次确认 |
| 未接线包 (其他) | `authportal`（模板渲染）无 server 路由；`server` 自身包不算 | 上述功能无法通过 API 使用 | 待接线 | 同上 |
| DPI 编译降级 | `ndpi_stub.go` 在非 linux/非 cgo 环境下 `ClassifyPacket` 恒返回 `ErrUnsupported` | 跨平台构建能编译但无分类能力 | CGO 依赖 nDPI | 目标机保持 CGO_ENABLED=1 |

说明：对比文档 `ikuai-vs-routerre-feature-comparison.md` 列出的许多「❌」项（IPv6、静态路由、DHCP 静态绑定、DDNS、WireGuard 客户端、无线 SSID 等）**已在代码中实现**（`internal/ipv6`、`internal/route/static.go`、`internal/dhcp`、`internal/ddnsup`、`internal/vpn`、`internal/wireless`），文档为旧版，勿按文档判定。不确定项标「未核实」。
