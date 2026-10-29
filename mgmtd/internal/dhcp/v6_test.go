package dhcp

import "testing"

func TestV6RenderContainsRangeAndRA(t *testing.T) {
	c := ServerConfig{
		DHCPConfig: DHCPConfig{
			Interface: "br-lan",
			RangeStart: "192.168.1.100",
			RangeEnd:   "192.168.1.200",
			Netmask:    "255.255.255.0",
		},
		Enabled:      true,
		V6RangeStart: "fd00::100",
		V6RangeEnd:   "fd00::200",
		V6PrefixLen:  64,
		EnableRA:     true,
	}
	out := c.RenderDnsmasq()
	if !contains(out, "dhcp-range=fd00::100,fd00::200,64,12h") {
		t.Fatalf("missing v6 range line: %s", out)
	}
	if !contains(out, "enable-ra") {
		t.Fatalf("missing enable-ra: %s", out)
	}
}

func TestV6PrefixLenInvalid(t *testing.T) {
	base := ServerConfig{
		DHCPConfig: DHCPConfig{
			Interface: "br-lan",
			RangeStart: "192.168.1.100",
			RangeEnd:   "192.168.1.200",
			Netmask:    "255.255.255.0",
		},
		Enabled:      true,
		V6RangeStart: "fd00::100",
		V6RangeEnd:   "fd00::200",
	}
	for _, plen := range []int{0, 129} {
		c := base
		c.V6PrefixLen = plen
		if err := ValidateConfig(c); err == nil {
			t.Fatalf("expected error for %d", plen)
		}
	}
}

func TestV6EmptyPoolNoRender(t *testing.T) {
	c := ServerConfig{
		DHCPConfig: DHCPConfig{
			Interface: "br-lan",
			RangeStart: "192.168.1.100",
			RangeEnd:   "192.168.1.200",
			Netmask:    "255.255.255.0",
		},
		Enabled: true,
	}
	out := c.RenderDnsmasq()
	if contains(out, "fd00::") {
		t.Fatalf("unexpected v6 line: %s", out)
	}
}

func TestV6ValidatePure(t *testing.T) {
	c := ServerConfig{
		DHCPConfig: DHCPConfig{Interface: "eth1"},
		V6RangeStart: "fd00::100",
		V6RangeEnd:   "fd00::200",
		V6PrefixLen:  64,
		EnableRA:     true,
	}
	if err := ValidateV6Config(c); err != nil {
		t.Fatalf("pure v6 config should pass: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
