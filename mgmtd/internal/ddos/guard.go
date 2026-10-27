// Package ddos 提供 DDoS 防护的 nft 规则渲染与白名单匹配。
package ddos

import (
	"fmt"
	"net"
	"strings"
)

// Threshold 是一类流量的限速阈值。
type Threshold struct {
	Pps       int    `json:"pps"`
	Bps       int    `json:"bps"`
	Burst     int    `json:"burst"`
	Action    string `json:"action"`
	LogPrefix string `json:"log_prefix"`
}

// Config 是整个 DDoS 防护配置。
type Config struct {
	SynFlood       Threshold `json:"syn_flood"`
	IcmpFlood      Threshold `json:"icmp_flood"`
	UdpFlood       Threshold `json:"udp_flood"`
	AckFlood       Threshold `json:"ack_flood"`
	PingOfDeath    bool      `json:"ping_of_death"`
	PortScanPps    int       `json:"port_scan_pps"`
	PortScanWindow int       `json:"port_scan_window"`
	BlockSeconds   int       `json:"block_seconds"`
	Whitelist      []string  `json:"whitelist"`
}

var validActions = map[string]bool{"drop": true, "log": true, "ratelimit": true}

// Validate 校验配置。
func Validate(c Config) error {
	for name, th := range map[string]Threshold{
		"syn_flood": c.SynFlood, "icmp_flood": c.IcmpFlood,
		"udp_flood": c.UdpFlood, "ack_flood": c.AckFlood,
	} {
		if th.Pps < 0 || th.Bps < 0 || th.Burst < 0 {
			return fmt.Errorf("%s: pps/bps/burst must be >= 0", name)
		}
		if th.Action != "" && !validActions[th.Action] {
			return fmt.Errorf("%s: action must be drop/log/ratelimit, got %q", name, th.Action)
		}
	}
	if c.PortScanPps != 0 && (c.PortScanPps < 1 || c.PortScanPps > 100000) {
		return fmt.Errorf("port_scan_pps must be 0 or 1-100000, got %d", c.PortScanPps)
	}
	if c.PortScanWindow < 1 || c.PortScanWindow > 3600 {
		return fmt.Errorf("port_scan_window must be 1-3600, got %d", c.PortScanWindow)
	}
	if c.BlockSeconds < 0 || c.BlockSeconds > 86400 {
		return fmt.Errorf("block_seconds must be 0-86400, got %d", c.BlockSeconds)
	}
	for _, w := range c.Whitelist {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if w == "0.0.0.0/0" {
			return fmt.Errorf("whitelist must not contain 0.0.0.0/0: that disables the entire protection")
		}
		if net.ParseIP(w) == nil {
			if _, _, err := net.ParseCIDR(w); err != nil {
				return fmt.Errorf("whitelist entry %q is not a valid IP or CIDR", w)
			}
		}
	}
	return nil
}

// Render 生成 nft 规则。
//
// 铁律：
//   - 绝不出现 `policy drop`（默认策略必须 accept，只对超限流量 drop）
//   - 绝不出现 `elements = {}`（空集合语法错误）
//   - 所有集合名带前缀 ddos_
//
// 用 meter/limit 实现，数字来自配置。
func Render(c Config) string {
	var b strings.Builder
	b.WriteString("table inet ddos_guard {\n")
	// 白名单集合
	b.WriteString("  set ddos_whitelist {\n")
	b.WriteString("    type ipv4_addr\n")
	b.WriteString("    flags interval\n")
	if len(c.Whitelist) > 0 {
		elems := make([]string, 0, len(c.Whitelist))
		for _, w := range c.Whitelist {
			w = strings.TrimSpace(w)
			if w != "" {
				elems = append(elems, w)
			}
		}
		if len(elems) > 0 {
			b.WriteString("    elements = { " + strings.Join(elems, ", ") + " }\n")
		}
	}
	b.WriteString("  }\n")
	b.WriteString("  chain ddos_guard {\n")
	b.WriteString("    type filter hook prerouting priority -300; policy accept;\n")
	b.WriteString("    ip saddr @ddos_whitelist accept\n")
	// syn flood
	if c.SynFlood.Pps > 0 {
		fmt.Fprintf(&b, "    tcp flags & (fin|syn|rst|ack) == syn limit rate over %d/second drop\n", c.SynFlood.Pps)
	}
	if c.IcmpFlood.Pps > 0 {
		fmt.Fprintf(&b, "    icmp type echo-request limit rate over %d/second drop\n", c.IcmpFlood.Pps)
	}
	if c.UdpFlood.Pps > 0 {
		fmt.Fprintf(&b, "    udp limit rate over %d/second drop\n", c.UdpFlood.Pps)
	}
	if c.AckFlood.Pps > 0 {
		fmt.Fprintf(&b, "    tcp flags & (fin|syn|rst|ack) == ack limit rate over %d/second drop\n", c.AckFlood.Pps)
	}
	if c.PingOfDeath {
		b.WriteString("    ip protocol icmp icmp type echo-request ip length > 1024 drop\n")
	}
	if c.PortScanPps > 0 {
		fmt.Fprintf(&b, "    # port scan threshold: %d pps over %ds window\n", c.PortScanPps, c.PortScanWindow)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

// IsWhitelisted 支持精确 IP 和 CIDR 匹配；非法输入返回 false。
func IsWhitelisted(ip string, whitelist []string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, w := range whitelist {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if net.ParseIP(w) != nil {
			if parsed.Equal(net.ParseIP(w)) {
				return true
			}
			continue
		}
		if _, cidr, err := net.ParseCIDR(w); err == nil {
			if cidr.Contains(parsed) {
				return true
			}
		}
	}
	return false
}
