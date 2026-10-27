package logcenter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogEntry 描述一条系统日志记录。
type LogEntry struct {
	TS       string `json:"ts"`       // ISO 时间戳或原始时间字符串
	Unit     string `json:"unit"`     // systemd unit 名称
	Priority string `json:"priority"` // emerg/alert/crit/err/warning/notice/info/debug
	Message  string `json:"message"`  // 消息内容
}

// QueryOpts 是 Query 的查询选项。
type QueryOpts struct {
	Limit int    // 最大条数（默认 100，上限 5000）
	Since string // --since 时间（如 "1 hour ago"）
	Unit  string // systemd unit 过滤
	Grep  string // message 子串过滤
}

// priorityMap 将数字优先级映射为 syslog 名称。
var priorityMap = map[string]string{
	"0": "emerg",
	"1": "alert",
	"2": "crit",
	"3": "err",
	"4": "warning",
	"5": "notice",
	"6": "info",
	"7": "debug",
}

// Query 查询系统日志。优先 journalctl -o json --no-pager -n <limit>，
// 有 Unit 加 -u、有 Since 加 --since；journalctl 不存在或无权限时降级读 /var/log/syslog 末尾 limit 行。
// Grep 在 message 上做子串过滤。
func Query(o QueryOpts) ([]LogEntry, error) {
	if o.Limit <= 0 {
		o.Limit = 100
	}
	if o.Limit > 5000 {
		o.Limit = 5000
	}
	entries, err := queryJournalctl(o)
	if err != nil {
		// 降级读 /var/log/syslog
		entries, err = readSyslogTail(o.Limit)
		if err != nil {
			// 最终降级：返回空切片 + error
			return []LogEntry{}, fmt.Errorf("journalctl and syslog both unavailable: %w", err)
		}
	}
	// Grep 过滤
	if o.Grep != "" {
		var filtered []LogEntry
		for _, e := range entries {
			if strings.Contains(e.Message, o.Grep) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	// limit 最终截断
	if len(entries) > o.Limit {
		entries = entries[:o.Limit]
	}
	if entries == nil {
		entries = []LogEntry{}
	}
	return entries, nil
}

func queryJournalctl(o QueryOpts) ([]LogEntry, error) {
	args := []string{"-o", "json", "--no-pager", "-n", strconv.Itoa(o.Limit)}
	if o.Unit != "" {
		args = append(args, "-u", o.Unit)
	}
	if o.Since != "" {
		args = append(args, "--since", o.Since)
	}
	cmd := exec.Command("journalctl", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		entry := LogEntry{}
		// __REALTIME_TIMESTAMP 是字符串形式的微秒时间戳（如 1793084380346468）。
		// 直接透传会让前端显示成一串裸数字，人看不懂 —— 这里转成可读时间。
		if ts, ok := raw["__REALTIME_TIMESTAMP"].(string); ok {
			if us, err := strconv.ParseInt(ts, 10, 64); err == nil && us > 0 {
				entry.TS = time.Unix(0, us*1000).Local().Format("2006-01-02 15:04:05")
			} else {
				entry.TS = ts
			}
		}
		// _SYSTEMD_UNIT
		if u, ok := raw["_SYSTEMD_UNIT"].(string); ok {
			entry.Unit = u
		}
		// PRIORITY
		if p, ok := raw["PRIORITY"].(string); ok {
			if name, ok := priorityMap[p]; ok {
				entry.Priority = name
			} else {
				entry.Priority = p
			}
		}
		// MESSAGE
		if m, ok := raw["MESSAGE"].(string); ok {
			entry.Message = m
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// readSyslogTail 降级读 /var/log/syslog 末尾 limit 行，正则解析。
func readSyslogTail(limit int) ([]LogEntry, error) {
	f, err := os.Open("/var/log/syslog")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	// 简化：读取全部行，取末尾 limit 行
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	re := regexp.MustCompile(`^(\w+\s+\d+\s+\d+:\d+:\d+)\s+(\S+)\s+(\S+)(?:\[\d+\])?:\s*(.*)$`)
	var entries []LogEntry
	for _, line := range lines[start:] {
		m := re.FindStringSubmatch(line)
		if m != nil {
			entries = append(entries, LogEntry{
				TS:       m[1],
				Unit:     m[2],
				Priority: "info",
				Message:  m[4],
			})
		} else {
			entries = append(entries, LogEntry{
				TS:       "",
				Unit:     "",
				Priority: "info",
				Message:  line,
			})
		}
	}
	return entries, nil
}

// Sources 返回 journalctl --field=_SYSTEMD_UNIT 去重前 30 个 unit 名称。
// journalctl --field=_SYSTEMD_UNIT 每次要扫完整索引（热缓存约 1s，冷启动
// 可到 19s）。前端下拉框每开一次都等这个延迟没有意义，加 60s 缓存。
var (
	sourcesMu    sync.Mutex
	sourcesCache []string
	sourcesAt    time.Time
)

const sourcesTTL = 60 * time.Second

func Sources() ([]string, error) {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	if sourcesCache != nil && time.Since(sourcesAt) < sourcesTTL {
		return sourcesCache, nil
	}

	// 加 10s 上限：journalctl 首次要建索引，慢机器上能拖到 19s+，
	// 无超时的 cmd.Output() 会让 HTTP 请求一起挂死。
	// ⚠ 超时/失败时返回空列表而不是 error：取不到 unit 列表只说明
	// journal 不可读，不等于接口坏了。返回 500 会让前端整块白屏
	// （实测：超时被杀后 detail 是 "signal: killed"）。
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "journalctl", "--field=_SYSTEMD_UNIT", "--no-pager")
	out, err := cmd.Output()
	if err != nil {
		if sourcesCache != nil {
			return sourcesCache, nil // 有旧缓存先用旧缓存
		}
		return []string{}, nil // 降级为空列表，不下发 500
	}
	seen := make(map[string]bool)
	var units []string
	for _, line := range strings.Split(string(out), "\n") {
		u := strings.TrimSpace(line)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		units = append(units, u)
		if len(units) >= 30 {
			break
		}
	}
	sort.Strings(units)
	sourcesCache = units
	sourcesAt = time.Now()
	return units, nil
}
