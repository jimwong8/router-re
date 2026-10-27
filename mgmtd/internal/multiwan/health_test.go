package multiwan

import (
	"fmt"
	"testing"
)

func TestValidateMemberRejectsLoopback(t *testing.T) {
	m := Member{Name: "w", Iface: "lo", Gateway: "1.2.3.1", Weight: 1, Enabled: true}
	if err := ValidateMember(m); err == nil {
		t.Fatal("iface=lo must fail")
	}
}

func TestValidateMemberRejectsBadGateway(t *testing.T) {
	m := Member{Name: "w", Iface: "ppp0", Gateway: "not-an-ip", Weight: 1, Enabled: true}
	if err := ValidateMember(m); err == nil {
		t.Fatal("invalid gateway must fail")
	}
}

func TestValidateMemberRejectsZeroWeight(t *testing.T) {
	m := Member{Name: "w", Iface: "ppp0", Gateway: "1.2.3.1", Weight: 0, Enabled: true}
	if err := ValidateMember(m); err == nil {
		t.Fatal("weight=0 must fail")
	}
}

func TestValidateProbeRejectsTimeoutGeInterval(t *testing.T) {
	p := Probe{Type: "icmp", Target: "1.1.1.1", IntervalSec: 5, TimeoutSec: 5, FailThreshold: 2, RecoverThreshold: 2}
	if err := ValidateProbe(p); err == nil {
		t.Fatal("timeout>=interval must fail (probes would overlap)")
	}
	p2 := Probe{Type: "icmp", Target: "1.1.1.1", IntervalSec: 5, TimeoutSec: 6, FailThreshold: 2, RecoverThreshold: 2}
	if err := ValidateProbe(p2); err == nil {
		t.Fatal("timeout>interval must fail")
	}
}

func TestUpdateFailCountAccumulatesWhileUp(t *testing.T) {
	st := State{Name: "w", Up: true}
	p := Probe{FailThreshold: 3, RecoverThreshold: 2}
	st = Update(st, false, "err1", 1, p)
	if st.FailCount != 1 {
		t.Fatalf("after 1 fail: FailCount=%d, want 1", st.FailCount)
	}
	if !st.Up {
		t.Fatal("after 1 fail (threshold 3) should still be up")
	}
	st = Update(st, false, "err2", 2, p)
	if st.FailCount != 2 {
		t.Fatalf("after 2 fails: FailCount=%d, want 2", st.FailCount)
	}
	if !st.Up {
		t.Fatal("after 2 fails (threshold 3) should still be up")
	}
}

func TestUpdateDownAfterThreshold(t *testing.T) {
	st := State{Name: "w", Up: true}
	p := Probe{FailThreshold: 3, RecoverThreshold: 2}
	for i := 1; i <= 2; i++ {
		st = Update(st, false, fmt.Sprintf("err%d", i), int64(i), p)
		if !st.Up {
			t.Fatalf("after %d fails should still be up", i)
		}
	}
	st = Update(st, false, "err3", 3, p)
	if st.Up {
		t.Fatal("after 3 fails should be down")
	}
}

func TestUpdateRecoverAfterThreshold(t *testing.T) {
	st := State{Name: "w", Up: false, FailCount: 3}
	p := Probe{FailThreshold: 3, RecoverThreshold: 2}
	st = Update(st, true, "", 10, p)
	if st.Up {
		t.Fatal("after 1 success (threshold 2) should still be down")
	}
	if st.RecoverCount != 1 {
		t.Fatalf("RecoverCount=%d, want 1", st.RecoverCount)
	}
	st = Update(st, true, "", 11, p)
	if !st.Up {
		t.Fatal("after 2 successes should be up")
	}
}

func TestUpdateSuccessResetsFailCount(t *testing.T) {
	st := State{Name: "w", Up: true, FailCount: 2}
	p := Probe{FailThreshold: 3, RecoverThreshold: 2}
	st = Update(st, true, "", 5, p)
	if st.FailCount != 0 {
		t.Fatalf("success must reset FailCount to 0, got %d", st.FailCount)
	}
	if st.RecoverCount != 1 {
		t.Fatalf("success must increment RecoverCount, got %d", st.RecoverCount)
	}
}

func TestPickMembersAllDownEmpty(t *testing.T) {
	members := []Member{
		{Name: "a", Weight: 1, Enabled: true},
		{Name: "b", Weight: 3, Enabled: true},
	}
	states := map[string]State{
		"a": {Name: "a", Up: false},
		"b": {Name: "b", Up: false},
	}
	got := PickMembers(members, states, 0)
	if len(got) != 0 {
		t.Fatalf("all down must return empty slice, got %v", got)
	}
}

func TestPickMembersOnlyEnabled(t *testing.T) {
	members := []Member{
		{Name: "a", Weight: 1, Enabled: false},
		{Name: "b", Weight: 3, Enabled: true},
	}
	states := map[string]State{
		"a": {Name: "a", Up: true},
		"b": {Name: "b", Up: true},
	}
	for r := 0; r < 4; r++ {
		got := PickMembers(members, states, r)
		if len(got) != 1 || got[0].Name != "b" {
			t.Fatalf("round %d: expected only b, got %v", r, got)
		}
	}
}

func TestPickMembersWeighted(t *testing.T) {
	members := []Member{
		{Name: "slow", Weight: 1, Enabled: true},
		{Name: "fast", Weight: 3, Enabled: true},
	}
	states := map[string]State{
		"slow": {Name: "slow", Up: true},
		"fast": {Name: "fast", Up: true},
	}
	counts := map[string]int{}
	for r := 0; r < 4; r++ {
		got := PickMembers(members, states, r)
		if len(got) != 1 {
			t.Fatalf("round %d: expected 1 pick, got %d", r, len(got))
		}
		counts[got[0].Name]++
	}
	if counts["fast"] != 3 || counts["slow"] != 1 {
		t.Fatalf("weighted distribution: fast=%d slow=%d, want fast=3 slow=1", counts["fast"], counts["slow"])
	}
}
