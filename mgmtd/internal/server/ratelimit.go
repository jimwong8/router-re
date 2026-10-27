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

// allow reports whether an attempt from ip should be permitted, and records
// the attempt if it is. It rotates buckets out of the window opportunistically.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.attempts[ip]
	if !ok || now.Sub(b.started) >= l.window {
		// New window: reset bucket.
		l.attempts[ip] = &bucket{count: 1, started: now}
		return true
	}
	if b.count >= l.max {
		return false
	}
	b.count++
	return true
}

// clientIP extracts the remote IP (stripping port) for rate-limit bucketing.
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
