package dhcp

import (
	"strings"
	"testing"
)

// TestRenderDnsmasqRange 验证渲染输出包含 dhcp-range 及关键字段。
func TestRenderDnsmasqRange(t *testing.T) {
	cfg := DHCPConfig{
		Interface:  "lan",
		RangeStart: "192.168.1.100",
		RangeEnd:   "192.168.1.200",
		Netmask:    "255.255.255.0",
		Gateway:    "192.168.1.1",
		LeaseTime:  "12h",
		DNS:        []string{"223.5.5.5", "114.114.114.114"},
	}
	out := RenderDnsmasq(cfg)
	if out == "" {
		t.Fatal("empty output")
	}
	for _, want := range []string{
		"dhcp-range=192.168.1.100,192.168.1.200,255.255.255.0,12h",
		"interface=lan",
		"dhcp-option=3,192.168.1.1",
		"dhcp-option=6,223.5.5.5,114.114.114.114",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

// TestRenderDnsmasqNoDNS 验证 DNS 为空时不含 dhcp-option=6。
func TestRenderDnsmasqNoDNS(t *testing.T) {
	cfg := DHCPConfig{
		Interface:  "lan",
		RangeStart: "192.168.1.100",
		RangeEnd:   "192.168.1.200",
		Gateway:    "192.168.1.1",
	}
	out := RenderDnsmasq(cfg)
	if strings.Contains(out, "dhcp-option=6") {
		t.Fatalf("unexpected dhcp-option=6 in output:\n%s", out)
	}
}

// TestRenderDnsmasqEmptyCfg 验证空配置仍可渲染且不产生垃圾。
func TestRenderDnsmasqEmptyCfg(t *testing.T) {
	out := RenderDnsmasq(DHCPConfig{})
	if out != "" {
		t.Fatalf("expected empty output for empty cfg, got:\n%s", out)
	}
}
