package svcmgr

import (
	"testing"
)

// FakeRunner records argv slices and returns configurable results.
type FakeRunner struct {
	Commands [][]string
	// Results maps a command key "name arg" -> (stdout, err). nil err = success.
	Results map[string]error
}

func (f *FakeRunner) Run(name string, args ...string) (string, error) {
	cmd := append([]string{name}, args...)
	f.Commands = append(f.Commands, cmd)
	if f.Results != nil {
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
		if err, ok := f.Results[key]; ok {
			if err != nil {
				return "", err
			}
			return "ok", nil
		}
	}
	return "ok", nil
}

func (f *FakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return f.Run(name, args...)
}

func newFake() *FakeRunner { return &FakeRunner{Results: map[string]error{}} }

func TestStatusAllReturnsFourServices(t *testing.T) {
	fr := newFake()
	m := New(fr)
	got := m.StatusAll()
	if len(got) != len(Services) {
		t.Fatalf("expected %d services, got %d", len(Services), len(got))
	}
	for i, s := range got {
		if s.Name != Services[i] {
			t.Fatalf("service[%d] name = %q, want %q", i, s.Name, Services[i])
		}
	}
}

func TestSetEnabledTrueCallsEnable(t *testing.T) {
	fr := newFake()
	m := New(fr)
	if err := m.SetEnabled("ssh", true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}
	if len(fr.Commands) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(fr.Commands))
	}
	want := []string{"/etc/init.d/ssh", "enable"}
	if !equalSlice(fr.Commands[0], want) {
		t.Fatalf("runner called with %v, want %v", fr.Commands[0], want)
	}
}

func TestSetEnabledFalseCallsDisable(t *testing.T) {
	fr := newFake()
	m := New(fr)
	if err := m.SetEnabled("ftp", false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if len(fr.Commands) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(fr.Commands))
	}
	want := []string{"/etc/init.d/ftp", "disable"}
	if !equalSlice(fr.Commands[0], want) {
		t.Fatalf("runner called with %v, want %v", fr.Commands[0], want)
	}
}

func TestSetEnabledUnknownService(t *testing.T) {
	fr := newFake()
	m := New(fr)
	if err := m.SetEnabled("evil;rm", true); err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
	if len(fr.Commands) != 0 {
		t.Fatalf("runner should not be called for unknown service, got %d calls", len(fr.Commands))
	}
}

func TestStatusEnabledRunningFromRunner(t *testing.T) {
	fr := newFake()
	fr.Results["/etc/init.d/ssh enabled"] = nil   // enabled
	fr.Results["/etc/init.d/ssh running"] = fmtErr // not running
	m := New(fr)
	st, err := m.Status("ssh")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.Enabled {
		t.Fatal("expected Enabled=true when 'enabled' probe succeeds")
	}
	if st.Running {
		t.Fatal("expected Running=false when 'running' probe fails")
	}
}

func TestStatusUnknownService(t *testing.T) {
	fr := newFake()
	m := New(fr)
	if _, err := m.Status("nope"); err == nil {
		t.Fatal("expected error for unknown service")
	}
}

func equalSlice(a, b []string) bool {
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

// fmtErr is a sentinel error for fake runner failures.
var fmtErr = &fakeErr{msg: "not running"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
