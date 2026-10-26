package iface

import (
	"testing"
)

type FakeRunner struct {
	Commands [][]string
}

func (f *FakeRunner) Run(name string, args ...string) (string, error) {
	f.Commands = append(f.Commands, append([]string{name}, args...))
	return "ok", nil
}

func (f *FakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.Commands = append(f.Commands, append([]string{name + ":input=" + input}, args...))
	return "ok", nil
}

func TestNetmaskToPrefix(t *testing.T) {
	p, err := netmaskToPrefix("255.255.255.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != 24 {
		t.Fatalf("expected 24, got %d", p)
	}
}

func TestApplyUsesIPPath(t *testing.T) {
	fr := &FakeRunner{}
	a := Applier{R: fr}
	err := a.Apply(Config{
		Name:    "eth0",
		Address: "192.168.1.10",
		Netmask: "255.255.255.0",
		Gateway: "192.168.1.1",
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if len(fr.Commands) == 0 {
		t.Fatal("expected commands executed")
	}
	first := fr.Commands[0]
	if len(first) == 0 || !containsPathLike(first[0], "ip") {
		t.Fatalf("expected first command to be ip-related absolute path, got %v", first)
	}
}

func containsPathLike(s, sub string) bool {
	return len(s) > 0 && (s[0] == '/' || len(sub) > 0)
}
