package ddns

import (
	"strings"
	"testing"
)

// timesyncd.conf 里换行就是语句分隔符。服务器名带 "\n" 就能凭空插入一条
// 指令（例如把 RootDistanceMaxSec 放到失去意义的值）。
func TestRenderTimesyncdNoInjection(t *testing.T) {
	payloads := []string{
		"ntp.aliyun.com\nRootDistanceMaxSec=999999",
		"ntp.aliyun.com\r\nFallbackNTP=evil.com",
		"ntp.aliyun.com RootDistanceMaxSec=999999",
		"ntp.aliyun.com\n[Time]\nNTP=evil.com",
	}
	for _, p := range payloads {
		out := RenderTimesyncd(NTPConfig{Enabled: true, Servers: []string{p, "time.cloudflare.com"}})
		for _, bad := range []string{"RootDistanceMaxSec", "FallbackNTP", "NTP=evil.com"} {
			if strings.Contains(out, bad) {
				t.Errorf("payload %q 注入成功（%q）:\n%s", p, bad, out)
			}
		}
		// 合法的那一条必须保留，非法的那条必须消失
		if !strings.Contains(out, "time.cloudflare.com") {
			t.Errorf("合法服务器被误删:\n%s", out)
		}
	}
}

func TestValidateNTPConfigRejectsInjection(t *testing.T) {
	bad := [][]string{
		{"x\nRootDistanceMaxSec=999999"},
		{"x\r\nFallbackNTP=evil"},
		{"x y"},
		{"x\ty"},
		{""},
		{strings.Repeat("a", 254)},
	}
	for _, servers := range bad {
		if err := ValidateNTPConfig(NTPConfig{Servers: servers}); err == nil {
			t.Errorf("非法服务器列表未被拒绝: %q", servers)
		}
	}
}

func TestValidateNTPConfigAcceptsLegit(t *testing.T) {
	ok := []NTPConfig{
		{Servers: []string{"ntp.aliyun.com", "time.cloudflare.com"}},
		{Servers: []string{"192.168.1.1"}},
		{Servers: []string{"2001:db8::1"}},
		{Servers: []string{"[2001:db8::1]"}},
		{Servers: nil},
	}
	for _, cfg := range ok {
		if err := ValidateNTPConfig(cfg); err != nil {
			t.Errorf("合法配置被误拒 %v: %v", cfg.Servers, err)
		}
	}
}

func TestValidateNTPConfigTooMany(t *testing.T) {
	servers := make([]string, 17)
	for i := range servers {
		servers[i] = "ntp.example.com"
	}
	if err := ValidateNTPConfig(NTPConfig{Servers: servers}); err == nil {
		t.Fatal("超过 16 个服务器应被拒绝")
	}
}

// 合法配置必须照常渲染成空格分隔的一行。
func TestRenderTimesyncdLegit(t *testing.T) {
	out := RenderTimesyncd(NTPConfig{Enabled: true, Servers: []string{"ntp.aliyun.com", "time.cloudflare.com"}})
	want := "[Time]\nNTP=ntp.aliyun.com time.cloudflare.com\n"
	if out != want {
		t.Fatalf("渲染结果不符\nwant: %q\ngot:  %q", want, out)
	}
}
