package upnp

import (
	"strings"
	"testing"
)

// ext_ifname / listening_ip 直接来自 ExternalIface / InternalIface，
// 两者原来只查非空。"eth0\nextra_option=1" 会在 miniupnpd.conf 里多一行。
func TestValidateRejectsIfaceInjection(t *testing.T) {
	cases := []Config{
		{ExternalIface: "eth0\nextra_option=1", InternalIface: "br-lan", MaxMappings: 10, LeaseSeconds: 600},
		{ExternalIface: "eth0", InternalIface: "br-lan\nenable_upnp=no", MaxMappings: 10, LeaseSeconds: 600},
		{ExternalIface: "eth0\r\nx", InternalIface: "br-lan", MaxMappings: 10, LeaseSeconds: 600},
		{ExternalIface: "eth 0", InternalIface: "br-lan", MaxMappings: 10, LeaseSeconds: 600},
		{ExternalIface: strings.Repeat("a", 16), InternalIface: "br-lan", MaxMappings: 10, LeaseSeconds: 600},
	}
	for i, c := range cases {
		if err := Validate(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestRenderMiniupnpdNoInjection(t *testing.T) {
	c := Config{
		ExternalIface: "eth0\nextra_option=1",
		InternalIface: "br-lan\nenable_upnp=no",
		MaxMappings:   10, LeaseSeconds: 600,
	}
	out := RenderMiniupnpd(c)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "extra_option") || strings.HasPrefix(trimmed, "enable_upnp=no") {
			t.Fatalf("接口名注入产生了独立配置行: %q\n%s", line, out)
		}
	}
}

func TestRenderMiniupnpdAcceptsLegit(t *testing.T) {
	c := Config{ExternalIface: "eth0", InternalIface: "br-lan",
		AllowCIDR: "192.168.1.0/24", MaxMappings: 10, LeaseSeconds: 600}
	if err := Validate(c); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := RenderMiniupnpd(c)
	for _, want := range []string{
		"ext_ifname=eth0",
		"listening_ip=br-lan",
		"allow 192.168.1.0/24",
		"clean_ruleset_interval=300",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}

// 0.0.0.0/0 等于对整个互联网开放映射，必须拒绝。
func TestValidateRefusesWorldCIDR(t *testing.T) {
	c := Config{ExternalIface: "eth0", InternalIface: "br-lan",
		AllowCIDR: "0.0.0.0/0", MaxMappings: 10, LeaseSeconds: 600}
	if err := Validate(c); err == nil {
		t.Error("0.0.0.0/0 未被拒绝")
	}
}
