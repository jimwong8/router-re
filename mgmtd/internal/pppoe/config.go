package pppoe

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// Config 描述 PPPoE 服务器基本配置。
type Config struct {
	Interface   string   `json:"interface"`    // 物理接口，如 "eth0"
	LocalIP     string   `json:"local_ip"`     // 服务器本地 IP
	PoolStart   string   `json:"pool_start"`   // 客户端 IP 池起始
	PoolEnd     string   `json:"pool_end"`     // 客户端 IP 池结束
	DNS         []string `json:"dns"`          // 提供给客户端的 DNS
	ServiceName string   `json:"service_name"` // PPPoE 服务名
}

// User 描述一个 PPPoE 认证用户（chap-secrets 格式 "name" * "password" "ip"）。
type User struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	IP       string `json:"ip"`
}

// Validate 校验一个 PPPoE 拨号账号。
//
// chap-secrets 每条记录是 `"client" server "secret" "IP"`，字段用双引号包裹。
// 用户名或密码里出现一个引号就能闭合字段、改写后面的字段甚至新开一条记录
// ——等于凭空给 PPPoE 服务器加一个已知密码的账号。
// 这三个字段原来**完全没有校验**（Config.Validate 只管接口和地址池）。
func (u User) Validate() error {
	if u.Name == "" {
		return fmt.Errorf("账号名不能为空")
	}
	if len(u.Name) > 64 || len(u.Password) > 64 {
		return fmt.Errorf("账号名和密码都不能超过 64 字节")
	}
	for _, r := range u.Name {
		if !safePPPoEChar(r) {
			return fmt.Errorf("账号名不能包含空格、引号、反斜杠或控制字符（发现 %q）", r)
		}
	}
	for _, r := range u.Password {
		if !safePPPoEChar(r) {
			return fmt.Errorf("密码不能包含空格、引号、反斜杠或控制字符（发现 %q）", r)
		}
	}
	if u.IP != "" && u.IP != "*" && net.ParseIP(u.IP) == nil {
		return fmt.Errorf("固定 IP 必须是合法地址或 *（收到 %q）", u.IP)
	}
	return nil
}

// safePPPoEChar 判断字符能否安全进入 chap-secrets 的引号字段。
func safePPPoEChar(r rune) bool {
	if r < 0x20 || r == 0x7f {
		return false
	}
	switch r {
	case ' ', '"', '\'', '\\', '	':
		return false
	}
	return true
}

// Validate 校验 Config 字段：接口非空、IP 有效、Pool 范围合法、PoolStart <= PoolEnd（按数值比较）。
func (c Config) Validate() error {
	if c.Interface == "" {
		return fmt.Errorf("pppoe interface must not be empty")
	}
	if net.ParseIP(c.LocalIP) == nil {
		return fmt.Errorf("pppoe local_ip invalid: %s", c.LocalIP)
	}
	poolStartIP := net.ParseIP(c.PoolStart)
	if poolStartIP == nil {
		return fmt.Errorf("pppoe pool_start invalid: %s", c.PoolStart)
	}
	poolEndIP := net.ParseIP(c.PoolEnd)
	if poolEndIP == nil {
		return fmt.Errorf("pppoe pool_end invalid: %s", c.PoolEnd)
	}
	// IPv4 数值比较
	s := toUint32(poolStartIP.To4())
	e := toUint32(poolEndIP.To4())
	if s == 0 || e == 0 {
		return fmt.Errorf("pppoe pool must use IPv4 addresses")
	}
	if s > e {
		return fmt.Errorf("pppoe pool_start %s is greater than pool_end %s", c.PoolStart, c.PoolEnd)
	}
	if c.ServiceName == "" {
		return fmt.Errorf("pppoe service_name must not be empty")
	}
	// Interface 只查了非空，ServiceName 只查了非空，两者都是自由文本，
	// 又都被拼进注释行 "# PPPoE server options for %s (service: %s)"。
	// 注释只保护到行尾：一个换行就能让后面的内容变成生效指令
	// （例如 noauth —— 直接关掉 PPPoE 认证）。
	for _, r := range c.Interface {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == '@' || r == ':'
		if !ok {
			return fmt.Errorf("pppoe interface 只能包含字母数字和 _-.:@（发现 %q）", r)
		}
	}
	if len(c.Interface) > 15 {
		return fmt.Errorf("pppoe interface 名不能超过 15 字节")
	}
	for _, r := range c.ServiceName {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return fmt.Errorf("pppoe service_name 不能包含控制字符、引号或反斜杠（发现 %q）", r)
		}
	}
	return nil
}

func toUint32(ip net.IP) uint32 {
	b := ip.To4()
	if b == nil || len(b) != 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// RenderOptions 生成 /etc/ppp/pppoe-server-options 内容（每项一行）。
// 说明：-I（绑定接口）、-L（本地 IP）、-R（远端/池起始）、-N（池大小）由启动参数控制，
// 此文件只包含认证选项（require-chap / login / lcp-echo-interval / lcp-echo-failure / ms-dns / 注释）。
func RenderOptions(cfg Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	// 渲染层兜底：注释只保护到行尾，Interface/ServiceName 里的换行
	// 会让后面的内容变成生效指令（如 noauth 关掉认证）。
	iface := oneLine(cfg.Interface)
	svc := oneLine(cfg.ServiceName)
	var out string
	out += fmt.Sprintf("# PPPoE server options for %s (service: %s)\n", iface, svc)
	out += fmt.Sprintf("# -I %s -L %s -R %s -N (pool size) 由启动参数给出\n", iface, cfg.LocalIP, cfg.PoolStart)
	out += "require-chap\n"
	out += "# 启用登录认证\n"
	out += "login\n"
	out += "lcp-echo-interval 10\n"
	out += "lcp-echo-failure 2\n"
	for _, d := range cfg.DNS {
		// ms-dns 只接受 IP 字面量
		if d != "" && net.ParseIP(d) != nil {
			out += fmt.Sprintf("ms-dns %s\n", d)
		}
	}
	return out, nil
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

// stripQuote 去掉会破坏 chap-secrets 引号字段的字符。
func stripQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '"' || r == '\'' || r == '\\':
			// 丢弃
		case r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			// 丢弃：换行会新开一条记录
		case r < 0x20 || r == 0x7f:
			// 丢弃
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// RenderChapSecrets 生成 /etc/ppp/chap-secrets 内容（每用户一行）。
func RenderChapSecrets(users []User) string {
	var out string
	out += "# PPPoE CHAP secrets\n"
	for _, u := range users {
		if u.Name == "" {
			continue
		}
		// 渲染层兜底：chap-secrets 的字段用双引号包裹，引号/换行/制表符
		// 任何一个都能闭合字段、新开一条记录 —— 凭空多一个拨号账号。
		name := stripQuote(u.Name)
		pass := stripQuote(u.Password)
		ip := stripQuote(u.IP)
		if ip == "" {
			ip = "*"
		}
		out += fmt.Sprintf("\"%s\" * \"%s\" \"%s\"\n", name, pass, ip)
	}
	return out
}

// DefaultPaths 返回默认配置路径（用于 Apply 写文件时的默认值）。
func DefaultPaths() (optsPath, secretsPath string) {
	return filepath.Join("/etc", "ppp", "pppoe-server-options"), filepath.Join("/etc", "ppp", "chap-secrets.pppoe")
}
