package firewall

import (
	"strings"
	"testing"
)

// 这些 payload 曾经能被原样写进 nft 脚本。渲染层现在直接丢弃非法字段，
// 所以正确结果是"输出里不出现注入的指令"，而不是"返回 error"。
func TestRenderNftablesNoInjection(t *testing.T) {
	const evil = "x\nflush ruleset"
	cases := []struct {
		name string
		rule Rule
	}{
		{"dst", Rule{Chain: "forward", Src: "10.0.0.0/8", Dst: evil, Proto: "tcp", DPort: "22", Action: "accept"}},
		{"src", Rule{Chain: "forward", Src: evil, Proto: "tcp", DPort: "22", Action: "accept"}},
		{"dport", Rule{Chain: "forward", Proto: "tcp", DPort: evil, Action: "accept"}},
		{"chain", Rule{Chain: "forward\nflush ruleset", Proto: "tcp", DPort: "22", Action: "accept"}},
		{"action", Rule{Chain: "forward", Proto: "tcp", DPort: "22", Action: "accept\nflush ruleset"}},
		{"proto", Rule{Chain: "forward", Proto: "tcp\nflush ruleset", DPort: "22", Action: "accept"}},
	}
	for _, c := range cases {
		out := RenderNftables([]Rule{c.rule})
		if strings.Contains(out, "flush ruleset") {
			t.Errorf("%s: 注入内容进入了渲染结果:\n%s", c.name, out)
		}
	}
}

// 合法规则必须照常渲染，白名单不能误伤。
func TestRenderNftablesAcceptsLegitRules(t *testing.T) {
	out := RenderNftables([]Rule{
		{Chain: "forward", Src: "10.0.0.0/8", Dst: "192.168.1.0/24", Proto: "tcp", DPort: "22", Action: "accept"},
		{Chain: "input", Proto: "udp", DPort: "53", Action: "accept"},
		{Chain: "forward", Proto: "icmp", Action: "drop"},
		{Chain: "forward", Src: "::1/128", Proto: "tcp", DPort: "8000-8100", Action: "accept"},
	})
	for _, want := range []string{
		"ip saddr 10.0.0.0/8 ip daddr 192.168.1.0/24 tcp dport 22 accept",
		"udp dport 53 accept",
		"icmp drop",
		"ip saddr ::1/128 tcp dport 8000-8100 accept",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("合法规则未正确渲染，期望包含 %q\n实际:\n%s", want, out)
		}
	}
}

// Validate 的字段级白名单：这些必须在调用 nft 之前就被拒。
func TestValidateRejectsMalformedFields(t *testing.T) {
	bad := []Rule{
		{Chain: "forward", Dst: "1.2.3.4; flush ruleset", Action: "accept"},
		{Chain: "forward; drop", DPort: "22", Action: "accept"},
		{Chain: "forward", Proto: "tcp", DPort: "99999", Action: "accept"},
		{Chain: "forward", Proto: "tcp", DPort: "22-", Action: "accept"},
		{Chain: "forward", Proto: "evil", Action: "accept"},
		{Chain: "forward", Action: "jump evil"},
		{Chain: "forward", Dst: "not-an-ip", Action: "accept"},
	}
	for i, r := range bad {
		if err := validateFields([]Rule{r}); err == nil {
			t.Errorf("第 %d 条非法规则未被拒绝: %+v", i, r)
		}
	}
}

func TestValidateFieldsAcceptsLegit(t *testing.T) {
	ok := []Rule{
		{Chain: "forward", Src: "192.168.0.0/16", Dst: "8.8.8.8", Proto: "tcp", DPort: "443", Action: "accept"},
		{Chain: "input", Proto: "icmp", Action: "drop"},
		{Chain: "my_chain", Proto: "udp", DPort: "1-65535", Action: "return"},
		{Proto: "tcp", DPort: "22", Action: "reject"},
	}
	if err := validateFields(ok); err != nil {
		t.Fatalf("合法规则被误拒: %v", err)
	}
}

// 规则条数上限，防止一次请求让渲染与 nft 解析爆炸。
func TestValidateFieldsRejectsTooManyRules(t *testing.T) {
	rules := make([]Rule, 1001)
	for i := range rules {
		rules[i] = Rule{Proto: "tcp", DPort: "22", Action: "accept"}
	}
	if err := validateFields(rules); err == nil {
		t.Fatal("超过上限的规则数应被拒绝")
	}
}
