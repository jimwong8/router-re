package policyroute

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DomainRule 是一条域名分流规则。
type DomainRule struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Action   string `json:"action"`
	Table    int    `json:"table"`
	Fwmark   int    `json:"fwmark"`
	Priority int    `json:"priority"`
	Enabled  bool   `json:"enabled"`
	Comment  string `json:"comment"`
}

var domainBodyRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
var domainStarRe = regexp.MustCompile(`^\*\.([A-Za-z0-9.-]+)$`)

// ValidateDomainRules 校验一条或多条域名分流规则。
//
// 校验规则：
//   - Domain：允许可选星号点前缀，主体只允许字母数字点短横线，长度<=253，不能为空
//   - Action 只能是 route 或 block
//   - route 时 Table 必须在 1..252
//   - Fwmark 在 0..65535
//   - Priority 在 0..10000
//   - 最多 512 条
//   - 同一域名只能有一条 Enabled 规则
func ValidateDomainRules(rs []DomainRule) error {
	if len(rs) > 512 {
		return fmt.Errorf("最多只能有 512 条域名分流规则，Got %d", len(rs))
	}

	// 检查同一域名只能有一条 Enabled 规则
	enabledDomains := make(map[string]bool)
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		if enabledDomains[r.Domain] {
			return fmt.Errorf("同一域名 %q 已经有 Enabled 规则", r.Domain)
		}
		enabledDomains[r.Domain] = true

		// 校验 Domain 格式
		if r.Domain == "" {
			return fmt.Errorf("domain 不能为空")
		}
		if len(r.Domain) > 253 {
			return fmt.Errorf("domain 长度不能超过 253，Got %d", len(r.Domain))
		}

		// 支持 *.example.com 前缀
		if domainStarRe.MatchString(r.Domain) {
			// 星号点前缀格式合法，主体已被星号捕获
		} else if domainBodyRe.MatchString(r.Domain) {
			// 无星号，主体必须只含字母数字点短横线
			// 已通过 MatchString 验证
		} else {
			return fmt.Errorf("domain 格式非法: %q，必须为 example.com 或 *.example.com 格式，主体只能含字母数字点短横线", r.Domain)
		}

		// 校验 Action
		if r.Action != "route" && r.Action != "block" {
			return fmt.Errorf("action 必须是 route 或 block，Got %q", r.Action)
		}

		// route 时 Table 必须在 1..252
		if r.Action == "route" && (r.Table < 1 || r.Table > 252) {
			return fmt.Errorf("route 动作时 table 必须在 1-252，Got %d", r.Table)
		}

		// Fwmark 在 0..65535
		if r.Fwmark < 0 || r.Fwmark > 65535 {
			return fmt.Errorf("fwmark 必须在 0-65535，Got %d", r.Fwmark)
		}

		// Priority 在 0..10000
		if r.Priority < 0 || r.Priority > 10000 {
			return fmt.Errorf("priority 必须在 0-10000，Got %d", r.Priority)
		}
	}

	return nil
}

// SetName 返回稳定的集合名 rr_dom_<idx>（只含小写字母数字下划线）。
func SetName(idx int) string {
	return "rr_dom_" + strconv.Itoa(idx)
}

// collectionNames 给每条 Enabled 规则分配集合名。
//
// 顺序固定为 Priority 升序、同优先级按域名——dnsmasq 的 ipset 与 nft 的 set
// 必须同名，否则流量会被写进一个没人匹配的集合，规则静默失效。
func collectionNames(rs []DomainRule) map[string]string {
	enabled := make([]DomainRule, 0, len(rs))
	for _, r := range rs {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		pi, pj := enabled[i].Priority, enabled[j].Priority
		if pi == 0 {
			pi = 100
		}
		if pj == 0 {
			pj = 100
		}
		if pi != pj {
			return pi < pj
		}
		return enabled[i].Domain < enabled[j].Domain
	})
	m := make(map[string]string, len(enabled))
	for i, r := range enabled {
		if _, ok := m[r.Domain]; !ok {
			m[r.Domain] = SetName(i)
		}
	}
	return m
}

// dnsmasqDomain 去掉 *. 前缀。
//
// dnsmasq 的 server=/example.com/ 本来就覆盖该域及其所有子域，
// 写成 /.example.com/ 或 /*.example.com/ 是无效语法（前导点会被当成空标签）。
func dnsmasqDomain(d string) string {
	if strings.HasPrefix(d, "*.") {
		return d[2:]
	}
	return d
}

// RenderDnsmasqIpset 对每条 Enabled 规则输出一行 ipset=/<域名>/<集合名>。
// 未启用返回空串；同域名只输出一次（幂等）。按域名排序，输出稳定。
func RenderDnsmasqIpset(rs []DomainRule) string {
	names := collectionNames(rs)
	if len(names) == 0 {
		return ""
	}
	domains := make([]string, 0, len(names))
	for d := range names {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	var b strings.Builder
	for _, d := range domains {
		fmt.Fprintf(&b, "ipset=/%s/%s\n", dnsmasqDomain(d), names[d])
	}
	return b.String()
}

// RenderNft 输出一份完整的 nft 配置：集合定义 + prerouting 链 + 规则。
//
// 之前这里只拼了 "ip daddr @set mark" 这种裸规则行，缺 table/chain/set 定义，
// nft -f 会直接语法报错——「渲染成功」和「能用」是两件事。
// Fwmark 未分配（0）时返回错误，不用占位数字糊过去：占位 mark 会静默
// 把所有流量打到同一个路由表。
func RenderNft(rs []DomainRule, tableByName func(int) string) (string, error) {
	names := collectionNames(rs)
	if len(names) == 0 {
		return "", nil
	}
	enabled := make([]DomainRule, 0, len(rs))
	for _, r := range rs {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		pi, pj := enabled[i].Priority, enabled[j].Priority
		if pi == 0 {
			pi = 100
		}
		if pj == 0 {
			pj = 100
		}
		if pi != pj {
			return pi < pj
		}
		return enabled[i].Domain < enabled[j].Domain
	})

	var rules []string
	for _, r := range enabled {
		set := names[r.Domain]
		if r.Action == "block" {
			rules = append(rules, fmt.Sprintf("    ip daddr @%s drop", set))
			continue
		}
		if r.Fwmark == 0 {
			return "", fmt.Errorf("域名 %s 尚未分配 fwmark，请先调用 AssignMarks", r.Domain)
		}
		line := fmt.Sprintf("    ip daddr @%s meta mark set %d", set, r.Fwmark)
		if tableByName != nil {
			if name := tableByName(r.Table); name != "" {
				line += "  # table " + name
			} else {
				line += fmt.Sprintf("  # table %d", r.Table)
			}
		} else {
			line += fmt.Sprintf("  # table %d", r.Table)
		}
		rules = append(rules, line)
	}

	setNames := make([]string, 0, len(names))
	for _, n := range names {
		setNames = append(setNames, n)
	}
	sort.Strings(setNames)

	var b strings.Builder
	b.WriteString("table inet rr_dom {\n")
	for _, n := range setNames {
		fmt.Fprintf(&b, "  set %s {\n    type ipv4_addr\n    flags timeout\n  }\n", n)
	}
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type filter hook prerouting priority mangle; policy accept;\n")
	for _, l := range rules {
		b.WriteString(l + "\n")
	}
	b.WriteString("  }\n}\n")
	return b.String(), nil
}

// RenderIpRules 为每条 Enabled 且 action=route 的规则返回字符串
// ip rule add fwmark <mark> lookup <table> priority <prio>，
// 稳定有序。
func RenderIpRules(rs []DomainRule) []string {
	var enabled []DomainRule
	for _, r := range rs {
		if r.Enabled && r.Action == "route" {
			enabled = append(enabled, r)
		}
	}

	// 按 Priority 升序，同优先级按 Domain 字典序排序
	sort.Slice(enabled, func(i, j int) bool {
		if enabled[i].Priority != enabled[j].Priority {
			return enabled[i].Priority < enabled[j].Priority
		}
		return enabled[i].Domain < enabled[j].Domain
	})

	var out []string
	for _, r := range enabled {
		mark := r.Fwmark
		if mark == 0 {
			mark = 100 // 默认从 100 开始，实际由 AssignMarks 分配
		}
		out = append(out, fmt.Sprintf("ip rule add fwmark %d lookup %d priority %d", mark, r.Table, r.Priority))
	}
	return out
}

// AssignMarks 对 Fwmark 为 0 的规则复制并赋 100+下标（下标为在传入切片中的位角），
// 返回新切片，不修改入参。分配后确保 Fwmark 单调递增。
//
// 重要：仅对原本 Fwmark=0 的规则进行单调递增调整，保留原本有明确 Fwmark 值的规则不被修改。
func AssignMarks(rs []DomainRule) []DomainRule {
	// 先深拷贝
	result := make([]DomainRule, len(rs))
	copy(result, rs)

	// 自动分配从 100 起，逐个跳过已被手工占用的值。
	// 只要 mark 全局唯一，nft 与 ip rule 就不会串；不追求"整体单调"——
	// 用户手填 50 的规则被强行抬到 100 以上会改掉他刻意的编号。
	used := make(map[int]bool, len(rs))
	for _, r := range rs {
		if r.Fwmark != 0 {
			used[r.Fwmark] = true
		}
	}
	next := 100
	for i := range result {
		if result[i].Fwmark != 0 {
			continue
		}
		for used[next] {
			next++
		}
		result[i].Fwmark = next
		used[next] = true
		next++
	}

	return result
}
