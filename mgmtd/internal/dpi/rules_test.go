package dpi

import (
	"strings"
	"testing"
)

// TestRenderNftRulesDport 验证 dport 规则渲染出 "dport" 与 "drop"。
func TestRenderNftRulesDport(t *testing.T) {
	rules := []AppRule{
		{Name: "bittorrent", Category: "p2p", MatchType: "dport", MatchValue: "6881", Action: "drop", Priority: 10},
		{Name: "netflix", Category: "video", MatchType: "host", MatchValue: "10.0.0.1", Action: "accept", Priority: 5},
	}
	out := RenderNftRules(rules)
	if !strings.Contains(out, "dport") {
		t.Fatalf("output missing dport:\n%s", out)
	}
	if !strings.Contains(out, "drop") {
		t.Fatalf("output missing drop:\n%s", out)
	}
	// drop 规则（dport=6881）应在 accept 规则之前（Priority 10 > 5）
	dportIdx := strings.Index(out, "dport")
	acceptIdx := strings.Index(out, "accept")
	if dportIdx > acceptIdx {
		t.Fatalf("higher-priority dport rule should render first:\n%s", out)
	}
}

// TestRenderNftRulesLimit 验证 limit action 渲染出 "limit rate"。
func TestRenderNftRulesLimit(t *testing.T) {
	rules := []AppRule{
		{Name: "gaming", Category: "game", MatchType: "dport", MatchValue: "tcp/27015", Action: "limit", RateMbps: 20, Priority: 3},
	}
	out := RenderNftRules(rules)
	if !strings.Contains(out, "limit rate 20 mbytes/second") {
		t.Fatalf("output missing limit rate:\n%s", out)
	}
	if !strings.Contains(out, "tcp dport 27015") {
		t.Fatalf("output missing tcp dport 27015:\n%s", out)
	}
}

// TestNewDportClassifierHit 验证分类器命中匹配端口的 Flow。
func TestNewDportClassifierHit(t *testing.T) {
	classifier := NewDportClassifier([]AppRule{
		{Name: "dns", Category: "other", MatchType: "dport", MatchValue: "udp/53", Action: "accept", Priority: 1},
		{Name: "p2p", Category: "p2p", MatchType: "dport", MatchValue: "6881", Action: "drop", Priority: 10},
	})
	// 命中 6881 → p2p drop
	rule, ok := classifier.Classify(Flow{DstPort: 6881, Proto: "tcp"})
	if !ok {
		t.Fatal("expected hit for port 6881")
	}
	if rule.Name != "p2p" || rule.Action != "drop" {
		t.Fatalf("unexpected rule: %+v", rule)
	}
	// 命中 udp/53 → dns（proto 匹配）
	rule, ok = classifier.Classify(Flow{DstPort: 53, Proto: "udp"})
	if !ok || rule.Name != "dns" {
		t.Fatalf("expected dns hit for udp/53, got %+v ok=%v", rule, ok)
	}
	// 不命中（tcp/53 但规则要求 udp/53）
	_, ok = classifier.Classify(Flow{DstPort: 53, Proto: "tcp"})
	if ok {
		t.Fatal("tcp/53 should not match udp/53 rule")
	}
	// 未定义端口 → 不命中
	_, ok = classifier.Classify(Flow{DstPort: 9999, Proto: "tcp"})
	if ok {
		t.Fatal("port 9999 should not match")
	}
}
