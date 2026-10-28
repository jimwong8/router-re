# router-re 剩余工作（未完成项）

> 依据：对 `mgmtd/internal` 与 `docs/ikuai-vs-routerre-feature-comparison.md` 的核对（grep/ls/proc 读取），不确定项标「未核实」。
> 验证环境（2026-10-29）：mgmtd Go 后端，root 起于本机 127.0.0.1:9090（admin/admin123 拿 Bearer token），
> 二进制 = 当前 HEAD 构建（`CGO_ENABLED=1 go build ./...` rc=0，`go test ./...` 全包通过）；
> acct/proc 读值于本机内核（nf_conntrack 模块已加载）。

> 更新 2026-10-29 04:50：remaining-work 原 4 项缺口已全部落地（acct 字节统计 / authportal 接线 / terminals 端点），见下表「已清零」。

| 模块 | 缺口 | 影响 | 阻塞原因 | 建议 |
|---|---|---|---|---|
| 无线服务 (wireless) | 仅本机 SSID/hostapd 配置渲染；AC（Access Controller）管理按要求永久移除 | 无法管理下联 AP/瘦 AP，仅能配本机 AP | 设计决定（见 `internal/wireless/ac.go` 注释） | 如需 AC，另立项目；当前保持现状 |
| DPI (dpi/dpiflow) | 仅用户态流跟踪（TLS SNI/HTTP Host/DNS），**未接入 NFQUEUE 数据面** | 无法在转发路径上实时分类/拦截，仅渲染 nft 规则文本 | 需内核 NFQUEUE + 用户态队列；`grep NFQUEUE` 在 dpi 包中无命中 | 接入 NFQUEUE + bypass，避免用户态挂掉断网 |
| DPI 编译降级 | `ndpi_stub.go` 在非 linux/非 cgo 环境下 `ClassifyPacket` 恒返回 `ErrUnsupported` | 跨平台构建能编译但无分类能力 | CGO 依赖 nDPI | 目标机保持 CGO_ENABLED=1 |

## 已清零（2026-10-29 复核通过）

| 模块 | 证据 | 复核方式 |
|---|---|---|
| 流量字节计数 (online) | `sysctl/acct.go` 实现 + `server/p24.go` 接线（GET/POST /api/v1/conntrack/acct），`PersistAcctConf("")` 写 `/etc/sysctl.d/90-router-re-conntrack.conf`；本机 `/proc/sys/net/netfilter/nf_conntrack_acct=1`，9090 实测 `{"on":true,"persisted":true}` | 9090 真实 HTTP + proc 读值 |
| 认证门户 (authportal) | `server/p18b.go` 已接线（loadPortalTemplate/Validate/handle*，`authportal.Builtin()` 内置模板） | grep 全仓 server 包引用 |
| 终端列表 | `server.go` L298-299：`GET /api/v1/terminals` + `GET /api/v1/conntrack/terminals`；9090 实测返回 32 台（arp 源） | 9090 真实 HTTP |
| 固件升级 confirm 门禁 | apply 需 confirm=true，plan 只计划不执行（设计保留，非缺口） | 代码审阅 |

说明：对比文档 `ikuai-vs-routerre-feature-comparison.md` 列出的许多「❌」项（IPv6、静态路由、DHCP 静态绑定、DDNS、WireGuard 客户端、无线 SSID 等）**已在代码中实现**（`internal/ipv6`、`internal/route/static.go`、`internal/dhcp`、`internal/ddnsup`、`internal/vpn`、`internal/wireless`），文档为旧版，勿按文档判定。不确定项标「未核实」。
