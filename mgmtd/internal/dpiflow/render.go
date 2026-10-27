package dpiflow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RenderNftMarkRules 把"应用→端口列表"渲染成 nftables 规则：对匹配端口的包打上
// 不同 fwmark（0x20 / 0x21 / …），供 tc/策略路由按 mark 分流限速。
//
// 约束（本模块硬性要求）：
//   - 绝不写 `flush ruleset`（会清空整个防火墙规则集，绝不允许）
//   - 绝不写 `policy drop`（只打 mark，不丢包）
//   - chain 声明必须多行（单行闭合会让后续规则落到 table 顶层导致 nft 语法错误）
func RenderNftMarkRules(appPorts map[string][]int) string {
	if appPorts == nil {
		appPorts = map[string][]int{}
	}
	// 稳定输出顺序：应用名排序，端口排序。
	apps := make([]string, 0, len(appPorts))
	for a := range appPorts {
		apps = append(apps, a)
	}
	sort.Strings(apps)

	var b strings.Builder
	b.WriteString("table inet router_dpi {\n")
	// chain 必须多行声明：闭括号留在下一行，规则才属于 chain 内部。
	b.WriteString("    chain classify {\n")
	b.WriteString("        type filter hook forward priority mangle; policy accept;\n")

	mark := 0x20
	first := true
	for _, app := range apps {
		ports := appPorts[app]
		if len(ports) == 0 {
			continue
		}
		sort.Ints(ports)
		if !first {
			b.WriteString("\n")
		}
		first = false
		// 端口集合渲染为 tcp dport { a, b, c } 形式。
		parts := make([]string, 0, len(ports))
		for _, p := range ports {
			parts = append(parts, strconv.Itoa(p))
		}
		b.WriteString(fmt.Sprintf("        tcp dport { %s } meta mark set 0x%02x\n",
			strings.Join(parts, ", "), mark))
		// 同样匹配 udp（多数 P2P/流媒体走 udp）
		b.WriteString(fmt.Sprintf("        udp dport { %s } meta mark set 0x%02x\n",
			strings.Join(parts, ", "), mark))
		mark++
	}
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

// RenderNfqueueRule 生成一条 NFQUEUE 规则，把匹配的包交给用户态队列 num。
//
// ⚠ bypass 是必须的：若用户态 DPI 进程挂掉/退出，内核的 queue 队列会积压，
// 所有命中流量会被卡死。`bypass` 让内核在队列无消费者时直接把包放行，
// 保证"DPI 挂了不至于断网"。这个特性是接入 NFQUEUE 数据面的前提。
func RenderNfqueueRule(chain string, queueNum int) string {
	if chain == "" {
		chain = "classify"
	}
	return fmt.Sprintf("table inet router_nfq {\n"+
		"    chain %s {\n"+
		"        type filter hook forward priority filter; policy accept;\n"+
		"        queue num %d bypass\n"+
		"    }\n"+
		"}\n", chain, queueNum)
}
