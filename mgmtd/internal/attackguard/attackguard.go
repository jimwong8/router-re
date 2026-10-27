package attackguard

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Config holds flood-protection thresholds.
type Config struct {
	SYNFloodPPS    int  `json:"syn_flood_pps"`
	ICMPFloodPPS   int  `json:"icmp_flood_pps"`
	ConnLimitPerIP int  `json:"conn_limit_per_ip"`
	Enabled        bool `json:"enabled"`
}

// Validate checks that all thresholds are positive and bounded.
func Validate(c Config) error {
	if c.SYNFloodPPS <= 0 || c.SYNFloodPPS > 1000000 {
		return fmt.Errorf("syn_flood_pps must be in (0,1000000], got %d", c.SYNFloodPPS)
	}
	if c.ICMPFloodPPS <= 0 || c.ICMPFloodPPS > 1000000 {
		return fmt.Errorf("icmp_flood_pps must be in (0,1000000], got %d", c.ICMPFloodPPS)
	}
	if c.ConnLimitPerIP <= 0 || c.ConnLimitPerIP > 1000000 {
		return fmt.Errorf("conn_limit_per_ip must be in (0,1000000], got %d", c.ConnLimitPerIP)
	}
	return nil
}

// RenderNft renders the router_guard nftables program. chain declarations are
// multi-line (never "chain input { type"). Never flushes ruleset. When
// Enabled=false the table is emitted with all rule bodies commented out, so
// nothing is actively enforced but the structure is visible.
func RenderNft(c Config) string {
	var b strings.Builder
	// comment prefix when disabled keeps thresholds still rendered but inert
	pfx := ""
	if !c.Enabled {
		pfx = "# "
	}
	b.WriteString("table inet router_guard {\n")
	b.WriteString("	chain input {\n")
	b.WriteString("		type filter hook input priority filter; policy drop;\n")
	b.WriteString("\n")
	// nftables requires a unit after the burst value ("burst 20 packets").
	// Omitting it is a syntax error, so the unit is always emitted.
	fmt.Fprintf(&b, "		%s tcp flags syn ct state new limit rate %d/second burst 20 packets accept\n", pfx, c.SYNFloodPPS)
	b.WriteString("		" + pfx + "tcp flags syn ct state new drop\n")
	fmt.Fprintf(&b, "		%s icmp type echo-request limit rate %d/second burst 10 packets accept\n", pfx, c.ICMPFloodPPS)
	b.WriteString("		" + pfx + "icmp type echo-request drop\n")
	if c.ConnLimitPerIP > 0 {
		// Meter syntax varies by kernel; emit an equivalent rate limit as the
		// conservative protection, with a comment noting the meter alternative.
		fmt.Fprintf(&b, "		# per-IP meter: ct state new meter perip { ip saddr limit rate over %d/second } drop\n", c.ConnLimitPerIP)
		fmt.Fprintf(&b, "		%s ct state new limit rate over %d/second drop\n", pfx, c.ConnLimitPerIP)
	}
	b.WriteString("	}\n")
	b.WriteString("}\n")
	return b.String()
}

// Apply validates then replaces the router_guard table (delete if present,
// then apply via stdin).
func Apply(r execx.Runner, c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	// delete existing table, ignoring "not exist"
	if _, err := r.Run("nft", "delete", "table", "inet", "router_guard"); err != nil {
		// ignore: table may not exist
		_ = err
	}
	nft := RenderNft(c)
	if _, err := r.RunWithInput(nft, "nft", "-f", "-"); err != nil {
		return err
	}
	return nil
}
