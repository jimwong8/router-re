package firewall

import (
	"fmt"
	"strings"
)

// RenderNftables renders the rule list into a complete nftables ruleset.
// Each chain is declared exactly once (hook attributes + its rules), so the
// output can be applied idempotently after the old table is deleted.
func RenderNftables(rules []Rule) string {
	var b strings.Builder
	b.WriteString("table inet router {\n")

	// Group rules by chain, preserving first-seen order.
	chainRules := map[string][]Rule{}
	var order []string
	for _, r := range rules {
		ch := r.Chain
		if ch == "" {
			ch = "forward"
		}
		if _, ok := chainRules[ch]; !ok {
			chainRules[ch] = []Rule{}
			order = append(order, ch)
		}
		chainRules[ch] = append(chainRules[ch], r)
	}

	// Always declare the three base chains so the hooks exist even when empty.
	for _, ch := range []string{"input", "forward", "output"} {
		if _, ok := chainRules[ch]; !ok {
			chainRules[ch] = []Rule{}
			order = append(order, ch)
		}
	}

	for _, ch := range order {
		// 链名同样只允许标识符：换行会把 chain 声明拆成两行，后面那段
		// 就成了自由指令（例如 "forward\nflush ruleset"）。
		if !chainRe.MatchString(ch) {
			continue
		}
		hook := ""
		switch ch {
		case "input":
			hook = "type filter hook input priority 0; policy accept;"
		case "forward":
			hook = "type filter hook forward priority 0; policy accept;"
		case "output":
			hook = "type filter hook output priority 0; policy accept;"
		}
		if hook == "" {
			// Custom chain: no hook, just a rule list.
			b.WriteString(fmt.Sprintf("  chain %s {\n", ch))
		} else {
			b.WriteString(fmt.Sprintf("  chain %s {\n    %s\n", ch, hook))
		}
		for _, r := range chainRules[ch] {
			parts := renderMatch(r)
			// 动作必须是白名单里的终结符。否则 "accept\nflush ruleset" 这类值
			// 会直接写成两行——渲染层不能假设调用方一定先跑过 Validate。
			if !actionAllow[r.Action] {
				continue
			}
			cond := ""
			if len(parts) > 0 {
				cond = strings.Join(parts, " ") + " "
			}
			b.WriteString("    " + cond + r.Action + "\n")
		}
		b.WriteString("  }\n")
	}

	b.WriteString("}\n")
	return b.String()
}

// renderMatch builds the match expression for a single rule.
//
// 每个字段都先过白名单：这里是渲染层，不能假设调用方一定跑了 Validate。
// 任何一个值带换行/分号/空格都会让一行规则变成多条指令。
func renderMatch(r Rule) []string {
	var parts []string
	if r.Src != "" && addrRe.MatchString(r.Src) {
		parts = append(parts, fmt.Sprintf("ip saddr %s", r.Src))
	}
	if r.Dst != "" && addrRe.MatchString(r.Dst) {
		parts = append(parts, fmt.Sprintf("ip daddr %s", r.Dst))
	}
	if r.Proto != "" && protoAllow[strings.ToLower(r.Proto)] {
		if r.DPort != "" && portRe.MatchString(r.DPort) {
			parts = append(parts, fmt.Sprintf("%s dport %s", strings.ToLower(r.Proto), r.DPort))
		} else {
			parts = append(parts, strings.ToLower(r.Proto))
		}
	}
	return parts
}
