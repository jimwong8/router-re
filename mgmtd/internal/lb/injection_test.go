package lb

import (
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// fakeRunner 记录被执行的 argv，不做真实调用。
type fakeRunner struct{ cmds [][]string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}

func baseConfig(m Member) Config {
	return Config{
		Mode:    "weighted",
		Members: []Member{m},
		Health: HealthCheck{Target: "1.1.1.1", IntervalSec: 5, TimeoutSec: 2,
			FailThreshold: 3, RiseThreshold: 2},
	}
}

// 这一组是真实漏洞的回归测试：Member.Name/Iface 曾被直接渲染进注释行，
// 而 ApplyIpRules 逐行 strings.Fields 后 exec argv[0] —— 换行注入
// "sh -c id>/tmp/pwn" 会在 root 下执行任意命令。
func TestValidateRejectsMemberNameInjection(t *testing.T) {
	for _, name := range []string{
		"wan1\nsh -c id",
		"wan1\nreboot",
		"wan 1",   // 空格会让 Fields 切成额外 argv
		"wan1\ttab",
		"wan1;id",
		"wan1`id`",
		"wan1$(id)",
		"",
		strings.Repeat("a", 65),
	} {
		if err := Validate(baseConfig(Member{Name: name, Iface: "eth0", Gateway: "1.1.1.1", Weight: 10, Enabled: true})); err == nil {
			t.Errorf("member name %q 未被拒绝", name)
		}
	}
}

func TestValidateRejectsIfaceInjection(t *testing.T) {
	for _, iface := range []string{
		"eth0\nsh -c id",
		"eth0 ; id",
		"eth 0",
		"eth0\t",
		strings.Repeat("a", 16),
		"",
	} {
		if err := Validate(baseConfig(Member{Name: "wan1", Iface: iface, Gateway: "1.1.1.1", Weight: 10, Enabled: true})); err == nil {
			t.Errorf("member iface %q 未被拒绝", iface)
		}
	}
}

// AcceptsChineseNames 保证加固没有误伤中文命名（常见：电信/联通/移动）。
func TestValidateAcceptsChineseNames(t *testing.T) {
	for _, name := range []string{"电信", "联通-1", "移动_2", "wan1", "WAN.2"} {
		if err := Validate(baseConfig(Member{Name: name, Iface: "eth0", Gateway: "1.1.1.1", Weight: 10, Enabled: true})); err != nil {
			t.Errorf("合法 name %q 被误拒: %v", name, err)
		}
	}
}

// 渲染层兜底：即使绕过 Validate，注入也不得多出一行可执行命令。
func TestRenderIpRuleScriptNoInjection(t *testing.T) {
	c := baseConfig(Member{Name: "wan1\nsh -c id>/tmp/pwn", Iface: "eth0", Gateway: "1.1.1.1", Weight: 10, Enabled: true})
	out := RenderIpRuleScript(c)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !ipRuleRe.MatchString(line) {
			t.Fatalf("渲染出非 ip rule 行: %q\n完整输出:\n%s", line, out)
		}
	}
}

// 执行侧 fail-closed：注入行即使出现在文本里，也绝不能被当作 argv[0] 执行。
func TestApplyIpRulesRefusesInjectedLine(t *testing.T) {
	f := &fakeRunner{}
	c := baseConfig(Member{Name: "wan1", Iface: "eth0", Gateway: "1.1.1.1", Weight: 10, Enabled: true})
	// 绕过 Validate 直接把注入串塞进 members，专测执行侧防线。
	c.Members[0].Name = "wan1\nsh -c id>/tmp/pwn"
	err := ApplyIpRules(f, c)
	if err != nil {
		// 拒绝执行也是正确行为
		return
	}
	for _, cmd := range f.cmds {
		if cmd[0] != "ip" {
			t.Fatalf("执行了非 ip 命令: %q", cmd)
		}
	}
}

func TestApplyIpRulesRunsLegit(t *testing.T) {
	f := &fakeRunner{}
	c := baseConfig(Member{Name: "wan1", Iface: "eth0", Gateway: "1.1.1.1", Weight: 10, Enabled: true})
	if err := ApplyIpRules(f, c); err != nil {
		t.Fatalf("合法配置执行失败: %v", err)
	}
	if len(f.cmds) != 1 || f.cmds[0][0] != "ip" {
		t.Fatalf("期望执行 1 条 ip 命令，实际 %q", f.cmds)
	}
	want := []string{"ip", "rule", "add", "fwmark"}
	for i, w := range want {
		if f.cmds[0][i] != w {
			t.Fatalf("argv 不符: %q", f.cmds[0])
		}
	}
}

var _ execx.Runner = (*fakeRunner)(nil)
