package acl

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// RenderSysctl renders the `sysctl -w` lines that enable ARP spoofing
// protection on the given interface: strict unicast reverse ARP
// (arp_ignore=1) and local-only announcement (arp_announce=2).
func RenderSysctl(iface string) string {
	return fmt.Sprintf(
		"sysctl -w net.ipv4.conf.%s.arp_ignore=1\n"+
			"sysctl -w net.ipv4.conf.%s.arp_announce=2\n", iface, iface)
}

// ApplyArpGuard applies ARP anti-spoof sysctls on iface and verifies by
// re-reading each value with `sysctl -n`.
func ApplyArpGuard(r execx.Runner, iface string) error {
	if strings.TrimSpace(iface) == "" {
		return fmt.Errorf("iface required")
	}
	pairs := []struct{ key, val string }{
		{"net.ipv4.conf." + iface + ".arp_ignore", "1"},
		{"net.ipv4.conf." + iface + ".arp_announce", "2"},
	}
	for _, p := range pairs {
		setCmd := fmt.Sprintf("sysctl -w %s=%s", p.key, p.val)
		if _, err := r.Run("sh", "-c", setCmd); err != nil {
			return fmt.Errorf("set %s: %w", p.key, err)
		}
		cur, err := r.Run("sysctl", "-n", p.key)
		if err != nil {
			return fmt.Errorf("verify %s: %w", p.key, err)
		}
		if strings.TrimSpace(cur) != p.val {
			return fmt.Errorf("verify %s: got %q want %q", p.key, cur, p.val)
		}
	}
	return nil
}

// NeighEntry is a parsed row from `ip -j neigh show`.
type NeighEntry struct {
	If    string `json:"if"`
	IP    string `json:"ip"`
	MAC   string `json:"mac"`
	State string `json:"state"`
}

// StaticBindings reads the current neighbor table (static entries) using the
// JSON output of `ip neigh show`.
func StaticBindings(r execx.Runner) ([]map[string]string, error) {
	out, err := r.RunWithInput("", "ip", "-j", "neigh", "show")
	if err != nil {
		return nil, fmt.Errorf("ip neigh show: %w", err)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse neighbor json: %w", err)
	}
	result := make([]map[string]string, 0, len(raw))
	for _, m := range raw {
		row := make(map[string]string)
		for k, v := range m {
			switch vv := v.(type) {
			case string:
				row[k] = vv
			case float64:
				row[k] = fmt.Sprintf("%v", vv)
			default:
				row[k] = fmt.Sprint(v)
			}
		}
		result = append(result, row)
	}
	return result, nil
}
