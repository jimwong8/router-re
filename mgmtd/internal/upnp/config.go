package upnp

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Config holds the UPnP/IGD + NAT-PMP service settings.
type Config struct {
	Enabled       bool   `json:"enabled"`
	ExternalIface string `json:"external_iface"`
	InternalIface string `json:"internal_iface"`
	AllowCIDR     string `json:"allow_cidr"`
	MaxMappings   int    `json:"max_mappings"`
	LeaseSeconds  int    `json:"lease_seconds"`
	NATPMP        bool   `json:"natpmp"`
}

// DefaultConfPath is where the rendered miniupnpd config is written.
const DefaultConfPath = "/etc/miniupnpd/miniupnpd.conf"

// UpnpTable is the nftables table name for UPnP mappings.
const UpnpTable = "router_upnp"

// Mapping is one port mapping requested through UPnP/NAT-PMP or added by hand.
type Mapping struct {
	Protocol     string `json:"protocol"`
	ExtPort      int    `json:"ext_port"`
	IntIP        string `json:"int_ip"`
	IntPort      int    `json:"int_port"`
	Description  string `json:"description"`
	LeaseSeconds int    `json:"lease_seconds"`
	CreatedAt    int64  `json:"created_at"`
}

// Validate checks the service configuration.
func Validate(c Config) error {
	if strings.TrimSpace(c.ExternalIface) == "" {
		return fmt.Errorf("external_iface is required")
	}
	if strings.TrimSpace(c.InternalIface) == "" {
		return fmt.Errorf("internal_iface is required")
	}
	if c.ExternalIface == c.InternalIface {
		return fmt.Errorf("external_iface and internal_iface must differ, both are %q", c.ExternalIface)
	}
	// 两个接口名都只查了非空，然后原样拼进 ext_ifname= / listening_ip=。
	// "eth0\nextra_option=1" 会在 miniupnpd.conf 里多出一行配置。
	for _, f := range []struct{ name, val string }{
		{"external_iface", c.ExternalIface}, {"internal_iface", c.InternalIface},
	} {
		for _, r := range f.val {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '@' || r == ':'
			if !ok {
				return fmt.Errorf("%s 只能包含字母数字和 _-.:@（发现 %q）", f.name, r)
			}
		}
		if len(f.val) > 15 {
			return fmt.Errorf("%s 名不能超过 15 字节", f.name)
		}
	}
	if strings.TrimSpace(c.AllowCIDR) != "" {
		ip, ipnet, err := net.ParseCIDR(strings.TrimSpace(c.AllowCIDR))
		if err != nil {
			return fmt.Errorf("allow_cidr %q is not a valid CIDR: %v", c.AllowCIDR, err)
		}
		ones, bits := ipnet.Mask.Size()
		// 0.0.0.0/0 (or ::/0) permits every address: allowing mappings from
		// the whole internet would expose internal hosts to anyone, so refuse.
		// blank /0 mask with a non-nil IP is the whole-address-space case.
		if ones == 0 && bits > 0 {
			return fmt.Errorf("allow_cidr %q permits every address: refusing to expose internal mappings to the whole internet", c.AllowCIDR)
		}
		_ = ip
	}
	if c.MaxMappings < 1 || c.MaxMappings > 1000 {
		return fmt.Errorf("max_mappings must be 1-1000, got %d", c.MaxMappings)
	}
	if c.LeaseSeconds < 60 || c.LeaseSeconds > 604800 {
		return fmt.Errorf("lease_seconds must be 60-604800, got %d", c.LeaseSeconds)
	}
	return nil
}

// isPrivateIP reports whether ip is in a private range.
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

// ValidateMapping checks a single mapping against the service limits.
func ValidateMapping(m Mapping, c Config) error {
	if m.Protocol != "tcp" && m.Protocol != "udp" {
		return fmt.Errorf("protocol must be tcp or udp, got %q", m.Protocol)
	}
	if m.ExtPort < 1 || m.ExtPort > 65535 {
		return fmt.Errorf("ext_port %d out of range 1-65535", m.ExtPort)
	}
	if m.IntPort < 1 || m.IntPort > 65535 {
		return fmt.Errorf("int_port %d out of range 1-65535", m.IntPort)
	}
	ip := net.ParseIP(strings.TrimSpace(m.IntIP))
	if ip == nil {
		return fmt.Errorf("int_ip %q is not a valid IP", m.IntIP)
	}
	if !isPrivateIP(ip) {
		return fmt.Errorf("int_ip %q must be a private address (10./172.16-31./192.168.)", m.IntIP)
	}
	if len([]rune(m.Description)) > 64 {
		return fmt.Errorf("description longer than 64 characters")
	}
	if m.LeaseSeconds > c.LeaseSeconds {
		return fmt.Errorf("lease_seconds %d exceeds the service maximum %d", m.LeaseSeconds, c.LeaseSeconds)
	}
	return nil
}

// RenderMiniupnpd renders the miniupnpd configuration.
func RenderMiniupnpd(c Config) string {
	var b strings.Builder
	// 渲染层兜底：接口名里的换行会在 conf 里多加一行
	ext := oneLine(strings.TrimSpace(c.ExternalIface))
	intf := oneLine(strings.TrimSpace(c.InternalIface))
	cidr := oneLine(strings.TrimSpace(c.AllowCIDR))
	if ext == "" {
		ext = "eth0"
	}
	if intf == "" {
		intf = "br-lan"
	}
	fmt.Fprintf(&b, "ext_ifname=%s\n", ext)
	fmt.Fprintf(&b, "listening_ip=%s\n", intf)
	if cidr != "" {
		fmt.Fprintf(&b, "allow %s\n", cidr)
	}
	natpmp := "no"
	if c.NATPMP {
		natpmp = "yes"
	}
	fmt.Fprintf(&b, "enable_natpmp=%s\n", natpmp)
	fmt.Fprintf(&b, "clean_ruleset_interval=%d\n", c.LeaseSeconds/2)
	return b.String()
}

// RenderNftMappings renders the port mappings as an nftables program.
//
// 在 inet 表里同时承载 ipv4/ipv6，DNAT 必须写 `dnat ip to`（或 `dnat ip6 to`）
// 消歧义，否则 nft 报 "specify `dnat ip' or `dnat ip6' in inet table to
// disambiguate"。chain 声明必须多行（chain 名单独一行、type hook 单独一行）。
//
// 绝不写 flush ruleset：如果做幂等替换也是 delete table 再重建，而不是 flush，
// 避免把同表里其它进程/会话链进的链误删。
func RenderNftMappings(ms []Mapping) string {
	var b strings.Builder
	b.WriteString("table inet " + UpnpTable + " {\n")
	b.WriteString("  chain prerouting {\n")
	b.WriteString("    type nat hook prerouting priority dstnat;\n")
	for _, m := range ms {
		fmt.Fprintf(&b, "    %s dport %d dnat ip to %s:%d\n", m.Protocol, m.ExtPort, strings.TrimSpace(m.IntIP), m.IntPort)
	}
	b.WriteString("  }\n")
	b.WriteString("}\n")
	return b.String()
}

// Apply validates the config, writes miniupnpd.conf and restarts the service.
// Both restart paths are attempted and both errors are reported when neither
// works, so an operator can tell "service missing" from "no permission".
func Apply(r execx.Runner, c Config, confPath string) error {
	if err := Validate(c); err != nil {
		return err
	}
	if confPath == "" {
		confPath = DefaultConfPath
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderMiniupnpd(c)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	sysErr := error(nil)
	if _, err := r.Run("systemctl", "restart", "miniupnpd"); err == nil {
		return nil
	} else {
		sysErr = err
	}
	initErr := error(nil)
	if _, err := r.Run("/etc/init.d/miniupnpd", "restart"); err == nil {
		return nil
	} else {
		initErr = err
	}
	return fmt.Errorf("配置已写入 %s，但 miniupnpd 未重启：systemctl: %v；init.d: %v", confPath, sysErr, initErr)
}

// ActiveMappings parses `upnpc -l` output into mappings.
//
// upnpc not being installed is a real, actionable error (the admin must apt
// install miniupnpc to see mappings), so it returns an explicit error rather
// than silently an empty list.
func ActiveMappings(r execx.Runner) ([]Mapping, error) {
	out, err := r.Run("upnpc", "-l")
	if err != nil {
		return nil, fmt.Errorf("upnpc -l: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, fmt.Errorf("upnpc -l returned no output")
	}
	var ms []Mapping
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		// upnpc -l port mappings look like: 0 TCP 8080->192.168.1.10:80 'http' '' 0s
		// so proto is fields[1] and "extport->ip:port" is fields[2].
		if len(fields) < 3 {
			continue
		}
		proto := strings.ToLower(fields[1])
		if proto != "tcp" && proto != "udp" {
			continue
		}
		arrow := strings.Index(fields[2], "->")
		if arrow <= 0 {
			continue
		}
		extPort, err := strconv.Atoi(fields[2][:arrow])
		if err != nil {
			continue
		}
		ipPort := fields[2][arrow+2:]
		ipPart, portPart, ok := strings.Cut(ipPort, ":")
		if !ok {
			continue
		}
		intPort, err := strconv.Atoi(portPart)
		if err != nil {
			continue
		}
		if !isPrivateIP(net.ParseIP(ipPart)) {
			continue
		}
		ms = append(ms, Mapping{
			Protocol: proto,
			ExtPort:  extPort,
			IntIP:    ipPart,
			IntPort:  intPort,
		})
	}
	return ms, nil
}

// oneLine 把任意字符串压成单行，渲染层兜底用。
func oneLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
