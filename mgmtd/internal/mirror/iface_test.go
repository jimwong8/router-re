package mirror

import (
	"strings"
	"testing"
)

// 接口名字段原先只查非空。当前 Apply 走 exec.Command 的 argv（不经 shell），
// 所以这不是命令注入；但渲染出的 tc 文本会返回给前端、也可能被人工复制到
// shell 执行，非法字符应当尽早失败。
func TestValidateRejectsIfaceInjection(t *testing.T) {
	for _, iface := range []string{
		"eth0 ; id",
		"eth0\n tc qdisc add dev eth1 root",
		"eth0|cat /etc/passwd",
		"eth0$(id)",
		"eth 0",
		strings.Repeat("a", 16),
	} {
		c := Config{SourceIface: iface, TargetIface: "eth1", Direction: "ingress", Enabled: true}
		if err := Validate(c); err == nil {
			t.Errorf("SourceIface=%q 未被拒绝", iface)
		}
		c2 := Config{SourceIface: "eth0", TargetIface: iface, Direction: "ingress", Enabled: true}
		if err := Validate(c2); err == nil {
			t.Errorf("TargetIface=%q 未被拒绝", iface)
		}
	}
}

// 渲染层兜底：即使绕过 Validate，接口名里的换行也不得多出一条 tc 命令。
func TestRenderScriptNoInjection(t *testing.T) {
	c := Config{SourceIface: "eth0\n tc qdisc add dev eth9 root",
		TargetIface: "eth1", Direction: "ingress", Enabled: true}
	out := RenderScript(c)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "tc qdisc add dev eth9") {
			t.Fatalf("接口名注入产生了额外的 tc 命令:\n%s", out)
		}
	}
}

func TestValidateAcceptsLegit(t *testing.T) {
	c := Config{SourceIface: "eth0", TargetIface: "eth1.100", Direction: "both", Enabled: true}
	if err := Validate(c); err != nil {
		t.Fatalf("合法配置被误拒: %v", err)
	}
	out := RenderScript(c)
	if !strings.Contains(out, "mirror dev eth1.100") {
		t.Errorf("缺少镜像目标:\n%s", out)
	}
}

func TestValidateStillRejectsLoopback(t *testing.T) {
	c := Config{SourceIface: "lo", TargetIface: "eth1", Direction: "ingress", Enabled: true}
	if err := Validate(c); err == nil {
		t.Error("lo 作为源必须被拒绝")
	}
}
