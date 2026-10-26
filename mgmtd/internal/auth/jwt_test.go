package auth

import (
	"testing"
	"time"
)

func TestTokenRoundTrip(t *testing.T) {
	tm := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	tok, err := tm.Issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := tm.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if sub != "admin" {
		t.Fatalf("expected admin, got %s", sub)
	}
}

func TestTokenExpired(t *testing.T) {
	tm := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"), -time.Minute)
	tok, err := tm.Issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail verification")
	}
}

func TestTokenWrongSecret(t *testing.T) {
	tm := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	tok, err := tm.Issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokenManager([]byte("ffffffffffffffffffffffffffffffff"), time.Minute)
	if _, err := other.Verify(tok); err == nil {
		t.Fatal("expected token signed with other secret to fail")
	}
}

func TestTokenTampered(t *testing.T) {
	tm := NewTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Minute)
	tok, err := tm.Issue("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Verify(tok + "x"); err == nil {
		t.Fatal("expected tampered token to fail")
	}
}
