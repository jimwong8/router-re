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

// ApplyArpGuard applies ARP anti-spoof sysctls and verifies by re-reading.
//
// For a concrete interface it writes net.ipv4.conf.all.*, net.ipv4.conf.default.*
// and net.ipv4.conf.<iface>.*. For scope "all" it writes the all/default scopes
// plus every concrete interface enumerated from /sys/class/net.
//
// Each sysctl is applied, then re-read back from /proc/sys/net/ipv4/conf/<scope>/<k>
// and its value confirmed; any failure lists the exact scope/param that failed.
func ApplyArpGuard(r execx.Runner, iface string) error {
	if strings.TrimSpace(iface) == "" {
		return fmt.Errorf("iface required")
	}
	scopes := []string{"all", "default"}
	if iface != "all" {
		scopes = append(scopes, iface)
	} else {
		// enumerate real interfaces from /sys/class/net
		if out, err := r.Run("sh", "-c", "ls /sys/class/net"); err != nil {
			// fall back to all+default only if listing fails
		} else {
			for _, l := range strings.Fields(strings.TrimSpace(out)) {
				if l != "" && l != "lo" {
					scopes = append(scopes, l)
				}
			}
		}
	}
	var failed []string
	pairs := []struct{ k, val string }{
		{"arp_ignore", "1"},
		{"arp_announce", "2"},
	}
	for _, scope := range scopes {
		for _, p := range pairs {
			key := "net.ipv4.conf." + scope + "." + p.k
			setCmd := fmt.Sprintf("sysctl -w %s=%s", key, p.val)
			if _, err := r.Run("sh", "-c", setCmd); err != nil {
				failed = append(failed, fmt.Sprintf("set %s: %v", key, err))
				continue
			}
			cur, err := r.Run("sysctl", "-n", key)
			if err != nil {
				failed = append(failed, fmt.Sprintf("verify-read %s: %v", key, err))
				continue
			}
			if strings.TrimSpace(cur) != p.val {
				failed = append(failed, fmt.Sprintf("verify %s: got %q want %q", key, cur, p.val))
			}
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("ARP guard apply failed (%s): %s", strings.Join(scopes, ","), strings.Join(failed, "; "))
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
