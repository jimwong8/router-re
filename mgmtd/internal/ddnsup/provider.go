// Package ddnsup 提供 DDNS 服务商列表与记录校验/打码。
package ddnsup

import (
	"fmt"
	"net"
	"strings"
)

// Provider 描述一个 DDNS 服务商。
type Provider struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	NeedsToken bool   `json:"needs_token"`
	NeedsUser  bool   `json:"needs_user"`
	Endpoint   string `json:"endpoint"`
}

// Providers 返回内置支持列表（至少 5 个）。
func Providers() []Provider {
	return []Provider{
		{Key: "cloudflare", Name: "Cloudflare", NeedsToken: true, NeedsUser: false, Endpoint: "https://api.cloudflare.com/client/v4/zones/{zone}/dns_records/{id}"},
		{Key: "aliyun", Name: "Aliyun", NeedsToken: true, NeedsUser: false, Endpoint: "https://alidns.aliyuncs.com/"},
		{Key: "duckdns", Name: "DuckDNS", NeedsToken: true, NeedsUser: false, Endpoint: "https://www.duckdns.org/update?domains={domain}&token={token}&ip={ip}"},
		{Key: "noip", Name: "No-IP", NeedsToken: false, NeedsUser: true, Endpoint: "https://dynupdate.no-ip.com/nic/update?hostname={host}&myip={ip}"},
		{Key: "custom", Name: "Custom (generic HTTP)", NeedsToken: false, NeedsUser: false, Endpoint: "{endpoint}"},
	}
}

// FindProvider 按 key 查找；找不到返回 error 并列出可用 key。
func FindProvider(key string) (Provider, error) {
	for _, p := range Providers() {
		if p.Key == key {
			return p, nil
		}
	}
	keys := make([]string, 0, len(Providers()))
	for _, p := range Providers() {
		keys = append(keys, p.Key)
	}
	return Provider{}, fmt.Errorf("unknown provider %q; available keys: %s", key, strings.Join(keys, ", "))
}

// Record 是一条 DDNS 记录。
type Record struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Domain    string `json:"domain"`
	Subdomain string `json:"subdomain"`
	Token     string `json:"token,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Enabled   bool   `json:"enabled"`
	LastIP    string `json:"last_ip"`
	LastOK    int64  `json:"last_ok"`
	LastError string `json:"last_error"`
}

// Validate 校验记录。
func Validate(r Record) error {
	p, err := FindProvider(r.Provider)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.Domain) == "" || !strings.Contains(r.Domain, ".") || strings.ContainsAny(r.Domain, " \t\n") {
		return fmt.Errorf("domain must contain a dot and no whitespace, got %q", r.Domain)
	}
	if p.NeedsToken {
		if strings.TrimSpace(r.Token) == "" {
			return fmt.Errorf("provider %q requires token", r.Provider)
		}
		if len(r.Token) < 8 {
			return fmt.Errorf("token must be >= 8 chars, got %d", len(r.Token))
		}
	}
	if p.NeedsUser {
		if strings.TrimSpace(r.Username) == "" {
			return fmt.Errorf("provider %q requires username", r.Provider)
		}
		if strings.TrimSpace(r.Password) == "" {
			return fmt.Errorf("provider %q requires password", r.Provider)
		}
		if len(r.Password) < 8 {
			return fmt.Errorf("password must be >= 8 chars, got %d", len(r.Password))
		}
	}
	// Token/Password 长度 >= 8 的通用校验（太短几乎肯定是填错）
	if r.Token != "" && len(r.Token) < 8 {
		return fmt.Errorf("token must be >= 8 chars, got %d", len(r.Token))
	}
	if r.Password != "" && len(r.Password) < 8 {
		return fmt.Errorf("password must be >= 8 chars, got %d", len(r.Password))
	}
	return nil
}

// maskSecret 打码：保留前 4 后 4，长度 <= 8 全打码。
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

// Masked 返回打码副本。
func Masked(r Record) Record {
	out := r
	out.Token = maskSecret(r.Token)
	out.Password = maskSecret(r.Password)
	return out
}

// NeedsUpdate 判断是否需要更新（IP 变化）。
// 做规范化比较：IPv6 不同写法归一化后再比。
func NeedsUpdate(last, current string) bool {
	last = strings.TrimSpace(last)
	current = strings.TrimSpace(current)
	if last == "" {
		return true // 首次
	}
	if current == "" {
		return false
	}
	lastIP := net.ParseIP(last)
	curIP := net.ParseIP(current)
	if lastIP != nil && curIP != nil {
		return lastIP.String() != curIP.String()
	}
	return last != current
}
