package acl

import (
	"strings"
	"testing"
)

func TestRenderSysctlArpGuard(t *testing.T) {
	out := RenderSysctl("eth0")
	if !strings.Contains(out, "net.ipv4.conf.eth0.arp_ignore=1") {
		t.Fatal("missing arp_ignore=1")
	}
	if !strings.Contains(out, "net.ipv4.conf.eth0.arp_announce=2") {
		t.Fatal("missing arp_announce=2")
	}
}

func TestApplyArpGuardVerify(t *testing.T) {
	r := &fakeRunner{}
	// set via `sh -c` always ok; verify `sysctl -n` returns the value matching
	// the key being read, so both checks pass.
	r.runHook = func(name string, args ...string) (string, error) {
		if name == "sysctl" && len(args) >= 2 && args[0] == "-n" {
			switch args[1] {
			case "net.ipv4.conf.eth0.arp_ignore":
				return "1", nil
			case "net.ipv4.conf.eth0.arp_announce":
				return "2", nil
			}
		}
		return "ok", nil
	}
	if err := ApplyArpGuard(r, "eth0"); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestApplyArpGuardVerifyFailure(t *testing.T) {
	// Simulate kernel rejecting the value back (verify mismatch).
	r := &fakeRunner{}
	calls := 0
	r.runHook = func(name string, args ...string) (string, error) {
		calls++
		if name == "sh" {
			return "ok", nil
		}
		// sysctl -n returns "0" (unchanged) -> verify mismatch
		return "0", nil
	}
	if err := ApplyArpGuard(r, "eth0"); err == nil {
		t.Fatal("expected verify-mismatch error")
	}
}

func TestStaticBindingsParse(t *testing.T) {
	jsonStr := `[{"if":"eth0","ip":"10.0.0.1","mac":"aa:bb:cc:dd:ee:ff","state":"REACHABLE"}]`
	r := &fakeRunner{out: jsonStr, err: nil}
	rows, err := StaticBindings(r)
	if err != nil {
		t.Fatalf("StaticBindings error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["ip"] != "10.0.0.1" || rows[0]["mac"] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected row: %v", rows[0])
	}
}
