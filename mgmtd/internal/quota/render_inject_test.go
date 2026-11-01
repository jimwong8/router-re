package quota

import (
	"strings"
	"testing"
)

// Target 会拼进 nft 表达式（ip saddr / ether saddr）和注释行。
// 渲染层必须自己再校验一次，不能假设调用方跑过 Validate。
func TestRenderNftNoInjection(t *testing.T) {
	cases := []Policy{
		{ID: "1", Name: "n", Target: "10.0.0.1\nPWNED", TargetType: TargetIP, RateMbit: 10, BurstKB: 100, Enabled: true},
		{ID: "2", Name: "n\nPWNED", Target: "10.0.0.1", TargetType: TargetIP, RateMbit: 10, BurstKB: 100, Enabled: true},
		{ID: "3", Name: "n", Target: "aa:bb:cc:dd:ee:ff\nPWNED", TargetType: TargetMAC, RateMbit: 10, Enabled: true},
		{ID: "4", Name: "n", Target: "10.0.0.0/8\nPWNED", TargetType: TargetCIDR, RateMbit: 10, Enabled: true},
	}
	for _, p := range cases {
		out := RenderNft([]Policy{p})
		// 只判"是否新开了一行"：单行内的 PWNED 仍在注释里，无害。
		if strings.Contains(out, "\nPWNED") || strings.Contains(out, "\n		PWNED") {
			t.Errorf("policy %s 注入成功:\n%s", p.ID, out)
		}
		// 生效行必须都形如 ip saddr / ether saddr / # 注释
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.Contains(trimmed, "PWNED") {
				t.Errorf("policy %s 的 PWNED 出现在生效行: %q", p.ID, trimmed)
			}
		}
	}
}

func TestRenderReportNoInjection(t *testing.T) {
	p := Policy{ID: "1", Name: "n", Target: "10.0.0.1\nPWNED", TargetType: TargetIP, RateMbit: 10, BurstKB: 100, Enabled: true}
	out := RenderReport([]Policy{p})
	if strings.Contains(out, "\nPWNED") {
		t.Fatalf("报告被注入:\n%s", out)
	}
}

func TestRenderNftAcceptsLegit(t *testing.T) {
	p := Policy{ID: "1", Name: "限速", Target: "10.0.0.5", TargetType: TargetIP,
		RateMbit: 10, BurstKB: 100, Enabled: true}
	out := RenderNft([]Policy{p})
	if !strings.Contains(out, "ip saddr 10.0.0.5") {
		t.Errorf("合法 IP 目标未渲染:\n%s", out)
	}
	if !strings.Contains(out, "table inet router_quota") {
		t.Errorf("缺少表声明:\n%s", out)
	}

	m := Policy{ID: "2", Name: "mac", Target: "aa:bb:cc:dd:ee:ff", TargetType: TargetMAC,
		RateMbit: 5, Enabled: true}
	out2 := RenderNft([]Policy{m})
	if !strings.Contains(out2, "ether saddr aa:bb:cc:dd:ee:ff") {
		t.Errorf("合法 MAC 目标未渲染:\n%s", out2)
	}
}

// app 维度的 Target 不进入表达式，但也必须压成单行。
func TestRenderNftAppTarget(t *testing.T) {
	p := Policy{ID: "1", Name: "app", Target: "app\nPWNED", TargetType: TargetApp, App: "bilibili",
		RateMbit: 10, Enabled: true}
	out := RenderNft([]Policy{p})
	if strings.Contains(out, "\nPWNED") {
		t.Fatalf("app 目标注入成功:\n%s", out)
	}
}
