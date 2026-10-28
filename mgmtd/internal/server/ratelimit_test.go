package server

import (
	"testing"
	"time"
)

// 限流语义：**只限失败**。
//
// 这是从一次真实事故反推出来的规则：回归套件里 11 个脚本各登录一次，
// 第 6 个开始全部 429（ui_acceptance 就是这么挂的），因为旧实现在 allow()
// 里就 count++，成功登录照样扣配额。暴力破解防护要限的是失败次数。
func TestLoginLimiterCountsFailuresNotSuccesses(t *testing.T) {
	l := newLoginLimiter(3, time.Minute)
	// 成功登录 100 次也不该被限（allow 只检查、不消费，成功还 reset）
	for i := 0; i < 100; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("第 %d 次成功登录被误限流", i+1)
		}
		l.reset("1.2.3.4")
	}
}

func TestLoginLimiterBlocksAfterMaxFailures(t *testing.T) {
	l := newLoginLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4") {
			t.Fatalf("第 %d 次失败尝试应当被放行（还没到上限）", i+1)
		}
		l.recordFailure("1.2.3.4")
	}
	if l.allow("1.2.3.4") {
		t.Fatal("第 4 次失败尝试应当被拒绝")
	}
}

func TestLoginLimiterSeparatesIPs(t *testing.T) {
	l := newLoginLimiter(1, time.Minute)
	l.recordFailure("10.0.0.1")
	if l.allow("10.0.0.1") {
		t.Fatal("同一 IP 在失败达上限后应被拒")
	}
	if !l.allow("10.0.0.2") {
		t.Fatal("不同 IP 不该受牵连")
	}
}

func TestLoginLimiterWindowResets(t *testing.T) {
	l := newLoginLimiter(1, 50*time.Millisecond)
	l.recordFailure("1.1.1.1")
	if l.allow("1.1.1.1") {
		t.Fatal("窗口内应被拒")
	}
	time.Sleep(80 * time.Millisecond)
	if !l.allow("1.1.1.1") {
		t.Fatal("窗口过期后应重新放行")
	}
}

// 打错密码再打对：成功登录要清掉之前的失败计数，不能让他继续顶着配额。
func TestLoginLimiterSuccessResetsFailures(t *testing.T) {
	l := newLoginLimiter(2, time.Minute)
	l.recordFailure("5.5.5.5")
	l.recordFailure("5.5.5.5")
	if l.allow("5.5.5.5") {
		t.Fatal("两次失败后应被拒")
	}
	l.reset("5.5.5.5")
	if !l.allow("5.5.5.5") {
		t.Fatal("成功登录 reset 后应重新放行")
	}
}

func TestClientIPStripsPort(t *testing.T) {
	if got := clientIP("192.168.1.5:34567"); got != "192.168.1.5" {
		t.Fatalf("expected stripped ip, got %q", got)
	}
	if got := clientIP("192.168.1.5"); got != "192.168.1.5" {
		t.Fatalf("expected pass-through, got %q", got)
	}
}
