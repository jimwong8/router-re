//go:build linux && cgo

package dpi

/*
#cgo pkg-config: libndpi
#include <stdlib.h>
#include <string.h>
#include <ndpi_api.h>
#include <ndpi_typedefs.h>
#include <stdio.h>

// 显式启用全部协议检测。
//
// 这是本包最关键的一行：ndpi_init_detection_module() 之后如果不显式设置
// 检测位图，检测回调列表是空的——nDPI 能正确解析 IP/TCP 头（flow->l4_proto
// 会变成 6），但永远不会命中任何协议，结果永远是 Unknown。
// 默认位图在不同版本/构建里并不一致，所以必须显式设成全启用。
static void rr_enable_all_protocols(struct ndpi_detection_module_struct *m) {
	NDPI_PROTOCOL_BITMASK all;
	NDPI_BITMASK_SET_ALL(all);
	ndpi_set_protocol_detection_bitmask2(m, &all);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// nDPI 集成层。
//
// 关键点不在于"调通 API"，而在于**把同一条连接的所有包喂给同一个 flow 结构**。
// nDPI 的检测是累积式的：TLS 要等 ClientHello、HTTP 要等 Host 头、
// 多数协议要看到第 2-4 个包。每个包新建 flow 等于每个包都是"第一包"，
// 除了端口能猜的少数协议，其余一律 Unknown。

// ndpiModule 持有 nDPI 检测模块的单例（初始化较重，进程内复用）。
type ndpiModule struct {
	mod *C.struct_ndpi_detection_module_struct
}

var (
	ndpiOnce    sync.Once
	ndpiGlobal  *ndpiModule
	ndpiInitErr error
)

// getNDPI 返回初始化的 nDPI 模块（惰性初始化）。
func getNDPI() (*ndpiModule, error) {
	ndpiOnce.Do(func() {
		mod := C.ndpi_init_detection_module(C.ndpi_no_prefs)
		if mod == nil {
			ndpiInitErr = fmt.Errorf("ndpi_init_detection_module returned nil")
			return
		}
		C.rr_enable_all_protocols(mod)
		C.ndpi_finalize_initialization(mod)
		ndpiGlobal = &ndpiModule{mod: mod}
	})
	return ndpiGlobal, ndpiInitErr
}

// cFlow 是一条被跟踪的流。
type cFlow struct {
	flow    *C.struct_ndpi_flow_struct
	last    time.Time
	packets uint32
	proto   string
	cat     string
	done    bool // 已识别：后续包不再送进 nDPI，只更新时间戳
}

var (
	flowMu   sync.Mutex
	flowMap  = map[FlowKey]*cFlow{}
	lastScan time.Time
	policy   = DefaultFlowPolicy()
)

// Stats 返回当前流表快照。
func Stats() FlowStats {
	flowMu.Lock()
	defer flowMu.Unlock()
	n, done := 0, 0
	for _, e := range flowMap {
		n++
		if e.done {
			done++
		}
	}
	return FlowStats{
		Active: n, Max: policy.MaxFlows,
		IdleSec: int(policy.IdleTimeout / time.Second), Identified: done,
	}
}

// ResetFlows 清空流表（测试用）。
func ResetFlows() {
	flowMu.Lock()
	defer flowMu.Unlock()
	for k, e := range flowMap {
		C.ndpi_flow_free(unsafe.Pointer(e.flow))
		delete(flowMap, k)
	}
	lastScan = time.Time{}
}

// scanLocked 回收空闲流。调用方必须已持锁。
func scanLocked(now time.Time) {
	for k, e := range flowMap {
		if policy.shouldEvict(e.last, now) {
			C.ndpi_free_flow(e.flow)
			delete(flowMap, k)
		}
	}
	lastScan = now
}

// ClassifyFlow 处理一个包并返回该流当前已知的协议。
//
// 返回 "Unknown" 不是错误，只表示"还不够多包，或本来就不认识"。
// 调用方应在同一条流的后续包上继续调用——识别会在某个包上突然完成。
// packets 是该流已处理的包数。
func ClassifyFlow(pkt []byte) (protocol, category string, packets int, err error) {
	mod, err := getNDPI()
	if err != nil {
		return "", "", 0, err
	}
	if len(pkt) == 0 {
		return "Unknown", "Unspecified", 0, nil
	}
	key, ok := ParseFlowKey(pkt)
	if !ok {
		// 非 TCP/UDP、分片包、非 IP：不参与应用识别，也不算错误
		return "Unknown", "Unspecified", 0, nil
	}

	now := time.Now()

	flowMu.Lock()
	defer flowMu.Unlock()

	if lastScan.IsZero() || now.Sub(lastScan) > policy.ScanEvery || policy.isFull(len(flowMap)) {
		scanLocked(now)
	}

	e := flowMap[key]
	if e == nil {
		if len(flowMap) >= policy.MaxFlows {
			// 表满：放弃跟踪新流，也不要无界增长
			return "Unknown", "Unspecified", 0, nil
		}
		raw := C.ndpi_flow_malloc(C.size_t(C.ndpi_detection_get_sizeof_ndpi_flow_struct()))
		if raw == nil {
			return "", "", 0, fmt.Errorf("ndpi flow alloc failed")
		}
		C.memset(raw, 0, C.size_t(C.ndpi_detection_get_sizeof_ndpi_flow_struct()))
		e = &cFlow{flow: (*C.struct_ndpi_flow_struct)(raw)}
		flowMap[key] = e
	}

	e.last = now
	e.packets++

	if e.done {
		return e.proto, e.cat, int(e.packets), nil
	}

	cLen := C.ushort(len(pkt))
	cBuf := C.malloc(C.size_t(len(pkt)))
	if cBuf == nil {
		return "", "", int(e.packets), fmt.Errorf("malloc packet buffer failed")
	}
	defer C.free(cBuf)
	C.memcpy(cBuf, unsafe.Pointer(&pkt[0]), C.size_t(len(pkt)))

	tMs := C.uint64_t(uint64(now.UnixMilli()))
	proto := C.ndpi_detection_process_packet(
		mod.mod, e.flow, (*C.uchar)(cBuf), cLen, tMs,
	)

	var nameBuf [64]C.char
	C.ndpi_protocol2name(mod.mod, proto, &nameBuf[0], C.uint(64))
	protocol = C.GoString(&nameBuf[0])

	cat := C.ndpi_get_proto_category(mod.mod, proto)
	catName := C.ndpi_category_get_name(mod.mod, cat)
	category = C.GoString(catName)

	if protocol == "" {
		protocol = "Unknown"
	}
	if category == "" {
		category = "Unspecified"
	}
	if protocol != "Unknown" {
		e.done = true
		e.proto = protocol
		e.cat = category
	}
	return protocol, category, int(e.packets), nil
}

// ClassifyPacket 是单包分类：给一个孤立的包，不参与流跟踪。
//
// 保留它是为了兼容「上传一段载荷试试能不能识别」这类接口。
// 真正处理流量请用 ClassifyFlow——单包看不到跨包状态，除端口能猜的协议外基本全是 Unknown。
func ClassifyPacket(pkt []byte) (protocol, category string, err error) {
	mod, err := getNDPI()
	if err != nil {
		return "", "", err
	}
	if len(pkt) == 0 {
		return "", "", fmt.Errorf("empty packet")
	}

	flow := C.ndpi_calloc(1, C.size_t(C.ndpi_detection_get_sizeof_ndpi_flow_struct()))
	if flow == nil {
		return "", "", fmt.Errorf("ndpi_calloc flow failed")
	}
	defer C.ndpi_free_flow((*C.struct_ndpi_flow_struct)(flow))

	cLen := C.ushort(len(pkt))
	cBuf := C.malloc(C.size_t(len(pkt)))
	if cBuf == nil {
		return "", "", fmt.Errorf("malloc packet buffer failed")
	}
	defer C.free(cBuf)
	C.memcpy(cBuf, unsafe.Pointer(&pkt[0]), C.size_t(len(pkt)))

	tMs := C.uint64_t(uint64(time.Now().UnixMilli()))
	proto := C.ndpi_detection_process_packet(
		mod.mod,
		(*C.struct_ndpi_flow_struct)(flow),
		(*C.uchar)(cBuf),
		cLen,
		tMs,
	)

	var nameBuf [64]C.char
	C.ndpi_protocol2name(mod.mod, proto, &nameBuf[0], C.uint(64))
	protocol = C.GoString(&nameBuf[0])

	cat := C.ndpi_get_proto_category(mod.mod, proto)
	catName := C.ndpi_category_get_name(mod.mod, cat)
	category = C.GoString(catName)

	if protocol == "" {
		protocol = "Unknown"
	}
	if category == "" {
		category = "Unspecified"
	}
	return protocol, category, nil
}
