package portal

import (
	"strings"
	"testing"
)

func TestRenderNftContainsAuthorizedSetAndRules(t *testing.T) {
	clients := []Client{
		{MAC: "aa:bb:cc:dd:ee:ff", Authorized: true},
		{MAC: "11:22:33:44:55:66", Authorized: false},
	}
	text := RenderNft(clients, 8080)
	if !strings.Contains(text, "table inet portal {") {
		t.Fatal("expected table declaration")
	}
	if !strings.Contains(text, "set authorized {") {
		t.Fatal("expected authorized set declaration")
	}
	if !strings.Contains(text, "aa:bb:cc:dd:ee:ff") {
		t.Fatal("expected authorized MAC in elements")
	}
	if strings.Contains(text, "11:22:33:44:55:66") {
		t.Fatal("unauthorized MAC should not appear")
	}
	if !strings.Contains(text, "ether saddr @authorized accept") {
		t.Fatal("expected accept rule for authorized MACs")
	}
	if !strings.Contains(text, "udp dport 53 accept") {
		t.Fatal("expected DNS allow rule")
	}
	if !strings.Contains(text, "tcp dport 8080 accept") {
		t.Fatal("expected portal port allow rule")
	}
	if !strings.Contains(text, "ip protocol icmp accept") {
		t.Fatal("expected ICMP allow rule")
	}
	if !strings.Contains(text, "drop") {
		t.Fatal("expected final drop rule")
	}
	// Chain must be multi-line: "chain forward {" on its own line, rules indented.
	if !strings.Contains(text, "chain forward {\n") {
		t.Fatal("expected multi-line chain declaration (chain forward { separate line)")
	}
}

func TestRenderNftNeverFlushesWholeRuleset(t *testing.T) {
	// A full `flush ruleset` would delete every other table on the box
	// (firewall / dpi / client-qos), not just `table inet portal`.
	text := RenderNft([]Client{{MAC: "aa:bb:cc:dd:ee:ff", Authorized: true}}, 8080)
	if strings.Contains(text, "flush ruleset") {
		t.Fatalf("RenderNft must not emit 'flush ruleset'; got:\n%s", text)
	}
}

func TestRenderNftChainMultiLineNotInline(t *testing.T) {
	text := RenderNft([]Client{}, 80)
	bad := strings.Contains(text, "chain forward { type")
	if bad {
		t.Fatal("chain declaration must be multi-line, not inline with type clause")
	}
}
