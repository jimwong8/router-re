// Package snmp 提供 SNMP 服务端配置模型、渲染与下发。
//
// 设计参考 internal/dhcp/server_config.go 的五件套：Validate / Render /
// Apply / Status / QuerySystem。
//
// 安全约定：AuthPass / PrivPass 绝不能出现在任何错误信息或日志里，
// 但写入 snmpd.conf 时必须明文（文件权限 0600）。响应给前端时必须
// 被 Masking。
package snmp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// DefaultConfPath 是 snmpd 配置文件默认路径。
const DefaultConfPath = "/etc/snmp/snmpd.conf"

// Community v2c 团体。
type Community struct {
	Name   string `json:"name"`
	Access string `json:"access"` // "ro" 或 "rw"
}

// V3User SNMPv3 用户（snmpv3）。
type V3User struct {
	Name      string `json:"name"`
	AuthProto string `json:"auth_proto"` // md5/sha/sha256；空= noAuthNoPriv
	AuthPass  string `json:"auth_pass"`  // 最小 8 位；绝不进错误/日志
	PrivProto string `json:"priv_proto"` // des/aes
	PrivPass  string `json:"priv_pass"`  // 最小 8 位；绝不进错误/日志
}

// Config 完整的 SNMP 服务端配置。
type Config struct {
	Enabled     bool        `json:"enabled"`
	Port        int         `json:"port"`
	Location    string      `json:"location"`
	Contact     string      `json:"contact"`
	Communities []Community `json:"communities"`
	V3Users     []V3User    `json:"v3_users"`
	TrapTargets []string    `json:"trap_targets"`
}

// Masked 返回密码被打码为 "***" 的副本，用于 JSON 响应。
// 深拷贝 Slice，保证不修改原始 Config。
func (c Config) Masked() Config {
	out := c
	if len(c.V3Users) > 0 {
		out.V3Users = make([]V3User, len(c.V3Users))
		for i, u := range c.V3Users {
			out.V3Users[i] = u
			if u.AuthPass != "" {
				out.V3Users[i].AuthPass = "***"
			}
			if u.PrivPass != "" {
				out.V3Users[i].PrivPass = "***"
			}
		}
	}
	return out
}

var (
	validAuthProto = map[string]bool{"md5": true, "sha": true, "sha256": true}
	validPrivProto = map[string]bool{"des": true, "aes": true}
)

// Validate 校验 SNMP 配置。
func Validate(c Config) error {
	// Port：0 时默认 161。
	if c.Port == 0 {
		c.Port = 161
	} else if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port out of range 1-65535: %d", c.Port)
	}

	for i, com := range c.Communities {
		if len(com.Name) < 1 || len(com.Name) > 64 {
			return fmt.Errorf("communities[%d] name length must be 1-64", i)
		}
		if strings.ContainsAny(com.Name, " \t") {
			return fmt.Errorf("communities[%d] name must not contain spaces", i)
		}
		if com.Access != "ro" && com.Access != "rw" {
			return fmt.Errorf("communities[%d] access must be ro or rw", i)
		}
	}

	for i, u := range c.V3Users {
		if u.Name == "" {
			return fmt.Errorf("v3_users[%d] name is required", i)
		}
		// noAuthNoPriv：AuthProto 空 且 AuthPass 空。
		if u.AuthProto == "" && u.AuthPass == "" {
			// 不带 auth 的情况下不允许配置 priv。
			if u.PrivProto != "" || u.PrivPass != "" {
				return fmt.Errorf("v3_users[%d] priv requires auth to be configured", i)
			}
			continue
		}
		// 有 AuthPass 必有 AuthProto。
		if u.AuthPass != "" && u.AuthProto == "" {
			return fmt.Errorf("v3_users[%d] auth_proto required when auth_pass set", i)
		}
		if !validAuthProto[u.AuthProto] {
			return fmt.Errorf("v3_users[%d] auth_proto must be md5/sha/sha256", i)
		}
		// AuthPass 长度 >= 8 —— 错误里绝不能带密码。
		if len(u.AuthPass) < 8 {
			return fmt.Errorf("v3_users[%d] auth_pass too short (min 8)", i)
		}
		// priv 校验。
		if u.PrivProto != "" {
			if !validPrivProto[u.PrivProto] {
				return fmt.Errorf("v3_users[%d] priv_proto must be des/aes", i)
			}
			if len(u.PrivPass) < 8 {
				return fmt.Errorf("v3_users[%d] priv_pass too short (min 8)", i)
			}
		}
		if u.PrivPass != "" && u.PrivProto == "" {
			return fmt.Errorf("v3_users[%d] priv_proto required when priv_pass set", i)
		}
	}

	for i, t := range c.TrapTargets {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("trap_targets[%d] is empty", i)
		}
		if _, port, err := net.SplitHostPort(t); err == nil {
			if port == "" {
				return fmt.Errorf("trap_targets[%d] missing port", i)
			}
			continue
		}
		if net.ParseIP(t) == nil {
			return fmt.Errorf("trap_targets[%d] must be host:port or ip, got %s", i, t)
		}
	}

	if c.Enabled && len(c.Communities) == 0 && len(c.V3Users) == 0 {
		return errors.New("enabled config must define at least one community or v3 user")
	}
	return nil
}

// RenderSnmpd 将 Config 渲染为 snmpd.conf 风格文本。
func RenderSnmpd(c Config) string {
	if c.Port == 0 {
		c.Port = 161
	}
	var b strings.Builder
	prefix := func(line string) string {
		if !c.Enabled {
			return "# " + line
		}
		return line
	}
	b.WriteString(prefix(fmt.Sprintf("agentAddress udp:%d\n", c.Port)))
	if c.Location != "" {
		b.WriteString(prefix("sysLocation " + c.Location + "\n"))
	}
	if c.Contact != "" {
		b.WriteString(prefix("sysContact " + c.Contact + "\n"))
	}
	for _, com := range c.Communities {
		if com.Access == "rw" {
			b.WriteString(prefix("rwcommunity " + com.Name + "\n"))
		} else {
			b.WriteString(prefix("rocommunity " + com.Name + "\n"))
		}
	}
	needV3Comment := false
	for _, u := range c.V3Users {
		needV3Comment = true
		line := "createUser " + u.Name
		if u.AuthProto != "" {
			line += " " + u.AuthProto + " " + u.AuthPass
		}
		if u.PrivProto != "" {
			line += " " + u.PrivProto + " " + u.PrivPass
		}
		b.WriteString(prefix(line + "\n"))
	}
	if needV3Comment {
		b.WriteString(prefix("# 注意：createUser 行含明文密码，首次启动后应改为 rouser 并删除此行中的密码（snmpd 安全惯例）。\n"))
	}
	for _, t := range c.TrapTargets {
		b.WriteString(prefix("trapsink " + t + "\n"))
	}
	return b.String()
}

// Apply 校验并下发 SNMP 配置。
// 写文件 0644 后立刻 chmod 0600（含明文密码），再重启 snmpd：
// 先 systemctl restart snmpd，失败退 /etc/init.d/snmpd restart；
// 两个都失败时把两个真实错误一起返回。
func Apply(r execx.Runner, c Config, confPath string) error {
	if err := Validate(c); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	dir := ""
	if confPath != "" {
		if i := strings.LastIndex(confPath, "/"); i >= 0 {
			dir = confPath[:i]
		}
	}
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	content := RenderSnmpd(c)
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config %s: %w", confPath, err)
	}
	// 明文密码已写入 → 立即收紧权限到 0600。
	_ = os.Chmod(confPath, 0600)

	if _, err := r.Run("systemctl", "restart", "snmpd"); err == nil {
		return nil
	}
	if _, err := r.Run("/etc/init.d/snmpd", "restart"); err == nil {
		return nil
	}
	_, e1 := r.Run("systemctl", "restart", "snmpd")
	_, e2 := r.Run("/etc/init.d/snmpd", "restart")
	return fmt.Errorf("snmpd restart failed (systemctl: %v; init.d: %v)", e1, e2)
}

// Status 查询 snmpd 运行状态。
func Status(r execx.Runner) (map[string]string, error) {
	running := "false"
	detail := "inactive"
	// 方法 1：systemctl is-active snmpd
	if out, err := r.Run("systemctl", "is-active", "snmpd"); err == nil && strings.ToLower(strings.TrimSpace(out)) == "active" {
		running = "true"
		detail = "active"
	} else if out, err := r.Run("pgrep", "-a", "snmpd"); err == nil && strings.TrimSpace(out) != "" {
		running = "true"
		detail = strings.TrimSpace(out)
	}
	version := "unknown"
	if _, err := execx.Lookup("snmpd"); err == nil {
		if v, err := r.Run("snmpd", "-V"); err == nil {
			version = strings.TrimSpace(v)
		}
	}
	port := ""
	if data, err := os.ReadFile(DefaultConfPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "agentAddress udp:") {
				if p, err := strconv.Atoi(strings.TrimPrefix(line, "agentAddress udp:")); err == nil {
					port = strconv.Itoa(p)
				}
			}
		}
	}
	_, statErr := os.Stat(DefaultConfPath)
	confExists := "false"
	if statErr == nil {
		confExists = "true"
	}
	return map[string]string{
		"running":       running,
		"port":          port,
		"version":       version,
		"config_exists": confExists,
		"detail":        detail,
	}, nil
}

// QuerySystem 查询本机 SNMP 系统信息。
// 未安装 snmpwalk 时返回明确错误 "snmpwalk not installed"。
func QuerySystem(r execx.Runner) (map[string]string, error) {
	if _, err := execx.Lookup("snmpwalk"); err != nil {
		return nil, errors.New("snmpwalk not installed")
	}
	out, err := r.Run("snmpwalk", "-v2c", "-c", "public", "127.0.0.1", "sysDescr")
	if err != nil {
		return nil, fmt.Errorf("snmpwalk: %w", err)
	}
	return map[string]string{"sysDescr": strings.TrimSpace(out)}, nil
}
