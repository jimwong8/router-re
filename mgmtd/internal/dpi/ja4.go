package dpi

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// TLS 客户端指纹（JA3 / JA4）。
//
// 用处：SNI 拿不到的时候（ECH、直连 IP、部分客户端根本不发 SNI），
// ClientHello 里的密码套件、扩展、椭圆曲线、签名算法仍然能把这个客户端
// 认出来。同一个软件连一百个站点，指纹是一样的——这是链路层/端口都给不了
// 的信息。
//
// JA4 是 FoxIO 对 JA3 的改良（把字段拆成三段、可读性更好、哈希只取前 12 位）。
// 这里两种都算：JA3 生态里有现成库，JA4 更适合人看。

// ClientHelloInfo 是 ClientHello 里与指纹相关的字段。
type ClientHelloInfo struct {
	LegacyVersion uint16
	SNI           string
	ALPN          []string
	CipherSuites  []uint16
	Extensions    []uint16
	Groups        []uint16 // supported_groups
	ECPointFmts   []uint8  // ec_point_formats
	SigAlgs       []uint16
	SupportedVers []uint16 // supported_versions（TLS 1.3 起）
	IsQUIC        bool
}

// ParseClientHello 解析 ClientHello。
//
// 入参可以是：
//   - 完整 TLS 记录（0x16 0x03 xx ...），TCP 上的常见形态
//   - 裸握手消息（0x01 xx xx xx ...），QUIC 里的形态
//
// 解析失败返回 nil，绝不 panic——这段代码会直接吃网络上的任意字节。
func ParseClientHello(b []byte) *ClientHelloInfo {
	if len(b) >= 5 && b[0] == 0x16 && b[1] == 0x03 {
		// TLS 记录头：type(1) version(2) length(2)
		recLen := int(binary.BigEndian.Uint16(b[3:5]))
		end := 5 + recLen
		if end > len(b) {
			end = len(b)
		}
		b = b[5:end]
	}
	if len(b) < 4 || b[0] != 0x01 {
		return nil
	}
	hlen := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if hlen == 0 || 4+hlen > len(b) {
		return nil
	}
	body := b[4 : 4+hlen]

	pos := 0
	if len(body) < 2+32+1 {
		return nil
	}
	info := &ClientHelloInfo{LegacyVersion: binary.BigEndian.Uint16(body[pos:])}
	pos += 2 + 32

	sidLen := int(body[pos])
	pos++
	if pos+sidLen > len(body) {
		return nil
	}
	pos += sidLen

	if pos+2 > len(body) {
		return nil
	}
	csLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2
	if csLen%2 != 0 || pos+csLen > len(body) {
		return nil
	}
	for i := 0; i < csLen; i += 2 {
		info.CipherSuites = append(info.CipherSuites, binary.BigEndian.Uint16(body[pos+i:]))
	}
	pos += csLen

	if pos+1 > len(body) {
		return nil
	}
	compLen := int(body[pos])
	pos++
	if pos+compLen > len(body) {
		return nil
	}
	pos += compLen

	if pos+2 > len(body) {
		// 没有扩展也是合法 ClientHello（SSLv3 时代），只是指纹信息少
		return info
	}
	extLen := int(binary.BigEndian.Uint16(body[pos:]))
	pos += 2
	end := pos + extLen
	if end > len(body) {
		end = len(body)
	}

	for pos+4 <= end {
		etype := binary.BigEndian.Uint16(body[pos:])
		elen := int(binary.BigEndian.Uint16(body[pos+2:]))
		pos += 4
		if pos+elen > end {
			return info
		}
		ext := body[pos : pos+elen]
		info.Extensions = append(info.Extensions, etype)

		switch etype {
		case 0x0000: // server_name
			if s := parseServerNameRaw(ext); s != "" {
				info.SNI = s
			}
		case 0x0010: // ALPN
			info.ALPN = parseALPN(ext)
		case 0x000a: // supported_groups
			info.Groups = parseU16List(ext)
		case 0x000b: // ec_point_formats
			if len(ext) >= 1 {
				n := int(ext[0])
				if n <= len(ext)-1 {
					info.ECPointFmts = append([]uint8(nil), ext[1:1+n]...)
				}
			}
		case 0x000d: // signature_algorithms
			info.SigAlgs = parseU16List(ext)
		case 0x002b: // supported_versions：长度前缀是 1 字节，和 0x000a/0x000d 不同
			info.SupportedVers = parseU16ListU8(ext)
		}
		pos += elen
	}
	return info
}

// parseServerNameRaw 取 server_name 扩展里的 host_name，不做域名合法性校验。
//
// 与 quic.go 里的 parseServerNameExt 的区别：那个用于「从网络流量里猜应用」，
// 必须挡住二进制误判；这里用于指纹，只关心 SNI 是否存在，localhost 这类
// 不含点的名字也是合法的。
func parseServerNameRaw(b []byte) string {
	if len(b) < 5 {
		return ""
	}
	if int(binary.BigEndian.Uint16(b))+2 > len(b) {
		return ""
	}
	off := 2
	for off+3 <= len(b) {
		nameType := b[off]
		nameLen := int(binary.BigEndian.Uint16(b[off+1:]))
		off += 3
		if off+nameLen > len(b) {
			return ""
		}
		if nameType == 0 {
			return string(b[off : off+nameLen])
		}
		off += nameLen
	}
	return ""
}

// parseALPN 解析 ALPN 扩展，返回协议名列表。
func parseALPN(b []byte) []string {
	if len(b) < 2 {
		return nil
	}
	listLen := int(binary.BigEndian.Uint16(b))
	if listLen+2 > len(b) {
		listLen = len(b) - 2
	}
	var out []string
	pos := 2
	for pos < 2+listLen {
		n := int(b[pos])
		pos++
		if pos+n > len(b) {
			break
		}
		out = append(out, string(b[pos:pos+n]))
		pos += n
	}
	return out
}

// parseU16List 解析「长度前缀 + uint16 数组」的扩展体。
func parseU16List(b []byte) []uint16 {
	if len(b) < 2 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(b))
	if 2+n > len(b) {
		n = len(b) - 2
	}
	var out []uint16
	for i := 0; i+2 <= n; i += 2 {
		out = append(out, binary.BigEndian.Uint16(b[2+i:]))
	}
	return out
}

// parseU16ListU8 解析「1 字节长度前缀 + uint16 数组」的扩展体
// （目前只有 supported_versions 是这种格式）。
func parseU16ListU8(b []byte) []uint16 {
	if len(b) < 1 {
		return nil
	}
	n := int(b[0])
	if 1+n > len(b) {
		n = len(b) - 1
	}
	var out []uint16
	for i := 0; i+2 <= n; i += 2 {
		out = append(out, binary.BigEndian.Uint16(b[1+i:]))
	}
	return out
}

// isGREASE 判断是否为 GREASE 值（0x0A0A/0x1A1A/…/0xFAFA）。
// 客户端随机塞进来的填充值，参与指纹会让同一客户端每次算出不同结果。
func isGREASE(v uint16) bool {
	return v&0x0f0f == 0x0a0a && (v>>8)&0x0f == v&0x0f
}

// JA4 计算 JA4 指纹，形如 t13d1516h2_8daaf6152771_b186095e22b6。
func (c *ClientHelloInfo) JA4() string {
	if c == nil {
		return ""
	}
	proto := "t"
	if c.IsQUIC {
		proto = "q"
	}

	ver := "00"
	best := c.LegacyVersion
	for _, v := range c.SupportedVers {
		if isGREASE(v) {
			continue
		}
		if v > best {
			best = v
		}
	}
	switch best {
	case 0x0304:
		ver = "13"
	case 0x0303:
		ver = "12"
	case 0x0302:
		ver = "11"
	case 0x0301:
		ver = "10"
	case 0x0300:
		ver = "s3"
	}

	sniFlag := "i"
	for _, e := range c.Extensions {
		if e == 0x0000 { // server_name 扩展存在即为 d，即使名字解析失败
			sniFlag = "d"
			break
		}
	}

	ciphers := nonGREASE(c.CipherSuites)
	allExts := nonGREASE(c.Extensions)
	exts := nonGREASEExt(c.Extensions)

	alpn := "00"
	if len(c.ALPN) > 0 && c.ALPN[0] != "" {
		a := c.ALPN[0]
		if len(a) == 1 {
			alpn = a + a
		} else {
			alpn = string(a[0]) + string(a[len(a)-1])
		}
	}

	a := fmt.Sprintf("%s%s%s%02d%02d%s", proto, ver, sniFlag,
		min(len(ciphers), 99), min(len(allExts), 99), alpn)

	b := hash12(joinHexU16(ciphers))

	cPart := joinHexU16(exts)
	if len(c.SigAlgs) > 0 {
		// 签名算法按出现顺序，不排序 —— 顺序本身是客户端特征的一部分
		cPart += "_" + joinHexU16Ordered(nonGREASE(c.SigAlgs))
	}
	cHash := hash12(cPart)

	return a + "_" + b + "_" + cHash
}

// JA3 计算 JA3 指纹（32 位 md5，生态兼容用）。
func (c *ClientHelloInfo) JA3() string {
	if c == nil {
		return ""
	}
	ver := int(c.LegacyVersion)
	if len(c.SupportedVers) > 0 {
		// JA3 传统上仍用 ClientHello.version 字段，这里保持一致
		_ = c.SupportedVers
	}
	parts := []string{
		fmt.Sprint(ver),
		joinDec(c.CipherSuites),
		joinDec(c.Extensions),
		joinDec(c.Groups),
		joinDecU8(c.ECPointFmts),
	}
	sum := md5.Sum([]byte(strings.Join(parts, ",")))
	return hex.EncodeToString(sum[:])
}

// ── 小工具 ────────────────────────────────────────────────────────────────

func nonGREASE(vs []uint16) []uint16 {
	out := make([]uint16, 0, len(vs))
	for _, v := range vs {
		if !isGREASE(v) {
			out = append(out, v)
		}
	}
	return out
}

// nonGREASEExt 去掉 GREASE，并排除 SNI(0x0000) 与 ALPN(0x0010)——
// 这两项已经体现在 JA4_a 里，重复计入会让"同一客户端不同站点"算出不同指纹。
func nonGREASEExt(vs []uint16) []uint16 {
	out := make([]uint16, 0, len(vs))
	for _, v := range vs {
		if isGREASE(v) || v == 0x0000 || v == 0x0010 {
			continue
		}
		out = append(out, v)
	}
	return out
}

func joinHexU16(vs []uint16) string {
	s := make([]uint16, len(vs))
	copy(s, vs)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = fmt.Sprintf("%04x", v)
	}
	return strings.Join(parts, ",")
}

func joinDec(vs []uint16) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, "-")
}

func joinDecU8(vs []uint8) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, "-")
}

func hash12(s string) string {
	if s == "" {
		return "000000000000"
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// joinHexU16Ordered 按原顺序拼 4 位十六进制，不排序。
func joinHexU16Ordered(vs []uint16) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = fmt.Sprintf("%04x", v)
	}
	return strings.Join(parts, ",")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// KnownClient 把实测到的 JA4 映射到客户端软件名。
//
// 这张表只放**实测过**的值——抓真实流量算出来、并复验过稳定性的，不凭
// 网上抄来的数字。认证信息只有这几条，其余一律不猜。
// 只收录本机实测过的值：抓真实握手算出、并复验过同一客户端多次连接结果稳定。
// 不同版本的 OpenSSL/浏览器会给不同指纹，所以不做"猜"的映射。
var KnownClient = map[string]string{
	"t13d3013h2_1d37bd780c83_8537cf56674e": "curl / OpenSSL 3.5",
	"t13d301200_1d37bd780c83_ecd0401ec68b": "Python ssl / OpenSSL 3.5",
	"t13d131000_f57a46bbacb6_e7c285222651": "Go crypto/tls",
}

// DescribeClient 返回已知客户端名，未知则返回空。
func DescribeClient(ja4 string) string {
	return KnownClient[ja4]
}
