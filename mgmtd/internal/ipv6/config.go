package ipv6

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Address is an IPv6 address bound to an interface.
type Address struct {
	Iface     string `json:"iface"`
	Addr      string `json:"addr"`
	PrefixLen int    `json:"prefix_len"`
}

// RAConfig controls radvd Router Advertisement emission.
type RAConfig struct {
	Iface             string `json:"iface"`
	Enabled           bool   `json:"enabled"`
	ManagedFlag       bool   `json:"managed_flag"`
	OtherFlag         bool   `json:"other_flag"`
	MaxInterval       int    `json:"max_interval"`
	Prefix            string `json:"prefix"`
	PrefixLen         int    `json:"prefix_len"`
	ValidLifetime     int    `json:"valid_lifetime"`
	PreferredLifetime int    `json:"preferred_lifetime"`
}

// ValidateAddr checks an IPv6 address.
func ValidateAddr(a Address) error {
	if strings.TrimSpace(a.Iface) == "" {
		return fmt.Errorf("iface required")
	}
	ip := net.ParseIP(strings.TrimSpace(a.Addr))
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("addr must be an IPv6 address, got %q", a.Addr)
	}
	if a.PrefixLen < 1 || a.PrefixLen > 128 {
		return fmt.Errorf("prefix_len must be in [1,128], got %d", a.PrefixLen)
	}
	return nil
}

// ValidateRA checks radvd config fields.
func ValidateRA(c RAConfig) error {
	if strings.TrimSpace(c.Iface) == "" {
		return fmt.Errorf("iface required")
	}
	if c.MaxInterval < 4 || c.MaxInterval > 1800 {
		return fmt.Errorf("max_interval must be in [4,1800], got %d", c.MaxInterval)
	}
	if c.Enabled {
		p := net.ParseIP(strings.TrimSpace(c.Prefix))
		if p == nil || p.To4() != nil {
			return fmt.Errorf("prefix must be an IPv6 network when enabled, got %q", c.Prefix)
		}
	}
	if c.PrefixLen < 1 || c.PrefixLen > 128 {
		return fmt.Errorf("prefix_len must be in [1,128], got %d", c.PrefixLen)
	}
	if c.ValidLifetime <= 0 || c.PreferredLifetime <= 0 {
		return fmt.Errorf("valid_lifetime and preferred_lifetime must be > 0")
	}
	if c.ValidLifetime < c.PreferredLifetime {
		return fmt.Errorf("valid_lifetime (%d) must be >= preferred_lifetime (%d)", c.ValidLifetime, c.PreferredLifetime)
	}
	return nil
}

// ApplyAddr idempotently adds/replaces an IPv6 address on an interface.
func ApplyAddr(r execx.Runner, a Address) error {
	if err := ValidateAddr(a); err != nil {
		return err
	}
	if _, err := r.Run("ip", "-6", "addr", "replace", fmt.Sprintf("%s/%d", a.Addr, a.PrefixLen), "dev", a.Iface); err != nil {
		return err
	}
	return nil
}

// DeleteAddr removes an IPv6 address, ignoring "not present" style errors.
func DeleteAddr(r execx.Runner, a Address) error {
	if err := ValidateAddr(a); err != nil {
		return err
	}
	_, err := r.Run("ip", "-6", "addr", "del", fmt.Sprintf("%s/%d", a.Addr, a.PrefixLen), "dev", a.Iface)
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	for _, frag := range []string{"cannot assign requested address", "no such file", "address not found", "cannot find device"} {
		if strings.Contains(msg, frag) {
			return nil
		}
	}
	return err
}

// firstIPv6 extracts (local, prefixlen) from a parsed addr_info entry.
type addrInfoEntry struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

// ListAddr enumerates global IPv6 addresses on all interfaces, skipping
// link-local (fe80::) and loopback (::1).
func ListAddr(r execx.Runner) ([]Address, error) {
	out, err := r.Run("ip", "-j", "-6", "addr", "show")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return []Address{}, nil
	}
	var raw []struct {
		Ifname   string          `json:"ifname"`
		AddrInfo []addrInfoEntry `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse ip -j -6 addr show: %w", err)
	}
	res := []Address{}
	for _, iface := range raw {
		for _, ai := range iface.AddrInfo {
			if ai.Family != "inet6" {
				continue
			}
			l := strings.ToLower(ai.Local)
			if strings.HasPrefix(l, "fe80:") || l == "::1" {
				continue
			}
			if ai.Local == "" {
				continue
			}
			res = append(res, Address{Iface: iface.Ifname, Addr: ai.Local, PrefixLen: ai.PrefixLen})
		}
	}
	return res, nil
}

// RenderRadvd produces a radvd.conf fragment for the RA config.
func RenderRadvd(c RAConfig) string {
	pfx := ""
	if !c.Enabled {
		pfx = "# "
	}
	managed := "off"
	if c.ManagedFlag {
		managed = "on"
	}
	other := "off"
	if c.OtherFlag {
		other = "on"
	}
	minInt := float64(c.MaxInterval) * 0.33
	var b strings.Builder
	fmt.Fprintf(&b, "%sinterface %s {\n", pfx, c.Iface)
	fmt.Fprintf(&b, "%s\tAdvSendAdvert on;\n", pfx)
	fmt.Fprintf(&b, "%s\tMinRtrAdvInterval %.1f;\n", pfx, minInt)
	fmt.Fprintf(&b, "%s\tMaxRtrAdvInterval %d;\n", pfx, c.MaxInterval)
	fmt.Fprintf(&b, "%s\tAdvManagedFlag %s;\n", pfx, managed)
	fmt.Fprintf(&b, "%s\tAdvOtherConfigFlag %s;\n", pfx, other)
	fmt.Fprintf(&b, "%s\tprefix %s/%d\n", pfx, c.Prefix, c.PrefixLen)
	if !c.Enabled {
		// continuation lines inside a commented block still need the prefix; we
		// comment each line so the block reads as comments.
		fmt.Fprintf(&b, "%s\t{\n", pfx)
		fmt.Fprintf(&b, "%s\t\tAdvOnLink on;\n", pfx)
		fmt.Fprintf(&b, "%s\t\tAdvAutonomous on;\n", pfx)
		fmt.Fprintf(&b, "%s\t\tAdvValidLifetime %d;\n", pfx, c.ValidLifetime)
		fmt.Fprintf(&b, "%s\t\tAdvPreferredLifetime %d;\n", pfx, c.PreferredLifetime)
		fmt.Fprintf(&b, "%s\t};\n", pfx)
	} else {
		fmt.Fprintf(&b, "\t{\n")
		fmt.Fprintf(&b, "\t\tAdvOnLink on;\n")
		fmt.Fprintf(&b, "\t\tAdvAutonomous on;\n")
		fmt.Fprintf(&b, "\t\tAdvValidLifetime %d;\n", c.ValidLifetime)
		fmt.Fprintf(&b, "\t\tAdvPreferredLifetime %d;\n", c.PreferredLifetime)
		fmt.Fprintf(&b, "\t};\n")
	}
	fmt.Fprintf(&b, "%s};\n", pfx)
	return b.String()
}

// ApplyRA validates, writes the radvd config and restarts radvd.
func ApplyRA(r execx.Runner, c RAConfig, confPath string) error {
	if err := ValidateRA(c); err != nil {
		return err
	}
	if confPath == "" {
		confPath = "/etc/radvd.conf"
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderRadvd(c)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	var err1, err2 error
	if _, err1 = r.Run("systemctl", "restart", "radvd"); err1 == nil {
		return nil
	}
	if _, err2 = r.Run("/etc/init.d/radvd", "restart"); err2 == nil {
		return nil
	}
	return fmt.Errorf("radvd restart failed: systemctl(%v); init.d(%v)", err1, err2)
}

// Status reports IPv6 runtime state.
func Status(r execx.Runner) (map[string]string, error) {
	res := map[string]string{}
	// forwarding from /proc
	fwd, err := os.ReadFile("/proc/sys/net/ipv6/conf/all/forwarding")
	if err != nil {
		res["forwarding"] = fmt.Sprintf("read error: %v", err)
	} else {
		res["forwarding"] = strings.TrimSpace(string(fwd))
	}
	// ipv6_enabled: any of the six sysctls >0, or simply presence of ip6tables
	if _, err := r.Run("ip", "-6", "addr", "show"); err == nil {
		res["ipv6_enabled"] = "yes"
	} else {
		res["ipv6_enabled"] = "no"
	}
	addrs, err := ListAddr(r)
	if err != nil {
		res["addr_count"] = fmt.Sprintf("err: %v", err)
	} else {
		res["addr_count"] = fmt.Sprintf("%d", len(addrs))
	}
	if _, err := r.Run("systemctl", "is-active", "radvd"); err == nil {
		res["radvd_running"] = "yes"
	} else {
		res["radvd_running"] = "no"
	}
	return res, nil
}
