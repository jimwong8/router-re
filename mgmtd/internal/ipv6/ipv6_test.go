package ipv6

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

type stubRunner struct {
	out string
	err error
}

func (s *stubRunner) Run(name string, args ...string) (string, error) { return s.out, s.err }
func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, s.err
}

var _ execx.Runner = (*stubRunner)(nil)

func TestValidateAddr(t *testing.T) {
	// IPv4 must be rejected
	if err := ValidateAddr(Address{Iface: "ens18", Addr: "192.168.1.1", PrefixLen: 64}); err == nil {
		t.Fatal("IPv4 address must be rejected")
	}
	// empty iface
	if err := ValidateAddr(Address{Iface: "", Addr: "2001:db8::1", PrefixLen: 64}); err == nil {
		t.Fatal("empty iface must be rejected")
	}
	// prefix out of range
	if err := ValidateAddr(Address{Iface: "ens18", Addr: "2001:db8::1", PrefixLen: 0}); err == nil {
		t.Fatal("prefix 0 must be rejected")
	}
	if err := ValidateAddr(Address{Iface: "ens18", Addr: "2001:db8::1", PrefixLen: 129}); err == nil {
		t.Fatal("prefix 129 must be rejected")
	}
	// valid IPv6
	if err := ValidateAddr(Address{Iface: "ens18", Addr: "2001:db8::1", PrefixLen: 64}); err != nil {
		t.Fatalf("valid addr rejected: %v", err)
	}
}

func TestValidateRA(t *testing.T) {
	// prefix is IPv4 -> reject
	if err := ValidateRA(RAConfig{Iface: "ens18", Enabled: true, Prefix: "192.168.1.0", PrefixLen: 64, ValidLifetime: 86400, PreferredLifetime: 14400, MaxInterval: 600}); err == nil {
		t.Fatal("IPv4 prefix with Enabled must be rejected")
	}
	// valid < preferred
	if err := ValidateRA(RAConfig{Iface: "ens18", Enabled: true, Prefix: "2001:db8::/64", PrefixLen: 64, ValidLifetime: 100, PreferredLifetime: 200, MaxInterval: 600}); err == nil {
		t.Fatal("valid < preferred must be rejected")
	}
	// interval out of range
	if err := ValidateRA(RAConfig{Iface: "ens18", Enabled: true, Prefix: "2001:db8::", PrefixLen: 64, ValidLifetime: 86400, PreferredLifetime: 14400, MaxInterval: 1}); err == nil {
		t.Fatal("max_interval 1 must be rejected")
	}
	// valid
	c := RAConfig{Iface: "ens18", Enabled: true, Prefix: "2001:db8::", PrefixLen: 64, ValidLifetime: 86400, PreferredLifetime: 14400, MaxInterval: 600}
	if err := ValidateRA(c); err != nil {
		t.Fatalf("valid RA rejected: %v", err)
	}
}

func TestRenderRadvd(t *testing.T) {
	c := RAConfig{Iface: "ens18", Enabled: true, ManagedFlag: true, OtherFlag: false,
		MaxInterval: 600, Prefix: "2001:db8:1::", PrefixLen: 64,
		ValidLifetime: 86400, PreferredLifetime: 14400}
	out := RenderRadvd(c)
	if !strings.Contains(out, "AdvSendAdvert on") {
		t.Errorf("missing AdvSendAdvert:\n%s", out)
	}
	if !strings.Contains(out, "prefix 2001:db8:1::/64") {
		t.Errorf("missing prefix block:\n%s", out)
	}
	if !strings.Contains(out, "MaxRtrAdvInterval 600") {
		t.Errorf("missing max interval:\n%s", out)
	}
	if !strings.Contains(out, "AdvManagedFlag on") {
		t.Errorf("managed flag not on:\n%s", out)
	}
}

func TestRenderRadvdDisabledCommented(t *testing.T) {
	c := RAConfig{Iface: "ens18", Enabled: false, MaxInterval: 600,
		Prefix: "2001:db8:1::", PrefixLen: 64, ValidLifetime: 86400, PreferredLifetime: 14400}
	out := RenderRadvd(c)
	// every meaningful line must be commented
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			t.Errorf("disabled RA has uncommented line: %q", trimmed)
		}
	}
}

// Build a realistic `ip -j -6 addr show` JSON with fe80/::1 and a global addr.
func sampleAddrJSON() string {
	type e struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		PrefixLen int    `json:"prefixlen"`
	}
	type i struct {
		Ifname   string `json:"ifname"`
		AddrInfo []e    `json:"addr_info"`
	}
	raw := []i{
		{Ifname: "ens18", AddrInfo: []e{
			{Family: "inet6", Local: "fe80::1:2:3:4", PrefixLen: 64},
			{Family: "inet6", Local: "2001:db8:1::42", PrefixLen: 64},
			{Family: "inet6", Local: "::1", PrefixLen: 128},
		}},
		{Ifname: "ens20", AddrInfo: []e{
			{Family: "inet6", Local: "2001:db8:2::7", PrefixLen: 48},
		}},
	}
	b, _ := json.Marshal(raw)
	return string(b)
}

func TestListAddrSkipsLinkLocalAndLoopback(t *testing.T) {
	r := &stubRunner{out: sampleAddrJSON()}
	addrs, err := ListAddr(r)
	if err != nil {
		t.Fatalf("ListAddr error: %v", err)
	}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 global addrs, got %d: %+v", len(addrs), addrs)
	}
	for _, a := range addrs {
		if strings.HasPrefix(strings.ToLower(a.Addr), "fe80:") || a.Addr == "::1" {
			t.Fatalf("link-local/loopback leaked: %+v", a)
		}
	}
	// order: ens18 then ens20 per JSON
	if addrs[0].Iface != "ens18" || addrs[0].Addr != "2001:db8:1::42" || addrs[0].PrefixLen != 64 {
		t.Errorf("unexpected first addr: %+v", addrs[0])
	}
	if addrs[1].Iface != "ens20" || addrs[1].PrefixLen != 48 {
		t.Errorf("unexpected second addr: %+v", addrs[1])
	}
}
