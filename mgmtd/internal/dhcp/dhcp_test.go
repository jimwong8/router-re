package dhcp

import (
	"strings"
	"testing"
)

func baseCfg() ServerConfig {
	return ServerConfig{
		DHCPConfig: DHCPConfig{
			Interface:  "lan",
			RangeStart: "192.168.1.100",
			RangeEnd:   "192.168.1.200",
			Netmask:    "255.255.255.0",
			Gateway:    "192.168.1.1",
			LeaseTime:  "12h",
			DNS:        []string{"223.5.5.5", "114.114.114.114"},
		},
		Domain:  "router.local",
		Enabled: true,
	}
}

// --- ValidateConfig 失败分支 ---

func TestValidateConfigMissingInterface(t *testing.T) {
	c := baseCfg()
	c.Interface = ""
	if err := ValidateConfig(c); err == nil {
		t.Fatal("expected error for empty interface")
	}
}

func TestValidateConfigRangeStartGTEnd(t *testing.T) {
	c := baseCfg()
	c.RangeStart = "192.168.1.200"
	c.RangeEnd = "192.168.1.100"
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), ">") {
		t.Fatalf("expected start>end error, got %v", err)
	}
}

func TestValidateConfigCrossSubnet(t *testing.T) {
	c := baseCfg()
	// start < end in byte order, but end is NOT in start's /24 → subnet error
	c.RangeStart = "172.16.0.50"
	c.RangeEnd = "192.168.1.200"
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "not in subnet") {
		t.Fatalf("expected subnet error, got %v", err)
	}
}

func TestValidateConfigStaticInDynamicRange(t *testing.T) {
	c := baseCfg()
	c.StaticLeases = append(c.StaticLeases, StaticLease{
		MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.150", Hostname: "dup",
	})
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "collides with dynamic range") {
		t.Fatalf("expected collision error, got %v", err)
	}
}

func TestValidateConfigDuplicateMAC(t *testing.T) {
	c := baseCfg()
	c.StaticLeases = append(c.StaticLeases,
		StaticLease{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.50"},
		StaticLease{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.51"},
	)
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "duplicate mac") {
		t.Fatalf("expected duplicate mac error, got %v", err)
	}
}

func TestValidateConfigBadLeaseTime(t *testing.T) {
	c := baseCfg()
	c.LeaseTime = "12x"
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "lease_time") {
		t.Fatalf("expected lease_time error, got %v", err)
	}
}

func TestValidateConfigBadDNS(t *testing.T) {
	c := baseCfg()
	c.DNS = []string{"not-an-ip"}
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "dns[0]") {
		t.Fatalf("expected dns error, got %v", err)
	}
}

func TestValidateConfigGatewayOutsideSubnet(t *testing.T) {
	c := baseCfg()
	c.Gateway = "192.168.2.1"
	err := ValidateConfig(c)
	if err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Fatalf("expected gateway subnet error, got %v", err)
	}
}

// --- ValidateConfig 正例 ---

func TestValidateConfigOK(t *testing.T) {
	c := baseCfg()
	c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.50", Hostname: "box"}}
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

// --- RenderDnsmasq 方法 ---

func TestRenderServerConfigContainsFields(t *testing.T) {
	c := baseCfg()
	c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.50", Hostname: "box"}}
	out := c.RenderDnsmasq()
	for _, want := range []string{
		"interface=lan",
		"dhcp-range=192.168.1.100,192.168.1.200,255.255.255.0,12h",
		"dhcp-option=3,192.168.1.1",
		"dhcp-option=6,223.5.5.5,114.114.114.114",
		"domain=router.local",
		"dhcp-host=aa:bb:cc:dd:ee:01,192.168.1.50,box",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestRenderServerConfigDisabledPrefix(t *testing.T) {
	c := baseCfg()
	c.StaticLeases = []StaticLease{{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.50", Hostname: "box"}}
	c.Enabled = false
	out := c.RenderDnsmasq()
	for _, want := range []string{
		"# interface=lan",
		"# dhcp-range",
		"# dhcp-option=3",
		"# dhcp-option=6",
		"# domain=router.local",
		"# dhcp-host=aa:bb:cc:dd:ee:01,192.168.1.50,box",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("disabled render missing %q:\n%s", want, out)
		}
	}
}
