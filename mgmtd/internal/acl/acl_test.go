package acl

import (
	"strings"
	"testing"
)

// fakeRunner records RunWithInput calls and returns canned output.
type fakeRunner struct {
	inputs  []string
	out     string
	err     error
	runHook func(name string, args ...string) (string, error) // optional override
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	if f.runHook != nil {
		return f.runHook(name, args...)
	}
	return f.out, f.err
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.inputs = append(f.inputs, input)
	if f.runHook != nil {
		return f.runHook(name, args...)
	}
	return f.out, f.err
}

func TestRenderNftOmitsEmptyFields(t *testing.T) {
	rs := []Rule{
		{Proto: "tcp", DstPort: 80, Action: "accept", Enabled: true},
		{Proto: "any", DstIP: "", Action: "drop", Enabled: true},
	}
	out := RenderNft(rs)
	// Empty src/dst must not produce dangling matchers.
	if strings.Contains(out, "ip saddr") || strings.Contains(out, "ip daddr") {
		t.Fatalf("should omit empty src/dst matchers, got:\n%s", out)
	}
	// proto=any should be omitted (no bare 'any ' token).
	if strings.Contains(out, " any ") {
		t.Fatal("proto 'any' should be omitted as a match token")
	}
}

func TestRenderNftMultilineChain(t *testing.T) {
	out := RenderNft([]Rule{{Proto: "tcp", DstPort: 22, Action: "accept", Enabled: true}})
	if !strings.Contains(out, "chain forward {\n") {
		t.Fatal("chain must be declared multiline: 'chain forward {' on its own line")
	}
	if strings.Contains(out, "chain forward { type") {
		t.Fatal("chain declaration must NOT be single-line 'chain forward { type ...'")
	}
}

func TestRenderNftSafetyAccept(t *testing.T) {
	out := RenderNft(nil)
	if !strings.Contains(out, "        accept\n") {
		t.Fatal("empty ruleset must end with an unconditional accept safety net")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{"ok tcp", Rule{Proto: "tcp", DstPort: 80, Action: "accept"}, false},
		{"ok udp", Rule{Proto: "udp", DstPort: 53, Action: "drop"}, false},
		{"ok icmp", Rule{Proto: "icmp", Action: "reject"}, false},
		{"bad proto", Rule{Proto: "icmp45", Action: "accept"}, true},
		{"bad action", Rule{Proto: "tcp", Action: "forward"}, true},
		{"port over", Rule{Proto: "tcp", DstPort: 70000, Action: "accept"}, true},
		{"bad src ip", Rule{SrcIP: "999.999.999.999", Proto: "tcp", Action: "accept"}, true},
		{"ok cidr src", Rule{SrcIP: "10.0.0.0/8", Proto: "tcp", Action: "accept"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate([]Rule{c.rule})
			if c.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestApplyValidatesThenRuns(t *testing.T) {
	r := &fakeRunner{}
	// Invalid rule must short-circuit: no nft invocation.
	if err := Apply(r, []Rule{{Proto: "bad", Action: "accept"}}); err == nil {
		t.Fatal("expected validation error")
	}
	if len(r.inputs) != 0 {
		t.Fatalf("expected no nft runs on validation failure, got %d", len(r.inputs))
	}

	// Valid rules: delete + apply (2 stdin runs).
	r.inputs = nil
	r.out = "ok"
	if err := Apply(r, []Rule{{Proto: "tcp", DstPort: 80, Action: "accept", Enabled: true}}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if len(r.inputs) != 2 {
		t.Fatalf("expected 2 nft stdin runs (delete+apply), got %d", len(r.inputs))
	}
	if !strings.Contains(r.inputs[1], "table inet router_acl") {
		t.Fatalf("apply stdin should contain the ruleset, got:\n%s", r.inputs[1])
	}
}

func TestRenderRuleFull(t *testing.T) {
	r := Rule{SrcIP: "10.0.0.0/8", DstIP: "192.168.1.1", DstPort: 443, Proto: "tcp", Action: "accept", Enabled: true}
	got := renderRule(r)
	want := "ip saddr 10.0.0.0/8 ip daddr 192.168.1.1 tcp dport 443 accept"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestRenderNftNeverFlushesWholeRuleset 锁死第二次踩到的同一类 bug：
// flush ruleset 会清空整机所有 nft 表（firewall/dpi/portal/portmap），
// 不只是 router_acl。另外空集合字面量 elements = {} 本身是语法错误。
func TestRenderNftNeverFlushesWholeRuleset(t *testing.T) {
	text := RenderNft([]Rule{{ID: 1, Name: "x", Proto: "tcp", DstPort: 80, Action: "accept", Enabled: true}})
	if strings.Contains(text, "flush ruleset") {
		t.Fatalf("RenderNft must not emit 'flush ruleset'; got:\n%s", text)
	}
	if strings.Contains(text, "elements = {}") {
		t.Fatalf("empty set literal is nft syntax error; got:\n%s", text)
	}
}
