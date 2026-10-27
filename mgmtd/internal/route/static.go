package route

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// StaticRoute describes a static routing entry.
type StaticRoute struct {
	Dest    string `json:"dest"`
	Gateway string `json:"gateway"`
	Device  string `json:"device"`
	Metric  int    `json:"metric"`
}

// SplitPriority defines the iKuai priority chain for routing rules.
// 静态路由 > 域名规则 > 端口规则 > 协议规则 > 多线负载 > 默认网关
var SplitPriority = []string{
	"static_route",
	"domain",
	"port",
	"protocol",
	"multilink",
	"default_gateway",
}

// RenderStaticCommands generates ip route command list for the given routes.
func RenderStaticCommands(routes []StaticRoute) []string {
	var out []string
	for _, r := range routes {
		cmd := fmt.Sprintf("ip route replace %s", r.Dest)
		if r.Gateway != "" {
			cmd += fmt.Sprintf(" via %s", r.Gateway)
		}
		if r.Device != "" {
			cmd += fmt.Sprintf(" dev %s", r.Device)
		}
		if r.Metric > 0 {
			cmd += fmt.Sprintf(" metric %d", r.Metric)
		}
		out = append(out, cmd)
	}
	return out
}

// ApplyStatic executes the rendered static route commands via the given Runner.
func ApplyStatic(r execx.Runner, routes []StaticRoute) error {
	path, err := execx.Lookup("ip")
	if err != nil {
		return fmt.Errorf("ip command not found: %w", err)
	}
	for _, cmdStr := range RenderStaticCommands(routes) {
		parts := strings.Split(cmdStr, " ")
		if len(parts) < 2 {
			continue
		}
		// Skip the "ip" prefix since Lookup gives absolute path.
		args := parts[1:]
		if _, err := r.Run(path, args...); err != nil {
			return fmt.Errorf("apply static route %v failed: %w", cmdStr, err)
		}
	}
	return nil
}
