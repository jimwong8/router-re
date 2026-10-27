// Package dhcp server-level apply/status (P5-1).
//
// 这些自由函数与已有的 *Applier 有所不同：
//   - Apply(r, c, confPath) 复用 execx.Runner + ServerConfig，写文件后
//     依次尝试 `systemctl restart dnsmasq` → `/etc/init.d/dnsmasq restart`，
//     两个方法都失败时把两个真实错误一起返回（不要只报文件路径）。
//   - Status(r) 通过 pgrep / systemctl is-active 判断运行状态，
//     并检查默认 confPath 是否存在。
package dhcp

import (
	"fmt"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// DefaultConfPath 是 dnsmasq 配置文件在 P5-1 服务端场景下的默认路径。
const DefaultConfPath = "/etc/dnsmasq.d/router-re.conf"

// Apply 校验 ServerConfig，写入 confPath，然后重启 dnsmasq。
// 重启策略：先 systemctl restart dnsmasq，失败则回退
// /etc/init.d/dnsmasq restart；两个都失败时返回合并错误。
func Apply(r execx.Runner, c ServerConfig, confPath string) error {
	if err := ValidateConfig(c); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	dir := ""
	if confPath != "" {
		dir = strings.TrimRight(confPath, "/")
		if i := strings.LastIndex(dir, "/"); i >= 0 {
			dir = dir[:i]
		}
	}
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	content := c.RenderDnsmasq()
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write dnsmasq config %s: %w", confPath, err)
	}

	// 方法 1：systemctl restart dnsmasq
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	}
	// 方法 2：/etc/init.d/dnsmasq restart
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	}
	// 两个方法都失败：抓取两个真实错误一起返回
	_, e1 := r.Run("systemctl", "restart", "dnsmasq")
	_, e2 := r.Run("/etc/init.d/dnsmasq", "restart")
	return fmt.Errorf("dnsmasq restart failed (systemctl: %v; init.d: %v)", e1, e2)
}

// Status 检测 dnsmasq 运行状态与默认配置文件是否存在。
func Status(r execx.Runner) (map[string]string, error) {
	detail := ""
	running := "false"

	// 方法 1：pgrep -a dnsmasq
	out, err := r.Run("pgrep", "-a", "dnsmasq")
	if err == nil && strings.TrimSpace(out) != "" {
		running = "true"
		detail = strings.TrimSpace(out)
	} else {
		// 方法 2：systemctl is-active dnsmasq
		out2, err2 := r.Run("systemctl", "is-active", "dnsmasq")
		if err2 == nil && strings.TrimSpace(strings.ToLower(out2)) == "active" {
			running = "true"
			detail = "active"
		} else {
			detail = "inactive"
		}
	}

	_, statErr := os.Stat(DefaultConfPath)
	confExists := "false"
	if statErr == nil {
		confExists = "true"
	}

	return map[string]string{
		"running":       running,
		"detail":        detail,
		"config_exists": confExists,
	}, nil
}
