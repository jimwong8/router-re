package ips

import (
	"strings"
	"testing"
)

// blocked 的 key 直接进 "elements = { ... }"。一个逗号或花括号就能改写
// 集合内容，甚至另开一张 nft 表。当前 handler 用 net.ParseIP 挡着，
// 但渲染层不能依赖调用方。
func TestRenderNftSkipsInvalidKeys(t *testing.T) {
	out := RenderNft(map[string]bool{
		"1.2.3.4": true,
		"1.2.3.5, 9.9.9.9":                    true,
		"1.2.3.6 }\n table inet evil {":       true,
		"not-an-ip":                           true,
		"2001:db8::1":                         true,
	})
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "table inet evil") {
			t.Fatalf("注入产生了额外的表:\n%s", out)
		}
		if strings.Contains(line, "9.9.9.9") {
			t.Fatalf("逗号注入被写进了 elements:\n%s", out)
		}
		if strings.Contains(line, "2001:db8") {
			t.Fatalf("IPv6 地址不应进 ipv4_addr 集合:\n%s", out)
		}
	}
	if !strings.Contains(out, "elements = { 1.2.3.4 }") {
		t.Errorf("合法 IP 应被保留:\n%s", out)
	}
}

func TestRenderNftEmptyWhenAllInvalid(t *testing.T) {
	out := RenderNft(map[string]bool{"bad": true, "1.2.3.4,5.6.7.8": true})
	if strings.Contains(out, "elements") {
		t.Errorf("全非法时不应输出 elements:\n%s", out)
	}
	// 结构必须仍然完整，nft 才能加载
	for _, want := range []string{"table inet router_ips", "set blackhole", "type ipv4_addr"} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}

func TestRenderNftAcceptsLegit(t *testing.T) {
	out := RenderNft(map[string]bool{"10.0.0.1": true, "192.0.2.7": true})
	if !strings.Contains(out, "elements = { 10.0.0.1, 192.0.2.7 }") {
		t.Errorf("合法集合渲染错误:\n%s", out)
	}
}
