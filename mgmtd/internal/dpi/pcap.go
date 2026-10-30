package dpi

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// pcap 文件解析（只读，够用即可）。
//
// 用途：把一段抓包交给 nDPI 回放。路由器上没法保证每条流都流经管理面，
// "上传一段 pcap 看看识别成什么"是最直接把问题说清楚的方式——
// 比在页面上争论"到底认不认得出抖音"有用。

// PCAPPacket 是一个解析出来的 IP 包与其时间戳。
type PCAPPacket struct {
	TS  time.Time
	IP  []byte // 从 IP 头开始（已剥链路层）
	Raw []byte
}

// pcapMagic 是四种常见字节序/精度组合。
const (
	magicMicroLE = 0xa1b2c3d4
	magicNanoLE  = 0xa1b23c4d
	magicMicroBE = 0xd4c3b2a1
	magicNanoBE  = 0x4d3cb2a1
)

// ReadPcap 解析 pcap 数据流。
//
// 链路类型支持：Ethernet(1)、Linux cooked(113)、BSD loopback/raw(0/108)。
// 其它类型返回错误，而不是"静默返回 0 个包"——后者会让用户以为抓包是空的。
func ReadPcap(r io.Reader) ([]PCAPPacket, error) {
	all, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, err
	}
	if len(all) < 24 {
		return nil, fmt.Errorf("不是有效的 pcap：文件头不足 24 字节")
	}

	var order binary.ByteOrder
	var nano bool
	switch binary.LittleEndian.Uint32(all[0:4]) {
	case magicMicroLE:
		order, nano = binary.LittleEndian, false
	case magicNanoLE:
		order, nano = binary.LittleEndian, true
	case magicMicroBE:
		order, nano = binary.BigEndian, false
	case magicNanoBE:
		order, nano = binary.BigEndian, true
	default:
		if binary.BigEndian.Uint32(all[0:4]) == magicMicroLE {
			order, nano = binary.LittleEndian, false
		} else {
			return nil, fmt.Errorf("不支持的 pcap 格式（magic=%08x）；pcapng 需要先转换", binary.LittleEndian.Uint32(all[0:4]))
		}
	}

	linkType := order.Uint32(all[20:24])

	switch linkType {
	case 0, 1, 12, 101, 108, 113, 228, 276:
	default:
		return nil, fmt.Errorf("不支持的链路类型 %d（支持以太网/VLAN/Linux cooked/裸 IP）", linkType)
	}

	var out []PCAPPacket
	off := 24
	for off+16 <= len(all) {
		tsSec := order.Uint32(all[off : off+4])
		tsFrac := order.Uint32(all[off+4 : off+8])
		incl := int(order.Uint32(all[off+8 : off+12]))
		off += 16
		if incl <= 0 || off+incl > len(all) {
			break
		}
		pkt := all[off : off+incl]
		off += incl

		ip, ok := stripLinkLayer(pkt, linkType)
		if !ok {
			continue
		}
		ns := int64(tsSec) * int64(time.Second)
		if nano {
			ns += int64(tsFrac)
		} else {
			ns += int64(tsFrac) * 1000
		}
		out = append(out, PCAPPacket{
			TS:  time.Unix(0, ns),
			IP:  ip,
			Raw: pkt,
		})
	}
	return out, nil
}

// stripLinkLayer 剥掉链路层头，返回从 IP 头开始的数据。
func stripLinkLayer(pkt []byte, linkType uint32) ([]byte, bool) {
	switch linkType {
	case 1: // Ethernet
		if len(pkt) < 14 {
			return nil, false
		}
		// 链路层类型字段永远是网络字节序，不能跟随 pcap 文件的字节序
		etype := binary.BigEndian.Uint16(pkt[12:14])
		hl := 14
		if etype == 0x8100 && len(pkt) >= 18 { // 一层 VLAN
			etype = binary.BigEndian.Uint16(pkt[16:18])
			hl = 18
		}
		if etype != 0x0800 && etype != 0x86dd {
			return nil, false
		}
		return pkt[hl:], true
	case 276: // Linux cooked capture v2 (SLL2)，"tcpdump -i any" 默认就是它
		if len(pkt) < 20 {
			return nil, false
		}
		etype := binary.BigEndian.Uint16(pkt[0:2])
		if etype != 0x0800 && etype != 0x86dd {
			return nil, false
		}
		return pkt[20:], true
	case 113: // Linux cooked capture (SLL)
		if len(pkt) < 16 {
			return nil, false
		}
		etype := binary.BigEndian.Uint16(pkt[14:16])
		if etype != 0x0800 && etype != 0x86dd {
			return nil, false
		}
		return pkt[16:], true
	case 0, 12, 101, 108, 228:
		// BSD loopback(0) / DLT_RAW(12) / LINKTYPE_RAW(101) / LOOP(108) / LINKTYPE_IPV4(228)
		// 这些链路类型下，抓到的就是裸 IP 包（tailscale/wireguard/tun 常见）
		return pkt, true
	}
	return nil, false
}

// FlowResult 是回放后按流聚合的结果。
type FlowResult struct {
	SrcIP    string `json:"src_ip"`
	DstIP    string `json:"dst_ip"`
	SrcPort  uint16 `json:"src_port"`
	DstPort  uint16 `json:"dst_port"`
	Protocol string `json:"protocol"`
	Category string `json:"category"`
	Packets  int    `json:"packets"`
	Bytes    int    `json:"bytes"`
	// Hosts 是从 TLS SNI / HTTP Host / DNS 里提到的域名（用于再查应用特征库）
	Hosts []string `json:"hosts,omitempty"`
}

// AnalyzePcap 回放抓包并给出每条流的识别结果。
//
// 这是"流状态是否生效"的最终体现：同一条连接的所有包会被累积识别，
// 最后一包的结果代表整条流。
func AnalyzePcap(pkts []PCAPPacket, classify func([]byte) (string, string, int, error)) []FlowResult {
	type acc struct {
		res   FlowResult
		key   FlowKey
		first bool
	}
	order := []FlowKey{}
	m := map[FlowKey]*acc{}

	for _, p := range pkts {
		key, ok := ParseFlowKey(p.IP)
		if !ok {
			continue
		}
		a := m[key]
		if a == nil {
			src, dst, sp, dp := endpointsOf(p.IP)
			a = &acc{
				key: key,
				res: FlowResult{
					SrcIP: src, DstIP: dst, SrcPort: sp, DstPort: dp,
					Protocol: "Unknown", Category: "Unspecified",
				},
			}
			m[key] = a
			order = append(order, key)
		}
		proto, cat, _, err := classify(p.IP)
		if err == nil && proto != "" && proto != "Unknown" {
			a.res.Protocol = proto
			a.res.Category = cat
		}
		a.res.Packets++
		a.res.Bytes += len(p.IP)
	}

	out := make([]FlowResult, 0, len(order))
	for _, k := range order {
		out = append(out, m[k].res)
	}
	return out
}

// endpointsOf 取出原始方向的地址与端口（聚合结果要显示"谁连了谁"）。
func endpointsOf(pkt []byte) (src, dst string, sp, dp uint16) {
	if len(pkt) < 20 {
		return "", "", 0, 0
	}
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0f) * 4
		if len(pkt) < ihl+4 {
			return "", "", 0, 0
		}
		src = fmt.Sprintf("%d.%d.%d.%d", pkt[12], pkt[13], pkt[14], pkt[15])
		dst = fmt.Sprintf("%d.%d.%d.%d", pkt[16], pkt[17], pkt[18], pkt[19])
		sp = binary.BigEndian.Uint16(pkt[ihl : ihl+2])
		dp = binary.BigEndian.Uint16(pkt[ihl+2 : ihl+4])
	case 6:
		if len(pkt) < 44 {
			return "", "", 0, 0
		}
		src = formatIPv6(pkt[8:24])
		dst = formatIPv6(pkt[24:40])
		sp = binary.BigEndian.Uint16(pkt[40:42])
		dp = binary.BigEndian.Uint16(pkt[42:44])
	}
	return
}

func formatIPv6(b []byte) string {
	if len(b) < 16 {
		return ""
	}
	out := make([]byte, 0, 8)
	for i := 0; i < 16; i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		v := binary.BigEndian.Uint16(b[i : i+2])
		out = append(out, []byte(fmt.Sprintf("%x", v))...)
	}
	return string(out)
}
