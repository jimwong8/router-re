package policyroute

import (
	"fmt"
	"net"
	"strings"
)

// Rule 是一条策略路由规则：把满足条件的流量导向指定 table。
type Rule struct {
	Priority int    `json:"priority"` // 规则优先级，越小越先匹配
	From     string `json:"from"`     // 源地址/CIDR，可空
	To       string `json:"to"`       // 目的地址/CIDR，可空
	FWMark   int    `json:"fwmark"`   // 防火墙标记，0=不用
	Table    int    `json:"table"`    // 目标表 ID
	Iif      string `json:"iif"`      // 入接口，可空
	Hits     uint64 `json:"hits,omitempty"` // 命中计数
}

// ValidateRule 校验一条路由规则。
//
// 安全红线：
//   - Priority 必须 > 0。ip rule 的 priority 0 属于 local 表，抢占它会导致
//     本机流量（含收包/发包）被重定向，行为异常甚至失联。
//   - From/To/FWMark/Iif **至少一个非空**。全空的规则匹配所有流量，等于
//     劫持默认路由，必须拒绝。
//   - Table 必须 1-252（283/254/255 内核保留）。
func ValidateRule(r Rule) error {
	if r.Priority <= 0 {
		return fmt.Errorf("rule priority 必须 > 0, got %d；priority 0 被内核 local 表占用，抢占会导致本机流量异常失联", r.Priority)
	}
	if r.Table < 1 || r.Table > 252 {
		return fmt.Errorf("rule table 必须在 1-252, got %d（253/254/255 保留），覆盖会使系统路由全乱", r.Table)
	}
	for _, v := range []struct{ what, s string }{
		{"from", r.From}, {"to", r.To},
	} {
		if v.s == "" {
			continue
		}
		if net.ParseIP(v.s) == nil {
			if _, _, err := net.ParseCIDR(v.s); err != nil {
				return fmt.Errorf("rule %s 必须是合法 IP 或 CIDR, got %q", v.what, v.s)
			}
		}
	}
	// 全条件为空 = 匹配所有流量，劫持默认路由，拒绝。
	if strings.TrimSpace(r.From) == "" &&
		strings.TrimSpace(r.To) == "" &&
		r.FWMark == 0 &&
		strings.TrimSpace(r.Iif) == "" {
		return fmt.Errorf("rule 至少需要 from/to/fwmark/iif 之一，全空的规则会匹配所有流量，等于劫持默认路由")
	}
	return nil
}

// RenderRule 生成 `ip rule add ...` 命令，只输出非空字段。
func RenderRule(r Rule) string {
	var b strings.Builder
	b.WriteString("ip rule add")
	if r.Priority > 0 {
		fmt.Fprintf(&b, " priority %d", r.Priority)
	}
	if r.From != "" {
		b.WriteString(" from ")
		b.WriteString(r.From)
	}
	if r.To != "" {
		b.WriteString(" to ")
		b.WriteString(r.To)
	}
	if r.FWMark > 0 {
		fmt.Fprintf(&b, " fwmark %d", r.FWMark)
	}
	if r.Iif != "" {
		b.WriteString(" iif ")
		b.WriteString(r.Iif)
	}
	fmt.Fprintf(&b, " table %d", r.Table)
	return b.String()
}

// RenderTableRoutes 为表内每条路由生成一条
// `ip route add <dst> via <via> dev <dev> metric <m> table <id>`。直连路由
// （Via 为空）不写 via；Dev/Metric 为空同样省略。
func RenderTableRoutes(t Table) []string {
	var out []string
	for _, r := range t.Routes {
		cmd := fmt.Sprintf("ip route add %s", r.Dst)
		if r.Via != "" {
			cmd += " via " + r.Via
		}
		if r.Dev != "" {
			cmd += " dev " + r.Dev
		}
		if r.Metric > 0 {
			cmd += fmt.Sprintf(" metric %d", r.Metric)
		}
		cmd += fmt.Sprintf(" table %d", t.ID)
		out = append(out, cmd)
	}
	return out
}
