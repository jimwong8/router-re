package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// hostRe matches the safe character set for host arguments.
// [a-zA-Z0-9.-] plus ':' for IPv6 literals.
var hostRe = regexp.MustCompile(`^[a-zA-Z0-9.:-]+$`)

// ValidateHost is a security-critical check: host strings are passed as
// arguments to external commands (ping / traceroute / dig). While exec.Command
// passes argv directly (no shell), this is the second line of defense:
// reject every shell metacharacter so even a future shell-wrapping change
// stays safe. Allowed: letters, digits, '.', '-', ':' (IPv6). The value must
// start and end with alphanumeric (no leading '-', which could be parsed as a
// flag), contain no whitespace, and be 1..253 chars.
func ValidateHost(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return fmt.Errorf("host required")
	}
	if len(h) > 253 {
		return fmt.Errorf("host too long: %d > 253", len(h))
	}
	if !hostRe.MatchString(h) {
		return fmt.Errorf("host contains forbidden characters (whitespace, shell metacharacters, or punctuation): %q", h)
	}
	first, last := h[0], h[len(h)-1]
	// must start/end with alphanumeric
	if !isAlnum(first) || !isAlnum(last) {
		return fmt.Errorf("host must start and end with a letter or digit: %q", h)
	}
	// reject leading '-' explicitly (flag injection)
	if strings.HasPrefix(h, "-") {
		return fmt.Errorf("host must not start with '-'")
	}
	// reject suspicious name segments: '.', '..', empty labels (e.g. "a..b")
	if strings.Contains(h, "..") {
		return fmt.Errorf("host must not contain '..'")
	}
	return nil
}

func isAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ValidateName is like ValidateHost but rejects ':' (IPv6) — used for DNS
// lookups where an IP literal is not a valid query name.
func ValidateName(name string) error {
	if strings.Contains(name, ":") {
		return fmt.Errorf("DNS name must not be an IPv6 literal: %q", name)
	}
	return ValidateHost(name)
}

// Result is the structured outcome of a diagnostic command.
type Result struct {
	Host       string            `json:"host,omitempty"`
	ResolvedIP string            `json:"resolved_ip,omitempty"`
	Sent       int               `json:"sent"`
	Received   int               `json:"received"`
	LossPct    float64           `json:"loss_pct"`
	MinMs      float64           `json:"min_ms"`
	AvgMs      float64           `json:"avg_ms"`
	MaxMs      float64           `json:"max_ms"`
	HopCount   int               `json:"hop_count,omitempty"`
	Records    []string          `json:"records,omitempty"`
	Raw        string            `json:"raw"`
	Extra      map[string]string `json:"extra,omitempty"`
}

// Ping runs ping against host. count must be 1..10 (default 4).
func Ping(r execx.Runner, host string, count int) (Result, error) {
	if err := ValidateHost(host); err != nil {
		return Result{}, err
	}
	if count == 0 {
		count = 4
	}
	if count < 1 || count > 10 {
		return Result{}, fmt.Errorf("count must be in [1,10], got %d", count)
	}
	isV6 := net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil
	args := []string{"-c", strconv.Itoa(count), "-W", "2"}
	if isV6 {
		args = append([]string{"-6"}, args...)
	}
	args = append(args, host)
	timeout := time.Duration(count*3)*time.Second + 5*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct {
		out string
		err error
	}, 1)
	go func() {
		out, err := r.Run("ping", args...)
		done <- struct {
			out string
			err error
		}{out, err}
	}()
	select {
	case <-ctx.Done():
		return Result{}, fmt.Errorf("ping timed out after %s", timeout)
	case res := <-done:
		return parsePing(res.out, res.err, host)
	}
}

func parsePing(out string, err error, host string) (Result, error) {
	res := Result{Host: host, Raw: out}
	if err != nil {
		// if the command itself is missing, surface that distinctly
		if strings.Contains(strings.ToLower(err.Error()), "executable file not found") ||
			strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return res, fmt.Errorf("ping not installed: %w", err)
		}
		// otherwise include raw output in the error for debugging
		return res, fmt.Errorf("ping failed: %v\n%s", err, out)
	}
	// loss: "X% packet loss"
	if m := regexp.MustCompile(`(\d+(?:\.\d+)?)% packet loss`).FindStringSubmatch(out); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			res.LossPct = v
		}
	}
	// sent/received: "N packets transmitted, M received"
	if m := regexp.MustCompile(`(\d+) packets transmitted, (\d+)(?: packets)? received`).FindStringSubmatch(out); len(m) == 3 {
		res.Sent, _ = strconv.Atoi(m[1])
		res.Received, _ = strconv.Atoi(m[2])
	}
	// rtt: "min/avg/max/mdev = a/b/c/d ms"
	if m := regexp.MustCompile(`min/avg/max(?:/mdev)? = ([\d.]+)/([\d.]+)/([\d.]+)`).FindStringSubmatch(out); len(m) == 4 {
		res.MinMs, _ = strconv.ParseFloat(m[1], 64)
		res.AvgMs, _ = strconv.ParseFloat(m[2], 64)
		res.MaxMs, _ = strconv.ParseFloat(m[3], 64)
	}
	// resolved IP: "PING host (1.2.3.4)" or "PING host (2001:db8::1)"
	if m := regexp.MustCompile(`PING [^ (]+ \(([^)]+)\)`).FindStringSubmatch(out); len(m) == 2 {
		res.ResolvedIP = m[1]
	}
	return res, nil
}

// Traceroute runs traceroute against host. maxHops 1..30 (default 15).
func Traceroute(r execx.Runner, host string, maxHops int) (Result, error) {
	if err := ValidateHost(host); err != nil {
		return Result{}, err
	}
	if maxHops == 0 {
		maxHops = 15
	}
	if maxHops < 1 || maxHops > 30 {
		return Result{}, fmt.Errorf("max_hops must be in [1,30], got %d", maxHops)
	}
	isV6 := net.ParseIP(host) != nil && net.ParseIP(host).To4() == nil
	args := []string{"-n", "-m", strconv.Itoa(maxHops), "-w", "2"}
	if isV6 {
		args = append([]string{"-6"}, args...)
	}
	args = append(args, host)
	timeout := time.Duration(maxHops)*time.Second + 10*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	runCmd := func(cmd string, a []string) (string, error) {
		done := make(chan struct {
			out string
			err error
		}, 1)
		go func() {
			out, err := r.Run(cmd, a...)
			done <- struct {
				out string
				err error
			}{out, err}
		}()
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("traceroute timed out after %s", timeout)
		case res := <-done:
			return res.out, res.err
		}
	}
	out, err := runCmd("traceroute", args)
	if err != nil {
		// fall back to tracepath
		tpArgs := []string{"-m", strconv.Itoa(maxHops)}
		if isV6 {
			tpArgs = append([]string{"-6"}, tpArgs...)
		}
		tpArgs = append(tpArgs, host)
		out2, err2 := runCmd("tracepath", tpArgs)
		if err2 != nil {
			// distinguish "command missing"
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file") {
				return Result{}, fmt.Errorf("traceroute not installed (and tracepath also unavailable)")
			}
			return Result{}, fmt.Errorf("traceroute failed: %v; tracepath failed: %v", err, err2)
		}
		// count hops from tracepath output lines
		hops := 0
		for _, line := range strings.Split(out2, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				if _, err := strconv.Atoi(strings.TrimSuffix(fields[0], ":")); err == nil {
					hops++
				}
			}
		}
		return Result{Host: host, Raw: out2, HopCount: hops, Extra: map[string]string{"tool": "tracepath"}}, nil
	}
	hops := 0
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			if _, err := strconv.Atoi(fields[0]); err == nil {
				hops++
			}
		}
	}
	return Result{Host: host, Raw: out, HopCount: hops, Extra: map[string]string{"tool": "traceroute"}}, nil
}

// DNSLookup resolves name (optionally via server). server empty → system DNS.
func DNSLookup(r execx.Runner, name, server string) (Result, error) {
	if err := ValidateName(name); err != nil {
		return Result{}, err
	}
	if server != "" {
		ip := net.ParseIP(server)
		if ip == nil {
			return Result{}, fmt.Errorf("invalid DNS server %q: must be an IP address", server)
		}
	}
	res := Result{Host: name}
	args := []string{"+short"}
	if server != "" {
		args = append(args, "@"+server)
	}
	args = append(args, name)
	out, err := r.Run("dig", args...)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file") {
			return Result{}, fmt.Errorf("dig not installed")
		}
		// fall back to getent hosts (system resolver) when server is empty
		if server == "" {
			out2, err2 := r.Run("getent", "hosts", name)
			if err2 != nil {
				return res, fmt.Errorf("dig failed: %v; getent failed: %v", err, err2)
			}
			records := []string{}
			for _, line := range strings.Split(strings.TrimSpace(out2), "\n") {
				if line != "" {
					records = append(records, line)
				}
			}
			res.Records = records
			res.Raw = out2
			return res, nil
		}
		return res, fmt.Errorf("dig failed: %v", err)
	}
	records := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			records = append(records, line)
		}
	}
	res.Records = records
	res.Raw = out
	return res, nil
}

// InterfaceStats returns per-interface counters from `ip -j -s link show`,
// skipping loopback.
func InterfaceStats(r execx.Runner) ([]map[string]string, error) {
	out, err := r.Run("ip", "-j", "-s", "link", "show")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return []map[string]string{}, nil
	}
	var raw []struct {
		Ifname  string `json:"ifname"`
		Stats64 struct {
			RxBytes   uint64 `json:"rx_bytes"`
			RxPackets uint64 `json:"rx_packets"`
			RxErrors  uint64 `json:"rx_errors"`
			RxDropped uint64 `json:"rx_dropped"`
			TxBytes   uint64 `json:"tx_bytes"`
			TxPackets uint64 `json:"tx_packets"`
			TxErrors  uint64 `json:"tx_errors"`
			TxDropped uint64 `json:"tx_dropped"`
		} `json:"stats64"`
		Stats struct {
			RxBytes   uint64 `json:"rx_bytes"`
			RxPackets uint64 `json:"rx_packets"`
			RxErrors  uint64 `json:"rx_errors"`
			RxDropped uint64 `json:"rx_dropped"`
			TxBytes   uint64 `json:"tx_bytes"`
			TxPackets uint64 `json:"tx_packets"`
			TxErrors  uint64 `json:"tx_errors"`
			TxDropped uint64 `json:"tx_dropped"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("parse ip -j -s link show: %w", err)
	}
	res := []map[string]string{}
	for _, iface := range raw {
		if iface.Ifname == "lo" {
			continue
		}
		st := iface.Stats64
		if st.RxBytes == 0 && st.TxBytes == 0 {
			st = iface.Stats
		}
		res = append(res, map[string]string{
			"iface":      iface.Ifname,
			"rx_bytes":   strconv.FormatUint(st.RxBytes, 10),
			"rx_packets": strconv.FormatUint(st.RxPackets, 10),
			"rx_errors":  strconv.FormatUint(st.RxErrors, 10),
			"rx_dropped": strconv.FormatUint(st.RxDropped, 10),
			"tx_bytes":   strconv.FormatUint(st.TxBytes, 10),
			"tx_packets": strconv.FormatUint(st.TxPackets, 10),
			"tx_errors":  strconv.FormatUint(st.TxErrors, 10),
			"tx_dropped": strconv.FormatUint(st.TxDropped, 10),
		})
	}
	return res, nil
}
