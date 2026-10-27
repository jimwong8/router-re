package ipv6

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// PDConfig holds DHCPv6 Prefix Delegation configuration.
type PDConfig struct {
	UpstreamIface   string `json:"upstream_iface"`   // 从哪条线上拿前缀（如 ppp0）
	DownstreamIface string `json:"downstream_iface"` // 分给哪个内网口
	PrefixLen       int    `json:"prefix_len"`       // 申请的前缀长度 32-64
	SubnetIDBits    int    `json:"subnet_id_bits"`   // 从委派前缀里切多少位做子网 0-16
	Enabled         bool   `json:"enabled"`
}

// ValidatePD checks PDConfig. The two interfaces must be non-empty, not "lo",
// and not identical.
func ValidatePD(c PDConfig) error {
	up := strings.TrimSpace(c.UpstreamIface)
	down := strings.TrimSpace(c.DownstreamIface)
	if up == "" {
		return fmt.Errorf("upstream_iface required")
	}
	if down == "" {
		return fmt.Errorf("downstream_iface required")
	}
	if up == "lo" || down == "lo" {
		return fmt.Errorf("iface must not be loopback (lo)")
	}
	if up == down {
		return fmt.Errorf("upstream_iface (%s) and downstream_iface must differ", up)
	}
	if c.PrefixLen < 32 || c.PrefixLen > 64 {
		return fmt.Errorf("prefix_len must be in [32,64], got %d", c.PrefixLen)
	}
	if c.SubnetIDBits < 0 || c.SubnetIDBits > 16 {
		return fmt.Errorf("subnet_id_bits must be in [0,16], got %d", c.SubnetIDBits)
	}
	return nil
}

// RenderDhcp6c generates a dhcp6c.conf fragment.
//
// interface <upstream> { send rapid-commit; request domain-name-servers, domain-name; send ia-pd 0; };
// id-assoc pd 0 { prefix-interface <downstream> { sla-id <subnet_id>; sla-len <bits>; }; };
func RenderDhcp6c(c PDConfig) string {
	pfx := pfxStr(c.Enabled)
	var b strings.Builder
	fmt.Fprintf(&b, "%sinterface %s {\n", pfx, c.UpstreamIface)
	fmt.Fprintf(&b, "%s	send rapid-commit;\n", pfx)
	fmt.Fprintf(&b, "%s	request domain-name-servers, domain-name;\n", pfx)
	fmt.Fprintf(&b, "%s	send ia-pd 0;\n", pfx)
	fmt.Fprintf(&b, "%s};\n", pfx)
	fmt.Fprintf(&b, "%sid-assoc pd 0 {\n", pfx)
	fmt.Fprintf(&b, "%s	prefix-interface %s {\n", pfx, c.DownstreamIface)
	fmt.Fprintf(&b, "%s		id 0;\n", pfx)
	fmt.Fprintf(&b, "%s		sla-id 0;\n", pfx)
	fmt.Fprintf(&b, "%s		sla-len %d;\n", pfx, c.SubnetIDBits)
	fmt.Fprintf(&b, "%s	};\n", pfx)
	fmt.Fprintf(&b, "%s};\n", pfx)
	return b.String()
}

func pfxStr(enabled bool) string {
	if !enabled {
		return "# "
	}
	return ""
}

// ParseDelegatedPrefix parses a "prefix <ipv6>/<len>" line as emitted by
// dhcp6c -f / found in DHCPv6 lease files. Returns (ip, prefixLen).
// Returns an explicit error when nothing is found (never a bare empty string).
func ParseDelegatedPrefix(text string) (string, int, error) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		// 匹配 "prefix 2409:8a5c:6419:d990::/60"
		if !strings.HasPrefix(strings.ToLower(line), "prefix ") {
			continue
		}
		rest := strings.TrimSpace(line[len("prefix"):])
		ci := strings.Index(rest, "/")
		if ci < 0 {
			return "", 0, fmt.Errorf("prefix line without '/' : %q", line)
		}
		ip := rest[:ci]
		lst := strings.TrimSpace(rest[ci+1:])
		n, err := parsePrefixLen(lst)
		if err != nil {
			return "", 0, fmt.Errorf("invalid prefix len %q in %q: %w", lst, line, err)
		}
		return ip, n, nil
	}
	return "", 0, fmt.Errorf("no delegated prefix found in output")
}

func parsePrefixLen(s string) (int, error) {
	// 取开头连续数字部分（忽略可能的 " prefix" 后缀等）。
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("no digits in prefix len %q", s)
	}
	n, err := atoi(s[:end])
	if err != nil {
		return 0, err
	}
	if n < 32 || n > 128 {
		return 0, fmt.Errorf("prefix len out of range [32,128]: %d", n)
	}
	return n, nil
}

func atoi(s string) (int, error) {
	n := 0
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit in %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// ApplyPD validates, writes dhcp6c.conf (0644, MkdirAll) and restarts dhcp6c
// (systemctl → init.d fallback). Both failure paths are reported together.
func ApplyPD(r execx.Runner, c PDConfig, confPath string) error {
	if err := ValidatePD(c); err != nil {
		return err
	}
	if confPath == "" {
		confPath = "/etc/dhcp6c.conf"
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderDhcp6c(c)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	// 重启 dhcp6c，两个都失败要把两个真实错误都报出来。
	var err1, err2 error
	if _, e := r.Run("systemctl", "restart", "dhcp6c"); e == nil {
		return nil
	} else {
		err1 = e
	}
	if _, e := r.Run("/etc/init.d/dhcp6c", "restart"); e == nil {
		return nil
	} else {
		err2 = e
	}
	return fmt.Errorf("dhcp6c restart failed: systemctl(%v); init.d(%v)", err1, err2)
}
