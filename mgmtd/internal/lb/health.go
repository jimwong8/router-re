package lb

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jimwong/router-re/mgmtd/internal/execx"
)

// Status is the health snapshot of one member.
type Status struct {
	Name      string  `json:"name"`
	Up        bool    `json:"up"`
	FailCount int     `json:"fail_count"`
	RiseCount int     `json:"rise_count"`
	LastCheck int64   `json:"last_check"`
	LatencyMs float64 `json:"latency_ms"`
	LastError string  `json:"last_error"`
}

// Checker probes each member with a bound-interface ping and applies flapping
// suppression: a member only goes Down after FailThreshold consecutive failures
// and only returns Up after RiseThreshold consecutive successes. This matters
// because a single transient packet loss would otherwise flip every member and
// churn the routing table.
type Checker struct {
	mu     sync.RWMutex
	status map[string]Status
	cfg    HealthCheck
}

// NewChecker creates a Checker with all members initially up.
func NewChecker(cfg HealthCheck) *Checker {
	return &Checker{status: map[string]Status{}, cfg: cfg}
}

// Check pings the target through the member's interface and updates state.
func (c *Checker) Check(r execx.Runner, m Member) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.status[m.Name]
	if st.Name == "" {
		st = Status{Name: m.Name, Up: true}
	}
	st.LastCheck = time.Now().Unix()
	// ping -I binds the probe to the member's interface — otherwise the probe
	// follows the default route and cannot distinguish the link under test.
	args := []string{"-c", "1", "-W", fmt.Sprintf("%d", c.cfg.TimeoutSec), "-I", m.Iface, c.cfg.Target}
	out, err := r.Run("ping", args...)
	if err != nil {
		st.LastError = err.Error()
		st.LatencyMs = 0
		st.RiseCount = 0
		st.FailCount++
		if st.FailCount >= c.cfg.FailThreshold {
			st.Up = false
		}
		c.status[m.Name] = st
		return st
	}
	// parse rtt min/avg/max = a/b/c ms
	if m2 := strings.Contains(out, "min/avg/max"); m2 {
		// try to extract the first rtt value
		if i := strings.Index(out, "rtt min/avg/max"); i >= 0 {
			seg := out[i:]
			if j := strings.Index(seg, " = "); j >= 0 {
				seg = seg[j+3:]
				if k := strings.Index(seg, " "); k >= 0 {
					seg = seg[:k]
				}
				if v, perr := parseMS(seg); perr == nil {
					st.LatencyMs = v
				}
			}
		}
	}
	st.LastError = ""
	st.FailCount = 0
	st.RiseCount++
	if st.RiseCount >= c.cfg.RiseThreshold {
		st.Up = true
	}
	c.status[m.Name] = st
	return st
}

func parseMS(s string) (float64, error) {
	v := 0.0
	n := 0
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			v = v*10 + float64(ch-'0')
			n++
		} else if ch == '.' {
			// ignore fractional here; good enough for latency display
		} else {
			break
		}
	}
	_ = n
	return v, nil
}

// Snapshot returns all statuses sorted by name.
func (c *Checker) Snapshot() []Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]Status, 0, len(c.status))
	for _, s := range c.status {
		res = append(res, s)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res
}
