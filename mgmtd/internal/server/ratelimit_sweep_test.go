package server

import (
	"fmt"
	"testing"
	"time"
)

// 限流表必须自我回收。原实现每个失败源 IP 都留一条永不删除的记录：
// 路由器常驻数月，扫描器换着源 IP 打，map 只增不减 → 慢速内存泄漏。
func TestLoginLimiterSweepsExpiredBuckets(t *testing.T) {
	l := newLoginLimiter(5, 50*time.Millisecond)
	for i := 0; i < 20; i++ {
		l.recordFailure(fmt.Sprintf("10.0.0.%d", i))
	}
	if got := l.size(); got != 20 {
		t.Fatalf("记录 20 个源 IP 后应为 20，得 %d", got)
	}
	time.Sleep(80 * time.Millisecond) // 超过窗口
	// 任意一次 allow 触发清扫
	l.allow("192.168.1.1")
	if got := l.size(); got != 0 {
		t.Fatalf("窗口过期后应清空全部桶，得 %d", got)
	}
}

// 极端情况（大量不同源 IP 同时失败）下条目数必须有界。
func TestLoginLimiterBoundedSize(t *testing.T) {
	l := newLoginLimiter(5, time.Hour)
	l.lastSweep = time.Now() // 抑制清扫，专测上限淘汰
	for i := 0; i < maxLimiterEntries+500; i++ {
		l.recordFailure(fmt.Sprintf("10.%d.%d.%d", i>>16&0xff, i>>8&0xff, i&0xff))
	}
	// recordFailure 内部会 sweep，但窗口是 1 小时，没有过期桶可删
	if got := l.size(); got > maxLimiterEntries {
		t.Fatalf("条目数应被限制在 %d 以内，得 %d", maxLimiterEntries, got)
	}
}

// 未过窗口的桶不能在被淘汰前丢失限流能力。
func TestLoginLimiterStillBlocksWithinWindow(t *testing.T) {
	l := newLoginLimiter(3, time.Hour)
	ip := "10.9.9.9"
	for i := 0; i < 3; i++ {
		if !l.allow(ip) {
			t.Fatalf("第 %d 次前不应被拦", i+1)
		}
		l.recordFailure(ip)
	}
	if l.allow(ip) {
		t.Fatal("超过 3 次失败后应被拦")
	}
	// reset 后恢复
	l.reset(ip)
	if !l.allow(ip) {
		t.Fatal("reset 后应恢复放行")
	}
}
