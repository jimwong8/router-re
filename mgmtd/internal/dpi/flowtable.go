package dpi

import (
	"encoding/binary"
	"time"
)

// 流表：nDPI 的识别能力建立在"跨包累积状态"上。
//
// 为什么必须有这张表：nDPI 的检测逻辑要看好几个包才能定性——
// TLS 要等 ClientHello、HTTP 要等 Host 头、很多协议要看到第 2-4 个包。
// 如果每个包都新建一个 flow 结构，那么每个包都是"流的第一包"，
// 除了端口能猜的几个协议之外几乎全都识别成 Unknown。
//
// 这里把 5 元组映射到同一个 flow 结构，让 nDPI 有连续上下文。

// FlowKey 是一条流的标识。正反向归一化到同一个 key：
// 客户端发的第一个包和服务器回的第一个包必须落在同一 entry 上，
// 否则双向就是两条流，nDPI 看不到应答就无从判断。
type FlowKey struct {
	IPLo   [16]byte // 排序后较小的那个地址（保证正反向同 key）
	IPHi   [16]byte
	PortLo uint16
	PortHi uint16
	Proto  uint8 // 6=TCP 17=UDP
	IsIPv6 bool
}

// ParseFlowKey 从原始 IP 包解析出归一化的 5 元组。
//
// 只接受 IPv4/IPv6 上的 TCP/UDP；其它（ICMP、分片后续包）返回 ok=false，
// 调用方应直接跳过——它们对应用识别没有价值。
func ParseFlowKey(pkt []byte) (FlowKey, bool) {
	if len(pkt) < 20 {
		return FlowKey{}, false
	}
	switch pkt[0] >> 4 {
	case 4:
		ihl := int(pkt[0]&0x0f) * 4
		if ihl < 20 || len(pkt) < ihl {
			return FlowKey{}, false
		}
		// 分片包（MF 置位或 offset!=0）不完整，跳过
		flagsFrag := binary.BigEndian.Uint16(pkt[6:8])
		if flagsFrag&0x3fff != 0 {
			return FlowKey{}, false
		}
		proto := pkt[9]
		if proto != 6 && proto != 17 {
			return FlowKey{}, false
		}
		if len(pkt) < ihl+4 {
			return FlowKey{}, false
		}
		var src, dst [16]byte
		copy(src[:4], pkt[12:16])
		copy(dst[:4], pkt[16:20])
		sp := binary.BigEndian.Uint16(pkt[ihl : ihl+2])
		dp := binary.BigEndian.Uint16(pkt[ihl+2 : ihl+4])
		return normalize(src, dst, sp, dp, proto, false), true
	case 6:
		if len(pkt) < 40 {
			return FlowKey{}, false
		}
		proto := pkt[6]
		if proto != 6 && proto != 17 {
			return FlowKey{}, false
		}
		var src, dst [16]byte
		copy(src[:], pkt[8:24])
		copy(dst[:], pkt[24:40])
		sp := binary.BigEndian.Uint16(pkt[40:42])
		dp := binary.BigEndian.Uint16(pkt[42:44])
		return normalize(src, dst, sp, dp, proto, true), true
	}
	return FlowKey{}, false
}

func normalize(src, dst [16]byte, sp, dp uint16, proto uint8, v6 bool) FlowKey {
	if compareAddr(src, dst) > 0 || (src == dst && sp > dp) {
		src, dst = dst, src
		sp, dp = dp, sp
	}
	return FlowKey{IPLo: src, IPHi: dst, PortLo: sp, PortHi: dp, Proto: proto, IsIPv6: v6}
}

func compareAddr(a, b [16]byte) int {
	for i := 0; i < 16; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// FlowStats 是流表状态，供管理接口展示。
type FlowStats struct {
	Active     int `json:"active"`
	Max        int `json:"max"`
	IdleSec    int `json:"idle_sec"`
	Identified int `json:"identified"`
}

// FlowPolicy 控制流表的容量与老化。
type FlowPolicy struct {
	MaxFlows    int           // 同时在跟踪的流上限
	IdleTimeout time.Duration // 超过这么久没包就回收
	ScanEvery   time.Duration // 多久扫一次空闲流
}

// DefaultFlowPolicy 的参数取自 nDPI 官方集成示例的量级：
// 单线程 2048 条活跃流、5 分钟空闲回收。路由器上流量集中但流数不大，
// 放大到 8192 足够覆盖高峰。
func DefaultFlowPolicy() FlowPolicy {
	return FlowPolicy{
		MaxFlows:    8192,
		IdleTimeout: 5 * time.Minute,
		ScanEvery:   30 * time.Second,
	}
}

// shouldEvict 判断某条流是否该回收。
func (p FlowPolicy) shouldEvict(lastSeen, now time.Time) bool {
	return now.Sub(lastSeen) > p.IdleTimeout
}

// isFull 判断是否到达容量上限（留 10% 余量触发提前清理）。
func (p FlowPolicy) isFull(n int) bool {
	return n >= p.MaxFlows*9/10
}
