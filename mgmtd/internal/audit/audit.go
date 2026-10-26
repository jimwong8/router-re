package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

// Query reads the log file and returns matching events, newest first.
// If action is non-empty, events whose Action contains it are kept.
// limit <= 0 defaults to 100. If the file does not exist, returns empty slice and nil.
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

	var matched []Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip corrupt lines
		}
		if action != "" && !strings.Contains(e.Action, action) {
			continue
		}
		matched = append(matched, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// reverse (newest first)
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched, nil
}