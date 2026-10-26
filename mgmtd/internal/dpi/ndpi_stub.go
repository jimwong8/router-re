//go:build !linux || !cgo

package dpi

// ClassifyPacket 是非 cgo/非 linux 环境的桩实现。
// 与 ndpi.go 的 ClassifyPacket 同签名，但始终返回 ErrUnsupported，
// 保证 CGO_ENABLED=0 或非 linux 环境也能编译整个包。
func ClassifyPacket(pkt []byte) (protocol string, category string, err error) {
	return "", "", ErrUnsupported
}
