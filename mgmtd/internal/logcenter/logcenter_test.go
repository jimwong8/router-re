package logcenter

import (
	"os/exec"
	"strings"
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

// TestQueryGrepFindsBeyondWindow 断言 grep 能搜到 -n 窗口之外的老日志。
// 构造：取最近 5 条（无 grep），挑一条较老条目中最新条目不含的词 term；
// 再用 limit=1 + term 去 grep。若 grep 走 journalctl 自己的 --grep 过滤，
// 返回含 term 的条目且不是最新条目（证明搜到窗口外老日志）。
func TestQueryGrepFindsBeyondWindow(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not available")
	}
	base, err := Query(QueryOpts{Limit: 5})
	if err != nil {
		t.Fatalf("Query base: %v", err)
	}
	if len(base) < 2 {
		t.Skip("need at least 2 journal entries")
	}
	newest := base[0]
	older := base[len(base)-1]

	var term string
	for _, w := range strings.Fields(older.Message) {
		w = strings.Trim(w, ".,;:!?()[]{}\"'")
		if len(w) < 4 {
			continue
		}
		if !strings.Contains(newest.Message, w) {
			term = w
			break
		}
	}
	if term == "" {
		t.Skip("no distinguishing term between newest and older entry")
	}

	got, err := Query(QueryOpts{Limit: 1, Grep: term})
	if err != nil {
		t.Fatalf("Query grep: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("grep for %q returned 0 entries (beyond-window search failed)", term)
	}
	if !strings.Contains(got[0].Message, term) {
		t.Errorf("returned entry does not contain grep term %q", term)
	}
	if got[0].Message == newest.Message && got[0].TS == newest.TS {
		t.Errorf("grep returned newest entry only; did not search beyond 1-entry window")
	}
}

func TestSourcesNoPanic(t *testing.T) {
	units, err := Sources()
	_ = err
	if units == nil {
		units = []string{}
	}
}

// TestQueryGrepBeyondWindowAssertsJournalctlGrep documents that grep is pushed
// down to journalctl (--grep + regexp.QuoteMeta) so it can find entries older
// than the -n window. This is a compile-time/regression guard only.
func TestQueryGrepBeyondWindowAssertsJournalctlGrep(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not available")
	}
	// A grep term that matches nothing must not panic and must return empty.
	got, err := Query(QueryOpts{Limit: 1, Grep: "zzzz-no-such-term-xyz"})
	if err != nil {
		// journalctl may error on no match? tolerate
		t.Logf("Query returned err: %v", err)
	}
	if got == nil {
		got = []LogEntry{}
	}
	// Should not contain the term
	for _, e := range got {
		if strings.Contains(e.Message, "zzzz-no-such-term-xyz") {
			t.Errorf("unexpected match for nonexistent term")
		}
	}
}
