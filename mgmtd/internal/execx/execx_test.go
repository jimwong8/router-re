package execx

import (
	"strings"
	"testing"
)

func TestLookupFindsShell(t *testing.T) {
	p, err := Lookup("sh")
	if err != nil {
		t.Fatalf("Lookup(sh) failed: %v", err)
	}
	if !strings.HasPrefix(p, "/") {
		t.Fatalf("expected absolute path, got %q", p)
	}
}

func TestLookupMissing(t *testing.T) {
	if _, err := Lookup("router-re-nonexistent-cmd-xyz"); err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestDefaultRunnerRun(t *testing.T) {
	out, err := DefaultRunner{}.Run("sh", "-c", "printf hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if out != "hello" {
		t.Fatalf("expected 'hello', got %q", out)
	}
}

func TestDefaultRunnerErrorIncludesStderr(t *testing.T) {
	_, err := DefaultRunner{}.Run("sh", "-c", "echo boom >&2; exit 3")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should include stderr, got %q", err.Error())
	}
}

func TestDefaultRunnerWithInput(t *testing.T) {
	out, err := DefaultRunner{}.RunWithInput("piped-data", "cat")
	if err != nil {
		t.Fatalf("RunWithInput failed: %v", err)
	}
	if out != "piped-data" {
		t.Fatalf("expected 'piped-data', got %q", out)
	}
}
