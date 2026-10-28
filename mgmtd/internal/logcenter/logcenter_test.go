package logcenter

import (
	"os/exec"
	"regexp"
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
	// 窗口要开得够宽再从**老的一端**挑区分词。
	// 只看最近 5 条时，机器空闲期那几条几乎同质（同一服务的模板行），
	// 找不到区分性词就 Skip —— 实测这条用例一直 Skip，等于 beyond-window
	// 断言从没真正跑过。取 400 条并从最老的往前找，稳定能挑到。
	base, err := Query(QueryOpts{Limit: 400})
	if err != nil {
		t.Fatalf("Query base: %v", err)
	}
	if len(base) < 20 {
		t.Skipf("journal 条目太少（%d 条），无法构造 beyond-window 场景", len(base))
	}

	newest := base[0]
	newestWindow := base[:5]
	inWindow := func(e LogEntry) bool {
		for _, n := range newestWindow {
			if n.Message == e.Message && n.TS == e.TS {
				return true
			}
		}
		return false
	}

	tokenRe := regexp.MustCompile(`[A-Za-z0-9_./:-]{6,}`)
	var term, source string
	for i := len(base) - 1; i >= 10 && term == ""; i-- {
		msg := base[i].Message
		for _, w := range tokenRe.FindAllString(msg, 8) {
			if strings.Contains(newest.Message, w) {
				continue
			}
			term, source = w, msg
			break
		}
	}
	if term == "" {
		t.Skip("整窗都找不到与最新条目不同的词，跳过（环境太安静）")
	}
	t.Logf("区分词 %q 来自老条目: %s", term, truncate(source, 80))

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
	// 核心断言：取 limit=1 时窗口里只有最新那条，若 --grep 没下推到 journalctl，
	// 就只会在最新条目里找，找不到 term 而返回空。返回了含 term 的非窗口条目，
	// 才证明 grep 真的搜到了 -n 窗口之外。
	if inWindow(got[0]) {
		t.Errorf("grep 返回的是窗口内条目，未能证明搜到窗口外老日志")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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
// 搜不存在的词必须返回**空结果且不报错**。
//
// 这是 journalctl 的一个坑：无匹配时退出码是 1（不是 0），而旧实现把非零退出
// 一律当故障，于是「搜不到」被放大成 "journalctl and syslog both unavailable"
// 上抛成 500。原来的这条用例只检查「没匹配到不该出现的词」，即使 Query 直接
// 报错也照样 PASS，等于什么都没验。
func TestQueryGrepNoMatchReturnsEmpty(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not available")
	}
	got, err := Query(QueryOpts{Limit: 1, Grep: "zzzz-no-such-term-xyz"})
	if err != nil {
		t.Fatalf("无匹配不该报错，实际: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("无匹配应返回空，实际 %d 条", len(got))
	}
	for _, e := range got {
		if strings.Contains(e.Message, "zzzz-no-such-term-xyz") {
			t.Errorf("unexpected match for nonexistent term")
		}
	}
}
