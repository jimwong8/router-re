// Package iperf runs an iperf3 client test and parses the JSON result.
//
// It follows the nettools2 convention: pure parameter construction + a Result
// summary; execution goes through an injected execx.Runner (real DefaultRunner
// in production, fake in tests). Arguments are always []string slices — never
// a shell string — so host/port/protocol inputs cannot be injected.
package iperf

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Config describes one iperf3 client run.
type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Duration int    `json:"duration"`
	Protocol string `json:"protocol"` // tcp | udp
}

// Result is the parsed outcome of one iperf3 run.
type Result struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Duration    int     `json:"duration"`
	Protocol    string  `json:"protocol"`
	Bandwidth   float64 `json:"bandwidth_bps"`
	Bytes       int64   `json:"bytes"`
	Retransmits int     `json:"retransmits"`
	LostPercent float64 `json:"lost_percent"`
	JitterMs    float64 `json:"jitter_ms"`
	OK          bool    `json:"ok"`
	Error       string  `json:"error,omitempty"`
}

// Validate checks the config. Defaults: Port=5201, Duration=10, Protocol=tcp.
func Validate(c *Config) error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if strings.HasPrefix(c.Host, "-") {
		return fmt.Errorf("host must not start with '-' (possible argument injection)")
	}
	if strings.ContainsAny(c.Host, " \t;|&$`><\"'\\\n\r") {
		return fmt.Errorf("host contains illegal characters")
	}
	if c.Port == 0 {
		c.Port = 5201
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", c.Port)
	}
	if c.Duration == 0 {
		c.Duration = 10
	}
	if c.Duration < 1 || c.Duration > 300 {
		return fmt.Errorf("duration must be 1-300 seconds, got %d", c.Duration)
	}
	p := strings.ToLower(strings.TrimSpace(c.Protocol))
	if p == "" {
		p = "tcp"
	}
	if p != "tcp" && p != "udp" {
		return fmt.Errorf("protocol must be tcp or udp, got %q", c.Protocol)
	}
	c.Protocol = p
	c.Host = strings.TrimSpace(c.Host)
	return nil
}

// RenderArgs builds the iperf3 argv slice (no shell string).
func RenderArgs(c Config) []string {
	args := []string{"-c", c.Host, "-p", fmt.Sprintf("%d", c.Port),
		"-t", fmt.Sprintf("%d", c.Duration), "-J"}
	if c.Protocol == "udp" {
		args = append(args, "-u")
	}
	return args
}

// iperfJSON is the subset of iperf3 -J output we care about.
type iperfJSON struct {
	End struct {
		SumSent struct {
			Bytes          int64   `json:"bytes"`
			BitsPerSecond  float64 `json:"bits_per_second"`
			Retransmits    int     `json:"retransmits"`
		} `json:"sum_sent"`
		SumReceived struct {
			Bytes         int64   `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_received"`
		Sum struct {
			Bytes         int64   `json:"bytes"`
			BitsPerSecond float64 `json:"bits_per_second"`
			LostPercent   float64 `json:"lost_percent"`
			JitterMs      float64 `json:"jitter_ms"`
		} `json:"sum"`
	} `json:"end"`
	Error string `json:"error"`
}

// Run executes iperf3 and returns a parsed Result.
func Run(r execx.Runner, c Config) Result {
	res := Result{
		Host:     c.Host,
		Port:     c.Port,
		Duration: c.Duration,
		Protocol: c.Protocol,
	}
	if err := Validate(&c); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Host = c.Host
	res.Port = c.Port
	res.Duration = c.Duration
	res.Protocol = c.Protocol

	args := RenderArgs(c)
	out, err := r.Run("iperf3", args...)
	if err != nil {
		// iperf3 not installed or run failed
		res.Error = err.Error()
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			res.Error = "iperf3 not available: " + err.Error()
		}
		return res
	}

	var parsed iperfJSON
	if jerr := json.Unmarshal([]byte(out), &parsed); jerr != nil {
		res.Error = fmt.Sprintf("iperf3 output parse failed: %v (raw: %s)", jerr, truncate(out, 200))
		return res
	}
	if parsed.Error != "" {
		res.Error = parsed.Error
		return res
	}

	// Prefer sum_received for bandwidth/bytes; fall back to sum.
	bw := parsed.End.SumReceived.BitsPerSecond
	bytes := parsed.End.SumReceived.Bytes
	if bw == 0 {
		bw = parsed.End.Sum.BitsPerSecond
	}
	if bytes == 0 {
		bytes = parsed.End.Sum.Bytes
	}
	res.Bandwidth = bw
	res.Bytes = bytes
	res.Retransmits = parsed.End.SumSent.Retransmits
	res.LostPercent = parsed.End.Sum.LostPercent
	res.JitterMs = parsed.End.Sum.JitterMs
	res.OK = true
	return res
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
