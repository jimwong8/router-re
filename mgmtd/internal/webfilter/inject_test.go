package webfilter

import (
	"strings"
	"testing"
)

// 域名会被原样拼进 dnsmasq 的 address=/<domain>/0.0.0.0。换行不是"空格或逗号"，
// 以前的校验放行，等于允许把任意域名劫持到攻击者指定的 IP。
func TestValidateRejectsControlChars(t *testing.T) {
	payloads := []string{
		"x\nserver=/evil.com/1.2.3.4",
		"x\r\nserver=/evil.com/1.2.3.4",
		"x\tY",
		"x\x0bY",
		"x\x0cY",
		"evil.com server=1.1.1.1",
		"evil.com;server=/a/1.1.1.1",
		"evil.com\x00",
	}
	for _, p := range payloads {
		if err := Validate([]Rule{{Domain: p, Action: "block", Enabled: true}}); err == nil {
			t.Errorf("payload %q 未被拒绝", p)
		}
	}
}

func TestRenderDnsmasqNoInjection(t *testing.T) {
	rules := []Rule{
		{Domain: "x\nserver=/evil.com/1.2.3.4", Action: "block", Enabled: true},
		{Domain: "ok.example.com", Action: "block", Enabled: true},
	}
	out := RenderDnsmasq(rules)
	if strings.Contains(out, "server=/evil.com/") {
		t.Fatalf("注入内容进入了 dnsmasq 配置:\n%s", out)
	}
	if !strings.Contains(out, "address=/ok.example.com/0.0.0.0") {
		t.Errorf("合法域名未渲染:\n%s", out)
	}
}

func TestValidateAcceptsLegit(t *testing.T) {
	ok := []string{
		"example.com", "*.ads.example.org", "a-b.c_d.example",
		"192.168.1.10", "2001:db8::1",
	}
	for _, d := range ok {
		if err := Validate([]Rule{{Domain: d, Action: "block", Enabled: true}}); err != nil {
			t.Errorf("合法值 %q 被误拒: %v", d, err)
		}
	}
}

func TestValidateRejectsBadAction(t *testing.T) {
	if err := Validate([]Rule{{Domain: "example.com", Action: "allow-all", Enabled: true}}); err == nil {
		t.Fatal("非法 action 应被拒绝")
	}
}
