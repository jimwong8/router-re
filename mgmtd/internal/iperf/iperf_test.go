package iperf

import (
	"testing"
)

// fakeRunner records argv and returns canned output.
type fakeRunner struct {
	out string
	err error
	cmd []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmd = append([]string{name}, args...)
	return f.out, f.err
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}

const sampleTCP = `{
  "end": {
    "sum_sent": {"bytes": 12500000, "bits_per_second": 100000000, "retransmits": 3},
    "sum_received": {"bytes": 12400000, "bits_per_second": 99200000},
    "sum": {"bytes": 12400000, "bits_per_second": 99200000}
  }
}`

const sampleUDP = `{
  "end": {
    "sum_sent": {"bytes": 1100000, "bits_per_second": 8800000, "retransmits": 0},
    "sum_received": {"bytes": 1090000, "bits_per_second": 8720000},
    "sum": {"bytes": 1090000, "bits_per_second": 8720000, "lost_percent": 0.5, "jitter_ms": 0.12}
  }
}`

func TestValidateDefaults(t *testing.T) {
	c := Config{Host: "10.0.0.1"}
	if err := Validate(&c); err != nil {
		t.Fatal(err)
	}
	if c.Port != 5201 || c.Duration != 10 || c.Protocol != "tcp" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestValidateRejectsBad(t *testing.T) {
	cases := []Config{
		{Host: ""},
		{Host: "h", Port: 70000},
		{Host: "h", Duration: 0 + 400},
		{Host: "h", Protocol: "icmp"},
		{Host: "-evil"},
	}
	for _, c := range cases {
		if err := Validate(&c); err == nil {
			t.Fatalf("expected error for %+v", c)
		}
	}
}

func TestRenderArgsTCP(t *testing.T) {
	c := Config{Host: "10.0.0.1", Port: 5201, Duration: 5, Protocol: "tcp"}
	args := RenderArgs(c)
	want := []string{"-c", "10.0.0.1", "-p", "5201", "-t", "5", "-J"}
	if !equalStr(args, want) {
		t.Fatalf("got %v want %v", args, want)
	}
}

func TestRenderArgsUDP(t *testing.T) {
	c := Config{Host: "10.0.0.1", Port: 5201, Duration: 5, Protocol: "udp"}
	args := RenderArgs(c)
	if args[len(args)-1] != "-u" {
		t.Fatalf("udp missing -u: %v", args)
	}
}

func TestRunParsesTCP(t *testing.T) {
	fr := &fakeRunner{out: sampleTCP}
	res := Run(fr, Config{Host: "10.0.0.1", Port: 5201, Duration: 5, Protocol: "tcp"})
	if !res.OK {
		t.Fatalf("not ok: %s", res.Error)
	}
	if res.Bandwidth != 99200000 {
		t.Fatalf("bandwidth = %v, want 99200000", res.Bandwidth)
	}
	if res.Bytes != 12400000 {
		t.Fatalf("bytes = %d, want 12400000", res.Bytes)
	}
	if res.Retransmits != 3 {
		t.Fatalf("retransmits = %d, want 3", res.Retransmits)
	}
	if len(fr.cmd) == 0 || fr.cmd[0] != "iperf3" {
		t.Fatalf("runner not called with iperf3: %v", fr.cmd)
	}
}

func TestRunParsesUDP(t *testing.T) {
	fr := &fakeRunner{out: sampleUDP}
	res := Run(fr, Config{Host: "10.0.0.1", Port: 5201, Duration: 5, Protocol: "udp"})
	if !res.OK {
		t.Fatalf("not ok: %s", res.Error)
	}
	if res.LostPercent != 0.5 {
		t.Fatalf("lost_percent = %v, want 0.5", res.LostPercent)
	}
	if res.JitterMs != 0.12 {
		t.Fatalf("jitter_ms = %v, want 0.12", res.JitterMs)
	}
}

func TestRunUnavailable(t *testing.T) {
	fr := &fakeRunner{err: &fakeErr{msg: "iperf3: executable file not found"}}
	res := Run(fr, Config{Host: "10.0.0.1"})
	if res.OK {
		t.Fatal("expected not ok")
	}
	if !stringsContains(res.Error, "iperf3 not available") && !stringsContains(res.Error, "not found") {
		t.Fatalf("error should mention not available/not found: %s", res.Error)
	}
}

func TestRunBadJSON(t *testing.T) {
	fr := &fakeRunner{out: "not json"}
	res := Run(fr, Config{Host: "10.0.0.1"})
	if res.OK {
		t.Fatal("expected not ok")
	}
	if !stringsContains(res.Error, "parse failed") {
		t.Fatalf("error should mention parse failed: %s", res.Error)
	}
}

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
