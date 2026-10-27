package pppoe

import (
	"fmt"
	"net"
	"path/filepath"
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
	var out string
	out += fmt.Sprintf("# PPPoE server options for %s (service: %s)\n", cfg.Interface, cfg.ServiceName)
	out += fmt.Sprintf("# -I %s -L %s -R %s -N (pool size) 由启动参数给出\n", cfg.Interface, cfg.LocalIP, cfg.PoolStart)
	out += "require-chap\n"
	out += "# 启用登录认证\n"
	out += "login\n"
	out += "lcp-echo-interval 10\n"
	out += "lcp-echo-failure 2\n"
	for _, d := range cfg.DNS {
		if d != "" {
			out += fmt.Sprintf("ms-dns %s\n", d)
		}
	}
	return out, nil
}

// RenderChapSecrets 生成 /etc/ppp/chap-secrets 内容（每用户一行）。
func RenderChapSecrets(users []User) string {
	var out string
	out += "# PPPoE CHAP secrets\n"
	for _, u := range users {
		if u.Name != "" {
			out += fmt.Sprintf("\"%s\" * \"%s\" \"%s\"\n", u.Name, u.Password, u.IP)
		}
	}
	return out
}

// DefaultPaths 返回默认配置路径（用于 Apply 写文件时的默认值）。
func DefaultPaths() (optsPath, secretsPath string) {
	return filepath.Join("/etc", "ppp", "pppoe-server-options"), filepath.Join("/etc", "ppp", "chap-secrets.pppoe")
}
