package webadmin

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Identity 描述主机身份。
type Identity struct {
	Hostname string `json:"hostname"`
	Domain   string `json:"domain"`   // 可空
	Location string `json:"location"` // 可空，机房/位置
}

// ValidateIdentity 校验主机名。
func ValidateIdentity(i Identity) error {
	h := strings.TrimSpace(i.Hostname)
	if h == "" {
		return fmt.Errorf("hostname required")
	}
	if len(h) > 63 {
		return fmt.Errorf("hostname must be 1-63 chars, got %d", len(h))
	}
	if err := validateDNSLabel(h); err != nil {
		return fmt.Errorf("hostname: %w", err)
	}
	if isAllDigits(h) {
		return fmt.Errorf("hostname %q is all digits — would be mistaken for an IP address", h)
	}
	d := strings.TrimSpace(i.Domain)
	if d != "" {
		if err := validateDNSLabel(d); err != nil {
			return fmt.Errorf("domain: %w", err)
		}
	}
	return nil
}

func validateDNSLabel(s string) error {
	if s == "" {
		return fmt.Errorf("empty label")
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return fmt.Errorf("label %q must not start or end with '-'", s)
	}
	for _, c := range s {
		if !(isAlnum(c) || c == '-') {
			return fmt.Errorf("label %q contains invalid char %q (only alphanumerics and '-')", s, c)
		}
	}
	return nil
}

func isAlnum(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ApplyIdentity 设置主机名并写 /etc/hosts。
//
// 1. hostnamectl set-hostname <hostname>
// 2. 先删旧的 127.0.1.1 行，再加新行（避免积累多条导致解析混乱）。
func ApplyIdentity(r execx.Runner, i Identity) (bool, string) {
	if err := ValidateIdentity(i); err != nil {
		return false, err.Error()
	}
	hn := strings.TrimSpace(i.Hostname)
	fqdn := hn
	if d := strings.TrimSpace(i.Domain); d != "" {
		fqdn = hn + "." + d
	}
	if _, err := r.Run("hostnamectl", "set-hostname", hn); err != nil {
		return false, fmt.Sprintf("hostnamectl set-hostname %s: %v", hn, err)
	}
	// 删旧 127.0.1.1 行再加新行 —— 先删后加，避免重复/混乱。
	if _, err := r.Run("sed", "-i", "/^127\\.0\\.1\\.1[[:space:]]/d", "/etc/hosts"); err != nil {
		return false, fmt.Sprintf("remove old 127.0.1.1 line: %v", err)
	}
	line := "127.0.1.1 " + fqdn + " " + hn
	if _, err := r.Run("sh", "-c", "echo '"+line+"' >> /etc/hosts"); err != nil {
		return false, fmt.Sprintf("append hosts line: %v", err)
	}
	return true, "hostname applied: " + fqdn
}
