// Package accounting 提供认证计费的真实流量统计：把 conntrack 的字节计数
// 归到账号上，生成配额使用报表。它是 internal/account 的"真实累计"补充——
// account.UsedMB 是配置字段（静态），本包用 conntrack 字节数动态累计。
//
// 设计取舍（沿用 internal/conntrack 的注释风格）：
//   - 流量归属按"IP -> 账号"绑定关系把连接字节数归到用户；只按**源 IP** 归属，
//     避免一条连接在请求/回复双向都计数导致翻倍。
//   - 密度换算：1 MB = 1024*1024 字节，向下取整。
//   - 数据源是 /proc/net/nf_conntrack 的 bytes= 字段，依赖 nf_conntrack_acct。
package accounting

import "sync"

// Counter 按账号累计流量（字节级），数据来自 conntrack 的字节计数。
type Counter struct {
	mu       sync.RWMutex
	used     map[string]uint64 // username -> bytes
	lastSeen map[string]int64  // username -> unix ts
}

// NewCounter 创建一个空计数器。
func NewCounter() *Counter {
	return &Counter{
		used:     make(map[string]uint64),
		lastSeen: make(map[string]int64),
	}
}

// Add 累加某账号的字节数（由上层按 IP 归属调用）。
// now 可为 0（调用方不关心最近活跃时间），否则更新 lastSeen。
func (c *Counter) Add(username string, bytes uint64, now int64) {
	if username == "" || bytes == 0 {
		return
	}
	c.mu.Lock()
	c.used[username] += bytes
	if now > 0 {
		if cur, ok := c.lastSeen[username]; !ok || now > cur {
			c.lastSeen[username] = now
		}
	}
	c.mu.Unlock()
}

// Snapshot 返回 username -> UsedMB。1MB = 1024*1024 字节，向下取整。
func (c *Counter) Snapshot() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int, len(c.used))
	for u, b := range c.used {
		out[u] = int(b / (1024 * 1024))
	}
	return out
}

// Reset 清零某账号的用量。返回是否该账号确实存在并清零；
// 未知用户返回 false 且不做改动。
func (c *Counter) Reset(username string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.used[username]; !ok {
		return false
	}
	delete(c.used, username)
	delete(c.lastSeen, username)
	return true
}

// LastSeen 返回某账号最近活跃时间戳（Add 时 now>0 才会记录）。
func (c *Counter) LastSeen(username string) int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSeen[username]
}

// Exceeded 返回该账号是否已超过配额。quotaMB<=0 视为不限，恒返回 false；
// 否则 used 以 MB 计，used>=quotaMB 即视为超额（边界含等号）。
func (c *Counter) Exceeded(username string, quotaMB int) bool {
	if quotaMB <= 0 {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	usedMB := int(c.used[username] / (1024 * 1024))
	return usedMB >= quotaMB
}
