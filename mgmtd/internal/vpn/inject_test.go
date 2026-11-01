package vpn

import (
	"strings"
	"testing"
)

// chap-secrets 每行是 "用户名 服务器 密码 IP"。用户名/密码里带换行就能
// 凭空多出一个账号——等于在 VPN 上开一条后门。
func TestRenderChapSecretsNoInjection(t *testing.T) {
	users := []VPNUser{
		{Username: "u1\nbackdoor", Password: "p1"},
		{Username: "u2", Password: "p\nbackdoor"},
		{Username: "u3", Password: "p3", IP: "10.0.0.9\nbackdoor"},
		{Username: "u4\tx", Password: "p4"},
	}
	out := RenderChapSecrets(users)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var account int
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		account++
		// 每条账号必须正好 4 个制表符分隔字段（注入若成功会多出字段/多行）
		if got := len(strings.Split(line, "	")); got != 4 {
			t.Errorf("字段数应为 4，实际 %d: %q", got, line)
		}
	}
	if account != len(users) {
		t.Errorf("账号行数应为 %d，实际 %d:\n%s", len(users), account, out)
	}
}

// LeftID/RightID 只查长度不查内容，会被原样写进 ipsec.conf。
func TestValidateL2TPRejectsIPSecIDInjection(t *testing.T) {
	base := L2TPConfig{
		ServerIP: "10.0.0.1", AddrPool: "192.168.100.10-192.168.100.20",
		IPSecSecret: "s3cret123", ForceIPSec: 1,
	}
	for _, id := range []string{"l\nrightsubnet=0.0.0.0/0", "l\r\nx", "l\tx", "l\x00"} {
		c := base
		c.LeftID = id
		if err := ValidateL2TP(c); err == nil {
			t.Errorf("leftid=%q 未被拒绝", id)
		}
		c2 := base
		c2.RightID = id
		if err := ValidateL2TP(c2); err == nil {
			t.Errorf("rightid=%q 未被拒绝", id)
		}
	}
}

func TestRenderIPSecConfNoInjection(t *testing.T) {
	c := L2TPConfig{
		ServerIP: "10.0.0.1", AddrPool: "192.168.100.10-192.168.100.20",
		IPSecSecret: "s3cret123", ForceIPSec: 1,
		LeftID: "l\nrightsubnet=0.0.0.0/0",
	}
	out := RenderIPSecConf(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "rightsubnet=0.0.0.0/0") {
			t.Fatalf("注入产生了独立 rightsubnet 行:\n%s", out)
		}
	}
	if !strings.Contains(out, "leftid=l rightsubnet=0.0.0.0/0") {
		t.Errorf("leftid 未被压成单行:\n%s", out)
	}
}

// ms-dns 只接受 IP 字面量。
func TestRenderXl2tpdOptionsNoInjection(t *testing.T) {
	c := L2TPConfig{
		ServerIP: "10.0.0.1", AddrPool: "192.168.100.10-192.168.100.20",
		IPSecSecret: "s3cret123", ForceIPSec: 1,
		DNS1: "8.8.8.8\nrequire-mppe",
	}
	out := RenderXl2tpdOptions(c)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require-mppe") {
			t.Fatalf("DNS 注入产生了独立选项行:\n%s", out)
		}
	}
}

func TestRenderXl2tpdOptionsAcceptsLegit(t *testing.T) {
	c := L2TPConfig{
		ServerIP: "10.0.0.1", AddrPool: "192.168.100.10-192.168.100.20",
		IPSecSecret: "s3cret123", ForceIPSec: 1,
		DNS1: "8.8.8.8", DNS2: "1.1.1.1", MTU: 1400, MRU: 1400,
	}
	out := RenderXl2tpdOptions(c)
	for _, want := range []string{"ms-dns 8.8.8.8", "ms-dns 1.1.1.1", "mtu 1400", "mru 1400"} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
}
