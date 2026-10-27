package upnp

import (
	"os"
	"strings"
	"testing"
)

type fakeRunner struct {
	cmds   []string
	failOn string
	out    string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.cmds = append(f.cmds, line)
	if f.failOn != "" && strings.Contains(line, f.failOn) {
		return "", errString("boom: " + line)
	}
	if name == "upnpc" {
		return f.out, nil
	}
	return "", nil
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " ")+" <input>")
	return "", nil
}

type errString string

func (e errString) Error() string { return string(e) }

func validConfig() Config {
	return Config{
		Enabled:       true,
		ExternalIface: "eth0",
		InternalIface: "br-lan",
		AllowCIDR:     "192.168.0.0/16",
		MaxMappings:   100,
		LeaseSeconds:  3600,
		NATPMP:        true,
	}
}

func TestValidateFailures(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty external iface", func() Config { c := validConfig(); c.ExternalIface = ""; return c }()},
		{"empty internal iface", func() Config { c := validConfig(); c.InternalIface = ""; return c }()},
		{"same iface", func() Config { c := validConfig(); c.InternalIface = "eth0"; return c }()},
		{"allow cidr 0.0.0.0/0", func() Config { c := validConfig(); c.AllowCIDR = "0.0.0.0/0"; return c }()},
		{"allow cidr ::/0", func() Config { c := validConfig(); c.AllowCIDR = "::/0"; return c }()},
		{"allow cidr invalid", func() Config { c := validConfig(); c.AllowCIDR = "not-a-cidr"; return c }()},
		{"max mappings too low", func() Config { c := validConfig(); c.MaxMappings = 0; return c }()},
		{"max mappings too high", func() Config { c := validConfig(); c.MaxMappings = 1001; return c }()},
		{"lease too short", func() Config { c := validConfig(); c.LeaseSeconds = 59; return c }()},
		{"lease too long", func() Config { c := validConfig(); c.LeaseSeconds = 604801; return c }()},
	}
	for _, tc := range cases {
		if err := Validate(tc.cfg); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
	// the 0.0.0.0/0 rejection must carry an explanatory message
	c := validConfig()
	c.AllowCIDR = "0.0.0.0/0"
	err := Validate(c)
	if err == nil {
		t.Fatal("0.0.0.0/0 should be rejected")
	}
	if !strings.Contains(err.Error(), "whole internet") && !strings.Contains(err.Error(), "entire internet") {
		t.Errorf("0.0.0.0/0 error should explain the internet exposure, got: %v", err)
	}
}

func TestValidateOK(t *testing.T) {
	if err := Validate(validConfig()); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	c := validConfig()
	c.AllowCIDR = ""
	if err := Validate(c); err != nil {
		t.Fatalf("empty allow_cidr (optional) rejected: %v", err)
	}
}

func TestValidateMappingFailures(t *testing.T) {
	c := validConfig()
	cases := []struct {
		name    string
		mapping Mapping
	}{
		{"bad proto", Mapping{Protocol: "icmp", ExtPort: 8080, IntIP: "192.168.1.10", IntPort: 80, LeaseSeconds: 600}},
		{"ext port too low", Mapping{Protocol: "tcp", ExtPort: 0, IntIP: "192.168.1.10", IntPort: 80, LeaseSeconds: 600}},
		{"ext port too high", Mapping{Protocol: "tcp", ExtPort: 65536, IntIP: "192.168.1.10", IntPort: 80, LeaseSeconds: 600}},
		{"int port too low", Mapping{Protocol: "tcp", ExtPort: 8080, IntIP: "192.168.1.10", IntPort: 0, LeaseSeconds: 600}},
		{"public int ip", Mapping{Protocol: "tcp", ExtPort: 8080, IntIP: "8.8.8.8", IntPort: 80, LeaseSeconds: 600}},
		{"invalid int ip", Mapping{Protocol: "tcp", ExtPort: 8080, IntIP: "999.1.1.1", IntPort: 80, LeaseSeconds: 600}},
		{"desc too long", Mapping{Protocol: "tcp", ExtPort: 8080, IntIP: "192.168.1.10", IntPort: 80, Description: strings.Repeat("x", 65), LeaseSeconds: 600}},
		{"lease over max", Mapping{Protocol: "tcp", ExtPort: 8080, IntIP: "192.168.1.10", IntPort: 80, LeaseSeconds: 604800}},
	}
	for _, tc := range cases {
		if err := ValidateMapping(tc.mapping, c); err == nil {
			t.Errorf("%s: expected error", tc.name)
		}
	}
}

func TestValidateMappingOK(t *testing.T) {
	c := validConfig()
	m := Mapping{Protocol: "udp", ExtPort: 5353, IntIP: "10.1.2.3", IntPort: 5353, Description: "mdns", LeaseSeconds: 3600}
	if err := ValidateMapping(m, c); err != nil {
		t.Fatalf("valid mapping rejected: %v", err)
	}
}

func TestRenderMiniupnpd(t *testing.T) {
	out := RenderMiniupnpd(validConfig())
	for _, want := range []string{
		"ext_ifname=eth0",
		"listening_ip=br-lan",
		"allow 192.168.0.0/16",
		"enable_natpmp=yes",
		"clean_ruleset_interval=1800",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	c := validConfig()
	c.NATPMP = false
	if !strings.Contains(RenderMiniupnpd(c), "enable_natpmp=no") {
		t.Errorf("natpmp=no not rendered:\n%s", RenderMiniupnpd(c))
	}
}

func TestRenderNftMappings(t *testing.T) {
	ms := []Mapping{
		{Protocol: "tcp", ExtPort: 8080, IntIP: "192.168.1.10", IntPort: 80},
		{Protocol: "udp", ExtPort: 5353, IntIP: "10.1.2.3", IntPort: 5353},
	}
	out := RenderNftMappings(ms)
	if !strings.Contains(out, "table inet router_upnp {") {
		t.Errorf("missing table header:\n%s", out)
	}
	if !strings.Contains(out, "tcp dport 8080 dnat ip to 192.168.1.10:80") {
		t.Errorf("missing tcp dnat ip to:\n%s", out)
	}
	if !strings.Contains(out, "udp dport 5353 dnat ip to 10.1.2.3:5353") {
		t.Errorf("missing udp dnat ip to:\n%s", out)
	}
	// chain declaration must be multi-line: "chain prerouting {" then the hook
	if strings.Contains(out, "chain prerouting { type") {
		t.Errorf("chain declaration not multi-line:\n%s", out)
	}
	if !strings.Contains(out, "type nat hook prerouting priority dstnat;") {
		t.Errorf("missing hook line:\n%s", out)
	}
	// never flush ruleset
	if strings.Contains(out, "flush ruleset") || strings.Contains(out, "flush table") {
		t.Errorf("rendered config contains a flush:\n%s", out)
	}
}

func TestApplyWritesAndRestarts(t *testing.T) {
	fr := &fakeRunner{}
	path := t.TempDir() + "/miniupnpd/miniupnpd.conf"
	if err := Apply(fr, validConfig(), path); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("conf not written: %v", err)
	}
	if !strings.Contains(string(content), "ext_ifname=eth0") {
		t.Errorf("written conf missing ext_ifname:\n%s", content)
	}
	if !strings.Contains(strings.Join(fr.cmds, "\n"), "systemctl restart miniupnpd") {
		t.Errorf("systemctl restart not attempted: %v", fr.cmds)
	}
}

func TestApplyReportsBothRestartFailures(t *testing.T) {
	fr := &fakeRunner{failOn: "restart"}
	path := t.TempDir() + "/miniupnpd/miniupnpd.conf"
	err := Apply(fr, validConfig(), path)
	if err == nil {
		t.Fatal("expected error when both restart paths fail")
	}
	if !strings.Contains(err.Error(), "systemctl") || !strings.Contains(err.Error(), "init.d") {
		t.Errorf("error must report both restart failures: %v", err)
	}
}

func TestActiveMappingsMissingUpnpc(t *testing.T) {
	fr := &fakeRunner{failOn: "upnpc"}
	_, err := ActiveMappings(fr)
	if err == nil {
		t.Fatal("expected error when upnpc is missing")
	}
	if !strings.Contains(err.Error(), "upnpc") {
		t.Errorf("error should mention upnpc: %v", err)
	}
}

func TestActiveMappingsEmptyOutput(t *testing.T) {
	fr := &fakeRunner{out: "\n"}
	if _, err := ActiveMappings(fr); err == nil {
		t.Fatal("expected error for empty upnpc output")
	}
}

func TestActiveMappingsParses(t *testing.T) {
	fr := &fakeRunner{out: "upnpc v1.14.1 (c) 2005-2018 Thomas Bernard\nNo IGD upnp device found on the network!\n 0 TCP 8080->192.168.1.10:80 'http' ''\n 1 UDP 5353->10.1.2.3:5353 'mdns' ''"}
	ms, err := ActiveMappings(fr)
	if err != nil {
		t.Fatalf("ActiveMappings failed: %v", err)
	}
	if len(ms) != 2 {
		t.Fatalf("expected 2 mappings, got %d: %v", len(ms), ms)
	}
	if ms[0].Protocol != "tcp" || ms[0].ExtPort != 8080 || ms[0].IntIP != "192.168.1.10" || ms[0].IntPort != 80 {
		t.Errorf("mapping[0] wrong: %+v", ms[0])
	}
	if ms[1].Protocol != "udp" || ms[1].ExtPort != 5353 || ms[1].IntIP != "10.1.2.3" || ms[1].IntPort != 5353 {
		t.Errorf("mapping[1] wrong: %+v", ms[1])
	}
}
