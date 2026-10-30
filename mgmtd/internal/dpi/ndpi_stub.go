//go:build !linux || !cgo

package dpi

// ClassifyPacket 是非 cgo/非 linux 环境的桩实现。
// 与 ndpi.go 的 ClassifyPacket 同签名，但始终返回 ErrUnsupported，
// 保证 CGO_ENABLED=0 或非 linux 环境也能编译整个包。
func ClassifyPacket(pkt []byte) (protocol string, category string, err error) {
	return "", "", ErrUnsupported
}

// ClassifyFlow 非 cgo 环境的桩：没有 nDPI 就没有跨包识别能力。
// 返回 Unknown 而不是报错——调用方会把"没识别出来"当正常情况对待。
func ClassifyFlow(pkt []byte) (protocol string, category string, packets int, err error) {
	return "Unknown", "Unspecified", 0, ErrUnsupported
}

// Stats 非 cgo 环境下没有流表。
func Stats() FlowStats {
	return FlowStats{Max: 0}
}

// ResetFlows 非 cgo 环境下是空操作。
func ResetFlows() {}
