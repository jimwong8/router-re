package dhcp

import (
	"strings"
	"testing"
)

func baseServer() ServerConfig {
	return ServerConfig{
		DHCPConfig: DHCPConfig{
			Interface: "br-lan", RangeStart: "10.0.0.10", RangeEnd: "10.0.0.200",
			Netmask: "255.255.255.0", Gateway: "10.0.0.1",
			DNS: []string{"10.0.0.1"}, LeaseTime: "12h",
		},
		Enabled: true,
	}
}

// dnsmasq 配置以行为单位。Interface 只查了非空，拼进 "interface=%s"，
// 一个换行就能插入任意指令 —— 包括 dhcp-option=3,<攻击者IP> 改掉全网网关。
func TestValidateConfigRejectsInterfaceInjection(t *testing.T) {
	for _, iface := range []string{
		"br-lan\nno-resolv",
		"br-lan\ndhcp-option=3,6.6.6.6",
		"br-lan\r\nno-resolv",
		"br lan",
		"br-lan;x",
		strings.Repeat("a", 16),
	} {
		c := baseServer()
		c.Interface = iface
		if err := ValidateConfig(c); err == nil {
			t.Errorf("Interface=%q 未被拒绝", iface)
		}
	}
}

// Domain 拼进 "domain=%s"，原来只挡住了非空。
func TestValidateConfigRejectsDomainInjection(t *testing.T) {
	for _, d := range []string{"lan\nno-resolv", "lan\r\nx", "lan x", "lan/x", strings.Repeat("a", 254)} {
		c := baseServer()
		c.Domain = d
		if err := ValidateConfig(c); err == nil {
			t.Errorf("Domain=%q 未被拒绝", d)
		}
	}
}

// Hostname 拼进 "dhcp-host=<mac>,<ip>,<hostname>"，原来完全没校验。
func TestValidateConfigRejectsHostnameInjection(t *testing.T) {
	for _, h := range []string{"host1\nno-resolv", "host1\r\nx", "host 1", "host/1", "-host", "host-"} {
		c := baseServer()
		c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.5", Hostname: h}}
		if err := ValidateConfig(c); err == nil {
			t.Errorf("Hostname=%q 未被拒绝", h)
		}
	}
}

// 渲染层兜底：即使调用方跳过 Validate，也不得产出独立的生效指令行。
func TestRenderDnsmasqNoInjection(t *testing.T) {
	c := baseServer()
	c.Interface = "br-lan\nno-resolv"
	c.Domain = "lan\nno-resolv"
	c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.5", Hostname: "h\nno-resolv"}}
	out := c.RenderDnsmasq()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "no-resolv") {
			t.Fatalf("渲染层未兜底，出现独立 no-resolv 行:\n%s", out)
		}
	}
}

// 禁用状态下 prefix("# ") 只保护整段的第一行 —— 字段里的换行会逃出注释。
func TestRenderDnsmasqDisabledFullyCommented(t *testing.T) {
	c := baseServer()
	c.Enabled = false
	c.Interface = "br-lan\nno-resolv"
	c.Domain = "lan\nno-resolv"
	out := c.RenderDnsmasq()
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			t.Fatalf("禁用状态下有成活行 %q:\n%s", line, out)
		}
	}
}

func TestValidateConfigAcceptsLegit(t *testing.T) {
	c := baseServer()
	c.Domain = "lan.example"
	c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:ff", IP: "10.0.0.5", Hostname: "nas-01"}}
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := c.RenderDnsmasq()
	for _, want := range []string{
		"interface=br-lan", "dhcp-range=10.0.0.10,10.0.0.200",
		"domain=lan.example", "dhcp-host=aa:bb:cc:dd:ee:ff,10.0.0.5,nas-01",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}
