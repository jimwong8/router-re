package l7filter

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	getentOut string
	cmds      []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.cmds = append(f.cmds, name+" "+strings.Join(args, " "))
	if name == "getent" {
		return f.getentOut, nil
	}
	return "", errString("unexpected cmd: " + name)
}

// ranInjection reports whether any command contained shell metacharacters
// (i.e. an unvalidated domain reached the executor).
func (f *fakeRunner) ranInjection() bool {
	for _, cmd := range f.cmds {
		if strings.ContainsAny(cmd, ";|&`$\\") {
			return true
		}
	}
	return false
}
func (f *fakeRunner) RunWithInput(input, name string, args ...string) (string, error) {
	return "", errString("unexpected RunWithInput: " + name)
}

type errString string

func (e errString) Error() string { return string(e) }

func goodRule() Rule {
	return Rule{ID: "r1", Name: "ads", MatchType: MatchSNI, Pattern: "*.doubleclick.net", Action: ActionBlock, Enabled: true}
}

func TestValidateMatchType(t *testing.T) {
	for _, mt := range []string{"sni", "http_host", "user_agent", "dns"} {
		r := goodRule()
		r.MatchType = mt
		if err := Validate(r); err != nil {
			t.Errorf("valid match_type %q rejected: %v", mt, err)
		}
	}
	bad := []string{"ip", "port", "url", "", "SNI", "http"}
	for _, mt := range bad {
		r := goodRule()
		r.MatchType = mt
		if err := Validate(r); err == nil {
			t.Errorf("invalid match_type %q should error", mt)
		}
	}
}

func TestValidateAction(t *testing.T) {
	for _, a := range []string{"block", "allow", "log"} {
		r := goodRule()
		r.Action = a
		if err := Validate(r); err != nil {
			t.Errorf("valid action %q rejected: %v", a, err)
		}
	}
	for _, a := range []string{"drop", "pass", "", "BLOCK"} {
		r := goodRule()
		r.Action = a
		if err := Validate(r); err == nil {
			t.Errorf("invalid action %q should error", a)
		}
	}
}

func TestValidateEmptyID(t *testing.T) {
	r := goodRule()
	r.ID = ""
	if err := Validate(r); err == nil {
		t.Fatal("empty id should error")
	}
}

func TestValidatePatternVarious(t *testing.T) {
	// empty pattern
	r := goodRule()
	r.Pattern = ""
	if err := Validate(r); err == nil {
		t.Fatal("empty pattern should error")
	}
	// too long
	r.Pattern = strings.Repeat("a", 257)
	if err := Validate(r); err == nil {
		t.Fatal("256+ pattern should error")
	}
	// domain with space
	r = goodRule()
	r.Pattern = "a b.com"
	if err := Validate(r); err == nil {
		t.Fatal("domain with space should error")
	}
	// leading dot
	r.Pattern = ".example.com"
	if err := Validate(r); err == nil {
		t.Fatal("leading dot should error")
	}
	// leading hyphen
	r.Pattern = "-example.com"
	if err := Validate(r); err == nil {
		t.Fatal("leading hyphen should error")
	}
	// trailing hyphen
	r.Pattern = "example-.com"
	if err := Validate(r); err == nil {
		t.Fatal("trailing hyphen should error")
	}
	// lone wildcard without suffix
	r.Pattern = "*."
	if err := Validate(r); err == nil {
		t.Fatal("bare '*' should error")
	}
	// '*' not at front
	r.Pattern = "foo*.example.com"
	if err := Validate(r); err == nil {
		t.Fatal("mid '*' should error")
	}
	// valid wildcard passes
	r.Pattern = "*.doubleclick.net"
	if err := Validate(r); err != nil {
		t.Fatalf("valid wildcard rejected: %v", err)
	}
}

func TestValidateUserAgentControlChars(t *testing.T) {
	r := goodRule()
	r.MatchType = MatchUserAgent
	r.Pattern = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) App/1.2.3"
	if err := Validate(r); err != nil {
		t.Fatalf("valid UA rejected: %v", err)
	}
	r.Pattern = "good\nbad"
	if err := Validate(r); err == nil {
		t.Fatal("UA with newline (control char) should error")
	}
	r.Pattern = "good\x00bad"
	if err := Validate(r); err == nil {
		t.Fatal("UA with NUL (control char) should error")
	}
}

func TestMatchDomainWildcardSemantics(t *testing.T) {
	cases := []struct {
		pattern string
		host    string
		want    bool
	}{
		// 通配：*.example.com
		{"*.example.com", "a.example.com", true},
		{"*.example.com", "b.c.example.com", true},
		{"*.example.com", "example.com", false},  // 裸域名不是子域
		{"*.example.com", "aexample.com", false}, // 前缀必须以 . 结尾
		{"*.example.com", "example.org", false},
		// 精确
		{"example.com", "example.com", true},
		{"example.com", "example.com.", true}, // 尾部点忽略
		{"example.com", "sub.example.com", false},
		{"example.com", "EXAMPLE.COM", true}, // 大小写不敏感
		// 子域精确
		{"a.b.example.com", "a.b.example.com", true},
		{"a.b.example.com", "x.a.b.example.com", false},
	}
	for _, c := range cases {
		if got := MatchDomain(c.pattern, c.host); got != c.want {
			t.Errorf("MatchDomain(%q, %q) = %v, want %v", c.pattern, c.host, got, c.want)
		}
	}
}

func TestMatchRequestOnlyEnabled(t *testing.T) {
	on := goodRule()
	off := goodRule()
	off.ID = "r2"
	off.Enabled = false
	rules := []Rule{on, off}
	req := Request{SNI: "ads.doubleclick.net"}
	hit := MatchRequest(rules, req)
	if len(hit) != 1 {
		t.Fatalf("expected 1 hit (disabled excluded), got %d", len(hit))
	}
	if hit[0].ID != "r1" {
		t.Errorf("expected r1 hit, got %s", hit[0].ID)
	}
}

func TestMatchRequestFieldSelection(t *testing.T) {
	rules := []Rule{
		{ID: "sni", MatchType: MatchSNI, Pattern: "*.doubleclick.net", Action: ActionBlock, Enabled: true},
		{ID: "host", MatchType: MatchHTTPHost, Pattern: "ads.example.com", Action: ActionBlock, Enabled: true},
		{ID: "dns", MatchType: MatchDNS, Pattern: "tracker.example.net", Action: ActionBlock, Enabled: true},
		{ID: "ua", MatchType: MatchUserAgent, Pattern: "curl", Action: ActionLog, Enabled: true},
	}
	req := Request{SNI: "x.doubleclick.net", HTTPHost: "other.com", DNSName: "clean.com", UserAgent: "curl/7.8"}
	hit := MatchRequest(rules, req)
	ids := map[string]bool{}
	for _, h := range hit {
		ids[h.ID] = true
	}
	if !ids["sni"] || !ids["ua"] {
		t.Errorf("expected sni+ua hits, got %v", ids)
	}
	if ids["host"] || ids["dns"] {
		t.Errorf("non-matching fields must not hit: %v", ids)
	}
}

func TestShouldBlockAllowOverridesBlock(t *testing.T) {
	// 只有 block 命中 → 拦
	rules := []Rule{{ID: "b", Name: "b", MatchType: MatchSNI, Pattern: "*.doubleclick.net", Action: ActionBlock, Enabled: true}}
	blocked, name := ShouldBlock(rules, Request{SNI: "a.doubleclick.net"})
	if !blocked || name != "b" {
		t.Fatalf("expected blocked by b, got blocked=%v name=%q", blocked, name)
	}
	// block + allow 同时命中 → allow 优先 → 放行
	rules = []Rule{
		{ID: "b", Name: "b", MatchType: MatchSNI, Pattern: "*.doubleclick.net", Action: ActionBlock, Enabled: true},
		{ID: "a", Name: "a", MatchType: MatchSNI, Pattern: "a.doubleclick.net", Action: ActionAllow, Enabled: true},
	}
	blocked, name = ShouldBlock(rules, Request{SNI: "a.doubleclick.net"})
	if blocked {
		t.Fatalf("allow must override block, got blocked name=%q", name)
	}
	// 只有 log 命中 → 放行
	rules = []Rule{{ID: "l", MatchType: MatchSNI, Pattern: "*.doubleclick.net", Action: ActionLog, Enabled: true}}
	blocked, _ = ShouldBlock(rules, Request{SNI: "a.doubleclick.net"})
	if blocked {
		t.Fatal("log-only must not block")
	}
	// 无命中 → 放行
	blocked, _ = ShouldBlock(rules, Request{SNI: "clean.com"})
	if blocked {
		t.Fatal("no match must not block")
	}
}

func TestRenderDnsmasqBlock(t *testing.T) {
	out := RenderDnsmasqBlock([]string{"ads.example.com", "tracker.example.net"})
	if !strings.Contains(out, "address=/ads.example.com/0.0.0.0") {
		t.Errorf("missing ipv4 block:\n%s", out)
	}
	if !strings.Contains(out, "address=/ads.example.com/::") {
		t.Errorf("missing ipv6 block:\n%s", out)
	}
	if !strings.Contains(out, "address=/tracker.example.net/0.0.0.0") {
		t.Errorf("missing second domain:\n%s", out)
	}
}

func TestRenderNftIpBlockNoFlush(t *testing.T) {
	out := RenderNftIpBlock([]string{"1.2.3.4", "5.6.7.8"})
	if !strings.Contains(out, "table inet router_l7 {") {
		t.Errorf("missing table header:\n%s", out)
	}
	if !strings.Contains(out, "1.2.3.4") || !strings.Contains(out, "5.6.7.8") {
		t.Errorf("missing set elements:\n%s", out)
	}
	if !strings.Contains(out, "ip daddr @blocked_ip drop") {
		t.Errorf("missing drop rule:\n%s", out)
	}
	if strings.Contains(out, "flush ruleset") || strings.Contains(out, "flush table") {
		t.Errorf("rendered config contains a flush:\n%s", out)
	}
	// chain declaration multi-line
	if strings.Contains(out, "chain forward { type") {
		t.Errorf("chain declaration not multi-line:\n%s", out)
	}
	if !strings.Contains(out, "type filter hook forward priority filter;") {
		t.Errorf("missing forward hook line:\n%s", out)
	}
}

func TestResolveDomainsValidatesInput(t *testing.T) {
	fr := &fakeRunner{getentOut: "1.2.3.4 ads.example.com"}
	res := ResolveDomains(fr, []string{"ads.example.com", "a;rm -rf /", "bad name.com"})
	if len(res) != 1 {
		t.Fatalf("only the valid domain should resolve, got %v", res)
	}
	if fr.ranInjection() {
		t.Errorf("malicious domain reached the executor; final cmds: %v", fr.cmds)
	}
	// only the valid domain should have produced a getent invocation
	if len(fr.cmds) != 1 || !strings.Contains(fr.cmds[0], "ads.example.com") {
		t.Errorf("expected exactly one getent for the valid domain, got %v", fr.cmds)
	}
	if ips := res["ads.example.com"]; len(ips) != 1 || ips[0] != "1.2.3.4" {
		t.Errorf("unexpected resolved ips: %v", ips)
	}
}

func TestResolveDomainsSkipsUnresolvable(t *testing.T) {
	fr := &fakeRunner{getentOut: ""}
	res := ResolveDomains(fr, []string{"nope.local"})
	if len(res) != 0 {
		t.Errorf("unresolvable domain should be skipped, got %v", res)
	}
}
