package dpiflow

import (
	"encoding/binary"
	"strings"
)

// 快速端口判断：已知服务端口直接命中，避免逐字节特征匹配。
// 返回 (app, category)；命中返回 ok=true。
func portDetect(dstPort int) (app, category string, ok bool) {
	switch dstPort {
	case 80:
		return "http", "Web", true
	case 443:
		return "https", "Web", true
	case 22:
		return "ssh", "Remote", true
	case 53:
		return "dns", "DNS", true
	case 25, 587:
		return "smtp", "Mail", true
	case 3306:
		return "mysql", "Database", true
	case 6379:
		return "redis", "Database", true
	}
	return "", "", false
}

// DetectApp 对一条流的首个载荷做应用识别。
//
// 两层策略：
//  1. 端口快速判断（80/443→http/https、22→ssh、53→dns、25/587→smtp、
//     3306→mysql、6379→redis）。
//  2. 载荷特征匹配：TLS ClientHello 的 SNI、HTTP Host 头、DNS 查询名。
//
// 识别（SNI/Host/DNS name）受端口与载荷双重约束，因此即使端口为常见端口
// 也仍会返回"可能"的应用名（调用方可决定置信度）。
//
// 越界防护：解析二进制每一层都要先判长度，任何截断都不能 panic。
func DetectApp(payload []byte, dstPort int) (app, category string) {
	// 特征优先于端口：先试深度特征（SNI/Host/DNS/HTTP），能识别出就返回。
	if a, c := detectByPayload(payload); a != "" {
		return a, c
	}
	// 无载荷特征时退回端口快速判断。
	if app, cat, ok := portDetect(dstPort); ok {
		return app, cat
	}
	return "Unknown", "Unspecified"
}

// detectByPayload 仅在载荷有可辨识特征时返回精确应用。
func detectByPayload(payload []byte) (app, category string) {
	if len(payload) == 0 {
		return "", ""
	}
	// 1. TLS ClientHello（记录头 0x16 0x03 0x01/0x03 0x03/0x03 0x04…）
	if len(payload) >= 3 && payload[0] == 0x16 && payload[1] == 0x03 {
		if sni := parseTLSClientHello(payload); sni != "" {
			return "tls-sni:" + sni, "Encrypted"
		}
		return "tls", "Encrypted"
	}
	// 2. DNS 查询（QR=0 的查询、QNAME 以长度前缀开头）。TCP DNS 也有 2 字节长度头。
	//    判断依据：opcode 域多为 0，且紧接着的 QNAME 由合法长度字节组成。
	if len(payload) >= 12 && payload[2]&0x80 == 0 { // QR bit 0 => query
		if name := parseDNSQueryName(payload); name != "" {
			return "dns:" + name, "DNS"
		}
	}
	// 3. HTTP 请求行/Host 头。宽松判断：以方法开头或包含 \r\nHost:
	if isHTTP(payload) {
		if host := httpHost(payload); host != "" {
			return "http:" + host, "Web"
		}
		return "http", "Web"
	}
	return "", ""
}

// parseTLSClientHello 解析 TLS ClientHello 里的 SNI（server_name 扩展 0x0000）。
//
// 布局（每个下标读前先判 len 足够）：
//
//	0   1   2       3       4..5  6..7
//	16  03  major  minor  len(2B)
//	[handshake]
//	0x01(ClientHello)  len(3B)
//	version(2) random(32) sessionIDlen(1)+id ...
//	[ cipher_suites(2B len) ... | compression(1B len) ... ]
//	[ extensions(2B len) | 依次 extension: type(2) len(2) data ]
//	server_name ext type=0x0000, data: listLen(2B) 0x00 nameType(1B) nameLen(2B) name
func parseTLSClientHello(p []byte) string {
	// p[0..4] 记录头：0x16 0x03 ver(len>=5) len(2B) —— 已由调用方确认 p[0]==0x16。
	// 记录头长度字段只是提示，实际按 handshake 结构解析，但可先粗略校验。
	if len(p) < 9 {
		return ""
	}
	// 跳过 record 头 5 字节。
	hs := p[5:]
	if len(hs) < 4 || hs[0] != 0x01 { // handshake type ClientHello
		return ""
	}
	hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
	// handshake body 从 hs[4:] 开始，长度受 hsLen 与切片长度双重约束。
	body := hs[4:]
	if hsLen > 0 && len(body) > hsLen {
		body = body[:hsLen]
	}
	if len(body) < 2 {
		return ""
	}
	// ClientHello.version 2B
	// random 32B
	pos := 2 + 32
	if len(body) < pos+1 {
		return ""
	}
	// session_id len
	sidLen := int(body[pos])
	pos++
	if len(body) < pos+sidLen {
		return ""
	}
	pos += sidLen
	// cipher_suites len (2B)
	if len(body) < pos+2 {
		return ""
	}
	csLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2
	if len(body) < pos+csLen {
		return ""
	}
	pos += csLen
	// compression_methods len (1B)
	if len(body) < pos+1 {
		return ""
	}
	compLen := int(body[pos])
	pos++
	if len(body) < pos+compLen {
		return ""
	}
	pos += compLen
	// extensions len (2B) —— 可选，有些 ClientHello 无扩展
	if len(body) < pos+2 {
		return ""
	}
	extLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2
	if pos+extLen > len(body) {
		extLen = len(body) - pos
	}
	ext := body[pos : pos+extLen]
	// 遍历 extension：type(2) len(2) data
	i := 0
	for i+4 <= len(ext) {
		typ := binary.BigEndian.Uint16(ext[i:])
		l := int(binary.BigEndian.Uint16(ext[i+2:]))
		i += 4
		if i+l > len(ext) {
			break
		}
		if typ == 0x0000 { // server_name
			data := ext[i : i+l]
			// server_name 列表：listLen(2) 0x00 nameType(1) nameLen(2) name
			if len(data) >= 3 {
				// listLen 可忽略（通常含一个 host_name）
				off := 2
				if len(data) < off+1 {
					return ""
				}
				if data[off] != 0x00 { // 非 host_name 类型，跳过
					// 仍尝试找第一个 0x00 后跟 nameLen
				}
				if len(data) >= off+3 {
					nl := int(binary.BigEndian.Uint16(data[off+1:]))
					if off+3+nl <= len(data) {
						name := string(data[off+3 : off+3+nl])
						if name != "" {
							return name
						}
					}
				}
			}
			// 更简单的兜底：直接在 data 里找 len 前缀后的可打印串
			for j := 0; j+2 < len(data); j++ {
				if data[j] == 0x00 {
					nl := int(binary.BigEndian.Uint16(data[j+1:]))
					if j+3+nl <= len(data) {
						if name := printable(data[j+3 : j+3+nl]); name != "" {
							return name
						}
					}
				}
			}
			return ""
		}
		i += l
	}
	return ""
}

func printable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return ""
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// parseDNSQueryName 解析 DNS 查询的 QNAME（长度前缀标签序列）。
// p 假定已从 DNS 报文开始（含 header）。返回第一个可读域名，失败返回 ""。
func parseDNSQueryName(p []byte) string {
	// DNS header 12 字节；QNAME 从 offset 12 开始，标签以 0x00 结束。
	if len(p) < 13 {
		return ""
	}
	// 跳过可能存在的 TCP 长度前缀（2B 0x00 xx）——UDP 查询则直接是 header。
	// 这里不强制，直接用 [12:]。
	off := 12
	var labels []string
	for {
		if off >= len(p) {
			return ""
		}
		l := int(p[off])
		if l == 0 {
			break
		}
		if l&0xc0 == 0xc0 { // 压缩指针：跳过（查询报文一般不用）
			break
		}
		if l > 63 || off+1+l > len(p) {
			return ""
		}
		labels = append(labels, string(p[off+1:off+1+l]))
		off += 1 + l
		if len(labels) > 10 {
			return ""
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ".")
}

// isHTTP 判断载荷是否像 HTTP 请求（方法行）。
func isHTTP(p []byte) bool {
	// 常见方法后跟空格
	methods := []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "CONNECT ", "PATCH "}
	head := p
	if len(head) > 16 {
		head = head[:16]
	}
	for _, m := range methods {
		if len(head) >= len(m) && strings.HasPrefix(string(head), m) {
			return true
		}
	}
	// 或包含 CRLF Host:
	return hasHostHeader(p)
}

func hasHostHeader(p []byte) bool {
	lower := strings.ToLower(string(p))
	return strings.Contains(lower, "\r\nhost:")
}

// httpHost 提取 HTTP 请求里的 Host 头值。
func httpHost(p []byte) string {
	s := string(p)
	idx := strings.Index(strings.ToLower(s), "\r\nhost:")
	if idx < 0 {
		return ""
	}
	rest := s[idx+7:] // 跳过 "\r\nhost:"
	rest = strings.TrimLeft(rest, " \t")
	end := strings.IndexAny(rest, "\r\n")
	if end < 0 {
		end = len(rest)
	}
	return rest[:end]
}
