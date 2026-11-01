package acl

import (
	"strings"
	"testing"
)

// MAC 以前是裸字符串拼接。带换行就能追加任意 nft 参数/规则。
// 渲染层必须用 ParseMAC 规整，解析失败的直接丢弃。
func TestRenderNftNoMacInjection(t *testing.T) {
	cases := []Rule{
		{ID: 1, Proto: "tcp", DstPort: 22, Action: "accept", SrcMAC: "aa:bb:cc:dd:ee:ff\nPWNED", Enabled: true},
		{ID: 2, Proto: "tcp", DstPort: 22, Action: "accept", DstMAC: "aa:bb:cc:dd:ee:01\nPWNED", Enabled: true},
		{ID: 3, Proto: "tcp", DstPort: 22, Action: "accept", SrcMAC: "aa:bb:cc:dd:ee:ff accept", Enabled: true},
	}
	for _, r := range cases {
		out := RenderNft([]Rule{r})
		if strings.Contains(out, "PWNED") {
			t.Errorf("rule %d MAC 注入成功:\n%s", r.ID, out)
		}
	}
}

// IP 解析失败以前会渲染成字面 "<nil>"，让整份 nft 脚本因语法错误加载失败
// ——面板显示下发成功，实际一条都没生效。
func TestRenderNftNoNilIP(t *testing.T) {
	r := Rule{ID: 1, Proto: "tcp", DstPort: 22, Action: "accept", SrcIP: "not-an-ip", Enabled: true}
	out := RenderNft([]Rule{r})
	if strings.Contains(out, "<nil>") {
		t.Fatalf("非法 IP 渲染成了 <nil>:\n%s", out)
	}
}

func TestRenderNftAcceptsLegit(t *testing.T) {
	r := Rule{
		ID: 1, Name: "ssh", SrcIP: "10.0.0.0/8", DstIP: "192.168.1.5",
		SrcMAC: "AA:BB:CC:DD:EE:FF", Proto: "tcp", DstPort: 22, Action: "accept", Enabled: true,
	}
	out := RenderNft([]Rule{r})
	for _, want := range []string{"ip saddr 10.0.0.0/8", "ip daddr 192.168.1.5", "ether saddr aa:bb:cc:dd:ee:ff", "dport 22"} {
		if !strings.Contains(out, want) {
			t.Errorf("合法规则渲染缺少 %q:\n%s", want, out)
		}
	}
	// 必须保留 self-lock 保护
	if !strings.Contains(out, "accept") {
		t.Errorf("缺少 accept 兜底:\n%s", out)
	}
}
