package lb

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode"
)

// Member is one WAN link in the load balancer.
type Member struct {
	Name    string `json:"name"`
	Iface   string `json:"iface"`
	Gateway string `json:"gateway"`
	Weight  int    `json:"weight"`
	Enabled bool   `json:"enabled"`
}

// HealthCheck controls per-link reachability probing.
type HealthCheck struct {
	Target        string `json:"target"`
	IntervalSec   int    `json:"interval_sec"`
	TimeoutSec    int    `json:"timeout_sec"`
	FailThreshold int    `json:"fail_threshold"`
	RiseThreshold int    `json:"rise_threshold"`
}

// Config is the full load-balancer configuration.
type Config struct {
	Mode    string      `json:"mode"`
	Members []Member    `json:"members"`
	Health  HealthCheck `json:"health"`
}

// Mode semantics:
//
//	weighted: smooth weighted round-robin — share traffic by weight.
//	failover: first enabled member is primary; others are standby.
//	per-connection: each new connection is hashed to a member (sticky).
//	per-packet: round-robin per packet — generally not recommended as it
//	  causes out-of-order delivery for TCP.
// regexp 白名单：Iface 必须是合法的 Linux 接口名（IFNAMSIZ=15），
// 且会被渲染进注释行 —— ApplyIpRules 逐行执行，换行即可注入一条新命令。
var memberIfaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,14}$`)

// validMemberName 允许中文等 Unicode 字母数字，但拒绝空白与不可打印字符。
// 名字会进注释行 `# <name> (...)`；注释只保护到行尾，所以换行必须禁止。
// 同时禁止空白，避免名字被 strings.Fields 切成额外 argv。
func validMemberName(s string) bool {
	if s == "" || len([]rune(s)) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case r == '_' || r == '.' || r == '-':
		default:
			return false
		}
	}
	return true
}

// Validate checks the load-balancer configuration.
func Validate(c Config) error {
	switch c.Mode {
	case "weighted", "failover", "per-connection", "per-packet":
	default:
		return fmt.Errorf("mode must be weighted/failover/per-connection/per-packet, got %q", c.Mode)
	}
	if len(c.Members) == 0 {
		return fmt.Errorf("at least one member is required")
	}
	ifaces := map[string]string{}
	totalWeight := 0
	for _, m := range c.Members {
		// Name 会进注释行 `# <name> (...)`，换行即可注入一条被执行的命令。
		if !validMemberName(m.Name) {
			return fmt.Errorf("member name %q is invalid: 只允许字母/数字/_ . -（最长 64 字符），不能含空白或换行", m.Name)
		}
		// Iface 同样进注释行，且必须是合法的 Linux 接口名（IFNAMSIZ=15）。
		if !memberIfaceRe.MatchString(m.Iface) {
			return fmt.Errorf("member %q: iface %q is invalid: must match ^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,14}$", m.Name, m.Iface)
		}
		if strings.TrimSpace(m.Iface) == "" {
			return fmt.Errorf("member %q: iface required", m.Name)
		}
		if net.ParseIP(strings.TrimSpace(m.Gateway)) == nil {
			return fmt.Errorf("member %q: gateway must be an IP, got %q", m.Name, m.Gateway)
		}
		if m.Weight < 1 || m.Weight > 100 {
			return fmt.Errorf("member %q: weight must be in [1,100], got %d", m.Name, m.Weight)
		}
		totalWeight += m.Weight
		if prev, ok := ifaces[m.Iface]; ok {
			return fmt.Errorf("duplicate iface %q: used by %q and %q", m.Iface, prev, m.Name)
		}
		ifaces[m.Iface] = m.Name
	}
	if totalWeight <= 0 {
		return fmt.Errorf("total weight must be > 0, got %d", totalWeight)
	}
	h := c.Health
	if h.IntervalSec < 1 || h.IntervalSec > 300 {
		return fmt.Errorf("health.interval_sec must be in [1,300], got %d", h.IntervalSec)
	}
	if h.TimeoutSec < 1 || h.TimeoutSec > 60 {
		return fmt.Errorf("health.timeout_sec must be in [1,60], got %d", h.TimeoutSec)
	}
	// timeout must be smaller than the interval, otherwise probes overlap and
	// the failure counter grows faster than the rise counter can recover.
	if h.TimeoutSec >= h.IntervalSec {
		return fmt.Errorf("health.timeout_sec (%d) must be < health.interval_sec (%d)", h.TimeoutSec, h.IntervalSec)
	}
	if h.FailThreshold < 1 || h.FailThreshold > 10 {
		return fmt.Errorf("health.fail_threshold must be in [1,10], got %d", h.FailThreshold)
	}
	if h.RiseThreshold < 1 || h.RiseThreshold > 10 {
		return fmt.Errorf("health.rise_threshold must be in [1,10], got %d", h.RiseThreshold)
	}
	if net.ParseIP(strings.TrimSpace(h.Target)) == nil {
		return fmt.Errorf("health.target must be an IP (not a domain, to avoid DNS dependency), got %q", h.Target)
	}
	return nil
}
