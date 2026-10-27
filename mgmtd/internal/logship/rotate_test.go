package logship

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type lsStub struct {
	out string
	err error
}

func (s *lsStub) Run(name string, args ...string) (string, error) { return s.out, s.err }
func (s *lsStub) RunWithInput(input, name string, args ...string) (string, error) {
	return s.out, s.err
}

func TestValidateRejectsRelativePath(t *testing.T) {
	if err := ValidateRotate(RotateConfig{Path: "var/log/mgmtd.log", MaxSizeMB: 10, KeepFiles: 5, Interval: "daily"}); err == nil {
		t.Fatal("relative path must be rejected")
	}
}

func TestValidateRejectsProcPath(t *testing.T) {
	if err := ValidateRotate(RotateConfig{Path: "/proc/sys/net/ipv4/ip_forward", MaxSizeMB: 10, KeepFiles: 5, Interval: "daily"}); err == nil {
		t.Fatal("/proc path must be rejected")
	}
}

func TestValidateRejectsMaxSizeMBZero(t *testing.T) {
	if err := ValidateRotate(RotateConfig{Path: "/var/log/mgmtd.log", MaxSizeMB: 0, KeepFiles: 5, Interval: "daily"}); err == nil {
		t.Fatal("max_size_mb=0 must be rejected")
	}
}

func TestRenderLogrotateContainsKeyDirectives(t *testing.T) {
	c := RotateConfig{Path: "/var/log/mgmtd.log", MaxSizeMB: 10, KeepFiles: 5, Compress: true, Interval: "none"}
	out := RenderLogrotate(c)
	for _, want := range []string{"size 10M", "rotate 5", "compress", "missingok", "notifempty", "copytruncate"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderLogrotate missing %q:\n%s", want, out)
		}
	}
}

func TestRenderLogrotateDailyNoSize(t *testing.T) {
	c := RotateConfig{Path: "/var/log/mgmtd.log", MaxSizeMB: 10, KeepFiles: 5, Interval: "daily"}
	out := RenderLogrotate(c)
	if !strings.Contains(out, "daily") {
		t.Errorf("daily interval missing:\n%s", out)
	}
	if strings.Contains(out, "size") {
		t.Errorf("size must not be present with interval=daily:\n%s", out)
	}
}

func TestApplyRotateDryRunFailureReturnsStderr(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "mgmtd")
	// stub returns a non-empty stderr-style error on `logrotate -d`
	stub := &lsStub{err: &lsErr{"logrotate: fatal: bad config: missing brace"}}
	err := ApplyRotate(stub, RotateConfig{Path: "/var/log/mgmtd.log", MaxSizeMB: 10, KeepFiles: 5, Interval: "none"}, conf)
	if err == nil {
		t.Fatal("dry-run failure must return an error")
	}
	if !strings.Contains(err.Error(), "bad config") {
		t.Errorf("error must contain raw stderr, got: %v", err)
	}
}

func TestApplyRotateSuccess(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "mgmtd")
	stub := &lsStub{out: "rotating /var/log/mgmtd.log"}
	if err := ApplyRotate(stub, RotateConfig{Path: "/var/log/mgmtd.log", MaxSizeMB: 10, KeepFiles: 5, Interval: "none"}, conf); err != nil {
		t.Fatalf("ApplyRotate should succeed: %v", err)
	}
	// conf file must exist with 0644 perms
	info, err := os.Stat(conf)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o777 != 0o644 {
		t.Errorf("conf perms should be 0644, got %v", info.Mode())
	}
}

type lsErr struct{ msg string }

func (e *lsErr) Error() string { return e.msg }
