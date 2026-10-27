package portmap

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Rule 描述一条端口映射规则（DNAT 到内网主机）。
type Rule struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Proto   string `json:"proto"`    // tcp 或 udp
	ExtPort int    `json:"ext_port"` // 外部端口
	IntIP   string `json:"int_ip"`   // 内网目标 IP
	IntPort int    `json:"int_port"` // 内网端口
	Enabled bool   `json:"enabled"`
}

// isPrivateIP 判断 IP 是否为常见私网段：10.0.0.0/8、172.16.0.0/12、192.168.0.0/16。
func isPrivateIP(ip net.IP) bool {
	b := ip.To4()
	if b == nil {
		return false
	}
	if b[0] == 10 {
		return true
	}
	if b[0] == 172 && b[1] >= 16 && b[1] <= 31 {
		return true
	}
	if b[0] == 192 && b[1] == 168 {
		return true
	}
	return false
}

// Validate 校验规则列表。
func Validate(rs []Rule) error {
	type key struct {
		proto   string
		extPort int
	}
	seen := make(map[key]bool)
	for _, r := range rs {
		if r.Proto != "tcp" && r.Proto != "udp" {
			return fmt.Errorf("rule %d (name=%q): proto must be tcp or udp, got %q", r.ID, r.Name, r.Proto)
		}
		if r.ExtPort < 1 || r.ExtPort > 65535 {
			return fmt.Errorf("rule %d (name=%q): ext_port %d out of range 1-65535", r.ID, r.Name, r.ExtPort)
		}
		if r.IntPort < 1 || r.IntPort > 65535 {
			return fmt.Errorf("rule %d (name=%q): int_port %d out of range 1-65535", r.ID, r.Name, r.IntPort)
		}
		ip := net.ParseIP(r.IntIP)
		if ip == nil {
			return fmt.Errorf("rule %d (name=%q): invalid int_ip %q", r.ID, r.Name, r.IntIP)
		}
		if !isPrivateIP(ip) {
			return fmt.Errorf("rule %d (name=%q): int_ip %q must be a private address (10./172.16-31./192.168.)", r.ID, r.Name, r.IntIP)
		}
		k := key{r.Proto, r.ExtPort}
		if seen[k] {
			return fmt.Errorf("rule %d (name=%q): duplicate proto %s + ext_port %d", r.ID, r.Name, r.Proto, r.ExtPort)
		}
		seen[k] = true
	}
	return nil
}

// RenderNft 生成端口映射的 nftables 规则文本。
// 三个 chain，每个声明必须多行（chain 名单独一行、type hook 单独一行）。
func RenderNft(rs []Rule) string {
	var b strings.Builder
	b.WriteString("table inet router_portmap {\n")

	// prerouting: DNAT
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type nat hook prerouting priority dstnat;\n")
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		// inet 表同时承载 ipv4/ipv6，必须写 `dnat ip to` 消歧义，
// 否则 nft 报 "specify `dnat ip' or `dnat ip6' in inet table to disambiguate"。
		fmt.Fprintf(&b, "    %s dport %d dnat ip to %s:%d\n", r.Proto, r.ExtPort, r.IntIP, r.IntPort)
	}
	b.WriteString("  }\n")

	// forward: 允许转发到内网目标
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority filter;\n")
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		fmt.Fprintf(&b, "    ip daddr %s %s dport %d accept\n", r.IntIP, r.Proto, r.IntPort)
	}
	b.WriteString("  }\n")

	// postrouting: hairpin masquerade（覆盖常见私网段）
	b.WriteString("  chain postrouting {\n")
	b.WriteString("    type nat hook postrouting priority srcnat;\n")
	b.WriteString("    # hairpin NAT: 只覆盖常见私网段，可按需补 10.0.0.0/8\n")
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		fmt.Fprintf(&b, "    ip saddr 192.168.0.0/16 ip daddr %s %s dport %d masquerade\n", r.IntIP, r.Proto, r.IntPort)
	}
	b.WriteString("  }\n")

	b.WriteString("}\n")
	return b.String()
}

// Apply 校验并下发端口映射规则。
func Apply(r execx.Runner, rs []Rule) error {
	if err := Validate(rs); err != nil {
		return err
	}
	// 幂等删除旧 table（不存在时忽略错误）
	if _, err := r.Run("nft", "delete", "table", "inet", "router_portmap"); err != nil {
		_ = err // 表不存在或删除失败都继续（先删再建，管理面可重复调用）
	}
	if _, err := r.RunWithInput(RenderNft(rs), "nft", "-f", "-"); err != nil {
		return fmt.Errorf("nft -f - (portmap): %w", err)
	}
	return nil
}
