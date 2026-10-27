package logship

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// RotateConfig controls logrotate-driven rotation of a log file.
type RotateConfig struct {
	Path      string `json:"path"`
	MaxSizeMB int    `json:"max_size_mb"`
	KeepFiles int    `json:"keep_files"`
	Compress  bool   `json:"compress"`
	Interval  string `json:"interval"`
}

// ValidateRotate checks a RotateConfig.
func ValidateRotate(c RotateConfig) error {
	if c.Path == "" {
		return fmt.Errorf("path required")
	}
	if !filepath.IsAbs(c.Path) {
		return fmt.Errorf("path must be absolute, got %q", c.Path)
	}
	if strings.HasPrefix(c.Path, "/") && (c.Path == "/" || strings.HasPrefix(c.Path, "/proc") || strings.HasPrefix(c.Path, "/sys")) {
		return fmt.Errorf("path must not be /, /proc/* or /sys/* (would damage the system): %s", c.Path)
	}
	if c.MaxSizeMB < 1 || c.MaxSizeMB > 10240 {
		return fmt.Errorf("max_size_mb must be in [1,10240], got %d", c.MaxSizeMB)
	}
	if c.KeepFiles < 1 || c.KeepFiles > 100 {
		return fmt.Errorf("keep_files must be in [1,100], got %d", c.KeepFiles)
	}
	switch c.Interval {
	case "daily", "weekly", "none":
	default:
		return fmt.Errorf("interval must be daily/weekly/none, got %q", c.Interval)
	}
	return nil
}

// RenderLogrotate emits a logrotate snippet. copytruncate is used so the
// application never has to reopen its log fd — the cost is a window where
// a few bytes written during the copy can be lost.
func RenderLogrotate(c RotateConfig) string {
	var b strings.Builder
	b.WriteString(c.Path + " {\n")
	if c.Interval == "none" {
		fmt.Fprintf(&b, "\tsize %dM\n", c.MaxSizeMB)
	} else {
		fmt.Fprintf(&b, "%s\n", c.Interval)
	}
	fmt.Fprintf(&b, "\trotate %d\n", c.KeepFiles)
	if c.Compress {
		b.WriteString("\tcompress\n")
	} else {
		b.WriteString("\tnocompress\n")
	}
	b.WriteString("\tmissingok\n")
	b.WriteString("\tnotifempty\n")
	b.WriteString("\tcopytruncate\n")
	b.WriteString("}\n")
	return b.String()
}

// ApplyRotate validates, writes the conf (0644), runs `logrotate -d` for a
// dry syntax check, then `logrotate -f` to force one rotation and prove the
// config works. A failed syntax check returns the raw logrotate stderr.
func ApplyRotate(r execx.Runner, c RotateConfig, confPath string) error {
	if err := ValidateRotate(c); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderLogrotate(c)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	// dry-run syntax check first — never apply a broken config
	if _, err := r.Run("logrotate", "-d", confPath); err != nil {
		return fmt.Errorf("logrotate -d %s: %w", confPath, err)
	}
	// force one rotation to prove the config is actually usable
	if _, err := r.Run("logrotate", "-f", confPath); err != nil {
		return fmt.Errorf("logrotate -f %s: %w", confPath, err)
	}
	return nil
}
