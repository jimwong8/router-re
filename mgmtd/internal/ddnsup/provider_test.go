package ddnsup

import (
	"strings"
	"testing"
)

func TestFindProviderListsKeys(t *testing.T) {
	_, err := FindProvider("no-such-provider")
	if err == nil {
		t.Fatal("unknown provider must fail")
	}
	msg := err.Error()
	for _, k := range []string{"cloudflare", "aliyun", "duckdns", "noip", "custom"} {
		if !strings.Contains(msg, k) {
			t.Fatalf("error must list available key %q, got %q", k, msg)
		}
	}
}

func TestValidateRejectsUnknownProvider(t *testing.T) {
	r := Record{Provider: "ghost", Domain: "example.com", Token: "12345678"}
	if err := Validate(r); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestValidateRejectsShortToken(t *testing.T) {
	r := Record{Provider: "cloudflare", Domain: "example.com", Token: "short"}
	if err := Validate(r); err == nil {
		t.Fatal("token < 8 must fail")
	}
}

func TestValidateCloudflareNeedsToken(t *testing.T) {
	r := Record{Provider: "cloudflare", Domain: "example.com"}
	if err := Validate(r); err == nil {
		t.Fatal("cloudflare without token must fail")
	}
}

func TestMaskedDoesNotLeak(t *testing.T) {
	secret := "supersecrettoken123"
	r := Record{Token: secret, Password: "anothersecret99"}
	m := Masked(r)
	if strings.Contains(m.Token, secret) {
		t.Fatal("masked token must not contain full secret")
	}
	if strings.Contains(m.Password, "anothersecret99") {
		t.Fatal("masked password must not contain full secret")
	}
	// 保留 4+4
	if m.Token != "supe*********n123" {
		// allow any masking as long as prefix/suffix 4
		if len(m.Token) < 8 || m.Token[:4] != "supe" || m.Token[len(m.Token)-4:] != "n123" {
			t.Fatalf("masked token format unexpected: %q", m.Token)
		}
	}
}

func TestMaskedShortFullyMasked(t *testing.T) {
	r := Record{Token: "short"}
	m := Masked(r)
	if m.Token != "*****" {
		t.Fatalf("short token must be fully masked, got %q", m.Token)
	}
	if strings.Contains(m.Token, "short") {
		t.Fatal("short token must not leak")
	}
}

func TestNeedsUpdateIPv6Normalization(t *testing.T) {
	if NeedsUpdate("2409:8a5c::1", "2409:8a5c:0:0:0:0:0:1") {
		t.Fatal("same IPv6 different notation must return false")
	}
}

func TestNeedsUpdateDifferentIP(t *testing.T) {
	if !NeedsUpdate("1.2.3.4", "5.6.7.8") {
		t.Fatal("different IPs must return true")
	}
}

func TestNeedsUpdateEmptyOld(t *testing.T) {
	if !NeedsUpdate("", "1.2.3.4") {
		t.Fatal("empty old (first run) must return true")
	}
}

func TestProvidersCountAndUnique(t *testing.T) {
	ps := Providers()
	if len(ps) < 5 {
		t.Fatalf("need at least 5 providers, got %d", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Key] {
			t.Fatalf("duplicate provider key %q", p.Key)
		}
		seen[p.Key] = true
	}
}
