package ddns

import (
	"fmt"
	"os"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// NTPConfig holds timesyncd settings.
type NTPConfig struct {
	Servers []string `json:"servers"`
	Enabled bool     `json:"enabled"`
}

// RenderTimesyncd produces /etc/systemd/timesyncd.conf content.
func RenderTimesyncd(cfg NTPConfig) string {
	s := fmt.Sprintf("[Time]\nNTP=%s\n", stringsJoin(cfg.Servers, " "))
	if !cfg.Enabled {
		s += "# Enabled=false commented out; keep above line for manual toggle.\n"
	}
	return s
}

func stringsJoin(a []string, sep string) string {
	if len(a) == 0 {
		return ""
	}
	s := a[0]
	for _, v := range a[1:] {
		s += sep + v
	}
	return s
}

// ApplyTimesyncd writes the conf file and restarts timesyncd.
func ApplyTimesyncd(r execx.Runner, cfg NTPConfig) error {
	conf := RenderTimesyncd(cfg)
	if err := os.WriteFile("/etc/systemd/timesyncd.conf", []byte(conf), 0644); err != nil {
		return fmt.Errorf("write timesyncd.conf: %w", err)
	}
	if _, err := r.Run("systemctl", "restart", "systemd-timesyncd"); err != nil {
		return fmt.Errorf("restart timesyncd: %w (conf written successfully; verify manually)", err)
	}
	return nil
}

// Status reads timedatectl output and returns parsed info.
func Status(r execx.Runner) (map[string]string, error) {
	out, err := r.Run("timedatectl", "show")
	if err != nil {
		return nil, fmt.Errorf("timedatectl failed: %w", err)
	}
	res := map[string]string{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(parts[0], " ", "_")))
			val := strings.TrimSpace(parts[1])
			res[key] = val
		}
	}
	return res, nil
}
