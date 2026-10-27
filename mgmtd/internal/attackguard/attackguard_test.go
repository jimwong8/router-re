package attackguard

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	inputs []string
	cmds   []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	// delete of non-existent table errors; ignore already handled in Apply
	return "", nil
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.inputs = append(f.inputs, input)
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	return "", nil
}

func TestValidate(t *testing.T) {
	bad := []Config{
		{SYNFloodPPS: 0, ICMPFloodPPS: 10, ConnLimitPerIP: 10},
		{SYNFloodPPS: 10, ICMPFloodPPS: 0, ConnLimitPerIP: 10},
		{SYNFloodPPS: 10, ICMPFloodPPS: 10, ConnLimitPerIP: 0},
		{SYNFloodPPS: 1000001, ICMPFloodPPS: 10, ConnLimitPerIP: 10},
		{SYNFloodPPS: 10, ICMPFloodPPS: -5, ConnLimitPerIP: 10},
	}
	for _, c := range bad {
		if err := Validate(c); err == nil {
			t.Errorf("expected error for %+v", c)
		}
	}
	good := Config{SYNFloodPPS: 100, ICMPFloodPPS: 50, ConnLimitPerIP: 1000}
	if err := Validate(good); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestRenderNft(t *testing.T) {
	c := Config{SYNFloodPPS: 100, ICMPFloodPPS: 50, ConnLimitPerIP: 1000, Enabled: true}
	out := RenderNft(c)
	// multi-line chain declaration — must NOT contain "chain input {"
	if strings.Contains(out, "chain input { type") {
		t.Fatalf("chain must be multi-line, got:\n%s", out)
	}
	if !strings.Contains(out, "chain input {") {
		t.Fatalf("missing chain input:\n%s", out)
	}
	if strings.Contains(out, "chain input { type filter") {
		t.Fatalf("chain decl on single line:\n%s", out)
	}
	// limit rate lines
	if !strings.Contains(out, "limit rate 100/second burst 20 packets accept") {
		t.Errorf("missing SYN limit accept:\n%s", out)
	}
	if !strings.Contains(out, "tcp flags syn ct state new drop") {
		t.Errorf("missing SYN drop:\n%s", out)
	}
	if !strings.Contains(out, "limit rate 50/second burst 10 packets accept") {
		t.Errorf("missing ICMP limit accept:\n%s", out)
	}
	if !strings.Contains(out, "icmp type echo-request drop") {
		t.Errorf("missing ICMP drop:\n%s", out)
	}
	// conn limit
	if !strings.Contains(out, "ct state new limit rate over 1000/second drop") {
		t.Errorf("missing conn per-IP equivalent limit:\n%s", out)
	}
	// NEVER flush ruleset
	if strings.Contains(out, "flush ruleset") {
		t.Fatalf("must never flush ruleset:\n%s", out)
	}
}

func TestRenderNftEnabledFalse(t *testing.T) {
	// Enabled=false: still renders a table but the guard rules are commented
	// out so nothing is actively enforced.
	c := Config{SYNFloodPPS: 100, ICMPFloodPPS: 50, ConnLimitPerIP: 1000, Enabled: false}
	out := RenderNft(c)
	// we keep the numeric thresholds valid; with Enabled false every effectful
	// rule line must be a comment (prefixed by #) except chain decls.
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// allow table/chain declaration lines
		if strings.HasPrefix(trimmed, "table") || strings.HasPrefix(trimmed, "chain") ||
			strings.HasPrefix(trimmed, "type") || strings.HasPrefix(trimmed, "policy") || strings.HasPrefix(trimmed, "}") {
			continue
		}
		// everything else (actual rules) must be commented when disabled
		t.Errorf("rule active while disabled: %q", line)
	}
}

func TestApply(t *testing.T) {
	r := &fakeRunner{}
	c := Config{SYNFloodPPS: 100, ICMPFloodPPS: 50, ConnLimitPerIP: 1000, Enabled: true}
	if err := Apply(r, c); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	// delete table first, then run -f -
	if len(r.cmds) < 2 || !strings.Contains(r.cmds[0], "nft delete table inet router_guard") {
		t.Errorf("expected delete-table first, got: %v", r.cmds)
	}
	if !strings.Contains(r.cmds[1], "nft -f -") {
		t.Errorf("expected nft -f -, got: %v", r.cmds[1])
	}
	// input passed the nft program, no flush ruleset
	joined := strings.Join(r.inputs, "")
	if !strings.Contains(joined, "table inet router_guard") {
		t.Errorf("input missing table:\n%q", joined)
	}
	if strings.Contains(joined, "flush ruleset") {
		t.Errorf("input must not flush ruleset")
	}
}

// TestRenderNftNeverDefaultsToDrop 锁死一个会锁死机器的设计错误：
// attack guard 只负责限速，绝不能把 input 链设成 policy drop。
// 一旦 policy drop，除 SYN/ICMP 外的入站全被静默丢弃 —— SSH、管理界面、
// DNS 响应、已建立的 keep-alive 全部断掉。默认拒绝属于 firewall 模块。
func TestRenderNftNeverDefaultsToDrop(t *testing.T) {
	text := RenderNft(Config{SYNFloodPPS: 1000, ICMPFloodPPS: 200, ConnLimitPerIP: 100, Enabled: true})
	if strings.Contains(text, "policy drop") {
		t.Fatalf("input chain must not default to drop (locks out SSH/HTTP); got:\n%s", text)
	}
	if !strings.Contains(text, "policy accept") {
		t.Fatalf("input chain should be policy accept; got:\n%s", text)
	}
	if !strings.Contains(text, "ct state established,related accept") {
		t.Fatalf("must let established connections through so limiters don't kill long-lived sessions; got:\n%s", text)
	}
}
