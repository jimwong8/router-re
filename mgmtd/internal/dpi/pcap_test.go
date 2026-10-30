package dpi

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// 构造一个最小 pcap（按指定链路类型）。
func buildPcap(linkType uint32, pkts [][]byte) []byte {
	var b bytes.Buffer
	hdr := make([]byte, 24)
	binary.LittleEndian.PutUint32(hdr[0:4], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(hdr[4:6], 2)
	binary.LittleEndian.PutUint16(hdr[6:8], 4)
	binary.LittleEndian.PutUint32(hdr[20:24], linkType)
	b.Write(hdr)
	for i, p := range pkts {
		rec := make([]byte, 16)
		binary.LittleEndian.PutUint32(rec[0:4], uint32(1700000000+i))
		binary.LittleEndian.PutUint32(rec[8:12], uint32(len(p)))
		binary.LittleEndian.PutUint32(rec[12:16], uint32(len(p)))
		b.Write(rec)
		b.Write(p)
	}
	return b.Bytes()
}

func TestReadPcapRawLinkType(t *testing.T) {
	// tailscale / wireguard / tun 抓出来的是裸 IP 包，链路类型 101。
	// 早期只处理了以太网，导致这类抓包被当成"没有 IP 包"直接拒绝。
	pkt := buildTCPPacket("100.64.0.1", "93.184.216.34", 40000, 443, []byte("x"), 0x18)
	data := buildPcap(101, [][]byte{pkt, pkt})

	pkts, err := ReadPcap(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 2 {
		t.Fatalf("应解析出 2 个包，实际 %d", len(pkts))
	}
	if pkts[0].IP[0]>>4 != 4 {
		t.Fatalf("应直接从 IP 头开始，实际首字节 %02x", pkts[0].IP[0])
	}
}

func TestReadPcapEthernetLinkType(t *testing.T) {
	ip := buildTCPPacket("10.0.0.1", "10.0.0.2", 1234, 80, []byte("hi"), 0x18)
	eth := make([]byte, 14)
	binary.BigEndian.PutUint16(eth[12:14], 0x0800)
	frame := append(eth, ip...)

	pkts, err := ReadPcap(bytes.NewReader(buildPcap(1, [][]byte{frame})))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 1 || pkts[0].IP[0]>>4 != 4 {
		t.Fatalf("以太网帧应被剥到 IP 层: %+v", pkts)
	}
}

func TestReadPcapRejectsUnknownLinkType(t *testing.T) {
	// 不支持的链路类型必须明确报错，不能静默返回 0 个包
	data := buildPcap(147, [][]byte{{0x00}})
	if _, err := ReadPcap(bytes.NewReader(data)); err == nil {
		t.Fatal("未知链路类型应报错")
	}
}

func TestReadPcapRejectsGarbage(t *testing.T) {
	if _, err := ReadPcap(bytes.NewReader([]byte("not a pcap at all, just text"))); err == nil {
		t.Fatal("非 pcap 数据应报错")
	}
	if _, err := ReadPcap(bytes.NewReader(nil)); err == nil {
		t.Fatal("空数据应报错")
	}
}

func TestReadPcapTruncatedRecord(t *testing.T) {
	// 记录长度超出文件末尾时应停止而不是越界 panic
	pkt := buildTCPPacket("1.1.1.1", "2.2.2.2", 1, 2, nil, 0x18)
	data := buildPcap(101, [][]byte{pkt})
	truncated := data[:len(data)-5]
	pkts, err := ReadPcap(bytes.NewReader(truncated))
	if err != nil {
		t.Fatalf("截断文件不该报错: %v", err)
	}
	if len(pkts) != 0 {
		t.Fatalf("不完整记录应被跳过，实际解析出 %d 个", len(pkts))
	}
}

func TestAnalyzePcapAggregatesFlows(t *testing.T) {
	ip1 := buildTCPPacket("10.0.0.1", "10.0.0.2", 1000, 80, []byte("GET / HTTP/1.1\r\nHost: a.com\r\n\r\n"), 0x18)
	ip2 := buildTCPPacket("10.0.0.3", "10.0.0.2", 2000, 80, []byte("GET / HTTP/1.1\r\nHost: b.com\r\n\r\n"), 0x18)
	pkts := []PCAPPacket{{IP: ip1}, {IP: ip2}, {IP: ip1}}

	// 用假分类器验证聚合逻辑（不依赖 nDPI）
	fake := func(p []byte) (string, string, int, error) { return "HTTP", "Web", 1, nil }
	flows := AnalyzePcap(pkts, fake)
	if len(flows) != 2 {
		t.Fatalf("应聚合为 2 条流，实际 %d", len(flows))
	}
	// 第一条流有 2 个包
	if flows[0].Packets != 2 {
		t.Fatalf("第一条流包数应为 2，实际 %d", flows[0].Packets)
	}
	if flows[0].Protocol != "HTTP" || flows[0].Category != "Web" {
		t.Fatalf("协议聚合错误: %+v", flows[0])
	}
}
