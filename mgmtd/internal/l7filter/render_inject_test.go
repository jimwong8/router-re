package l7filter

import (
	"strings"
	"testing"
)

// 域名里带换行会拼出额外的 dnsmasq 指令（"server=/evil.com/1.2.3.4"），
// 等于把任意域名劫持到指定 IP。渲染层必须丢弃而不是原样写出。
func TestRenderDnsmasqBlockNoInjection(t *testing.T) {
	payloads := []string{
		"x\nserver=/evil.com/1.2.3.4",
		"x\r\nserver=/evil.com/1.2.3.4",
		"x\naddress=/evil.com/0.0.0.0",
		"evil.com server=1.1.1.1",
		"evil.com;server=/a/1.1.1.1",
		"evil.com\tfoo",
	}
	for _, p := range payloads {
		out := RenderDnsmasqBlock([]string{p})
		if out != "" {
			t.Errorf("payload %q 应被丢弃，实际渲染出:\n%s", p, out)
		}
	}
}

func TestRenderDnsmasqBlockAcceptsLegitDomains(t *testing.T) {
	out := RenderDnsmasqBlock([]string{"evil.com", "*.ads.example.org", "a-b.c_d.example"})
	for _, want := range []string{
		"address=/evil.com/0.0.0.0",
		"address=/evil.com/::",
		"address=/*.ads.example.org/0.0.0.0",
		"address=/a-b.c_d.example/0.0.0.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("合法域名未渲染，期望包含 %q\n实际:\n%s", want, out)
		}
	}
}

// IP 列表里带换行会把 nft 的 set 结构写坏。
func TestRenderNftIpBlockNoInjection(t *testing.T) {
	payloads := []string{
		"1.2.3.4\nflush ruleset",
		"1.2.3.4 } chain evil {",
		"1.2.3.4, 5.6.7.8",
		"1.2.3.4/33",
		"not-an-ip",
	}
	for _, p := range payloads {
		out := RenderNftIpBlock([]string{p})
		if strings.Contains(out, "flush ruleset") || strings.Contains(out, "chain evil") {
			t.Errorf("payload %q 进入了渲染结果:\n%s", p, out)
		}
		// 元素集合里不应出现该 payload
		if strings.Contains(out, p) {
			t.Errorf("payload %q 原样出现在输出中:\n%s", p, out)
		}
	}
}

func TestRenderNftIpBlockAcceptsLegitIPs(t *testing.T) {
	out := RenderNftIpBlock([]string{"1.2.3.4", "10.0.0.0/8", "2001:db8::1"})
	for _, want := range []string{"1.2.3.4", "10.0.0.0/8", "2001:db8::1"} {
		if !strings.Contains(out, want) {
			t.Errorf("合法 IP %q 未渲染，实际:\n%s", want, out)
		}
	}
}
