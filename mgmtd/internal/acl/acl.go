package acl

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// NftName is the nft binary resolved via execx.Lookup at apply time.
const NftName = "nft"

// NftListArgs lists the router_acl table for verification.
var NftListArgs = []string{"list", "table", "inet", "router_acl"}

// Rule is a single forward-chain ACL rule backed by nftables.
type Rule struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	SrcIP   string `json:"src_ip"`
	DstIP   string `json:"dst_ip"`
	SrcMAC  string `json:"src_mac"`
	DstMAC  string `json:"dst_mac"`
	DstPort int    `json:"dst_port"`
	Proto   string `json:"proto"`
	Action  string `json:"action"`
	Enabled bool   `json:"enabled"`
}

var validProtos = map[string]bool{"tcp": true, "udp": true, "icmp": true, "any": true}
var validActions = map[string]bool{"accept": true, "drop": true, "reject": true}

// Validate checks the structural correctness of a rule set. Empty slices
// are allowed (an empty rule set means "allow all").
func Validate(rs []Rule) error {
	for i, r := range rs {
		proto := strings.ToLower(strings.TrimSpace(r.Proto))
		if !validProtos[proto] {
			return fmt.Errorf("rule[%d] invalid proto %q: must be tcp/udp/icmp/any", i, r.Proto)
		}
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if !validActions[action] {
			return fmt.Errorf("rule[%d] invalid action %q: must be accept/drop/reject", i, r.Action)
		}
		if r.DstPort < 0 || r.DstPort > 65535 {
			return fmt.Errorf("rule[%d] dst_port %d out of range 0-65535", i, r.DstPort)
		}
		if r.SrcIP != "" && !isValidNet(r.SrcIP) {
			return fmt.Errorf("rule[%d] invalid src_ip %q: must be IP or CIDR", i, r.SrcIP)
		}
		if r.DstIP != "" && !isValidNet(r.DstIP) {
			return fmt.Errorf("rule[%d] invalid dst_ip %q: must be IP or CIDR", i, r.DstIP)
		}
		if r.SrcMAC != "" {
			if _, err := net.ParseMAC(r.SrcMAC); err != nil {
				return fmt.Errorf("rule[%d] invalid src_mac %q", i, r.SrcMAC)
			}
		}
		if r.DstMAC != "" {
			if _, err := net.ParseMAC(r.DstMAC); err != nil {
				return fmt.Errorf("rule[%d] invalid dst_mac %q", i, r.DstMAC)
			}
		}
	}
	return nil
}

func isValidNet(s string) bool {
	if ip := net.ParseIP(s); ip != nil {
		return true
	}
	_, _, err := net.ParseCIDR(s)
	return err == nil
}

// RenderNft renders a rule set to an nftables ruleset fragment. The chain
// declaration is MULTILINE (closing brace on its own line) to satisfy the
// nft parser. A final unconditional `accept` rule is appended so an
// empty/misconfigured list cannot lock out the operator.
func RenderNft(rs []Rule) string {
	var b bytes.Buffer
	// 绝不写 flush ruleset：Apply() 只删 table inet router_acl，全量 flush 会
	// 连带清掉 firewall/dpi/portal/portmap 等所有表，等于把整台机器的规则集
	// 干掉。也不需要任何占位 set —— 空集合字面量 {} 本身在 nft 里就是语法错误
	// （实测报 `syntax error, unexpected '}'``）。
	b.WriteString("table inet router_acl {\n")
	b.WriteString("    chain forward {\n")
	b.WriteString("        type filter hook forward priority filter;\n")
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		b.WriteString("        ")
		b.WriteString(renderRule(r))
		b.WriteString("\n")
	}
	b.WriteString("        accept\n") // safety net: never drop-all (avoids self-lock)
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

// renderRule renders a single rule's match+action, omitting empty fields.
func renderRule(r Rule) string {
	var parts []string
	if r.SrcIP != "" {
		if _, cidr, err := net.ParseCIDR(r.SrcIP); err == nil && cidr != nil {
			parts = append(parts, "ip saddr "+cidr.String())
		} else {
			parts = append(parts, "ip saddr "+net.ParseIP(r.SrcIP).String())
		}
	}
	if r.DstIP != "" {
		if _, cidr, err := net.ParseCIDR(r.DstIP); err == nil && cidr != nil {
			parts = append(parts, "ip daddr "+cidr.String())
		} else {
			parts = append(parts, "ip daddr "+net.ParseIP(r.DstIP).String())
		}
	}
	proto := strings.ToLower(strings.TrimSpace(r.Proto))
	action := strings.ToLower(strings.TrimSpace(r.Action))
	if proto != "" && proto != "any" {
		parts = append(parts, proto)
	}
	if r.DstPort > 0 {
		parts = append(parts, "dport "+strconv.Itoa(r.DstPort))
	}
	if r.SrcMAC != "" {
		parts = append(parts, "ether saddr "+r.SrcMAC)
	}
	if r.DstMAC != "" {
		parts = append(parts, "ether daddr "+r.DstMAC)
	}
	parts = append(parts, action)
	return strings.Join(parts, " ")
}

// Apply validates then atomically replaces the nft ruleset via stdin.
// `nft delete table inet router_acl` is run first (errors ignored: table may
// not exist on a fresh apply).
func Apply(r execx.Runner, rs []Rule) error {
	if err := Validate(rs); err != nil {
		return err
	}
	// Drop the existing table (ignore "table not found" / no-such errors).
	del := "nft delete table inet router_acl 2>/dev/null || true\n"
	_, _ = r.RunWithInput("nft", "nft", del)
	script := RenderNft(rs)
	applyOut, applyErr := r.RunWithInput(script, "nft", "-f", "-")
	if applyErr != nil {
		return fmt.Errorf("nft apply failed: %v: %s", applyErr, applyOut)
	}
	return nil
}
