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

// recordingRunner records every command; sysctl -n reads return a value that
// can be overridden via verifyVal.
type recordingRunner struct {
	cmds      []string
	netns     []string
	verifyVal string // value returned by sysctl -n (defaults per key)
	failRead  bool
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	return r.run(name, args...)
}
func (r *recordingRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return r.run(name, args...)
}
func (r *recordingRunner) run(name string, args ...string) (string, error) {
	full := name + " " + strings.Join(args, " ")
	r.cmds = append(r.cmds, full)
	if name == "sh" && strings.Contains(full, "ls /sys/class/net") {
		return strings.Join(r.netns, "\n"), nil
	}
	if name == "sysctl" && len(args) >= 2 && args[0] == "-n" {
		if r.failRead {
			return "0", nil
		}
		if r.verifyVal != "" {
			return r.verifyVal, nil
		}
		if strings.Contains(strings.Join(args, " "), "arp_ignore") {
			return "1", nil
		}
		return "2", nil
	}
	return "", nil
}

func TestApplyArpGuardWritesAllScopes(t *testing.T) {
	r := &recordingRunner{netns: []string{"lo", "ens18", "eth0"}}
	if err := ApplyArpGuard(r, "ens18"); err != nil {
		t.Fatalf("ApplyArpGuard for ens18 failed: %v", err)
	}
	joined := strings.Join(r.cmds, "\n")
	for _, scope := range []string{"all", "default", "ens18"} {
		if !strings.Contains(joined, "sysctl -w net.ipv4.conf."+scope+".arp_ignore=1") {
			t.Errorf("missing arp_ignore set for scope %s; got:\n%s", scope, joined)
		}
		if !strings.Contains(joined, "sysctl -w net.ipv4.conf."+scope+".arp_announce=2") {
			t.Errorf("missing arp_announce set for scope %s; got:\n%s", scope, joined)
		}
		if !strings.Contains(joined, "sysctl -n net.ipv4.conf."+scope+".arp_ignore") {
			t.Errorf("missing verify read for %s.arp_ignore", scope)
		}
	}
}

func TestApplyArpGuardAllEnumerates(t *testing.T) {
	r := &recordingRunner{netns: []string{"lo", "ens18", "eth0"}}
	if err := ApplyArpGuard(r, "all"); err != nil {
		t.Fatalf("ApplyArpGuard all failed: %v", err)
	}
	joined := strings.Join(r.cmds, "\n")
	for _, scope := range []string{"all", "default", "ens18", "eth0"} {
		if !strings.Contains(joined, "net.ipv4.conf."+scope+".arp_ignore=1") {
			t.Errorf("missing scope %s in all-enumeration; got:\n%s", scope, joined)
		}
	}
	if strings.Contains(joined, "net.ipv4.conf.lo.arp_") {
		t.Errorf("lo should be excluded; got:\n%s", joined)
	}
}

func TestApplyArpGuardVerifyFailure(t *testing.T) {
	r := &recordingRunner{netns: []string{"lo", "ens18"}}
	r.failRead = true // sysctl -n returns "0", mismatch for value "1"
	err := ApplyArpGuard(r, "ens18")
	if err == nil {
		t.Fatal("expected verify-mismatch error")
	}
	// error must name the failing scope/param, not be generic
	if !strings.Contains(err.Error(), "net.ipv4.conf.") || !strings.Contains(err.Error(), "arp_") {
		t.Errorf("error should list exact scope+param, got: %v", err)
	}
}
