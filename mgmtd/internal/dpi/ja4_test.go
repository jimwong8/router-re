package dpi

import (
	"os"
	"strings"
	"testing"
)

// 这两条期望值来自 FoxIO 官方仓库的测试语料（python/test/testdata/*.pcap.json），
// 是独立实现算出来的结果——拿它当基准，比自己算自己比有意义得多。
const (
	foxioFirefoxJA4 = "t13d1715h2_5b57614c22b0_3d5424432f57"
	foxioTLS12JA4   = "t12d4605h2_85626a9a5f7f_aaf95bb78ec9"
)

func ja4SetFromPcap(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("缺样本 %s: %v", path, err)
	}
	defer f.Close()
	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	var out []string
	for _, p := range pkts {
		if ch := clientHelloOf(p.IP); ch != nil {
			ja4 := ch.JA4()
			if len(out) == 0 || out[len(out)-1] != ja4 {
				out = append(out, ja4)
			}
		}
	}
	return out
}

// TestJA4OfficialVectors 用 FoxIO 官方样本校验实现是否与参考实现一致。
func TestJA4OfficialVectors(t *testing.T) {
	cases := []struct{ file, want string }{
		{"testdata/ja4_firefox_tls12.pcap", foxioFirefoxJA4},
		{"testdata/ja4_tls12_ipv6.pcap", foxioTLS12JA4},
	}
	for _, c := range cases {
		got := ja4SetFromPcap(t, c.file)
		if len(got) == 0 {
			t.Fatalf("%s 里没解析出 ClientHello", c.file)
		}
		found := false
		for _, g := range got {
			if g == c.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: 期望 %s，实际 %v", c.file, c.want, got)
		}
		t.Logf("%s -> %s ✓", c.file, c.want)
	}
}

// TestJA4Format 校验格式各段（协议、版本、SNI 标记、计数、ALPN）。
func TestJA4Format(t *testing.T) {
	got := ja4SetFromPcap(t, "testdata/ja4_firefox_tls12.pcap")
	if len(got) == 0 {
		t.Skip("缺样本")
	}
	j := got[0]
	parts := strings.Split(j, "_")
	if len(parts) != 3 {
		t.Fatalf("JA4 应为三段，实际 %q", j)
	}
	a := parts[0]
	if len(a) < 8 {
		t.Fatalf("首段长度异常: %q", a)
	}
	if a[0] != 't' {
		t.Fatalf("TCP 的协议标记应为 t，实际 %q", a)
	}
	if a[1:3] != "13" {
		t.Fatalf("版本段应为 13，实际 %q", a[1:3])
	}
	if a[3] != 'd' {
		t.Fatalf("有 SNI 时标记应为 d，实际 %q", a[3])
	}
	// 首段布局：协议(1) 版本(2) SNI 标记(1) 密码套件数(2) 扩展数(2) ALPN(2)
	if a[8:] != "h2" && a[8:] != "00" {
		t.Fatalf("ALPN 段异常: %q", a[8:])
	}
	if len(parts[1]) != 12 || len(parts[2]) != 12 {
		t.Fatalf("哈希段应为 12 位，实际 %q / %q", parts[1], parts[2])
	}
}

// buildHello 构造最小 ClientHello；host 为空则不发送 server_name 扩展。
func buildHello(host string, alpn []string) []byte {
	ext := []byte{}
	if host != "" {
		name := []byte(host)
		sn := []byte{byte((len(name) + 3) >> 8), byte(len(name) + 3), 0,
			byte(len(name) >> 8), byte(len(name))}
		sn = append(sn, name...)
		ext = append(ext, 0x00, 0x00, byte(len(sn)>>8), byte(len(sn)))
		ext = append(ext, sn...)
	}
	if len(alpn) > 0 {
		list := []byte{}
		for _, a := range alpn {
			list = append(list, byte(len(a)))
			list = append(list, a...)
		}
		body := append([]byte{byte(len(list) >> 8), byte(len(list))}, list...)
		ext = append(ext, 0x00, 0x10, byte(len(body)>>8), byte(len(body)))
		ext = append(ext, body...)
	}
	// supported_versions: TLS1.3 + 1.2（1 字节长度前缀）
	sv := []byte{0x04, 0x03, 0x04, 0x03, 0x03}
	ext = append(ext, 0x00, 0x2b, byte(len(sv)>>8), byte(len(sv)))
	ext = append(ext, sv...)

	body := []byte{0x03, 0x03}
	body = append(body, make([]byte, 32)...)
	body = append(body, 0x00)                   // session id
	body = append(body, 0x00, 0x02, 0x13, 0x01) // cipher suites
	body = append(body, 0x01, 0x00)             // compression
	body = append(body, byte(len(ext)>>8), byte(len(ext)))
	body = append(body, ext...)

	out := []byte{0x01, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}
	return append(out, body...)
}

// TestJA4NoSNI 无 SNI 的 ClientHello 应标记为 i，且仍能算出指纹。
func TestJA4NoSNI(t *testing.T) {
	ch := ParseClientHello(buildHello("", nil))
	if ch == nil {
		t.Fatal("构造的 ClientHello 应能解析")
	}
	if !strings.HasPrefix(ch.JA4(), "t13i") {
		t.Fatalf("无 SNI 时应为 t13i…，实际 %q", ch.JA4())
	}
}

// TestJA4QUICMarker QUIC 的协议标记是 q，与 TCP 区分开。
func TestJA4QUICMarker(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	f, err := os.Open("testdata/quic_http3.pcap")
	if err != nil {
		t.Skip("缺 QUIC 样本")
	}
	defer f.Close()
	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, p := range pkts {
		if ch := clientHelloOf(p.IP); ch != nil && ch.IsQUIC {
			found = append(found, ch.JA4())
			if ch.SNI == "" {
				t.Fatal("QUIC ClientHello 应带 SNI")
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("QUIC 抓包里没算出一个 JA4")
	}
	for _, j := range found {
		if j[0] != 'q' {
			t.Fatalf("QUIC 的协议标记应为 q，实际 %q", j)
		}
	}
	t.Logf("QUIC 指纹: %v", found)
}

// TestJA4EmptyHash 空字段的哈希段应为 12 个 0（与参考实现一致）。
func TestJA4EmptyHash(t *testing.T) {
	if got := hash12(""); got != "000000000000" {
		t.Fatalf("空串哈希应为 12 个 0，实际 %q", got)
	}
}

// TestJA4GreaseIgnored GREASE 值不参与指纹，否则同一客户端每次结果都变。
func TestJA4GreaseIgnored(t *testing.T) {
	if !isGREASE(0x0a0a) || !isGREASE(0xfafa) || !isGREASE(0x1a1a) {
		t.Fatal("GREASE 判定错误")
	}
	if isGREASE(0x1301) || isGREASE(0x0304) {
		t.Fatal("正常值被误判为 GREASE")
	}
}

// TestLoadKnownClients 覆盖文件缺失、合并与非法 JSON 三种情况。
func TestLoadKnownClients(t *testing.T) {
	dir := t.TempDir()

	// 文件不存在不是错误
	if n, err := LoadKnownClients(dir + "/nope.json"); err != nil || n != 0 {
		t.Fatalf("缺文件应静默跳过，得到 n=%d err=%v", n, err)
	}

	p := dir + "/ja4_clients.json"
	if err := os.WriteFile(p, []byte(`{"q13d0000h3_aaaabbbbcccc_ddddeeeeffff":"测试客户端"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := LoadKnownClients(p)
	if err != nil || n != 1 {
		t.Fatalf("期望载入 1 条，得到 n=%d err=%v", n, err)
	}
	if got := DescribeClient("q13d0000h3_aaaabbbbcccc_ddddeeeeffff"); got != "测试客户端" {
		t.Fatalf("覆盖未生效，得到 %q", got)
	}
	// 内置条目不能被覆盖文件清掉
	if got := DescribeClient("t13d131000_f57a46bbacb6_e7c285222651"); got == "" {
		t.Fatal("内置条目应保留")
	}
	// 未知指纹返回空，不能瞎猜
	if got := DescribeClient("t13d9999h9_000000000000_000000000000"); got != "" {
		t.Fatalf("未知指纹应返回空，得到 %q", got)
	}

	// 坏 JSON 要报错，而不是把名单清空
	if err := os.WriteFile(p, []byte(`{oops`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKnownClients(p); err == nil {
		t.Fatal("非法 JSON 应返回错误")
	}
	if got := DescribeClient("t13d131000_f57a46bbacb6_e7c285222651"); got == "" {
		t.Fatal("解析失败后不应清空已有名单")
	}
}

// TestPcapngMatchesChrome 用 FoxIO 的 Chrome 样本校验 pcapng 解析。
// 期望值与 python/test/testdata/browsers-x509.pcapng.json 一致。
func TestPcapngMatchesChrome(t *testing.T) {
	const want = "t13d1516h2_8daaf6152771_e5627efa2ab1"
	f, err := os.Open("testdata/ja4_chrome.pcapng")
	if err != nil {
		t.Skip("缺 pcapng 样本")
	}
	defer f.Close()
	pkts, err := ReadPcap(f)
	if err != nil {
		t.Fatalf("pcapng 解析失败: %v", err)
	}
	if len(pkts) == 0 {
		t.Fatal("pcapng 里没解析出包")
	}
	got := map[string]int{}
	for _, p := range pkts {
		if ch := clientHelloOf(p.IP); ch != nil {
			got[ch.JA4()]++
		}
	}
	if got[want] != 3 {
		t.Fatalf("期望 3 条 %s，实际 %v", want, got)
	}
	t.Logf("pcapng 包数=%d 指纹=%v", len(pkts), got)
}

// TestPcapngTruncated 截断的 pcapng 不能 panic，解析出来的部分要保留。
func TestPcapngTruncated(t *testing.T) {
	all, err := os.ReadFile("testdata/ja4_chrome.pcapng")
	if err != nil {
		t.Skip("缺 pcapng 样本")
	}
	// 截到一个块中间
	cut := all[:len(all)/2]
	pkts, err := ReadPcapng(cut)
	if err != nil {
		t.Fatalf("截断文件应尽量解析而不是报错: %v", err)
	}
	if len(pkts) == 0 {
		t.Fatal("截断后的前半段应该有包")
	}
}

// TestPcapngBadMagic 不是 pcapng 的数据要给出明确错误，不能静默返回空。
func TestPcapngBadMagic(t *testing.T) {
	if _, err := ReadPcapng([]byte("this is not a pcapng file at all")); err == nil {
		t.Fatal("非法输入应报错")
	}
}
