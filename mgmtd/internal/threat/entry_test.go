package threat

import (
	"strings"
	"testing"
)

func TestValidateEntryOK(t *testing.T) {
	cases := []Entry{
		{IP: "203.0.113.5", Reason: "scan", CreatedAt: 100, ExpiresAt: 200},
		{IP: "203.0.113.0/24", Reason: "netblock", CreatedAt: 100},
		{IP: "2001:db8::1", Reason: "v6", CreatedAt: 100, ExpiresAt: 200},
		{IP: "2001:db8::/32", Reason: "v6 cidr", CreatedAt: 100},
	}
	for _, c := range cases {
		if err := ValidateEntry(c); err != nil {
			t.Fatalf("合法条目 %q 报错: %v", c.IP, err)
		}
	}
}

// TestValidateEntryRejectsFullNetwork —— 最重要的红线测试。
func TestValidateEntryRejectsFullNetwork(t *testing.T) {
	for _, ip := range []string{"0.0.0.0/0", "::/0", "0.0.0.0", "::"} {
		err := ValidateEntry(Entry{IP: ip, Reason: "oops"})
		if err == nil {
			t.Fatalf("封全网 %q 必须被拒绝", ip)
		}
		if !strings.Contains(err.Error(), "锁死") && !strings.Contains(err.Error(), "阻断") {
			t.Fatalf("拒绝理由需说明为何危险: ip=%s err=%v", ip, err)
		}
	}
}

func TestValidateEntryBadIP(t *testing.T) {
	for _, ip := range []string{"", "banana", "10.0.0.0/33", "999.1.1.1"} {
		if err := ValidateEntry(Entry{IP: ip}); err == nil {
			t.Fatalf("非法 IP %q 应被拒绝", ip)
		}
	}
}

func TestValidateEntryReasonTooLong(t *testing.T) {
	long := strings.Repeat("x", maxReasonLen+1)
	err := ValidateEntry(Entry{IP: "203.0.113.5", Reason: long})
	if err == nil {
		t.Fatal("超长 reason 应被拒绝")
	}
	// 边界：恰好 maxReasonLen 应通过
	ok := strings.Repeat("x", maxReasonLen)
	if err := ValidateEntry(Entry{IP: "203.0.113.5", Reason: ok}); err != nil {
		t.Fatalf("恰好上限的 reason 应通过, got %v", err)
	}
}

func TestValidateEntryExpiresBeforeCreated(t *testing.T) {
	err := ValidateEntry(Entry{IP: "203.0.113.5", CreatedAt: 200, ExpiresAt: 100})
	if err == nil {
		t.Fatal("expires_at < created_at 应被拒绝")
	}
	// 相等也应拒绝
	if err := ValidateEntry(Entry{IP: "203.0.113.5", CreatedAt: 100, ExpiresAt: 100}); err == nil {
		t.Fatal("expires_at == created_at 应被拒绝")
	}
}

func TestExpiredBoundary(t *testing.T) {
	// ExpiresAt=0 永久不过期
	if Expired(Entry{ExpiresAt: 0}, 999999) {
		t.Fatal("expires=0 应永久不过期")
	}
	// 正好等于 ExpiresAt 算过期
	if !Expired(Entry{ExpiresAt: 100}, 100) {
		t.Fatal("now == expires_at 应算过期")
	}
	// 未到不过期
	if Expired(Entry{ExpiresAt: 100}, 99) {
		t.Fatal("now < expires_at 不应过期")
	}
	// 超过则过期
	if !Expired(Entry{ExpiresAt: 100}, 101) {
		t.Fatal("now > expires_at 应过期")
	}
}

func TestPruneRemovesExpired(t *testing.T) {
	entries := []Entry{
		{IP: "203.0.113.1", ExpiresAt: 0},   // 永久保留
		{IP: "203.0.113.2", ExpiresAt: 50},  // 过期
		{IP: "203.0.113.3", ExpiresAt: 100}, // 过期（边界）
		{IP: "203.0.113.4", ExpiresAt: 200}, // 保留
	}
	got, removed := Prune(entries, 100)
	if removed != 2 {
		t.Fatalf("应移除 2 条, got %d", removed)
	}
	if len(got) != 2 {
		t.Fatalf("应剩 2 条, got %d", len(got))
	}
	for _, e := range got {
		if e.IP != "203.0.113.1" && e.IP != "203.0.113.4" {
			t.Fatalf("不应保留 %s", e.IP)
		}
	}
	// 输入切片不被修改
	if len(entries) != 4 {
		t.Fatalf("Prune 不应修改入参, got %d", len(entries))
	}
}
