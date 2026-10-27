package threat

import (
	"strings"
	"testing"
)

func TestRenderNftContainsInterval(t *testing.T) {
	entries := []Entry{
		{IP: "203.0.113.5"},
		{IP: "10.0.0.0/8"},
	}
	out := RenderNft(entries)
	if !strings.Contains(out, "flags interval") {
		t.Fatal("CIDR 需要 flags interval，必须存在")
	}
	if !strings.Contains(out, "table inet router_threat") {
		t.Fatal("缺 table 声明")
	}
	// 两类集合都应渲染
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "blocked6") {
		t.Fatal("应渲染 v4 与 v6 两个集合")
	}
	// forward 链命中集合 drop
	if !strings.Contains(out, "ip saddr @blocked drop") {
		t.Fatal("缺 ip saddr @blocked drop")
	}
	if !strings.Contains(out, "ip6 saddr @blocked6 drop") {
		t.Fatal("缺 ip6 saddr @blocked6 drop")
	}
}

// TestRenderNftNoFlushRuleset —— 最硬的红线。
func TestRenderNftNoFlushRuleset(t *testing.T) {
	out := RenderNft([]Entry{{IP: "203.0.113.5"}})
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("绝不允许 flush ruleset")
	}
}

// TestRenderNftNoPolicyDrop —— 只 drop 命中集合，默认放行。
func TestRenderNftNoPolicyDrop(t *testing.T) {
	out := RenderNft([]Entry{{IP: "203.0.113.5"}})
	// 必须显式 policy accept
	if !strings.Contains(out, "policy accept") {
		t.Fatal("链必须 policy accept（默认放行）")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("绝不允许 policy drop")
	}
	// 只 drop 命中集合的，不应有无条件 drop
	if strings.Contains(out, "drop;") && !strings.Contains(out, "@blocked") {
		t.Fatal("不应有无条件 drop")
	}
}

func TestRenderNftElements(t *testing.T) {
	out := RenderNft([]Entry{
		{IP: "203.0.113.5"},
		{IP: "10.0.0.0/8"},
	})
	if !strings.Contains(out, "203.0.113.5") {
		t.Fatal("应包含 v4 元素")
	}
	if !strings.Contains(out, "10.0.0.0/8") {
		t.Fatal("应包含 CIDR 元素")
	}
}

func TestRenderNftEmpty(t *testing.T) {
	out := RenderNft(nil)
	// 空集合也要输出合法骨架，仍不能 flush/policy drop
	if strings.Contains(out, "flush ruleset") || strings.Contains(out, "policy drop") {
		t.Fatal("空渲染也不可触碰红线")
	}
	if !strings.Contains(out, "policy accept") {
		t.Fatal("空渲染也须 policy accept")
	}
}

func TestRenderHostsDeny(t *testing.T) {
	entries := []Entry{
		{IP: "203.0.113.5", Reason: "ssh brute"},
		{IP: "10.0.0.0/8", Reason: "cidr net"},
		{IP: "2001:db8::1", Reason: "v6"},
	}
	out := RenderHostsDeny(entries)
	// IPv4 单 IP 出现
	if !strings.Contains(out, "ALL: 203.0.113.5") {
		t.Fatal("IPv4 单 IP 应写入 hosts.deny")
	}
	if !strings.Contains(out, "ssh brute") {
		t.Fatal("应包含原因")
	}
	// CIDR 应被注释说明不支持
	if !strings.Contains(out, "# 10.0.0.0/8") {
		t.Fatal("CIDR 应注释并说明 hosts.deny 不支持")
	}
	// IPv6 应被注释跳过
	if !strings.Contains(out, "# 2001:db8::1") {
		t.Fatal("IPv6 应注释跳过")
	}
}
