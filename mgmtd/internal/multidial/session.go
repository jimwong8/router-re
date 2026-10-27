package multidial

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Session is one PPPoE client dial-out (multi-dial).
type Session struct {
	Name     string `json:"name"`
	Iface    string `json:"iface"`
	Username string `json:"username"`
	Password string `json:"password"`
	Service  string `json:"service"`
	AC       string `json:"ac"`
	Metric   int    `json:"metric"`
	MRU      int    `json:"mru"`
	LCPEcho  int    `json:"lcp_echo"`
	Enabled  bool   `json:"enabled"`
}

// ValidateSession checks a PPPoE client session.
func ValidateSession(s Session) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name required")
	}
	for _, ch := range s.Name {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return fmt.Errorf("name %q: only letters, digits, '-' and '_' are allowed", s.Name)
		}
	}
	if strings.TrimSpace(s.Iface) == "" {
		return fmt.Errorf("iface required")
	}
	if s.Iface == "lo" {
		return fmt.Errorf("iface must not be lo")
	}
	if strings.TrimSpace(s.Username) == "" {
		return fmt.Errorf("username required")
	}
	if s.Metric < 0 || s.Metric > 1000 {
		return fmt.Errorf("metric must be in [0,1000], got %d", s.Metric)
	}
	if s.MRU != 0 && (s.MRU < 1280 || s.MRU > 1492) {
		// MRU above 1492 fails because PPPoE adds a 6-byte header on top
		// of the Ethernet payload, so the effective MTU must stay <= 1492.
		return fmt.Errorf("mru must be 0 or in [1280,1492], got %d (PPPoE 6-byte header makes >1492 impossible)", s.MRU)
	}
	if s.LCPEcho != 0 && (s.LCPEcho < 1 || s.LCPEcho > 3600) {
		return fmt.Errorf("lcp_echo must be 0 or in [1,3600], got %d", s.LCPEcho)
	}
	return nil
}

// LoadSessions reads a JSON file of sessions. A missing file is not an error.
func LoadSessions(path string) ([]Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Session{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var ss []Session
	if err := json.Unmarshal(data, &ss); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return ss, nil
}

// SaveSessions writes sessions as JSON with 0600 permissions (passwords in
// the same file, so the file must not be world-readable).
func SaveSessions(path string, ss []Session) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(ss, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
