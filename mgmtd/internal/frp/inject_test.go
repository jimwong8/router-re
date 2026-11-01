package frp

import (
	"strings"
	"testing"
)

// frpc 配置是 INI：段头是 [name]，值直接拼接。任何字段带换行或 "]" 都能
// 闭合当前节并伪造新的隧道——最危险的形态是把本机 22 端口暴露到公网。
func TestRenameCannotInjectSection(t *testing.T) {
	c := Config{
		ServerAddr: "1.2.3.4", ServerPort: 7000,
		Tunnels: []Tunnel{{
			Name: "evil]\n[admin_ssh]\ntype = tcp\nlocal_ip = 127.0.0.1\nlocal_port=22\nremote_port=2222\n[x",
			LocalPort: 80, RemotePort: 8080, Proto: "tcp",
		}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("带 ] 和换行的隧道名应被 Validate 拒绝")
	}
	out := RenderConfig(c)
	for _, bad := range []string{"[admin_ssh]", "local_port=22", "remote_port=2222"} {
		if strings.Contains(out, bad) {
			t.Fatalf("注入的隧道进入了渲染结果（%q）:\n%s", bad, out)
		}
	}
}

func TestServerAddrCannotInjectDirective(t *testing.T) {
	c := Config{
		ServerAddr: "1.2.3.4\nadmin_addr = 0.0.0.0", ServerPort: 7000,
		Tunnels: []Tunnel{{Name: "t1", LocalPort: 80, RemotePort: 8080, Proto: "tcp"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("带换行的 server_addr 应被拒绝")
	}
	if out := RenderConfig(c); strings.Contains(out, "admin_addr") {
		t.Fatalf("server_addr 注入进入了渲染结果:\n%s", out)
	}
}

func TestLocalIPCannotInjectDirective(t *testing.T) {
	c := Config{
		ServerAddr: "1.2.3.4", ServerPort: 7000,
		Tunnels: []Tunnel{{Name: "t1", LocalIP: "127.0.0.1\nlocal_port = 22", LocalPort: 80, RemotePort: 8080, Proto: "tcp"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("带换行的 local_ip 应被拒绝")
	}
	if out := RenderConfig(c); strings.Contains(out, "local_port = 22") {
		t.Fatalf("local_ip 注入进入了渲染结果:\n%s", out)
	}
}

func TestTokenNewlineRejected(t *testing.T) {
	c := Config{
		ServerAddr: "1.2.3.4", ServerPort: 7000, Token: "abc\nadmin_addr = 0.0.0.0",
		Tunnels: []Tunnel{{Name: "t1", LocalPort: 80, RemotePort: 8080, Proto: "tcp"}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("带换行的 token 应被拒绝")
	}
	if out := RenderConfig(c); strings.Contains(out, "admin_addr") {
		t.Fatalf("token 注入进入了渲染结果:\n%s", out)
	}
}

// 合法配置必须照常渲染——白名单不能误伤隧道名里的 . _ -。
func TestRenderConfigAcceptsLegit(t *testing.T) {
	c := Config{
		ServerAddr: "frp.example.com", ServerPort: 7000, Token: "abc123",
		Tunnels: []Tunnel{
			{Name: "web-1", LocalIP: "192.168.1.10", LocalPort: 80, RemotePort: 8080, Proto: "tcp"},
			{Name: "dns.udp", LocalPort: 53, RemotePort: 5353, Type: "udp"},
			{Name: "v6", LocalIP: "2001:db8::1", LocalPort: 443, RemotePort: 8443, Proto: "tcp"},
		},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := RenderConfig(c)
	for _, want := range []string{
		"server_addr = frp.example.com",
		"server_port = 7000",
		"token = abc123",
		"[web-1]",
		"local_ip = 192.168.1.10",
		"[dns.udp]",
		"type = udp",
		"local_ip = 2001:db8::1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("合法配置未正确渲染，期望包含 %q\n实际:\n%s", want, out)
		}
	}
}
