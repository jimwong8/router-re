// Package cfgsync 提供完整配置的导出/校验/差异对比与恢复规划（用于迁移与备份）。
// 它把配置表示成 Bundle（多个 section 的原始 JSON + checksum），并支持扁平化
// 逐字段 diff 与"先给方案、确认后执行"的安全恢复流程。
//
// 设计取舍（沿用 internal/config 的事务语义与 internal/conntrack 的注释风格）：
//   - Checksum 用 SHA256，必须**按 key 排序**后拼接 —— 否则 Go map 的遍历顺序
//     随机会让同一份配置算出不同 checksum（这是最容易错的地方，注释写清）。
//   - 恢复采取"Plan 先行 + confirm 显式确认"（ValidateApply），绝不静默执行。
//   - 危险 section（网络接口 / 防火墙 / NAT / 路由等）会单独标注，提示恢复后
//     可能与本机硬件不符导致失联。
package cfgsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// bundleVersion 是当前支持的 Bundle 版本。
const bundleVersion = 1

// Bundle 是一次完整配置导出（用于迁移/备份）。
type Bundle struct {
	Version   int                        `json:"version"`
	CreatedAt int64                      `json:"created_at"`
	Hostname  string                     `json:"hostname"`
	Sections  map[string]json.RawMessage `json:"sections"` // 例: "firewall" -> 原始 JSON
	Checksum  string                     `json:"checksum"`
}

// NewBundle 构造一个 Bundle 并计算 checksum。
func NewBundle(hostname string, now int64, sections map[string]json.RawMessage) Bundle {
	b := Bundle{
		Version:   bundleVersion,
		CreatedAt: now,
		Hostname:  hostname,
		Sections:  sections,
	}
	b.Checksum = b.ComputeChecksum()
	return b
}

// ComputeChecksum 把所有 section 的 key 排序后拼接 + Version + Hostname，
// 算 SHA256 hex。
//
// 为什么必须排序：Sections 是 map，Go 的 map 遍历顺序随每次迭代都可能是随机的。
// 如果不排序，同一份配置（同样的 section 内容）会因遍历顺序不同算出不同的
// checksum，导致"没改任何东西"却校验失败。强制按 key 排序保证确定性。
func (b *Bundle) ComputeChecksum() string {
	h := sha256.New()
	h.Write([]byte("v" + itoa(b.Version) + "|" + b.Hostname + "|"))
	keys := make([]string, 0, len(b.Sections))
	for k := range b.Sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write(b.Sections[k])
		h.Write([]byte(";"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Verify 重算 checksum 并与 b.Checksum 比较。任一 section 被篡改都会导致
// 重算结果不同而返回 false。
func (b Bundle) Verify() bool {
	if b.Checksum == "" {
		return false
	}
	return b.ComputeChecksum() == b.Checksum
}

// ValidateBundle 校验导入的包：
//   - Version 必须 == 1（不认识的版本直接拒绝，不尝试兼容）；
//   - Sections 非空；
//   - 每个 section 的值必须是合法 JSON；
//   - Checksum 必须验证通过。
func ValidateBundle(b Bundle) error {
	if b.Version != bundleVersion {
		return &bundleError{"unsupported bundle version: " + itoa(b.Version)}
	}
	if len(b.Sections) == 0 {
		return &bundleError{"bundle has no sections"}
	}
	for k, raw := range b.Sections {
		if len(raw) == 0 {
			return &bundleError{"section " + k + " is empty"}
		}
		if !json.Valid(raw) {
			return &bundleError{"section " + k + " is not valid JSON"}
		}
	}
	if !b.Verify() {
		return &bundleError{"checksum mismatch: bundle is corrupted or tampered"}
	}
	return nil
}

type bundleError struct{ msg string }

func (e *bundleError) Error() string { return e.msg }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ValidateHostname 校验 hostname 非空（供调用方检查导入包的来源标识）。
func ValidateHostname(b Bundle) error {
	if strings.TrimSpace(b.Hostname) == "" {
		return &bundleError{"bundle hostname is empty"}
	}
	return nil
}
