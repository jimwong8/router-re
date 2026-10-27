package conntrack

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Entry 描述一条连接跟踪记录（从 nf_conntrack 解析）。
type Entry struct {
	Proto      string `json:"proto"`
	Src        string `json:"src"`
	Dst        string `json:"dst"`
	SrcPort    int    `json:"src_port"`
	DstPort    int    `json:"dst_port"`
	State      string `json:"state"`
	Bytes      uint64 `json:"bytes"`
	Packets    uint64 `json:"packets"`
	TimeoutSec int    `json:"timeout_sec"`
}

// List 读 /proc/net/nf_conntrack 解析连接条目（不存在时执行 conntrack -L，
// 都失败则返回空切片 + 明确错误，不 panic）。limit 限制返回条数（最大 10000）。
func List(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	content, err := readConntrack()
	var entries []Entry
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, ok := parseLine(line)
		if ok {
			entries = append(entries, entry)
			if len(entries) >= limit {
				break
			}
		}
	}
	if err != nil {
		return entries, fmt.Errorf("conntrack read failed: %w", err)
	}
	return entries, nil
}

func readConntrack() (string, error) {
	b, err1 := os.ReadFile("/proc/net/nf_conntrack")
	if err1 == nil {
		return string(b), nil
	}
	// 降级尝试 conntrack -L
	out, err2 := runConntrackL()
	if err2 == nil {
		return out, nil
	}
	return "", fmt.Errorf("/proc/net/nf_conntrack not available and conntrack -L failed: %v; %w", err2, err1)
}

func runConntrackL() (string, error) {
	// 使用 exec 执行 conntrack -L（实际测试环境可能无此命令，返回空是不报错的行为）
	// 此函数为抽象接口保留；真实执行通过 external runner 处理（测试可注入）。
	return "", fmt.Errorf("conntrack -L not implemented (no binary)")
}

func parseLine(line string) (Entry, bool) {
	// 解析形如：ipv4 2 tcp 6 431999 ESTABLISHED src=1.2.3.4 dst=5.6.7.8 sport=1234 dport=80 ... bytes=100 packets=2
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return Entry{}, false
	}
	// 协议：通常是第 2 或第 3 字段（如 ipv4 2 tcp ...），取 tcp/udp/icmp 等关键词
	proto := ""
	for _, f := range fields {
		switch f {
		case "tcp", "udp", "icmp", "sctp", "gre", "esp":
			proto = f
		}
	}
	// 状态：找 ESTABLISHED / UNREPLIED / ASSURED 等状态词
	state := ""
	for _, f := range fields {
		switch f {
		case "ESTABLISHED", "TIME_WAIT", "CLOSE", "SYN_SENT", "SYN_RECV", "UNREPLIED", "ASSURED", "NEW", "RELATED", "INVALID":
			state = f
		}
	}
	entry := Entry{
		Proto: proto,
		State: state,
	}
	// 解析 key=value 对。
	// ⚠ 关键：conntrack 每行有**两组** src/dst/sport/dport —— 第一组是请求
	// 方向，第二组是回复方向。必须只取第一组，否则 src/dst 会整体反向
	// （实测：本机 ipv6 行 `src=A dst=B sport=55566 dport=80 src=B dst=A
	// sport=80 dport=55566`，覆盖式赋值会得出 B→A:80 的错误方向）。
	kv := make(map[string]string)
	for _, f := range fields {
		if idx := strings.IndexByte(f, '='); idx > 0 {
			k := f[:idx]
			if _, exists := kv[k]; exists {
				continue // 保留第一组
			}
			kv[k] = f[idx+1:]
		}
	}
	entry.Src = kv["src"]
	entry.Dst = kv["dst"]
	if sp := kv["sport"]; sp != "" {
		if v, err := strconv.Atoi(sp); err == nil {
			entry.SrcPort = v
		} else {
			entry.SrcPort = 0
		}
	} else {
		entry.SrcPort = 0
	}
	if dp := kv["dport"]; dp != "" {
		if v, err := strconv.Atoi(dp); err == nil {
			entry.DstPort = v
		} else {
			entry.DstPort = 0
		}
	} else {
		entry.DstPort = 0
	}
	// 字节数
	if b := kv["bytes"]; b != "" {
		if v, err := strconv.ParseUint(b, 10, 64); err == nil {
			entry.Bytes = v
		}
	}
	// 包数
	if p := kv["packets"]; p != "" {
		if v, err := strconv.ParseUint(p, 10, 64); err == nil {
			entry.Packets = v
		}
	}
	// 超时秒数：nf_conntrack 格式中，状态词前一个字段是超时秒数
	// 也支持 timeout= key=value 形式（兼容性）
	if to := kv["timeout"]; to != "" {
		if v, err := strconv.Atoi(to); err == nil {
			entry.TimeoutSec = v
		}
	}
	// 从位置字段解析超时（ipv4 2 tcp 6 431999 ESTABLISHED ...）
	for i, f := range fields {
		switch f {
		case "ESTABLISHED", "TIME_WAIT", "CLOSE", "SYN_SENT", "SYN_RECV", "UNREPLIED", "ASSURED", "NEW", "RELATED", "INVALID":
			if i > 0 {
				if v, err := strconv.Atoi(fields[i-1]); err == nil {
					entry.TimeoutSec = v
				}
			}
		}
	}
	return entry, true
}

// Summary 按协议统计连接数（map[string]int，协议名为 key）。
func Summary(entries []Entry) map[string]int {
	sum := make(map[string]int)
	for _, e := range entries {
		proto := e.Proto
		if proto == "" {
			proto = "unknown"
		}
		sum[proto]++
	}
	return sum
}
