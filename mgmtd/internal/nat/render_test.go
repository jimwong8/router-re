package nat

import (
	"strings"
	"testing"
)

func TestRenderMasquerade(t *testing.T) {
	rules := []NATRule{
		{Type: "masquerade", Interface: "eth0"},
	}
	out := RenderNftables(rules)
	if !strings.Contains(out, "masquerade") {
		t.Fatalf("expected 'masquerade', got:\n%s", out)
	}
	if !strings.Contains(out, "postrouting") {
		t.Fatalf("expected postrouting chain, got:\n%s", out)
	}
}

func TestRenderPortForward(t *testing.T) {
	rules := []NATRule{
		{Type: "portforward", Proto: "tcp", SrcPort: "8080", DstAddr: "192.168.1.10", DstPort: "80"},
	}
	out := RenderNftables(rules)
	if !strings.Contains(out, "dnat to") {
		t.Fatalf("expected 'dnat to', got:\n%s", out)
	}
	// dnat target should include the DstAddr
	if !strings.Contains(out, "192.168.1.10") {
		t.Fatalf("expected dst addr in dnat, got:\n%s", out)
	}
	// dport match should be present once, no doubled proto
	if !strings.Contains(out, "tcp dport 8080") {
		t.Fatalf("expected 'tcp dport 8080', got:\n%s", out)
	}
	if strings.Contains(out, "tcp tcp") {
		t.Fatalf("proto duplicated:\n%s", out)
	}
}

func TestRenderPortForwardToPort(t *testing.T) {
	rules := []NATRule{
		{Type: "portforward", Proto: "tcp", SrcPort: "8080", DstAddr: "192.168.1.10", ToPort: "8081"},
	}
	out := RenderNftables(rules)
	// when ToPort differs from SrcPort, it should be appended as a port suffix
	if !strings.Contains(out, "dnat to 192.168.1.10:8081") {
		t.Fatalf("expected to-port suffix, got:\n%s", out)
	}
}

func TestRenderEmptyNoRules(t *testing.T) {
	out := RenderNftables(nil)
	// even with no rules, both chains should exist so the hooks are defined
	if !strings.Contains(out, "postrouting") || !strings.Contains(out, "prerouting") {
		t.Fatalf("expected both chains, got:\n%s", out)
	}
	if strings.Contains(out, "masquerade") {
		t.Fatalf("expected no masquerade with zero rules, got:\n%s", out)
	}
	if strings.Contains(out, "dnat to") {
		t.Fatalf("expected no dnat with zero rules, got:\n%s", out)
	}
}
