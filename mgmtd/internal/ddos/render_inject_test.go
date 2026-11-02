package ddos

import (
	"strings"
	"testing"
)

// 白名单直接进 "elements = { ... }"。Validate 已有 IP/CIDR 校验，
// 这个测试锁住渲染层的独立兜底 —— 将来新增入口漏了 Validate 也不至于注入。
func TestRenderSkipsInvalidWhitelist(t *testing.T) {
	c := Config{
		Whitelist: []string{
			"10.0.0.1",
			"10.0.0.0/24",
			"1.2.3.4 } chain evil { ip saddr 0.0.0.0/0 accept }",
			"1.2.3.4, 9.9.9.9",
			"",
		},
		PortScanWindow: 10,
	}
	out := Render(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "chain evil") {
			t.Fatalf("白名单注入产生了额外链:\n%s", out)
		}
		if strings.Contains(line, "9.9.9.9") {
			t.Fatalf("逗号注入被写进 elements:\n%s", out)
		}
	}
	if !strings.Contains(out, "10.0.0.1") || !strings.Contains(out, "10.0.0.0/24") {
		t.Errorf("合法条目被丢弃:\n%s", out)
	}
}

func TestValidateRejectsWhitelistInjection(t *testing.T) {
	for _, w := range []string{
		"1.2.3.4 } chain evil {",
		"1.2.3.4,9.9.9.9",
		"not-an-ip",
		"0.0.0.0/0",
	} {
		c := Config{Whitelist: []string{w}, PortScanWindow: 10, BlockSeconds: 600}
		if err := Validate(c); err == nil {
			t.Errorf("白名单 %q 未被拒绝", w)
		}
	}
}
