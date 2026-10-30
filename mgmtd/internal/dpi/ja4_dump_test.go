package dpi

import (
	"os"
	"testing"
)

// tcpPayloadOf 取出 IP 包里的 TCP 载荷（测试用）。
func tcpPayloadOf(p []byte) []byte {
	if len(p) < 20 {
		return nil
	}
	switch p[0] >> 4 {
	case 4:
		ihl := int(p[0]&0x0f) * 4
		if p[9] != 6 || len(p) < ihl+20 {
			return nil
		}
		doff := int(p[ihl+12]>>4) * 4
		if doff < 20 || len(p) <= ihl+doff {
			return nil
		}
		return p[ihl+doff:]
	case 6:
		if len(p) < 60 || p[6] != 6 {
			return nil
		}
		doff := int(p[52]>>4) * 4
		if doff < 20 || len(p) <= 40+doff {
			return nil
		}
		return p[40+doff:]
	}
	return nil
}

// TestJA4DumpClients 用于实测：把一段抓包里每个 TLS 连接的 JA4 打出来。
// 只在设置 JA4_PCAP 时运行，避免日常测试依赖外部文件。
func TestJA4DumpClients(t *testing.T) {
	path := os.Getenv("JA4_PCAP")
	if path == "" {
		t.Skip("设置 JA4_PCAP 指向抓包文件")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	for _, p := range pkts {
		payload := tcpPayloadOf(p.IP)
		if payload == nil || len(payload) < 6 || payload[0] != 0x16 {
			continue
		}
		ch := ParseClientHello(payload)
		if ch == nil {
			continue
		}
		if os.Getenv("JA4_VERBOSE") != "" && len(seen) < 6 {
			t.Logf("legacy=%04x sni=%q alpn=%v ciphers=%d exts=%d groups=%d sigalgs=%d supver=%04x",
				ch.LegacyVersion, ch.SNI, ch.ALPN, len(ch.CipherSuites), len(ch.Extensions),
				len(ch.Groups), len(ch.SigAlgs), ch.SupportedVers)
		}
		seen[ch.JA4()]++
	}
	for ja4, n := range seen {
		t.Logf("JA4=%s  次数=%d  JA3=%s", ja4, n, ja3For(pkts, ja4))
	}
	if len(seen) == 0 {
		t.Fatal("抓包里没有 ClientHello")
	}
}

// ja3For 反查某个 JA4 对应的 JA3（仅用于实测打印）。
func ja3For(pkts []PCAPPacket, ja4 string) string {
	for _, p := range pkts {
		payload := tcpPayloadOf(p.IP)
		if payload == nil || len(payload) < 6 || payload[0] != 0x16 {
			continue
		}
		if ch := ParseClientHello(payload); ch != nil && ch.JA4() == ja4 {
			return ch.JA3()
		}
	}
	return ""
}
