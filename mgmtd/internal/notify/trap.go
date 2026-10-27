package notify

import (
	"fmt"
	"strings"
)

// TrapTarget is an SNMP trap destination.
type TrapTarget struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Version   string `json:"version"`
	Community string `json:"community"`
	Enabled   bool   `json:"enabled"`
}

// ValidateTrap checks a TrapTarget.
func ValidateTrap(t TrapTarget) error {
	if strings.TrimSpace(t.Host) == "" {
		return fmt.Errorf("host required")
	}
	if t.Port < 1 || t.Port > 65535 {
		return fmt.Errorf("port must be in [1,65535], got %d", t.Port)
	}
	switch t.Version {
	case "v1", "v2c":
	default:
		return fmt.Errorf("version must be v1 or v2c, got %q", t.Version)
	}
	if strings.TrimSpace(t.Community) == "" {
		return fmt.Errorf("community required")
	}
	if strings.Contains(t.Community, " ") {
		return fmt.Errorf("community must not contain spaces, got %q", t.Community)
	}
	return nil
}

// Masked returns a copy with the community replaced by "***".
func (t TrapTarget) Masked() TrapTarget {
	m := t
	m.Community = "***"
	return m
}

// RenderSink emits an snmptrapd.conf action line.
func RenderSink(t TrapTarget) string {
	return fmt.Sprintf("authCommunity log,execute,net %s default\n", t.Community)
}
