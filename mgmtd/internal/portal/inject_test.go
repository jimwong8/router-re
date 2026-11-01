package portal

import (
	"strings"
	"testing"
)

// NormalizeMAC 只做小写+去空白，不做格式校验。带 "\n}" 的 MAC 能提前闭合
// elements = { ... } 集合并追加任意 nft 规则。
func TestRenderNftNoMacInjection(t *testing.T) {
	payloads := []string{
		"aa:bb:cc:dd:ee:ff\nPWNED",
		"aa:bb:cc:dd:ee:ff } chain evil {",
		"aa:bb:cc:dd:ee:ff, bb:cc:dd:ee:ff:00:11",
		"not-a-mac",
	}
	for _, p := range payloads {
		out := RenderNft([]Client{{MAC: p, IP: "10.0.0.5", Authorized: true}}, 8080)
		if strings.Contains(out, "PWNED") || strings.Contains(out, "chain evil") {
			t.Errorf("payload %q 注入成功:\n%s", p, out)
		}
	}
}

func TestRenderNftAcceptsLegit(t *testing.T) {
	out := RenderNft([]Client{
		{MAC: "AA:BB:CC:DD:EE:FF", IP: "10.0.0.5", Authorized: true},
		{MAC: "11:22:33:44:55:66", IP: "10.0.0.6", Authorized: true},
	}, 8080)
	for _, want := range []string{
		"elements = { aa:bb:cc:dd:ee:ff, 11:22:33:44:55:66 }",
		"ether saddr @authorized accept",
		"tcp dport 8080 accept",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}

// 未授权客户端不能进集合。
func TestRenderNftSkipsUnauthorized(t *testing.T) {
	out := RenderNft([]Client{{MAC: "aa:bb:cc:dd:ee:ff", Authorized: false}}, 8080)
	if strings.Contains(out, "aa:bb:cc:dd:ee:ff") {
		t.Fatalf("未授权 MAC 出现在集合里:\n%s", out)
	}
}
