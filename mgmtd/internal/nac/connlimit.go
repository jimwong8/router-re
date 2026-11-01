package nac

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Limit is a per-IP connection limit.
type Limit struct {
	Name      string `json:"name"`
	SrcIP     string `json:"src_ip"`
	MaxConns  int    `json:"max_conns"`
	MaxPerSec int    `json:"max_per_sec"`
	Enabled   bool   `json:"enabled"`
}

// ValidateLimit checks a connection limit rule.
func ValidateLimit(l Limit) error {
	if strings.TrimSpace(l.Name) == "" {
		return fmt.Errorf("name required")
	}
	// Name 会被拼进 nft 脚本的注释行（"# <name> ..."）。若放任换行/控制字符，
	// 一个形如 "x\nflush ruleset" 的名字就能在 nft -f 时注入任意规则——
	// 直接把防火墙清空，路由器对 WAN 全面敞开。注释不是安全边界，校验才是。
	if len(l.Name) > 64 {
		return fmt.Errorf("name too long (max 64), got %d", len(l.Name))
	}
	for _, r := range l.Name {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("name contains illegal control character")
		}
	}
	if _, _, err := net.ParseCIDR(strings.TrimSpace(l.SrcIP)); err != nil {
		if net.ParseIP(strings.TrimSpace(l.SrcIP)) == nil {
			return fmt.Errorf("src_ip must be an IP or CIDR, got %q", l.SrcIP)
		}
	}
	if l.MaxConns < 1 || l.MaxConns > 10000 {
		return fmt.Errorf("max_conns must be in [1,10000], got %d", l.MaxConns)
	}
	if l.MaxPerSec < 0 || l.MaxPerSec > 10000 {
		return fmt.Errorf("max_per_sec must be in [0,10000], got %d", l.MaxPerSec)
	}
	return nil
}

// RenderConnlimit emits the router_connlimit table. Uses ct count over <n>
// (the correct connlimit syntax) plus a rate limit. The forward chain never
// sets policy drop (default accept); only matched flows are dropped. Never
// flushes the ruleset.
func RenderConnlimit(ls []Limit) string {
	var b strings.Builder
	b.WriteString("table inet router_connlimit {\n")
	b.WriteString("\tchain forward {\n")
	b.WriteString("\t\ttype filter hook forward priority filter;\n")
	b.WriteString("\t\t# connlimit: default accept; only matched flows are dropped.\n")
	b.WriteString("\t\tct state established,related accept\n")
	for _, l := range ls {
		if !l.Enabled {
			continue
		}
		// per-IP connection count
		b.WriteString("\t\t# " + l.Name + " " + l.SrcIP + " max " + fmt.Sprintf("%d", l.MaxConns) + " conns\n")
		b.WriteString("\t\tip saddr " + l.SrcIP + " ct count over " + fmt.Sprintf("%d", l.MaxConns) + " drop\n")
		if l.MaxPerSec > 0 {
			b.WriteString("\t\tip saddr " + l.SrcIP + " limit rate " + fmt.Sprintf("%d", l.MaxPerSec) + "/second burst " + fmt.Sprintf("%d", l.MaxPerSec*2) + " packets drop\n")
		}
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

// Apply validates, replaces the router_connlimit table and confirms via nft list.
func Apply(r execx.Runner, ls []Limit) error {
	for _, l := range ls {
		if err := ValidateLimit(l); err != nil {
			return err
		}
	}
	if _, err := r.Run("nft", "delete", "table", "inet", "router_connlimit"); err != nil {
		_ = err
	}
	nft := RenderConnlimit(ls)
	if _, err := r.RunWithInput(nft, "nft", "-f", "-"); err != nil {
		return err
	}
	return nil
}

// TopConnections returns the source IPs with the most established connections.
func TopConnections(r execx.Runner) (map[string]int, error) {
	out, err := r.Run("ss", "-Htn", "state", "established")
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ss -Htn output: "ESTAB 0 0 127.0.0.1:443 127.0.0.1:54321"
		fields := strings.Fields(line)
		// find the peer address field (after the local address)
		peer := ""
		for _, f := range fields {
			if strings.Contains(f, ":") {
				host, _, err := net.SplitHostPort(f)
				if err == nil {
					peer = host
				}
			}
		}
		if peer != "" {
			counts[peer]++
		}
	}
	return counts, nil
}

// CountByIface returns the number of established connections per source IP.
func CountByIface(r execx.Runner) (map[string]int, error) {
	return TopConnections(r)
}
