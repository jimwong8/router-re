package pppoe

import (
	"strings"
	"testing"
)

func baseConfig() Config {
	return Config{
		Interface: "eth1", LocalIP: "10.0.0.1",
		PoolStart: "10.0.0.10", PoolEnd: "10.0.0.20",
		ServiceName: "svc", DNS: []string{"8.8.8.8"},
	}
}

// Interface / ServiceName 被拼进注释行 "# PPPoE server options for %s (service: %s)"。
// 注释只保护到行尾，一个换行就能让后面的内容变成生效指令（如 noauth）。
func TestValidateRejectsInterfaceInjection(t *testing.T) {
	payloads := []string{
		"eth1\nms-dns 6.6.6.6",
		"eth1\nnoauth",
		"eth1\r\nnoauth",
		"eth 1",
		"eth1\"",
		strings.Repeat("a", 16),
	}
	for _, p := range payloads {
		c := baseConfig()
		c.Interface = p
		if err := c.Validate(); err == nil {
			t.Errorf("Interface=%q 未被拒绝", p)
		}
	}
}

func TestValidateRejectsServiceNameInjection(t *testing.T) {
	for _, p := range []string{"svc\nnoauth", "svc\r\nnoauth", "svc\"", "svc\\x", "svc\x00"} {
		c := baseConfig()
		c.ServiceName = p
		if err := c.Validate(); err == nil {
			t.Errorf("ServiceName=%q 未被拒绝", p)
		}
	}
}

func TestRenderOptionsNoInjection(t *testing.T) {
	c := baseConfig()
	c.Interface = "eth1\nms-dns 6.6.6.6"
	c.ServiceName = "svc\nnoauth"
	// Validate 会拒绝，这里直接验证渲染层兜底：注释里的换行不得产出生效行
	out, err := RenderOptions(c)
	if err != nil {
		// 走到校验分支也算修好，但渲染层仍需独立成立
		out = renderOptionsUnchecked(c)
	}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "noauth") {
			t.Fatalf("ServiceName 注入出了 noauth 指令行:\n%s", out)
		}
		if trimmed == "ms-dns 6.6.6.6" {
			t.Fatalf("Interface 注入出了 ms-dns 行:\n%s", out)
		}
	}
}

// renderOptionsUnchecked 复刻渲染逻辑，用于在没有校验层时验证兜底。
func renderOptionsUnchecked(cfg Config) string {
	iface, svc := oneLine(cfg.Interface), oneLine(cfg.ServiceName)
	out := "# PPPoE server options for " + iface + " (service: " + svc + ")\n"
	out += "# -I " + iface + " -L " + cfg.LocalIP + "\n"
	out += "require-chap\nlogin\n"
	for _, d := range cfg.DNS {
		if d != "" {
			out += "ms-dns " + d + "\n"
		}
	}
	return out
}

// chap-secrets 每条记录是 `"client" server "secret" "IP"`。
// 引号能闭合字段、新开一条记录 —— 等于凭空加一个已知密码的拨号账号。
func TestUserValidateRejectsChapsecretsInjection(t *testing.T) {
	cases := []User{
		{Name: `u" * "p" "*"`},
		{Name: "u\nbackdoor"},
		{Name: "u\tbackdoor"},
		{Name: "u", Password: `p" * "*" "*"`},
		{Name: "u", Password: "p\nbackdoor"},
		{Name: "u", Password: "p x"},
		{Name: "u", IP: "10.0.0.1\" *"},
		{Name: ""},
	}
	for i, u := range cases {
		if err := u.Validate(); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, u)
		}
	}
}

func TestUserValidateAcceptsLegit(t *testing.T) {
	cases := []User{
		{Name: "alice", Password: "s3cret", IP: "10.0.0.11"},
		{Name: "bob-01", Password: "p@ssw0rd!"},
		{Name: "carol", Password: "x", IP: "*"},
	}
	for i, u := range cases {
		if err := u.Validate(); err != nil {
			t.Errorf("用例 %d 被误拒: %v (%+v)", i, err, u)
		}
	}
}

// 渲染层兜底：即使调用方跳过 Validate，也必须保证每条用户只产出
// 一条记录、且记录里正好 4 个引号字段。
func TestRenderChapSecretsNoInjection(t *testing.T) {
	users := []User{
		{Name: `u" * "p" "*"` + "\nbackdoor", Password: `p"x`, IP: "10.0.0.11"},
		{Name: "u2\n" + `"backdoor" * "pass" "*"`, Password: "p2"},
	}
	out := RenderChapSecrets(users)
	var records []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		records = append(records, line)
	}
	if len(records) != len(users) {
		t.Fatalf("记录数应为 %d，实际 %d:\n%s", len(users), len(records), out)
	}
	for _, r := range records {
		// 格式是 "client" * "secret" "IP"：3 个引号字段 + 1 个裸 *，共 6 个引号
		if n := strings.Count(r, `"`); n != 6 {
			t.Errorf("每条记录应有 3 个引号字段（6 个引号），实际 %d: %q", n, r)
		}
		if !strings.Contains(r, `" * "`) {
			t.Errorf("记录结构应为 \"client\" * \"secret\" \"ip\"，实际 %q", r)
		}
	}
}

func TestRenderChapSecretsAcceptsLegit(t *testing.T) {
	out := RenderChapSecrets([]User{{Name: "alice", Password: "s3cret", IP: "10.0.0.11"}})
	want := "\"alice\" * \"s3cret\" \"10.0.0.11\""
	if !strings.Contains(out, want) {
		t.Errorf("缺少 %q:\n%s", want, out)
	}
}
