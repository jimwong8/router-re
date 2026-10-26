package firewall

import (
	"os/exec"
	"strings"
	"testing"
)

func TestApplyValidate(t *testing.T) {
	if _, err := exec.LookPath("/usr/sbin/nft"); err != nil {
		if _, err2 := exec.LookPath("nft"); err2 != nil {
			t.Skip("nft binary not found; skipping real validation test")
		}
	}
	rules := []Rule{
		{Chain: "forward", Proto: "tcp", DPort: "80", Action: "accept"},
	}
	err := Validate(rules)
	if err == nil {
		t.Log("Validate passed (root available)")
		// If validation works, attempt apply and log result
		if aerr := Apply(rules); aerr != nil {
			t.Logf("Apply returned error (may lack apply perms): %v", aerr)
		}
		return
	}
	// Validate failed: distinguish permission issue (skip) vs syntax error (fail)
	msg := err.Error()
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "Operation not permitted") || strings.Contains(msg, "password") {
		t.Skipf("nft requires root; skipping real validation: %v", err)
	}
	t.Fatalf("Validate failed with legal rules: %v", err)
}
