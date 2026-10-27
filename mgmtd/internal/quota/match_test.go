package quota

import (
	"testing"
)

func TestMatchIPTarget(t *testing.T) {
	p := Policy{TargetType: TargetIP, Target: "203.0.113.5", Enabled: true}
	if !Match(p, ConnCtx{SrcIP: "203.0.113.5"}) {
		t.Fatal("ip 精确应命中")
	}
	if Match(p, ConnCtx{SrcIP: "203.0.113.6"}) {
		t.Fatal("非封禁 ip 不应命中")
	}
	// 空 src 不命中
	if Match(p, ConnCtx{}) {
		t.Fatal("空 src_ip 不应命中")
	}
}

func TestMatchCIDRTarget(t *testing.T) {
	p := Policy{TargetType: TargetCIDR, Target: "10.0.0.0/8", Enabled: true}
	if !Match(p, ConnCtx{SrcIP: "10.0.0.5"}) {
		t.Fatal("10.0.0.5 应在 10.0.0.0/8 内")
	}
	if !Match(p, ConnCtx{SrcIP: "10.5.5.5"}) {
		t.Fatal("10.5.5.5 应在 /8 内")
	}
	if Match(p, ConnCtx{SrcIP: "11.0.0.1"}) {
		t.Fatal("11.0.0.1 不应命中 /8")
	}
	// cidr 边界包含
	p2 := Policy{TargetType: TargetCIDR, Target: "192.168.1.0/24", Enabled: true}
	if !Match(p2, ConnCtx{SrcIP: "192.168.1.50"}) {
		t.Fatal("192.168.1.50 应在 /24 内")
	}
	if Match(p2, ConnCtx{SrcIP: "192.168.2.1"}) {
		t.Fatal("192.168.2.1 不应命中 /24")
	}
	// 非法 src 不命中
	if Match(p, ConnCtx{SrcIP: "not-an-ip"}) {
		t.Fatal("非法 src 不应命中")
	}
}

func TestMatchMACTarget(t *testing.T) {
	p := Policy{TargetType: TargetMAC, Target: "AA:BB:CC:DD:EE:FF", Enabled: true}
	// 大小写不敏感
	if !Match(p, ConnCtx{SrcMAC: "aa:bb:cc:dd:ee:ff"}) {
		t.Fatal("mac 应大小写不敏感命中")
	}
	if !Match(p, ConnCtx{SrcMAC: "AA:BB:CC:DD:EE:FF"}) {
		t.Fatal("mac 全大写应命中")
	}
	if Match(p, ConnCtx{SrcMAC: "AA:BB:CC:DD:EE:00"}) {
		t.Fatal("不同 mac 不应命中")
	}
}

func TestMatchAppTarget(t *testing.T) {
	p := Policy{TargetType: TargetApp, App: "wechat", Enabled: true}
	if !Match(p, ConnCtx{App: "wechat"}) {
		t.Fatal("app 等值应命中")
	}
	if Match(p, ConnCtx{App: "tiktok"}) {
		t.Fatal("不同 app 不应命中")
	}
}

func TestMatchDisabledNeverMatches(t *testing.T) {
	p := Policy{TargetType: TargetIP, Target: "203.0.113.5", Enabled: false}
	if Match(p, ConnCtx{SrcIP: "203.0.113.5"}) {
		t.Fatal("禁用策略不应命中")
	}
}

// FirstMatch 只返回启用的、按顺序第一个命中的。
func TestFirstMatch(t *testing.T) {
	ps := []Policy{
		{ID: "p1", TargetType: TargetIP, Target: "203.0.113.5", Enabled: false},
		{ID: "p2", TargetType: TargetCIDR, Target: "10.0.0.0/8", Enabled: true},
		{ID: "p3", TargetType: TargetCIDR, Target: "10.0.0.0/16", Enabled: true},
	}
	// 10.0.0.5 命中 p2（p1 禁用跳过），即使 p3 也在列表中——顺序优先。
	got := FirstMatch(ps, ConnCtx{SrcIP: "10.0.0.5"})
	if got == nil || got.ID != "p2" {
		t.Fatalf("应按顺序返回第一个启用命中 p2, got %+v", got)
	}
	// 完全不命中
	if FirstMatch(ps, ConnCtx{SrcIP: "8.8.8.8"}) != nil {
		t.Fatal("无关地址应返回 nil")
	}
}

func TestFirstMatchDisabledSkipped(t *testing.T) {
	ps := []Policy{
		{ID: "p1", TargetType: TargetIP, Target: "8.8.8.8", Enabled: false},
		{ID: "p2", TargetType: TargetIP, Target: "8.8.8.8", Enabled: false},
	}
	if FirstMatch(ps, ConnCtx{SrcIP: "8.8.8.8"}) != nil {
		t.Fatal("全部禁用时应返回 nil")
	}
}
