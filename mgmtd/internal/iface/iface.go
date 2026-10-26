package iface

import (
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Config describes a network interface configuration.
type Config struct {
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Netmask  string   `json:"netmask"`
	Gateway  string   `json:"gateway"`
	DNS      []string `json:"dns"`
}

// Applier applies interface settings using an execx.Runner.
type Applier struct {
	R execx.Runner
}

func netmaskToPrefix(netmask string) (int, error) {
	// Simple parser for dotted-decimal netmask (e.g., 255.255.255.0) -> prefix length.
	parts := strings.Split(netmask, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid netmask: %s", netmask)
	}
	bits := 0
	for _, s := range parts {
		var octet int
		if _, err := fmt.Sscanf(s, "%d", &octet); err != nil {
			return 0, fmt.Errorf("invalid netmask octet: %s", s)
		}
		switch octet {
		case 255:
			bits += 8
		case 254:
			bits += 7
		case 252:
			bits += 6
		case 248:
			bits += 5
		case 240:
			bits += 4
		case 224:
			bits += 3
		case 192:
			bits += 2
		case 128:
			bits += 1
		case 0:
			bits += 0
		default:
			return 0, fmt.Errorf("invalid netmask octet value: %d", octet)
		}
	}
	return bits, nil
}

// Apply applies the interface configuration using the bound Runner.
func (a *Applier) Apply(cfg Config) error {
	if cfg.Name == "" {
		return fmt.Errorf("interface name is required")
	}
	if cfg.Address == "" {
		return fmt.Errorf("interface address is required")
	}
	prefix, err := netmaskToPrefix(cfg.Netmask)
	if err != nil {
		prefix = 24 // default fallback for simplicity
	}

	path, err := execx.Lookup("ip")
	if err != nil {
		return fmt.Errorf("ip not found: %w", err)
	}

	_, err = a.R.Run(path, "addr", "replace", fmt.Sprintf("%s/%d", cfg.Address, prefix), "dev", cfg.Name)
	if err != nil {
		return fmt.Errorf("ip addr replace failed: %w", err)
	}

	if cfg.Gateway != "" {
		_, err = a.R.Run(path, "route", "replace", "default", "via", cfg.Gateway, "dev", cfg.Name)
		if err != nil {
			return fmt.Errorf("ip route replace default failed: %w", err)
		}
	}
	return nil
}

// Status returns interface info using ip -j addr show <name>.
func (a *Applier) Status(name string) (map[string]interface{}, error) {
	path, err := execx.Lookup("ip")
	if err != nil {
		return nil, fmt.Errorf("ip not found: %w", err)
	}
	out, err := a.R.Run(path, "-j", "addr", "show", name)
	if err != nil {
		return nil, fmt.Errorf("ip addr show %s failed: %w", name, err)
	}
	// Minimal parsing: return address and netmask extracted via strings.
	res := map[string]interface{}{
		"name":    name,
		"raw":     out,
	}
	if strings.Contains(out, `"local"`) {
		res["has_local"] = true
	}
	return res, nil
}
