package server

import (
	"net"
	"sync"
	"time"
)

// maxLimiterEntries 限制限流表的最大条目数。
//
// 没有这个上限时，每个新的失败源 IP 都会在 map 里留一条永不删除的记录：
// 路由器是常驻数月的进程，扫描器/爆破器换着源 IP 打，map 只增不减，
// 属于慢速内存泄漏（也方便攻击者用大量不同源 IP 把内存吃满）。
const maxLimiterEntries = 8192

// loginLimiter is a fixed-window per-IP rate limiter for the login endpoint.
// It is intentionally simple and in-memory: on a small router daemon it
// protects against one-host brute force without pulling in a cache backend.
type loginLimiter struct {
	mu        sync.Mutex
	max       int           // allowed attempts per IP per window
	window    time.Duration // sliding window length
	attempts  map[string]*bucket
	lastSweep time.Time // 上次清理过期桶的时间
}

type bucket struct {
	count   int
	started time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		max:      max,
		window:   window,
		attempts: make(map[string]*bucket),
	}
}

// enforceCapLocked 保证条目数有界。调用方必须持有 l.mu。
//
// 必须在"插入之后"调用：先清扫再插入会多留一条（实测 8193 > 8192）。
func (l *loginLimiter) enforceCapLocked() {
	for len(l.attempts) > maxLimiterEntries {
		var oldestIP string
		var oldest time.Time
		first := true
		for ip, b := range l.attempts {
			if first || b.started.Before(oldest) {
				oldestIP, oldest, first = ip, b.started, false
			}
		}
		delete(l.attempts, oldestIP)
	}
}

// sweepLocked 回收过期桶并施加条目上限。调用方必须持有 l.mu。
//
// 过期扫描只在窗口到期后做一次（每个请求都全表遍历在条目多时是浪费）；
// 条目上限则每次都要检查——否则窗口很长时上限永远不生效。
func (l *loginLimiter) sweepLocked(now time.Time) {
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		l.lastSweep = now
		for ip, b := range l.attempts {
			if now.Sub(b.started) >= l.window {
				delete(l.attempts, ip)
			}
		}
	}
}

// allow 报告 ip 当前是否还有失败尝试的余量。**只检查、不消费**——
// 早期版本在这里就 count++，于是「成功登录也扣配额」：回归套件里 11 个脚本
// 各登录一次，第 6 个开始全部 429（实测 ui_acceptance 就是这么挂的）。
// 暴力破解防护要限的是失败次数，不是成功次数。
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweepLocked(now)
	b, ok := l.attempts[ip]
	if !ok || now.Sub(b.started) >= l.window {
		return true
	}
	return b.count < l.max
}

// recordFailure 记一次失败尝试。窗口过期则开新桶。
func (l *loginLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweepLocked(now)
	b, ok := l.attempts[ip]
	if !ok || now.Sub(b.started) >= l.window {
		l.attempts[ip] = &bucket{count: 1, started: now}
		l.enforceCapLocked()
		return
	}
	b.count++
}

// reset 在登录成功后清空该 IP 的失败计数：用户可能是先打错密码再打对，
// 不该让之前的失败继续压着他的配额。
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// size 返回当前跟踪的 IP 数（测试与诊断用）。
func (l *loginLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.attempts)
}

// clientIP extracts the remote IP (stripping port) for rate-limit bucketing.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
