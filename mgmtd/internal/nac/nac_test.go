package nac

import (
	"strings"
	"testing"
)

type stubRunner struct {
	out string
	err error
}

func (s *stubRunner) Run(name string, args ...string) (string, error) { return s.out, s.err }
func (s *stubRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, s.err
}

func TestValidateRejectsEUI64(t *testing.T) {
	// 8-byte MAC must be rejected
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "00:11:22:33:44:55:66:77", Policy: "allow"}}}
	if err := Validate(c); err == nil {
		t.Fatal("8-byte MAC must be rejected")
	}
}

func TestValidateDuplicateMAC(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{
		{MAC: "aa:bb:cc:dd:ee:ff", Name: "a", Policy: "deny"},
		{MAC: "AA:BB:CC:DD:EE:FF", Name: "b", Policy: "deny"},
	}}
	if err := Validate(c); err == nil {
		t.Fatal("duplicate MAC must be rejected")
	} else if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Errorf("error should name both conflicting devices: %v", err)
	}
}

func TestValidateWhitelistEmpty(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "deny"}}}
	if err := Validate(c); err == nil {
		t.Fatal("whitelist with no allow device must be rejected")
	} else if !strings.Contains(err.Error(), "block everyone") && !strings.Contains(err.Error(), "self-lockout") {
		t.Errorf("error should warn about self-lockout: %v", err)
	}
}

func TestValidateBadPolicy(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "banned"}}}
	if err := Validate(c); err == nil {
		t.Fatal("bad policy must be rejected")
	}
}

func TestValidateMaxConnsZero(t *testing.T) {
	if err := ValidateLimit(Limit{Name: "x", SrcIP: "1.2.3.4", MaxConns: 0}); err == nil {
		t.Fatal("max_conns=0 must be rejected")
	}
}

func TestValidateLimitOK(t *testing.T) {
	if err := ValidateLimit(Limit{Name: "x", SrcIP: "192.168.1.0/24", MaxConns: 100, MaxPerSec: 50}); err != nil {
		t.Fatalf("valid limit rejected: %v", err)
	}
}

func TestRenderNftUsesEtherSaddr(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "allow"}}}
	out := RenderNft(c)
	if !strings.Contains(out, "ether saddr") {
		t.Errorf("MAC filter must use ether saddr, got:\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("must never flush ruleset")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("must never set policy drop")
	}
}

func TestRenderNftWhitelistDropsNonAllow(t *testing.T) {
	c := Config{Mode: "whitelist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "allow"}}}
	out := RenderNft(c)
	if !strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF accept") {
		t.Errorf("whitelist must accept listed MAC:\n%s", out)
	}
	if !strings.Contains(out, "drop") {
		t.Errorf("whitelist must drop non-listed:\n%s", out)
	}
}

func TestRenderNftBlacklistDropsListed(t *testing.T) {
	c := Config{Mode: "blacklist", Devices: []Device{{MAC: "aa:bb:cc:dd:ee:ff", Policy: "deny"}}}
	out := RenderNft(c)
	if strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF accept") {
		t.Errorf("blacklist must not accept listed MAC:\n%s", out)
	}
	if !strings.Contains(out, "ether saddr AA:BB:CC:DD:EE:FF drop") {
		t.Errorf("blacklist must drop listed MAC:\n%s", out)
	}
}

func TestRenderConnlimitUsesCtCount(t *testing.T) {
	ls := []Limit{{Name: "x", SrcIP: "1.2.3.4", MaxConns: 10, MaxPerSec: 5, Enabled: true}}
	out := RenderConnlimit(ls)
	if !strings.Contains(out, "ct count over 10") {
		t.Errorf("connlimit must use ct count over, got:\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") {
		t.Fatal("must never flush ruleset")
	}
	if strings.Contains(out, "policy drop") {
		t.Fatal("must never set policy drop")
	}
}

func TestNeighborsJSON(t *testing.T) {
	jsonStr := `[{"dev":"ens18","dst":"10.0.0.1","lladdr":"aa:bb:cc:dd:ee:ff","state":"REACHABLE"},{"dev":"ens18","dst":"10.0.0.2","lladdr":"11:22:33:44:55:66","state":"STALE"}]`
	r := &stubRunner{out: jsonStr}
	devs, err := Neighbors(r)
	if err != nil {
		t.Fatalf("Neighbors error: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(devs))
	}
	if devs[0].MAC != "aa:bb:cc:dd:ee:ff" || devs[0].IP != "10.0.0.1" {
		t.Errorf("unexpected first neighbor: %+v", devs[0])
	}
}

// TestNeighborsParsesStateArray 锁死一个真实踩到的解析错误：
// iproute2 的 `ip -j neigh show` 里 state 是数组（["REACHABLE"]）而不是字符串。
// 写成 string 会让整个邻居表 Unmarshal 失败 -> HTTP 500。
func TestNeighborsParsesStateArray(t *testing.T) {
	const realJSON = `[{"dst":"10.100.1.168","dev":"ens18","lladdr":"bc:24:11:76:6f:e2","state":["STALE"]},` +
		`{"dst":"10.250.17.2","dev":"vgh772002093","lladdr":"1a:12:d0:e8:d4:58","state":["REACHABLE","DELAY"]}]`
	r := &stubRunner{out: realJSON}
	got, err := Neighbors(r)
	if err != nil {
		t.Fatalf("Neighbors must accept array state, got error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 neighbors, got %d", len(got))
	}
	if got[0].Note != "STALE" {
		t.Fatalf("want STALE, got %q", got[0].Note)
	}
	if got[1].Note != "REACHABLE,DELAY" {
		t.Fatalf("multi-state should join with comma, got %q", got[1].Note)
	}
	if got[0].MAC != "bc:24:11:76:6f:e2" || got[0].IP != "10.100.1.168" {
		t.Fatalf("field mapping wrong: %+v", got[0])
	}
}
