package dpi

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// 构造一个 IPv4 + TCP 包（校验和填 0，nDPI 不校验）。
func buildTCPPacket(srcIP, dstIP string, srcPort, dstPort uint16, payload []byte, flags byte) []byte {
	return buildTCPPacketSeq(srcIP, dstIP, srcPort, dstPort, payload, flags, 1000, 0)
}

// buildTCPPacketSeq 可指定序号，用于构造完整的 TCP 握手——
// 对"中途插入"的流 nDPI 识别能力有限，先建连接再送数据才走得通。
func buildTCPPacketSeq(srcIP, dstIP string, srcPort, dstPort uint16, payload []byte, flags byte, seq, ack uint32) []byte {
	src := net.ParseIP(srcIP).To4()
	dst := net.ParseIP(dstIP).To4()
	tcpLen := 20 + len(payload)
	pkt := make([]byte, 20+tcpLen)

	pkt[0] = 0x45 // IPv4, IHL=5
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	pkt[8] = 64 // TTL
	pkt[9] = 6  // TCP
	copy(pkt[12:16], src)
	copy(pkt[16:20], dst)

	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	binary.BigEndian.PutUint32(pkt[24:28], seq)
	binary.BigEndian.PutUint32(pkt[28:32], ack)
	pkt[32] = 0x50 // data offset
	pkt[33] = flags
	copy(pkt[40:], payload)
	// 校验和必须正确：nDPI 会据此判断包是否完整可信
	binary.BigEndian.PutUint16(pkt[36:38], tcpChecksum(pkt))
	return pkt
}

// tcpChecksum 计算 IPv4/TCP 校验和（含伪首部）。
func tcpChecksum(pkt []byte) uint16 {
	ihl := int(pkt[0]&0x0f) * 4
	seg := pkt[ihl:]
	var sum uint32
	sum += uint32(binary.BigEndian.Uint16(pkt[12:14]))
	sum += uint32(binary.BigEndian.Uint16(pkt[14:16]))
	sum += uint32(binary.BigEndian.Uint16(pkt[16:18]))
	sum += uint32(binary.BigEndian.Uint16(pkt[18:20]))
	sum += uint32(pkt[9])
	sum += uint32(len(seg))
	for i := 0; i+1 < len(seg); i += 2 {
		if i == 16 { // 校验和字段按 0 参与计算
			continue
		}
		sum += uint32(binary.BigEndian.Uint16(seg[i : i+2]))
	}
	if len(seg)%2 == 1 {
		sum += uint32(seg[len(seg)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

// handshake 生成三次握手：SYN / SYN-ACK / ACK。
func handshake(srcIP, dstIP string, sport, dport uint16) [][]byte {
	cseq, sseq := uint32(1000), uint32(5000)
	return [][]byte{
		buildTCPPacketSeq(srcIP, dstIP, sport, dport, nil, 0x02, cseq, 0),
		buildTCPPacketSeq(dstIP, srcIP, dport, sport, nil, 0x12, sseq, cseq+1),
		buildTCPPacketSeq(srcIP, dstIP, sport, dport, nil, 0x10, cseq+1, sseq+1),
	}
}

func buildIPv6TCPPacket(t *testing.T, srcIP, dstIP string, srcPort, dstPort uint16, payload []byte) []byte {
	t.Helper()
	src := net.ParseIP(srcIP).To16()
	dst := net.ParseIP(dstIP).To16()
	if src == nil || dst == nil {
		t.Fatal("bad ipv6")
	}
	pkt := make([]byte, 40+20+len(payload))
	pkt[0] = 0x60
	binary.BigEndian.PutUint16(pkt[4:6], uint16(20+len(payload)))
	pkt[6] = 6
	pkt[7] = 64
	copy(pkt[8:24], src)
	copy(pkt[24:40], dst)
	binary.BigEndian.PutUint16(pkt[40:42], srcPort)
	binary.BigEndian.PutUint16(pkt[42:44], dstPort)
	pkt[52] = 0x50
	copy(pkt[60:], payload)
	return pkt
}

func TestParseFlowKeyIPv4(t *testing.T) {
	pkt := buildTCPPacket("10.0.0.1", "93.184.216.34", 51234, 80, []byte("GET /"), 0x18)
	k, ok := ParseFlowKey(pkt)
	if !ok {
		t.Fatal("应能解析")
	}
	if k.Proto != 6 || k.IsIPv6 {
		t.Fatalf("协议/地址族错误: %+v", k)
	}
	// 归一化是按"地址较小的一方"排在前面，端口跟着它走：
	// 10.0.0.1 < 93.184.216.34，所以 IPLo=10.0.0.1、PortLo=它的端口 51234。
	// 关键是正反向一致，而不是"小端口在前"。
	if k.PortLo != 51234 || k.PortHi != 80 {
		t.Fatalf("归一化错误: %+v", k)
	}
}

func TestFlowKeyNormalizesDirection(t *testing.T) {
	// 关键：客户端→服务器 与 服务器→客户端 必须落在同一个 key 上，
	// 否则 nDPI 看不到应答，双向各算一条流。
	fwd := buildTCPPacket("10.0.0.1", "93.184.216.34", 51234, 80, []byte("hi"), 0x18)
	rev := buildTCPPacket("93.184.216.34", "10.0.0.1", 80, 51234, []byte("yo"), 0x18)
	k1, ok1 := ParseFlowKey(fwd)
	k2, ok2 := ParseFlowKey(rev)
	if !ok1 || !ok2 {
		t.Fatal("都应解析成功")
	}
	if k1 != k2 {
		t.Fatalf("正反向应归一化为同一 key:\n%+v\n%+v", k1, k2)
	}
}

func TestFlowKeyDistinguishesFlows(t *testing.T) {
	a := buildTCPPacket("10.0.0.1", "1.1.1.1", 1000, 443, nil, 0x18)
	b := buildTCPPacket("10.0.0.2", "1.1.1.1", 1000, 443, nil, 0x18)
	c := buildTCPPacket("10.0.0.1", "1.1.1.1", 1001, 443, nil, 0x18)
	ka, _ := ParseFlowKey(a)
	kb, _ := ParseFlowKey(b)
	kc, _ := ParseFlowKey(c)
	if ka == kb || ka == kc || kb == kc {
		t.Fatal("不同源地址/端口必须是不同流")
	}
}

func TestFlowKeyIPv6(t *testing.T) {
	pkt := buildIPv6TCPPacket(t, "2001:db8::1", "2001:db8::2", 40000, 443, []byte("x"))
	k, ok := ParseFlowKey(pkt)
	if !ok || !k.IsIPv6 || k.Proto != 6 {
		t.Fatalf("IPv6 解析失败: %+v ok=%v", k, ok)
	}
}

func TestFlowKeyRejectsNonTCPUDPSegments(t *testing.T) {
	// ICMP（proto=1）
	icmp := make([]byte, 28)
	icmp[0] = 0x45
	binary.BigEndian.PutUint16(icmp[2:4], 28)
	icmp[9] = 1
	if _, ok := ParseFlowKey(icmp); ok {
		t.Fatal("ICMP 不该参与应用识别")
	}

	// 分片包（MF 置位）
	frag := buildTCPPacket("10.0.0.1", "10.0.0.2", 1, 2, nil, 0x18)
	frag[6] = 0x20 // MF
	if _, ok := ParseFlowKey(frag); ok {
		t.Fatal("分片包不完整，不该参与识别")
	}

	// 太短
	if _, ok := ParseFlowKey([]byte{0x45, 0}); ok {
		t.Fatal("截断包应被拒绝")
	}
	// 空
	if _, ok := ParseFlowKey(nil); ok {
		t.Fatal("空包应被拒绝")
	}
}

func TestFlowPolicyEviction(t *testing.T) {
	p := FlowPolicy{MaxFlows: 100, IdleTimeout: 5 * time.Minute, ScanEvery: 30 * time.Second}
	now := time.Now()
	if p.shouldEvict(now.Add(-6*time.Minute), now) != true {
		t.Fatal("超过空闲时间应回收")
	}
	if p.shouldEvict(now.Add(-time.Minute), now) != false {
		t.Fatal("活跃流不该回收")
	}
	if !p.isFull(90) {
		t.Fatal("到达 90% 应视为需要提前清理")
	}
	if p.isFull(50) {
		t.Fatal("50% 不该触发")
	}
}
