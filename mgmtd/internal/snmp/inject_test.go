package snmp

import (
	"strings"
	"testing"
)

// prefix("# ") 只作用于整段字符串的第一行：Location/Contact 带换行时，
// 后面的行会脱离注释直接生效——即使在"禁用"状态下。
// 最危险的形态是注入 rwcommunity，给自己开一个 SNMP 读写 community。
func TestRenderSnmpdNoInjection(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"location", Config{Location: "机房\nrwcommunity evil"}},
		{"contact", Config{Contact: "ops\nrwcommunity evil"}},
		{"community", Config{Communities: []Community{{Name: "public\nrwcommunity evil", Access: "ro"}}}},
		{"trap", Config{Communities: []Community{{Name: "public", Access: "ro"}},
			TrapTargets: []string{"1.2.3.4\ntrapsink evil"}}},
	}
	for _, c := range cases {
		out := RenderSnmpd(c.cfg)
		for _, line := range strings.Split(out, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.Contains(trimmed, "evil") {
				t.Errorf("%s: 注入逃出注释成为生效行: %q\n完整输出:\n%s", c.name, line, out)
			}
		}
	}
}

func TestValidateRejectsControlChars(t *testing.T) {
	bad := []Config{
		{Location: "机房\nrwcommunity evil"},
		{Location: "机房\r\nrwcommunity evil"},
		{Contact: "ops\nrwcommunity evil"},
	}
	for i, c := range bad[:3] {
		if err := Validate(c); err == nil {
			t.Errorf("用例 %d 未被拒绝: %+v", i, c)
		}
	}
}

func TestValidateRejectsCommunityControlChars(t *testing.T) {
	for _, name := range []string{"pub\nlic", "pub\rlic", "pub\tlic", "pub lic"} {
		c := Config{Communities: []Community{{Name: name, Access: "ro"}}}
		if err := Validate(c); err == nil {
			t.Errorf("community name %q 未被拒绝", name)
		}
	}
}

func TestRenderSnmpdAcceptsLegit(t *testing.T) {
	c := Config{
		Enabled:  true,
		Location: "Fangchenggang DC",
		Contact:  "ops@example.com",
		Communities: []Community{
			{Name: "public", Access: "ro"},
			{Name: "private", Access: "rw"},
		},
		TrapTargets: []string{"10.0.0.9"},
	}
	if err := Validate(c); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := RenderSnmpd(c)
	for _, want := range []string{
		"agentAddress udp:161",
		"sysLocation Fangchenggang DC",
		"sysContact ops@example.com",
		"rocommunity public",
		"rwcommunity private",
		"trapsink 10.0.0.9",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("缺少 %q:\n%s", want, out)
		}
	}
	if strings.HasPrefix(out, "# ") {
		t.Errorf("Enabled=true 时不应加注释前缀:\n%s", out)
	}
}

// 禁用的配置整段都应被注释掉。
func TestRenderSnmpdDisabledIsCommented(t *testing.T) {
	out := RenderSnmpd(Config{Location: "DC", Communities: []Community{{Name: "public", Access: "ro"}}})
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "# ") {
			t.Errorf("禁用状态下有成活行: %q", line)
		}
	}
}
