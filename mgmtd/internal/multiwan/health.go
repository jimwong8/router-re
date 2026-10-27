// Package multiwan 实现多 WAN 线路的健康检查状态机与选路。
package multiwan

import (
	"fmt"
	"net"
	"strings"
)

// Member 是一条 WAN 线路。
type Member struct {
	Name    string `json:"name"`
	Iface   string `json:"iface"`
	Gateway string `json:"gateway"`
	Weight  int    `json:"weight"`
	Enabled bool   `json:"enabled"`
}

// Probe 是一种健康检查方式。
type Probe struct {
	Type             string `json:"type"`
	Target           string `json:"target"`
	IntervalSec      int    `json:"interval_sec"`
	TimeoutSec       int    `json:"timeout_sec"`
	FailThreshold    int    `json:"fail_threshold"`
	RecoverThreshold int    `json:"recover_threshold"`
}

// ValidateMember 校验成员。
func ValidateMember(m Member) error {
	if m.Name == "" {
		return fmt.Errorf("member name must not be empty")
	}
	if m.Iface == "" {
		return fmt.Errorf("member iface must not be empty")
	}
	if m.Iface == "lo" {
		return fmt.Errorf("refusing loopback iface %q for WAN member", m.Iface)
	}
	if net.ParseIP(m.Gateway) == nil {
		return fmt.Errorf("gateway %q is not a valid IP", m.Gateway)
	}
	if m.Weight < 1 || m.Weight > 100 {
		return fmt.Errorf("weight must be 1-100, got %d", m.Weight)
	}
	return nil
}

// ValidateProbe 校验探测配置。
func ValidateProbe(p Probe) error {
	switch p.Type {
	case "icmp", "http", "dns":
	default:
		return fmt.Errorf("probe type must be icmp/http/dns, got %q", p.Type)
	}
	if strings.TrimSpace(p.Target) == "" {
		return fmt.Errorf("probe target must not be empty")
	}
	if p.IntervalSec < 1 || p.IntervalSec > 300 {
		return fmt.Errorf("interval_sec must be 1-300, got %d", p.IntervalSec)
	}
	if p.TimeoutSec < 1 || p.TimeoutSec > 30 {
		return fmt.Errorf("timeout_sec must be 1-30, got %d", p.TimeoutSec)
	}
	// TimeoutSec 必须小于 IntervalSec，否则检查会重叠。
	if p.TimeoutSec >= p.IntervalSec {
		return fmt.Errorf("timeout_sec (%d) must be < interval_sec (%d): otherwise probes overlap", p.TimeoutSec, p.IntervalSec)
	}
	if p.FailThreshold < 1 || p.FailThreshold > 10 {
		return fmt.Errorf("fail_threshold must be 1-10, got %d", p.FailThreshold)
	}
	if p.RecoverThreshold < 1 || p.RecoverThreshold > 10 {
		return fmt.Errorf("recover_threshold must be 1-10, got %d", p.RecoverThreshold)
	}
	return nil
}

// State 是一条线路的实时状态。
type State struct {
	Name         string `json:"name"`
	Up           bool   `json:"up"`
	FailCount    int    `json:"fail_count"`
	RecoverCount int    `json:"recover_count"`
	LastProbeAt  int64  `json:"last_probe_at"`
	LastError    string `json:"last_error"`
}

// Update 根据一次探测结果更新状态（核心状态机）。
func Update(st State, success bool, errMsg string, now int64, p Probe) State {
	st.LastProbeAt = now
	if success {
		st.FailCount = 0
		st.RecoverCount++
		if !st.Up && st.RecoverCount >= p.RecoverThreshold {
			st.Up = true
		}
		st.LastError = ""
	} else {
		st.RecoverCount = 0
		st.FailCount++
		// up 状态下 fail_count 也要累加，否则永远触发不了下线
		if st.Up && st.FailCount >= p.FailThreshold {
			st.Up = false
		}
		st.LastError = errMsg
	}
	return st
}

// PickMembers 选可用的成员做负载分担。
// 只取 Up && Enabled 的；按 Weight 做加权轮询（用 round 算偏移）；
// 全部 down 返回空 slice（不要随便挑一个，会把流量送到坏线路上）。
func PickMembers(members []Member, states map[string]State, round int) []Member {
	// 收集可用成员
	var avail []Member
	for _, m := range members {
		if !m.Enabled {
			continue
		}
		st, ok := states[m.Name]
		if !ok || !st.Up {
			continue
		}
		w := m.Weight
		if w <= 0 {
			w = 1
		}
		avail = append(avail, m)
		_ = w
	}
	if len(avail) == 0 {
		return []Member{}
	}
	// 计算总权重
	total := 0
	for _, m := range avail {
		w := m.Weight
		if w <= 0 {
			w = 1
		}
		total += w
	}
	if total <= 0 {
		return []Member{}
	}
	slot := round % total
	if slot < 0 {
		slot += total
	}
	// 加权轮询：累计权重找到对应成员
	acc := 0
	for _, m := range avail {
		w := m.Weight
		if w <= 0 {
			w = 1
		}
		acc += w
		if slot < acc {
			return []Member{m}
		}
	}
	// 兜底（不应到达）
	return []Member{avail[len(avail)-1]}
}
