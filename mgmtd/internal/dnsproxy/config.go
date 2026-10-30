package dnsproxy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"strconv"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Upstream is a recursive DNS server dnsmasq forwards to.
type Upstream struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	// Source is the source IP address to use when querying this upstream.
	// When set, dnsmasq uses `server=upstream@source` syntax,
	// useful for multi-WAN setups where different WAN lines
	// should use different source IPs for DNS queries.
	Source string `json:"source"`
}

// HostEntry maps one IP to one or more names for local resolution.
type HostEntry struct {
	IP    string   `json:"ip"`
	Names []string `json:"names"`
}

// Config is the whole DNS proxy configuration.
type Config struct {
	Upstreams    []Upstream  `json:"upstreams"`
	Hosts        []HostEntry `json:"hosts"`
	CacheSize    int         `json:"cache_size"`
	DomainNeeded bool        `json:"domain_needed"`
	BogusPriv    bool        `json:"bogus_priv"`
	Enabled      bool        `json:"enabled"`
}

// DefaultConfPath is where the rendered dnsmasq config is written.
const DefaultConfPath = "/etc/dnsmasq.d/router-re-dnsproxy.conf"

// nameRe is the strict allowlist for a host name passed to Resolve. Anything
// outside it is rejected before it can reach a shell, which is what keeps
// `getent hosts` / `nslookup` from being a command-injection vector.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

// UpstreamPort returns the effective port for an upstream: 53 when unset.
func UpstreamPort(p int) int {
	if p == 0 {
		return 53
	}
	return p
}

// Validate checks the configuration for structural correctness.
func Validate(c Config) error {
	if len(c.Upstreams) == 0 && c.Enabled {
		return fmt.Errorf("at least one upstream is required when enabled")
	}
	for i, u := range c.Upstreams {
		if net.ParseIP(u.Address) == nil {
			return fmt.Errorf("upstream %d: invalid address %q", i, u.Address)
		}
		if u.Port != 0 && (u.Port < 1 || u.Port > 65535) {
			return fmt.Errorf("upstream %d: port must be 1-65535, got %d", i, u.Port)
		}
	}
	if c.CacheSize < 0 || c.CacheSize > 1000000 {
		return fmt.Errorf("cache_size must be 0-1000000, got %d", c.CacheSize)
	}
	for i, h := range c.Hosts {
		if net.ParseIP(h.IP) == nil {
			return fmt.Errorf("host %d: invalid ip %q", i, h.IP)
		}
		if len(h.Names) == 0 {
			return fmt.Errorf("host %d: at least one name is required", i)
		}
		for _, n := range h.Names {
			n = strings.TrimSpace(n)
			if n == "" {
				return fmt.Errorf("host %d: name must not be empty", i)
			}
			if strings.ContainsAny(n, " ,") {
				return fmt.Errorf("host %d: name must not contain space or comma: %q", i, n)
			}
			if len(n) > 253 {
				return fmt.Errorf("host %d: name longer than 253 chars", i)
			}
		}
	}
	return nil
}

// RenderDnsmasq renders the dnsmasq configuration for this config.
//
// 关于 address= 的语义（重要）：`address=/<name>/<ip>` 会把该域名（及其所有
// 子域）的**所有**查询固定返回这个 IP —— 效果等同于本地解析，但作用在
// dnsmasq 内部，不需要写 /etc/hosts。它不区分 A/AAAA：同一条规则同时覆盖
// IPv4 与 IPv6 查询。若只想映射单个地址族，用 `host-record=` 或 hosts 文件。
func RenderDnsmasq(c Config) string {
	var b strings.Builder
	pfx := ""
	if !c.Enabled {
		pfx = "# "
	}
	b.WriteString(pfx + "no-resolv\n")
	for _, u := range c.Upstreams {
		server := strings.TrimSpace(u.Address)
		if u.Port != 0 {
			server += "#" + strconv.Itoa(UpstreamPort(u.Port))
		} else {
			server += "#53"
		}
		if u.Source != "" {
			server += "@" + u.Source
		}
		fmt.Fprintf(&b, "%sserver=%s\n", pfx, server)
	}
	fmt.Fprintf(&b, "%scache-size=%d\n", pfx, c.CacheSize)
	if c.DomainNeeded {
		b.WriteString(pfx + "domain-needed\n")
	}
	if c.BogusPriv {
		b.WriteString(pfx + "bogus-priv\n")
	}
	for _, h := range c.Hosts {
		for _, n := range h.Names {
			fmt.Fprintf(&b, "%saddress=/%s/%s\n", pfx, strings.TrimSpace(n), strings.TrimSpace(h.IP))
		}
	}
	return b.String()
}

// RenderHosts renders an /etc/hosts style mapping.
//
// 与 RenderDnsmasq 中 address= 的区别：
//   - hosts 文件由 glibc/nss 在**解析链最前端**直接查表，只对本机进程生效，
//     且只映射名字 -> IP（不参与转发、不影响其他客户端）；
//   - address=/name/ip 是 dnsmasq 的转发规则，对所有经过该 DNS 的客户端生效，
//     并且会拦截该名字的所有查询（包括 A 与 AAAA）。
//
// 两者可以同时存在，但语义不同，不要混用。
func RenderHosts(c Config) string {
	var b strings.Builder
	pfx := ""
	if !c.Enabled {
		pfx = "# "
	}
	for _, h := range c.Hosts {
		names := make([]string, 0, len(h.Names))
		for _, n := range h.Names {
			names = append(names, strings.TrimSpace(n))
		}
		fmt.Fprintf(&b, "%s%s %s\n", pfx, strings.TrimSpace(h.IP), strings.Join(names, " "))
	}
	return b.String()
}

// Apply validates the config, writes it to confPath and restarts dnsmasq.
// Both restart paths are attempted and both errors are reported when neither
// works, so an operator can tell "service missing" from "no permission".
func Apply(r execx.Runner, c Config, confPath string) error {
	if err := Validate(c); err != nil {
		return err
	}
	if confPath == "" {
		confPath = DefaultConfPath
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(confPath), err)
	}
	if err := os.WriteFile(confPath, []byte(RenderDnsmasq(c)), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	sysErr := error(nil)
	if _, err := r.Run("systemctl", "restart", "dnsmasq"); err == nil {
		return nil
	} else {
		sysErr = err
	}
	initErr := error(nil)
	if _, err := r.Run("/etc/init.d/dnsmasq", "restart"); err == nil {
		return nil
	} else {
		initErr = err
	}
	return fmt.Errorf("配置已写入 %s，但 dnsmasq 未重启：systemctl: %v；init.d: %v", confPath, sysErr, initErr)
}

// Resolve looks a name up on the local resolver and returns the IPs found.
//
// name is restricted to [a-zA-Z0-9.-] before it reaches any command: the value
// is passed as an argv element (never through a shell), but the allowlist keeps
// a hostile caller from smuggling shell metacharacters into the argument list
// and makes the rejection explicit instead of relying on exec quoting.
func Resolve(r execx.Runner, name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("name %q contains characters outside [a-zA-Z0-9.-]", name)
	}
	// getent is preferred: it consults nsswitch (hosts file + dnsmasq) and
	// prints one "IP name" line per address. nslookup is the fallback for
	// images without getent.
	if out, err := r.Run("getent", "hosts", name); err == nil {
		ips := parseGetent(out)
		if len(ips) > 0 {
			return ips, nil
		}
	}
	if out, err := r.Run("nslookup", name); err == nil {
		ips := parseNslookup(out)
		if len(ips) > 0 {
			return ips, nil
		}
	}
	return nil, fmt.Errorf("no address found for %q", name)
}

// parseGetent extracts the first field of each non-empty getent hosts line.
func parseGetent(out string) []string {
	var ips []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if ip := net.ParseIP(f[0]); ip != nil {
			ips = append(ips, ip.String())
		}
	}
	return ips
}

// parseNslookup extracts Address:/Address lines that hold a real IP.
func parseNslookup(out string) []string {
	var ips []string
	for _, line := range strings.Split(out, "\n") {
		idx := strings.LastIndex(line, ":")
		if idx < 0 {
			continue
		}
		v := strings.TrimSpace(line[idx+1:])
		if ip := net.ParseIP(v); ip != nil {
			ips = append(ips, ip.String())
		}
	}
	return ips
}
