package webfilter

import (
	"fmt"
	"net"
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
		// 换行/回车/制表符以前没挡。它们不是"空格或逗号"，但危害更大：
		// 域名会被原样拼进 dnsmasq 的 address=/<domain>/0.0.0.0，
		// 一个 "\nserver=/evil.com/1.2.3.4" 就能把任意域名劫持到指定 IP。
		// 这里统一按"控制字符与空白一律拒绝"处理，不再逐个字符补。
		if strings.ContainsAny(d, " 	\r\n\v\f,") {
			return fmt.Errorf("rule %d: domain must not contain whitespace, control characters or comma: %q", i, d)
		}
		if strings.HasPrefix(d, ".") {
			return fmt.Errorf("rule %d: domain must not start with '.': %q", i, d)
		}
		if len(d) > 253 {
			return fmt.Errorf("rule %d: domain longer than 253 chars (got %d)", i, len(d))
		}
		// 字面 IP 走 nft 集合（由 net.ParseIP 单独校验）；其余必须是域名形态，
		// 只允许字母数字、点、短横线、下划线和可选的 "*." 通配前缀。
		if net.ParseIP(d) == nil && !isPlainDomain(d) {
			return fmt.Errorf("rule %d: invalid domain %q (only letters, digits, '.', '-', '_' and a leading '*.' are allowed)", i, d)
		}
		if r.Action != "block" && r.Action != "allow" {
			return fmt.Errorf("rule %d: action must be block or allow, got %q", i, r.Action)
		}
	}
	return nil
}

// isPlainDomain 判断字符串是否只含域名字符集（允许前导 "*."）。
func isPlainDomain(d string) bool {
	s := strings.TrimPrefix(d, "*.")
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
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
		d := strings.TrimSpace(r.Domain)
		// 渲染层兜底：调用方若漏了 Validate，这里也不能把带换行/空白的
		// 域名写进 dnsmasq —— 那等于往配置里插任意指令（DNS 劫持）。
		if d == "" || strings.ContainsAny(d, " 	\r\n\v\f,") || !isPlainDomain(d) {
			continue
		}
		switch r.Action {
		case "block":
			fmt.Fprintf(&b, "address=/%s/0.0.0.0\n", d)
			fmt.Fprintf(&b, "address=/%s/::\n", d)
		case "allow":
			fmt.Fprintf(&b, "# allow %s（真实白名单需 server=/%s/<上游>）\n", d, d)
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

// ApplyWebfilter 把过滤规则真正落到系统上。
//
// 关键事实：nft 的 set 只能装 IP，装不了域名。原实现把 "example.com" 写进
// `type ipv4_addr` 的集合里，nft 加载时直接报语法错——面板显示下发成功、
// 实际一条都没生效。所以这里按规则类型分流：
//   - 域名 → dnsmasq（address=/domain/0.0.0.0），这是路由器上拦域名唯一靠谱的方式；
//   - 字面 IP → nft set（ipv4_addr），走 forward 丢包。
// 全部规则被删空时，把 dnsmasq 配置清空并删掉 nft 表，避免"删了还在拦"。
func ApplyWebfilter(r execx.Runner, rules []Rule) (string, error) {
	return ApplyWebfilterTo(r, rules, "")
}

// ApplyWebfilterTo 与 ApplyWebfilter 相同，但可指定 dnsmasq 配置路径。
// 存在的唯一理由是测试：测试绝不能写 /etc/dnsmasq.d（会污染本机、也会
// 因为无权限而失败）。
func ApplyWebfilterTo(r execx.Runner, rules []Rule, confPath string) (string, error) {
	if err := Validate(rules); err != nil {
		return "", err
	}

	var domains, ips []Rule
	for _, rl := range rules {
		if !rl.Enabled || rl.Action != "block" {
			continue
		}
		if net.ParseIP(strings.TrimSpace(rl.Domain)) != nil {
			ips = append(ips, rl)
		} else {
			domains = append(domains, rl)
		}
	}

	var out strings.Builder

	// 域名部分：写 dnsmasq 配置并重启（Apply 内部处理重启与真实报错）。
	if len(domains) > 0 {
		if err := Apply(r, domains, confPath); err != nil {
			return out.String(), err
		}
		out.WriteString(RenderDnsmasq(domains))
	} else if err := RemoveDnsmasq(r, confPath); err != nil {
		return out.String(), err
	}

	// IP 部分：只有真·IP 规则才进 nft 集合。
	if len(ips) == 0 {
		if _, err := r.Run("nft", "delete", "table", "inet", "webfilter"); err != nil {
			// 表不存在时 nft 会报错，不算失败。
			out.WriteString("# nft table inet webfilter 不存在，无需删除\n")
		}
		return out.String(), nil
	}

	var b strings.Builder
	b.WriteString("table inet webfilter {\n")
	b.WriteString("  set blocked_ips {\n")
	b.WriteString("    type ipv4_addr\n")
	b.WriteString("    flags interval\n")
	b.WriteString("    elements = { ")
	for i, rl := range ips {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strings.TrimSpace(rl.Domain))
	}
	b.WriteString(" }\n")
	b.WriteString("  }\n")
	b.WriteString("  chain forward {\n")
	b.WriteString("    type filter hook forward priority 0; policy accept;\n")
	b.WriteString("    ip daddr @blocked_ips drop\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")
	script := b.String()
	out.WriteString(script)
	_, err := r.RunWithInput(script, "nft", "-f", "-")
	if err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

// RemoveDnsmasq 清空 webfilter 的 dnsmasq 配置并重启，用于"规则全部删掉"
// 的场景。不重启的话 dnsmasq 会继续按旧配置拦截，用户看到的是"删了还在拦"。
func RemoveDnsmasq(r execx.Runner, confPath string) error {
	if confPath == "" {
		confPath = "/etc/dnsmasq.d/router-re-webfilter.conf"
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte("# router-re webfilter: 无启用规则\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	}
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	}
	return nil // 清空配置本身已生效，重启失败不阻断（下次 reload 会读到）
}
