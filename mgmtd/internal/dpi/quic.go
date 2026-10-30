package dpi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"sync"
	"time"
)

// QUIC Initial 包解密，用来取出明文 SNI。
//
// 为什么值得做：QUIC（HTTP/3）的 SNI 在解密前完全不可见，而抖音、YouTube、
// B 站这类流量现在大量走 QUIC。只靠 nDPI 的 TLS 识别，这类流会一直停在
// Unknown——实测一段普通浏览抓包里 21 条流有 14 条是这种情况，识别率被卡在
// 三分之一。
//
// QUIC 的 Initial 包用的是「公开的盐 + 客户端首个 DCID」派生的密钥（RFC 9001
// §5.2）：密钥材料全部来自握手包里的明文连接 ID，谁都能算。也就是说这不算
// 破解，只是把协议公开允许的计算做一遍。0-RTT/Handshake/1-RTT 的密钥来自
// TLS 握手，这里不碰——那些确实解不开。

// QUIC 版本号与对应的 Initial 盐值（RFC 9001 §5.2、RFC 9369 §3.3）。
const (
	quicVersionV1      = 0x00000001
	quicVersionV2      = 0x6b3343cf
	quicVersionDraft29 = 0xff00001d
)

var quicInitialSalts = map[uint32][]byte{
	quicVersionV1:      {0x38, 0x76, 0x2c, 0xf7, 0xf5, 0x59, 0x34, 0xb3, 0x4d, 0x17, 0x9a, 0xe6, 0xa4, 0xc8, 0x0c, 0xad, 0xcc, 0xbb, 0x7f, 0x0a},
	quicVersionV2:      {0x0d, 0xed, 0xe3, 0xde, 0xf7, 0x00, 0xa6, 0xdb, 0x81, 0x93, 0x81, 0xbe, 0x6e, 0x26, 0x9d, 0xcb, 0xf9, 0xbd, 0x2e, 0xd9},
	quicVersionDraft29: {0xaf, 0xbf, 0xec, 0x28, 0x99, 0x93, 0xd2, 0x4c, 0x9e, 0x97, 0x86, 0xf1, 0x9c, 0x61, 0x11, 0xe0, 0x43, 0x90, 0xa8, 0x99},
}

// 各种长包头包类型在两个版本里的编号不同（v2 重排过，RFC 9369 §3.2）。
const (
	quicTypeInitialV1 = 0
	quicTypeInitialV2 = 2
)

// quicMaxDatagram 限制单个 UDP 载荷的处理长度，避免异常包撑爆解析。
const quicMaxDatagram = 1500

// QUICSNI 尝试从一段 UDP 载荷里解出客户端 Initial 包中的 SNI。
//
// 返回空字符串表示：不是 QUIC、不是 Initial、是服务端包、或解密失败。
// 调用方不应把它当成错误——绝大多数 UDP 都不是可解的 QUIC Initial。
func QUICSNI(datagram []byte) string {
	if len(datagram) < 48 || len(datagram) > quicMaxDatagram {
		return ""
	}
	// 长包头：首字节最高位为 1
	if datagram[0]&0x80 == 0 {
		return ""
	}

	version := binary.BigEndian.Uint32(datagram[1:5])
	salt, ok := quicInitialSalts[version]
	if !ok {
		return "" // 版本协商(0x0)或未知版本
	}

	pktType := (datagram[0] >> 4) & 0x03
	wantType := byte(quicTypeInitialV1)
	if version == quicVersionV2 {
		wantType = quicTypeInitialV2
	}
	if pktType != wantType {
		return ""
	}

	off := 5
	dcid, off, ok := quicConnID(datagram, off)
	if !ok {
		return ""
	}
	_, off, ok = quicConnID(datagram, off) // SCID：解析出来只为推进偏移
	if !ok {
		return ""
	}

	// Initial 包带 token（首次连接时长度为 0，Retry 之后会有值）
	tokenLen, n, ok := quicVarint(datagram[off:])
	if !ok {
		return ""
	}
	off += n
	if uint64(len(datagram)-off) < tokenLen {
		return ""
	}
	off += int(tokenLen)

	length, n, ok := quicVarint(datagram[off:])
	if !ok {
		return ""
	}
	off += n
	pnOffset := off
	if length == 0 || pnOffset+int(length) > len(datagram) {
		return ""
	}

	// 头保护：采样从包号字段之后 4 字节开始，取 16 字节
	sampleOff := pnOffset + 4
	if sampleOff+16 > len(datagram) {
		return ""
	}
	sample := datagram[sampleOff : sampleOff+16]

	hpKey, key, iv, err := quicInitialKeys(salt, dcid)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(hpKey)
	if err != nil {
		return ""
	}
	mask := make([]byte, 16)
	block.Encrypt(mask, sample)

	// 去头保护：首字节低 4 位是包号长度（长包头还需清掉保留位）
	first := datagram[0] ^ (mask[0] & 0x0f)
	pnLen := int(first&0x03) + 1
	if pnOffset+pnLen > len(datagram) {
		return ""
	}
	pnBytes := make([]byte, pnLen)
	for i := 0; i < pnLen; i++ {
		pnBytes[i] = datagram[pnOffset+i] ^ mask[1+i]
	}
	var pn uint64
	for _, b := range pnBytes {
		pn = pn<<8 | uint64(b)
	}

	// AAD 是未加密的头（含去掉保护后的包号）
	header := make([]byte, pnOffset+pnLen)
	copy(header, datagram[:pnOffset])
	header[0] = first
	copy(header[pnOffset:], pnBytes)

	aead, err := cipher.NewGCM(block2(key))
	if err != nil {
		return ""
	}
	nonce := make([]byte, len(iv))
	copy(nonce, iv)
	var pnFull [8]byte
	binary.BigEndian.PutUint64(pnFull[:], pn)
	for i := 0; i < 8; i++ {
		nonce[len(nonce)-8+i] ^= pnFull[i]
	}

	ciphertext := datagram[pnOffset+pnLen : pnOffset+int(length)]
	plaintext, err := aead.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return ""
	}

	return quicFrames(string(dcid), plaintext)
}

// block2 把密钥包成 cipher.Block（GCM 需要）。
func block2(key []byte) cipher.Block {
	b, err := aes.NewCipher(key)
	if err != nil {
		panic("quic: invalid aes key length")
	}
	return b
}

// quicConnID 读一个「1 字节长度 + 数据」的连接 ID。
func quicConnID(b []byte, off int) ([]byte, int, bool) {
	if off >= len(b) {
		return nil, off, false
	}
	l := int(b[off])
	off++
	if l > 20 || off+l > len(b) {
		return nil, off, false
	}
	return b[off : off+l], off + l, true
}

// quicVarint 解析 QUIC 可变长整数（RFC 9000 §16）。
func quicVarint(b []byte) (uint64, int, bool) {
	if len(b) == 0 {
		return 0, 0, false
	}
	n := 1 << (b[0] >> 6) // 00->1, 01->2, 10->4, 11->8
	if len(b) < n {
		return 0, 0, false
	}
	v := uint64(b[0] & 0x3f)
	for i := 1; i < n; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v, n, true
}

// quicInitialKeys 派生 Initial 包的 hp/key/iv（客户端方向）。
func quicInitialKeys(salt, dcid []byte) (hp, key, iv []byte, err error) {
	initialSecret, err := hkdf.Extract(sha256.New, dcid, salt)
	if err != nil {
		return nil, nil, nil, err
	}
	clientSecret, err := quicExpandLabel(initialSecret, "client in", 32)
	if err != nil {
		return nil, nil, nil, err
	}
	if key, err = quicExpandLabel(clientSecret, "quic key", 16); err != nil {
		return nil, nil, nil, err
	}
	if iv, err = quicExpandLabel(clientSecret, "quic iv", 12); err != nil {
		return nil, nil, nil, err
	}
	if hp, err = quicExpandLabel(clientSecret, "quic hp", 16); err != nil {
		return nil, nil, nil, err
	}
	return hp, key, iv, nil
}

// quicExpandLabel 实现 TLS 1.3 的 HKDF-Expand-Label。
func quicExpandLabel(secret []byte, label string, length int) ([]byte, error) {
	full := "tls13 " + label
	info := make([]byte, 0, 4+len(full)+1)
	info = append(info, byte(length>>8), byte(length))
	info = append(info, byte(len(full)))
	info = append(info, full...)
	info = append(info, 0) // 空 context
	return hkdf.Expand(sha256.New, secret, string(info), length)
}

// sniFromQUICFrames 从解密后的帧里取出 CRYPTO 片段（保留旧名供测试与阅读）。
func sniFromQUICFrames(plain []byte) string {
	return quicFrames("", plain)
}

// quicFrames 遍历解密后的帧，把 CRYPTO 片段交给重组器。
//
// 为什么必须重组：Initial 包最大 1200 字节，而一个真实的 ClientHello
// 常见 1500 字节以上，客户端会把它切成两个 Initial 包发。只看单包，
// CRYPTO 帧里是半截握手，长度字段对不上，永远解析不出 SNI。
func quicFrames(cryptoKey string, plain []byte) string {
	off := 0
	for off < len(plain) {
		frameType, n, ok := quicVarint(plain[off:])
		if !ok {
			return ""
		}
		off += n
		switch frameType {
		case 0x00, 0x01: // PADDING / PING
			continue
		case 0x02, 0x03: // ACK：跳过 largest/last/delay/count/ranges
			if _, n, ok := quicVarint(plain[off:]); !ok {
				return ""
			} else {
				off += n
			}
			if _, n, ok := quicVarint(plain[off:]); !ok {
				return ""
			} else {
				off += n
			}
			if _, n, ok := quicVarint(plain[off:]); !ok {
				return ""
			} else {
				off += n
			}
			cnt, n, ok := quicVarint(plain[off:])
			if !ok {
				return ""
			}
			off += n
			if _, n, ok := quicVarint(plain[off:]); !ok { // first_range
				return ""
			} else {
				off += n
			}
			for i := uint64(0); i < cnt && i < 64; i++ {
				if _, n, ok := quicVarint(plain[off:]); !ok {
					return ""
				} else {
					off += n
				}
				if _, n, ok := quicVarint(plain[off:]); !ok {
					return ""
				} else {
					off += n
				}
			}
		case 0x06: // CRYPTO
			frameOff, n, ok := quicVarint(plain[off:])
			if !ok {
				return ""
			}
			off += n
			length, n, ok := quicVarint(plain[off:])
			if !ok {
				return ""
			}
			off += n
			if length == 0 || off+int(length) > len(plain) {
				return ""
			}
			data := plain[off : off+int(length)]
			off += int(length)
			if sni := quicReassemble(cryptoKey, frameOff, data); sni != "" {
				return sni
			}
		default:
			// 其它帧类型（如 CONNECTION_CLOSE）无法可靠跳过，直接放弃
			return ""
		}
	}
	return ""
}

// sniFromHandshake 从 TLS 握手字节流里取 ClientHello 的 SNI。
//
// 与 dpiflow 里的实现刻意分开：QUIC 的握手没有 TLS 记录层，从 handshake
// message 开始，不能复用带 0x16/0x03 记录头的解析器。
func sniFromHandshake(b []byte) string {
	if len(b) < 4 || b[0] != 0x01 { // handshake type = ClientHello
		return ""
	}
	hlen := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if hlen == 0 || 4+hlen > len(b) {
		return ""
	}
	b = b[4 : 4+hlen]

	off := 0
	// legacy_version(2) + random(32)
	off += 2 + 32
	// session id
	if off >= len(b) {
		return ""
	}
	off += 1 + int(b[off])
	// cipher suites
	if off+2 > len(b) {
		return ""
	}
	csLen := int(binary.BigEndian.Uint16(b[off:]))
	off += 2 + csLen
	// compression methods
	if off >= len(b) {
		return ""
	}
	off += 1 + int(b[off])
	// extensions
	if off+2 > len(b) {
		return ""
	}
	extLen := int(binary.BigEndian.Uint16(b[off:]))
	off += 2
	end := off + extLen
	if end > len(b) {
		end = len(b)
	}
	for off+4 <= end {
		etype := binary.BigEndian.Uint16(b[off:])
		elen := int(binary.BigEndian.Uint16(b[off+2:]))
		off += 4
		if off+elen > end {
			return ""
		}
		if etype == 0x0000 { // server_name
			return parseServerNameExt(b[off : off+elen])
		}
		off += elen
	}
	return ""
}

// parseServerNameExt 解析 server_name 扩展。
func parseServerNameExt(b []byte) string {
	if len(b) < 5 {
		return ""
	}
	off := 2 // server_name_list 长度
	for off+3 <= len(b) {
		nameType := b[off]
		nameLen := int(binary.BigEndian.Uint16(b[off+1:]))
		off += 3
		if off+nameLen > len(b) {
			return ""
		}
		if nameType == 0 { // host_name
			h := string(b[off : off+nameLen])
			if validQUICHost(h) {
				return h
			}
			return ""
		}
		off += nameLen
	}
	return ""
}

// validQUICHost 校验解出来的名字像域名，挡住解密"成功"但内容其实是垃圾的情况。
func validQUICHost(h string) bool {
	if len(h) < 4 || len(h) > 253 || !strings.Contains(h, ".") {
		return false
	}
	alpha := false
	for _, c := range h {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			alpha = true
		case c >= '0' && c <= '9', c == '-', c == '.':
		default:
			return false
		}
	}
	return alpha
}

// ── CRYPTO 流重组 ─────────────────────────────────────────────────────────
//
// 每个连接保留一小段「从偏移 0 开始的连续握手字节」。Initial 包的 CRYPTO
// 片段可能乱序、重叠或分片，这里按偏移铺平，能连续读到完整 ClientHello
// 时解析一次 SNI 并缓存；解析成功后该连接不再参与重组。

type quicStream struct {
	prefix []byte            // 从偏移 0 起的连续数据
	chunks map[uint64][]byte // 尚未接上的片段
	last   time.Time
	done   bool
	sni    string
}

var (
	quicMu       sync.Mutex
	quicStreams  = map[string]*quicStream{}
	quicLastScan time.Time
)

const (
	quicStreamTTL    = 60 * time.Second
	quicMaxStreams   = 2048
	quicMaxHandshake = 16 << 10 // 握手前 16KB 足够覆盖 ClientHello
)

// quicReassemble 把一段 CRYPTO 数据放进该连接的重组缓冲，能解析时返回 SNI。
func quicReassemble(key string, frameOff uint64, data []byte) string {
	if key == "" || len(data) == 0 {
		return ""
	}

	quicMu.Lock()
	defer quicMu.Unlock()

	now := time.Now()
	if now.Sub(quicLastScan) > quicStreamTTL {
		quicLastScan = now
		for k, st := range quicStreams {
			if now.Sub(st.last) > quicStreamTTL {
				delete(quicStreams, k)
			}
		}
		// 兜底：条目过多时清空，避免异常流量把内存撑爆
		if len(quicStreams) > quicMaxStreams {
			quicStreams = map[string]*quicStream{}
		}
	}

	st := quicStreams[key]
	if st == nil {
		st = &quicStream{chunks: map[uint64][]byte{}}
		quicStreams[key] = st
	}
	st.last = now
	if st.done {
		return st.sni
	}

	// 与已有前缀重叠的部分直接丢弃，只补新字节
	if frameOff < uint64(len(st.prefix)) {
		skip := uint64(len(st.prefix)) - frameOff
		if skip >= uint64(len(data)) {
			return ""
		}
		data = data[skip:]
		frameOff = uint64(len(st.prefix))
	}
	if frameOff > uint64(len(st.prefix)) {
		if _, ok := st.chunks[frameOff]; !ok {
			st.chunks[frameOff] = append([]byte(nil), data...)
		}
	} else {
		st.prefix = append(st.prefix, data...)
		// 把能接上的后续片段连起来
		for {
			next, ok := st.chunks[uint64(len(st.prefix))]
			if !ok {
				break
			}
			delete(st.chunks, uint64(len(st.prefix)))
			st.prefix = append(st.prefix, next...)
		}
	}

	if uint64(len(st.prefix)) > quicMaxHandshake {
		st.prefix = nil
		st.chunks = map[uint64][]byte{}
		return ""
	}

	// 手上已有完整 ClientHello 才解析
	if len(st.prefix) < 4 || st.prefix[0] != 0x01 {
		return ""
	}
	hlen := int(st.prefix[1])<<16 | int(st.prefix[2])<<8 | int(st.prefix[3])
	if hlen == 0 || 4+hlen > len(st.prefix) {
		return ""
	}

	sni := sniFromHandshake(st.prefix[:4+hlen])
	if sni != "" {
		st.done = true
		st.sni = sni
		st.prefix = nil
		st.chunks = map[uint64][]byte{}
	}
	return sni
}

// ResetQUICStreams 清空重组状态（测试与 pcap 回放用）。
func ResetQUICStreams() {
	quicMu.Lock()
	quicStreams = map[string]*quicStream{}
	quicLastScan = time.Time{}
	quicMu.Unlock()
}

// QUICStreamStats 返回重组器状态。
func QUICStreamStats() map[string]interface{} {
	quicMu.Lock()
	defer quicMu.Unlock()
	solved := 0
	for _, st := range quicStreams {
		if st.done {
			solved++
		}
	}
	return map[string]interface{}{
		"active":  len(quicStreams),
		"solved":  solved,
		"max":     quicMaxStreams,
		"ttl_sec": int(quicStreamTTL / time.Second),
	}
}
