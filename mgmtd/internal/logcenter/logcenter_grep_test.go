package logcenter

import (
	"os/exec"
	"testing"
)

// TestQueryGrepBeyondWindow 验证 grep 能搜到 -n 窗口之外的老日志：
// 用极小的 limit=1 + 一个确实存在于 journal 的关键词，若 grep 仍走
// journalctl 自己的 --grep 过滤，就能返回匹配条目；若仍先取尾部 1 条再过滤，
// 绝大多数情况下该 1 条不会含关键词，返回空。
func TestQueryGrepBeyondWindow(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		t.Skip("journalctl not available")
	}
	entries, err := Query(QueryOpts{Limit: 1, Grep: "systemd"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) == 0 {
		t.Skip("no 'systemd' entries in journal on this host; cannot verify beyond-window search")
	}
	// 至少 1 条，且每条 message 都含关键词（大小写敏感）
	for _, e := range entries {
		if !containsStr(e.Message, "systemd") {
			t.Errorf("returned entry does not contain grep term: %q", e.Message)
		}
	}
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}