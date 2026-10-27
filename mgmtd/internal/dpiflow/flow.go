// Package dpiflow 把 nDPI/启发式分类从"离线单包识别"升级为"按流跟踪的应用识别"。
//
// 核心是 Tracker：
//   - 双向流归一：客户端→服务端 与 服务端→客户端 的包必须落到同一条 Flow（见 normalizedTuple）。
//   - 生命周期：Observe 累积字节/包数与时间，EvictIdle 按空闲超时清理（防止 map 无限增长）。
//   - DPI 结果后置回填：识别出应用后调用 MarkDetected 写回。
package dpiflow

import (
	"sort"
	"strconv"
	"sync"
	"time"
)

// Packet 是进入数据面的单个包，归一化后交给 Tracker。
type Packet struct {
	SrcIP   string // 源地址
	DstIP   string // 目的地址
	SrcPort int    // 源端口
	DstPort int    // 目的端口
	Proto   string // "tcp" / "udp" / "icmp"
	Payload []byte // 应用层载荷（识别用）
	Inbound bool   // 是否入站方向
	Ts      int64  // 到达时间（ms）
}

// FlowKey 唯一标识一条流（两端已按字典序归一化为低/高端）。
type FlowKey struct {
	SrcIP   string
	DstIP   string
	SrcPort int
	DstPort int
	Proto   string
}

// Flow 是 Tracker 里一条被跟踪的流。
type Flow struct {
	Key        FlowKey `json:"key"`
	App        string  `json:"app"`
	Category   string  `json:"category"`
	BytesIn    uint64  `json:"bytes_in"`
	BytesOut   uint64  `json:"bytes_out"`
	PacketsIn  uint64  `json:"packets_in"`
	PacketsOut uint64  `json:"packets_out"`
	FirstSeen  int64   `json:"first_seen"`
	LastSeen   int64   `json:"last_seen"`
	Detected   bool    `json:"detected"`
}

// Tracker 维护流表，带并发保护与上限。
type Tracker struct {
	mu         sync.RWMutex
	flows      map[string]*Flow
	maxEntries int
	evicted    uint64 // 累计因超时或超限淘汰的流数
}

// NewTracker 返回容量上限为 max 的跟踪器；max<=0 时默认 10000。
func NewTracker(max int) *Tracker {
	if max <= 0 {
		max = 10000
	}
	return &Tracker{flows: make(map[string]*Flow), maxEntries: max}
}

// endpoint 是一个 ip:port 端点。
type endpoint struct {
	ip   string
	port int
}

func (e endpoint) String() string { return e.ip + ":" + strconv.Itoa(e.port) }

// normalizedTuple 把 (src, dst) 两个端点按字典序排序。
//
// 为什么要排序：客户端→服务端 与 服务端→客户端 是同一连接的两个反向包，
// 若不排序，反方向的 endpoint 会交换，key 就不同，导致同一条流被拆成两条"半流"，
// 字节/包数与 DPI 结果都对不上。排序后无论从哪个方向到达都落到同一条 Flow。
func normalizedTuple(p Packet) (low, high endpoint) {
	a := endpoint{p.SrcIP, p.SrcPort}
	b := endpoint{p.DstIP, p.DstPort}
	if a.String() > b.String() {
		a, b = b, a
	}
	return a, b
}

// flowKeyOf 由 Packet 生成唯一 key。
func flowKeyOf(p Packet) string {
	low, high := normalizedTuple(p)
	// 端口可能为 0（如 icmp），用字符串拼避免与边界歧义。
	return low.ip + ":" + strconv.Itoa(low.port) + "|" +
		high.ip + ":" + strconv.Itoa(high.port) + "|" + p.Proto
}

// KeyString 暴露给数据面，在 MarkDetected 前计算同一 key。
func (t *Tracker) KeyString(p Packet) string { return flowKeyOf(p) }

// Observe 记录一个包并返回对应 Flow（不存在则新建）。
// 新流 Detected=false；累计字节/包数，更新 LastSeen。
func (t *Tracker) Observe(pkt Packet) *Flow {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := flowKeyOf(pkt)
	f, ok := t.flows[key]
	if !ok {
		// 达到上限时逐出最老的流，保证 map 有界。
		if len(t.flows) >= t.maxEntries {
			t.evictLRU()
		}
		now := pkt.Ts
		if now <= 0 {
			now = time.Now().UnixMilli()
		}
		low, high := normalizedTuple(pkt)
		f = &Flow{
			Key: FlowKey{
				SrcIP: low.ip, SrcPort: low.port,
				DstIP: high.ip, DstPort: high.port,
				Proto: pkt.Proto,
			},
			FirstSeen: now,
			LastSeen:  now,
		}
		t.flows[key] = f
	}
	if pkt.Inbound {
		f.BytesIn += uint64(len(pkt.Payload))
		f.PacketsIn++
	} else {
		f.BytesOut += uint64(len(pkt.Payload))
		f.PacketsOut++
	}
	if pkt.Ts > 0 {
		if f.FirstSeen == 0 || pkt.Ts < f.FirstSeen {
			f.FirstSeen = pkt.Ts
		}
		if pkt.Ts > f.LastSeen {
			f.LastSeen = pkt.Ts
		}
	}
	return f
}

// MarkDetected 在被 DPI 识别后回填应用与分类。key 由 KeyString/Observe 路径生成。
func (t *Tracker) MarkDetected(key string, app, category string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f, ok := t.flows[key]; ok {
		f.App = app
		f.Category = category
		f.Detected = true
	}
}

// List 按 LastSeen 降序返回最多 limit 条流的快照（limit<=0 返回全部）。
func (t *Tracker) List(limit int) []Flow {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Flow, 0, len(t.flows))
	for _, f := range t.flows {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// EvictIdle 清理超过 idleSec 秒未见新包的流，返回清理数量。
// 这是防止 map 无限增长的关键回收路径；调用方应周期性调用（如数据面 tick/cron）。
func (t *Tracker) EvictIdle(now int64, idleSec int64) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	cutoff := now - idleSec*1000
	n := 0
	for k, f := range t.flows {
		if f.LastSeen < cutoff {
			delete(t.flows, k)
			n++
		}
	}
	t.evicted += uint64(n)
	return n
}

// Snapshot 按 App 分组统计流数量。
func (t *Tracker) Snapshot() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m := make(map[string]int)
	for _, f := range t.flows {
		app := f.App
		if app == "" {
			app = "Unknown"
		}
		m[app]++
	}
	return m
}

// Stats 返回追踪器统计（供 /dpi/tracker-stats）。
func (t *Tracker) Stats() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return map[string]int{
		"total":   t.maxEntries,
		"tracked": len(t.flows),
		"evicted": int(t.evicted),
		"max":     t.maxEntries,
	}
}

// evictLRU 在达到上限时逐出 LastSeen 最老的流（仅在持锁下调用）。
func (t *Tracker) evictLRU() {
	var oldestKey string
	var oldest int64 = -1
	for k, f := range t.flows {
		if oldest == -1 || f.LastSeen < oldest {
			oldest = f.LastSeen
			oldestKey = k
		}
	}
	if oldestKey != "" {
		delete(t.flows, oldestKey)
		t.evicted++
	}
}
