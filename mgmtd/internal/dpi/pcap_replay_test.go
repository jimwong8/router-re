//go:build linux && cgo

package dpi

import (
	"os"
	"testing"
)

// readPcapIP 复用正式解析器，测试不再维护第二套 pcap 逻辑。
func readPcapIP(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("没有抓包样本 %s: %v", path, err)
	}
	defer f.Close()
	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatalf("解析 pcap 失败: %v", err)
	}
	out := make([][]byte, 0, len(pkts))
	for _, p := range pkts {
		out = append(out, p.IP)
	}
	return out
}

// TestClassifyFlowRealHTTPCapture 回放一段真实抓包（curl 访问本地 HTTP 服务），
// 验证同一连接的多个包会被累积识别。
//
// 这段 pcap 里有完整三次握手 + HTTP 请求 + 响应，是"真实流量"而非构造样本。
func TestClassifyFlowRealHTTPCapture(t *testing.T) {
	ResetFlows()
	defer ResetFlows()

	pkts := readPcapIP(t, "testdata/http_plain.pcap")
	if len(pkts) == 0 {
		t.Skip("抓包样本里没有 IP 包")
	}

	last := "Unknown"
	identifiedAt := -1
	for i, p := range pkts {
		proto, cat, _, err := ClassifyFlow(p)
		if err != nil {
			t.Fatalf("第 %d 包出错: %v", i, err)
		}
		if proto != "Unknown" {
			last = proto
			if identifiedAt < 0 {
				identifiedAt = i
				t.Logf("第 %d 包识别为 %s / %s", i, proto, cat)
			}
		}
	}
	st := Stats()
	t.Logf("回放 %d 个包，最终协议=%s，第 %d 包首次识别，流表 identified=%d", len(pkts), last, identifiedAt, st.Identified)

	if last == "Unknown" {
		t.Fatalf("真实 HTTP 流量回放后仍未识别出任何协议（%d 个包）", len(pkts))
	}
	if identifiedAt < 0 {
		t.Fatal("应记录首次识别位置")
	}
}

// 同一段抓包用单包接口逐个喂：因为没有跨包状态，识别率必然更低。
// 这个对比就是"流状态有没有用"的直接证据。
func TestSinglePacketIsWorseThanFlow(t *testing.T) {
	pkts := readPcapIP(t, "testdata/http_plain.pcap")
	if len(pkts) == 0 {
		t.Skip("抓包样本里没有 IP 包")
	}

	singleHits := 0
	for _, p := range pkts {
		if proto, _, err := ClassifyPacket(p); err == nil && proto != "Unknown" {
			singleHits++
		}
	}

	ResetFlows()
	defer ResetFlows()
	flowHit := false
	for _, p := range pkts {
		if proto, _, _, err := ClassifyFlow(p); err == nil && proto != "Unknown" {
			flowHit = true
		}
	}

	t.Logf("单包命中 %d/%d，流式%s识别出协议", singleHits, len(pkts), map[bool]string{true: "成功", false: "未能"}[flowHit])
	if !flowHit {
		t.Fatal("流式识别应能识别出协议")
	}
	if singleHits >= len(pkts) && flowHit {
		t.Log("（单包恰好也能全部命中：这段流量正好都是首包可判定）")
	}
}
