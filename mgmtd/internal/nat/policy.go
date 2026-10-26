package nat

import (
	"fmt"
	"strings"
)

// NATRule is a single NAT rule.
// Type is either "masquerade" (outbound source NAT) or "portforward"
// (destination NAT / port mapping).
type NATRule struct {
	Type      string `json:"type"`
	Interface string `json:"interface"`
	Proto     string `json:"proto"`
	SrcPort   string `json:"src_port"`
	DstAddr   string `json:"dst_addr"`
	DstPort   string `json:"dst_port"`
	ToPort    string `json:"to_port"`
}

// RenderNftables renders the rule list into a complete nftables NAT ruleset.
// - masquerade rules go in the postrouting chain (source NAT / SNAT).
// - portforward rules go in the prerouting chain as dnat to (destination NAT).
// Declares each chain exactly once; the output is idempotent when the old
// table is deleted first.
func RenderNftables(rules []NATRule) string {
	var b strings.Builder
	b.WriteString("table ip nat {\n")

	// Group rules by target chain, preserving first-seen order.
	masq := []NATRule{}
	dnat := []NATRule{}
	for _, r := range rules {
		switch r.Type {
		case "portforward":
			dnat = append(dnat, r)
		default: // "masquerade" (and anything unrecognized) → SNAT
			masq = append(masq, r)
		}
	}

	// postrouting chain (masquerade / SNAT)
	b.WriteString("  chain postrouting {\n")
	b.WriteString("    type nat hook postrouting priority srcnat; policy accept;\n")
	for _, r := range masq {
		parts := []string{}
		if r.Interface != "" {
			parts = append(parts, fmt.Sprintf("oifname %q", r.Interface))
		}
		if r.Proto != "" && r.SrcPort != "" {
			parts = append(parts, fmt.Sprintf("%s sport %s", r.Proto, r.SrcPort))
		}
		cond := ""
		if len(parts) > 0 {
			cond = strings.Join(parts, " ") + " "
		}
		b.WriteString("    " + cond + "masquerade\n")
	}
	b.WriteString("  }\n")

	// prerouting chain (dnat / port forward)
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type nat hook prerouting priority dstnat; policy accept;\n")
	for _, r := range dnat {
		parts := []string{}
		if r.Proto != "" {
			parts = append(parts, r.Proto)
		}
		if r.SrcPort != "" {
			parts = append(parts, fmt.Sprintf("dport %s", r.SrcPort))
		}
		cond := ""
		if len(parts) > 0 {
			cond = strings.Join(parts, " ") + " "
		}
		dnatTo := fmt.Sprintf("dnat to %s", r.DstAddr)
		if r.ToPort != "" && r.ToPort != r.SrcPort {
			dnatTo += fmt.Sprintf(":%s", r.ToPort)
		}
		b.WriteString("    " + cond + dnatTo + "\n")
	}
	b.WriteString("  }\n")

	b.WriteString("}\n")
	return b.String()
}
