package dnsproxy

import (
	"os"
	"strings"
	"testing"
)

// fakeRunner records every command it was asked to run and can be told to
// fail on a specific one, so Apply/Resolve can be exercised without a real
// dnsmasq or resolver.
type fakeRunner struct {
	cmds     []string
	failOn   string
	getent   string
	nslookup string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.cmds = append(f.cmds, line)
	if f.failOn != "" && strings.Contains(line, f.failOn) {
		return "", errString("boom: " + line)
	}
	switch name {
	case "getent":
		return f.getent, nil
	case "nslookup":
		return f.nslookup, nil
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
		Upstreams:    []Upstream{{Address: "223.5.5.5"}, {Address: "8.8.8.8", Port: 5353}},
		Hosts:        []HostEntry{{IP: "192.168.1.10", Names: []string{"nas.lan", "files.lan"}}},
		CacheSize:    150,
		DomainNeeded: true,
		BogusPriv:    true,
		Enabled:      true,
	}
}

func TestValidateFailures(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"invalid upstream ip", func() Config { c := validConfig(); c.Upstreams[0].Address = "not-an-ip"; return c }()},
		{"upstream ip with space", func() Config { c := validConfig(); c.Upstreams[0].Address = " 223.5.5.5 "; return c }()},
		{"port too low", func() Config { c := validConfig(); c.Upstreams[1].Port = -1; return c }()},
		{"port too high", func() Config { c := validConfig(); c.Upstreams[0].Port = 65536; return c }()},
		{"cache size negative", func() Config { c := validConfig(); c.CacheSize = -1; return c }()},
		{"cache size too large", func() Config { c := validConfig(); c.CacheSize = 1000001; return c }()},
		{"empty host ip", func() Config { c := validConfig(); c.Hosts[0].IP = ""; return c }()},
		{"host ip invalid", func() Config { c := validConfig(); c.Hosts[0].IP = "999.1.1.1"; return c }()},
		{"host no names", func() Config { c := validConfig(); c.Hosts[0].Names = nil; return c }()},
		{"host empty name", func() Config { c := validConfig(); c.Hosts[0].Names = []string{""}; return c }()},
		{"host name with space", func() Config { c := validConfig(); c.Hosts[0].Names = []string{"a b.lan"}; return c }()},
		{"host name with comma", func() Config { c := validConfig(); c.Hosts[0].Names = []string{"a,b.lan"}; return c }()},
		{"host name too long", func() Config {
			c := validConfig()
			c.Hosts[0].Names = []string{strings.Repeat("a", 254) + ".lan"}
			return c
		}()},
		{"enabled without upstream", func() Config { c := validConfig(); c.Upstreams = nil; return c }()},
	}
	for _, tc := range cases {
		if err := Validate(tc.cfg); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

func TestValidateOK(t *testing.T) {
	if err := Validate(validConfig()); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	c := validConfig()
	c.Enabled = false
	c.Upstreams = nil
	if err := Validate(c); err != nil {
		t.Fatalf("disabled config without upstream rejected: %v", err)
	}
	c2 := validConfig()
	c2.Upstreams[0].Port = 0
	if err := Validate(c2); err != nil {
		t.Fatalf("port 0 (default) rejected: %v", err)
	}
	c3 := validConfig()
	c3.Upstreams[0].Address = "2400:3200::1"
	if err := Validate(c3); err != nil {
		t.Fatalf("ipv6 upstream rejected: %v", err)
	}
}

func TestRenderDnsmasq(t *testing.T) {
	out := RenderDnsmasq(validConfig())
	for _, want := range []string{
		"no-resolv",
		"server=223.5.5.5#53",
		"server=8.8.8.8#5353",
		"cache-size=150",
		"domain-needed",
		"bogus-priv",
		"address=/nas.lan/192.168.1.10",
		"address=/files.lan/192.168.1.10",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// port 0 must render as the default 53
	if strings.Contains(out, "server=223.5.5.5#0") {
		t.Errorf("port 0 rendered instead of default 53:\n%s", out)
	}
}

func TestRenderDnsmasqDisabledIsAllComments(t *testing.T) {
	c := validConfig()
	c.Enabled = false
	out := RenderDnsmasq(c)
	if out == "" {
		t.Fatal("disabled config rendered nothing")
	}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "# ") {
			t.Errorf("non-commented line when disabled: %q", line)
		}
	}
	if !strings.Contains(out, "# no-resolv") || !strings.Contains(out, "# server=223.5.5.5#53") {
		t.Errorf("disabled render missing commented directives:\n%s", out)
	}
}

func TestRenderHosts(t *testing.T) {
	out := RenderHosts(validConfig())
	if !strings.Contains(out, "192.168.1.10 nas.lan files.lan") {
		t.Errorf("hosts line wrong:\n%s", out)
	}
	c := validConfig()
	c.Enabled = false
	out2 := RenderHosts(c)
	if !strings.HasPrefix(strings.TrimSpace(out2), "# ") {
		t.Errorf("disabled hosts not commented:\n%s", out2)
	}
}

func TestApplyWritesAndRestarts(t *testing.T) {
	fr := &fakeRunner{}
	path := t.TempDir() + "/dnsmasq.d/dnsproxy.conf"
	if err := Apply(fr, validConfig(), path); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if !strings.Contains(string(content), "server=223.5.5.5#53") {
		t.Errorf("written config missing upstream:\n%s", content)
	}
	if !strings.Contains(strings.Join(fr.cmds, "\n"), "systemctl restart dnsmasq") {
		t.Errorf("systemctl restart not attempted: %v", fr.cmds)
	}
}

func TestApplyReportsBothRestartFailures(t *testing.T) {
	fr := &fakeRunner{failOn: "restart"}
	path := t.TempDir() + "/dnsmasq.d/dnsproxy.conf"
	err := Apply(fr, validConfig(), path)
	if err == nil {
		t.Fatal("expected error when both restart paths fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "systemctl") || !strings.Contains(msg, "init.d") {
		t.Errorf("error must report both restart failures, got: %s", msg)
	}
	// the config must still have been written
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("config not written despite restart failure: %v", statErr)
	}
}

func TestApplyInvalidConfigNotWritten(t *testing.T) {
	fr := &fakeRunner{}
	path := t.TempDir() + "/dnsmasq.d/dnsproxy.conf"
	c := validConfig()
	c.Upstreams[0].Address = "bad"
	if err := Apply(fr, c, path); err == nil {
		t.Fatal("expected validation error")
	}
	if len(fr.cmds) != 0 {
		t.Errorf("no command should run for invalid config, got: %v", fr.cmds)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("invalid config must not be written")
	}
}

func TestResolveGetent(t *testing.T) {
	fr := &fakeRunner{getent: "192.168.1.10 nas.lan\n192.168.1.11 nas.lan"}
	ips, err := Resolve(fr, "nas.lan")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(ips) != 2 || ips[0] != "192.168.1.10" || ips[1] != "192.168.1.11" {
		t.Errorf("unexpected ips: %v", ips)
	}
}

func TestResolveFallsBackToNslookup(t *testing.T) {
	fr := &fakeRunner{nslookup: "Name:\texample.com\nAddress: 93.184.216.34"}
	ips, err := Resolve(fr, "example.com")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(ips) != 1 || ips[0] != "93.184.216.34" {
		t.Errorf("unexpected ips: %v", ips)
	}
}

func TestResolveNotFound(t *testing.T) {
	fr := &fakeRunner{}
	if _, err := Resolve(fr, "nope.example"); err == nil {
		t.Fatal("expected error for unresolvable name")
	}
}

func TestResolveRejectsInjection(t *testing.T) {
	fr := &fakeRunner{}
	for _, bad := range []string{"a;rm -rf /", "a && cat /etc/passwd", "a`id`", "a$(id)", "a|id", "a b", ""} {
		if _, err := Resolve(fr, bad); err == nil {
			t.Errorf("Resolve(%q) should be rejected", bad)
		}
	}
	if len(fr.cmds) != 0 {
		t.Errorf("no command may run for a rejected name, ran: %v", fr.cmds)
	}
}

func TestResolveAcceptsValidNames(t *testing.T) {
	fr := &fakeRunner{getent: "10.0.0.1 a.b-c.example"}
	for _, ok := range []string{"example.com", "a.b-c.example", "1.2.3.4", "x"} {
		if _, err := Resolve(fr, ok); err != nil {
			t.Errorf("Resolve(%q) rejected: %v", ok, err)
		}
	}
}
