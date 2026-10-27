// Package threat 提供威胁情报与 IP 封禁功能：单一来源的封禁条目、内存封禁表、
// 到 nftables / hosts.deny 的渲染，以及纯文本情报源抓取与解析。
//
// 设计取舍（沿用 internal/traffic / internal/remote 的注释风格）：
//   - 封禁关键操作（blocklist 维护、nft 下发）以 execx.Runner 为外层执行差异点，
//     解析/匹配是本包纯 Go 实现，可脱离真实系统单测。
//   - **安全红线**：
//     ① 绝不写 `flush ruleset`（会清空整机所有表）；
//     ② 绝不 `policy drop`（只 drop 命中集合的流量，默认放行，避免把整机封死）；
//     ③ **必须拒绝 0.0.0.0/0 与 ::/0**（封全网等于自断，是能锁死机器的操作）。
//   - 情报源 URL 只允许 http/https，防 SSRF 到 file:// / gopher:// 等本机协议。
package threat

import (
	"fmt"
	"net/netip"
)

// Entry 是一条封禁条目。
type Entry struct {
	IP        string `json:"ip"`         // 单个 IP 或 CIDR，如 "203.0.113.5" 或 "203.0.113.0/24"
	Reason    string `json:"reason"`     // 封禁原因，≤200 字符
	Source    string `json:"source"`     // 来源（手工 / 情报源名 / 导入）
	CreatedAt int64  `json:"created_at"` // Unix 秒
	ExpiresAt int64  `json:"expires_at"` // Unix 秒；0 表示永久
	Hits      int64  `json:"hits"`       // 命中次数
}

// maxReasonLen 是 Reason 允许的最大长度。
const maxReasonLen = 200

// ValidateEntry 校验一条封禁条目。
//
// 关键安全检查：**必须拒绝 0.0.0.0/0 和 ::/0**。把"全网"加入 drop 集合等于切断所有
// 进出流量——任何误触都会让整台路由器（及其背后的网络）失联，这属于能锁死机器的
// 高危操作，必须在写入前拦下，而不只是依赖 nft 层兜底。
func ValidateEntry(e Entry) error {
	// IP 支持单个地址或 CIDR：ParsePrefix 同时覆盖两者（把单 IP 看成 /32 或 /128）。
	prefix, err := netip.ParsePrefix(e.IP)
	if err != nil {
		// netip.ParsePrefix("1.2.3.4") 会失败，需退化为 ParseAddr。
		addr, addrErr := netip.ParseAddr(e.IP)
		if addrErr != nil {
			return fmt.Errorf("ip %q 非法（既不是合法 IP 也不是合法 CIDR）", e.IP)
		}
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		prefix = netip.PrefixFrom(addr, bits)
	}

	// 拒绝"封全网"：IPv4 的 /0 与 IPv6 的 /0。
	if prefix.Bits() == 0 {
		kind := "IPv6"
		if prefix.Addr().Is4() {
			kind = "IPv4"
		}
		return fmt.Errorf("拒绝封禁 %s 全网 %q：这会阻断所有进出流量并锁死本机，无法远程恢复", kind, e.IP)
	}
	// 额外的保护：拒绝裸的"未指定地址" 0.0.0.0 / ::。它们是 /0 的单点写法，在
	// nftables 区间集合里同样会退化成"匹配一切"，封掉等于断开整机网络。
	if prefix.Addr().IsUnspecified() {
		return fmt.Errorf("拒绝封禁未指定地址 %q（等价于 %s 全网）：这会阻断所有进出流量并锁死本机", e.IP, prefix.Addr().Unmap())
	}

	if len(e.Reason) > maxReasonLen {
		return fmt.Errorf("reason 长度 %d 超过上限 %d", len(e.Reason), maxReasonLen)
	}

	if e.ExpiresAt != 0 && e.ExpiresAt <= e.CreatedAt {
		return fmt.Errorf("expires_at 必须大于 created_at（当前 created=%d expires=%d）", e.CreatedAt, e.ExpiresAt)
	}
	return nil
}

// Expired 报告条目是否已过期。ExpiresAt 为 0 表示永久不过期；否则 now>=ExpiresAt
// 即视为过期（边界含等号：正好到点就算过期）。
func Expired(e Entry, now int64) bool {
	return e.ExpiresAt > 0 && now >= e.ExpiresAt
}

// Prune 移除已过期条目，返回新列表与移除数量。输入切片不被修改。
func Prune(entries []Entry, now int64) ([]Entry, int) {
	out := make([]Entry, 0, len(entries))
	removed := 0
	for _, e := range entries {
		if Expired(e, now) {
			removed++
			continue
		}
		out = append(out, e)
	}
	return out, removed
}
