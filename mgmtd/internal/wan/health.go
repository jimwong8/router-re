package wan

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Health captures reachability and latency for one WAN gateway.
type Health struct {
	Name      string  `json:"name"`
	Interface string  `json:"interface"`
	Gateway   string  `json:"gateway"`
	Alive     bool    `json:"alive"`
	LatencyMs int64   `json:"latency_ms"`
	LossPct   float64 `json:"loss_pct"`
	CheckedAt int64   `json:"checked_at"`
}

// Checker performs concurrent ICMP health checks against WAN gateways.
type Checker struct {
	R     execx.Runner
	Count int
}

// Check pings every link concurrently and returns results in the same order.
func (c *Checker) Check(links []WANLink) []Health {
	count := c.Count
	if count <= 0 {
		count = 3
	}
	path, err := execx.Lookup("ping")
	results := make([]Health, len(links))
	var wg sync.WaitGroup
	for i, l := range links {
		wg.Add(1)
		go func(i int, l WANLink) {
			defer wg.Done()
			h := Health{
				Name:      l.Name,
				Interface: l.Interface,
				Gateway:   l.Gateway,
				CheckedAt: time.Now().Unix(),
			}
			if err != nil {
				// ping unavailable: mark dead with 100% loss.
				h.Alive = false
				h.LatencyMs = 0
				h.LossPct = 100
				results[i] = h
				return
			}
			out, _ := c.R.Run(path, "-c", strconv.Itoa(count), "-W", "2", l.Gateway)
			results[i] = parsePingStats(h, out)
		}(i, l)
	}
	wg.Wait()
	return results
}

// parsePingStats extracts transmit/receive counts and avg RTT from Linux ping
// output. A failed ping (nonzero exit) still yields "-c 3" style partial stat
// lines; we tolerate empty output by returning 100% loss.
func parsePingStats(h Health, out string) Health {
	tx, rx, avg := parseTxRxAvg(out)
	if tx > 0 {
		h.LossPct = float64(tx-rx) / float64(tx) * 100
	}
	h.Alive = rx > 0
	h.LatencyMs = int64(avg)
	if h.LatencyMs < 0 {
		h.LatencyMs = 0
	}
	if tx == 0 {
		h.LossPct = 100
		h.Alive = false
	}
	return h
}

// parseTxRxAvg pulls "N packets transmitted", "M received" and the avg value
// from "rtt min/avg/max/mdev = ..." into integers/float.
func parseTxRxAvg(s string) (tx, rx int, avg float64) {
	if i := strings.Index(s, "packets transmitted"); i >= 0 {
		seg := strings.TrimSpace(s[:i])
		if n, ok := lastLeadingInt(seg); ok {
			tx = n
		}
	}
	if i := strings.Index(s, "received"); i >= 0 {
		seg := strings.TrimSpace(s[:i])
		if n, ok := lastLeadingInt(seg); ok {
			rx = n
		}
	}
	if i := strings.Index(s, "min/avg/max/mdev"); i >= 0 {
		rest := s[i:]
		if j := strings.Index(rest, "="); j >= 0 {
			val := strings.TrimSpace(rest[j+1:])
			if k := strings.Index(val, "/"); k >= 0 {
				if f, err := strconv.ParseFloat(strings.TrimSpace(val[:k]), 64); err == nil {
					avg = f
				}
			}
		}
	}
	return tx, rx, avg
}

// lastLeadingInt finds the trailing integer in a "seg" that precedes the
// keyword, e.g. "3 packets" -> trims the keyword-laden tail: we search the
// end of the segment for the last number.
func lastLeadingInt(s string) (int, bool) {
	// The text before "packets transmitted" looks like "...: 3" or "3".
	fields := strings.Fields(s)
	for i := len(fields) - 1; i >= 0; i-- {
		clean := strings.Trim(fields[i], ",:")
		if n, err := strconv.Atoi(clean); err == nil {
			return n, true
		}
	}
	return 0, false
}
