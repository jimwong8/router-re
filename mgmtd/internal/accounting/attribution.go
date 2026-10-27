package accounting

import (
	"strconv"
	"strings"
)

// Entry 是 conntrack 的一条连接（只保留归属判定需要的字段）。
type Entry struct {
	SrcIP string
	DstIP string
	Bytes int64
}

// Attribute 把连接的字节数归到账号上。
//
// 规则：按 IP 查找绑定关系（bindings: IP -> username）。
// 一条连接同时涉及两个绑定 IP 时，**只按源 IP 归属**——请求方向（src）才代表
// 用户主动发起的流量，同时归 src 与 dst 会把一条连接在双向都计数（翻倍），
// 与 internal/conntrack 只取第一组 src/dst 的教训一致。目标（dst）是远端，
// 不应把它的字节也记到用户头上。
//
// 找不到归属（src 与 dst 都无绑定，或 src 无绑定）的条目跳过，但返回跳过数，
// 让调用方知道覆盖率（有多少流量无法归到任何账号）。
func Attribute(entries []Entry, bindings map[string]string) (perUser map[string]uint64, skipped int) {
	perUser = make(map[string]uint64)
	for _, e := range entries {
		// 只按源 IP 归属：dst 的绑定被忽略，避免双向重复计数。
		username, ok := bindings[e.SrcIP]
		if !ok {
			skipped++
			continue
		}
		if e.Bytes < 0 {
			continue // 防御：负数字节不累计（也不计 skipped，属数据异常）
		}
		perUser[username] += uint64(e.Bytes)
	}
	return perUser, skipped
}

// ParseConntrackBytes 解析 /proc/net/nf_conntrack 每行的 src 与 bytes= 字段。
//
// ⚠ 关键：conntrack 每行有两组 src/dst（请求+回复），bytes= 也可能出现两次。
// **必须取第一个 bytes=（请求方向）**——与 internal/conntrack 只取第一组
// src/dst 的教训一致，否则会拿到回复方向的字节数导致方向错误。
//
// 没有 bytes= 字段（未启用 nf_conntrack_acct）时返回 0 且不报错；但通过
// 第二返回值 acctEnabled=false 告诉调用方"accounting 未启用"，让上层如实
// 展示而不是显示 0 用量误导用户。
func ParseConntrackBytes(line string) (srcIP string, bytes int64, acctEnabled bool) {
	src := ""
	for _, f := range strings.Fields(line) {
		if strings.HasPrefix(f, "src=") {
			src = f[len("src="):]
			break // 只取第一组 src
		}
	}
	foundBytes := false
	var byteVal int64
	for _, f := range strings.Fields(line) {
		if strings.HasPrefix(f, "bytes=") {
			v, err := strconv.ParseInt(f[len("bytes="):], 10, 64)
			if err != nil {
				continue
			}
			byteVal = v
			foundBytes = true
			break // 只取第一个 bytes=（请求方向）
		}
	}
	if !foundBytes {
		return src, 0, false
	}
	return src, byteVal, true
}
