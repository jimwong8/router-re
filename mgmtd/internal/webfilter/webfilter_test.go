package webfilter

import (
	"os"
	"strings"
	"testing"
)

type fakeRunner struct{ cmds []string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	return "", nil
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	return "", nil
}

func TestValidateFailures(t *testing.T) {
	cases := []struct {
		name string
		rule Rule
	}{
		{"empty domain", Rule{Domain: "", Action: "block"}},
		{"space", Rule{Domain: "a b.com", Action: "block"}},
		{"comma", Rule{Domain: "a,b.com", Action: "block"}},
		{"leading dot", Rule{Domain: ".example.com", Action: "block"}},
		{"too long", Rule{Domain: strings.Repeat("a", 254) + ".com", Action: "block"}},
		{"bad action", Rule{Domain: "example.com", Action: "whitelist"}},
	}
	for _, c := range cases {
		if err := Validate([]Rule{c.rule}); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestValidateOK(t *testing.T) {
	if err := Validate([]Rule{{Domain: "example.com", Action: "block", Enabled: true}}); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
}

func TestRenderDnsmasq(t *testing.T) {
	rs := []Rule{
		{Domain: "ads.example.com", Action: "block", Enabled: true},
		{Domain: "tracker.example.net", Action: "block", Enabled: true},
		{Domain: "good.example.org", Action: "allow", Enabled: true},
		{Domain: "disabled.example.com", Action: "block", Enabled: false},
	}
	out := RenderDnsmasq(rs)
	if !strings.Contains(out, "address=/ads.example.com/0.0.0.0") {
		t.Errorf("missing ipv4 block line:\n%s", out)
	}
	if !strings.Contains(out, "address=/ads.example.com/::") {
		t.Errorf("missing ipv6 block line:\n%s", out)
	}
	if !strings.Contains(out, "address=/tracker.example.net/0.0.0.0") {
		t.Errorf("missing second domain ipv4 line:\n%s", out)
	}
	// disabled block must not render
	if strings.Contains(out, "disabled.example.com") {
		t.Errorf("disabled rule rendered:\n%s", out)
	}
	// allow must be comment, not address=
	if !strings.Contains(out, "# allow good.example.org") {
		t.Errorf("allow not a comment:\n%s", out)
	}
	if strings.Contains(out, "address=/good.example.org/") {
		t.Errorf("allow rendered as address line:\n%s", out)
	}
}

func TestApplyWritesAndRestarts(t *testing.T) {
	fr := &fakeRunner{}
	path := t.TempDir() + "/dnsmasq.d/webfilter.conf"
	rs := []Rule{{Domain: "ads.example.com", Action: "block", Enabled: true}}
	if err := Apply(fr, rs, path); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(fr.cmds) == 0 {
		t.Fatal("expected dnsmasq restart command")
	}
	if !strings.Contains(fr.cmds[0], "systemctl restart dnsmasq") {
		t.Errorf("expected systemctl restart first, got: %v", fr.cmds)
	}
	// verify file content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(content), "address=/ads.example.com/0.0.0.0") {
		t.Errorf("config file missing block line: %s", content)
	}
}
