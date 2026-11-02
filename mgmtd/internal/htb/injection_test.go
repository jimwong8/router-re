package htb

import (
	"strings"
	"testing"
)

type fakeRunner struct{ cmds [][]string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return "", nil
}

func cfgWithClass(name string) Config {
	return Config{
		Iface:        "eth0",
		RootRateMbit: 100,
		Enabled:      true,
		Classes: []Class{{
			Name: name, RateMbit: 10, CeilMbit: 20, Priority: 1,
		}},
	}
}

// 真实漏洞回归：Class.Name 曾被直接渲染进注释行
//   "# 叶子类 <name>：保证 ..."
// 而 Apply 把渲染文本逐行 strings.Fields 后 exec argv[0]。
// 名字里一个换行就能注入 "sh -c id>/tmp/pwn" —— root 下任意命令执行。
func TestValidateRejectsClassNameInjection(t *testing.T) {
	for _, name := range []string{
		"guest\nsh -c id",
		"guest\nreboot",
		"guest class", // 空格 → Fields 切成额外 argv
		"guest\tx",
		"guest;id",
		"guest`id`",
		"guest$(id)",
		"",
		strings.Repeat("类", 65),
	} {
		if err := Validate(cfgWithClass(name)); err == nil {
			t.Errorf("class name %q 未被拒绝", name)
		}
	}
}

func TestValidateAcceptsChineseClassNames(t *testing.T) {
	for _, name := range []string{"办公", "guest-1", "web_2", "video.3"} {
		if err := Validate(cfgWithClass(name)); err != nil {
			t.Errorf("合法类名 %q 被误拒: %v", name, err)
		}
	}
}

// 渲染层兜底：注释行不得被换行拆出可执行行。
func TestRenderScriptNoInjection(t *testing.T) {
	out := RenderScript(cfgWithClass("guest\nsh -c id>/tmp/pwn"))
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !classLineRe.MatchString(line) {
			t.Fatalf("渲染出非 tc 行: %q\n完整输出:\n%s", line, out)
		}
	}
}

// 执行侧 fail-closed：绕过 Validate 时，注入行也绝不能被当成程序执行。
func TestApplyRefusesInjectedLine(t *testing.T) {
	f := &fakeRunner{}
	c := cfgWithClass("guest\nsh -c id>/tmp/pwn ")
	ok, detail := Apply(f, c)
	if ok {
		t.Fatalf("注入配置不应报告成功")
	}
	for _, cmd := range f.cmds {
		if cmd[0] != "tc" {
			t.Fatalf("执行了非 tc 命令: %q（detail=%s）", cmd, detail)
		}
	}
}

func TestApplyRunsLegit(t *testing.T) {
	f := &fakeRunner{}
	c := cfgWithClass("guest")
	// 回读会失败（fakeRunner 返回空），但执行序列必须全是 tc。
	_, _ = Apply(f, c)
	if len(f.cmds) == 0 {
		t.Fatal("没有执行任何命令")
	}
	for _, cmd := range f.cmds {
		if cmd[0] != "tc" {
			t.Fatalf("出现非 tc 命令: %q", cmd)
		}
	}
}
