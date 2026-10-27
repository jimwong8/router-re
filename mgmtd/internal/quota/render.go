package quota

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// RenderNft 把策略列表渲染成 nftables 文本：table inet router_quota 下一张 forward 链，
// 链内按"限速/连接数/每秒新建"逐条生成 drop 规则。
//
// **安全红线**：
//   - **绝不 `flush ruleset`**——它会清空内核所有表，是灾难级事故。
//   - **绝不 `policy drop`**——链默认 accept，只 drop 命中策略的流量，否则一条
//     空/误渲染配置就会把整机转发流量掐断。
//
// 各维度的映射：
//   - ip/cidr  —— `ip saddr <target> limit ...`（按包速率）或指向 ipset 用字节；
//   - 连接数   —— `ip saddr <target> ct count over <n> drop`；
//   - mac      —— `ether saddr <mac> ...`；
//   - app      —— **nft 无法直接按应用名匹配**。这里生成注释说明"由 DPI 模块对
//     该应用打 meta mark 后按 mark 匹配"，并给每条 app 策略分配一个确定性 mark。
//     绝不假装能直接按应用名匹配。
func RenderNft(policies []Policy) string {
	var b strings.Builder
	b.WriteString("table inet router_quota {\n")
	b.WriteString("\tchain forward {\n")
	b.WriteString("\t\ttype filter hook forward priority 20;\n")
	b.WriteString("\t\t# 配额限速：default accept，仅 drop 命中策略的流量。\n")
	b.WriteString("\t\tct state established,related accept\n")

	// app 维度：为每个不同 app 分配一个确定性 mark（1 起）。
	appMark := map[string]int{}
	nextMark := 1
	for _, p := range policies {
		if !p.Enabled || p.TargetType != TargetApp {
			continue
		}
		if _, ok := appMark[p.App]; !ok {
			appMark[p.App] = nextMark
			nextMark++
		}
	}

	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		renderOne(&b, p, appMark)
	}

	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// renderOne 生成单条策略的规则文本。app 类额外输出依赖 mark 的说明。
func renderOne(b *strings.Builder, p Policy, appMark map[string]int) {
	// 描述行
	b.WriteString("\t\t# quota: " + p.Name + " (" + string(p.TargetType) + " " + p.Target + ")\n")

	// 选择匹配头的表达式前缀（按维度）。
	expr := ""
	isApp := false
	switch p.TargetType {
	case TargetIP, TargetCIDR:
		host := strings.TrimSpace(p.Target)
		if p.TargetType == TargetIP {
			// 单 IP 规整
			if ip := net.ParseIP(host); ip != nil {
				host = ip.String()
			}
		}
		expr = "ip saddr " + host
	case TargetMAC:
		expr = "ether saddr " + strings.TrimSpace(p.Target)
	case TargetApp:
		isApp = true
		// nft 不能按应用名匹配：依赖 DPI 对该应用打 mark。
		mark := appMark[p.App]
		expr = fmt.Sprintf("meta mark 0x%x", mark)
	default:
		return
	}

	if isApp {
		// 必须写清 app 的限制与依赖，绝不假装能直接匹配。
		b.WriteString(fmt.Sprintf("\t\t# app %q: nft 无法直接按应用名匹配，由 DPI 模块对命中该应用的流量打此 mark 后匹配（meta mark 0x%x）。\n", p.App, appMark[p.App]))
		if p.RateMbit > 0 {
			pps := rateMbitToPPS(p.RateMbit)
			b.WriteString(fmt.Sprintf("\t\t%s limit rate over %d/second burst %s drop\n", expr, pps, burstPackets(p.BurstKB)))
		}
		if p.MaxConns > 0 {
			b.WriteString(fmt.Sprintf("\t\t%s ct count over %d drop\n", expr, p.MaxConns))
		}
		if p.MaxConnsPerSec > 0 {
			b.WriteString(fmt.Sprintf("\t\t%s limit rate %d/second drop\n", expr, p.MaxConnsPerSec))
		}
		return
	}

	// 非 app：限速（rate_mbit>0）
	if p.RateMbit > 0 {
		pps := rateMbitToPPS(p.RateMbit)
		b.WriteString(fmt.Sprintf("\t\t%s limit rate over %d/second burst %s drop\n", expr, pps, burstPackets(p.BurstKB)))
	}
	// 连接数
	if p.MaxConns > 0 {
		b.WriteString(fmt.Sprintf("\t\t%s ct count over %d drop\n", expr, p.MaxConns))
	}
	// 每秒新建
	if p.MaxConnsPerSec > 0 {
		b.WriteString(fmt.Sprintf("\t\t%s limit rate %d/second drop\n", expr, p.MaxConnsPerSec))
	}
}

// rateMbitToPPS 把 RateMbit(Mbit/s) 粗略换算成包/s（按 ~64 字节包估算，偏保守下限）。
// 这是近似：真实字节限速需配合 ipset + byte counters，这里为可读文案用包速率。
// 说明见 RenderNft 注释：app/ip 类用包速率近似，保证 nft 语法成立。
func rateMbitToPPS(mbit float64) int {
	// 1 Mbit/s ≈ 1000 kbit/s；按平均 512 byte/包算 ≈ 约 1900 包/s 每 Mbit。
	// 用 1500 包/s 每 Mbit 取整，避免过大 burst 值。
	pps := int(float64(mbit) * 1900)
	if pps < 1 {
		pps = 1
	}
	return pps
}

// burstPackets 从 BurstKB 推一个合理的 burst 包数（保证 ≥1）。
func burstPackets(burstKB int) string {
	if burstKB <= 0 {
		return "5"
	}
	// 保守按平均 1024 字节/包取包数。
	n := burstKB // KB → 约 1 包/KB
	if n < 1 {
		n = 1
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// RenderReport 生成人类可读的策略摘要，每条一行说明命中条件与动作。
func RenderReport(policies []Policy) string {
	var lines []string
	for _, p := range policies {
		if !p.Enabled {
			lines = append(lines, fmt.Sprintf("[disabled] %s (%s %s)", p.Name, p.TargetType, p.Target))
			continue
		}
		var acts []string
		if p.RateMbit > 0 {
			acts = append(acts, fmt.Sprintf("限速 %.2f Mbit/s", p.RateMbit))
		}
		if p.MaxConns > 0 {
			acts = append(acts, fmt.Sprintf("最多 %d 并发连接", p.MaxConns))
		}
		if p.MaxConnsPerSec > 0 {
			acts = append(acts, fmt.Sprintf("每秒新建 ≤%d", p.MaxConnsPerSec))
		}
		action := strings.Join(acts, " + ")
		if action == "" {
			action = "（无限速/连接限制）"
		}
		if p.TargetType == TargetApp {
			lines = append(lines, fmt.Sprintf("[%s] %-12s app=%s（依赖 DPI 打 mark）：%s", p.ID, p.Name, p.App, action))
		} else {
			lines = append(lines, fmt.Sprintf("[%s] %-12s %s %-18s：%s", p.ID, p.Name, p.TargetType, p.Target, action))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
