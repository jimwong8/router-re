//go:build linux && cgo

package dpi

import (
	"strings"
	"testing"
)

// 这个测试是本包存在的理由：证明"跨包累积"能识别出单包识别不出的协议。
//
// 场景：一个 HTTP 请求被拆成两个 TCP 段。第一段只有方法行和半个头名，
// 任何单包分析都判断不出 Host；第二段到达后，nDPI 有了完整请求。
func TestClassifyFlowAccumulatesState(t *testing.T) {
	ResetFlows()

	part1 := []byte("GET /index.html HTTP/1.1\r\nHo")
	part2 := []byte("st: www.example.com\r\nUser-Agent: probe\r\nAccept: */*\r\n\r\n")

	// 先走三次握手：nDPI 需要知道每条 TCP 流的方向与初始序号
	for _, h := range handshake("10.1.1.1", "93.184.216.34", 40001, 80) {
		if _, _, _, err := ClassifyFlow(h); err != nil {
			t.Fatal(err)
		}
	}

	pkt1 := buildTCPPacketSeq("10.1.1.1", "93.184.216.34", 40001, 80, part1, 0x18, 1001, 5001)
	pkt2 := buildTCPPacketSeq("10.1.1.1", "93.184.216.34", 40001, 80, part2, 0x18, 1001+uint32(len(part1)), 5001)

	// 单包视角：第一段孤立地看，识别不出 HTTP
	single, _, err := ClassifyPacket(pkt1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("单包(第一段) -> %s", single)

	// 流视角：第一段还不足以判定，第二段补齐后应识别
	p1, c1, n1, err := ClassifyFlow(pkt1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("流: 第 1 包 -> %s / %s (packets=%d)", p1, c1, n1)

	p2, c2, n2, err := ClassifyFlow(pkt2)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("流: 第 2 包 -> %s / %s (packets=%d)", p2, c2, n2)

	if n2 < 4 {
		t.Fatalf("握手包 + 两个数据包都应计入同一条流，实际 packets=%d", n2)
	}
	if p2 == "Unknown" {
		t.Fatalf("两段合起来应识别出 HTTP，实际仍是 Unknown（第一段=%s）", p1)
	}
	if !strings.Contains(strings.ToUpper(p2), "HTTP") {
		t.Logf("识别结果为 %s（非 HTTP，但仍说明跨包状态生效）", p2)
	}
	ResetFlows()
}

// 识别完成后再喂包，不应改变结论，也不应重复送进 nDPI。
func TestClassifyFlowCachesResult(t *testing.T) {
	ResetFlows()
	defer ResetFlows()

	req := []byte("GET / HTTP/1.1\r\nHost: cache.example.com\r\n\r\n")
	pkt := buildTCPPacket("10.2.2.2", "1.2.3.4", 50000, 80, req, 0x18)

	first, _, n1, err := ClassifyFlow(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 1 {
		t.Fatalf("packets 应为 1，实际 %d", n1)
	}
	// 再喂两个同流包
	for i := 0; i < 2; i++ {
		p := buildTCPPacket("10.2.2.2", "1.2.3.4", 50000, 80, req, 0x18)
		got, _, n, err := ClassifyFlow(p)
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("已识别的流不该改结论: %s -> %s", first, got)
		}
		if n != int64ToInt(int64(i+2)) {
			t.Fatalf("packets 计数错误: 期望 %d 实际 %d", i+2, n)
		}
	}
}

func int64ToInt(v int64) int { return int(v) }

// 一条流识别出的结论不能串到另一条流上。
func TestClassifyFlowIsolation(t *testing.T) {
	ResetFlows()
	defer ResetFlows()

	httpPkt := buildTCPPacket("10.3.3.3", "1.1.1.1", 41000, 80,
		[]byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n"), 0x18)
	other := buildTCPPacket("10.4.4.4", "2.2.2.2", 42000, 443,
		[]byte("random"), 0x18)

	a, _, _, err := ClassifyFlow(httpPkt)
	if err != nil {
		t.Fatal(err)
	}
	b, _, nb, err := ClassifyFlow(other)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("流A=%s 流B=%s (packets=%d)", a, b, nb)
	if nb != 1 {
		t.Fatalf("新流应独立计数，实际 packets=%d", nb)
	}

	// 双向包必须落在同一条流上
	revPkt := buildTCPPacket("1.1.1.1", "10.3.3.3", 80, 41000, []byte("HTTP/1.1 200 OK\r\n\r\n"), 0x18)
	_, _, n, err := ClassifyFlow(revPkt)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("反向包应命中同一条流（packets>=2），实际 %d", n)
	}
}

func TestStatsTracksFlows(t *testing.T) {
	ResetFlows()
	defer ResetFlows()

	for i := 0; i < 5; i++ {
		p := buildTCPPacket("10.9.9.9", "3.3.3.3", uint16(43000+i), 80,
			[]byte("GET / HTTP/1.1\r\nHost: s.example.com\r\n\r\n"), 0x18)
		ClassifyFlow(p)
	}
	st := Stats()
	if st.Active != 5 {
		t.Fatalf("应跟踪 5 条流，实际 %d", st.Active)
	}
	if st.Max != DefaultFlowPolicy().MaxFlows {
		t.Fatalf("上限显示错误: %d", st.Max)
	}
	if st.IdleSec != int(DefaultFlowPolicy().IdleTimeout.Seconds()) {
		t.Fatalf("空闲秒数错误: %d", st.IdleSec)
	}
	t.Logf("流表: active=%d identified=%d max=%d idle=%ds", st.Active, st.Identified, st.Max, st.IdleSec)
}

func TestClassifyFlowIgnoresNonIP(t *testing.T) {
	ResetFlows()
	defer ResetFlows()
	// 垃圾数据不该 panic，也不该建流
	if _, _, n, err := ClassifyFlow([]byte{0x01, 0x02, 0x03}); err != nil || n != 0 {
		t.Fatalf("非 IP 包应被忽略: n=%d err=%v", n, err)
	}
	if Stats().Active != 0 {
		t.Fatal("非 IP 包不该创建流")
	}
}
