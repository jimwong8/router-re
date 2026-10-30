package dpi

import (
	"encoding/binary"
	"fmt"
	"time"
)

// pcapng 文件解析。
//
// 为什么必须有：Wireshark 默认存的就是 pcapng，tcpdump -w 才是 pcap。
// 只支持 pcap 意味着"上传界面自己抓的包"经常直接报错，用户还得先装个工具转换。
//
// 块结构（所有块 4 字节对齐）：
//
//	type(4) total_len(4) body... total_len(4)
//
// 只处理四类块，其余（NRB/ISB 等）按长度跳过——不认识不等于文件坏了。

const (
	blockSHB uint32 = 0x0A0D0D0A // Section Header
	blockIDB uint32 = 0x00000001 // Interface Description
	blockEPB uint32 = 0x00000006 // Enhanced Packet
	blockSPB uint32 = 0x00000003 // Simple Packet
	blockPB  uint32 = 0x00000002 // obsolete Packet Block
)

// pcapngIface 是一个接口定义（一个文件里可以有多块网卡）。
type pcapngIface struct {
	linkType uint32
	tsDiv    uint64 // 时间戳单位的分母：1e6=微秒，1e9=纳秒
}

// ReadPcapng 解析 pcapng 数据。
func ReadPcapng(all []byte) ([]PCAPPacket, error) {
	if len(all) < 28 {
		return nil, fmt.Errorf("不是有效的 pcapng：文件头不足")
	}
	var out []PCAPPacket
	var order binary.ByteOrder
	var ifaces []pcapngIface

	off := 0
	for off+12 <= len(all) {
		// 第一个块必须是 SHB，字节序从它的 BOM 里读出来；
		// 在此之前没有任何可用的 ByteOrder，拿它去解块头就会空指针。
		if order == nil {
			if binary.BigEndian.Uint32(all[off:off+4]) != blockSHB {
				return nil, fmt.Errorf("pcapng 缺少 Section Header Block（文件不以 0x0A0D0D0A 开头）")
			}
			bom := binary.LittleEndian.Uint32(all[off+8 : off+12])
			switch bom {
			case 0x1A2B3C4D:
				order = binary.LittleEndian
			case 0x4D3C2B1A:
				order = binary.BigEndian
			default:
				return nil, fmt.Errorf("pcapng 字节序标记异常（%08x），文件可能已损坏", bom)
			}
		}

		btype := order.Uint32(all[off : off+4])
		if btype == blockSHB {
			// 新 section：字节序可能变，接口定义全部失效
			bom := binary.LittleEndian.Uint32(all[off+8 : off+12])
			switch bom {
			case 0x1A2B3C4D:
				order = binary.LittleEndian
			case 0x4D3C2B1A:
				order = binary.BigEndian
			default:
				return nil, fmt.Errorf("pcapng 字节序标记异常（%08x），文件可能已损坏", bom)
			}
			ifaces = nil
		}

		blen := int(order.Uint32(all[off+4 : off+8]))
		if blen < 12 || blen%4 != 0 {
			return nil, fmt.Errorf("pcapng 块长度异常：%d（偏移 %d）", blen, off)
		}
		if off+blen > len(all) {
			// 截断的文件：已经解析出来的部分照样返回，不整体失败
			break
		}
		body := all[off+8 : off+blen-4]

		switch btype {
		case blockSHB:
			// 帧头已处理，块内没有其它需要的东西

		case blockIDB:
			if len(body) < 8 {
				break
			}
			lt := uint32(order.Uint16(body[0:2]))
			iface := pcapngIface{linkType: lt, tsDiv: 1e6}
			if res, ok := pcapngOption(body[8:], order, 9); ok && len(res) >= 1 {
				// 最高位为 1 表示二进制底（2^-n 秒），否则十进制的 10^-n 秒
				exp := res[0] & 0x7f
				if res[0]&0x80 != 0 {
					if exp < 63 {
						iface.tsDiv = 1 << exp
					}
				} else if exp <= 18 {
					iface.tsDiv = 1
					for i := uint8(0); i < exp; i++ {
						iface.tsDiv *= 10
					}
				}
			}
			ifaces = append(ifaces, iface)

		case blockEPB:
			if len(body) < 20 {
				break
			}
			ifIdx := int(order.Uint32(body[0:4]))
			tsHigh := uint64(order.Uint32(body[4:8]))
			tsLow := uint64(order.Uint32(body[8:12]))
			capLen := int(order.Uint32(body[12:16]))
			if ifIdx >= len(ifaces) || capLen < 0 || 20+capLen > len(body) {
				break
			}
			data := body[20 : 20+capLen]
			ts := pcapngTime(tsHigh<<32|tsLow, ifaces[ifIdx].tsDiv)
			if ip, ok := stripLinkLayer(data, ifaces[ifIdx].linkType); ok {
				out = append(out, PCAPPacket{TS: ts, IP: ip, Raw: data})
			}

		case blockSPB:
			if len(body) < 4 || len(ifaces) == 0 {
				break
			}
			// SPB 没有时间戳，也没有 caplen，整块 body 去掉 4 字节 origlen 后就是数据
			data := body[4:]
			if ip, ok := stripLinkLayer(data, ifaces[0].linkType); ok {
				out = append(out, PCAPPacket{IP: ip, Raw: data})
			}

		case blockPB:
			// 老式 Packet Block：iface(2) drops(2) ts_high(4) ts_low(4) caplen(4) origlen(4) data
			if len(body) < 20 {
				break
			}
			ifIdx := int(order.Uint16(body[0:2]))
			tsHigh := uint64(order.Uint32(body[4:8]))
			tsLow := uint64(order.Uint32(body[8:12]))
			capLen := int(order.Uint32(body[12:16]))
			if ifIdx >= len(ifaces) || capLen < 0 || 20+capLen > len(body) {
				break
			}
			data := body[20 : 20+capLen]
			ts := pcapngTime(tsHigh<<32|tsLow, ifaces[ifIdx].tsDiv)
			if ip, ok := stripLinkLayer(data, ifaces[ifIdx].linkType); ok {
				out = append(out, PCAPPacket{TS: ts, IP: ip, Raw: data})
			}
		}

		off += blen
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("pcapng 里没有解析出 IP 包（可能是空文件或只含非 IP 流量）")
	}
	return out, nil
}

// pcapngOption 在选项区里找指定 code 的值。
// 选项格式：code(2) len(2) value(padding 到 4)。
func pcapngOption(b []byte, order binary.ByteOrder, want uint16) ([]byte, bool) {
	off := 0
	for off+4 <= len(b) {
		code := order.Uint16(b[off : off+2])
		l := int(order.Uint16(b[off+2 : off+4]))
		off += 4
		if code == 0 { // opt_endofopt
			return nil, false
		}
		if off+l > len(b) {
			return nil, false
		}
		if code == want {
			return b[off : off+l], true
		}
		off += l
		off += (4 - l%4) % 4
	}
	return nil, false
}

// pcapngTime 把原始时间戳换算成 time.Time。
func pcapngTime(v, div uint64) time.Time {
	if div == 0 {
		div = 1e6
	}
	sec := v / div
	frac := v % div
	var nsec uint64
	if frac != 0 {
		// frac < div，乘 1e9 一般不会溢出；div 极大时退化成浮点，精度够用
		if div <= 1e12 {
			nsec = frac * 1e9 / div
		} else {
			nsec = uint64(float64(frac) / float64(div) * 1e9)
		}
	}
	return time.Unix(int64(sec), int64(nsec))
}
