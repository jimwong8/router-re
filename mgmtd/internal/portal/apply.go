package portal

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// RenderNft generates nftables text for the portal authorization rules.
// The output is designed to be piped via nft -f -.
func RenderNft(clients []Client, portalPort int) string {
	var b strings.Builder
	// NOTE: never emit `flush ruleset` here. Apply() already deletes only
	// `table inet portal`; a full flush would wipe every other table on the
	// box (firewall, dpi, client-qos) and take the router's ruleset offline.
	b.WriteString("table inet portal {\n")
	b.WriteString("\tset authorized {\n")
	b.WriteString("\t\ttype ether_addr;\n")
	b.WriteString("\t\telements = { ")
	first := true
	for _, c := range clients {
		if !c.Authorized {
			continue
		}
		mac := NormalizeMAC(c.MAC)
		// 渲染层兜底：NormalizeMAC 只做小写+去空白，不做格式校验。
		// 原样拼进 elements = { ... } 后，一个带 "\n}" 的 MAC 就能提前闭合
		// 集合并追加任意 nft 规则。这里用 ParseMAC 规整成标准形态，
		// 解析失败的直接跳过。
		parsed, err := net.ParseMAC(mac)
		if err != nil {
			continue
		}
		if first {
			first = false
		} else {
			b.WriteString(", ")
		}
		b.WriteString(parsed.String())
	}
	b.WriteString(" }\n")
	b.WriteString("\t}\n")
	b.WriteString("\tchain forward {\n")
	b.WriteString("\t\ttype filter hook forward priority filter;\n")
	b.WriteString("\t\tether saddr @authorized accept\n")
	b.WriteString("\t\tudp dport 53 accept\n")
	b.WriteString(fmt.Sprintf("\t\ttcp dport %d accept\n", portalPort))
	b.WriteString("\t\tip protocol icmp accept\n")
	b.WriteString("\t\tdrop\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// Apply writes portal nft rules to the host nftables subsystem.
func Apply(r execx.Runner, clients []Client, portalPort int) error {
	ip, err := execx.Lookup("nft")
	if err != nil {
		return fmt.Errorf("nft command not found: %w", err)
	}
	// Delete any existing table first (ignore missing errors).
	_, _ = r.Run(ip, "delete", "table", "inet", "portal")
	// Apply new rules via stdin (-f - consumes stdin).
	_, err = r.RunWithInput(RenderNft(clients, portalPort), ip, "-f", "-")
	if err != nil {
		return fmt.Errorf("nft apply failed: %w", err)
	}
	return nil
}
