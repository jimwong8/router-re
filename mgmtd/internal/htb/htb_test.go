package htb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// --- stub Runner ---

type stubRunner struct {
	calls  [][]string
	tcOut  string
	failAt int
}

func (s *stubRunner) Run(name string, args ...string) (string, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	if s.failAt > 0 && len(s.calls) == s.failAt {
		return "", fmt.Errorf("boom: simulated failure")
	}
	if name == "tc" && len(args) > 1 && args[1] == "class" {
		return s.tcOut, nil
	}
	if name == "tc" && len(args) > 1 && args[0] == "class" && args[1] == "show" {
		return s.tcOut, nil
	}
	return "", nil
}

func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.Run(name, args...)
}

var _ execx.Runner = (*stubRunner)(nil)

func validConfig() Config {
	return Config{
		Iface:        "eth0",
		RootRateMbit: 100,
		Enabled:      true,
		Classes: []Class{
			{Name: "voip", RateMbit: 10, CeilMbit: 20, Priority: 1, MatchPorts: []int{5060}},
			{Name: "web", RateMbit: 40, CeilMbit: 100, Priority: 2, MatchPorts: []int{80, 443}},
		},
	}
}

// --- ValidateIface ---

func TestValidateIfaceRejectsLoopback(t *testing.T) {
	if err := ValidateIface("lo"); err == nil {
		t.Fatal("ValidateIface(lo) must fail: tc on lo throttles local traffic and locks out ssh/curl")
	}
}

func TestValidateIfaceRejectsEmpty(t *testing.T) {
	if err := ValidateIface(""); err == nil {
		t.Fatal("ValidateIface(\"\") must fail")
	}
}

func TestValidateIfaceRejectsWhitespace(t *testing.T) {
	for _, bad := range []string{"eth 0", "eth\t0", "eth\n0"} {
		if err := ValidateIface(bad); err == nil {
			t.Errorf("ValidateIface(%q) must fail", bad)
		}
	}
}

func TestValidateIfaceAcceptsNormal(t *testing.T) {
	for _, ok := range []string{"eth0", "wan", "br-lan", "ppp0"} {
		if err := ValidateIface(ok); err != nil {
			t.Errorf("ValidateIface(%q) unexpected error: %v", ok, err)
		}
	}
}

// --- Validate ---

func TestValidateRejectsCeilBelowRate(t *testing.T) {
	c := validConfig()
	c.Classes[0].RateMbit = 10
	c.Classes[0].CeilMbit = 5
	if err := Validate(c); err == nil {
		t.Fatal("ceil_mbit < rate_mbit must fail (ceil < rate is meaningless)")
	}
}

func TestValidateRejectsRateAboveRoot(t *testing.T) {
	c := validConfig()
	c.Classes[0].RateMbit = 200
	if err := Validate(c); err == nil {
		t.Fatal("rate_mbit > root_rate_mbit must fail")
	}
}

func TestValidateRejectsDuplicateClass(t *testing.T) {
	c := validConfig()
	c.Classes = append(c.Classes, Class{Name: "voip", RateMbit: 5, CeilMbit: 5, Priority: 3})
	if err := Validate(c); err == nil {
		t.Fatal("duplicate class name must fail")
	}
}

func TestValidateRejectsBadPriority(t *testing.T) {
	c := validConfig()
	c.Classes[0].Priority = 8
	if err := Validate(c); err == nil {
		t.Fatal("priority 8 must fail (1-7)")
	}
	c = validConfig()
	c.Classes[0].Priority = 0
	if err := Validate(c); err == nil {
		t.Fatal("priority 0 must fail (1-7)")
	}
}

func TestValidateRejectsBadPort(t *testing.T) {
	c := validConfig()
	c.Classes[0].MatchPorts = []int{0}
	if err := Validate(c); err == nil {
		t.Fatal("port 0 must fail")
	}
	c = validConfig()
	c.Classes[0].MatchPorts = []int{65536}
	if err := Validate(c); err == nil {
		t.Fatal("port 65536 must fail")
	}
}

func TestValidateRejectsBadDstIP(t *testing.T) {
	c := validConfig()
	c.Classes[0].MatchDstIPs = []string{"not-an-ip"}
	if err := Validate(c); err == nil {
		t.Fatal("invalid match dst ip must fail")
	}
}

func TestValidateRejectsLoopback(t *testing.T) {
	c := validConfig()
	c.Iface = "lo"
	if err := Validate(c); err == nil {
		t.Fatal("Validate with iface=lo must fail")
	}
}

func TestValidateRejectsZeroRoot(t *testing.T) {
	c := validConfig()
	c.RootRateMbit = 0
	if err := Validate(c); err == nil {
		t.Fatal("root_rate_mbit=0 must fail")
	}
}

func TestValidateRejectsUnknownDefaultClass(t *testing.T) {
	c := validConfig()
	c.DefaultClass = "nope"
	if err := Validate(c); err == nil {
		t.Fatal("default_class not in classes must fail")
	}
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	c := validConfig()
	c.DefaultClass = "web"
	c.Classes[0].MatchDstIPs = []string{"10.0.0.0/24", "192.168.1.5"}
	if err := Validate(c); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

// --- RenderScript ---

func TestRenderScriptDeletesOldQdiscFirst(t *testing.T) {
	out := RenderScript(validConfig())
	lines := strings.Split(out, "\n")
	// 找到第一条真正执行的命令（跳过空行和注释）
	firstCmd := ""
	for _, l := range lines {
		tr := strings.TrimSpace(l)
		if tr == "" || strings.HasPrefix(tr, "#") {
			continue
		}
		firstCmd = tr
		break
	}
	if !strings.Contains(firstCmd, "tc qdisc del") {
		t.Fatalf("first command must be 'tc qdisc del' (idempotency: delete old root qdisc before add); got %q", firstCmd)
	}
}

func TestRenderScriptRootQdiscDefault10(t *testing.T) {
	out := RenderScript(validConfig())
	if !strings.Contains(out, "tc qdisc add dev eth0 root handle 1: htb default 10") {
		t.Fatalf("missing root qdisc add with default 10:\n%s", out)
	}
	if !strings.Contains(out, "tc class add dev eth0 parent 1: classid 1:1 htb rate 100mbit") {
		t.Fatalf("missing root class 1:1:\n%s", out)
	}
}

func TestRenderScriptClassIDsStartAt10(t *testing.T) {
	out := RenderScript(validConfig())
	// 第一个叶子类必须是 classid 1:10
	if !strings.Contains(out, "classid 1:10 htb rate 10mbit ceil 20mbit prio 1") {
		t.Fatalf("first leaf class must be classid 1:10 with expected params:\n%s", out)
	}
	if !strings.Contains(out, "classid 1:11 htb rate 40mbit ceil 100mbit prio 2") {
		t.Fatalf("second leaf class must be classid 1:11:\n%s", out)
	}
}

func TestRenderScriptNoRootConflictsWith101(t *testing.T) {
	out := RenderScript(validConfig())
	// 根类 1:1 只出现一次（class add），叶子类从 1:10 开始，绝不冲突
	if strings.Count(out, "classid 1:1 ") != 1 {
		t.Fatalf("root class 1:1 must appear exactly once:\n%s", out)
	}
	if strings.Contains(out, "classid 1:2 ") {
		t.Fatalf("leaf classes must start at 1:10, never 1:2:\n%s", out)
	}
}

func TestRenderScriptFilters(t *testing.T) {
	out := RenderScript(validConfig())
	if !strings.Contains(out, "match ip dport 5060 0xffff flowid 1:10") {
		t.Fatalf("missing dport filter for voip port 5060:\n%s", out)
	}
	// 默认类 10 的 classid 是 1:10；当配置里给第一个类设置了 MatchDstIPs 时也要分流
	if !strings.Contains(out, "flowid 1:10") || !strings.Contains(out, "flowid 1:11") {
		t.Fatalf("filters must reference leaf classids 1:10 and 1:11:\n%s", out)
	}
}

func TestRenderScriptDstIPMask(t *testing.T) {
	c := validConfig()
	c.Classes[0].MatchDstIPs = []string{"10.0.0.0/24", "192.168.1.5"}
	out := RenderScript(c)
	if !strings.Contains(out, "match ip dst 10.0.0.0/24 flowid 1:10") {
		t.Fatalf("CIDR dst must be rendered unchanged:\n%s", out)
	}
	if !strings.Contains(out, "match ip dst 192.168.1.5/32 flowid 1:10") {
		t.Fatalf("bare IP dst must be masked /32:\n%s", out)
	}
}

func TestRenderScriptHasNoRawFlush(t *testing.T) {
	out := RenderScript(validConfig())
	if strings.Contains(out, "flush ruleset") || strings.Contains(out, "qdisc flush") {
		t.Fatal("render must never emit a flush")
	}
}

// --- Apply ---

func TestApplyRejectsLoopback(t *testing.T) {
	r := &stubRunner{}
	c := validConfig()
	c.Iface = "lo"
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

func TestApplyRejectsLoopbackEvenIfEnabled(t *testing.T) {
	r := &stubRunner{}
	c := validConfig()
	c.Iface = "lo"
	c.Enabled = true
	applied, detail := Apply(r, c)
	if applied {
		t.Fatal("Apply on lo must fail even when enabled")
	}
	if len(r.calls) != 0 {
		t.Fatalf("must execute zero commands on lo, got %d", len(r.calls))
	}
	_ = detail
}

func TestApplySuccess(t *testing.T) {
	r := &stubRunner{}
	r.tcOut = "class htb 1:1 root\nclass htb 1:10 leaf\nclass htb 1:11 leaf\n"
	applied, detail := Apply(r, validConfig())
	if !applied {
		t.Fatalf("valid apply should succeed, got detail=%q", detail)
	}
	// 断言命令序列包含 del → add → class 1:1 → class 1:10
	sawDel, sawAdd, sawReadback, sawClass10 := false, false, false, false
	for _, c := range r.calls {
		if c[0] == "tc" && len(c) > 1 && c[1] == "qdisc" && len(c) > 2 && c[2] == "del" {
			sawDel = true
		}
		if c[0] == "tc" && len(c) > 1 && c[1] == "qdisc" && len(c) > 2 && c[2] == "add" {
			sawAdd = true
		}
		if c[0] == "tc" && len(c) > 1 && c[1] == "class" && len(c) > 2 && c[2] == "show" {
			sawReadback = true
		}
		if c[0] == "tc" && len(c) > 1 && c[1] == "class" && len(c) > 2 && c[2] == "add" {
			for _, a := range c {
				if a == "1:10" {
					sawClass10 = true
				}
			}
		}
	}
	if !sawDel || !sawAdd || !sawReadback || !sawClass10 {
		t.Fatalf("expected del+add+readback + class 1:10, got del=%v add=%v readback=%v class10=%v", sawDel, sawAdd, sawReadback, sawClass10)
	}
}

func TestApplyFailsWhenCommandFails(t *testing.T) {
	// 第 3 条命令（根 class add）注入失败
	r := &stubRunner{failAt: 3}
	applied, detail := Apply(r, validConfig())
	if applied {
		t.Fatal("expected failure when a tc command fails")
	}
	if !strings.Contains(detail, "command failed") {
		t.Fatalf("detail must mention the failed command, got %q", detail)
	}
	if !strings.Contains(detail, "tc class add") {
		t.Fatalf("detail must quote the failing command text, got %q", detail)
	}
}

func TestApplyFailsOnReadbackMismatch(t *testing.T) {
	// 回读只报 1 条 class，期望 3 条（2 classes + 1 root）
	r := &stubRunner{tcOut: "class htb 1:1 root\n"}
	applied, detail := Apply(r, validConfig())
	if applied {
		t.Fatal("expected failure on readback mismatch")
	}
	if !strings.Contains(detail, "readback mismatch") {
		t.Fatalf("detail must mention readback mismatch, got %q", detail)
	}
	if !strings.Contains(detail, "want 3") {
		t.Fatalf("detail must state the expected count, got %q", detail)
	}
}

func TestApplyDisabledConfigDoesNothing(t *testing.T) {
	r := &stubRunner{}
	c := validConfig()
	c.Enabled = false
	applied, detail := Apply(r, c)
	if applied {
		t.Fatal("disabled config must not apply")
	}
	if len(r.calls) != 0 {
		t.Fatalf("disabled config must execute zero commands, got %d", len(r.calls))
	}
	if !strings.Contains(detail, "disabled") {
		t.Fatalf("detail should mention disabled, got %q", detail)
	}
	_ = detail
}

// --- Cleanup / Show ---

func TestCleanupRejectsLoopback(t *testing.T) {
	r := &stubRunner{}
	if err := Cleanup(r, "lo"); err == nil {
		t.Fatal("Cleanup on lo must fail")
	}
	if len(r.calls) != 0 {
		t.Fatalf("Cleanup on lo must execute zero commands, got %d", len(r.calls))
	}
}

func TestCleanupToleratesMissingQdisc(t *testing.T) {
	// tc qdisc del 在无 qdisc 时报 "Cannot find device" —— 不算错误
	rf := &failRunner{err: "Cannot find device"}
	if err := Cleanup(rf, "eth0"); err != nil {
		t.Fatalf("Cleanup should tolerate missing qdisc, got %v", err)
	}
}

func TestCleanupRejectsWhitespaceIface(t *testing.T) {
	r := &stubRunner{}
	if err := Cleanup(r, "eth 0"); err == nil {
		t.Fatal("Cleanup on iface with whitespace must fail")
	}
	if len(r.calls) != 0 {
		t.Fatalf("must not execute commands on invalid iface, got %d", len(r.calls))
	}
}

func TestShowRejectsLoopback(t *testing.T) {
	r := &stubRunner{}
	if _, err := Show(r, "lo"); err == nil {
		t.Fatal("Show on lo must fail")
	}
	if len(r.calls) != 0 {
		t.Fatalf("Show on lo must execute zero commands, got %d", len(r.calls))
	}
}

// failRunner 总是返回固定错误。
type failRunner struct{ err string }

func (f *failRunner) Run(name string, args ...string) (string, error) {
	return "", fmt.Errorf("%s", f.err)
}

func (f *failRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return "", fmt.Errorf("%s", f.err)
}

var _ execx.Runner = (*failRunner)(nil)
