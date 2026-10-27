// Package firmware 提供固件镜像检查、版本比较、升级计划与执行。
package firmware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Image 描述一个固件镜像文件。
type Image struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
	Board   string `json:"board"`
	Magic   string `json:"magic"`
}

// InspectImage 读取镜像文件：算 sha256、大小、识别文件头魔数。
//
// OpenWrt sysupgrade 镜像通常是 tar 归档：按 POSIX tar 规范，偏移 257 处
// 有 6 字节 "ustar" 魔数。读前 512 字节判断。
// 文件不存在/读不到返回含路径的明确错误。
func InspectImage(path string) (Image, error) {
	img := Image{Path: path}
	fi, err := os.Stat(path)
	if err != nil {
		return img, fmt.Errorf("inspect image %s: %w", path, err)
	}
	if fi.IsDir() {
		return img, fmt.Errorf("inspect image %s: is a directory, not a file", path)
	}
	img.Size = fi.Size()

	// 读前 512 字节判魔数
	f, err := os.Open(path)
	if err != nil {
		return img, fmt.Errorf("inspect image %s: open: %w", path, err)
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]
	if len(head) >= 262 && bytes.HasPrefix(head[257:], []byte("ustar")) {
		img.Magic = "tar"
	} else if len(head) >= 3 && head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F' {
		img.Magic = "elf"
	} else if len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b {
		img.Magic = "gzip"
	} else {
		img.Magic = "unknown"
	}

	// sha256
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		m, err := f.Read(buf)
		if m > 0 {
			h.Write(buf[:m])
		}
		if err != nil {
			break
		}
	}
	img.SHA256 = hex.EncodeToString(h.Sum(nil))
	return img, nil
}

// ValidateImage 校验镜像能否用于本机升级。
//
//   - Size 须在 [1MB, 512MB]（太小不是固件，太大可能是错误文件）
//   - Board 非空且与本机 board 一致（不一致直接拒绝 —— **刷错板型会变砖**）
//   - Magic 必须被识别（tar/gzip/elf 之一，而非 unknown）
func ValidateImage(img Image, localBoard string) error {
	if img.Size < 1<<20 {
		return fmt.Errorf("image %s too small (%d bytes < 1MB) — not a firmware image", img.Path, img.Size)
	}
	if img.Size > 512<<20 {
		return fmt.Errorf("image %s too large (%d bytes > 512MB) — likely a wrong file", img.Path, img.Size)
	}
	switch img.Magic {
	case "tar", "gzip", "elf":
	default:
		return fmt.Errorf("image %s has unrecognized magic %q — refusing to flash", img.Path, img.Magic)
	}
	if strings.TrimSpace(img.Board) == "" {
		return fmt.Errorf("image %s board not detected", img.Path)
	}
	if img.Board != localBoard {
		return fmt.Errorf("board mismatch: image is for %q but this device is %q — flashing the wrong board type will brick the router; refusing", img.Board, localBoard)
	}
	return nil
}

// CompareVersion 按数字比较版本号（形如 24.10.8）。
//
// ⚠ 绝不能字符串比较：字符串比较会让 "24.10.10" < "24.10.8"（因为 "10" < "8" 按字典序）。
// 这里把每段转成数字逐段比，返回 -1/0/1。段数不同时用 0 补齐。
func CompareVersion(a, b string) (int, error) {
	as, err := versionParts(a)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", a, err)
	}
	bs, err := versionParts(b)
	if err != nil {
		return 0, fmt.Errorf("parse version %q: %w", b, err)
	}
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1, nil
		}
		if av > bv {
			return 1, nil
		}
	}
	return 0, nil
}

func versionParts(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("empty version")
	}
	segs := strings.Split(v, ".")
	var out []int
	for _, s := range segs {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("non-numeric segment %q in %q", s, v)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty version")
	}
	return out, nil
}
