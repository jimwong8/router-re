package remote

import (
	"fmt"
	"net"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Access 描述远程访问通道的配置。端口与来源 CIDR 校验见 ValidateAccess；
// 变更应用时只有 SSH/Web/Telnet 的 systemctl 开关会被真的执行，端口与监听项只会
// 返回"需要人工处理"提示（见 ApplyAccess 注释，改错 sshd_config 会锁死自己）。
type Access struct {
	SSHEnabled    bool     `json:"ssh_enabled"`
	SSHPort       int      `json:"ssh_port"`
	WebEnabled    bool     `json:"web_enabled"`
	WebPort       int      `json:"web_port"`
	AllowSources  []string `json:"allow_sources"`
	TelnetEnabled bool     `json:"telnet_enabled"`
	FtpEnabled    bool     `json:"ftp_enabled"`
	FtpPort       int      `json:"ftp_port"`
}

// ValidateAccess 检查 Access 的静态合法性：
//   - SSH/Web 端口（均 1-65535）且二者不能相同；
//   - AllowSources 每项必须是合法 CIDR。
//
// 返回的错误是"硬性非法"，调用方应拒绝保存；Telnet 的明文警告走 AccessWarnings，
// 不会被当成错误拒绝。
func ValidateAccess(a Access) error {
	if a.SSHPort < 1 || a.SSHPort > 65535 {
		return fmt.Errorf("ssh_port %d 超出 1-65535", a.SSHPort)
	}
	if a.WebPort < 1 || a.WebPort > 65535 {
		return fmt.Errorf("web_port %d 超出 1-65535", a.WebPort)
	}
	if a.FtpPort != 0 && (a.FtpPort < 1 || a.FtpPort > 65535) {
		return fmt.Errorf("ftp_port %d 超出 1-65535", a.FtpPort)
	}
	if a.SSHPort == a.WebPort {
		return fmt.Errorf("ssh_port 与 web_port 不能相同（都是 %d）", a.SSHPort)
	}
	// FTP 端口不能和 SSH/Web 撞：撞了就是一个服务静默抢不到端口，
	// 面板上显示 enabled、实际连不上。
	if a.FtpEnabled && a.FtpPort != 0 {
		if a.FtpPort == a.SSHPort {
			return fmt.Errorf("ftp_port 与 ssh_port 不能相同（都是 %d）", a.FtpPort)
		}
		if a.FtpPort == a.WebPort {
			return fmt.Errorf("ftp_port 与 web_port 不能相同（都是 %d）", a.FtpPort)
		}
	}
	for _, cidr := range a.AllowSources {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if _, _, err := parseCIDR(cidr); err != nil {
			return fmt.Errorf("allow_sources 含非法 CIDR %q: %v", cidr, err)
		}
	}
	return nil
}

// parseCIDR 封装 net.ParseCIDR，返回 (网络, 掩码长度)。
func parseCIDR(s string) (net.IPNet, int, error) {
	ip, network, err := net.ParseCIDR(s)
	if err != nil {
		return net.IPNet{}, 0, err
	}
	ones, _ := network.Mask.Size()
	_ = ip
	return *network, ones, nil
}

// AccessWarnings 返回"不影响保存但值得备注"的安全警告。
// 目前唯一一条：Telnet 是明文协议，建议改用 SSH。
func AccessWarnings(a Access) []string {
	var warnings []string
	if a.TelnetEnabled {
		warnings = append(warnings, "Telnet 是明文协议，凭据与流量不加密，建议改用 SSH。")
	}
	if a.FtpEnabled {
		warnings = append(warnings, "FTP 是明文协议（含密码），建议改用 SFTP/FTPS 或仅在受信网段开放。")
	}
	return warnings
}

// ApplyAccess 应用远程访问配置。返回 "需要人工处理" 的项列表，以及执行错误。
//
// 设计：只安全地执行 systemctl 开关（enable/disable --now），Web/SSH 的端口与监听
// 需要改 sshd_config / web 配置，这两处一旦写错会把管理员锁在门外，因此**绝不自动改
// 文件**，只把提示放进返回列表由人工确认。错误仅表示执行层面失效（无 systemctl、
// 无权限等），不影响人工提示收集。
func ApplyAccess(r execx.Runner, a Access) ([]string, error) {
	if r == nil {
		r = execx.DefaultRunner{}
	}
	var pending []string

	// 端口与会话配置一律不进自动执行 —— 返回"需要人工处理"。
	if a.SSHPort != 0 && a.SSHPort != 22 {
		pending = append(pending, fmt.Sprintf("SSH 端口需为 %d，请人工修改 /etc/ssh/sshd_config 的 Port 并 systemctl reload ssh。", a.SSHPort))
	}
	if a.WebPort != 0 && a.WebPort != 443 && a.WebPort != 80 {
		pending = append(pending, fmt.Sprintf("Web 端口需为 %d，请人工调整 web 监听配置。", a.WebPort))
	}
	if len(a.AllowSources) > 0 {
		pending = append(pending, fmt.Sprintf("请在下发防火墙规则前确认来源白名单 %v 已生效。", a.AllowSources))
	}
	if a.TelnetEnabled {
		pending = append(pending, "Telnet 已启用，这是明文协议，请确认在受控网络内使用。")
	}

	// 只执行安全的 systemctl 开关。
	for _, svc := range serviceActions(a) {
		if svc.action == "none" {
			continue
		}
		action := svc.action
		if _, err := r.Run("systemctl", action, "--now", svc.unit); err != nil {
			return pending, fmt.Errorf("systemctl %s --now %s 失败: %w", action, svc.unit, err)
		}
	}
	return pending, nil
}

// serviceAction 描述单个 systemd 单元期望动作。
type serviceAction struct {
	unit   string
	action string // "enable" / "disable" / "none"
}

// serviceActions 将 Access 布尔开关翻译成稳定的 systemd 动作序列（便于测试断言）。
func serviceActions(a Access) []serviceAction {
	out := []serviceAction{
		{unit: "ssh", action: onOff(a.SSHEnabled, true)},
		{unit: "telnet", action: onOff(a.TelnetEnabled, true)},
		{unit: "vsftpd", action: onOff(a.FtpEnabled, true)},
		{unit: "nginx.service", action: onOff(a.WebEnabled, true)},
	}
	return out
}

// onOff 把布尔开关翻译成 enable/disable/none。
func onOff(enabled, supportsDisable bool) string {
	switch {
	case enabled:
		return "enable"
	case supportsDisable:
		return "disable"
	default:
		return "none"
	}
}

// AccessStatus 读取当前远程访问运行状态。返回键：
//   - ssh_active    : "active" / "inactive"
//   - ssh_port      : sshd 实际监听端口（缺省 22）
//   - ssh_listen    : sshd 监听地址（尽力解析，可能为空）
//   - telnet_active : "active" / "inactive"
func AccessStatus(r execx.Runner) (map[string]string, error) {
	if r == nil {
		r = execx.DefaultRunner{}
	}
	out := map[string]string{
		"ssh_active":    "inactive",
		"ssh_port":      "22",
		"ssh_listen":    "",
		"telnet_active": "inactive",
		"ftp_active":    "inactive",
		"ftp_port":      "21",
	}
	if out2, err := r.Run("systemctl", "is-active", "vsftpd"); err == nil {
		st := strings.TrimSpace(out2)
		if st == "active" {
			out["ftp_active"] = "active"
		}
	} else if out2, err := r.Run("systemctl", "is-active", "ftp"); err == nil {
		st := strings.TrimSpace(out2)
		if st == "active" {
			out["ftp_active"] = "active"
		}
	}
	if ports, err := sshPorts(r); err == nil && len(ports) > 0 {
		out["ssh_port"] = strings.Join(ports, ",")
	}
	return out, nil
}

// sshPorts 尽力解析 sshd 监听端口（可能多个）。优先 systemctl show，回退 ss -tlnp。
func sshPorts(r execx.Runner) ([]string, error) {
	var ports []string
	if out, err := r.Run("systemctl", "show", "ssh", "-p", "ListenStream"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "ListenStream=") {
				continue
			}
			v := strings.TrimPrefix(line, "ListenStream=")
			if v = strings.TrimSpace(v); v != "" {
				// 形如 "[::]:22" 或 "0.0.0.0:2222"，取最后冒号后端口
				if idx := strings.LastIndex(v, ":"); idx >= 0 {
					ports = append(ports, v[idx+1:])
				}
			}
		}
		if len(ports) > 0 {
			return ports, nil
		}
	}
	if out, err := r.Run("ss", "-tlnp"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if !strings.Contains(line, "ssh") && !strings.Contains(line, ":22 ") {
				continue
			}
			for _, f := range strings.Fields(line) {
				if strings.Contains(f, ":") {
					if idx := strings.LastIndex(f, ":"); idx >= 0 {
						p := f[idx+1:]
						if p != "" && p != "*" {
							ports = append(ports, p)
						}
					}
				}
			}
		}
	}
	return ports, nil
}
