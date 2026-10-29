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
func ValidateConfig(c ServerConfig) error {
	if c.Interface == "" {
		return fmt.Errorf("interface is required")
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

// ValidateV6Config validates only the v6-related fields of ServerConfig.
func ValidateV6Config(c ServerConfig) error {
	if c.Interface == "" {
		return fmt.Errorf("interface is empty")
	}
	if c.V6RangeStart != "" {
		if net.ParseIP(c.V6RangeStart) == nil {
			return fmt.Errorf("v6_range_start is not a valid ipv6: %s", c.V6RangeStart)
		}
		if c.V6RangeEnd != "" {
			if net.ParseIP(c.V6RangeEnd) == nil {
				return fmt.Errorf("v6_range_end is not a valid ipv6: %s", c.V6RangeEnd)
			}
		}
		if c.V6RangeEnd != "" && c.V6RangeStart == c.V6RangeEnd {
			return fmt.Errorf("v6_range_start and v6_range_end must differ")
		}
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
	if c.Interface != "" {
		b.WriteString(prefix("bind-interfaces\n"))
		b.WriteString(prefix("interface=" + c.Interface + "\n"))
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
		b.WriteString(prefix("domain=" + c.Domain + "\n"))
	}
	for _, sl := range c.StaticLeases {
		line := "dhcp-host=" + sl.MAC + "," + sl.IP
		if sl.Hostname != "" {
			line += "," + sl.Hostname
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
