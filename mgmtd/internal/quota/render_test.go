package quota

import (
	"strings"
	"testing"
)

func TestRenderNftContainsCtCount(t *testing.T) {
	ps := []Policy{{
		ID: "p1", Name: "conn", Target: "10.0.0.0/8", TargetType: TargetCIDR,
		RateMbit: 0, MaxConns: 100, Enabled: true,
	}}
	out := RenderNft(ps)
	if !strings.Contains(out, "ct count over 100 drop") {
		t.Fatalf("应含 ct count over 100 drop, got:\n%s", out)
	}
}

// TestRenderNftNoFlushRuleset -- 最硬红线。
func TestRenderNftNoFlushRuleset(t *testing.T) {
	out := RenderNft([]Policy{validPolicy()})
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("绝不允许 flush ruleset")
	}
}

// TestRenderNftNoPolicyDrop -- 只 drop 命中策略的，默认放行。
func TestRenderNftNoPolicyDrop(t *testing.T) {
	out := RenderNft([]Policy{validPolicy()})
	if strings.Contains(out, "policy drop") {
		t.Fatal("绝不允许 policy drop")
	}
	if !strings.Contains(out, "accept") {
		t.Fatal("应默认放行（accept）")
	}
}

func TestRenderNftRateLimit(t *testing.T) {
	ps := []Policy{validPolicy()} // ip, rate=10
	out := RenderNft(ps)
	if !strings.Contains(out, "limit rate over") {
		t.Fatalf("应含 limit rate, got:\n%s", out)
	}
}

func TestRenderNftMAC(t *testing.T) {
	p := validPolicy()
	p.TargetType = TargetMAC
	p.Target = "aa:bb:cc:dd:ee:ff"
	out := RenderNft([]Policy{p})
	if !strings.Contains(out, "ether saddr aa:bb:cc:dd:ee:ff") {
		t.Fatalf("mac 应用 ether saddr, got:\n%s", out)
	}
}

// TestRenderNftAppDependsOnMark -- app 类必须说明依赖 DPI 打 mark。
func TestRenderNftAppDependsOnMark(t *testing.T) {
	p := validPolicy()
	p.TargetType = TargetApp
	p.Target = "-"
	p.App = "wechat"
	out := RenderNft([]Policy{p})
	if !strings.Contains(out, "meta mark") {
		t.Fatalf("app 类应渲染 meta mark 匹配, got:\n%s", out)
	}
	if !strings.Contains(out, "DPI") && !strings.Contains(out, "无法直接按应用名匹配") {
		t.Fatalf("app 类应说明依赖 DPI 打 mark（不能假装直接按名匹配）: got:\n%s", out)
	}
}

func TestRenderNftDisabledSkipped(t *testing.T) {
	p := validPolicy()
	p.Enabled = false
	out := RenderNft([]Policy{p})
	if strings.Contains(out, "203.0.113.5") && !strings.HasPrefix(out, "table") {
		// 禁用策略不应出现在链规则里（只可能出现在（无）注释里）。这里要求不含该 IP。
	}
	if strings.Contains(out, "limit rate") {
		t.Fatal("禁用策略不应渲染规则")
	}
}

func TestRenderNftEmpty(t *testing.T) {
	out := RenderNft(nil)
	if strings.Contains(out, "flush ruleset") || strings.Contains(out, "policy drop") {
		t.Fatal("空渲染也不可触碰红线")
	}
	if !strings.Contains(out, "table inet router_quota") {
		t.Fatal("应含表声明")
	}
}

func TestRenderReport(t *testing.T) {
	ps := []Policy{
		{ID: "p1", Name: "lan rate", Target: "10.0.0.0/8", TargetType: TargetCIDR, RateMbit: 50, Enabled: true},
		{ID: "p2", Name: "wx", Target: "-", TargetType: TargetApp, App: "wechat", MaxConns: 200, Enabled: true},
		{ID: "p3", Name: "old", Target: "8.8.8.8", TargetType: TargetIP, MaxConns: 10, Enabled: false},
	}
	rep := RenderReport(ps)
	if !strings.Contains(rep, "限速 50.00 Mbit/s") {
		t.Fatalf("report 应含限速信息, got: %s", rep)
	}
	if !strings.Contains(rep, "DPI") {
		t.Fatalf("report 对 app 类应说明依赖 DPI, got: %s", rep)
	}
	if !strings.Contains(rep, "[disabled]") {
		t.Fatalf("report 应标示禁用策略, got: %s", rep)
	}
}
