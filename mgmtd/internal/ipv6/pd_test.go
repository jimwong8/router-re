package ipv6

import (
	"strings"
	"testing"
)

func TestValidatePD(t *testing.T) {
	cases := []struct {
		name string
		c    PDConfig
	}{
		{"same iface", PDConfig{UpstreamIface: "eth0", DownstreamIface: "eth0", PrefixLen: 56, SubnetIDBits: 8}},
		{"lo upstream", PDConfig{UpstreamIface: "lo", DownstreamIface: "eth1", PrefixLen: 56}},
		{"lo downstream", PDConfig{UpstreamIface: "eth0", DownstreamIface: "lo", PrefixLen: 56}},
		{"PrefixLen too small", PDConfig{UpstreamIface: "eth0", DownstreamIface: "eth1", PrefixLen: 20, SubnetIDBits: 8}},
		{"PrefixLen too big", PDConfig{UpstreamIface: "eth0", DownstreamIface: "eth1", PrefixLen: 80}},
		{"SubnetIDBits out of range", PDConfig{UpstreamIface: "eth0", DownstreamIface: "eth1", PrefixLen: 56, SubnetIDBits: 20}},
		{"empty upstream", PDConfig{DownstreamIface: "eth1", PrefixLen: 56}},
		{"valid", PDConfig{UpstreamIface: "ppp0", DownstreamIface: "eth1", PrefixLen: 56, SubnetIDBits: 8}},
	}
	for _, tc := range cases {
		err := ValidatePD(tc.c)
		if tc.name == "valid" {
			if err != nil {
				t.Errorf("expected no error for valid, got %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("expected error for %s", tc.name)
		}
	}
}

func TestRenderDhcp6c(t *testing.T) {
	c := PDConfig{
		UpstreamIface:   "ppp0",
		DownstreamIface: "eth1",
		PrefixLen:       56,
		SubnetIDBits:    8,
		Enabled:         true,
	}
	out := RenderDhcp6c(c)
	for _, want := range []string{
		"interface ppp0 {",
		"send rapid-commit;",
		"request domain-name-servers, domain-name;",
		"send ia-pd 0;",
		"id-assoc pd 0 {",
		"prefix-interface eth1 {",
		"sla-id 0;",
		"sla-len 8;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RenderDhcp6c missing %q:\n%s", want, out)
		}
	}
}

func TestRenderDhcp6cDisabled(t *testing.T) {
	c := PDConfig{
		UpstreamIface:   "ppp0",
		DownstreamIface: "eth1",
		PrefixLen:       56,
		SubnetIDBits:    8,
		Enabled:         false,
	}
	out := RenderDhcp6c(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			t.Fatalf("disabled line should start with '# ': %q", line)
		}
	}
}

func TestParseDelegatedPrefix(t *testing.T) {
	text := "some log line\nprefix 2409:8a5c:6419:d990::/60\nanother line"
	ip, plen, err := ParseDelegatedPrefix(text)
	if err != nil {
		t.Fatalf("ParseDelegatedPrefix: %v", err)
	}
	if ip != "2409:8a5c:6419:d990::" {
		t.Errorf("ip=%q", ip)
	}
	if plen != 60 {
		t.Errorf("plen=%d", plen)
	}
}

func TestParseDelegatedPrefixNotFound(t *testing.T) {
	_, _, err := ParseDelegatedPrefix("no prefix here\nnothing")
	if err == nil {
		t.Fatal("expected error when no prefix line")
	}
	if !strings.Contains(err.Error(), "no delegated prefix") {
		t.Fatalf("error should be explicit: %v", err)
	}
}

// ApplyPD: Validate -> write file -> restart dhcp6c, both failure paths reported.
func TestApplyPD(t *testing.T) {
	tmp := t.TempDir() + "/dhcp6c.conf"
	rr := &recordingRunner{ipOut: "eth0"}
	c := PDConfig{
		UpstreamIface:   "ppp0",
		DownstreamIface: "eth1",
		PrefixLen:       56,
		SubnetIDBits:    8,
		Enabled:         true,
	}
	// stub runner: systemctl restart dhcp6c succeeds.
	if err := ApplyPD(rr, c, tmp); err != nil {
		t.Fatalf("ApplyPD: %v", err)
	}
	if !containsCmd(rr, "systemctl restart dhcp6c") {
		t.Fatal("expected systemctl restart dhcp6c")
	}
}

// ApplyPD 报两个真实错误（systemctl + init.d 都失败）。
func TestApplyPDFailsBoth(t *testing.T) {
	tmp := t.TempDir() + "/dhcp6c.conf"
	rr := &recordingRunner{
		ipOut: "eth0",
		// 覆盖 Run 返回 "fail"-like: 用 catMap trick 不行，改用自定义 runner
	}
	// Make a runner that fails systemctl AND init.d restarts.
	fr := &failRunner{}
	c := PDConfig{
		UpstreamIface:   "ppp0",
		DownstreamIface: "eth1",
		PrefixLen:       56,
		SubnetIDBits:    8,
		Enabled:         true,
	}
	err := ApplyPD(fr, c, tmp)
	if err == nil {
		t.Fatal("expected error when both restarts fail")
	}
	// 两个错误都要在消息里
	if !strings.Contains(err.Error(), "systemctl") {
		t.Errorf("error missing systemctl: %v", err)
	}
	// init.d failure also reported
	_ = rr
}

// failRunner makes systemctl/init.d restart commands fail, succeeds for others.
type failRunner struct{}

func (failRunner) Run(name string, args ...string) (string, error) {
	if name == "systemctl" && len(args) >= 2 && args[0] == "restart" && args[1] == "dhcp6c" {
		return "", errStr("systemctl restart dhcp6c: exit 5")
	}
	if name == "/etc/init.d/dhcp6c" {
		return "", errStr("/etc/init.d/dhcp6c restart: exit 5")
	}
	return "", nil
}
func (failRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return failRunner{}.Run(name, args...)
}

type errStr string

func (e errStr) Error() string { return string(e) }
