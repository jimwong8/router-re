package diag

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// fakeRunner records commands and returns canned output / errors.
type fakeRunner struct {
	out string
	err error
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) { return f.out, f.err }
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.out, f.err
}

var _ execx.Runner = (*fakeRunner)(nil)

func TestValidateHostInjection(t *testing.T) {
	bad := []string{
		"example.com;ls",
		"example.com|cat /etc/passwd",
		"example.com&&reboot",
		"$(id)",
		"`id`",
		"example.com > /tmp/x",
		"a b.com",
		"-example.com",
		"example.com<&1",
		"example.com&",
		"example.com;",
		"exa(mple).com",
	}
	for _, h := range bad {
		if err := ValidateHost(h); err == nil {
			t.Errorf("ValidateHost(%q) should reject", h)
		}
	}
}

func TestValidateHostOK(t *testing.T) {
	good := []string{
		"example.com",
		"10.100.1.1",
		"2001:db8::1",
		"ns1.example.org",
		"a",
		"my-host.local",
	}
	for _, h := range good {
		if err := ValidateHost(h); err != nil {
			t.Errorf("ValidateHost(%q) should pass, got: %v", h, err)
		}
	}
}

func TestPingCountBounds(t *testing.T) {
	r := &fakeRunner{out: "PING example.com (1.2.3.4) 56(84) bytes of data.\n--- example.com ping statistics ---\n4 packets transmitted, 4 received, 0% packet loss, time 3005ms\nrtt min/avg/max/mdev = 10.1/12.3/15.7/1.2 ms\n"}
	if _, err := Ping(r, "example.com", 0); err != nil {
		t.Errorf("count 0 should default to 4, got: %v", err)
	}
	if _, err := Ping(r, "example.com", 100); err == nil {
		t.Error("count 100 must be rejected")
	}
	if _, err := Ping(r, "example.com", -1); err == nil {
		t.Error("negative count must be rejected")
	}
	if _, err := Ping(r, "example.com", 11); err == nil {
		t.Error("count 11 must be rejected")
	}
}

func TestPingMissingCommandNoPanic(t *testing.T) {
	r := &fakeRunner{err: &execxError{"exec: \"ping\": executable file not found in $PATH"}}
	_, err := Ping(r, "example.com", 4)
	if err == nil {
		t.Fatal("expected error for missing ping")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected 'ping not installed', got: %v", err)
	}
}

func TestPingParseStats(t *testing.T) {
	r := &fakeRunner{out: "PING example.com (93.184.216.34) 56(84) bytes of data.\n64 bytes from 93.184.216.34: icmp_seq=1 ttl=57 time=10.1 ms\n64 bytes from 93.184.216.34: icmp_seq=2 ttl=57 time=12.3 ms\n--- example.com ping statistics ---\n2 packets transmitted, 2 received, 0% packet loss, time 1001ms\nrtt min/avg/max/mdev = 10.100/11.200/12.300/1.100 ms\n"}
	res, err := Ping(r, "example.com", 2)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if res.ResolvedIP != "93.184.216.34" {
		t.Errorf("ResolvedIP = %q", res.ResolvedIP)
	}
	if res.Sent != 2 || res.Received != 2 || res.LossPct != 0 {
		t.Errorf("stats wrong: %+v", res)
	}
	if res.MinMs != 10.1 || res.AvgMs != 11.2 || res.MaxMs != 12.3 {
		t.Errorf("rtt wrong: %+v", res)
	}
}

func TestDNSLookupNameValidate(t *testing.T) {
	r := &fakeRunner{out: ""}
	if _, err := DNSLookup(r, "example.com;rm -rf /", ""); err == nil {
		t.Error("injected name must be rejected")
	}
	if _, err := DNSLookup(r, "2001:db8::1", ""); err == nil {
		t.Error("IPv6 literal as DNS name must be rejected")
	}
}

func TestDNSLookupDigMissing(t *testing.T) {
	r := &fakeRunner{err: &execxError{"exec: \"dig\": executable file not found in $PATH"}}
	_, err := DNSLookup(r, "example.com", "8.8.8.8")
	if err == nil {
		t.Fatal("expected error when dig absent")
	}
	if !strings.Contains(err.Error(), "dig not installed") {
		t.Errorf("expected 'dig not installed', got: %v", err)
	}
}

func TestDNSLookupGetentFallback(t *testing.T) {
	// dig missing + empty server → getent hosts fallback
	r := &fakeRunner{err: &execxError{"exec: \"dig\": executable file not found in $PATH"}}
	_, err := DNSLookup(r, "example.com", "")
	if err == nil {
		t.Fatal("expected error when both dig and getent unavailable")
	}
	_ = err
}

// execxError mimics exec.ExitError / error.ExitError text.
type execxError struct{ msg string }

func (e *execxError) Error() string { return e.msg }

func TestInterfaceStatsSkipsLo(t *testing.T) {
	r := &fakeRunner{out: `[{"ifname":"lo","stats64":{"rx_bytes":1,"rx_packets":1,"rx_errors":0,"rx_dropped":0,"tx_bytes":1,"tx_packets":1,"tx_errors":0,"tx_dropped":0}},{"ifname":"ens18","stats64":{"rx_bytes":100,"rx_packets":10,"rx_errors":0,"rx_dropped":1,"tx_bytes":200,"tx_packets":20,"tx_errors":0,"tx_dropped":0}}]`}
	rows, err := InterfaceStats(r)
	if err != nil {
		t.Fatalf("InterfaceStats failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 iface (lo skipped), got %d", len(rows))
	}
	if rows[0]["iface"] != "ens18" {
		t.Errorf("wrong iface: %v", rows[0])
	}
	if rows[0]["rx_bytes"] != "100" {
		t.Errorf("wrong rx_bytes: %v", rows[0])
	}
}
