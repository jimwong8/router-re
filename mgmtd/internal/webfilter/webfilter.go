package webfilter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Rule is a single domain filtering rule.
type Rule struct {
	Domain  string `json:"domain"`
	Action  string `json:"action"` // block | allow
	Enabled bool   `json:"enabled"`
}

// Validate checks rules for structural correctness.
func Validate(rs []Rule) error {
	for i, r := range rs {
		d := strings.TrimSpace(r.Domain)
		if d == "" {
			return fmt.Errorf("rule %d: domain required", i)
		}
		if strings.ContainsAny(d, " ,") {
			return fmt.Errorf("rule %d: domain must not contain space or comma: %q", i, d)
		}
		if strings.HasPrefix(d, ".") {
			return fmt.Errorf("rule %d: domain must not start with '.': %q", i, d)
		}
		if len(d) > 253 {
			return fmt.Errorf("rule %d: domain longer than 253 chars", i)
		}
		if r.Action != "block" && r.Action != "allow" {
			return fmt.Errorf("rule %d: action must be block or allow, got %q", i, r.Action)
		}
	}
	return nil
}

// RenderDnsmasq renders dnsmasq config lines for the rules. Blocked domains
// resolve to 0.0.0.0 (and ::); allow rules are recorded as comments since a
// real whitelist needs server=/<domain>/<upstream>.
func RenderDnsmasq(rs []Rule) string {
	var b strings.Builder
	for _, r := range rs {
		if !r.Enabled {
			continue
		}
		switch r.Action {
		case "block":
			fmt.Fprintf(&b, "address=/%s/0.0.0.0\n", r.Domain)
			fmt.Fprintf(&b, "address=/%s/::\n", r.Domain)
		case "allow":
			fmt.Fprintf(&b, "# allow %s（真实白名单需 server=/%s/<上游>）\n", r.Domain, r.Domain)
		}
	}
	return b.String()
}

// Apply validates, writes the config file and restarts dnsmasq.
func Apply(r execx.Runner, rs []Rule, confPath string) error {
	if err := Validate(rs); err != nil {
		return err
	}
	if confPath == "" {
		confPath = "/etc/dnsmasq.d/router-re-webfilter.conf"
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderDnsmasq(rs)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	}
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	}
	return fmt.Errorf("配置已写入但服务未重启：%s", confPath)
}
