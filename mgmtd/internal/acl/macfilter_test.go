package acl

import (
	"strings"
	"testing"
)

func TestRenderNftMAC(t *testing.T) {
	rs := []Rule{{SrcMAC: "00:11:22:33:44:55", DstMAC: "aa:bb:cc:dd:ee:ff", Action: "drop", Enabled: true}}
	out := RenderNft(rs)
	if !strings.Contains(out, "ether saddr 00:11:22:33:44:55") {
		t.Fatalf("src mac missing: %s", out)
	}
	if !strings.Contains(out, "ether daddr aa:bb:cc:dd:ee:ff") {
		t.Fatalf("dst mac missing: %s", out)
	}
}

func TestMacFilterValidate(t *testing.T) {
	if err := ValidateMacFilter(MacFilter{Mode: "blacklist", MACs: []string{"00:11:22:33:44:55"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMacFilter(MacFilter{Mode: "nope", MACs: nil}); err == nil {
		t.Fatal("expected invalid mode error")
	}
	if err := ValidateMacFilter(MacFilter{Mode: "whitelist", MACs: []string{"bad"}}); err == nil {
		t.Fatal("expected invalid mac error")
	}
}

func TestRenderMacFilterBlacklist(t *testing.T) {
	out := RenderMacFilter(MacFilter{Mode: "blacklist", MACs: []string{"00:11:22:33:44:55"}})
	if !strings.Contains(out, "ether saddr 00:11:22:33:44:55 drop") {
		t.Fatalf("blacklist drop missing: %s", out)
	}
	if !strings.Contains(out, "accept") {
		t.Fatalf("blacklist must accept others: %s", out)
	}
}

func TestRenderMacFilterWhitelist(t *testing.T) {
	out := RenderMacFilter(MacFilter{Mode: "whitelist", MACs: []string{"00:11:22:33:44:55"}})
	if !strings.Contains(out, "ether saddr 00:11:22:33:44:55 accept") {
		t.Fatalf("whitelist accept missing: %s", out)
	}
	if !strings.Contains(out, "\n        drop\n") {
		t.Fatalf("whitelist must end with drop: %s", out)
	}
}
