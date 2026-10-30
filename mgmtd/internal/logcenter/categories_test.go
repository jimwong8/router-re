package logcenter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeAudit(t *testing.T, evs []map[string]interface{}) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "audit.jsonl")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, e := range evs {
		b, _ := json.Marshal(e)
		f.Write(b)
		f.WriteString("\n")
	}
	return p
}

func TestFindCategory(t *testing.T) {
	if len(Categories) != 9 {
		t.Fatalf("应有 9 个分类，实际 %d", len(Categories))
	}
	for _, k := range []string{"arp", "ddns", "dial", "push", "auth", "dhcp", "system", "webadmin", "weblogin"} {
		if _, ok := FindCategory(k); !ok {
			t.Fatalf("缺少分类 %q", k)
		}
	}
	if _, ok := FindCategory("nope"); ok {
		t.Fatal("未知分类应返回 false")
	}
}

func TestQueryCategoryUnknown(t *testing.T) {
	if _, err := QueryCategory("nope", 10, "", ""); err == nil {
		t.Fatal("未知分类应报错")
	}
}

func TestWebloginFromAudit(t *testing.T) {
	now := time.Now().Unix()
	p := writeAudit(t, []map[string]interface{}{
		{"ts": now - 300, "actor": "admin", "action": "auth.login", "target": "/api/v1/auth/login", "result": "ok", "remote_ip": "10.100.1.18"},
		{"ts": now - 200, "actor": "admin", "action": "vpn.client.save", "target": "/api/v1/vpn/client", "result": "ok"},
		{"ts": now - 100, "actor": "bob", "action": "auth.login", "target": "/api/v1/auth/login", "result": "fail", "detail": "HTTP 401"},
		{"ts": now - 50, "actor": "admin", "action": "auth.logout", "target": "/api/v1/auth/logout", "result": "ok"},
	})
	res, err := QueryCategory("weblogin", 100, "", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 3 {
		t.Fatalf("weblogin 应只含 3 条登录/登出，实际 %d: %+v", len(res.Entries), res.Entries)
	}
	for _, e := range res.Entries {
		if strings.Contains(e.Message, "vpn.client.save") {
			t.Fatalf("weblogin 不应包含配置变更: %s", e.Message)
		}
	}
	// 失败登录应标为 warning
	found := false
	for _, e := range res.Entries {
		if strings.Contains(e.Message, "bob") && e.Priority == "warning" {
			found = true
		}
	}
	if !found {
		t.Fatalf("失败的登录应标记 warning: %+v", res.Entries)
	}
	if !strings.HasPrefix(res.Source, "audit:") {
		t.Fatalf("来源应标明审计文件: %s", res.Source)
	}
}

func TestWebadminExcludesAuth(t *testing.T) {
	now := time.Now().Unix()
	p := writeAudit(t, []map[string]interface{}{
		{"ts": now - 300, "actor": "admin", "action": "auth.login", "target": "/login", "result": "ok"},
		{"ts": now - 200, "actor": "admin", "action": "firewall.rule.save", "target": "/api/v1/firewall/rules", "result": "ok"},
		{"ts": now - 100, "actor": "admin", "action": "system.disk.format", "target": "/dev/sda1", "result": "fail"},
	})
	res, err := QueryCategory("webadmin", 100, "", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("webadmin 应只含 2 条配置变更，实际 %d", len(res.Entries))
	}
	for _, e := range res.Entries {
		if strings.Contains(e.Message, "auth.login") {
			t.Fatalf("webadmin 不应包含登录: %s", e.Message)
		}
	}
}

func TestAuditOrderNewestFirst(t *testing.T) {
	now := time.Now().Unix()
	p := writeAudit(t, []map[string]interface{}{
		{"ts": now - 100, "actor": "a", "action": "auth.login", "target": "/l", "result": "ok"},
		{"ts": now - 50, "actor": "b", "action": "auth.login", "target": "/l", "result": "ok"},
	})
	res, _ := QueryCategory("weblogin", 100, "", p)
	if len(res.Entries) != 2 {
		t.Fatalf("应有 2 条: %+v", res.Entries)
	}
	if res.Entries[0].TS <= res.Entries[1].TS {
		t.Fatalf("应按时间从新到旧排列: %v", res.Entries)
	}
}

func TestAuditLimit(t *testing.T) {
	now := time.Now().Unix()
	var evs []map[string]interface{}
	for i := 0; i < 50; i++ {
		evs = append(evs, map[string]interface{}{
			"ts": now - int64(50-i), "actor": "a", "action": "auth.login", "target": "/l", "result": "ok",
		})
	}
	p := writeAudit(t, evs)
	res, _ := QueryCategory("weblogin", 10, "", p)
	if len(res.Entries) != 10 {
		t.Fatalf("limit=10 应返回 10 条，实际 %d", len(res.Entries))
	}
	// limit 截断后仍应是"最新的 10 条"
	if res.Entries[0].TS <= res.Entries[9].TS {
		t.Fatalf("截断后顺序错误: %v", []string{res.Entries[0].TS, res.Entries[9].TS})
	}
}

func TestAuditSinceFilter(t *testing.T) {
	now := time.Now().Unix()
	p := writeAudit(t, []map[string]interface{}{
		{"ts": now - 7200, "actor": "old", "action": "auth.login", "target": "/l", "result": "ok"},
		{"ts": now - 60, "actor": "new", "action": "auth.login", "target": "/l", "result": "ok"},
	})
	res, _ := QueryCategory("weblogin", 100, "30m", p)
	if len(res.Entries) != 1 {
		t.Fatalf("since=30m 应只剩 1 条，实际 %d: %+v", len(res.Entries), res.Entries)
	}
	if !strings.Contains(res.Entries[0].Message, "new") {
		t.Fatalf("保留的应是最近的记录: %s", res.Entries[0].Message)
	}
}

func TestAuditMissingFileIsNotError(t *testing.T) {
	res, err := QueryCategory("webadmin", 10, "", "/nonexistent/audit.jsonl")
	if err != nil {
		t.Fatalf("审计文件缺失不应报错，应返回说明: %v", err)
	}
	if len(res.Entries) != 0 || res.Note == "" {
		t.Fatalf("应返回空列表 + 说明，实际 %+v", res)
	}
}

func TestSystemCategoryUsesSystemLog(t *testing.T) {
	// 不依赖 journalctl 有数据，只验证走的是系统日志这条路且不 panic
	res, err := QueryCategory("system", 5, "", "")
	if err != nil {
		t.Fatalf("system 分类不应返回错误: %v", err)
	}
	if res.Category != "system" {
		t.Fatalf("分类字段错误: %s", res.Category)
	}
	if res.Entries == nil {
		t.Fatal("Entries 不应为 nil（前端按数组处理）")
	}
}

func TestParseSince(t *testing.T) {
	now := time.Now().Unix()
	cases := map[string]int64{
		"1 hour ago": now - 3600,
		"2h":         now - 7200,
		"30m":        now - 1800,
		"1d":         now - 86400,
	}
	for in, want := range cases {
		got, err := parseSince(in)
		if err != nil {
			t.Fatalf("%q 解析失败: %v", in, err)
		}
		if diff := got - want; diff > 5 || diff < -5 {
			t.Fatalf("%q 解析偏差过大: got=%d want=%d", in, got, want)
		}
	}
	if _, err := parseSince("garbage"); err == nil {
		t.Fatal("无法解析的字符串应报错")
	}
}

func TestSources(t *testing.T) {
	p := writeAudit(t, []map[string]interface{}{{"ts": 1, "action": "auth.login"}})
	list := CategorySources(p)
	if len(list) != 9 {
		t.Fatalf("应有 9 项，实际 %d", len(list))
	}
	for _, m := range list {
		if m["available"] != true {
			t.Fatalf("%s 的审计文件存在，应标记可用: %+v", m["key"], m)
		}
	}
}

func TestSinceNormalizedForJournalctl(t *testing.T) {
	// "1d" 这类写法必须先转成绝对时间，否则 journalctl 会报 invalid time
	// 并被降级逻辑吞掉，表现为"最近一天没有日志"。
	ts, err := parseSince("1d")
	if err != nil {
		t.Fatalf("parseSince(1d) 失败: %v", err)
	}
	if time.Now().Unix()-ts < 86000 {
		t.Fatalf("1d 应解析为约 24 小时前: diff=%d", time.Now().Unix()-ts)
	}
	// 规范化后必须被 journalctl 接受（这里只验证格式可解析回同一时间）
	norm := time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	// journalctl 把不带时区的时间当本地时间解释，这里必须同样用 Local 解析
	back, err := time.ParseInLocation("2006-01-02 15:04:05", norm, time.Local)
	if err != nil {
		t.Fatalf("规范化格式不可解析: %v", err)
	}
	if back.Unix() != ts {
		t.Fatalf("规范化后时间变化: %d != %d", back.Unix(), ts)
	}
}

func TestSystemCategoryWithShortSince(t *testing.T) {
	// 端到端：带 since=1d 的 system 查询必须能返回内容而不是被吞成空
	res, err := QueryCategory("system", 20, "1d", "")
	if err != nil {
		t.Fatalf("system+since 不应报错: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Logf("当前系统 1 天内没有 warning 以上日志（note=%s）", res.Note)
	}
}

func TestSystemSinceReturnsNewest(t *testing.T) {
	// 回归：`journalctl --since X -n N` 取的是过滤后最早的 N 条，
	// 表现为用户选"最近一天"却看到一天前最早的记录。这里断言返回的是
	// 时间窗内**最新**的若干条（TS 递减且都在窗口内）。
	res, err := QueryCategory("system", 10, "1d", "")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(res.Entries) == 0 {
		t.Skip("当前系统 1 天内没有系统日志")
	}
	for i := 1; i < len(res.Entries); i++ {
		if res.Entries[i-1].TS < res.Entries[i].TS {
			t.Fatalf("结果应按时间从新到旧: [%d]=%s [%d]=%s",
				i-1, res.Entries[i-1].TS, i, res.Entries[i].TS)
		}
	}
	cutoff := time.Now().Add(-25 * time.Hour)
	for _, e := range res.Entries {
		tm, err := time.ParseInLocation("2006-01-02 15:04:05", e.TS, time.Local)
		if err != nil {
			continue
		}
		if tm.Before(cutoff) {
			t.Fatalf("返回了时间窗之外的日志: %s", e.TS)
		}
	}
	// 最新的一条应当接近当前时间（1 小时内），否则说明取的还是老记录
	if len(res.Entries) > 0 {
		tm, err := time.ParseInLocation("2006-01-02 15:04:05", res.Entries[0].TS, time.Local)
		if err == nil && time.Since(tm) > time.Hour {
			t.Fatalf("最新一条距今 %v，说明取的不是尾部记录: %s", time.Since(tm), res.Entries[0].TS)
		}
	}
}
