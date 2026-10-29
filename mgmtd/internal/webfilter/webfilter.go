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
	sysErr := error(nil)
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	} else {
		sysErr = err
	}
	initErr := error(nil)
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	} else {
		initErr = err
	}
	// 报真实原因（服务不存在 / 无权限 / 配置语法错），而不是把文件路径
	// 当成"原因"抛出去 —— 那样运维看不出到底哪一步失败。
	return fmt.Errorf("配置已写入 %s，但 dnsmasq 未重启：systemctl: %v；init.d: %v", confPath, sysErr, initErr)
}

// ApplyWebfilter generates an nftables script that installs a table inet webfilter
// with a set blocked_domains and a forward chain dropping daddr in the set, then
// executes it through runner. When rules is empty the table is deleted instead.
// Returns the generated nft script text and any error from the runner.
func ApplyWebfilter(r execx.Runner, rules []Rule) (string, error) {
	if err := Validate(rules); err != nil {
		return "", err
	}
	var domains []string
	for _, rl := range rules {
		if !rl.Enabled {
			continue
		}
		if rl.Action != "block" {
			continue
		}
		domains = append(domains, rl.Domain)
	}
	if len(domains) == 0 {
		// delete table
		_, err := r.Run("nft", "delete", "table", "inet", "webfilter")
		if err != nil {
			return "", err
		}
		return "delete table inet webfilter\n", nil
	}
	var b strings.Builder
	b.WriteString("table inet webfilter {\n")
	b.WriteString("  set blocked_domains {\n")
	b.WriteString("    type ipv4_addr\n")
	b.WriteString("    flags interval\n")
	b.WriteString("    elements = { ")
	for i, d := range domains {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(d)
	}
	b.WriteString(" }\n")
	b.WriteString("  }\n")
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority 0; policy accept;\n")
	b.WriteString("    ip daddr @blocked_domains drop\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	script := b.String()
	_, err := r.RunWithInput(script, "nft", "-f", "-")
	if err != nil {
		return script, err
	}
	return script, nil
}
