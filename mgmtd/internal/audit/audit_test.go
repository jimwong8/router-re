package audit

import (
	"path/filepath"
	"testing"
)

func TestLogAndQuery(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "audit.jsonl")
	l := New(p)

	l.Log(Event{TS: 1, Actor: "alice", Action: "test.read", Target: "x", Result: "ok", Detail: "d1"})
	l.Log(Event{TS: 2, Actor: "bob", Action: "test.write", Target: "y", Result: "fail", RemoteIP: "1.2.3.4"})
	l.Log(Event{TS: 3, Actor: "alice", Action: "login.success", Target: "admin", Result: "ok"})

	all, err := l.Query(10, "")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 events, got %d", len(all))
	}
	// newest first (reverse)
	if all[0].TS != 3 || all[1].TS != 2 || all[2].TS != 1 {
		t.Errorf("expected reversed order; got %v", all)
	}

	filtered, err := l.Query(10, "test")
	if err != nil {
		t.Fatalf("Query filter failed: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 'test' events, got %d", len(filtered))
	}
}

func TestQueryNotExists(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "missing.jsonl")
	l := New(p)
	events, err := l.Query(10, "")
	if err != nil {
		t.Fatalf("Query on missing file should return nil error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice for missing file, got %d", len(events))
	}
}

func TestLogAutoTS(t *testing.T) {
	d := t.TempDir()
	l := New(filepath.Join(d, "auto.jsonl"))
	l.Log(Event{Actor: "a", Action: "b", Target: "t", Result: "r"})
	all, _ := l.Query(1, "")
	if len(all) != 1 || all[0].TS == 0 {
		t.Errorf("expected TS auto-filled; got %v", all)
	}
}
