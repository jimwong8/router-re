package wan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Applier applies policy-routing rules for WAN links using an execx.Runner.
// All commands run through execx.Lookup("ip") absolute paths.
type Applier struct {
	R execx.Runner
}

// Apply sets up one routing table per WAN link, an fwmark rule for each
// table (deduplicated via `ip rule show`), and the main-table default route
// on the first link.
func (a *Applier) Apply(links []WANLink) error {
	ip, err := execx.Lookup("ip")
	if err != nil {
		return fmt.Errorf("ip command not found: %w", err)
	}
	for i, l := range links {
		tbl := l.Table
		if tbl <= 0 {
			tbl = 100
		}
		gw := l.Gateway
		iface := l.Interface
		if iface == "" {
			iface = l.Name
		}
		if gw == "" {
			continue
		}

		// Per-link table default route (replace -> idempotent).
		if _, err := a.R.Run(ip, "route", "replace", "default", "via", gw, "dev", iface, "table", strconv.Itoa(tbl)); err != nil {
			return fmt.Errorf("ip route replace table %d failed: %w", tbl, err)
		}

		// fwmark rule with dedup: skip if the same rule already exists.
		exists, err := a.ruleExists(ip, tbl)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := a.R.Run(ip, "rule", "add", "fwmark", strconv.Itoa(tbl), "lookup", strconv.Itoa(tbl)); err != nil {
				return fmt.Errorf("ip rule add fwmark %d failed: %w", tbl, err)
			}
		}

		// First link becomes the main-table default route.
		if i == 0 {
			if _, err := a.R.Run(ip, "route", "replace", "default", "via", gw, "dev", iface); err != nil {
				return fmt.Errorf("ip route replace default (main) failed: %w", err)
			}
		}
	}
	return nil
}

// Cleanup removes the fwmark rules and flushes the per-link routing tables.
// Missing rules/tables are treated as success.
func (a *Applier) Cleanup(tables []int) error {
	ip, err := execx.Lookup("ip")
	if err != nil {
		return fmt.Errorf("ip command not found: %w", err)
	}
	for _, tbl := range tables {
		if tbl <= 0 {
			continue
		}
		if _, err := a.R.Run(ip, "rule", "del", "fwmark", strconv.Itoa(tbl), "lookup", strconv.Itoa(tbl)); err != nil {
			// ignore "no such rule" style errors
			_ = err
		}
		if _, err := a.R.Run(ip, "route", "flush", "table", strconv.Itoa(tbl)); err != nil {
			// ignore missing table errors
			_ = err
		}
	}
	return nil
}

// ruleExists reports whether an fwmark rule for the table already exists.
func (a *Applier) ruleExists(ip string, tbl int) (bool, error) {
	out, err := a.R.Run(ip, "rule", "show")
	if err != nil {
		return false, fmt.Errorf("ip rule show failed: %w", err)
	}
	// ip rule show output looks like:
	//   100:	from all	fwmark 0x64 lookup 100
	needle := fmt.Sprintf("fwmark 0x%x lookup %d", tbl, tbl)
	return strings.Contains(out, needle), nil
}
