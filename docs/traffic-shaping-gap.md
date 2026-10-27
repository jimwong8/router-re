# 「按应用限速」到底有没有真的生效？

## 结论（一句话）

**按应用限速在真实转发链路上未生效**：DPI 只在用户态做流跟踪/端口打标记（fwmark），数据面没有接 NFQUEUE，也没有把识别结果实时接回内核做限速——`RenderNfqueueRule` 只存在于渲染函数与其单测里，生产代码没有任何 NFQUEUE 用户态队列消费者。

---

## 证据（每条带命令和真实输出片段）

### 1. NFQUEUE 引用只在渲染函数 + 单测，生产代码无消费者

```
cd /home/jimwong/projects/router-re && grep -rn 'NFQUEUE\|nfqueue\|nfq' mgmtd/internal --include=*.go

mgmtd/internal/dpiflow/render.go:63: // RenderNfqueueRule 生成一条 NFQUEUE 规则，把匹配的包交给用户态队列 num。
mgmtd/internal/dpiflow/render.go:67: // 保证"DPI 挂了不至于断网"。这个特性是接入 NFQUEUE 数据面的前提。
mgmtd/internal/dpiflow/render.go:72: return fmt.Sprintf("table inet router_nfq {\n"+ ...
mgmtd/internal/dpiflow/render_test.go:59: t.Fatalf("NFQUEUE must have bypass ...")
mgmtd/internal/dpiflow/render_test.go:65: t.Fatalf("nfq chain must be accept ...")
```

`RenderNfqueueRule` 被调用的地方**只有测试**：

```
grep -rn "RenderNfqueueRule(" mgmtd/internal --include=*.go | grep -v render.go
mgmtd/internal/dpiflow/render_test.go:57: out := RenderNfqueueRule("classify", 7)
mgmtd/internal/dpiflow/render_test.go:70: out := RenderNfqueueRule("", 3)
```

server 端也没有任何把包 `Observe` 进 ${code:Tracker} 的入口（mark rule 只做端口打标，不走队列）：

```
grep -rn "RenderNfqueueRule\|\.Tracker\|dpiflow.Tracker\|Observe(" mgmtd/internal/server --include=*.go
（无输出 —— server 没有 NFQUEUE 用户态消费者）
```

### 2. `dpiflow` 里 DPI 是用户态流跟踪，不接内核

`flow.go` 开头即声明"把从'离线单包识别'升级为'按流跟踪的应用识别'"：

```
// dpiflow 把 nDPI/启发式分类从"离线单包识别"升级为"按流跟踪的应用识别"。
//
// 核心是 Tracker：
//   - 双向流归一 ...
//   - DPI 结果后置回填：识别出应用后调用 MarkDetected 写回。
```
其中 `Packet` 是用户态构造的结构体（含 `Payload []byte`），**并不是从 NFQUEUE 收包**。

`render.go` 的 `RenderNftMarkRules` 只按"应用→端口列表"打 `meta mark set`（fwmark），**和限速无关**：

```
b.WriteString("table inet router_dpi {\n")
b.WriteString("        tcp dport { %s } meta mark set 0x%02x\n", ...)
b.WriteString("        udp dport { %s } meta mark set 0x%02x\n", ...)
```

### 3. `dpi/mark.go` 的 "limit" 动作并未真限速

代码注释明说"break 只打 mark，tc 侧限速"，但实际渲染 `limit` 落到 default 分支**只打 fwmark**，限速依赖的 tc 侧在 DPI 路径上并不存在：

```
mgmtd/internal/dpi/mark.go:13: // then either set an fwmark (for tc shaping) or drop the packet.
mgmtd/internal/dpi/mark.go:70:    case "drop":
mgmtd/internal/dpi/mark.go:72:    default: // mark / limit break 只打 mark，tc 侧限速
```

`MarkRulesForTC` 确实生成 `tc filter ... handle <mark> fw classid`，但 DPI 路径没有调用它；tc 命令在生产代码里只出现在 `mirror`（流量镜像）与 `htb`（按网卡分层榨**接口级**带宽）两处，均非"按识别的应用"限速。

### 4. 运行时规则集里没有 router_dpi / router_nfq 表，内核没挂 nfnetlink_queue

```
sudo -n nft list ruleset 2>/dev/null | grep -i queue
（无输出 —— 没有任何 queue 语句）

lsmod | grep -i nfnetlink_queue
（无输出 —— 内核未加载 nfnetlink_queue 模块）

ls /proc/net/netfilter/ 2>/dev/null
nfnetlink_log
（只有 log，没有 queue；ion 此环境为 LXC 容器）
```

当前真实规则的 nft 表只有 `xrdp_guard`、`ip6 lxc`、`ip filter(DOCKER)` 等，**没有 `router_dpi` 表**。

### 5. `rate_kbps` 只在"时间段策略"包里，跟 DPI 应用分类无关

```
grep -rn 'RateKbps\|rate_kbps' mgmtd/internal --include=*.go
internal/timesched/schedule.go:22:  RateKbps int `json:"rate_kbps"`
internal/timesched/schedule.go:30:  RateKbps int `json:"rate_kbps"`
internal/timesched/schedule.go:68:  if r.RateKbps < 1 || r.RateKbps > 1000000 ...
```
`rate_kbps` 属于 `timesched`（按时段/目标/IP/端口限速），**不是"按 DPI 识别出的应用"**。

### 6. 项目自己的待办文档已承认该缺口

```
grep -n "NFQUEUE\|DPI" docs/remaining-work.md

8: | DPI (dpi/dpiflow) | 仅用户态流跟踪（TLS SNI/HTTP Host/DNS），**未接入 NFQUEUE 数据面** |
   无法在转发路径上实时分类/拦截，仅渲染 nft 规则文本 | 需内核 NFQUEUE + 用户态队列；`grep NFQUEUE` 在 dpi 包中无命中 | 接入 NFQUEUE + bypass，避免用户态挂掉断网 |
```

---

## 缺口（差哪一步才能真正按应用限速）

1. **数据面无队列**：没在转发链上插入 `queue` 语句，内核 `nfnetlink_queue` 模块未加载，没有任何 NFQUEUE 用户态接收/回写 loop。DPI 的结果**回不到内核去影响转发决策**。
2. **识别结果不落地**：`dpiflow.Tracker` 只把识别结果记在内存流表（`App/Category` 字段），`MarkDetected` 只回填内存，没有转换成 fwmark 或限速规则下发给内核。
3. **mark 与限速断链**：即使 `RenderNftMarkRules` 打了 fwmark（0x20/0x21…），也没有对应的 tc class/tbf 按该 mark 真正限制速率——`MarkRulesForTC` 生成 tc filter 但 DPI 路径从未执行它。
4. **现有限速是"按时段/接口/端口"，不是"按应用"**：`timesched`（按规则目标/IP/端口）+ `htb`（按接口）都能限速，但都不依赖 DPI 做出的应用分类。

---

## 要补齐需要什么（具体到文件和内核模块）

- **内核侧**：启用 `nfnetlink_queue` 模块（以及 CONFIG_NETFILTER_NETLINK_QUEUE），并在转发链（`router_dpi`/新增 `router_nfq` 表）插入一条**带 bypass 的 queue 规则**——`dpiflow/render.go:RenderNfqueueRule` 已生成该文本，注释也写了"DPI 挂了不至于断网"的前提，但从未接入生产。
- **用户态消费者（核心缺口，目前完全没有）**：在守护进程里开一个 NFQUEUE 收包 loop（Go 用 `github.com/florianl/go-nflog` / `nfq` 库，或 CGO 的 libnetfilter_queue），从内核队列读包 → 送 `dpiflow.Tracker`/`DetectApp` 识别 → 决策后调用 `nft` 打 fwmark（`dpi/mark.go:RenderMarkNft`）或直接对匹配流 `drop`，并**在队列满/超时时 netfilter 默认作 `ACCEPT`**（即注释里强调的 bypass，宁可漏检也不断网）。
- **tc 限速补齐**：真正把 fwmark 映射到类限速——每应用一个 tc class + `tbf`/`htb` 叶子，用 `dpi/mark.go:MarkRulesForTC` 生成的 `tc filter ... handle <mark> fw classid` 把打了 mark 的流导向对应 class；生产代码目前只生成了这段文本，从未执行。
- **接线文件**：
  - `mgmtd/internal/dpiflow/render.go` —— 已有 `RenderNfqueueRule`，把它接进一个真正启用的 chain。
  - `mgmtd/internal/dpi/mark.go` —— `MarkRulesForTC` 已有，但在生产未被调用。
  - `mgmtd/internal/server/server.go` —— 目前 `handleSetDPIMarkRules` 只把 `app_ports` 渲染成 `router_dpi` 打标 nft，且未接队列；需新增：队列监听启动 + 按 mark 的 tc class 落地。
- **自检**：`lsmod | grep nfnetlink_queue`、`nft list ruleset | grep -i queue`、以及单测里 `RenderNfqueueRule` 的"bypass 必须 accept"断言（`render_test.go:65`）——三者齐全才算真正接通。

## 诚实性说明

- 已核实：DPI 仅在用户态做流跟踪，按应用限速在真实转发链路**未生效**（证据 1/2/4/5 直接支撑）。
- 未核实：本环境为 LXC 容器、`id -u=1000` 非 root，未见完整的路由器 host 上的 nft 部署现场；路由器真实运行时的整机规则集未能全量读取，故"路由器生产机是否拉了其他脚本补上 dataplane"这一点标 **未核实**。但**当前仓库代码本身不包含任何 NFQUEUE 用户态消费者**，这是可复现的结论。