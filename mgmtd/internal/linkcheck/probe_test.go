package linkcheck

import (
	"strings"
	"testing"
)

func validTarget(kind string) Target {
	t := Target{
		ID: "t1", Name: "wan", Iface: "eth0", Kind: kind,
		Interval: 10, Timeout: 3, FailThreshold: 3, RecoverThreshold: 2, Enabled: true,
	}
	switch kind {
	case "icmp":
		t.Host = "8.8.8.8"
	case "dns":
		t.Host = "example.com"
	case "http":
		t.URL = "https://example.com"
	}
	return t
}

func TestValidateRejectsIfaceInjection(t *testing.T) {
	tt := validTarget("icmp")
	tt.Iface = "eth0; rm -rf /"
	if err := Validate(tt); err == nil {
		t.Fatal("expected rejection for injected iface")
	}
}

func TestValidateRejectsBadKind(t *testing.T) {
	tt := validTarget("icmp")
	tt.Kind = "tcp"
	if err := Validate(tt); err == nil {
		t.Fatal("expected rejection for bad kind")
	}
}

func TestValidateAcceptsThreeKinds(t *testing.T) {
	for _, k := range []string{"icmp", "dns", "http"} {
		if err := Validate(validTarget(k)); err != nil {
			t.Fatalf("expected valid %s, got %v", k, err)
		}
	}
}

func TestStepFailHysteresis(t *testing.T) {
	tt := validTarget("icmp")
	st := State{Up: true}
	// 2 failures: still up
	st, changed := Step(st, false, 100, tt)
	if !st.Up || changed {
		t.Fatalf("after 1 fail: up=%v changed=%v", st.Up, changed)
	}
	st, changed = Step(st, false, 101, tt)
	if !st.Up || changed {
		t.Fatalf("after 2 fails: up=%v changed=%v", st.Up, changed)
	}
	// 3rd failure: down
	st, changed = Step(st, false, 102, tt)
	if st.Up || !changed {
		t.Fatalf("after 3 fails: up=%v changed=%v", st.Up, changed)
	}
}

func TestStepRecoverHysteresis(t *testing.T) {
	tt := validTarget("icmp")
	st := State{Up: false}
	st, changed := Step(st, true, 100, tt)
	if st.Up || changed {
		t.Fatalf("after 1 ok: up=%v changed=%v", st.Up, changed)
	}
	st, changed = Step(st, true, 101, tt)
	if !st.Up || !changed {
		t.Fatalf("after 2 ok: up=%v changed=%v", st.Up, changed)
	}
}

func TestStepSuccessResetsConsecFail(t *testing.T) {
	tt := validTarget("icmp")
	st := State{Up: true, ConsecFail: 2}
	st, _ = Step(st, true, 100, tt)
	if st.ConsecFail != 0 {
		t.Fatalf("consec_fail should reset to 0, got %d", st.ConsecFail)
	}
}

func TestParseProbeOKIcmpReceived(t *testing.T) {
	tt := validTarget("icmp")
	ok, msg := ParseProbeOK(tt, "64 bytes from 8.8.8.8: icmp_seq=1 ttl=117 time=10.2 ms\n1 received", nil)
	if !ok {
		t.Fatalf("expected ok, msg=%s", msg)
	}
}

func TestParseProbeOKIcmpLoss(t *testing.T) {
	tt := validTarget("icmp")
	ok, _ := ParseProbeOK(tt, "100% packet loss", nil)
	if ok {
		t.Fatal("expected false for 100% packet loss")
	}
}

func TestParseProbeOKHttp404(t *testing.T) {
	tt := validTarget("http")
	ok, msg := ParseProbeOK(tt, "404", nil)
	if !ok {
		t.Fatalf("expected true for http 404 (link up), msg=%s", msg)
	}
	if !strings.Contains(msg, "404") {
		t.Fatalf("msg should contain status code, got %s", msg)
	}
}

func TestProbeArgsFirstNotCommandName(t *testing.T) {
	tt := validTarget("icmp")
	args, err := ProbeArgs(tt)
	if err != nil {
		t.Fatalf("ProbeArgs: %v", err)
	}
	if len(args) == 0 {
		t.Fatal("expected args")
	}
	// 首元素不应是命令名（ping/curl/dig）
	for _, cmd := range []string{"ping", "curl", "dig", "nslookup"} {
		if args[0] == cmd {
			t.Fatalf("first element should not be command name %q", cmd)
		}
	}
}
