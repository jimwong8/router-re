package portal

import (
	"testing"
	"time"
)

func TestAuthorizeAndListSorted(t *testing.T) {
	m := NewManager()
	_, err := m.Authorize("aa:aa:aa:aa:aa:aa", "10.0.0.1", 30*time.Minute)
	if err != nil {
		t.Fatalf("authorize failed: %v", err)
	}
	_, err = m.Authorize("bb:bb:bb:bb:bb:bb", "10.0.0.2", 60*time.Minute)
	if err != nil {
		t.Fatalf("authorize second failed: %v", err)
	}
	clients := m.List()
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
	if clients[0].MAC != "aa:aa:aa:aa:aa:aa" || clients[1].MAC != "bb:bb:bb:bb:bb:bb" {
		t.Fatal("clients should be sorted by MAC ascending")
	}
}

func TestRevokeRemovesClient(t *testing.T) {
	m := NewManager()
	m.Authorize("aa:aa:aa:aa:aa:aa", "10.0.0.1", 30*time.Minute)
	if m.Revoke("aa:aa:aa:aa:aa:aa") != true {
		t.Fatal("expected revoke to return true")
	}
	if m.Revoke("aa:aa:aa:aa:aa:aa") != false {
		t.Fatal("expected second revoke to return false")
	}
	clients := m.List()
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients after revoke, got %d", len(clients))
	}
}

func TestAuthorizedFiltersExpired(t *testing.T) {
	m := NewManager()
	m.Authorize("aa:aa:aa:aa:aa:aa", "10.0.0.1", 30*time.Minute)
	auth := m.Authorized(time.Now().Add(2 * time.Hour))
	if len(auth) != 0 {
		t.Fatalf("expected 0 authorized clients (expired), got %d", len(auth))
	}
}
