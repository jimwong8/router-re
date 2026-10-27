package threat

import (
	"testing"
)

func TestAddUnique(t *testing.T) {
	l := NewList()
	if l.Size() != 0 {
		t.Fatalf("new list size=%d", l.Size())
	}
	if err := l.Add(Entry{IP: "203.0.113.5", Reason: "a", CreatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	if l.Size() != 1 {
		t.Fatalf("size=%d", l.Size())
	}
}

func TestAddDuplicateUpdatesNotInserts(t *testing.T) {
	l := NewList()
	// 先加一条 CreatedAt=100
	if err := l.Add(Entry{IP: "203.0.113.5", Reason: "first", Source: "s1", CreatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	// 再加同样的 IP，CreatedAt=200（更晚）
	if err := l.Add(Entry{IP: "203.0.113.5", Reason: "second", Source: "s2", CreatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	if l.Size() != 1 {
		t.Fatalf("重复 IP 不应新增, size=%d", l.Size())
	}
	got, ok := l.Get("203.0.113.5")
	if !ok {
		t.Fatal("应仍能取到该 IP")
	}
	// 保留最早 CreatedAt
	if got.CreatedAt != 100 {
		t.Fatalf("应保留最早 CreatedAt=100, got %d", got.CreatedAt)
	}
	// reason/source 更新为新值
	if got.Reason != "second" || got.Source != "s2" {
		t.Fatalf("重复加入应刷新字段, got %+v", got)
	}
}

func TestAddRejectsFullNetwork(t *testing.T) {
	l := NewList()
	if err := l.Add(Entry{IP: "0.0.0.0/0"}); err == nil {
		t.Fatal("Add 封全网必须拒绝")
	}
	if l.Size() != 0 {
		t.Fatalf("被封全网的条目不应进表, size=%d", l.Size())
	}
}

func TestRemove(t *testing.T) {
	l := NewList()
	_ = l.Add(Entry{IP: "203.0.113.5", CreatedAt: 1})
	if !l.Remove("203.0.113.5") {
		t.Fatal("Remove 存在的 IP 应返回 true")
	}
	if l.Remove("203.0.113.5") {
		t.Fatal("重复 Remove 应返回 false")
	}
	if l.Size() != 0 {
		t.Fatalf("size=%d", l.Size())
	}
}

func TestAllSortedByCreatedDesc(t *testing.T) {
	l := NewList()
	_ = l.Add(Entry{IP: "203.0.113.1", CreatedAt: 100})
	_ = l.Add(Entry{IP: "203.0.113.2", CreatedAt: 50})
	_ = l.Add(Entry{IP: "203.0.113.3", CreatedAt: 300})
	all := l.All()
	if len(all) != 3 {
		t.Fatalf("len=%d", len(all))
	}
	// 降序：300, 100, 50
	if all[0].IP != "203.0.113.3" || all[1].IP != "203.0.113.1" || all[2].IP != "203.0.113.2" {
		t.Fatalf("按 CreatedAt 降序排序失败: %+v", all)
	}
}

func TestAddMany(t *testing.T) {
	l := NewList()
	added, skipped, errs := l.AddMany([]Entry{
		{IP: "203.0.113.5", Reason: "a"},
		{IP: "203.0.113.0/24", Reason: "b"},
		{IP: "0.0.0.0/0", Reason: "c"}, // 跳过
		{IP: "notanip", Reason: "d"},   // 跳过
	})
	if added != 2 {
		t.Fatalf("added=%d", added)
	}
	if skipped != 2 {
		t.Fatalf("skipped=%d", skipped)
	}
	if len(errs) != 2 {
		t.Fatalf("errs=%v", errs)
	}
	if l.Size() != 2 {
		t.Fatalf("size=%d", l.Size())
	}
}

func TestMatchIPSingle(t *testing.T) {
	entries := []Entry{
		{IP: "203.0.113.7", Reason: "x"},
		{IP: "198.51.100.9", Reason: "y"},
	}
	hit := MatchIP(entries, "203.0.113.7")
	if hit == nil {
		t.Fatal("单 IP 应命中")
	}
	if hit.IP != "203.0.113.7" {
		t.Fatalf("命中错误条目 %+v", hit)
	}
	// 精确单 IP：其他不相邻地址不应命中
	if MatchIP(entries, "203.0.113.8") != nil {
		t.Fatal("非封禁地址不应命中单 IP 条目")
	}
}

func TestMatchIPCIDR(t *testing.T) {
	entries := []Entry{
		{IP: "10.0.0.0/8", Reason: "net"},
		{IP: "192.168.1.0/24", Reason: "lan"},
	}
	// 10.0.0.5 命中 10.0.0.0/8
	hit := MatchIP(entries, "10.0.0.5")
	if hit == nil || hit.IP != "10.0.0.0/8" {
		t.Fatalf("10.0.0.5 应命中 10.0.0.0/8, got %+v", hit)
	}
	// 10.5.5.5 也命中 /8
	hit = MatchIP(entries, "10.5.5.5")
	if hit == nil || hit.IP != "10.0.0.0/8" {
		t.Fatalf("10.5.5.5 应在 /8 内, got %+v", hit)
	}
	// 11.0.0.1 不在 /8
	if MatchIP(entries, "11.0.0.1") != nil {
		t.Fatal("11.0.0.1 不应命中 10.0.0.0/8")
	}
	// CIDR 边界包含：192.168.1.50 命中 /24，192.168.2.1 不命中
	if MatchIP(entries, "192.168.1.50") == nil {
		t.Fatal("192.168.1.50 应命中 /24")
	}
	if MatchIP(entries, "192.168.2.1") != nil {
		t.Fatal("192.168.2.1 不应命中 /24")
	}
}

func TestMatchIPNoHit(t *testing.T) {
	entries := []Entry{{IP: "203.0.113.0/24"}}
	if MatchIP(entries, "8.8.8.8") != nil {
		t.Fatal("无关地址不应命中")
	}
	// 非法输入返回 nil
	if MatchIP(entries, "notanip") != nil {
		t.Fatal("非法 IP 输入应返回 nil")
	}
	if MatchIP(nil, "8.8.8.8") != nil {
		t.Fatal("空列表应返回 nil")
	}
}

// TestMatchIPV6 验证 IPv6 匹配（防回归：只有 v4 实现的容易漏）。
func TestMatchIPV6(t *testing.T) {
	entries := []Entry{{IP: "2001:db8::/32"}}
	if MatchIP(entries, "2001:db8:0:1::5") == nil {
		t.Fatal("2001:db8 前缀内的 v6 应命中")
	}
	// 2001:db9::/32 前缀不同（db9 vs db8），不应命中。
	if MatchIP(entries, "2001:db9::1") != nil {
		t.Fatal("2001:db9::1 不应命中 2001:db8::/32")
	}
	// 单 v6 地址匹配
	entries2 := []Entry{{IP: "2001:db8::5"}}
	if MatchIP(entries2, "2001:db8::5") == nil {
		t.Fatal("单 v6 地址应命中")
	}
	if MatchIP(entries2, "2001:db8::6") != nil {
		t.Fatal("非封禁 v6 地址不应命中单地址条目")
	}
}
