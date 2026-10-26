//go:build linux && cgo

package dpi

/*
#cgo pkg-config: libndpi
#include <stdlib.h>
#include <string.h>
#include <ndpi_api.h>
#include <ndpi_typedefs.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// ndpiModule 持有 nDPI 检测模块的单例。
// 每个 ClassifyPacket 调用复用同一个 module（初始化较重），
// 但每个包分配独立的 flow 结构以保持状态隔离。
type ndpiModule struct {
	mod *C.struct_ndpi_detection_module_struct
}

var (
	ndpiOnce    sync.Once
	ndpiGlobal  *ndpiModule
	ndpiInitErr error
)

// getNDPI 返回初始化的 nDPI 模块（单例，惰性初始化）。
func getNDPI() (*ndpiModule, error) {
	ndpiOnce.Do(func() {
		mod := C.ndpi_init_detection_module(C.ndpi_no_prefs)
		if mod == nil {
			ndpiInitErr = fmt.Errorf("ndpi_init_detection_module returned nil")
			return
		}
		C.ndpi_finalize_initialization(mod)
		ndpiGlobal = &ndpiModule{mod: mod}
	})
	return ndpiGlobal, ndpiInitErr
}

// ClassifyPacket 把原始 IP 包（含 IP 头 + 传输层头）喂给 nDPI，
// 返回识别出的协议名（如 "TLS"/"HTTP"/"BitTorrent"）与类别名（如 "Streaming"/"Download"）。
//
// 单包往往不足以让 nDPI 完成深度识别，此时返回 "Unknown" 与 "Unspecified"，
// err 为 nil（nDPI 无报错机制，未识别不算错误）。
//
// 注意：调用方必须保证 pkt 是一个完整的 IP 包（以 IPv4/IPv6 头开始）。
func ClassifyPacket(pkt []byte) (protocol string, category string, err error) {
	mod, err := getNDPI()
	if err != nil {
		return "", "", err
	}
	if len(pkt) == 0 {
		return "", "", fmt.Errorf("empty packet")
	}

	// 每包分配独立的 flow（单包分类，flow 无历史，用完释放）。
	// src/dst id 传 NULL：nDPI 允许（仅当不需要 per-host 统计时）。
	flow := C.ndpi_calloc(1, C.size_t(C.ndpi_detection_get_sizeof_ndpi_flow_struct()))
	if flow == nil {
		return "", "", fmt.Errorf("ndpi_calloc flow failed")
	}
	defer C.ndpi_free_flow((*C.struct_ndpi_flow_struct)(flow))

	// 包数据拷进 C 可寻址缓冲区，供 nDPI 读取。
	cLen := C.ushort(len(pkt))
	cBuf := C.malloc(C.size_t(len(pkt)))
	if cBuf == nil {
		return "", "", fmt.Errorf("malloc packet buffer failed")
	}
	defer C.free(cBuf)
	if len(pkt) > 0 {
		C.memcpy(cBuf, unsafe.Pointer(&pkt[0]), C.size_t(len(pkt)))
	}

	tMs := C.uint64_t(time.Now().UnixMilli())
	proto := C.ndpi_detection_process_packet(
		mod.mod,
		(*C.struct_ndpi_flow_struct)(flow),
		(*C.uchar)(cBuf),
		cLen,
		tMs,
	)

	// 协议名：ndpi_protocol2name 写入 buf
	var nameBuf [64]C.char
	C.ndpi_protocol2name(mod.mod, proto, &nameBuf[0], C.uint(64))
	protocol = C.GoString(&nameBuf[0])

	// 类别名：ndpi_get_proto_category → ndpi_category_get_name
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
