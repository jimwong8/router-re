// Package tailscale 把系统里的 tailscaled 接进 router-re：渲染 `tailscale up` 参数、
// 解析 `tailscale status --json`，并把「参数生效了没有」用真实后端状态回报。
//
// 与 OpenVPN / EasyTier 的区别：这里不托管进程，tailscaled 由系统 systemd 管，
// 我们只通过它的 unix socket 下发命令。所以没有任何"启动成功"可以宣称，
// 唯一可信的结论来自 BackendState——`up` 命令返回 0 也可能停在 NeedsLogin。
package tailscale

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const defaultSocket = "/var/run/tailscale/tailscaled.sock"

// Config 是节点自身的设置（不是整网策略）。
type Config struct {
	Enabled bool `json:"enabled"`
	// AuthKey 只写不读；GET 返回空串 + KeySet。留空且节点已登录时表示"只改其它参数"。
	AuthKey string `json:"auth_key"`
	// 节点名；留空用系统主机名
	Hostname string `json:"hostname"`
	// 对外通告的子网，如 192.168.1.0/24
	AdvertiseRoutes []string `json:"advertise_routes"`
	// 作为出口节点（其他设备可经本机上网）
	AdvertiseExitNode bool `json:"advertise_exit_node"`
	// 接受其他节点通告的子网路由
	AcceptRoutes bool `json:"accept_routes"`
	// 接受 MagicDNS
	AcceptDNS bool `json:"accept_dns"`
	// 允许 SSH 到本机（Tailscale SSH）
	SSH bool `json:"ssh"`
	// 自建控制面（headscale）地址，留空用官方
	LoginServer string `json:"login_server"`
	// 重新认证（会重新走一次授权流程）
	ForceReauth bool `json:"force_reauth"`
}

// Default 返回默认配置。
func Default() Config {
	return Config{Enabled: false, AcceptDNS: true}
}

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func hasControl(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// Validate 校验配置。auth key 会作为进程参数出现，同样不能带控制字符。
func Validate(c Config) error {
	if hasControl(c.AuthKey) || hasControl(c.Hostname) || hasControl(c.LoginServer) {
		return fmt.Errorf("配置项含换行或控制字符")
	}
	if c.Hostname != "" && !hostnameRe.MatchString(c.Hostname) {
		return fmt.Errorf("节点名只能包含字母数字和连字符，且不能以连字符开头/结尾：%s", c.Hostname)
	}
	for _, r := range c.AdvertiseRoutes {
		r = strings.TrimSpace(r)
		if r == "" {
			return fmt.Errorf("通告子网不能为空")
		}
		ip, _, err := net.ParseCIDR(r)
		if err != nil || ip.To4() == nil {
			return fmt.Errorf("通告子网必须是 IPv4 CIDR，如 192.168.1.0/24：%s", r)
		}
	}
	if c.LoginServer != "" && !strings.HasPrefix(c.LoginServer, "http://") && !strings.HasPrefix(c.LoginServer, "https://") {
		return fmt.Errorf("控制面地址必须以 http:// 或 https:// 开头：%s", c.LoginServer)
	}
	if c.Enabled && c.AuthKey != "" && len(c.AuthKey) < 16 {
		return fmt.Errorf("auth key 看起来不完整（长度 %d）；官方 key 形如 tskey-auth-xxxx", len(c.AuthKey))
	}
	return nil
}

// BuildUpArgs 生成 `tailscale up` 的参数（不含 tailscale 自身与 --socket）。
// AuthKey 为空时不传 --authkey：节点已登录的情况下这就是"只改其它开关"。
func BuildUpArgs(c Config) []string {
	args := []string{"up"}
	if c.AuthKey != "" {
		args = append(args, "--authkey="+c.AuthKey)
	}
	if c.Hostname != "" {
		args = append(args, "--hostname="+c.Hostname)
	}
	if len(c.AdvertiseRoutes) > 0 {
		clean := make([]string, 0, len(c.AdvertiseRoutes))
		for _, r := range c.AdvertiseRoutes {
			if s := strings.TrimSpace(r); s != "" {
				clean = append(clean, s)
			}
		}
		if len(clean) > 0 {
			args = append(args, "--advertise-routes="+strings.Join(clean, ","))
		}
	}
	args = append(args, "--advertise-exit-node="+boolStr(c.AdvertiseExitNode))
	args = append(args, "--accept-routes="+boolStr(c.AcceptRoutes))
	args = append(args, "--accept-dns="+boolStr(c.AcceptDNS))
	args = append(args, "--ssh="+boolStr(c.SSH))
	if c.LoginServer != "" {
		args = append(args, "--login-server="+c.LoginServer)
	}
	if c.ForceReauth {
		args = append(args, "--force-reauth")
	}
	return args
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// MaskArgs 用于预览/日志：把 authkey 与任何形似密钥的值打码。
func MaskArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.HasPrefix(a, "--authkey=") {
			out[i] = "--authkey=tskey-********"
			continue
		}
		out[i] = a
	}
	return out
}

// SocketPath 返回 tailscaled 的 unix socket（MGM_TAILSCALE_SOCKET 可覆盖，
// 测试用独立实例时必须覆盖，否则会动到系统节点）。
func SocketPath() string {
	if p := os.Getenv("MGM_TAILSCALE_SOCKET"); p != "" {
		return p
	}
	return defaultSocket
}

// IsDefaultSocket 报告当前是否在操作系统自身的 tailscaled。
// 关掉它可能直接切断路由器的远程管理通道，界面上要按这个给警告。
func IsDefaultSocket() bool { return SocketPath() == defaultSocket }

// BinaryPath 返回 tailscale CLI 路径（MGM_TAILSCALE_BINARY 可覆盖）。
func BinaryPath() string {
	if p := os.Getenv("MGM_TAILSCALE_BINARY"); p != "" {
		return p
	}
	return "tailscale"
}

// BinaryAvailable 报告 CLI 是否存在。
func BinaryAvailable() bool {
	if strings.ContainsRune(BinaryPath(), os.PathSeparator) {
		_, err := os.Stat(BinaryPath())
		return err == nil
	}
	_, err := exec.LookPath(BinaryPath())
	return err == nil
}

// SocketExists 报告 unix socket 是否存在（tailscaled 没起来时立刻说不清，
// 否则用户只会看到一句含糊的 "failed to connect to local tailscaled"）。
func SocketExists() bool {
	_, err := os.Stat(SocketPath())
	return err == nil
}
