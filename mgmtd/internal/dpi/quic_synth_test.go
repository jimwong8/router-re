package dpi

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"testing"
)

// QUIC Initial 的合成测试。
//
// 为什么需要：QUIC 路径此前只有一段真实抓包作证据（3 条流），
// 而抓包里恰好是「一个包装下整个 ClientHello」的理想情况。
// 分片、Retry 带 token、v2 版本这些分支根本没有样本覆盖，
// 一旦改坏也没人知道。这里用真实密钥派生出可解密的 Initial，
// 把这几条分支逐个钉住。
//
// 注意：加密用的 quicInitialKeys 与被测代码是同一个函数。
// 所以这组测试证明的是「组帧、头保护、重组、解析」这些环节正确，
// 不能证明密钥派生本身正确——密钥派生由真实抓包与在线 QUIC 服务验证。

// quicVarintAppend 按 RFC 9000 编码变长整数。
func quicVarintAppend(dst []byte, v uint64) []byte {
	switch {
	case v < 1<<6:
		return append(dst, byte(v))
	case v < 1<<14:
		return append(dst, byte(v>>8)|0x40, byte(v))
	case v < 1<<30:
		return append(dst, byte(v>>24)|0x80, byte(v>>16), byte(v>>8), byte(v))
	default:
		return append(dst, byte(v>>56)|0xc0, byte(v>>48), byte(v>>40), byte(v>>32),
			byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
}

// quicTestNonce 按 RFC 9001 §5.3 计算 AEAD nonce：iv 与包号异或。
// 包号为 0 时 nonce 恰好等于 iv —— 只测 pn=0 的话，漏掉这一步也发现不了。
func quicTestNonce(iv []byte, pn uint64) []byte {
	nonce := append([]byte(nil), iv...)
	var pnFull [8]byte
	binary.BigEndian.PutUint64(pnFull[:], pn)
	for i := 0; i < 8; i++ {
		nonce[len(nonce)-8+i] ^= pnFull[i]
	}
	return nonce
}

// buildInitialPacket 构造一个可被 quicDecrypt 解开的长包头 Initial 包。
func buildInitialPacket(t *testing.T, version uint32, dcid, scid, token, crypto []byte, pn uint64) []byte {
	t.Helper()
	salt, ok := quicInitialSalts[version]
	if !ok {
		t.Fatalf("版本 %08x 没有 salt", version)
	}
	hpKey, key, iv, err := quicInitialKeys(salt, dcid)
	if err != nil {
		t.Fatalf("派生密钥失败: %v", err)
	}

	// CRYPTO 帧：type(0x06) offset(len 前缀变长) length(data)
	frame := []byte{0x06}
	frame = quicVarintAppend(frame, 0)
	frame = quicVarintAppend(frame, uint64(len(crypto)))
	frame = append(frame, crypto...)

	pktType := byte(quicTypeInitialV1)
	if version == quicVersionV2 {
		pktType = quicTypeInitialV2
	}
	// 首字节：长包头(1) 固定位(1) 类型(2) 保留(2) 包号长度-1(2) → 1 字节包号
	first := byte(0xc0) | (pktType << 4)

	hdr := []byte{first}
	hdr = binary.BigEndian.AppendUint32(hdr, version)
	hdr = append(hdr, byte(len(dcid)))
	hdr = append(hdr, dcid...)
	hdr = append(hdr, byte(len(scid)))
	hdr = append(hdr, scid...)
	hdr = quicVarintAppend(hdr, uint64(len(token)))
	hdr = append(hdr, token...)

	payloadLen := 1 + len(frame) + 16 // 包号 + 帧 + AEAD tag
	hdr = quicVarintAppend(hdr, uint64(payloadLen))
	hdr = append(hdr, byte(pn)) // 包号是头部的一部分，参与 AAD 与头保护

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	sealed := aead.Seal(nil, quicTestNonce(iv, pn), frame, hdr)

	pkt := append(append([]byte{}, hdr...), sealed...)

	// 头保护样本：从包号字段起 4 字节之后取 16 字节
	sampleOff := len(hdr) - 1 + 4
	if sampleOff+16 > len(pkt) {
		t.Fatalf("包太短无法做头保护（%d 字节）", len(pkt))
	}
	hpBlock, err := aes.NewCipher(hpKey)
	if err != nil {
		t.Fatal(err)
	}
	mask := make([]byte, 16)
	hpBlock.Encrypt(mask, pkt[sampleOff:sampleOff+16])

	pkt[0] ^= mask[0] & 0x0f
	pkt[len(hdr)-1] ^= mask[1]
	return pkt
}

// TestQUICReassemblesFragmentedClientHello 一个 ClientHello 拆成两个 Initial 包也要能还原。
func TestQUICReassemblesFragmentedClientHello(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	ch := buildHello("split.example.com", []string{"h3"})
	// 去掉握手头（前 4 字节），quicReassemble 会自己按偏移重组
	body := ch[4:]
	half := len(body) / 2

	dcid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	// 第一个包带完整握手头 + 前半段
	first := append([]byte{0x01,
		byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body[:half]...)
	second := body[half:]

	if got := QUICClientHello(buildInitialPacket(t, quicVersionV1, dcid, nil, nil, first, 0)); got != nil && got.SNI != "" {
		t.Fatalf("前半段不该已经能解析出 SNI，实际 %q", got.SNI)
	}
	// 第二个包从流偏移 len(first) 开始
	frame2 := []byte{0x06}
	frame2 = quicVarintAppend(frame2, uint64(len(first)))
	frame2 = quicVarintAppend(frame2, uint64(len(second)))
	frame2 = append(frame2, second...)

	pkt2 := buildInitialPacketWithFrame(t, quicVersionV1, dcid, nil, nil, frame2, 1)
	got := QUICClientHello(pkt2)
	if got == nil {
		t.Fatal("分片重组失败：第二个包之后仍解析不出 ClientHello")
	}
	if got.SNI != "split.example.com" {
		t.Fatalf("SNI 应为 split.example.com，实际 %q", got.SNI)
	}
	if got.JA4() == "" || got.JA4()[0] != 'q' {
		t.Fatalf("QUIC 指纹应为 q 前缀，实际 %q", got.JA4())
	}
}

// buildInitialPacketWithFrame 用一个现成的 CRYPTO 帧（自定义偏移）构造 Initial 包。
func buildInitialPacketWithFrame(t *testing.T, version uint32, dcid, scid, token, frame []byte, pn uint64) []byte {
	t.Helper()
	salt := quicInitialSalts[version]
	hpKey, key, iv, err := quicInitialKeys(salt, dcid)
	if err != nil {
		t.Fatal(err)
	}
	pktType := byte(quicTypeInitialV1)
	if version == quicVersionV2 {
		pktType = quicTypeInitialV2
	}
	hdr := []byte{byte(0xc0) | (pktType << 4)}
	hdr = binary.BigEndian.AppendUint32(hdr, version)
	hdr = append(hdr, byte(len(dcid)))
	hdr = append(hdr, dcid...)
	hdr = append(hdr, byte(len(scid)))
	hdr = append(hdr, scid...)
	hdr = quicVarintAppend(hdr, uint64(len(token)))
	hdr = append(hdr, token...)
	hdr = quicVarintAppend(hdr, uint64(1+len(frame)+16))
	hdr = append(hdr, byte(pn))

	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	pkt := append(append([]byte{}, hdr...), aead.Seal(nil, quicTestNonce(iv, pn), frame, hdr)...)

	sampleOff := len(hdr) - 1 + 4
	hpBlock, _ := aes.NewCipher(hpKey)
	mask := make([]byte, 16)
	hpBlock.Encrypt(mask, pkt[sampleOff:sampleOff+16])
	pkt[0] ^= mask[0] & 0x0f
	pkt[len(hdr)-1] ^= mask[1]
	return pkt
}

// TestQUICPlainV1 最小用例：不分片、无 token、v1。构造器错了这里一定先红。
func TestQUICPlainV1(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	ch := buildHello("plain.example.com", []string{"h3"})
	frame := []byte{0x06}
	frame = quicVarintAppend(frame, 0)
	frame = quicVarintAppend(frame, uint64(len(ch)))
	frame = append(frame, ch...)

	dcid := []byte{1, 1, 1, 1, 2, 2, 2, 2}
	got := QUICClientHello(buildInitialPacketWithFrame(t, quicVersionV1, dcid, nil, nil, frame, 0))
	if got == nil {
		t.Fatal("最小 v1 Initial 解不开")
	}
	if got.SNI != "plain.example.com" {
		t.Fatalf("SNI 应为 plain.example.com，实际 %q", got.SNI)
	}
	if j := got.JA4(); len(j) < 10 || j[0] != 'q' {
		t.Fatalf("JA4 异常: %q", j)
	}
	t.Logf("v1 指纹 %s", got.JA4())
}

// TestQUICVersion2 v2 的包类型编号与 v1 不同，salt 也不同，必须都能解。
func TestQUICVersion2(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	ch := buildHello("v2.example.com", []string{"h3"})
	frame := []byte{0x06}
	frame = quicVarintAppend(frame, 0)
	frame = quicVarintAppend(frame, uint64(len(ch)))
	frame = append(frame, ch...)

	dcid := []byte{9, 9, 9, 9, 1, 2, 3, 4}
	got := QUICClientHello(buildInitialPacketWithFrame(t, quicVersionV2, dcid, nil, nil, frame, 0))
	if got == nil {
		t.Fatal("QUIC v2 的 Initial 解不开")
	}
	if got.SNI != "v2.example.com" {
		t.Fatalf("v2 SNI 应为 v2.example.com，实际 %q", got.SNI)
	}
}

// TestQUICRetryToken 带 token 的 Initial（Retry 之后）要能跳过 token 正常解密。
func TestQUICRetryToken(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	ch := buildHello("retry.example.com", []string{"h3"})
	frame := []byte{0x06}
	frame = quicVarintAppend(frame, 0)
	frame = quicVarintAppend(frame, uint64(len(ch)))
	frame = append(frame, ch...)

	dcid := []byte{7, 7, 7, 7, 8, 8, 8, 8}
	token := make([]byte, 40) // 模拟 Retry token
	for i := range token {
		token[i] = byte(i)
	}
	got := QUICClientHello(buildInitialPacketWithFrame(t, quicVersionV1, dcid, nil, token, frame, 3))
	if got == nil || got.SNI != "retry.example.com" {
		t.Fatalf("带 token 的 Initial 应能解出 SNI，实际 %v", got)
	}
}

// TestQUICServerInitialIgnored 服务端 Initial（用它自己的 DCID 派生的密钥）不应被当成客户端。
func TestQUICServerInitialIgnored(t *testing.T) {
	ResetQUICStreams()
	defer ResetQUICStreams()

	ch := buildHello("server.example.com", []string{"h3"})
	frame := []byte{0x06}
	frame = quicVarintAppend(frame, 0)
	frame = quicVarintAppend(frame, uint64(len(ch)))
	frame = append(frame, ch...)

	// 用「错误的」dcid 派生（模拟拿客户端 dcid 去解服务端包的情形）
	wrongDCID := []byte{0xde, 0xad, 0xbe, 0xef, 1, 1, 1, 1}
	pkt := buildInitialPacketWithFrame(t, quicVersionV1, wrongDCID, nil, nil, frame, 0)
	// 用一个不同的 dcid 头去解析：密钥不匹配 → 解密失败 → 返回 nil，不能 panic
	pkt[6] = 0x11
	pkt[7] = 0x22
	if got := QUICClientHello(pkt); got != nil && got.SNI == "server.example.com" {
		t.Fatal("密钥不匹配时不应解析出内容")
	}
}

// TestQUICGarbageNoPanic 任意 UDP 载荷都不能 panic。
func TestQUICGarbageNoPanic(t *testing.T) {
	for i := 0; i < 200; i++ {
		buf := make([]byte, i)
		for j := range buf {
			buf[j] = byte(i*31 + j*7)
		}
		if i > 0 {
			buf[0] |= 0x80 // 让它进长包头分支
		}
		_ = QUICClientHello(buf)
		_ = QUICSNI(buf)
	}
}
