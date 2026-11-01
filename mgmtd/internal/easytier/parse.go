package easytier

import (
	"strconv"
	"strings"
)

// Peer 是 easytier-cli peer 表格的一行。
type Peer struct {
	IPv4      string  `json:"ipv4"`
	Hostname  string  `json:"hostname"`
	Cost      string  `json:"cost"` // Local / p2p / relay
	LatencyMS float64 `json:"latency_ms"`
	LatencyOK bool    `json:"latency_ok"`
	Loss      string  `json:"loss"`
	Rx        string  `json:"rx"`
	Tx        string  `json:"tx"`
	Tunnel    string  `json:"tunnel"`
	NAT       string  `json:"nat"`
	Version   string  `json:"version"`
	Local     bool    `json:"local"`
}

// stripANSI 去掉终端颜色码：CLI 在 tty 下会带 \x1b[...m，非 tty 一般没有，
// 但容器里 TTY 继承不一致，带着颜色码会让字段值变成 "\x1b[32mp2p\x1b[0m"。
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			skip = true
		case skip && r == 'm':
			skip = false
		case skip:
			// 丢弃 CSI 参数
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParsePeerTable 解析 easytier-cli 的表格输出。
//
// 表格是 Markdown 风格（| 分隔 + 一行 --- 分隔符），列宽随内容变化，
// 所以按表头名字取列，不按固定字节偏移——偏移会随 hostname 长度变化而错位。
func ParsePeerTable(out string) []Peer {
	lines := strings.Split(out, "\n")
	var header []string
	peers := make([]Peer, 0, 4)

	for _, raw := range lines {
		line := strings.TrimSpace(stripANSI(raw))
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitRow(line)
		if len(cells) == 0 {
			continue
		}
		if header == nil {
			if isSeparatorRow(cells) {
				continue
			}
			header = cells
			continue
		}
		if isSeparatorRow(cells) {
			continue
		}
		if len(cells) < len(header) {
			continue
		}
		row := map[string]string{}
		for i, name := range header {
			row[strings.ToLower(name)] = cells[i]
		}
		p := Peer{
			IPv4:     row["ipv4"],
			Hostname: row["hostname"],
			Cost:     row["cost"],
			Loss:     row["loss"],
			Rx:       row["rx"],
			Tx:       row["tx"],
			Tunnel:   row["tunnel"],
			NAT:      row["nat"],
			Version:  row["version"],
		}
		if p.IPv4 == "" {
			continue
		}
		p.Local = strings.EqualFold(p.Cost, "Local")
		if lat := strings.TrimSpace(row["lat(ms)"]); lat != "" && lat != "-" {
			if v, err := strconv.ParseFloat(lat, 64); err == nil {
				p.LatencyMS = v
				p.LatencyOK = true
			}
		}
		peers = append(peers, p)
	}
	return peers
}

func splitRow(line string) []string {
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		trimmed := strings.Trim(c, "-:")
		if trimmed != "" {
			return false
		}
	}
	return true
}
