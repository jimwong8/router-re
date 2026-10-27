package dpiflow

import (
	"testing"
	"time"
)

func pkt(srcIP string, srcPort int, dstIP string, dstPort int, proto string, payload []byte, inbound bool, ts int64) Packet {
	return Packet{SrcIP: srcIP, SrcPort: srcPort, DstIP: dstIP, DstPort: dstPort, Proto: proto, Payload: payload, Inbound: inbound, Ts: ts}
}

// 双向归一：正反两个方向 observe 同一个连接，Tracker 里只有 1 条流。
func TestBidirectionalNormalization(t *testing.T) {
	tr := NewTracker(10)
	now := time.Now().UnixMilli()
	// 客户端 1.1.1.1:12345 → 服务端 2.2.2.2:80
	tr.Observe(pkt("1.1.1.1", 12345, "2.2.2.2", 80, "tcp", []byte("GET / HTTP/1.1\r\n"), false, now))
	// 反向：服务端 → 客户端（必须落到同一条流）
	tr.Observe(pkt("2.2.2.2", 80, "1.1.1.1", 12345, "tcp", []byte("HTTP/1.1 200 OK\r\n"), true, now+10))

	if n := len(tr.List(100)); n != 1 {
		t.Fatalf("expected 1 flow after both directions, got %d", n)
	}
	flow := tr.List(100)[0]
	if flow.BytesIn == 0 || flow.BytesOut == 0 {
		t.Fatalf("both directions must be aggregated, got in=%d out=%d", flow.BytesIn, flow.BytesOut)
	}
	if flow.PacketsIn != 1 || flow.PacketsOut != 1 {
		t.Fatalf("packet counts wrong: in=%d out=%d", flow.PacketsIn, flow.PacketsOut)
	}
}

// EvictIdle 真实清理超时流。
func TestEvictIdle(t *testing.T) {
	tr := NewTracker(100)
	now := int64(1000)
	tr.Observe(pkt("1.1.1.1", 1, "2.2.2.2", 80, "tcp", nil, false, now))       // LastSeen=1000
	tr.Observe(pkt("3.3.3.3", 1, "4.4.4.4", 443, "tcp", nil, false, now+2500)) // LastSeen=3500
	tr.Observe(pkt("5.5.5.5", 1, "6.6.6.6", 53, "udp", nil, false, now+2600))  // LastSeen=3600

	// idleSec=5s：now=4000 时 cutoff=4000-5000=-1000，都不淘汰。
	if n := tr.EvictIdle(now+3000, 5); n != 0 {
		t.Fatalf("expected 0 evicted (recent), got %d", n)
	}
	// idleSec=1s：now=2500 时 cutoff=2500-1000=1500。
	// 只有第一条 LastSeen=1000<1500 被淘汰；第二三条（3500/3600）保留。
	if n := tr.EvictIdle(now+1500, 1); n != 1 {
		t.Fatalf("expected 1 evicted (oldest flow idle >1s), got %d", n)
	}
	if len(tr.List(100)) != 2 {
		t.Fatalf("expected 2 flows remaining, got %d", len(tr.List(100)))
	}
	// 被淘汰的是最老的（1.1.1.1）
	for _, f := range tr.List(100) {
		if f.Key.SrcIP == "1.1.1.1" {
			t.Fatal("oldest flow should have been evicted")
		}
	}
}

// maxEntries 保护：超过上限时逐出最老的流。
func TestMaxEntries(t *testing.T) {
	tr := NewTracker(2)
	now := int64(1000)
	tr.Observe(pkt("1.1.1.1", 1, "2.2.2.2", 80, "tcp", nil, false, now))
	tr.Observe(pkt("3.3.3.3", 1, "4.4.4.4", 443, "tcp", nil, false, now+10))
	// 第三次观察：达到上限，应逐出第一条（最老）。
	tr.Observe(pkt("5.5.5.5", 1, "6.6.6.6", 53, "udp", nil, false, now+20))

	if len(tr.List(100)) != 2 {
		t.Fatalf("expected 2 flows after cap, got %d", len(tr.List(100)))
	}
	flows := tr.List(100)
	// 被淘汰的是最老的（1.1.1.1:1<->2.2.2.2:80）
	for _, f := range flows {
		if f.Key.SrcIP == "1.1.1.1" {
			t.Fatalf("oldest flow should have been evicted, still present: %v", f.Key)
		}
	}
	if tr.Stats()["evicted"] != 1 {
		t.Fatalf("evicted counter wrong: %d", tr.Stats()["evicted"])
	}
}

// Snapshot 按 App 分组统计。
func TestSnapshot(t *testing.T) {
	tr := NewTracker(100)
	now := int64(1000)
	k1 := tr.KeyString(pkt("1.1.1.1", 1, "2.2.2.2", 80, "tcp", nil, false, now))
	k2 := tr.KeyString(pkt("3.3.3.3", 1, "4.4.4.4", 443, "tcp", nil, false, now))
	tr.Observe(pkt("1.1.1.1", 1, "2.2.2.2", 80, "tcp", nil, false, now))
	tr.Observe(pkt("3.3.3.3", 1, "4.4.4.4", 443, "tcp", nil, false, now))
	tr.MarkDetected(k1, "http", "Web")
	tr.MarkDetected(k2, "http", "Web")
	s := tr.Snapshot()
	if s["http"] != 2 {
		t.Fatalf("expected 2 http flows, got %v", s)
	}
}

// MarkDetected 后置回填应用。
func TestMarkDetected(t *testing.T) {
	tr := NewTracker(100)
	now := int64(1000)
	k := tr.KeyString(pkt("1.1.1.1", 1, "2.2.2.2", 443, "tcp", nil, false, now))
	tr.Observe(pkt("1.1.1.1", 1, "2.2.2.2", 443, "tcp", nil, false, now))
	flow := tr.List(100)[0]
	if flow.Detected {
		t.Fatal("new flow should be Detected=false")
	}
	tr.MarkDetected(k, "https", "Encrypted")
	flow = tr.List(100)[0]
	if !flow.Detected || flow.App != "https" || flow.Category != "Encrypted" {
		t.Fatalf("mark failed: %+v", flow)
	}
}

// List 按 LastSeen 降序。
func TestListOrder(t *testing.T) {
	tr := NewTracker(100)
	tr.Observe(pkt("1.1.1.1", 1, "2.2.2.2", 80, "tcp", nil, false, 100))
	tr.Observe(pkt("3.3.3.3", 1, "4.4.4.4", 443, "tcp", nil, false, 200))
	flows := tr.List(100)
	if len(flows) != 2 {
		t.Fatalf("want 2, got %d", len(flows))
	}
	if flows[0].Key.SrcIP != "3.3.3.3" { // 更新的在前
		t.Fatalf("list not sorted by LastSeen desc: first=%v", flows[0].Key)
	}
}
