package stats

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sample is a point-in-time snapshot of host metrics.
type Sample struct {
	TS             int64   `json:"ts"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemUsedMB      int     `json:"mem_used_mb"`
	MemTotalMB     int     `json:"mem_total_mb"`
	Load1          float64 `json:"load1"`
	Load5          float64 `json:"load5"`
	Load15         float64 `json:"load15"`
	ConntrackCount int     `json:"conntrack_count"`
	RXBytes        uint64  `json:"rx_bytes"`
	TXBytes        uint64  `json:"tx_bytes"`
}

// Collect gathers one host metrics sample. CPU percent is measured by
// reading /proc/stat twice 100ms apart and computing the delta ratio.
func Collect() (Sample, error) {
	var s Sample
	s.TS = time.Now().Unix()

	// CPU usage: delta between two /proc/stat reads.
	c1, err := readCPUStats()
	if err != nil {
		return s, err
	}
	time.Sleep(100 * time.Millisecond)
	c2, err := readCPUStats()
	if err != nil {
		return s, err
	}
	total := c2.total - c1.total
	idle := c2.idle - c1.idle
	if total > 0 {
		s.CPUPercent = float64(total-idle) / float64(total) * 100
	}

	// Memory from /proc/meminfo.
	memTotal, memAvail, err := readMemInfo()
	if err == nil {
		s.MemTotalMB = memTotal / 1024
		used := memTotal - memAvail
		if used < 0 {
			used = 0
		}
		s.MemUsedMB = used / 1024
	}

	// Load averages.
	s.Load1, s.Load5, s.Load15 = readLoadAvg()

	// Conntrack count; missing file is fine (kernel may not have nf_conntrack).
	if n, ok := readConntrack(); ok {
		s.ConntrackCount = n
	}

	// Network traffic across all non-loopback interfaces.
	s.RXBytes, s.TXBytes = readNetDevTotals()
	return s, nil
}

type cpuStats struct {
	total uint64
	idle  uint64
}

func readCPUStats() (cpuStats, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuStats{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			break
		}
		// fields[0] = "cpu"; 1..N are jiffies: user nice system idle iowait irq softirq steal.
		var vals []uint64
		for _, fld := range fields[1:] {
			v, err := strconv.ParseUint(fld, 10, 64)
			if err != nil {
				return cpuStats{}, err
			}
			vals = append(vals, v)
		}
		total := uint64(0)
		for _, v := range vals {
			total += v
		}
		idle := vals[3] // idle
		if len(vals) > 4 {
			idle += vals[4] // iowait is treated as idle-ish for our purpose
		}
		return cpuStats{total: total, idle: idle}, nil
	}
	return cpuStats{}, os.ErrNotExist
}

func readMemInfo() (total, avail int, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseKb(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			avail = parseKb(line)
		}
	}
	if total == 0 {
		return 0, 0, os.ErrNotExist
	}
	return total, avail, nil
}

func parseKb(line string) int {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return v
}

func readLoadAvg() (l1, l5, l15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	l1, _ = strconv.ParseFloat(fields[0], 64)
	l5, _ = strconv.ParseFloat(fields[1], 64)
	l15, _ = strconv.ParseFloat(fields[2], 64)
	return
}

func readConntrack() (int, bool) {
	data, err := os.ReadFile("/proc/sys/net/netfilter/nf_conntrack_count")
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return v, true
}

// readNetDevTotals sums rx/tx bytes across all interfaces except loopback.
func readNetDevTotals() (rx, tx uint64) {
	ifaces, err := readNetDev()
	if err != nil {
		return 0, 0
	}
	for _, ifc := range ifaces {
		if ifc.Name == "lo" {
			continue
		}
		rx += ifc.RXBytes
		tx += ifc.TXBytes
	}
	return rx, tx
}

// ReadInterfaces returns per-interface counters (excluding loopback).
func ReadInterfaces() ([]IfaceStats, error) {
	ifaces, err := readNetDev()
	if err != nil {
		return nil, err
	}
	// Keep only non-loopback interfaces.
	out := make([]IfaceStats, 0, len(ifaces))
	for _, ifc := range ifaces {
		if ifc.Name == "lo" {
			continue
		}
		out = append(out, ifc)
	}
	return out, nil
}

// IfaceStats holds per-interface counters from /proc/net/dev.
type IfaceStats struct {
	Name      string `json:"name"`
	RXBytes   uint64 `json:"rx_bytes"`
	RXPackets uint64 `json:"rx_packets"`
	RXErrors  uint64 `json:"rx_errors"`
	TXBytes   uint64 `json:"tx_bytes"`
	TXPackets uint64 `json:"tx_packets"`
	TXErrors  uint64 `json:"tx_errors"`
}

// readNetDev parses /proc/net/dev into per-interface counters (skip lo).
func readNetDev() ([]IfaceStats, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []IfaceStats
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ":") {
			continue // header lines
		}
		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		nums := strings.Fields(parts[1])
		if len(nums) < 16 {
			continue
		}
		u := func(i int) uint64 {
			v, _ := strconv.ParseUint(nums[i], 10, 64)
			return v
		}
		out = append(out, IfaceStats{
			Name:      name,
			RXBytes:   u(0),
			RXPackets: u(1),
			RXErrors:  u(2),
			TXBytes:   u(8),
			TXPackets: u(9),
			TXErrors:  u(10),
		})
	}
	return out, nil
}

// Ring is a thread-safe fixed-size ring buffer of samples.
type Ring struct {
	mu   sync.RWMutex
	buf  []Sample
	size int
	pos  int // next write position
}

// NewRing creates a ring buffer holding up to size samples.
func NewRing(size int) *Ring {
	if size <= 0 {
		size = 1
	}
	return &Ring{
		buf:  make([]Sample, 0, size),
		size: size,
	}
}

// Add appends a sample, overwriting the oldest when full.
func (r *Ring) Add(s Sample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.size {
		r.buf = append(r.buf, s)
		return
	}
	r.buf[r.pos] = s
	r.pos = (r.pos + 1) % r.size
}

// Since returns samples with TS within the last minutes (>= now-minutes),
// in chronological order.
func (r *Ring) Since(minutes int) []Sample {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if minutes <= 0 {
		minutes = 60
	}
	cutoff := time.Now().Unix() - int64(minutes*60)
	// Build in chronological order starting from the oldest valid sample.
	out := make([]Sample, 0, len(r.buf))
	for i := 0; i < len(r.buf); i++ {
		idx := (r.pos + i) % len(r.buf)
		s := r.buf[idx]
		if s.TS >= cutoff {
			out = append(out, s)
		}
	}
	return out
}

// Latest returns the most recently added sample.
func (r *Ring) Latest() (Sample, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.buf) == 0 {
		return Sample{}, false
	}
	// The most recent write is at (pos-1) mod len when the ring is full;
	// otherwise the last element.
	var last Sample
	if len(r.buf) < r.size {
		last = r.buf[len(r.buf)-1]
	} else {
		idx := (r.pos - 1 + r.size) % r.size
		last = r.buf[idx]
	}
	return last, true
}
