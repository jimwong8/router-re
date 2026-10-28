package logcenter

import (
	"os/exec"
	"strings"
	"testing"
)

// TestQueryGrepBeyondWindow 验证 grep 能搜到 -n 窗口之外的老日志：
// 取最近 5 条无 grep，挑一条较老的条目中、最新条目不含的词，
// 再用 limit=1 + 该词去 grep；若仍能返回含该词的条目，且该条目不是最新条目，
// 说明 grep 走了 journalctl 自己的 --grep 过滤（能越过 1 条的窗口），
// 而不是先取尾部 1 条再过滤。
func TestQueryGrepBeyondWindow(t *testing.T) {
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
		t.Fatalf("grep for %q returned 0 entries, but it exists in older log", term)
	}
	if !strings.Contains(got[0].Message, term) {
		t.Errorf("returned entry does not contain grep term %q: %q", term, got[0].Message)
	}
	if got[0].Message == newest.Message && got[0].TS == newest.TS {
		t.Errorf("grep returned the newest entry, did not search beyond the 1-entry window")
	}
}