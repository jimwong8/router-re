package dpi

import "testing"

func TestRenderMarkNftLimit(t *testing.T) {
	rules := []MarkRule{
		{App: "netflix", Protocol: "tcp", DPorts: []int{443}, Action: "limit", RateMbit: 10},
	}
	out, err := RenderMarkNft(rules)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "limit rate 10 mbytes/second") {
		t.Fatalf("missing limit render: %s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
