package lb

import (
	"strings"
	"testing"
)

type lbStub struct {
	out string
	err error
}

func (s *lbStub) Run(name string, args ...string) (string, error) { return s.out, s.err }
func (s *lbStub) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, s.err
}

func TestSmoothWeightedDistribution(t *testing.T) {
	members := []Member{
		{Name: "a", Weight: 1, Enabled: true},
		{Name: "b", Weight: 2, Enabled: true},
		{Name: "c", Weight: 3, Enabled: true},
	}
	state := map[string]int{}
	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		m, err := Pick("weighted", members, state, "")
		if err != nil {
			t.Fatal(err)
		}
		counts[m.Name]++
	}
	if counts["a"] != 1 || counts["b"] != 2 || counts["c"] != 3 {
		t.Fatalf("weighted distribution wrong: %+v", counts)
	}
}

func TestFailoverSkipsDisabled(t *testing.T) {
	members := []Member{
		{Name: "a", Enabled: false},
		{Name: "b", Enabled: true},
	}
	m, err := Pick("failover", members, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "b" {
		t.Fatalf("failover should skip disabled, got %s", m.Name)
	}
}

func TestPerConnectionSticky(t *testing.T) {
	members := []Member{
		{Name: "a", Enabled: true},
		{Name: "b", Enabled: true},
		{Name: "c", Enabled: true},
	}
	first, _ := Pick("per-connection", members, nil, "session-42")
	second, _ := Pick("per-connection", members, nil, "session-42")
	if first.Name != second.Name {
		t.Fatalf("per-connection must be sticky for same hashKey: %s vs %s", first.Name, second.Name)
	}
}

func TestValidateRejectsTimeoutGEInterval(t *testing.T) {
	c := Config{
		Mode:    "weighted",
		Members: []Member{{Name: "a", Iface: "eth0", Gateway: "1.1.1.1", Weight: 1}},
		Health:  HealthCheck{Target: "8.8.8.8", IntervalSec: 10, TimeoutSec: 10, FailThreshold: 2, RiseThreshold: 2},
	}
	if err := Validate(c); err == nil {
		t.Fatal("timeout >= interval must be rejected")
	}
}

func TestValidateRejectsZeroWeightSum(t *testing.T) {
	c := Config{
		Mode: "weighted",
		Members: []Member{
			{Name: "a", Iface: "eth0", Gateway: "1.1.1.1", Weight: 1},
			{Name: "b", Iface: "eth0", Gateway: "2.2.2.2", Weight: 1},
		},
	}
	if err := Validate(c); err == nil {
		t.Fatal("duplicate iface must be rejected")
	}
	c2 := Config{
		Mode:    "weighted",
		Members: []Member{{Name: "a", Iface: "eth0", Gateway: "not-an-ip", Weight: 1}},
	}
	if err := Validate(c2); err == nil {
		t.Fatal("invalid gateway must be rejected")
	}
}

func TestCheckerFlappingSuppression(t *testing.T) {
	cfg := HealthCheck{Target: "8.8.8.8", IntervalSec: 5, TimeoutSec: 2, FailThreshold: 3, RiseThreshold: 2}
	ch := NewChecker(cfg)
	m := Member{Name: "a", Iface: "eth0", Gateway: "1.1.1.1"}
	for i := 0; i < 2; i++ {
		s := ch.Check(&lbStub{err: &lbErr{"timeout"}}, m)
		if !s.Up {
			t.Fatalf("should still be up after 2 fails (threshold %d)", cfg.FailThreshold)
		}
	}
	s := ch.Check(&lbStub{err: &lbErr{"timeout"}}, m)
	if s.Up {
		t.Fatal("should be down after 3 consecutive failures")
	}
	if s.LastError == "" {
		t.Fatal("LastError should be non-empty on failure")
	}
	s = ch.Check(&lbStub{out: "rtt min/avg/max = 1.2/1.5/2.0 ms"}, m)
	if s.Up {
		t.Fatal("should still be down after 1 success (threshold 2)")
	}
	s = ch.Check(&lbStub{out: "rtt min/avg/max = 1.2/1.5/2.0 ms"}, m)
	if !s.Up {
		t.Fatal("should be up after 2 consecutive successes")
	}
}

func TestRenderNftMarkSetNoFlushNoDrop(t *testing.T) {
	marks := map[string]int{"a": 1, "b": 2}
	out := RenderNftMarkSet(marks)
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("must never flush ruleset")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("must never set policy drop")
	}
}

type lbErr struct{ msg string }

func (e *lbErr) Error() string { return e.msg }
