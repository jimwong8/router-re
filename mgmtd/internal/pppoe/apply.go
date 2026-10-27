package pppoe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Apply 写入选项与 chap-secrets 文件并重启 pppoe-server（真实执行）。
// 返回 (applied bool, detail string, err error)。如果配置文件已写入但服务重启失败，
// 返回 applied=false 且 detail 说明原因（不谎称成功）。
func Apply(r execx.Runner, cfg Config, users []User, optsPath, secretsPath string) (bool, string, error) {
	if err := cfg.Validate(); err != nil {
		return false, fmt.Sprintf("invalid config: %v", err), err
	}
	// 写选项文件（自动 MkdirAll 父目录）
	optsDir := filepath.Dir(optsPath)
	if optsDir != "." && optsDir != "/" {
		_ = os.MkdirAll(optsDir, 0755)
	}
	optsText, err := RenderOptions(cfg)
	if err != nil {
		return false, fmt.Sprintf("render options failed: %v", err), err
	}
	if err := os.WriteFile(optsPath, []byte(optsText), 0644); err != nil {
		return false, fmt.Sprintf("write options file %s: %v", optsPath, err), err
	}
	// 写 chap-secrets 文件
	secretsDir := filepath.Dir(secretsPath)
	if secretsDir != "." && secretsDir != "/" {
		_ = os.MkdirAll(secretsDir, 0755)
	}
	secretsText := RenderChapSecrets(users)
	if err := os.WriteFile(secretsPath, []byte(secretsText), 0644); err != nil {
		return false, fmt.Sprintf("write secrets file %s: %v", secretsPath, err), err
	}
	// 重启 pppoe-server：先 systemctl，再 init.d，都失败则说明配置已写入但服务未启动
	details := []string{}
	_, err1 := r.Run("systemctl", "restart", "pppoe-server")
	if err1 == nil {
		return true, "pppoe-server restarted via systemctl", nil
	}
	details = append(details, fmt.Sprintf("systemctl restart failed: %v", err1))
	_, err2 := r.Run("/etc/init.d/pppoe-server", "restart")
	if err2 == nil {
		return true, "pppoe-server restarted via init script", nil
	}
	details = append(details, fmt.Sprintf("init.d restart failed: %v", err2))
	// 配置文件已写入但服务未启动：不谎报 applied
	msg := fmt.Sprintf("配置文件已写入（%s, %s）但服务未启动：%s", optsPath, secretsPath, strings.Join(details, "; "))
	return false, msg, fmt.Errorf("%s", msg)
}

// Status 检查 pppoe-server 是否运行（通过 pidof）。
func Status(r execx.Runner) (running bool, detail string, err error) {
	out, err := r.Run("pidof", "pppoe-server")
	if err == nil && strings.TrimSpace(out) != "" {
		return true, fmt.Sprintf("pppoe-server PID(s): %s", out), nil
	}
	return false, "pppoe-server not running", nil // 不将缺失视为错误
}
