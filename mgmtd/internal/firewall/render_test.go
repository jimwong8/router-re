package firewall

import "testing"

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRenderNftables(t *testing.T) {
	rules := []Rule{{Chain: "forward", Proto: "tcp", DPort: "80", Action: "accept"}}
	out := RenderNftables(rules)
	if out == "" {
		t.Fatal("empty output")
	}
	if !contains(out, "tcp dport 80") || !contains(out, "accept") {
		t.Fatalf("rule not rendered correctly: %s", out)
	}
	// 关键：DPort 已带 proto 前缀，不应出现重复 proto
	if contains(out, "tcp tcp dport") {
		t.Fatalf("duplicate proto in rule: %s", out)
	}
}
