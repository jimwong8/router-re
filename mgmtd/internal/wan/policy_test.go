package wan

import (
	"strings"
	"testing"
)

func TestPickLinkWeightedDistribution(t *testing.T) {
	links := []WANLink{
		{Name: "wan0", Interface: "eth0", Gateway: "1.1.1.1", Weight: 1, Table: 100},
		{Name: "wan1", Interface: "eth1", Gateway: "2.2.2.2", Weight: 2, Table: 101},
	}
	b := NewBalancer()
	counts := map[string]int{}
	for i := 0; i < 3; i++ {
		l := b.PickLink(links)
		counts[l.Name]++
	}
	if counts["wan0"] != 1 || counts["wan1"] != 2 {
		t.Fatalf("expected distribution 1:2, got wan0=%d wan1=%d", counts["wan0"], counts["wan1"])
	}
}

func TestRenderPolicyRouting(t *testing.T) {
	links := []WANLink{
		{Name: "wan0", Interface: "eth0", Gateway: "1.1.1.1", Weight: 1, Table: 100},
	}
	cmds := RenderPolicyRouting(links)
	if len(cmds) == 0 {
		t.Fatal("expected commands")
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "ip route add") {
		t.Fatalf("expected 'ip route add', got:\n%s", joined)
	}
	if !strings.Contains(joined, "table") {
		t.Fatalf("expected 'table', got:\n%s", joined)
	}
	if !strings.Contains(joined, "1.1.1.1") {
		t.Fatalf("expected gateway in commands, got:\n%s", joined)
	}
}
