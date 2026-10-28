package logcenter

import (
	"os/exec"
	"strings"
	"testing"
)

// TestQueryGrepFindsBeyondWindow 断言 grep 能搜到 -n 窗口之外的老日志。
//
// 构造方法：先取最近 5 条（无 grep），取其中一条较老的条目，挑一个它含但
// 最新条目不含的词 term；再用 limit=1 + term 去 grep。若 grep 走 journalctl
// 自己的 --grep 过滤，就能返回含 term 的条目，且该条目不是最新条目（证明搜到了
// 1 条窗口之外的老日志）；若仍先取尾部 1 条再过滤，返回空或最新条目。
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
		t.Fatalf("grep for %q returned 0 entries, but it exists in older log (beyond 1-entry window)", term)
	}
	if !strings.Contains(got[0].Message, term) {
		t.Errorf("returned entry does not contain grep term %q: %q", term, got[0].Message)
	}
	if got[0].Message == newest.Message && got[0].TS == newest.TS {
		t.Errorf("grep returned the newest entry only; did not search beyond the 1-entry window")
	}
}
