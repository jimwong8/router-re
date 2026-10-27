package lb

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// RenderNftMarkSet emits the nft mark table. Never flushes the ruleset and
// never sets a policy drop — marks are just labels consumed by the routing
// table (fwmark → lookup).
func RenderNftMarkSet(marks map[string]int) string {
	var b strings.Builder
	b.WriteString("table inet router_lb_marks {\n")
	b.WriteString("\tchain prerouting {\n")
	b.WriteString("\t\ttype nat hook prerouting priority filter;\n")
	for name, v := range marks {
		fmt.Fprintf(&b, "\t\t# mark %d = %s\n", v, name)
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// RenderIpRuleScript emits `ip rule add fwmark <mark> lookup <table>` lines
// for each member, one per line with a comment.
func RenderIpRuleScript(c Config) string {
	var b strings.Builder
	for _, m := range c.Members {
		mark := memberMark(m.Name)
		fmt.Fprintf(&b, "# %s (%s gw %s): fwmark %d -> lookup %d\n", m.Name, m.Iface, m.Gateway, mark, mark)
		fmt.Fprintf(&b, "ip rule add fwmark %d lookup %d\n", mark, mark)
	}
	return b.String()
}

// ApplyMarkSet validates and applies the mark table via nft -f -.
func ApplyMarkSet(r execx.Runner, marks map[string]int) error {
	if _, err := r.Run("nft", "delete", "table", "inet", "router_lb_marks"); err != nil {
		_ = err
	}
	nft := RenderNftMarkSet(marks)
	if _, err := r.RunWithInput(nft, "nft", "-f", "-"); err != nil {
		return err
	}
	return nil
}

// ApplyIpRules runs the ip rule script line by line.
func ApplyIpRules(r execx.Runner, c Config) error {
	script := RenderIpRuleScript(c)
	for _, line := range strings.Split(script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if _, err := r.Run(fields[0], fields[1:]...); err != nil {
			return fmt.Errorf("ip rule: %v", err)
		}
	}
	return nil
}

// memberMark maps a member name to a deterministic mark value (1..255).
func memberMark(name string) int {
	s := 0
	for _, ch := range name {
		s = (s*31 + int(ch)) % 255
	}
	if s < 1 {
		s = 1
	}
	return s
}

// MemberMark is the exported form of memberMark.
var MemberMark = memberMark
