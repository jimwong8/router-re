package dpi

import (
	"encoding/binary"
	"testing"
)

// TestClassifyPacketTCP443 用最小合法 IPv4+TCP 包（目的端口 443）调用 ClassifyPacket，
// 验证不 panic 且 err == nil。nDPI 单包可能无法深度识别，允许返回 "Unknown"，
// 但必须不报错。
//
// 该测试依赖 cgo + libndpi 环境；在 CGO_ENABLED=0 或非 linux 环境，
// ndpi_stub.go 的 ClassifyPacket 返回 ErrUnsupported，本测试也相应校验。
func TestClassifyPacketTCP443(t *testing.T) {
	pkt := buildIPv4TCP(443)

	proto, cat, err := ClassifyPacket(pkt)
	if err != nil {
		// 桩环境（CGO_ENABLED=0）返回 ErrUnsupported，跳过（由 TestClassifyPacketStubBehavior 覆盖）。
		if err == ErrUnsupported {
			t.Skip("stub build detected (CGO_ENABLED=0); real nDPI test skipped")
		}
		t.Fatalf("ClassifyPacket error: %v", err)
	}
	if proto == "" {
		t.Fatal("empty protocol returned")
	}
	// 真实 nDPI 环境：proto/cat 非空（可为 Unknown/Unspecified）
	t.Logf("classified: proto=%q category=%q len=%d", proto, cat, len(pkt))
}

// TestClassifyPacketUnsupportedOrWorks 验证非 cgo 桩: CGO_ENABLED=0 时 err 应为 ErrUnsupported。
// 由于同一测试会在两种环境跑，这里判断: 若构建是 stub（ErrUnsupported），则确认 err 非 nil 即通过；
// 若是真实 cgo，则走上面 TestClassifyPacketTCP443 的语义。此测试兜底覆盖桩路径。
func TestClassifyPacketStubBehavior(t *testing.T) {
	proto, _, err := ClassifyPacket([]byte{0x45})
	if err == nil {
		// 真实 cgo 环境: 1 字节不足，nDPI 返回 Unknown/未识别，但 err==nil
		if proto == "" {
			t.Fatal("empty protocol with nil error")
		}
	} else if err == ErrUnsupported {
		// stub 桩: 符合预期
		t.Log("stub build detected (expected on CGO_ENABLED=0)")
	} else {
		t.Fatalf("unexpected error: %v", err)
	}
}

// buildIPv4TCP 构造一个最小合法 IPv4 + TCP 包（无负载，仅头）。
// 返回的字节数组: 20B IPv4 头 + 20B TCP 头 = 40B。
func buildIPv4TCP(dstPort uint16) []byte {
	pkt := make([]byte, 40)

	// --- IPv4 头 (20B) ---
	pkt[0] = 0x45                                        // version=4, ihl=5 (20B word)
	pkt[1] = 0x00                                        // DSCP/ECN
	binary.BigEndian.PutUint16(pkt[2:4], 40)             // total length = 40
	binary.BigEndian.PutUint16(pkt[4:6], 1)              // identification
	binary.BigEndian.PutUint16(pkt[6:8], 0x4000)         // flags(dont-fragment)+frag offset
	pkt[8] = 64                                          // TTL
	pkt[9] = 6                                           // protocol = TCP
	binary.BigEndian.PutUint16(pkt[10:12], 0)            // checksum (nDPI 不校验)
	pkt[12], pkt[13], pkt[14], pkt[15] = 10, 0, 0, 1     // src 10.0.0.1
	pkt[16], pkt[17], pkt[18], pkt[19] = 198, 51, 100, 1 // dst 198.51.100.1

	// --- TCP 头 (20B) ---
	binary.BigEndian.PutUint16(pkt[20:22], 12345)   // src port
	binary.BigEndian.PutUint16(pkt[22:24], dstPort) // dst port
	binary.BigEndian.PutUint32(pkt[24:28], 1)       // seq
	binary.BigEndian.PutUint32(pkt[28:32], 0)       // ack
	pkt[32] = 0x50                                  // data offset = 5 (20B), reserved
	pkt[33] = 0x02                                  // flags = SYN
	binary.BigEndian.PutUint16(pkt[34:36], 65535)   // window
	binary.BigEndian.PutUint16(pkt[36:38], 0)       // checksum
	binary.BigEndian.PutUint16(pkt[38:40], 0)       // urgent ptr

	return pkt
}
