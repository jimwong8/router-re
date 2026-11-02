package route

import (
	"fmt"
	"net"
	"strings"
)

// ValidateBGP 校验 BGP 配置。
//
// 这个包原来**一个校验函数都没有**：handleSetBGP 解码 JSON 后直接 store，
// 再原样渲染进 frr.conf。frr.conf 以行为单位，任何自由字段里一个换行
// 就能插入任意 FRR 指令。最危险的形态是往 password 里塞换行：
//     neighbor 10.0.0.2 password pw
//      neighbor 6.6.6.6 remote-as 1
// 等于让本机与攻击者控制的路由器建立 BGP peering —— 路由劫持。
func ValidateBGP(c BGPConfig) error {
	if c.LocalAS < 1 || c.LocalAS > 4294967295 {
		return fmt.Errorf("local_as 必须在 1-4294967295，收到 %d", c.LocalAS)
	}
	if c.RouterID != "" && net.ParseIP(c.RouterID) == nil {
		return fmt.Errorf("router_id 不是合法 IP: %q", c.RouterID)
	}
	for i, p := range c.Peers {
		if net.ParseIP(p.Address) == nil {
			return fmt.Errorf("peers[%d].address 不是合法 IP: %q", i, p.Address)
		}
		if p.RemoteAS < 1 || p.RemoteAS > 4294967295 {
			return fmt.Errorf("peers[%d].remote_as 必须在 1-4294967295，收到 %d", i, p.RemoteAS)
		}
		if len(p.Password) > 128 {
			return fmt.Errorf("peers[%d].password 不能超过 128 字节", i)
		}
		// FRR 的 password 不能含空白；换行还能插入指令
		for _, r := range p.Password {
			if r < 0x20 || r == 0x7f || r == ' ' {
				return fmt.Errorf("peers[%d].password 不能包含空格、控制字符或换行（发现 %#U）", i, r)
			}
		}
	}
	for i, n := range c.Networks {
		if _, _, err := net.ParseCIDR(n); err != nil {
			return fmt.Errorf("networks[%d] 不是合法网段: %q", i, n)
		}
	}
	return nil
}

// ValidateOSPF 校验 OSPF 配置。
//
// area 同样是自由文本，注入 "0\n default-information originate" 会让本机
// 向全网宣告自己是默认网关 —— 流量直接改道。
func ValidateOSPF(c OSPFConfig) error {
	if c.RouterID != "" && net.ParseIP(c.RouterID) == nil {
		return fmt.Errorf("router_id 不是合法 IP: %q", c.RouterID)
	}
	for i, n := range c.Networks {
		if _, _, err := net.ParseCIDR(n); err != nil {
			return fmt.Errorf("networks[%d] 不是合法网段: %q", i, n)
		}
	}
	// OSPF area 是 32 位整数，可写成十进制或点分十进制
	if c.Area != "" && !validAreaID(c.Area) {
		return fmt.Errorf("area 必须是 0-4294967295 或点分十进制（如 0.0.0.1），收到 %q", c.Area)
	}
	return nil
}

// validAreaID 校验 OSPF area id。
func validAreaID(s string) bool {
	if net.ParseIP(s) != nil {
		return true // 点分十进制形式
	}
	var v uint64
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
		v = v*10 + uint64(r-'0')
		if v > 4294967295 {
			return false
		}
	}
	return s != ""
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

// BGPPeer describes a single BGP neighbor.
type BGPPeer struct {
	Address  string `json:"address"`
	RemoteAS int    `json:"remote_as"`
	Password string `json:"password,omitempty"`
	MultiHop bool   `json:"multi_hop,omitempty"`
}

// BGPConfig describes a BGP instance for FRR.
type BGPConfig struct {
	LocalAS  int       `json:"local_as"`
	RouterID string    `json:"router_id"`
	Peers    []BGPPeer `json:"peers"`
	Networks []string  `json:"networks"`
}

// OSPFConfig describes an OSPF instance for FRR.
type OSPFConfig struct {
	RouterID string   `json:"router_id"`
	Networks []string `json:"networks"`
	Area     string   `json:"area"`
}

// RenderBGP renders the BGP section of an FRR configuration.
func RenderBGP(cfg BGPConfig) string {
	var b strings.Builder
	if cfg.LocalAS <= 0 {
		return ""
	}
	// 渲染层兜底：frr.conf 以行为单位，任何字段一个换行就能插入指令。
	b.WriteString(fmt.Sprintf("router bgp %d\n", cfg.LocalAS))
	if cfg.RouterID != "" {
		b.WriteString(fmt.Sprintf(" bgp router-id %s\n", oneLine(cfg.RouterID)))
	}
	for _, p := range cfg.Peers {
		if p.Address == "" {
			continue
		}
		addr := oneLine(p.Address)
		b.WriteString(fmt.Sprintf(" neighbor %s remote-as %d\n", addr, p.RemoteAS))
		if p.Password != "" {
			// 密码里的空格会截断参数，换行会插入新指令
			pw := strings.ReplaceAll(oneLine(p.Password), " ", "")
			b.WriteString(fmt.Sprintf(" neighbor %s password %s\n", addr, pw))
		}
		if p.MultiHop {
			b.WriteString(fmt.Sprintf(" neighbor %s ebgp-multihop 2\n", addr))
		}
	}
	b.WriteString(" address-family ipv4 unicast\n")
	for _, n := range cfg.Networks {
		if n = strings.TrimSpace(oneLine(n)); n == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("  network %s\n", n))
	}
	for _, p := range cfg.Peers {
		if p.Address == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("  neighbor %s activate\n", oneLine(p.Address)))
	}
	b.WriteString(" exit-address-family\n")
	return b.String()
}

// RenderOSPF renders the OSPF section of an FRR configuration.
func RenderOSPF(cfg OSPFConfig) string {
	var b strings.Builder
	b.WriteString("router ospf\n")
	if cfg.RouterID != "" {
		b.WriteString(fmt.Sprintf(" ospf router-id %s\n", oneLine(cfg.RouterID)))
	}
	area := oneLine(cfg.Area)
	if area == "" {
		area = "0"
	}
	for _, n := range cfg.Networks {
		if n = strings.TrimSpace(oneLine(n)); n == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(" network %s area %s\n", n, area))
	}
	return b.String()
}

// RenderFrrConf combines BGP and OSPF configs into a complete frr.conf.
func RenderFrrConf(bgp *BGPConfig, ospf *OSPFConfig) string {
	var b strings.Builder
	b.WriteString("frr version 8.4\n")
	b.WriteString("frr defaults traditional\n")
	b.WriteString("hostname router\n")
	b.WriteString("log syslog informational\n")
	if bgp != nil {
		b.WriteString("!\n")
		b.WriteString(RenderBGP(*bgp))
	}
	if ospf != nil {
		b.WriteString("!\n")
		b.WriteString(RenderOSPF(*ospf))
	}
	b.WriteString("!\n")
	b.WriteString("line vty\n")
	return b.String()
}
