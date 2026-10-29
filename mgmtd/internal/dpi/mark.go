package dpi

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// MarkRule describes an application-marking rule: match by protocol+dports,
// then either set an fwmark (for tc shaping) or drop the packet.
type MarkRule struct {
	App      string `json:"app"`      // 应用名，如 "netflix"
	Protocol string `json:"protocol"` // "tcp" / "udp"
	DPorts   []int  `json:"dports"`   // 目标端口列表，可为空
	Mark     int    `json:"mark"`     // fwmark 值（mark/drop 动作使用）
	RateMbit int    `json:"rate_mbit"` // limit 时速率 Mbit
	RateMbps int    `json:"rate_mbps"` // alias
	Action   string `json:"action"`   // "mark" / "drop" / "limit"
}

func (r MarkRule) validate() error {
	if r.Protocol != "" && r.Protocol != "tcp" && r.Protocol != "udp" {
		return fmt.Errorf("invalid protocol %q (want tcp|udp)", r.Protocol)
	}
	if r.Action != "mark" && r.Action != "drop" && r.Action != "limit" {
		return fmt.Errorf("invalid action %q (mark|drop|limit)", r.Action)
	}
	if r.Action == "limit" && r.RateMbit <= 0 && r.RateMbps <= 0 {
		return fmt.Errorf("limit requires rate > 0")
	}
	return nil
}

// RenderMarkNft renders the mark rules into nftables text under
// table inet router_dpi. Returns an error if any rule is invalid.
func RenderMarkNft(rules []MarkRule) (string, error) {
	for _, r := range rules {
		if err := r.validate(); err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString("table inet router_dpi {\n")
	// The chain body must be multi-line: writing
	//   `chain classify { type filter hook forward priority mangle; }`
	// closes the chain on the same line, so every following rule lands at
	// the table's top level and nft rejects it ("syntax error, unexpected tcp").
	b.WriteString("  chain classify {\n")
	b.WriteString("    type filter hook forward priority mangle;\n")

	for _, r := range rules {
		var match string
		// nftables rejects `meta l4proto tcp tcp dport {...}` — the protocol
		// keyword already implies the L4 match. Use either `<proto> dport {...}`
		// or a bare `meta l4proto <proto>`, never both.
		switch {
		case len(r.DPorts) > 0 && (r.Protocol == "tcp" || r.Protocol == "udp"):
			parts := make([]string, 0, len(r.DPorts))
			for _, p := range r.DPorts {
				parts = append(parts, strconv.Itoa(p))
			}
			match = r.Protocol + " dport { " + strings.Join(parts, ", ") + " }"
		case r.Protocol != "":
			match = "meta l4proto " + r.Protocol
		}
		b.WriteString("    ")
		if match != "" {
			b.WriteString(match)
			b.WriteString(" ")
		}
		switch r.Action {
		case "drop":
			b.WriteString("drop\n")
		case "limit":
			rate := r.RateMbit
			if rate <= 0 { rate = r.RateMbps }
			if rate <= 0 { rate = 10 }
			b.WriteString(fmt.Sprintf("limit rate %d mbytes/second\n", rate))
		default: // mark
			if r.Mark > 0 {
				b.WriteString(fmt.Sprintf("meta mark set %d\n", r.Mark))
			} else {
				b.WriteString("accept\n")
			}
		}
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String(), nil
}

// ApplyMarks applies the mark rules via the runner: first delete the
// router_dpi table (ignore "not exist"), then feed the rendered rules.
func ApplyMarks(r execx.Runner, rules []MarkRule) error {
	if r == nil {
		return errors.New("nil runner")
	}
	// Delete table, ignoring "not exist" error (first apply).
	_, _ = r.Run("nft", "delete", "table", "inet", "router_dpi")
	rendered, err := RenderMarkNft(rules)
	if err != nil {
		return err
	}
	if rendered == "" {
		return nil
	}
	_, err = r.RunWithInput(rendered, "nft", "-f", "-")
	return err
}

// LimitRulesToHTB is implemented in internal/htb/htb.go and converts limit
// rules into an HTB Config (one class per limit rule, rate=ceil=RateMbit,
// prio 5). Use htb.LimitRulesToHTB(rules, iface, rootMbit).

// MarkRulesForTC generates tc filter commands that route marked flows to a
// classid. Each mark rule becomes: tc filter replace dev <dev> parent 1:
// protocol ip handle <mark> fw classid 1:<i>.
func MarkRulesForTC(rules []MarkRule) []string {
	cmds := []string{}
	// Deterministic base interface placeholder — caller replaces <dev>.
	dev := "eth0"
	for i, r := range rules {
		if r.Action != "mark" || r.Mark <= 0 {
			continue
		}
		cmds = append(cmds, fmt.Sprintf("tc filter replace dev %s parent 1: protocol ip handle %d fw classid 1:%d", dev, r.Mark, i+1))
	}
	return cmds
}
