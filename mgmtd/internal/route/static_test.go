package route

import (
	"strings"
	"testing"
)

func TestRenderStaticCommands(t *testing.T) {
	routes := []StaticRoute{
		{Dest: "10.0.0.0/8", Gateway: "192.168.1.1", Device: "eth0", Metric: 100},
		{Dest: "0.0.0.0/0", Gateway: "192.168.1.254", Device: "wan0"},
	}
	cmds := RenderStaticCommands(routes)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if !strings.Contains(cmds[0], "ip route replace 10.0.0.0/8") {
		t.Fatalf("unexpected first command: %s", cmds[0])
	}
	if !strings.Contains(cmds[0], "via 192.168.1.1") {
		t.Fatalf("expected gateway via, got %s", cmds[0])
	}
	if !strings.Contains(cmds[0], "dev eth0") {
		t.Fatalf("expected device, got %s", cmds[0])
	}
	if !strings.Contains(cmds[0], "metric 100") {
		t.Fatalf("expected metric, got %s", cmds[0])
	}
}

func TestSplitPriorityOrder(t *testing.T) {
	priority := []string{"static_route", "domain", "port", "protocol", "multilink", "default_gateway"}
	if len(SplitPriority) != len(priority) {
		t.Fatalf("expected %d priorities, got %d", len(priority), len(SplitPriority))
	}
	for i := range priority {
		if SplitPriority[i] != priority[i] {
			t.Fatalf("priority %d expected %s, got %s", i, priority[i], SplitPriority[i])
		}
	}
}
