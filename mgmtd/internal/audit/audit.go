package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 审计文件的体积上限与查询窗口。
//
// 路由器上是 flash 存储：审计文件若无上限增长，任何人只要反复触发被审计的
// 接口（失败登录、写配置）就能把盘填满——这是最廉价的持久化 DoS。轮转把
// 在线占用压在 2×maxFileBytes 以内。
const (
	// maxFileBytes 达到后把当前文件改名为 .1（覆盖旧 .1）再重新开始。
	maxFileBytes = 8 << 20 // 8MB
	// maxQueryBytes 是查询时最多回读的尾部字节数。审计页面只需要最新 N 条，
	// 把整个文件读进内存等于让一次翻页变成内存放大攻击。
	maxQueryBytes = 4 << 20 // 4MB
)

// Event is a single audit log entry.
type Event struct {
	TS       int64  `json:"ts"`
	Actor    string `json:"actor"`
	Action   string `json:"action"`
	Target   string `json:"target"`
	Result   string `json:"result"`
	Detail   string `json:"detail"`
	RemoteIP string `json:"remote_ip"`
}

// Logger appends JSON audit events to a file.
type Logger struct {
	Path string
	mu   sync.Mutex
}

// New returns a Logger writing to the given path.
func New(path string) *Logger {
	return &Logger{Path: path}
}

// Log appends one event as a JSON line. Creates parent dirs and the file if
// missing (0644). If e.TS == 0 it is filled with time.Now().Unix().
func (l *Logger) Log(e Event) error {
	if e.TS == 0 {
		e.TS = time.Now().Unix()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	if dir := filepath.Dir(l.Path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// 轮转：超过上限就把当前文件挪成 .1（覆盖上一个 .1）。
	// 只保留一代，够回溯最近的记录，同时把占用钉在 2×maxFileBytes。
	// 轮转失败不影响本次写入——宁可暂时超限，也不要丢掉这条审计记录。
	if fi, err := os.Stat(l.Path); err == nil && fi.Size() >= maxFileBytes {
		_ = os.Remove(l.Path + ".1")
		_ = os.Rename(l.Path, l.Path+".1")
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

// Query returns matching events, newest first.
//
// 只回读文件尾部的 maxQueryBytes，并用定长环形缓冲保留最后 limit 条命中，
// 因此内存占用与 limit 成正比、与文件大小无关。action 非空时按子串过滤。
// limit <= 0 默认 100；文件不存在时返回空切片与 nil。
//
// 逐行解析而不是用 bufio.Scanner：Scanner 的 token 默认上限 64KB，一条
// 超长 detail 就会让整个查询返回 ErrTooLong，审计页直接白屏。
func (l *Logger) Query(limit int, action string) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	f, err := os.Open(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	if size == 0 {
		return []Event{}, nil
	}

	var start int64
	skipFirstLine := false
	if size > maxQueryBytes {
		start = size - maxQueryBytes
		// 判断起点是否正好落在行边界上：看一眼前一个字节是不是 '\n'。
		// 不看就会白丢一条有效记录（或被截断的半行解析失败）。
		var prev [1]byte
		if _, err := f.ReadAt(prev[:], start-1); err == nil && prev[0] != '\n' {
			skipFirstLine = true
		}
	}

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	remaining := size - start
	if remaining <= 0 {
		return []Event{}, nil
	}
	buf := make([]byte, remaining)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}

	lines := splitLines(buf)
	if skipFirstLine && len(lines) > 0 {
		// 起点落在行中间：第一段是被截断的半行，丢掉。
		lines = lines[1:]
	}

	// 从文件末尾倒着扫，凑够 limit 条就停——审计页永远只要最新的 N 条，
	// 没必要把 4MB 窗口里的几万行全部 JSON 解析一遍。
	out := make([]Event, 0, limit)
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // 跳过损坏行，不要因为一行坏掉就让整个审计页失败
		}
		if action != "" && !strings.Contains(e.Action, action) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// splitLines 按 '\n' 切分为字节切片，避免为每一行都分配字符串——
// 4MB 窗口里有几万行，逐行 string(...) 会让一次翻页多分配几十 MB。
func splitLines(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(b)/128+1)
	startIdx := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			out = append(out, b[startIdx:i])
			startIdx = i + 1
		}
	}
	if startIdx < len(b) {
		out = append(out, b[startIdx:])
	}
	return out
}
