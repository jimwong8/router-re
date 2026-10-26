// Package dhcp 提供 DHCP 服务器配置模型与 dnsmasq 配置渲染。
//
// 渲染器的输入是 DHCPConfig 结构体，输出是可直接写入
// /etc/dnsmasq.d/ 的 dnsmasq 配置文本。
package dhcp

import (
	"fmt"
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
}

// RenderDnsmasq 将 DHCPConfig 渲染为 dnsmasq 配置文本。
//
// 生成的配置包含：
//   - bind-interfaces + interface=<Interface>：只在指定接口上提供 DHCP
//   - dhcp-range=<start>,<end>,<netmask>,<leasetime>：地址池
//   - dhcp-option=3,<gateway>：网关
//   - dhcp-option=6,<dns1>,<dns2>,...：DNS
func RenderDnsmasq(cfg DHCPConfig) string {
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
