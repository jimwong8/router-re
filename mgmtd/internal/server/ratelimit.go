package server

import (
	"net"
	"sync"
	"time"
)

// loginLimiter is a fixed-window per-IP rate limiter for the login endpoint.
// It is intentionally simple and in-memory: on a small router daemon it
// protects against one-host brute force without pulling in a cache backend.
type loginLimiter struct {
	mu       sync.Mutex
	max      int           // allowed attempts per IP per window
	window   time.Duration // sliding window length
	attempts map[string]*bucket
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

// allow 报告 ip 当前是否还有失败尝试的余量。**只检查、不消费**——
// 早期版本在这里就 count++，于是「成功登录也扣配额」：回归套件里 11 个脚本
// 各登录一次，第 6 个开始全部 429（实测 ui_acceptance 就是这么挂的）。
// 暴力破解防护要限的是失败次数，不是成功次数。
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
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
	b, ok := l.attempts[ip]
	if !ok || now.Sub(b.started) >= l.window {
		l.attempts[ip] = &bucket{count: 1, started: now}
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

// clientIP extracts the remote IP (stripping port) for rate-limit bucketing.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
