package server

import (
	"testing"
	"time"
)

func TestLoginLimiterAllowsWithinWindow(t *testing.T) {
	l := newLoginLimiter(3, time.Minute)
	if !l.allow("1.2.3.4") {
		t.Fatal("first attempt should be allowed")
	}
	if !l.allow("1.2.3.4") {
		t.Fatal("second attempt should be allowed")
	}
	if !l.allow("1.2.3.4") {
		t.Fatal("third attempt should be allowed")
	}
	// fourth exceeds the cap
	if l.allow("1.2.3.4") {
		t.Fatal("fourth attempt should be rejected")
	}
}

func TestLoginLimiterSeparatesIPs(t *testing.T) {
	l := newLoginLimiter(1, time.Minute)
	if !l.allow("10.0.0.1") {
		t.Fatal("first ip should be allowed")
	}
	if l.allow("10.0.0.1") {
		t.Fatal("same ip second attempt should be rejected")
	}
	if !l.allow("10.0.0.2") {
		t.Fatal("different ip should be allowed")
	}
}

func TestLoginLimiterWindowResets(t *testing.T) {
	l := newLoginLimiter(1, 50*time.Millisecond)
	if !l.allow("1.1.1.1") {
		t.Fatal("first should be allowed")
	}
	if l.allow("1.1.1.1") {
		t.Fatal("second within window should be rejected")
	}
	time.Sleep(80 * time.Millisecond)
	if !l.allow("1.1.1.1") {
		t.Fatal("after window expiry should be allowed again")
	}
}

func TestClientIPStripsPort(t *testing.T) {
	if got := clientIP("192.168.1.5:34567"); got != "192.168.1.5" {
		t.Fatalf("expected stripped ip, got %q", got)
	}
	if got := clientIP("192.168.1.5"); got != "192.168.1.5" {
		t.Fatalf("expected pass-through, got %q", got)
	}
}