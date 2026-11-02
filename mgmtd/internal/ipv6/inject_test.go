package ipv6

import (
	"strings"
	"testing"
)

func raBase() RAConfig {
	return RAConfig{
		Iface: "eth0", Enabled: true, MaxInterval: 600,
		Prefix: "2001:db8::", PrefixLen: 64,
		ValidLifetime: 3600, PreferredLifetime: 1800,
	}
}

// Iface 被拼进 "interface %s {"。一个换行就能闭合当前块并开出新块，
// 给另一块网卡也广播 RA —— IPv6 前缀被通告到不该通的网段。
func TestValidateRARejectsIfaceInjection(t *testing.T) {
	for _, iface := range []string{
		"eth0\n}; interface eth1 {",
		"eth0\r\n}; interface eth1 {",
		"eth0 ",
		"eth0;x",
		strings.Repeat("a", 16),
	} {
		c := raBase()
		c.Iface = iface
		if err := ValidateRA(c); err == nil {
			t.Errorf("Iface=%q 未被拒绝", iface)
		}
	}
}

// 关闭状态用 "# " 注释整段，但前缀只加在第一行 —— 注入行会逃出注释。
func TestRenderRadvdNoInjection(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		c := raBase()
		c.Enabled = enabled
		c.Iface = "eth0\n}; interface eth1 {"
		out := RenderRadvd(c)
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			// 注入若成功，会出现"独占一行"的 interface eth1 块头
			if strings.HasPrefix(trimmed, "interface eth1") ||
				strings.HasPrefix(trimmed, "}; interface eth1") {
				t.Fatalf("enabled=%v 时注入产生了独立块:\n%s", enabled, out)
			}
		}
	}
}

func TestRenderRadvdDisabledFullyCommented(t *testing.T) {
	c := raBase()
	c.Enabled = false
	out := RenderRadvd(c)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			t.Errorf("关闭状态下有成活行: %q\n%s", line, out)
		}
	}
}

func TestRenderRadvdAcceptsLegit(t *testing.T) {
	out := RenderRadvd(raBase())
	for _, want := range []string{
		"interface eth0 {",
		"AdvSendAdvert on;",
		"prefix 2001:db8::/64",
		"AdvValidLifetime 3600;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}
