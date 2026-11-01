package ddns

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// NTPConfig holds timesyncd settings.
type NTPConfig struct {
	Servers []string `json:"servers"`
	Enabled bool     `json:"enabled"`
}

// ntpServerRe 限定 NTP 服务器只能是主机名或 IP 字面量（含 IPv6 的方括号形态）。
//
// Servers 会被 stringsJoin(..., " ") 拼进一行 NTP=...。systemd 的配置里
// 换行就是语句分隔符，所以一个带 "\n" 的服务器字符串能凭空多出一条指令
// （例如 RootDistanceMaxSec=999999，把时钟同步的容差放到毫无意义）。
// 合法的多服务器写法用空格分隔，因此禁止空白不会损失任何功能。
var ntpServerRe = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]+$`)

// ValidateNTPConfig 校验 timesyncd 配置。
func ValidateNTPConfig(cfg NTPConfig) error {
	// systemd-timesyncd 的 NTP= 是空格分隔的列表，实测上限远低于 16；
	// 卡在 16 既够用又能防止一次请求写入超长配置。
	if len(cfg.Servers) > 16 {
		return fmt.Errorf("too many ntp servers: %d (max 16)", len(cfg.Servers))
	}
	for i, s := range cfg.Servers {
		if s == "" {
			return fmt.Errorf("servers[%d]: empty hostname", i)
		}
		if len(s) > 253 {
			return fmt.Errorf("servers[%d]: hostname too long (%d)", i, len(s))
		}
		if !ntpServerRe.MatchString(s) {
			return fmt.Errorf("servers[%d]: invalid hostname %q (whitespace and control characters are not allowed)", i, s)
		}
	}
	return nil
}

// RenderTimesyncd produces /etc/systemd/timesyncd.conf content.
func RenderTimesyncd(cfg NTPConfig) string {
	// 渲染层兜底：调用方若漏了 ValidateNTPConfig，这里也不能把带换行的
	// 服务器名写进配置 —— 那等于往 timesyncd.conf 里插任意指令。
	valid := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		if ntpServerRe.MatchString(s) {
			valid = append(valid, s)
		}
	}
	s := fmt.Sprintf("[Time]\nNTP=%s\n", stringsJoin(valid, " "))
	if !cfg.Enabled {
		s += "# Enabled=false commented out; keep above line for manual toggle.\n"
	}
	return s
}

func stringsJoin(a []string, sep string) string {
	if len(a) == 0 {
		return ""
	}
	s := a[0]
	for _, v := range a[1:] {
		s += sep + v
	}
	return s
}

// ApplyTimesyncd writes the conf file and restarts timesyncd.
func ApplyTimesyncd(r execx.Runner, cfg NTPConfig) error {
	conf := RenderTimesyncd(cfg)
	if err := os.WriteFile("/etc/systemd/timesyncd.conf", []byte(conf), 0644); err != nil {
		return fmt.Errorf("write timesyncd.conf: %w", err)
	}
	if _, err := r.Run("systemctl", "restart", "systemd-timesyncd"); err != nil {
		return fmt.Errorf("restart timesyncd: %w (conf written successfully; verify manually)", err)
	}
	return nil
}

// Status reads timedatectl output and returns parsed info.
func Status(r execx.Runner) (map[string]string, error) {
	out, err := r.Run("timedatectl", "show")
	if err != nil {
		return nil, fmt.Errorf("timedatectl failed: %w", err)
	}
	res := map[string]string{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(parts[0], " ", "_")))
			val := strings.TrimSpace(parts[1])
			res[key] = val
		}
	}
	return res, nil
}
