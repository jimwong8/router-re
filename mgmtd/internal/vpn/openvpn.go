package vpn

import (
	"fmt"
	"net"
	"strings"
)

// OpenVPNConfig 是 OpenVPN 服务端配置。
//
// 字段名对齐 iKuai 的 vpn/openvpn/services（subnet/mask/proto/port/tun_mtu/
// cipher/comp_lzo/dev_type/topology/method）。
//
// 与 iKuai 的一处有意差异：iKuai 的 cipher 白名单里有 BF-CBC / DES-CBC /
// RC2-40-CBC 这类 1990 年代的算法，我们只接受 AEAD（GCM）和 AES-CBC。
// 允许 DES/RC2 等于把"VPN"当成明文隧道用，宁可拒绝配置也不假装加密。
type OpenVPNConfig struct {
	Enabled    bool      `json:"enabled"`
	Subnet     string    `json:"subnet"`   // 隧道网段，如 10.8.0.0
	Mask       string    `json:"mask"`     // 如 255.255.255.0
	Proto      string    `json:"proto"`    // udp / tcp
	Port       int       `json:"port"`     // 1-65535
	TunMTU     int       `json:"tun_mtu"`  // 576-1500
	Cipher     string    `json:"cipher"`   // 见 allowedCiphers
	CompLZO    int       `json:"comp_lzo"` // 0/1
	DevType    string    `json:"dev_type"` // tun / tap
	Topology   string    `json:"topology"` // net30 / subnet
	Method     int       `json:"method"`   // 0=证书 1=证书+用户名密码 2=仅用户名密码
	DNS1       string    `json:"dns1"`
	DNS2       string    `json:"dns2"`
	RedirectGW bool      `json:"redirect_gateway"` // 全局代理
	PushRoutes []string  `json:"push_routes"`      // 推送给客户端的内网路由 CIDR
	MaxClients int       `json:"max_clients"`
	Users      []VPNUser `json:"users"` // method>=1 时使用
}

// allowedCiphers 只保留仍被认为安全的算法。
var allowedCiphers = map[string]bool{
	"AES-256-GCM": true,
	"AES-192-GCM": true,
	"AES-128-GCM": true,
	"AES-256-CBC": true,
	"AES-192-CBC": true,
	"AES-128-CBC": true,
}

// ValidateOpenVPN 校验 OpenVPN 服务端配置。
func ValidateOpenVPN(c OpenVPNConfig) error {
	if c.Subnet == "" || c.Mask == "" {
		return fmt.Errorf("隧道网段与掩码不能为空")
	}
	ip := net.ParseIP(c.Subnet)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("隧道网段不是合法 IPv4：%s", c.Subnet)
	}
	maskIP := net.ParseIP(c.Mask)
	if maskIP == nil || maskIP.To4() == nil {
		return fmt.Errorf("掩码不是合法 IPv4：%s", c.Mask)
	}
	ipnet := net.IPNet{IP: ip.To4().Mask(net.IPMask(maskIP.To4())), Mask: net.IPMask(maskIP.To4())}
	ones, bits := ipnet.Mask.Size()
	if bits != 32 || ones < 16 || ones > 30 {
		return fmt.Errorf("掩码长度必须在 /16-/30 之间，当前 /%d", ones)
	}
	if c.Proto != "udp" && c.Proto != "tcp" {
		return fmt.Errorf("协议只能是 udp 或 tcp，当前 %q", c.Proto)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("端口必须在 1-65535 之间，当前 %d", c.Port)
	}
	if c.TunMTU != 0 && (c.TunMTU < 576 || c.TunMTU > 1500) {
		return fmt.Errorf("TUN MTU 必须在 576-1500 之间")
	}
	if c.Cipher != "" && !allowedCiphers[c.Cipher] {
		return fmt.Errorf("不支持的加密算法 %q（只接受 AES-GCM / AES-CBC，DES/RC2/BF 已不安全）", c.Cipher)
	}
	if c.CompLZO != 0 && c.CompLZO != 1 {
		return fmt.Errorf("comp_lzo 只能是 0 或 1")
	}
	if c.DevType != "" && c.DevType != "tun" && c.DevType != "tap" {
		return fmt.Errorf("设备类型只能是 tun 或 tap")
	}
	if c.Topology != "" && c.Topology != "net30" && c.Topology != "subnet" {
		return fmt.Errorf("拓扑只能是 net30 或 subnet")
	}
	if c.Method < 0 || c.Method > 2 {
		return fmt.Errorf("认证方式只能是 0/1/2")
	}
	if c.MaxClients < 0 || c.MaxClients > 1024 {
		return fmt.Errorf("最大客户端数必须在 0-1024 之间")
	}
	for _, d := range []string{c.DNS1, c.DNS2} {
		if d == "" {
			continue
		}
		if v := net.ParseIP(d); v == nil || v.To4() == nil {
			return fmt.Errorf("DNS 不是合法 IPv4：%s", d)
		}
	}
	for _, r := range c.PushRoutes {
		if _, _, err := net.ParseCIDR(r); err != nil {
			return fmt.Errorf("推送路由不是合法 CIDR：%s", r)
		}
	}
	if c.Enabled && c.Method >= 1 && len(c.Users) == 0 {
		return fmt.Errorf("认证方式包含用户名密码时，至少需要一个账号")
	}
	// OpenVPN 账号没有地址池概念，传空池只校验用户名/密码格式
	return validateVPNUsers(c.Users, AddrPool{})
}

// RenderOpenVPNServerConf 生成 server.conf。
func RenderOpenVPNServerConf(c OpenVPNConfig) string {
	proto := c.Proto
	if proto == "" {
		proto = "udp"
	}
	port := c.Port
	if port == 0 {
		port = 1194
	}
	dev := c.DevType
	if dev == "" {
		dev = "tun"
	}
	cipher := c.Cipher
	if cipher == "" {
		cipher = "AES-256-GCM"
	}
	topology := c.Topology
	if topology == "" {
		topology = "subnet"
	}
	mtu := c.TunMTU
	if mtu == 0 {
		mtu = 1420
	}
	var b strings.Builder
	b.WriteString("# router-re OpenVPN server\n")
	fmt.Fprintf(&b, "port %d\n", port)
	fmt.Fprintf(&b, "proto %s\n", proto)
	fmt.Fprintf(&b, "dev %s\n", dev)
	if dev == "tun" {
		fmt.Fprintf(&b, "topology %s\n", topology)
	}
	fmt.Fprintf(&b, "server %s %s\n", c.Subnet, c.Mask)
	fmt.Fprintf(&b, "tun-mtu %d\n", mtu)
	if strings.HasSuffix(cipher, "GCM") {
		fmt.Fprintf(&b, "data-ciphers %s\n", cipher)
		fmt.Fprintf(&b, "data-ciphers-fallback AES-256-CBC\n")
	} else {
		fmt.Fprintf(&b, "cipher %s\n", cipher)
	}
	b.WriteString("auth SHA256\n")
	b.WriteString("tls-version-min 1.2\n")
	b.WriteString("dh none\n")
	if c.Method == 0 || c.Method == 1 {
		b.WriteString("ca /etc/openvpn/server/rr-ca.crt\n")
		b.WriteString("cert /etc/openvpn/server/rr-server.crt\n")
		b.WriteString("key /etc/openvpn/server/rr-server.key\n")
		b.WriteString("tls-crypt /etc/openvpn/server/rr-tls-crypt.key\n")
	}
	if c.Method >= 1 {
		b.WriteString("plugin /usr/lib/openvpn/openvpn-plugin-auth-pam.so login\n")
		b.WriteString("verify-client-cert require\n")
	}
	if c.CompLZO == 1 {
		b.WriteString("comp-lzo yes\n")
	} else {
		b.WriteString("allow-compression no\n")
	}
	if c.MaxClients > 0 {
		fmt.Fprintf(&b, "max-clients %d\n", c.MaxClients)
	}
	if c.RedirectGW {
		b.WriteString("push \"redirect-gateway def1 bypass-dhcp\"\n")
	}
	dns := []string{}
	if c.DNS1 != "" {
		dns = append(dns, c.DNS1)
	}
	if c.DNS2 != "" {
		dns = append(dns, c.DNS2)
	}
	if len(dns) == 0 {
		dns = []string{"223.5.5.5", "119.29.29.29"}
	}
	for _, d := range dns {
		fmt.Fprintf(&b, "push \"dhcp-option DNS %s\"\n", d)
	}
	for _, r := range c.PushRoutes {
		fmt.Fprintf(&b, "push \"route %s\"\n", r)
	}
	b.WriteString("keepalive 10 120\n")
	b.WriteString("persist-key\n")
	b.WriteString("persist-tun\n")
	b.WriteString("user nobody\n")
	b.WriteString("group nogroup\n")
	b.WriteString("status /var/log/openvpn/rr-status.log\n")
	b.WriteString("verb 3\n")
	b.WriteString("explicit-exit-notify 1\n")
	return b.String()
}

// RenderOpenVPNClientConf 生成单个客户端的 .ovpn 配置（内联证书版）。
// 证书材料由 certmgr 生成后传入，这个函数只负责拼装文本。
func RenderOpenVPNClientConf(c OpenVPNConfig, clientName, ca, cert, key, tlsCrypt, remote string) string {
	proto := c.Proto
	if proto == "" {
		proto = "udp"
	}
	port := c.Port
	if port == 0 {
		port = 1194
	}
	dev := c.DevType
	if dev == "" {
		dev = "tun"
	}
	cipher := c.Cipher
	if cipher == "" {
		cipher = "AES-256-GCM"
	}
	var b strings.Builder
	b.WriteString("# router-re OpenVPN client\n")
	b.WriteString("client\n")
	fmt.Fprintf(&b, "dev %s\n", dev)
	fmt.Fprintf(&b, "proto %s\n", proto)
	fmt.Fprintf(&b, "remote %s %d\n", remote, port)
	b.WriteString("resolv-retry infinite\n")
	b.WriteString("nobind\n")
	b.WriteString("persist-key\n")
	b.WriteString("persist-tun\n")
	b.WriteString("remote-cert-tls server\n")
	if strings.HasSuffix(cipher, "GCM") {
		fmt.Fprintf(&b, "data-ciphers %s\n", cipher)
	} else {
		fmt.Fprintf(&b, "cipher %s\n", cipher)
	}
	b.WriteString("auth SHA256\n")
	b.WriteString("verb 3\n")
	if clientName != "" {
		fmt.Fprintf(&b, "# client: %s\n", clientName)
	}
	writeInlineBlock(&b, "ca", ca)
	writeInlineBlock(&b, "cert", cert)
	writeInlineBlock(&b, "key", key)
	writeInlineBlock(&b, "tls-crypt", tlsCrypt)
	return b.String()
}

func writeInlineBlock(b *strings.Builder, tag, body string) {
	if strings.TrimSpace(body) == "" {
		return
	}
	fmt.Fprintf(b, "<%s>\n", tag)
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n")
	fmt.Fprintf(b, "</%s>\n", tag)
}

// RenderOpenVPNCommands 返回启用/停用 OpenVPN 的命令。
func RenderOpenVPNCommands(enabled bool) [][]string {
	if !enabled {
		return [][]string{
			{"systemctl", "stop", "openvpn-server@router-re"},
			{"systemctl", "disable", "openvpn-server@router-re"},
		}
	}
	return [][]string{
		{"systemctl", "enable", "openvpn-server@router-re"},
		{"systemctl", "restart", "openvpn-server@router-re"},
	}
}
