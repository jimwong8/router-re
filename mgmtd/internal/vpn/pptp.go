package vpn

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

// PPTPConfig 是 PPTP 服务端配置。
//
// 字段名与取值区间逐字对齐 iKuai 的 vpn/pptp/services 接口
// （enabled / server_ip / server_port / addr_pool / dns1 / dns2 /
// open_mppe / mtu / mru），这样从爱快导出的配置能直接照搬，
// 排查问题时两边的字段能一一对上。
//
// PPTP 本身早已不安全（MS-CHAPv2 可离线爆破），但大量存量客户端和
// 老式设备只认它，iKuai 至今保留。我们不改变这个事实，只在默认值上
// 站到安全一侧：open_mppe 默认 2（128 位），并拒绝弱口令账号。
type PPTPConfig struct {
	Enabled     bool      `json:"enabled"`
	ServerIP    string    `json:"server_ip"`   // 服务端在隧道内的地址
	ServerPort  int       `json:"server_port"` // 0 表示默认 1723
	AddrPool    string    `json:"addr_pool"`   // 如 192.168.20.100-192.168.20.200
	DNS1        string    `json:"dns1"`
	DNS2        string    `json:"dns2"`
	OpenMPPE    int       `json:"open_mppe"`    // 0=不加密 1=40位 2=128位
	MTU         int       `json:"mtu"`          // 1000-1492
	MRU         int       `json:"mru"`          // 1000-1492
	MaxSessions int       `json:"max_sessions"` // 0 表示不限
	Users       []VPNUser `json:"users"`        // 拨号账号（iKuai 在 auth/users 里单独管理）
}

// VPNUser 是 PPTP/L2TP 共用的拨号账号。
type VPNUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IP       string `json:"ip"` // 固定分配地址，必须在地址池内
	Comment  string `json:"comment"`
}

var vpnUserRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,32}$`)

// ValidatePPTP 校验 PPTP 配置。Enabled=false 时也校验已有字段，
// 避免"配一半存下来，真要开的时候才发现地址写错"。
func ValidatePPTP(c PPTPConfig) error {
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
	if c.OpenMPPE < 0 || c.OpenMPPE > 2 {
		return fmt.Errorf("MPPE 加密级别只能是 0/1/2，当前 %d", c.OpenMPPE)
	}
	if c.MTU != 0 && (c.MTU < 1000 || c.MTU > 1492) {
		return fmt.Errorf("MTU 必须在 1000-1492 之间，当前 %d", c.MTU)
	}
	if c.MRU != 0 && (c.MRU < 1000 || c.MRU > 1492) {
		return fmt.Errorf("MRU 必须在 1000-1492 之间，当前 %d", c.MRU)
	}
	if c.MaxSessions < 0 || c.MaxSessions > 1024 {
		return fmt.Errorf("最大会话数必须在 0-1024 之间")
	}
	for _, d := range []string{c.DNS1, c.DNS2} {
		if d == "" {
			continue
		}
		if v := net.ParseIP(d); v == nil || v.To4() == nil {
			return fmt.Errorf("DNS 不是合法 IPv4：%s", d)
		}
	}
	return validateVPNUsers(c.Users, pool)
}

// validateVPNUsers 是 PPTP/L2TP 共用的账号校验。
func validateVPNUsers(users []VPNUser, pool AddrPool) error {
	if len(users) > 256 {
		return fmt.Errorf("账号最多 256 个，当前 %d 个", len(users))
	}
	seen := map[string]bool{}
	for i, u := range users {
		if !vpnUserRe.MatchString(u.Username) {
			return fmt.Errorf("第 %d 个账号用户名非法（只允许字母数字 _ . -，1-32 位）：%q", i+1, u.Username)
		}
		key := strings.ToLower(u.Username)
		if seen[key] {
			return fmt.Errorf("账号重复：%s", u.Username)
		}
		seen[key] = true
		if len(u.Password) < 6 {
			return fmt.Errorf("账号 %s 的密码少于 6 位（弱口令等于没有认证）", u.Username)
		}
		if strings.ContainsAny(u.Password, "\n\r\t ") {
			return fmt.Errorf("账号 %s 的密码不能包含空白字符", u.Username)
		}
		if u.IP != "" {
			if v := net.ParseIP(u.IP); v == nil || v.To4() == nil {
				return fmt.Errorf("账号 %s 的固定地址不是合法 IPv4：%s", u.Username, u.IP)
			}
			if len(pool.Ranges) > 0 && !pool.Contains(u.IP) {
				return fmt.Errorf("账号 %s 的固定地址 %s 不在地址池内", u.Username, u.IP)
			}
		}
	}
	return nil
}

// RenderPptpdConf 生成 /etc/pptpd.conf。
func RenderPptpdConf(c PPTPConfig) string {
	var b strings.Builder
	b.WriteString("# router-re PPTP server\n")
	b.WriteString("option /etc/ppp/pptpd-options\n")
	b.WriteString("logwtmp\n")
	if c.ServerPort != 0 {
		fmt.Fprintf(&b, "listen %s:%d\n", c.ServerIP, c.ServerPort)
	}
	fmt.Fprintf(&b, "localip %s\n", c.ServerIP)
	pool, _ := ParseAddrPool(c.AddrPool)
	for _, r := range pool.Ranges {
		fmt.Fprintf(&b, "remoteip %s-%s\n", r.Start, r.End)
	}
	if c.MaxSessions > 0 {
		fmt.Fprintf(&b, "connections %d\n", c.MaxSessions)
	}
	return b.String()
}

// RenderPptpdOptions 生成 /etc/ppp/pptpd-options。
// open_mppe 决定 MPPE 强度：0 不加密，1 40 位，2 128 位（默认）。
func RenderPptpdOptions(c PPTPConfig) string {
	mtu := c.MTU
	if mtu == 0 {
		mtu = 1400
	}
	mru := c.MRU
	if mru == 0 {
		mru = mtu
	}
	mppe := c.OpenMPPE
	if mppe == 0 && !hasExplicitMPPE(c) {
		mppe = 2 // 未显式配置时默认 128 位，避免配出明文 PPTP
	}
	var b strings.Builder
	b.WriteString("# router-re PPTP options\n")
	b.WriteString("name pptpd\n")
	b.WriteString("refuse-pap\n")
	b.WriteString("refuse-chap\n")
	b.WriteString("refuse-mschap\n")
	b.WriteString("refuse-eap\n")
	b.WriteString("require-mschap-v2\n")
	switch mppe {
	case 0:
		b.WriteString("# MPPE disabled by config (open_mppe=0)\n")
	case 1:
		b.WriteString("require-mppe\n")
	default:
		b.WriteString("require-mppe-128\n")
	}
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
		fmt.Fprintf(&b, "ms-dns %s\n", d)
	}
	fmt.Fprintf(&b, "mtu %d\n", mtu)
	fmt.Fprintf(&b, "mru %d\n", mru)
	return b.String()
}

// hasExplicitMPPE 判断 OpenMPPE 是否被显式设过（0 是"不加密"也是零值，
// 无法与"未设置"区分；用 MaxSessions 之外的方式无法判断，因此约定
// 调用方在需要不加密时必须同时把 MTU 设为非零来标记"这是显式配置"。
func hasExplicitMPPE(c PPTPConfig) bool {
	return c.MTU != 0 || c.MRU != 0 || c.ServerPort != 0
}

// RenderChapSecrets 生成 /etc/ppp/chap-secrets。
// 每行：用户名 服务 密码 允许地址；固定地址账号第 4 列为该地址，否则 *。
func RenderChapSecrets(users []VPNUser) string {
	var b strings.Builder
	b.WriteString("# router-re PPTP/L2TP accounts\n")
	b.WriteString("# username	server	password	IP addresses\n")
	sorted := append([]VPNUser(nil), users...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Username < sorted[j].Username })
	for _, u := range sorted {
		addr := "*"
		if u.IP != "" {
			addr = u.IP
		}
		// 渲染层兜底：chap-secrets 每行是 "用户名 服务器 密码 IP"。
		// 用户名或密码带换行就能凭空多出一个账号（等于开一条 VPN 后门），
		// 带制表符则会把字段错位。这里统一压成单行并去掉分隔符。
		user := strings.NewReplacer("	", " ", "\n", " ", "\r", " ").Replace(u.Username)
		pass := strings.NewReplacer("	", " ", "\n", " ", "\r", " ").Replace(u.Password)
		ip := strings.NewReplacer("	", " ", "\n", " ", "\r", " ").Replace(addr)
		fmt.Fprintf(&b, "%s	*	%s	%s\n", user, pass, ip)
	}
	return b.String()
}

// RenderPPTPCommands 返回启用/停用 PPTP 需要执行的命令（不含写文件）。
func RenderPPTPCommands(enabled bool) [][]string {
	if !enabled {
		return [][]string{
			{"systemctl", "stop", "pptpd"},
			{"systemctl", "disable", "pptpd"},
		}
	}
	return [][]string{
		{"systemctl", "enable", "pptpd"},
		{"systemctl", "restart", "pptpd"},
	}
}

func sameSubnet(a, b net.IP, bits int) bool {
	mask := net.CIDRMask(bits, 32)
	return a.Mask(mask).Equal(b.Mask(mask))
}

func ip4ToUint(ip net.IP) uint32 {
	v := ip.To4()
	if v == nil {
		return 0
	}
	return uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
}
