package porttrigger

import (
	"fmt"
	"strings"
)

// MDNSRelay 描述 mDNS（组播 DNS）中继：从 in 接口收到 mDNS 组播后转发到
// out 接口，实现跨 VLAN 的 .local 解析。
type MDNSRelay struct {
	InIfaces  []string `json:"in_ifaces"`  // 从哪些接口收
	OutIfaces []string `json:"out_ifaces"` // 转发到哪些接口
	Enabled   bool     `json:"enabled"`
}

// ValidateMDNS 校验 mDNS 中继配置。
//
// **组播环路红线**：in 和 out 不能完全相同——如果 in 列表与 out 列表等价，
// 一台主机发出 mDNS 组播会被转发回它自己所在的所有接口，再被转发回来，
// 形成组播风暴，流量指数增长，几秒内就能打满内网。此后果写进错误信息。
func ValidateMDNS(m MDNSRelay) error {
	if len(m.InIfaces) == 0 {
		return fmt.Errorf("至少需要一个入接口 (in_ifaces)")
	}
	if len(m.OutIfaces) == 0 {
		return fmt.Errorf("至少需要一个出接口 (out_ifaces)")
	}
	for _, i := range append(append([]string{}, m.InIfaces...), m.OutIfaces...) {
		if i == "lo" {
			return fmt.Errorf("不允许使用 lo（loopback）作为 mDNS 中继接口")
		}
	}
	if sameStrings(m.InIfaces, m.OutIfaces) {
		return fmt.Errorf("入接口与出接口不能完全相同（%v）：中继会把组播转发回自身，形成组播环路，流量指数增长，几秒内打满内网", m.InIfaces)
	}
	return nil
}

// sameStrings 判断两个字符串列表（忽略顺序）是否完全等价。
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[string]int, len(a))
	for _, s := range a {
		ma[s]++
	}
	for _, s := range b {
		ma[s]--
		if ma[s] < 0 {
			return false
		}
	}
	return true
}

// RenderAvahi 生成 avahi-daemon.conf 的 [server] + [publish] 段。
func RenderAvahi(m MDNSRelay) string {
	if !m.Enabled || len(m.InIfaces) == 0 {
		return ""
	}
	ifaces := append([]string{}, m.InIfaces...)
	ifaces = append(ifaces, m.OutIfaces...)
	var b strings.Builder
	b.WriteString("[server]\n")
	b.WriteString("use-ipv4=yes\n")
	b.WriteString(fmt.Sprintf("allow-interfaces=%s\n", strings.Join(ifaces, ",")))
	b.WriteString("\n[publish]\n")
	b.WriteString("publish-aaaa-on-ipv4=no\n")
	return b.String()
}
