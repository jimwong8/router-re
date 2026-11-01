package dnsproxy

import (
	"strings"
	"testing"
)

// 上游地址带换行时，即使整体被 "# " 注释掉的配置，第二行也会脱离注释
// 变成生效指令 —— 完整的 DNS 劫持。渲染层必须按 IP 字面量过滤。
func TestRenderDnsmasqNoInjection(t *testing.T) {
	c := Config{
		Enabled: false, // 关键：注释态下更容易漏（pfx="# " 只作用于第一行）
		Upstreams: []Upstream{
			{Address: "x\r\nserver=/evil.com/1.2.3.4", Port: 53},
			{Address: "1.1.1.1\nlocal=/evil.com/", Port: 53},
		},
		Hosts: []HostEntry{
			{IP: "1.1.1.1", Names: []string{"ok.com\nserver=/evil.com/1.2.3.4"}},
			{IP: "1.1.1.1\nserver=/evil.com/1.2.3.4", Names: []string{"ok.com"}},
		},
	}
	out := RenderDnsmasq(c)
	if strings.Contains(out, "server=/evil.com/") {
		t.Fatalf("DNS 劫持指令进入了配置:\n%s", out)
	}
	if strings.Contains(out, "local=/evil.com/") {
		t.Fatalf("注入指令进入了配置:\n%s", out)
	}
}

func TestRenderDnsmasqAcceptsLegit(t *testing.T) {
	c := Config{
		Enabled:      true,
		Upstreams:    []Upstream{{Address: "223.5.5.5", Port: 53}, {Address: "1.1.1.1", Source: "192.168.1.2"}},
		CacheSize:    10000,
		DomainNeeded: true,
		Hosts:        []HostEntry{{IP: "192.168.1.10", Names: []string{"nas.lan", "nas"}}},
	}
	out := RenderDnsmasq(c)
	for _, want := range []string{
		"server=223.5.5.5#53",
		"server=1.1.1.1#53@192.168.1.2",
		"cache-size=10000",
		"domain-needed",
		"address=/nas.lan/192.168.1.10",
		"address=/nas/192.168.1.10",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("合法配置未渲染，期望包含 %q\n实际:\n%s", want, out)
		}
	}
}
