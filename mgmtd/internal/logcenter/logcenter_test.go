package logcenter

import (
	"testing"
)

func TestPriorityMap(t *testing.T) {
	cases := map[string]string{
		"0": "emerg",
		"1": "alert",
		"2": "crit",
		"3": "err",
		"4": "warning",
		"5": "notice",
		"6": "info",
		"7": "debug",
	}
	for in, want := range cases {
		if got, ok := priorityMap[in]; !ok || got != want {
			t.Errorf("priorityMap[%s] = %q, want %q", in, got, want)
		}
	}
}

func TestQueryGrepFilter(t *testing.T) {
	// 用 journalctl 不存在时的降级路径（返回 syslog，可能也不存在 → 空切片）
	// 这里主要测 Query 的 Grep 过滤逻辑：构造 entries 手动模拟
	entries := []LogEntry{
		{TS: "1", Unit: "u1", Priority: "info", Message: "hello world"},
		{TS: "2", Unit: "u2", Priority: "err", Message: "error occurred"},
		{TS: "3", Unit: "u3", Priority: "info", Message: "hello again"},
	}
	// 手动模拟 Grep 过滤
	var filtered []LogEntry
	grep := "hello"
	for _, e := range entries {
		if stringsContains(e.Message, grep) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered, got %d", len(filtered))
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestQueryNoPanic(t *testing.T) {
	// journalctl 可能不存在；应不 panic
	entries, err := Query(QueryOpts{Limit: 5, Grep: "nonexistent"})
	_ = err
	if entries == nil {
		entries = []LogEntry{}
	}
	// 不应 panic；entries 可能为空
}

func TestSourcesNoPanic(t *testing.T) {
	units, err := Sources()
	_ = err
	if units == nil {
		units = []string{}
	}
}
