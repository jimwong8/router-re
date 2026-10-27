package accounting

import (
	"testing"
)

func TestCounterAddAndSnapshot(t *testing.T) {
	c := NewCounter()
	c.Add("alice", 1024*1024, 100) // 正好 1MB
	c.Add("alice", 500000, 200)    // 500000 字节，不到 1MB
	c.Add("bob", 1500000, 300)     // 1500000 字节 -> 1MB（向下取整）
	snap := c.Snapshot()
	if snap["alice"] != 1 {
		t.Fatalf("alice MB=%d (want 1)", snap["alice"])
	}
	if snap["bob"] != 1 {
		t.Fatalf("bob MB=%d (want 1), 1500000 字节应下取整为 1MB", snap["bob"])
	}
}

func TestCounterExceededBoundary(t *testing.T) {
	c := NewCounter()
	// quota=100, used=100 已超额（>= 算超）
	c.Add("alice", 100*1024*1024, 0)
	if !c.Exceeded("alice", 100) {
		t.Fatal("used=100 quota=100 应算超额")
	}
	// used=99 不超
	c2 := NewCounter()
	c2.Add("bob", 99*1024*1024, 0)
	if c2.Exceeded("bob", 100) {
		t.Fatal("used=99 quota=100 不应超额")
	}
}

func TestCounterExceededUnlimited(t *testing.T) {
	c := NewCounter()
	c.Add("alice", 999*1024*1024, 0)
	// quota<=0 视为不限，恒 false
	if c.Exceeded("alice", 0) {
		t.Fatal("quota=0 不限，不应超额")
	}
	if c.Exceeded("alice", -5) {
		t.Fatal("quota<0 不限，不应超额")
	}
}

func TestCounterReset(t *testing.T) {
	c := NewCounter()
	c.Add("alice", 5*1024*1024, 100)
	if _, ok := c.Snapshot()["alice"]; !ok {
		t.Fatal("alice 应存在")
	}
	if !c.Reset("alice") {
		t.Fatal("重置已存在的用户应返回 true")
	}
	if _, ok := c.Snapshot()["alice"]; ok {
		t.Fatal("重置后 alice 应清零")
	}
	if c.LastSeen("alice") != 0 {
		t.Fatal("重置后 lastSeen 应清零")
	}
	// 未知用户返回 false
	if c.Reset("nonexistent") {
		t.Fatal("重置未知用户应返回 false")
	}
}

// TestAttributeOnlySourceIP —— 只按源 IP 归属，双向不重复计数。
func TestAttributeOnlySourceIP(t *testing.T) {
	entries := []Entry{
		{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", Bytes: 1000},
	}
	// A、B 都绑定：10.0.0.1 -> alice，10.0.0.2 -> bob
	bindings := map[string]string{
		"10.0.0.1": "alice",
		"10.0.0.2": "bob",
	}
	perUser, skipped := Attribute(entries, bindings)
	if skipped != 0 {
		t.Fatalf("skipped=%d (want 0)", skipped)
	}
	if perUser["alice"] != 1000 {
		t.Fatalf("alice 应得全部 1000, got %d", perUser["alice"])
	}
	if perUser["bob"] != 0 {
		t.Fatalf("dst 绑定 bob 不应被归属（避免双向计数）, got %d", perUser["bob"])
	}
}

func TestAttributeSkipped(t *testing.T) {
	entries := []Entry{
		{SrcIP: "10.0.0.1", DstIP: "9.9.9.9", Bytes: 100}, // src 绑定 alice
		{SrcIP: "8.8.8.8", DstIP: "10.0.0.2", Bytes: 200}, // src 未绑定 -> skip
		{SrcIP: "7.7.7.7", DstIP: "6.6.6.6", Bytes: 300},  // src,dst 都未绑定 -> skip
	}
	bindings := map[string]string{"10.0.0.1": "alice"}
	perUser, skipped := Attribute(entries, bindings)
	if skipped != 2 {
		t.Fatalf("skipped=%d (want 2)", skipped)
	}
	if perUser["alice"] != 100 {
		t.Fatalf("alice=%d (want 100)", perUser["alice"])
	}
}

// TestParseConntrackBytesFirstField —— 用任务给的真实 conntrack 行，
// 有两组 src/dst 和两个 bytes=，必须取第一个（请求方向）。
func TestParseConntrackBytesFirstField(t *testing.T) {
	line := `ipv4 2 tcp 6 431992 ESTABLISHED src=10.100.1.18 dst=10.100.1.13 sport=55566 dport=5432 packets=120 bytes=48213 src=10.100.1.13 dst=10.100.1.18 sport=5432 dport=55566 packets=90 bytes=32144 [ASSURED] mark=0 zone=0 use=2`
	src, bytes, enabled := ParseConntrackBytes(line)
	if !enabled {
		t.Fatal("存在 bytes= 字段时 acctEnabled 应为 true")
	}
	if src != "10.100.1.18" {
		t.Fatalf("src=%q (want 10.100.1.18，取第一组 src)", src)
	}
	if bytes != 48213 {
		t.Fatalf("bytes=%d (want 48213，取第一个 bytes==请求方向)", bytes)
	}
}

func TestParseConntrackBytesNoAccounting(t *testing.T) {
	// 无 bytes= 字段（未启用 nf_conntrack_acct）
	line := `ipv4 2 tcp 6 120 ESTABLISHED src=10.0.0.1 dst=10.0.0.2 sport=55566 dport=5432 [ASSURED]`
	src, bytes, enabled := ParseConntrackBytes(line)
	if enabled {
		t.Fatal("无 bytes= 字段时 acctEnabled 应为 false")
	}
	if bytes != 0 {
		t.Fatalf("无 bytes 时 bytes=%d (want 0)", bytes)
	}
	if src != "10.0.0.1" {
		t.Fatalf("src=%q (want 10.0.0.1)", src)
	}
}

func TestBuildReport(t *testing.T) {
	used := map[string]int{
		"alice": 50,  // 已用 50
		"bob":   200, // 已用 200，超 100 配额
		"carol": 30,  // 不限配额
	}
	quotas := map[string]int{
		"alice": 100, // 50%
		"bob":   100, // 200% 超额
		"carol": 0,   // 不限
	}
	lastSeen := map[string]int64{
		"alice": 1000,
		"bob":   3000,
		"carol": 2000,
	}
	reports := BuildReport(used, quotas, lastSeen)
	if len(reports) != 3 {
		t.Fatalf("len=%d (want 3)", len(reports))
	}
	// 降序：bob 200%% > alice 50%% > carol 0%%（不限）
	if reports[0].Username != "bob" || reports[0].Percent != 200 || !reports[0].Exceeded {
		t.Fatalf("reports[0]=%+v (want bob 200 percent exceeded)", reports[0])
	}
	if reports[1].Username != "alice" || reports[1].Percent != 50 {
		t.Fatalf("reports[1]=%+v (want alice 50 percent)", reports[1])
	}
	if reports[2].Username != "carol" || reports[2].Percent != 0 || reports[2].Exceeded {
		t.Fatalf("reports[2]=%+v (want carol 0 percent unlimited)", reports[2])
	}
	if reports[0].QuotaMB != 100 || reports[0].UsedMB != 200 {
		t.Fatalf("bob quota/used 错误: %+v", reports[0])
	}
}

// TestAddUpdatesLastSeenOnlyWithNow —— Add 传 now 才更新 lastSeen。
func TestAddUpdatesLastSeenOnlyWithNow(t *testing.T) {
	c := NewCounter()
	c.Add("alice", 1024, 0) // now=0 不更新
	if c.LastSeen("alice") != 0 {
		t.Fatalf("now=0 不应更新 lastSeen, got %d", c.LastSeen("alice"))
	}
	c.Add("alice", 1024, 500)
	if c.LastSeen("alice") != 500 {
		t.Fatalf("lastSeen=%d (want 500)", c.LastSeen("alice"))
	}
	// 更早的 now 不覆盖
	c.Add("alice", 1024, 100)
	if c.LastSeen("alice") != 500 {
		t.Fatalf("更早 now 不应覆盖, got %d", c.LastSeen("alice"))
	}
}
