package acl

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// MacFilterModeBlacklist / Whitelist.
const (
	MacFilterModeBlacklist = "blacklist"
	MacFilterModeWhitelist = "whitelist"
)

// MacFilter is a source-MAC based access policy.
type MacFilter struct {
	Mode string   `json:"mode"` // blacklist | whitelist
	MACs []string `json:"macs"`
}

// NftMacFilterTable / list args.
const NftMacFilterTable = "mac_filter"
var NftMacFilterListArgs = []string{"list", "table", "inet", NftMacFilterTable}

// ValidateMacFilter checks mode and MAC syntax.
func ValidateMacFilter(cfg MacFilter) error {
	m := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if m != MacFilterModeBlacklist && m != MacFilterModeWhitelist {
		return fmt.Errorf("invalid mode %q: must be blacklist/whitelist", cfg.Mode)
	}
	for i, mc := range cfg.MACs {
		if _, err := net.ParseMAC(mc); err != nil {
			return fmt.Errorf("macs[%d] invalid mac %q", i, mc)
		}
	}
	return nil
}

// RenderMacFilter renders nft ruleset for MAC filter.
func RenderMacFilter(cfg MacFilter) string {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	var b strings.Builder
	b.WriteString("table inet mac_filter {\n")
	b.WriteString("    chain forward {\n")
	b.WriteString("        type filter hook forward priority filter;\n")
	if mode == MacFilterModeBlacklist {
		for _, mc := range cfg.MACs {
			b.WriteString("        ether saddr " + mc + " drop\n")
		}
		b.WriteString("        accept\n")
	} else { // whitelist
		for _, mc := range cfg.MACs {
			b.WriteString("        ether saddr " + mc + " accept\n")
		}
		b.WriteString("        drop\n")
	}
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

// ApplyMacFilter validates then replaces nft table.
func ApplyMacFilter(r execx.Runner, cfg MacFilter) error {
	if err := ValidateMacFilter(cfg); err != nil {
		return err
	}
	del := "nft delete table inet mac_filter 2>/dev/null || true\n"
	_, _ = r.RunWithInput("nft", "nft", del)
	script := RenderMacFilter(cfg)
	out, err := r.RunWithInput(script, "nft", "-f", "-")
	if err != nil {
		return fmt.Errorf("nft apply failed: %v: %s", err, out)
	}
	return nil
}
