// Package dhcp 提供 DHCP 配置应用（真实执行 dnsmasq 写入与重载）与租约解析。
package dhcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Lease 描述 dnsmasq 租约文件中的一条租约记录。
// 解析自 /var/lib/misc/dnsmasq.leases。
type Lease struct {
	Expiry    int64  `json:"expiry"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	ClientID  string `json:"client_id"`
}

// Applier 执行 DHCP 配置的真实应用（写配置文件 + 重载 dnsmasq）。
// 使用 execx.Runner 执行外部命令（可被测试 FakeRunner 替代）。
type Applier struct {
	R        execx.Runner
	ConfPath string
}

// NewApplier 创建真实 Applier（使用 DefaultRunner 与标准配置路径）。
func NewApplier() *Applier {
	return &Applier{
		R:        execx.DefaultRunner{},
		ConfPath: "/etc/dnsmasq.d/router-re.conf",
	}
}

// Apply 将 DHCPConfig 渲染为 dnsmasq 配置并写入 ConfPath，
// 然后尝试重载 dnsmasq（逐步尝试：systemctl → init.d → kill -HUP 通过 PID 文件），
// 任一成功即返回 nil，全部失败则返回带 stderr 的错误。
func (a *Applier) Apply(cfg DHCPConfig) error {
	content := RenderDnsmasq(cfg)
	dir := filepath.Dir(a.ConfPath)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	if err := os.WriteFile(a.ConfPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write dnsmasq config %s: %w", a.ConfPath, err)
	}

	// 重载 dnsmasq：依次尝试三种方法。
	errReload := a.reloadDnsmasq()
	if errReload == nil {
		return nil
	}
	return fmt.Errorf("dnsmasq reload failed after 3 methods: %w", errReload)
}

func (a *Applier) reloadDnsmasq() error {
	// 方法 1: systemctl reload dnsmasq
	if _, e := a.R.Run("systemctl", "reload", "dnsmasq"); e == nil {
		return nil
	}
	// 方法 2: /etc/init.d/dnsmasq reload
	if _, e := a.R.Run("/etc/init.d/dnsmasq", "reload"); e == nil {
		return nil
	}
	// 方法 3: 通过 /var/run/dnsmasq.pid 读 PID，用 kill -HUP 发信号（只用明确 PID，不匹配进程名）
	pidData, e := os.ReadFile("/var/run/dnsmasq.pid")
	if e == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if pidStr != "" {
			if _, e := a.R.Run("kill", "-HUP", pidStr); e == nil {
				return nil
			}
		}
	}
	// 所有方法失败：合并错误信息
	_, e1 := a.R.Run("systemctl", "reload", "dnsmasq")
	_, e2 := a.R.Run("/etc/init.d/dnsmasq", "reload")
	return fmt.Errorf("systemctl: %v; init.d: %v; kill -HUP pid: %v", e1, e2, e)
}

// parseLeases 解析 dnsmasq 租约文件内容（每行 5 字段：expiry mac ip hostname clientid）。
// hostname 为 "*" 表示无主机名，转为空串。字段不足 4 的行跳过。
func parseLeases(data string) []Lease {
	var result []Lease
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		l := Lease{}
		if len(fields) >= 1 {
			_, _ = fmt.Sscanf(fields[0], "%d", &l.Expiry)
		}
		if len(fields) >= 2 {
			l.MAC = fields[1]
		}
		if len(fields) >= 3 {
			l.IP = fields[2]
		}
		if len(fields) >= 4 {
			l.Hostname = fields[3]
			if l.Hostname == "*" {
				l.Hostname = ""
			}
		}
		if len(fields) >= 5 {
			l.ClientID = fields[4]
		}
		result = append(result, l)
	}
	return result
}

// Leases 解析 /var/lib/misc/dnsmasq.leases，文件不存在时返回空切片和 nil 错误。
func (a *Applier) Leases() ([]Lease, error) {
	data, err := os.ReadFile("/var/lib/misc/dnsmasq.leases")
	if err != nil {
		if os.IsNotExist(err) {
			return []Lease{}, nil
		}
		return nil, fmt.Errorf("read leases: %w", err)
	}
	return parseLeases(string(data)), nil
}