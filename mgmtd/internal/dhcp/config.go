// Package dhcp 提供 DHCP 服务器配置模型与 dnsmasq 配置渲染。
//
// 渲染器的输入是 DHCPConfig 结构体，输出是可直接写入
// /etc/dnsmasq.d/ 的 dnsmasq 配置文本。
package dhcp

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

// DHCPConfig 描述一个静态 DHCP 服务区段。
type DHCPConfig struct {
	Interface  string   `json:"interface"`   // 绑定网卡，如 "lan"
	RangeStart string   `json:"range_start"` // 地址池起始，如 "192.168.1.100"
	RangeEnd   string   `json:"range_end"`   // 地址池结束，如 "192.168.1.200"
	Netmask    string   `json:"netmask"`     // 子网掩码，如 "255.255.255.0"（冗余，供 dhcp-range 计算掩码）
	Gateway    string   `json:"gateway"`     // 网关，写入 dhcp-option=3
	LeaseTime  string   `json:"lease_time"`  // 租期，如 "12h" 或 "86400"
	DNS        []string `json:"dns"`         // DNS 服务器列表，写入 dhcp-option=6
	// DHCPv6
	V6RangeStart string `json:"v6_range_start"`
	V6RangeEnd   string `json:"v6_range_end"`
	V6PrefixLen  int    `json:"v6_prefix_len"`
	EnableRA     bool   `json:"enable_ra"`
}

type Config struct {
	DHCPConfig
	Domain       string        `json:"domain"`
	StaticLeases []StaticLease `json:"static_leases"`
	Enabled      bool          `json:"enabled"`
	V6RangeStart string        `json:"v6_range_start"`
	V6RangeEnd   string        `json:"v6_range_end"`
	V6PrefixLen  int           `json:"v6_prefix_len"`
	EnableRA     bool          `json:"enable_ra"`
}

// RenderDnsmasq 将 DHCPConfig 渲染为 dnsmasq 配置文本。
//
// 生成的配置包含：
//   - bind-interfaces + interface=<Interface>：只在指定接口上提供 DHCP
//   - dhcp-range=<start>,<end>,<netmask>,<leasetime>：地址池
//   - dhcp-option=3,<gateway>：网关
//   - dhcp-option=6,<dns1>,<dns2>,...：DNS
func RenderDnsmasq(cfg Config) string {
	var b strings.Builder

	if cfg.Interface != "" {
		b.WriteString("bind-interfaces\n")
		b.WriteString("interface=" + cfg.Interface + "\n")
	}

	// dhcp-range 需要显式的起始与结束地址。
	if cfg.RangeStart != "" && cfg.RangeEnd != "" {
		b.WriteString("dhcp-range=" + cfg.RangeStart + "," + cfg.RangeEnd)
		if cfg.Netmask != "" {
			b.WriteString("," + cfg.Netmask)
		}
		if cfg.LeaseTime != "" {
			b.WriteString("," + cfg.LeaseTime)
		}
		b.WriteString("\n")
	}

	// 网关,dhcp-option=3
	if cfg.Gateway != "" {
		b.WriteString(fmt.Sprintf("dhcp-option=3,%s\n", cfg.Gateway))
	}

	// DNS,dhcp-option=6。值用逗号分隔成一行;没有 DNS 时不输出。
	if len(cfg.DNS) > 0 {
		b.WriteString("dhcp-option=6," + strings.Join(cfg.DNS, ",") + "\n")
	}

	return b.String()
}

// leaseTimeRe 校验租约时间格式：纯数字 + 单位 (s/m/h/d)，如 12h、30m、1d。
var leaseTimeRe = regexp.MustCompile(`^\d+[smhd]$`)

// StaticLease 描述一条 DHCP 静态租约。
type StaticLease struct {
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// ServerConfig 是 P5-1 补强的完整 DHCP 服务端配置。
// 它嵌入了 DHCPConfig（保留与 /api/v1/dhcp/config 兼用），并新增了
// 静态租约、域名与全局开关。
type ServerConfig struct {
	DHCPConfig
	Domain       string        `json:"domain"`
	StaticLeases []StaticLease `json:"static_leases"`
	Enabled      bool          `json:"enabled"`
	// DHCPv6
	V6RangeStart string `json:"v6_range_start"`
	V6RangeEnd   string `json:"v6_range_end"`
	V6PrefixLen  int    `json:"v6_prefix_len"`
	EnableRA     bool   `json:"enable_ra"`
}

// subnetFor 根据地址池起点与子网掩码构造 *net.IPNet。
func subnetFor(rangeStart, netmask string) (*net.IPNet, error) {
	ip := net.ParseIP(rangeStart)
	mask := net.IPMask(net.ParseIP(netmask).To4())
	if ip == nil || mask == nil {
		return nil, fmt.Errorf("invalid range start or netmask: %s/%s", rangeStart, netmask)
	}
	ip = ip.To4()
	if ip == nil {
		return nil, fmt.Errorf("range start is not ipv4: %s", rangeStart)
	}
	return &net.IPNet{IP: ip, Mask: mask}, nil
}

// ValidateConfig 校验 ServerConfig：
//   - Interface 非空
//   - RangeStart/RangeEnd 必须为 IP，同子网，start<=end
//   - LeaseTime 匹配 ^\d+[smhd]$
//   - DNS 每项为 IP
//   - Gateway(若非空)为 IP 且在地址池子段内
//   - 静态租约的 MAC 合法、IP 为 IP 且在子段内、不落在动态 range 内、MAC 不重复
func ValidateV6Config(c ServerConfig) error {
	if c.Interface == "" {
		return fmt.Errorf("interface is required")
	}
	if c.V6RangeStart != "" {
		startIP := net.ParseIP(c.V6RangeStart)
		if startIP == nil {
			return fmt.Errorf("v6_range_start is not a valid ipv6: %s", c.V6RangeStart)
		}
		if startIP.To4() != nil {
			return fmt.Errorf("v6_range_start must be ipv6, got ipv4: %s", c.V6RangeStart)
		}
	} else {
		return fmt.Errorf("v6_range_start is required")
	}
	if c.V6RangeEnd != "" {
		endIP := net.ParseIP(c.V6RangeEnd)
		if endIP == nil {
			return fmt.Errorf("v6_range_end is not a valid ipv6: %s", c.V6RangeEnd)
		}
		if endIP.To4() != nil {
			return fmt.Errorf("v6_range_end must be ipv6, got ipv4: %s", c.V6RangeEnd)
		}
	} else {
		return fmt.Errorf("v6_range_end is required")
	}
	if c.V6PrefixLen != 0 {
		if c.V6PrefixLen < 1 || c.V6PrefixLen > 128 {
			return fmt.Errorf("v6_prefix_len %d 超出 1-128", c.V6PrefixLen)
		}
	}
	return nil
}

func ValidateConfig(c ServerConfig) error {
	if c.Interface == "" {
		return fmt.Errorf("interface is required")
	}
	// Interface 只查了非空，然后拼进 "interface=%s"。一个换行就能在
	// dnsmasq 里插入任意指令（no-resolv / dhcp-option=3,<攻击者IP> ...）。
	// 这是 pppoe/ipv6/upnp 之后第四处同型缺陷。
	for _, r := range c.Interface {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '@' || r == ':'
		if !ok {
			return fmt.Errorf("interface 只能包含字母数字和 _-.:@（发现 %q）", r)
		}
	}
	if len(c.Interface) > 15 {
		return fmt.Errorf("interface 名不能超过 15 字节")
	}
	// Domain 拼进 "domain=%s"，同样只挡住了"非空"。
	if c.Domain != "" {
		for _, r := range c.Domain {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_'
			if !ok {
				return fmt.Errorf("domain 只能包含字母数字和 -._（发现 %q）", r)
			}
		}
		if len(c.Domain) > 253 {
			return fmt.Errorf("domain 不能超过 253 字节")
		}
	}
	startIP := net.ParseIP(c.RangeStart)
	endIP := net.ParseIP(c.RangeEnd)
	if startIP == nil {
		return fmt.Errorf("range_start is not a valid ip: %s", c.RangeStart)
	}
	if endIP == nil {
		return fmt.Errorf("range_end is not a valid ip: %s", c.RangeEnd)
	}
	if net.ParseIP(c.Netmask) == nil {
		return fmt.Errorf("netmask is not a valid ip: %s", c.Netmask)
	}
	ipNet, err := subnetFor(c.RangeStart, c.Netmask)
	if err != nil {
		return err
	}
	// start<=end（按 4 字节比较）
	sb, eb := startIP.To4(), endIP.To4()
	if sb == nil || eb == nil {
		return fmt.Errorf("range start/end must be ipv4")
	}
	if bytes.Compare(sb, eb) > 0 {
		return fmt.Errorf("range_start %s > range_end %s", c.RangeStart, c.RangeEnd)
	}
	// 同一个子网：两个端点必须在同一个掩码下
	if !ipNet.Contains(startIP) || !ipNet.Contains(endIP) {
		return fmt.Errorf("range_start/range_end not in subnet %s/%s", c.RangeStart, c.Netmask)
	}
	if c.LeaseTime != "" && !leaseTimeRe.MatchString(c.LeaseTime) {
		return fmt.Errorf("invalid lease_time %q: must match ^\\d+[smhd]$", c.LeaseTime)
	}
	for i, d := range c.DNS {
		if net.ParseIP(d) == nil {
			return fmt.Errorf("dns[%d] is not a valid ip: %s", i, d)
		}
	}
	if c.Gateway != "" {
		gw := net.ParseIP(c.Gateway)
		if gw == nil {
			return fmt.Errorf("gateway is not a valid ip: %s", c.Gateway)
		}
		if !ipNet.Contains(gw) {
			return fmt.Errorf("gateway %s not in subnet %s/%s", c.Gateway, c.RangeStart, c.Netmask)
		}
	}
	seenMAC := make(map[string]bool, len(c.StaticLeases))
	for i, sl := range c.StaticLeases {
		if _, err := net.ParseMAC(sl.MAC); err != nil {
			return fmt.Errorf("static_lease[%d] mac invalid: %s", i, sl.MAC)
		}
		if sl.MAC == "" {
			return fmt.Errorf("static_lease[%d] mac is empty", i)
		}
		// Hostname 拼进 "dhcp-host=<mac>,<ip>,<hostname>"，原来完全没有校验。
		// RFC 952/1123 主机名：字母数字、连字符、点；不能以连字符开头结尾。
		if sl.Hostname != "" {
			if len(sl.Hostname) > 253 {
				return fmt.Errorf("static_lease[%d] hostname 不能超过 253 字节", i)
			}
			for _, r := range sl.Hostname {
				ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_'
				if !ok {
					return fmt.Errorf("static_lease[%d] hostname 只能包含字母数字和 -._（发现 %q）", i, r)
				}
			}
			if sl.Hostname[0] == '-' || sl.Hostname[len(sl.Hostname)-1] == '-' {
				return fmt.Errorf("static_lease[%d] hostname 不能以连字符开头或结尾: %q", i, sl.Hostname)
			}
		}
		ip := net.ParseIP(sl.IP)
		if ip == nil {
			return fmt.Errorf("static_lease[%d] ip invalid: %s", i, sl.IP)
		}
		if !ipNet.Contains(ip) {
			return fmt.Errorf("static_lease[%d] ip %s not in subnet %s/%s", i, sl.IP, c.RangeStart, c.Netmask)
		}
		// 不能落在动态 range 内（会冲突）
		ib := ip.To4()
		if ib != nil && bytes.Compare(sb, ib) <= 0 && bytes.Compare(ib, eb) <= 0 {
			return fmt.Errorf("static_lease[%d] ip %s collides with dynamic range %s-%s", i, sl.IP, c.RangeStart, c.RangeEnd)
		}
		if seenMAC[sl.MAC] {
			return fmt.Errorf("static_lease[%d] duplicate mac %s", i, sl.MAC)
		}
		seenMAC[sl.MAC] = true
	}
	if c.V6RangeStart != "" && c.V6RangeEnd != "" {
		if c.V6PrefixLen < 1 || c.V6PrefixLen > 128 {
			return fmt.Errorf("v6_prefix_len %d 超出 1-128", c.V6PrefixLen)
		}
	}
	return nil
}

//
// NOTE：这里是方法 (ServerConfig.RenderDnsmasq) 而非新包级函数，
// 目的是避免与已有的包级函数 RenderDnsmasq(DHCPConfig) 发生命名冲突
// （Go 不允许同名包级函数多态）。调用形式为 cfg.RenderDnsmasq()，
// 语义上仍是 "RenderDnsmasq(c ServerConfig) string"。
func (c ServerConfig) RenderDnsmasq() string {
	var b strings.Builder
	// 禁用时整行前缀 #
	prefix := func(s string) string {
		if !c.Enabled {
			return "# " + s
		}
		return s
	}
	// 渲染层兜底：prefix("# ") 只保护整段的第一行，字段里的换行会逃出注释。
	if c.Interface != "" {
		b.WriteString(prefix("bind-interfaces\n"))
		b.WriteString(prefix("interface=" + oneLine(c.Interface) + "\n"))
	}
	if c.RangeStart != "" && c.RangeEnd != "" {
		rangeLine := "dhcp-range=" + c.RangeStart + "," + c.RangeEnd
		if c.Netmask != "" {
			rangeLine += "," + c.Netmask
		}
		if c.LeaseTime != "" {
			rangeLine += "," + c.LeaseTime
		}
		b.WriteString(prefix(rangeLine + "\n"))
	}
	if c.Gateway != "" {
		b.WriteString(prefix(fmt.Sprintf("dhcp-option=3,%s\n", c.Gateway)))
	}
	if len(c.DNS) > 0 {
		b.WriteString(prefix("dhcp-option=6," + strings.Join(c.DNS, ",") + "\n"))
	}
	if c.Domain != "" {
		b.WriteString(prefix("domain=" + oneLine(c.Domain) + "\n"))
	}
	for _, sl := range c.StaticLeases {
		line := "dhcp-host=" + oneLine(sl.MAC) + "," + oneLine(sl.IP)
		if sl.Hostname != "" {
			line += "," + oneLine(sl.Hostname)
		}
		line += "\n"
		b.WriteString(prefix(line))
	}
	if c.V6RangeStart != "" && c.V6RangeEnd != "" {
		v6line := "dhcp-range=" + c.V6RangeStart + "," + c.V6RangeEnd
		if c.V6PrefixLen > 0 {
			v6line += "," + fmt.Sprintf("%d", c.V6PrefixLen)
		}
		v6line += ",12h"
		b.WriteString(prefix(v6line + "\n"))
	}
	if c.EnableRA {
		b.WriteString(prefix("enable-ra\n"))
	}
	if !c.Enabled {
		// 最后补一条兜底，让禁用状态下文件内容仍完整可读
		return b.String()
	}
	return b.String()
}

// FormatIP 辅助：将 net.IP 格式化为点分字符串。
func FormatIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// SortStaticLeases 按 MAC 排序（用于稳定输出）。
func SortStaticLeases(sl []StaticLease) {
	sort.Slice(sl, func(i, j int) bool {
		return sl[i].MAC < sl[j].MAC
	})
}

// oneLine 把任意字符串压成单行，渲染层兜底用。
// dnsmasq 配置以行为单位，注释前缀只保护第一行。
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
