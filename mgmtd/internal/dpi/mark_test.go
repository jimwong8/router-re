package dpi

import (
	"strings"
	"testing"
)

// fakeRunner records Run / RunWithInput invocations in order.
type fakeRunner struct {
	calls []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return "", nil
}

func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	f.calls = append(f.calls, "input:"+name+" "+strings.Join(args, " "))
	return "", nil
}

func TestRenderMarkNft(t *testing.T) {
	rules := []MarkRule{
		{App: "netflix", Protocol: "tcp", DPorts: []int{443, 8080}, Mark: 10, Action: "mark"},
		{App: "dns", Protocol: "udp", DPorts: []int{53}, Mark: 11, Action: "mark"},
		{App: "p2p", Protocol: "tcp", DPorts: []int{6881}, Action: "drop"},
		{App: "game", Protocol: "udp", Mark: 12, Action: "limit"},
	}
	out, err := RenderMarkNft(rules)
	if err != nil {
		t.Fatalf("RenderMarkNft failed: %v", err)
	}
	if !strings.Contains(out, "table inet router_dpi") {
		t.Errorf("missing table header")
	}
	if !strings.Contains(out, "tcp dport { 443, 8080 }") {
		t.Errorf("missing tcp+ports match: %s", out)
	}
	// nftables rejects `meta l4proto tcp tcp dport {...}`; the protocol keyword
	// already implies the L4 match, so this must NOT appear.
	if strings.Contains(out, "meta l4proto tcp tcp dport") {
		t.Errorf("invalid nft syntax (duplicate L4 match): %s", out)
	}
	if !strings.Contains(out, "meta mark set 10") {
		t.Errorf("missing mark set: %s", out)
	}
	if !strings.Contains(out, "udp dport { 53 }") {
		t.Errorf("missing udp+ports match: %s", out)
	}
	if !strings.Contains(out, "drop") {
		t.Errorf("missing drop action: %s", out)
	}
	// no-DPorts rule matches by protocol only (limit → mark)
	if !strings.Contains(out, "meta l4proto udp") {
		t.Errorf("missing protocol-only match: %s", out)
	}
}

func TestRenderMarkNftInvalid(t *testing.T) {
	if _, err := RenderMarkNft([]MarkRule{{Protocol: "icmp", Action: "mark"}}); err == nil {
		t.Fatal("expected error for invalid protocol")
	}
	if _, err := RenderMarkNft([]MarkRule{{Protocol: "tcp", Action: "bogus"}}); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestApplyMarksDeleteBeforeFeed(t *testing.T) {
	fr := &fakeRunner{}
	rules := []MarkRule{{App: "a", Protocol: "tcp", DPorts: []int{80}, Mark: 1, Action: "mark"}}
	if err := ApplyMarks(fr, rules); err != nil {
		t.Fatalf("ApplyMarks failed: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(fr.calls), fr.calls)
	}
	// delete must come first, then feed via -f -
	if !strings.HasPrefix(fr.calls[0], "nft delete table inet router_dpi") {
		t.Errorf("call[0] should be delete table, got %q", fr.calls[0])
	}
	if !strings.HasPrefix(fr.calls[1], "input:nft -f -") {
		t.Errorf("call[1] should be nft -f -, got %q", fr.calls[1])
	}
}

func TestApplyMarksDeleteIgnoredOnMissing(t *testing.T) {
	fr := &fakeRunner{}
	// Even if delete fails (table missing), ApplyMarks still proceeds
	// and feeds the (empty) rule table via -f -.
	if err := ApplyMarks(fr, nil); err != nil {
		t.Fatalf("ApplyMarks(nil rules) failed: %v", err)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("expected 2 calls (delete + feed) for empty rules, got %v", fr.calls)
	}
	if !strings.HasPrefix(fr.calls[0], "nft delete table inet router_dpi") {
		t.Errorf("call[0] should be delete table, got %q", fr.calls[0])
	}
	if !strings.HasPrefix(fr.calls[1], "input:nft -f -") {
		t.Errorf("call[1] should be nft -f -, got %q", fr.calls[1])
	}
}

func TestMarkRulesForTC(t *testing.T) {
	rules := []MarkRule{
		{App: "a", Mark: 10, Action: "mark"},
		{App: "b", Mark: 11, Action: "mark"},
		{App: "drop-me", Action: "drop"},
	}
	cmds := MarkRulesForTC(rules)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 tc commands, got %d: %v", len(cmds), cmds)
	}
	if !strings.Contains(cmds[0], "handle 10 fw classid 1:1") {
		t.Errorf("mark 10 command wrong: %s", cmds[0])
	}
	if !strings.Contains(cmds[1], "handle 11 fw classid 1:2") {
		t.Errorf("mark 11 command wrong: %s", cmds[1])
	}
	for _, c := range cmds {
		if !strings.HasPrefix(c, "tc filter replace dev ") {
			t.Errorf("tc command should start with 'tc filter replace dev', got: %s", c)
		}
	}
}
