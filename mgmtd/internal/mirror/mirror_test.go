package mirror

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// stubRunner 记录调用并可按第 N 次注入失败。
type stubRunner struct {
	calls  [][]string
	failAt int
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	if s.failAt > 0 && len(s.calls) == s.failAt {
		return "", fmt.Errorf("boom")
	}
	return "", nil
}

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.Run(name, args...)
}

var _ execx.Runner = (*stubRunner)(nil)

func validConfig() Config {
	return Config{
		SourceIface: "eth0",
		TargetIface: "eth1",
		Direction:   "both",
		Enabled:     true,
	}
}

func TestValidateRejectsLoopback(t *testing.T) {
	c := validConfig()
	c.SourceIface = "lo"
	if err := Validate(c); err == nil {
		t.Fatal("source=lo must fail")
	}
	c = validConfig()
	c.TargetIface = "lo"
	if err := Validate(c); err == nil {
		t.Fatal("target=lo must fail")
	}
}

func TestValidateRejectsSameIface(t *testing.T) {
	c := validConfig()
	c.TargetIface = "eth0"
	if err := Validate(c); err == nil {
		t.Fatal("source == target must fail")
	}
}

func TestValidateRejectsBadDirection(t *testing.T) {
	c := validConfig()
	c.Direction = "sideways"
	if err := Validate(c); err == nil {
		t.Fatal("direction 'sideways' must fail")
	}
}

func TestValidateAcceptsValid(t *testing.T) {
	for _, dir := range []string{"ingress", "egress", "both"} {
		c := validConfig()
		c.Direction = dir
		if err := Validate(c); err != nil {
			t.Errorf("valid direction %q rejected: %v", dir, err)
		}
	}
}

func TestRenderScriptIngress(t *testing.T) {
	c := validConfig()
	c.Direction = "ingress"
	out := RenderScript(c)
	if !strings.Contains(out, "parent ffff:") {
		t.Fatalf("ingress filter must use parent ffff:, got:\n%s", out)
	}
	if !strings.Contains(out, "mirred egress mirror dev eth1") {
		t.Fatalf("ingress filter must mirror to eth1, got:\n%s", out)
	}
}

func TestRenderScriptDisabled(t *testing.T) {
	c := validConfig()
	c.Enabled = false
	if out := RenderScript(c); out != "" {
		t.Fatalf("disabled config must render empty, got:\n%s", out)
	}
}

func TestApplyRejectsLoopbackWithNoCommands(t *testing.T) {
	r := &stubRunner{}
	c := validConfig()
	c.SourceIface = "lo"
	applied, detail := Apply(r, c)
	if applied {
		t.Fatal("Apply on lo must fail")
	}
	if len(r.calls) != 0 {
		t.Fatalf("Apply on lo must execute zero commands, got %d: %v", len(r.calls), r.calls)
	}
	if !strings.Contains(strings.ToLower(detail), "loopback") {
		t.Fatalf("detail must explain loopback risk, got %q", detail)
	}
}

// TestApplyStripsShellSyntax 锁死一个实际踩到的 bug：
// RenderScript 生成的是给人看的 shell 行（末尾 `2>/dev/null || true`），
// 但 Apply 走 exec.Command 的 argv，不经 shell。直接 Fields 拆分会把
// "2>/dev/null"、"||"、"true" 当参数传给 tc，tc 报错又被 qdisc del 分支吞掉，
// 结果旧 qdisc 实际没删 → 下次 qdisc add 报 File exists，第二次应用必然失败。
func TestApplyStripsShellSyntax(t *testing.T) {
	r := &stubRunner{}
	applied, detail := Apply(r, Config{SourceIface: "eth1", TargetIface: "eth2", Direction: "both", Enabled: true})
	if !applied {
		t.Fatalf("合法配置应 applied=true, got: %s", detail)
	}
	for _, call := range r.calls {
		for _, a := range call {
			if strings.Contains(a, "/dev/null") || a == "||" || a == "true" {
				t.Fatalf("shell 语法泄漏进 tc 的 argv: %q（整条: %v）", a, call)
			}
		}
	}
	// del 必须真的执行（幂等重建的前提）
	var del bool
	for _, call := range r.calls {
		if len(call) >= 3 && call[1] == "qdisc" && call[2] == "del" {
			del = true
		}
	}
	if !del {
		t.Fatalf("必须真的执行 tc qdisc del, 实际调用: %v", r.calls)
	}
}
