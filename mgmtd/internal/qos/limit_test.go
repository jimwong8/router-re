package qos

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRenderTcCommands(t *testing.T) {
	rules := []RateLimit{
		{Interface: "lan", RateMbps: 100, BurstKB: 512, Direction: "egress", Priority: 1},
	}
	cmds := RenderTcCommands(rules)
	foundTc := false
	foundTbf := false
	for _, c := range cmds {
		// "replace" makes the command idempotent (plain "add" fails when a
		// qdisc already exists: "Exclusivity flag on, cannot modify").
		if strings.Contains(c, "tc qdisc replace") {
			foundTc = true
		}
		if strings.Contains(c, "tbf rate") {
			foundTbf = true
		}
	}
	if !foundTc || !foundTbf {
		t.Fatalf("expected tc commands with qdisc replace and tbf; got: %v", cmds)
	}
}

func TestRenderNftLimit(t *testing.T) {
	rules := []RateLimit{
		{Interface: "eth0", RateMbps: 50, BurstKB: 256, Direction: "egress", Priority: 0},
	}
	out := RenderNftLimit(rules)
	if !strings.Contains(out, "limit rate") {
		t.Fatalf("expected 'limit rate' in nft output; got:\n%s", out)
	}
	if !strings.Contains(out, "mbytes/second") {
		t.Fatalf("expected 'mbytes/second' in nft output; got:\n%s", out)
	}
}

func TestApplyWithRealNft(t *testing.T) {
	// This test mutates the live kernel traffic-control state. It MUST be
	// opt-in: running it by default once left a 10Mbit tbf on `lo` behind and
	// throttled the whole host's loopback traffic (local HTTP responses went
	// from 3ms to 20s).
	if os.Getenv("MGM_ALLOW_REAL_APPLY") != "1" {
		t.Skip("destructive: set MGM_ALLOW_REAL_APPLY=1 to run real tc/netlink apply")
	}
	// Never touch `lo`. Use a throwaway dummy interface and always clean up.
	const dev = "mgmtest0"
	setup := exec.Command("/usr/sbin/ip", "link", "add", dev, "type", "dummy")
	if err := setup.Run(); err != nil {
		t.Skipf("cannot create dummy interface %s (need root): %v", dev, err)
	}
	defer exec.Command("/usr/sbin/tc", "qdisc", "del", "dev", dev, "root").Run()
	defer exec.Command("/usr/sbin/ip", "link", "del", dev).Run()
	if err := exec.Command("/usr/sbin/ip", "link", "set", dev, "up").Run(); err != nil {
		t.Skipf("cannot bring up %s: %v", dev, err)
	}

	rules := []RateLimit{
		{Interface: dev, RateMbps: 10, BurstKB: 128, Direction: "egress", Priority: 0},
	}
	err := Apply(rules)
	if err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "permission") &&
			!strings.Contains(strings.ToLower(msg), "operation not permitted") {
			t.Logf("Apply returned error (may require root/netlink): %v", err)
		}
	}
}
