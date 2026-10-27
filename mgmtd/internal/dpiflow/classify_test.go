package dpiflow

import (
	"encoding/binary"
	"strings"
	"testing"
)

// 构造最小合法 TLS ClientHello，含一个 server_name 扩展。
// 返回 (记录头 + handshake + ClientHelloBody)，SNI 为 want。
func buildClientHelloSNI(sni string) []byte {
	sniBytes := []byte(sni)

	// ---- ClientHello body ----
	// version(2) + random(32) + sessionID_len(1)+0 + cipher_len(2)+2 + cipher(0x1301)
	// + comp_len(1)+1 + 0x00 + ext_len(2) + [ext server_name]
	var body []byte
	body = append(body, 0x03, 0x03)          // TLS 1.2
	body = append(body, make([]byte, 32)...) // random
	body = append(body, 0)                   // session_id_len = 0
	body = append(body, 0x00, 0x02)          // cipher_suites len = 2
	body = append(body, 0x13, 0x01)          // TLS_AES_128_GCM_SHA256
	body = append(body, 0x01, 0x00)          // compression_methods len=1, method=null

	// server_name extension
	// ext: type(2)=0x0000, len(2), server_name_list
	// server_name_list: listLen(2)=1+2+len(name), nameType(1)=0x00, nameLen(2)=len, name
	nameBlock := make([]byte, 0)
	nameBlock = append(nameBlock, 0x00)                              // nameType host_name
	nameBlock = append(nameBlock, byte(len(sni)>>8), byte(len(sni))) // nameLen
	nameBlock = append(nameBlock, sniBytes...)
	listLen := 1 + 2 + len(sni)
	nameList := make([]byte, 2, 2+len(nameBlock))
	binary.BigEndian.PutUint16(nameList, uint16(listLen))
	nameList = append(nameList, nameBlock...)
	extData := nameList
	ext := make([]byte, 0, 4+len(extData))
	ext = append(ext, 0x00, 0x00) // type server_name
	ext = append(ext, byte(len(extData)>>8), byte(len(extData)))
	ext = append(ext, extData...)

	extBlock := make([]byte, 0, 2+len(ext))
	extBlock = append(extBlock, byte(len(ext)>>8), byte(len(ext)))
	extBlock = append(extBlock, ext...)
	body = append(body, extBlock...)

	// ---- handshake ----
	hs := make([]byte, 0, 4+len(body))
	hs = append(hs, 0x01) // ClientHello
	hs = append(hs, byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	hs = append(hs, body...)

	// ---- record header ----
	rec := make([]byte, 0, 5+len(hs))
	rec = append(rec, 0x16, 0x03, 0x01)
	rec = append(rec, byte(len(hs)>>8), byte(len(hs)))
	rec = append(rec, hs...)
	return rec
}

func TestTLSClientHelloSNI(t *testing.T) {
	hello := buildClientHelloSNI("api.example.com")
	app, cat := DetectApp(hello, 8443) // 非标准端口仍应靠 SNI 识别
	if !strings.HasPrefix(app, "tls-sni:api.example.com") {
		t.Fatalf("expected tls-sni:api.example.com, got %q (cat=%q)", app, cat)
	}
}

func TestHTTPHost(t *testing.T) {
	req := []byte("GET /index.html HTTP/1.1\r\nHost: www.test.com\r\nUser-Agent: curl\r\n\r\n")
	app, _ := DetectApp(req, 8080) // 非标准端口
	if app != "http:www.test.com" {
		t.Fatalf("expected http:www.test.com, got %q", app)
	}
}

func TestDNSQueryName(t *testing.T) {
	// 构造 DNS 查询：header(12) + QNAME "www.example.com"
	qname := "\x03www\x07example\x03com\x00"
	dns := make([]byte, 12)
	dns[2] = 0x01 // flags: 只有 RD 位，QR=0 查询
	dns = append(dns, []byte(qname)...)
	app, _ := DetectApp(dns, 53)
	if app != "dns:www.example.com" {
		t.Fatalf("expected dns:www.example.com, got %q", app)
	}
}

// 越界防护：各种截断/畸形载荷都不能 panic。
func TestDetectAppBoundsSafety(t *testing.T) {
	cases := []struct {
		name string
		p    []byte
	}{
		{"empty", nil},
		{"empty2", []byte{}},
		{"tls-1byte", []byte{0x16}},
		{"tls-2byte", []byte{0x16, 0x03}},
		{"tls-0x16-nothing", []byte{0x16}},
		{"tls-trunc-record", []byte{0x16, 0x03, 0x01, 0x00, 0x0a, 0x01, 0x00}},
		{"tls-null-after", buildClientHelloSNI("normal.example.com")[:8]},
		{"tls-handshake-trunc", buildClientHelloSNI("normal.example.com")[:14]},
		{"dns-1byte", []byte{0x00}},
		{"dns-short", []byte{0x00, 0x01, 0x02, 0x03}},
		{"dns-maxlabel", []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x41}},
		{"http-hostsig", []byte{0x0d, 0x0a, 0x68, 0x6f, 0x73, 0x74, 0x3a}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on input %q: %v", c.name, r)
				}
			}()
			_, _ = DetectApp(c.p, 80)
		})
	}
}

// 超长载荷不能 panic。
func TestDetectAppHugePayload(t *testing.T) {
	huge := make([]byte, 1<<20)
	for i := range huge {
		huge[i] = byte(i % 251)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on huge payload: %v", r)
		}
	}()
	_, _ = DetectApp(huge, 443)
}

// 端口快速判断。
func TestPortFastPath(t *testing.T) {
	app, cat := DetectApp(nil, 3306)
	if app != "mysql" || cat != "Database" {
		t.Fatalf("port 3306 -> mysql, got %q/%q", app, cat)
	}
	app, cat = DetectApp(nil, 6379)
	if app != "redis" || cat != "Database" {
		t.Fatalf("port 6379 -> mysql, got %q/%q", app, cat)
	}
	app, _ = DetectApp(nil, 9999)
	if app != "Unknown" {
		t.Fatalf("unknown port with empty payload -> Unknown, got %q", app)
	}
}
