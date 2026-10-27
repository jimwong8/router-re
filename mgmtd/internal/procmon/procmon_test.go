package procmon

import (
	"testing"
)

func TestParseUnits(t *testing.T) {
	out := `ssh.service loaded active running OpenSSH server daemon
nginx.service loaded inactive dead A high performance web server
cron.service loaded active exited Regular background program processing daemon
`
	units := ParseUnits(out)
	if len(units) != 3 {
		t.Fatalf("expected 3 units, got %d", len(units))
	}
	if units[0].Name != "ssh.service" || units[0].Load != "loaded" || units[0].Active != "active" || units[0].Sub != "running" {
		t.Fatalf("first unit wrong: %+v", units[0])
	}
	if units[0].Description != "OpenSSH server daemon" {
		t.Fatalf("description wrong: %q", units[0].Description)
	}
}

func TestValidateUnitNameInjection(t *testing.T) {
	for _, bad := range []string{
		"sshd; rm -rf /",
		"sshd$(id)",
		"sshd | cat",
		"sshd`id`",
		"sshd service",
	} {
		if err := ValidateUnitName(bad); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}

func TestValidateUnitNameNoSuffix(t *testing.T) {
	if err := ValidateUnitName("sshd"); err == nil {
		t.Fatal("expected error for missing suffix")
	}
	if err := ValidateUnitName("foo.txt"); err == nil {
		t.Fatal("expected error for bad suffix")
	}
}

func TestValidateUnitNameOK(t *testing.T) {
	for _, name := range []string{"sshd.service", "nginx.socket", "cron.timer", "multi-user.target"} {
		if err := ValidateUnitName(name); err != nil {
			t.Fatalf("expected valid %q, got %v", name, err)
		}
	}
}

func TestValidateActionRejects(t *testing.T) {
	for _, bad := range []string{"mask", "disable", "kill", "enable"} {
		if err := ValidateAction(AllowedAction(bad)); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}

func TestValidateActionOK(t *testing.T) {
	for _, a := range []AllowedAction{ActionStart, ActionStop, ActionRestart, ActionReload} {
		if err := ValidateAction(a); err != nil {
			t.Fatalf("expected valid %q, got %v", a, err)
		}
	}
}

func TestIsProtected(t *testing.T) {
	for _, u := range []string{"sshd.service", "networking.service", "mgmtd.service"} {
		if !IsProtected(u) {
			t.Fatalf("expected %s protected", u)
		}
	}
	if IsProtected("nginx.service") {
		t.Fatal("nginx should not be protected")
	}
}

func TestRenderCmd(t *testing.T) {
	args, err := RenderCmd(ActionRestart, "nginx.service")
	if err != nil {
		t.Fatalf("RenderCmd: %v", err)
	}
	if len(args) != 2 || args[0] != "restart" || args[1] != "nginx.service" {
		t.Fatalf("bad args: %v", args)
	}
	// 注入尝试应被拒
	if _, err := RenderCmd(ActionStart, "sshd; rm -rf /"); err == nil {
		t.Fatal("RenderCmd should reject injection")
	}
}

func TestParsePS(t *testing.T) {
	out := `  123 root      12.5  1.2  45678 /usr/sbin/sshd -D
  456 www-data   0.5  2.1  12345 nginx: worker process
`
	procs := ParsePS(out)
	if len(procs) != 2 {
		t.Fatalf("expected 2 procs, got %d", len(procs))
	}
	if procs[0].PID != 123 || procs[0].User != "root" {
		t.Fatalf("first proc wrong: %+v", procs[0])
	}
	if procs[0].Command != "/usr/sbin/sshd -D" {
		t.Fatalf("command wrong: %q", procs[0].Command)
	}
	if procs[1].Command != "nginx: worker process" {
		t.Fatalf("command with spaces wrong: %q", procs[1].Command)
	}
}

func TestTopN(t *testing.T) {
	procs := []Process{
		{PID: 1, CPU: 1.0},
		{PID: 2, CPU: 5.0},
		{PID: 3, CPU: 3.0},
	}
	top := TopN(procs, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2, got %d", len(top))
	}
	if top[0].PID != 2 || top[1].PID != 3 {
		t.Fatalf("order wrong: %+v", top)
	}
	// n 大于长度
	all := TopN(procs, 100)
	if len(all) != 3 {
		t.Fatalf("expected 3 (clamped), got %d", len(all))
	}
	// n <= 0
	none := TopN(procs, 0)
	if len(none) != 0 {
		t.Fatalf("expected 0, got %d", len(none))
	}
}

func TestValidatePID(t *testing.T) {
	for _, bad := range []int{0, 1, -5} {
		if err := ValidatePID(bad); err == nil {
			t.Fatalf("expected rejection for pid=%d", bad)
		}
	}
	if err := ValidatePID(42); err != nil {
		t.Fatalf("expected valid pid 42, got %v", err)
	}
}

func TestProtectedUnitsMap(t *testing.T) {
	m := ProtectedUnits()
	if len(m) == 0 {
		t.Fatal("protected units map must not be empty")
	}
	if !m["sshd.service"] {
		t.Fatal("sshd.service must be protected")
	}
}
