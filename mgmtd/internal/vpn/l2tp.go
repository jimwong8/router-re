package vpn

import (
	"fmt"
	"net"
	"strings"
)

// L2TPConfig 是 L2TP/IPSec 服务端配置。
//
// 走 strongSwan + xl2tpd + pppd 这套经典组合：IPSec 负责传输加密，
// xl2tpd 负责隧道，pppd 复用 chap-secrets 做账号认证。
// 字段名对齐 iKuai 的 vpn/l2tp/services：server_ip / server_port / addr_pool /
// dns1 / dns2 / mtu / mru / ipsec_secret / force_ipsec / leftid / rightid。
//
// 只跑 xl2tpd 不做 IPSec 等于把明文 L2TP 暴露在公网，所以 force_ipsec=1
// （默认）时 ipsec_secret 为空直接拒绝启用。
type L2TPConfig struct {
	Enabled     bool      `json:"enabled"`
	ServerIP    string    `json:"server_ip"`
	ServerPort  int       `json:"server_port"` // 0 表示默认 1701
	AddrPool    string    `json:"addr_pool"`
	DNS1        string    `json:"dns1"`
	DNS2        string    `json:"dns2"`
	MTU         int       `json:"mtu"`          // 1000-1492
	MRU         int       `json:"mru"`          // 1000-1492
	IPSecSecret string    `json:"ipsec_secret"` // PSK，<=64
	ForceIPSec  int       `json:"force_ipsec"`  // 1=必须 IPSec（默认）0=允许纯 L2TP
	LeftID      string    `json:"leftid"`       // IPSec 本端身份，可空
	RightID     string    `json:"rightid"`      // IPSec 对端身份，可空
	LeftSubnet  string    `json:"left_subnet"`  // 允许客户端访问的内网段（可选）
	MaxSessions int       `json:"max_sessions"`
	Users       []VPNUser `json:"users"`
}

// ValidateL2TP 校验 L2TP 配置。
func ValidateL2TP(c L2TPConfig) error {
	pool, err := ParseAddrPool(c.AddrPool)
	if err != nil {
		return err
	}
	if err := pool.Validate(c.Enabled); err != nil {
		return err
	}
	if c.ServerIP == "" {
		return fmt.Errorf("服务端地址不能为空")
	}
	ip := net.ParseIP(c.ServerIP)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("服务端地址不是合法 IPv4：%s", c.ServerIP)
	}
	if pool.Contains(c.ServerIP) {
		return fmt.Errorf("服务端地址 %s 落在地址池内，会与拨号客户端冲突", c.ServerIP)
	}
	if c.ServerPort != 0 && (c.ServerPort < 1 || c.ServerPort > 65535) {
		return fmt.Errorf("服务端端口必须在 1-65535 之间")
	}
	if len(c.IPSecSecret) > 64 {
		return fmt.Errorf("IPSec 预共享密钥不能超过 64 个字符")
	}
	if strings.ContainsAny(c.IPSecSecret, "\"\n\r") {
		return fmt.Errorf("预共享密钥不能包含双引号或换行")
	}
	if c.ForceIPSec != 0 && c.ForceIPSec != 1 {
		return fmt.Errorf("force_ipsec 只能是 0 或 1")
	}
	if c.Enabled && c.ForceIPSec == 1 {
		if c.IPSecSecret == "" {
			return fmt.Errorf("启用 L2TP 必须配置 IPSec 预共享密钥（只跑 xl2tpd 等于明文传输）")
		}
		if len(c.IPSecSecret) < 8 {
			return fmt.Errorf("IPSec 预共享密钥至少 8 位")
		}
	}
	if c.MTU != 0 && (c.MTU < 1000 || c.MTU > 1492) {
		return fmt.Errorf("MTU 必须在 1000-1492 之间")
	}
	if c.MRU != 0 && (c.MRU < 1000 || c.MRU > 1492) {
		return fmt.Errorf("MRU 必须在 1000-1492 之间")
	}
	for _, d := range []string{c.DNS1, c.DNS2} {
		if d == "" {
			continue
		}
		if v := net.ParseIP(d); v == nil || v.To4() == nil {
			return fmt.Errorf("DNS 不是合法 IPv4：%s", d)
		}
	}
	if c.LeftSubnet != "" {
		if _, _, err := net.ParseCIDR(c.LeftSubnet); err != nil {
			return fmt.Errorf("内网段不是合法 CIDR：%s", c.LeftSubnet)
		}
	}
	if len(c.LeftID) > 100 || len(c.RightID) > 100 {
		return fmt.Errorf("IPSec 身份不能超过 100 个字符")
	}
	// LeftID/RightID 以前只查长度，不查内容。它们是自由文本，会原样写进
	// ipsec.conf（leftid=/rightid=）。ipsec.conf 以行为单位，换行就能插入
	// 任意配置指令——例如改掉 leftsubnet 或追加一条 conn。
	if err := validIPSecID("leftid", c.LeftID); err != nil {
		return err
	}
	if err := validIPSecID("rightid", c.RightID); err != nil {
		return err
	}
	return validateVPNUsers(c.Users, pool)
}

// validIPSecID 校验 ipsec.conf 里的身份字段（自由文本，但不能跨行）。
func validIPSecID(field, v string) error {
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s 不能包含控制字符或换行（发现 %#U）", field, r)
		}
	}
	return nil
}

// singleLine 把任意字符串压成单行，渲染层兜底用。
func singleLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '	' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderXl2tpdConf 生成 /etc/xl2tpd/xl2tpd.conf。
func RenderXl2tpdConf(c L2TPConfig) string {
	var b strings.Builder
	b.WriteString("# router-re L2TP server\n")
	b.WriteString("[global]\n")
	if c.ServerPort != 0 {
		fmt.Fprintf(&b, "port = %d\n", c.ServerPort)
	} else {
		b.WriteString("port = 1701\n")
	}
	b.WriteString("access control = no\n")
	b.WriteString("auth file = /etc/ppp/chap-secrets\n\n")
	b.WriteString("[lns default]\n")
	pool, _ := ParseAddrPool(c.AddrPool)
	for _, r := range pool.Ranges {
		fmt.Fprintf(&b, "ip range = %s-%s\n", r.Start, r.End)
	}
	fmt.Fprintf(&b, "local ip = %s\n", c.ServerIP)
	b.WriteString("require chap = yes\n")
	b.WriteString("refuse pap = yes\n")
	b.WriteString("require authentication = yes\n")
	b.WriteString("name = l2tpd\n")
	b.WriteString("ppp debug = no\n")
	b.WriteString("pppoptfile = /etc/ppp/options.xl2tpd\n")
	b.WriteString("length bit = yes\n")
	return b.String()
}

// RenderXl2tpdOptions 生成 /etc/ppp/options.xl2tpd。
func RenderXl2tpdOptions(c L2TPConfig) string {
	mtu := c.MTU
	if mtu == 0 {
		mtu = 1400
	}
	mru := c.MRU
	if mru == 0 {
		mru = mtu
	}
	var b strings.Builder
	b.WriteString("# router-re L2TP pppd options\n")
	b.WriteString("require-mschap-v2\n")
	b.WriteString("refuse-pap\n")
	b.WriteString("refuse-chap\n")
	b.WriteString("refuse-mschap\n")
	b.WriteString("refuse-eap\n")
	b.WriteString("proxyarp\n")
	b.WriteString("lock\n")
	b.WriteString("nobsdcomp\n")
	b.WriteString("nodeflate\n")
	b.WriteString("novj\n")
	b.WriteString("novjccomp\n")
	b.WriteString("nologfd\n")
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
		// 渲染层兜底：ms-dns 只接受 IP 字面量。校验层已用 net.ParseIP 挡过，
		// 但渲染器不该依赖调用方——一个换行就能插入任意 pppd 选项。
		if net.ParseIP(d) == nil {
			continue
		}
		fmt.Fprintf(&b, "ms-dns %s\n", d)
	}
	fmt.Fprintf(&b, "mtu %d\n", mtu)
	fmt.Fprintf(&b, "mru %d\n", mru)
	return b.String()
}

// RenderIPSecConf 生成 strongSwan 的 /etc/ipsec.d/router-re-l2tp.conf。
func RenderIPSecConf(c L2TPConfig) string {
	// 渲染层兜底：leftid/rightid 是自由文本，换行会插入任意 ipsec.conf 指令。
	leftID := singleLine(c.LeftID)
	if leftID == "" {
		leftID = c.ServerIP
	}
	var b strings.Builder
	b.WriteString("# router-re L2TP/IPSec\n")
	b.WriteString("config setup\n")
	b.WriteString("    uniqueids=no\n\n")
	b.WriteString("conn %default\n")
	b.WriteString("    keyingtries=%forever\n")
	b.WriteString("    ikelifetime=60m\n")
	b.WriteString("    keylife=20m\n")
	b.WriteString("    rekeymargin=3m\n\n")
	b.WriteString("conn L2TP-PSK\n")
	b.WriteString("    authby=secret\n")
	b.WriteString("    pfs=no\n")
	b.WriteString("    auto=add\n")
	b.WriteString("    keyingtries=3\n")
	b.WriteString("    rekey=no\n")
	b.WriteString("    left=%any\n")
	fmt.Fprintf(&b, "    leftid=%s\n", leftID)
	if c.LeftSubnet != "" {
		fmt.Fprintf(&b, "    leftsubnet=%s\n", c.LeftSubnet)
	}
	b.WriteString("    right=%any\n")
	b.WriteString("    rightprotoport=17/1701\n")
	b.WriteString("    rightsubnet=vhost:%priv,%no\n")
	b.WriteString("    dpddelay=10\n")
	b.WriteString("    dpdtimeout=30\n")
	b.WriteString("    dpdaction=clear\n")
	return b.String()
}

// RenderIPSecSecrets 生成 IPSec 预共享密钥文件内容。
func RenderIPSecSecrets(c L2TPConfig) string {
	if c.IPSecSecret == "" {
		return ""
	}
	return fmt.Sprintf("%%any %%any : PSK \"%s\"\n", c.IPSecSecret)
}

// RenderL2TPCommands 返回启用/停用 L2TP 需要执行的命令。
func RenderL2TPCommands(c L2TPConfig) [][]string {
	if !c.Enabled {
		return [][]string{
			{"systemctl", "stop", "xl2tpd"},
			{"systemctl", "disable", "xl2tpd"},
		}
	}
	cmds := [][]string{{"systemctl", "enable", "xl2tpd"}}
	if c.ForceIPSec == 1 && c.IPSecSecret != "" {
		cmds = append(cmds, []string{"systemctl", "restart", "strongswan-starter"})
	}
	return append(cmds, []string{"systemctl", "restart", "xl2tpd"})
}
