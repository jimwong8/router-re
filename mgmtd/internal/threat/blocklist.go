package threat

import (
	"fmt"
	"net/netip"
	"sort"
	"sync"
)

// List 是内存中的封禁表，线程安全。以 IP 文本为主键。
//
// 设计取舍：用 map[string]Entry 而不是 []Entry——Add 的"重复 IP 更新而不重复插入"
// 和 Remove 都要求 O(1) 按 IP 定位，map 最直接；All() 负责排序输出。
type List struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewList 创建一个空封禁表。
func NewList() *List {
	return &List{entries: make(map[string]Entry)}
}

// Add 加入一条封禁。重复 IP 时更新条目但**保留最早 CreatedAt**（防止延长刷新把
// 封禁起始时间推后导致实际封禁时长小于预期）。返回 error（含校验失败、封全网）。
func (l *List) Add(e Entry) error {
	if err := ValidateEntry(e); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.entries[e.IP]; ok {
		// 保留最早 CreatedAt；其余字段按新值刷新。
		e.CreatedAt = existing.CreatedAt
		// 若新条目是永久(0)而旧的是临时的，视为延长到永久；保留最早创建时间不变。
	}
	l.entries[e.IP] = e
	return nil
}

// Remove 移除指定 IP，返回是否存在并移除。
func (l *List) Remove(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.entries[ip]; !ok {
		return false
	}
	delete(l.entries, ip)
	return true
}

// Get 查询指定 IP 的条目。
func (l *List) Get(ip string) (Entry, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	e, ok := l.entries[ip]
	return e, ok
}

// All 返回全部条目，按 CreatedAt 降序（新的在前）。tie-break 用 IP 保证稳定。
func (l *List) All() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Entry, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt > out[j].CreatedAt
		}
		return out[i].IP < out[j].IP
	})
	return out
}

// Size 返回当前条目数。
func (l *List) Size() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// AddMany 批量加入。返回 (成功新增数, 因校验失败被跳过数, 错误列表)。
// 一条失败不影响其余加入——批量导入情报源时常有坏行，不应整批拒绝。
func (l *List) AddMany(entries []Entry) (added int, skipped int, errs []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range entries {
		if err := ValidateEntry(e); err != nil {
			skipped++
			errs = append(errs, fmt.Sprintf("%s: %v", e.IP, err))
			continue
		}
		if existing, ok := l.entries[e.IP]; ok {
			e.CreatedAt = existing.CreatedAt // 保留最早
		}
		l.entries[e.IP] = e
		added++
	}
	return added, skipped, errs
}

// MatchIP 判断 ip 是否命中封禁列表中的任意条目，返回命中的那条。
// 同时支持单 IP 与 CIDR：对每条封禁先解析成 netip.Prefix，再用 Contains 判断——
// **这是关键逻辑**，不能只做字符串相等（10.0.0.5 必须命中 10.0.0.0/8）。
// ip 传入单个地址。返回 nil 表示不命中。
func MatchIP(entries []Entry, ip string) *Entry {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return nil
	}
	for i := range entries {
		e := &entries[i]
		prefix, perr := parseEntryPrefix(e.IP)
		if perr != nil {
			continue
		}
		if prefix.Contains(addr) {
			return e
		}
	}
	return nil
}

// parseEntryPrefix 把条目 IP（单 IP 或 CIDR）解析成 netip.Prefix。
func parseEntryPrefix(ip string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(ip); err == nil {
		return prefix, nil
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Prefix{}, err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits), nil
}
