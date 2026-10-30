package dpi

import (
	"os"
	"testing"
)

// udpPayloadOf 取出 IP 包里的 UDP 载荷（测试用）。
func udpPayloadOf(p []byte) []byte {
	if len(p) < 20 {
		return nil
	}
	switch p[0] >> 4 {
	case 4:
		ihl := int(p[0]&0x0f) * 4
		if p[9] != 17 || len(p) < ihl+8 {
			return nil
		}
		return p[ihl+8:]
	case 6:
		if len(p) < 48 || p[6] != 17 {
			return nil
		}
		return p[48:]
	}
	return nil
}

// TestQUICSNIRealCapture 用真实抓包验证 QUIC Initial 解密。
//
// 样本来源：`curl --http3-only https://cloudflare-quic.com` 期间抓的
// tcpdump（IPv6 + UDP 443）。这是真流量，不是构造包——自己加密自己解
// 只能证明左右手对称，证明不了能解开别人发的包。
func TestQUICSNIRealCapture(t *testing.T) {
	f, err := os.Open("testdata/quic_http3.pcap")
	if err != nil {
		t.Skipf("没有 QUIC 抓包样本: %v", err)
	}
	defer f.Close()

	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatal(err)
	}
	ResetQUICStreams()
	defer ResetQUICStreams()
	if len(pkts) == 0 {
		t.Fatal("抓包样本是空的")
	}

	tried, found := 0, 0
	hosts := map[string]int{}
	for _, p := range pkts {
		payload := udpPayloadOf(p.IP)
		if payload == nil {
			continue
		}
		tried++
		if sni := QUICSNI(payload); sni != "" {
			found++
			hosts[sni]++
		}
	}

	t.Logf("回放 %d 个包，UDP 载荷 %d 个，解出 SNI %d 次", len(pkts), tried, found)
	for h, n := range hosts {
		t.Logf("  SNI=%s x%d", h, n)
	}

	if found == 0 {
		t.Fatalf("真实 QUIC 流量里一个 SNI 都没解出来（UDP 载荷 %d 个）", tried)
	}
	if _, ok := hosts["cloudflare-quic.com"]; !ok {
		t.Fatalf("应解出 cloudflare-quic.com，实际得到 %v", hosts)
	}
}

// TestQUICSNIRejectsGarbage 非 QUIC 数据必须安静地返回空串，不能 panic。
func TestQUICSNIRejectsGarbage(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		{0x00},
		make([]byte, 10),
		make([]byte, 200), // 全 0：短包头
		append([]byte{0xc0, 0, 0, 0, 1}, make([]byte, 100)...), // 长包头但长度字段为 0
	}
	for i, c := range cases {
		if got := QUICSNI(c); got != "" {
			t.Fatalf("第 %d 组垃圾数据不应解出 SNI，实际 %q", i, got)
		}
	}
}

// TestQUICVarint 覆盖四种长度的可变长整数。
func TestQUICVarint(t *testing.T) {
	cases := []struct {
		in   []byte
		want uint64
		n    int
	}{
		{[]byte{0x25}, 37, 1},
		{[]byte{0x40, 0x25}, 37, 2},
		{[]byte{0x80, 0x00, 0x00, 0x25}, 37, 4},
		{[]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x25}, 37, 8},
	}
	for _, c := range cases {
		got, n, ok := quicVarint(c.in)
		if !ok || got != c.want || n != c.n {
			t.Fatalf("varint %x: got (%d,%d,%v) want (%d,%d,true)", c.in, got, n, ok, c.want, c.n)
		}
	}
	// 截断输入必须失败而不是越界
	if _, _, ok := quicVarint([]byte{0xc0, 0x00}); ok {
		t.Fatal("8 字节 varint 只有 2 字节时应失败")
	}
}

// TestQUICHandshakeSNI 直接喂一段构造的 ClientHello，验证握手解析本身。
func TestQUICHandshakeSNI(t *testing.T) {
	ch := buildClientHello("example.org")
	if got := sniFromHandshake(ch); got != "example.org" {
		t.Fatalf("应解出 example.org，实际 %q", got)
	}
	// 非 ClientHello 类型
	bad := append([]byte{0x02}, ch[1:]...)
	if got := sniFromHandshake(bad); got != "" {
		t.Fatalf("ServerHello 不应解出 SNI，实际 %q", got)
	}
	// 长度声明超出实际数据
	short := append([]byte{}, ch...)
	short[3] = 0xff
	if got := sniFromHandshake(short); got != "" {
		t.Fatalf("长度越界时应返回空，实际 %q", got)
	}
}

// buildClientHello 构造一个最小的 ClientHello（只带 server_name 扩展）。
func buildClientHello(host string) []byte {
	name := []byte(host)
	sn := make([]byte, 0, 5+len(name))
	sn = append(sn, byte((len(name)+3)>>8), byte(len(name)+3)) // list len（2 字节）
	sn = append(sn, 0)                                         // host_name
	sn = append(sn, byte(len(name)>>8), byte(len(name)))
	sn = append(sn, name...)

	ext := make([]byte, 0, 4+len(sn))
	ext = append(ext, 0x00, 0x00, byte(len(sn)>>8), byte(len(sn)))
	ext = append(ext, sn...)

	body := make([]byte, 0, 64)
	body = append(body, 0x03, 0x03)             // legacy_version
	body = append(body, make([]byte, 32)...)    // random
	body = append(body, 0)                      // session id len
	body = append(body, 0x00, 0x02, 0x13, 0x01) // cipher suites
	body = append(body, 0x01, 0x00)             // compression
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	out := []byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(out, body...)
}
