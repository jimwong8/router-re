// Package fwlog 解析并分析防火墙日志（nft/iptables LOG）。
// 本包只做分析与建议，绝不自动执行封禁。
package fwlog

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Event 是一条防火墙日志事件。
type Event struct {
	Time    int64  `json:"time"`
	SrcIP   string `json:"src_ip"`
	DstIP   string `json:"dst_ip"`
	SrcPort int    `json:"src_port"`
	DstPort int    `json:"dst_port"`
	Proto   string `json:"proto"`
	Action  string `json:"action"`
	Iface   string `json:"iface"`
	Chain   string `json:"chain"`
	Len     int    `json:"len"`
}

// Stat 是按源 IP 聚合的统计。
type Stat struct {
	IP      string `json:"ip"`
	Total   int    `json:"total"`
	Dropped int    `json:"dropped"`
	Bytes   int64  `json:"bytes"`
}

// Rule 是一条"建议封禁"记录（仅建议，不执行）。
type Rule struct {
	SrcIP  string `json:"src_ip"`
	Count  int    `json:"count"`
	Reason string `json:"reason"`
}

func getKV(line, key string) string {
	// 解析 "KEY=value"，value 到下一个空格
	prefix := key + "="
	idx := 0
	for {
		i := strings.Index(line[idx:], prefix)
		if i < 0 {
			return ""
		}
		i += idx
		// 确保是单词边界（前一个字符是空格或行首）
		if i > 0 && line[i-1] != ' ' {
			idx = i + len(prefix)
			continue
		}
		start := i + len(prefix)
		end := start
		for end < len(line) && line[end] != ' ' {
			end++
		}
		return line[start:end]
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ParseLines 解析 nft/iptables LOG 行，解析不了的跳过。
func ParseLines(lines []string) []Event {
	out := []Event{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		src := getKV(line, "SRC")
		if src == "" {
			continue
		}
		ev := Event{
			SrcIP:   src,
			DstIP:   getKV(line, "DST"),
			SrcPort: atoi(getKV(line, "SPT")),
			DstPort: atoi(getKV(line, "DPT")),
			Proto:   getKV(line, "PROTO"),
			Len:     atoi(getKV(line, "LEN")),
			Iface:   getKV(line, "IN"),
			Chain:   getKV(line, "CHAIN"),
		}
		// Action
		up := strings.ToUpper(line)
		if strings.Contains(up, "DROP") {
			ev.Action = "drop"
		} else if strings.Contains(up, "ACCEPT") {
			ev.Action = "accept"
		} else if strings.Contains(up, "REJECT") {
			ev.Action = "reject"
		} else {
			ev.Action = "log"
		}
		out = append(out, ev)
	}
	return out
}

// TopTalkers 按 SrcIP 聚合，按 Dropped 降序。
func TopTalkers(evts []Event, n int) []Stat {
	m := map[string]*Stat{}
	for _, e := range evts {
		if e.SrcIP == "" {
			continue
		}
		s, ok := m[e.SrcIP]
		if !ok {
			s = &Stat{IP: e.SrcIP}
			m[e.SrcIP] = s
		}
		s.Total++
		if e.Action == "drop" || e.Action == "reject" {
			s.Dropped++
		}
		s.Bytes += int64(e.Len)
	}
	out := make([]Stat, 0, len(m))
	for _, s := range m {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dropped != out[j].Dropped {
			return out[i].Dropped > out[j].Dropped
		}
		return out[i].Total > out[j].Total
	})
	if n > 0 && n < len(out) {
		out = out[:n]
	}
	if out == nil {
		out = []Stat{}
	}
	return out
}

// Summarize 统计总数/按动作/按协议/按链。
func Summarize(evts []Event) map[string]interface{} {
	byAction := map[string]int{}
	byProto := map[string]int{}
	byChain := map[string]int{}
	for _, e := range evts {
		byAction[e.Action]++
		byProto[e.Proto]++
		byChain[e.Chain]++
	}
	return map[string]interface{}{
		"total":     len(evts),
		"by_action": byAction,
		"by_proto":  byProto,
		"by_chain":  byChain,
	}
}

// Suspicious 同一源IP在 window 秒内 drop 超过 50 次 -> 返回该IP。
func Suspicious(evts []Event, window int) []string {
	if window <= 0 {
		window = 60
	}
	// 收集每个 IP 的 drop 时间
	times := map[string][]int64{}
	for _, e := range evts {
		if e.Action != "drop" && e.Action != "reject" {
			continue
		}
		times[e.SrcIP] = append(times[e.SrcIP], e.Time)
	}
	result := []string{}
	seen := map[string]bool{}
	for ip, ts := range times {
		sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
		// 滑动窗口
		susp := false
		for i := range ts {
			cnt := 0
			for j := i; j < len(ts) && ts[j]-ts[i] <= int64(window); j++ {
				cnt++
			}
			if cnt > 50 {
				susp = true
				break
			}
		}
		if susp && !seen[ip] {
			seen[ip] = true
			result = append(result, ip)
		}
	}
	sort.Strings(result)
	if result == nil {
		result = []string{}
	}
	return result
}

// SuggestBlock 生成"建议封禁"清单（只给建议，绝不自动执行）。
func SuggestBlock(evts []Event, threshold int) []Rule {
	if threshold <= 0 {
		threshold = 50
	}
	counts := map[string]int{}
	for _, e := range evts {
		if e.Action == "drop" || e.Action == "reject" {
			counts[e.SrcIP]++
		}
	}
	out := []Rule{}
	for ip, c := range counts {
		if c >= threshold {
			out = append(out, Rule{SrcIP: ip, Count: c, Reason: fmt.Sprintf("dropped %d times", c)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if out == nil {
		out = []Rule{}
	}
	return out
}
