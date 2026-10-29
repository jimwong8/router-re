package dpi

import "testing"

// limit 在 nft 侧只打 fwmark：nft 的 `limit rate` 是"匹配"语义（超过阈值只是
// 不再命中，并不丢包），真正限速由 tc/HTB 按 mark 整形完成。这个测试锁住
// 该契约，避免有人把 `limit rate ... mbytes/second` 写回来。
func TestRenderMarkNftLimit(t *testing.T) {
	rules := []MarkRule{
		{App: "netflix", Protocol: "tcp", DPorts: []int{443}, Action: "limit", RateMbit: 10},
	}
	out, err := RenderMarkNft(rules)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "meta mark set 200") {
		t.Fatalf("limit 规则必须打 fwmark(200)，实际输出: %s", out)
	}
	if contains(out, "limit rate") {
		t.Fatalf("limit 规则不得渲染 nft `limit rate`（只匹配不限速）: %s", out)
	}
}

// 显式 mark 的规则要沿用用户给的 mark，tc 侧按同一个值匹配。
func TestRenderMarkNftLimitExplicitMark(t *testing.T) {
	rules := []MarkRule{
		{App: "yt", Protocol: "tcp", DPorts: []int{443}, Action: "limit", Mark: 777, RateMbit: 5},
	}
	out, err := RenderMarkNft(rules)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(out, "meta mark set 777") {
		t.Fatalf("应沿用显式 mark 777，实际: %s", out)
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
